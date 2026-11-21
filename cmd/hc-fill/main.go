package main

// 명령 나누기와 add·rm·prio·watch·run 잠금 — 셸 판 bin/hc-fill 앞부분

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/version"
)

var heldLocks []*os.File // 프로세스가 끝날 때까지 쥐는 잠금(GC가 닫지 않게)

func main() {
	closeInherited()
	proc.Who = "hc-fill"
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("hc-fill")) // 릴리스 바이너리는 커밋 12자(gofix-fill F19)
		return
	}
	// 문자열 비교는 바이트로(셸 판과 같은 환경을 자식에게도)
	if v, ok := os.LookupEnv("LC_ALL"); ok && v != "" {
		os.Setenv("LC_CTYPE", v)
	}
	os.Unsetenv("LC_ALL")
	os.Setenv("LC_COLLATE", "C")

	HOME = os.Getenv("HOME")
	cdir := os.Getenv("HERDR_TOOLS_CONFIG")
	if cdir == "" {
		cdir = HOME + "/.config/herdr-tools"
	}
	CONF = cdir + "/hc-fill.tsv"
	DATA = os.Getenv("HC_FILL_DATA")
	if DATA == "" {
		DATA = HOME + "/.local/share/hc-fill"
	}
	Q, LED, AT = DATA+"/queue.tsv", DATA+"/inbox.tsv", DATA+"/audit.tsv"
	if v := os.Getenv("HC_FILL_WAIT"); v != "" {
		WAIT, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("HC_FILL_POLL"); v != "" {
		POLL, _ = strconv.Atoi(v)
	}
	SELF = os.Getenv("HC_FILL_SELF")
	if SELF == "" {
		if e, err := os.Executable(); err == nil {
			SELF = e
		} else {
			SELF = os.Args[0]
		}
	}

	text, ok := readFile(CONF)
	if !isFile(CONF) || !ok {
		die("설정 파일이 없다: %s (저장소 config.example/hc-fill.tsv를 복사해 값을 고친다)", CONF)
	}
	confLines = lines(text)
	single := map[string]bool{}
	for _, k := range strings.Fields("local agent coord task launch gc early early_wait idle hold audit audit_max audit_final wake_every ack_max overlap done_stall inbox_audit") {
		single[k] = true
	}
	cntk := map[string]int{}
	var dup []string
	for _, l := range confLines {
		if f := fields(l); len(f) > 0 && single[f[0]] {
			if cntk[f[0]]++; cntk[f[0]] == 2 {
				dup = append(dup, f[0])
			}
		}
	}
	if len(dup) > 0 {
		die("설정에 %s 줄이 두 번 있다: %s", strings.Join(dup, " "), CONF)
	}
	LOCAL, AGENT = conf("local"), conf("agent")
	if AGENT == "" {
		AGENT = "claude-code"
	}
	if LOCAL == "" {
		die("설정에 local 줄이 없다: %s", CONF)
	}

	cmd, args := "", []string(nil)
	if len(os.Args) > 1 {
		cmd, args = os.Args[1], os.Args[2:]
	}
	switch cmd {
	case "_gc":
		gcMain(args)
	case "add":
		addCmd(args)
	case "rm":
		if len(args) != 1 {
			usage()
		}
		if withQ(func() { qRm(args[0]) }) != 0 {
			exit(1)
		}
		say("뺌 %s (떠 있는 터미널은 그대로)", args[0])
	case "prio":
		prioCmd(args)
	case "ls":
		lsCmd(args)
	case "watch":
		watchCmd(args)
	case "run":
		runCmd(args)
	default:
		usage()
	}
	exit(0)
}

func prioCmd(args []string) {
	if len(args) != 2 {
		usage()
	}
	p, ok := pnorm(args[1])
	if !ok {
		die("우선순위는 긴급|높음|보통|낮음(urgent|high|normal|low): %s", args[1])
	}
	rc := withQ(func() {
		r := qrow(args[0])
		st := ""
		if r != nil {
			st = r.F(6)
		}
		if st == "" {
			die("대기열에 없다: %s", args[0])
		}
		if st != "wait" {
			die("대기 중이 아니다(%s): %s — 우선순위는 띄우기 전에만 바꾼다", st, args[0])
		}
		var out []Row
		for _, x := range qRowsW() {
			if x.F(1) == args[0] {
				for i := x.NF() + 1; i < 13; i++ {
					x = x.Set(i, "-")
				}
				x = x.Set(13, p)
			}
			out = append(out, x)
		}
		if writeAtomic(Q, rowsText(out)) != nil {
			exit(1)
		}
	})
	if rc != 0 {
		exit(1)
	}
	say("우선 %s → %s", args[0], plabel(p))
}

func watchCmd(args []string) {
	every := "60"
	for len(args) > 0 {
		if args[0] == "--every" && len(args) >= 2 {
			every, args = args[1], args[2:]
			continue
		}
		usage()
	}
	if !isDigits(every) {
		usage()
	}
	if err := os.MkdirAll(DATA, 0o755); err != nil {
		die("%v", err)
	}
	f, busy, err := lockx.TryLock(DATA + "/watch.lock")
	if err != nil {
		die("watch 잠금: %v", err)
	}
	if busy {
		fmt.Fprintf(os.Stderr, "hc-fill: watch가 이미 돌고 있다(%s/watch.lock)\n", DATA)
		exit(75)
	}
	heldLocks = append(heldLocks, f)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sig
		proc.StopCurrent(5 * time.Second) // 돌던 run 무리째 끈다
		os.Exit(128 + int(s.(syscall.Signal)))
	}()
	sec := atoi10(every)
	// 바퀴 상한(go-followups-2): 한 바퀴가 이보다 길면 그 바퀴를 끝내고 다음 바퀴가 이어 간다. 바깥 명령마다 상한(proc.Limit)이
	// 있으니 이것은 마지막 울타리 — 레인 몇 개를 띄우며 herdr-setup.sh를 기다리는 바퀴는 넉넉히 둔다
	roundMax := 20 * time.Minute
	if v := os.Getenv("HC_FILL_ROUND_TIMEOUT"); isDigits(v) {
		roundMax = time.Duration(atoi10(v)) * time.Second
	}
	for {
		say("--- %s", stampLog())
		c := exec.Command(SELF, "run")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		proc.WaitMax(c, "", roundMax)
		time.Sleep(time.Duration(sec) * time.Second)
	}
}

func runCmd(args []string) {
	for _, a := range args {
		if a != "--dry-run" {
			usage()
		}
		dry = true
	}
	if err := os.MkdirAll(DATA, 0o755); err != nil {
		die("%v", err)
	}
	f, busy, err := lockx.TryLock(DATA + "/run.lock")
	if err != nil {
		die("run 잠금: %v", err)
	}
	if busy {
		fmt.Fprintln(os.Stderr, "hc-fill: 다른 run이 돌고 있다, 이번 바퀴는 건너뜀")
		exit(75)
	}
	heldLocks = append(heldLocks, f)
	// 바퀴 상한·watch 멈춤으로 TERM을 받으면: 상한 걸고 따로 무리로 띄운 자식(git·hc 등 — 이 무리 신호가 안 닿는다)까지 거두고 끝낸다
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sig
		// 끝내기 훅(held.tsv 비움·지시문 사본 지움 — 셸 판 EXIT trap과 같게)을 먼저: internal/proc도 같은 신호를 받아 무리를 끈 뒤
		// 신호를 다시 올려 끝낸다 — 무리 끄기(TERM 뒤 끝날 때까지)를 기다리면 훅이 못 돈다.
		// ponytail: 파일 둘이라 proc 쪽보다 먼저 끝나는 것에 기댄다 — 길어지면 proc에 끝내기 훅 자리를 둔다(다른 영역)
		runExitHooks()
		say("바퀴를 끝낸다(신호 %v) — 다음 바퀴가 이어 간다", s)
		proc.StopCurrent(2 * time.Second)
		os.Exit(128 + int(s.(syscall.Signal)))
	}()
	if dry {
		P = "[dry-run] "
	}
	runRound()
}

// gcMain: run이 배경으로 띄운 정리(_gc <잠금 경로>). 잠금은 이 프로세스가 쥔다(끝나면 풀림)
func gcMain(args []string) {
	gcLock = DATA + "/gc.lock"
	if len(args) > 0 {
		gcLock = args[0]
	}
	f, busy, err := lockx.TryLock(gcLock)
	if err != nil || busy {
		fmt.Fprintf(os.Stderr, "lockf: %s: already locked\n", gcLock)
		exit(75)
	}
	heldLocks = append(heldLocks, f)
	dry = false
	say("--- %s", stampLog())
	gcAll()
	exit(0)
}
