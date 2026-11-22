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
	for _, k := range strings.Fields("local agent coord task launch gc early early_wait idle hold audit audit_max audit_final wake_every ack_max overlap done_stall inbox_audit dup_warn lows lows_hour lows_min") {
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
	case "_onland":
		onlandMain(args)
	case "add":
		addCmd(args)
	case "rm":
		rmCmd(args)
	case "edit":
		editCmd(args)
	case "prio":
		prioCmd(args)
	case "ls":
		lsCmd(args)
	case "status":
		statusCmd(args)
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
	st, prj := "", ""
	rc := withQ(func() {
		rows := qRowsW() // 못 읽으면 「없다」가 아니라 종료 1(R-29)
		for _, x := range rows {
			if x.F(1) == args[0] {
				st, prj = x.F(6), x.F(2)
				break
			}
		}
		if st == "" {
			die("대기열에 없다: %s", args[0])
		}
		var out []Row
		for _, x := range rows {
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
	// 떠 있는 레인: 이미 병합기 큐에 냈으면 그 큐 메타도(2026-11-22 fill-prio-pass — 지휘가 메타 제목을 손으로 고쳤다)
	if st != "wait" && prioMeta(args[0], prj, plabel(p)) == 1 {
		say("알림: %s 는 아직 병합기 큐에 안 냈다 — 떠 있는 레인 환경의 HC_LANE_PRIORITY는 띄울 때 값이라, 제출한 뒤 다시 prio 하면 큐 메타를 고친다", args[0])
	}
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
	proc.TakeSignals() // 이 처리기 하나만(재감사 R-H04 — proc 처리기가 짧은 유예로 먼저 끝내지 않게)
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
	runHooks() // 신호 고루틴보다 먼저(R-20)
	// 바퀴 상한·watch 멈춤으로 TERM을 받으면: 상한 걸고 따로 무리로 띄운 자식(git·hc 등 — 이 무리 신호가 안 닿는다)까지 거두고 끝낸다.
	// 이 처리기 하나만(proc.TakeSignals — 재감사 R-H04: proc 처리기가 함께 받아 먼저 끝내면 아래 훅·거두기가 반쯤 끊겼다)
	proc.TakeSignals()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sig
		// 끝내기 훅(held.tsv 비움·지시문 사본 지움 — 셸 판 EXIT trap과 같게)
		runExitHooks()
		say("바퀴를 끝낸다(신호 %v) — 다음 바퀴가 이어 간다", s)
		proc.StopCurrent(2 * time.Second)
		os.Exit(128 + int(s.(syscall.Signal)))
	}()
	if dry {
		P = "[dry-run] "
	}
	// 앞 바퀴(SIGKILL로 죽음)의 자식 무리를 거두고도 소유를 모르는 채 우두머리가 도는 것이 있으면 그 자식(hc ls·herdr-balance 등이
	// 쥔 잠금)과 겹치지 않게 이 바퀴를 미룬다(재감사 fill R-06)
	if n := proc.Orphans(); n > 0 {
		say("앞 바퀴가 남긴 자식 무리 %d개가 아직 도는지 모른다(%s) — 이번 바퀴는 건너뜀", n, proc.RecordDir())
		exit(75)
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
