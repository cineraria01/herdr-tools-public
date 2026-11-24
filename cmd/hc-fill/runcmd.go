package main

// run(한 바퀴)·watch(바퀴를 늘) 명령 — 잠금과 신호 처리. 바퀴 본문은 run.go(runRound)

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/selfexec"
)

func init() {
	register("run", runCmd)
	register("watch", watchCmd)
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
		busyExit()
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
	self := selfexec.New("hc-fill watch", func(l string) { say("%s %s", stampLog(), l) })
	for {
		self.Check() // 바퀴 사이(run 자식이 끝난 뒤) — 바퀴 run은 늘 새로 띄우니 새 판이지만, 이 고리(쉼·상한) 자신도 새 판으로
		say("--- %s", stampLog())
		t0 := time.Now()
		c := exec.Command(SELF, "run")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		proc.WaitMax(c, "", roundMax)
		// 주기에 맞춰 쉰다(2026-11-24 fill-tick-cost — 옛 판은 바퀴를 다 돈 뒤 주기만큼 더 쉬어 간격이 주기 + 바퀴였다)
		time.Sleep(restFor(time.Duration(sec)*time.Second, time.Since(t0), restMin()))
	}
}

// restMin: 바퀴 사이 최소 쉼 — 설정 rest_min(초, 바퀴마다 설정 파일을 다시 읽어 바로 먹는다), 없거나 숫자가 아니면 5초
func restMin() time.Duration {
	s := 5
	if text, ok := readFile(CONF); ok {
		for _, l := range lines(text) {
			if f := fields(l); len(f) == 2 && f[0] == "rest_min" && isDigits(f[1]) {
				s = atoi10(f[1])
				break
			}
		}
	}
	return time.Duration(s) * time.Second
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
		busyExit()
	}
	heldLocks = append(heldLocks, f)
	runHooks()  // 신호 고루틴보다 먼저(R-20)
	tickStart() // 바퀴 계측(tick.go) — 끝내기 훅으로 찍으니 같은 자리
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
	// 쥔 잠금)과 겹치지 않게 이 바퀴를 미룬다(재감사 fill R-06). 다른 도구가 남긴 무리는 proc이 세지 않는다(기록의 도구 칸). 건너뜀이
	// orphanAlarm 바퀴 이어지면 주인 없는 막힘 사건 한 줄(재감사 3바퀴 fill/R-3M02 — 옛 판은 무리가 사라질 때까지 사건 없이 75)
	if left := proc.Left(); len(left) > 0 {
		why := strings.Join(left, ", ")
		say("앞 바퀴가 남긴 자식 무리 %d개가 아직 도는지 모른다(%s): %s — 이번 바퀴는 건너뜀", len(left), proc.RecordDir(), why)
		orphanSkip(why)
		exit(75)
	}
	orphanSkip("")
	runRound()
}

// orphanAlarm: 남은 무리로 바퀴를 건너뜀이 같은 까닭으로 이만큼 이어지면 사건
const orphanAlarm = 10

// orphanSkip: 남은 무리로 건너뛴 바퀴를 센다(<DATA>/orphan-skip: 「<횟수>\t<처음 초>\t<sent|->\t<까닭>」). 같은 까닭이 orphanAlarm 바퀴째면
// 주인 없는 막힘 사건(열쇠 — 같은 까닭은 한 번, 못 적으면 다음 바퀴에 다시). 빈 까닭(돈 바퀴)이면 센 것을 지운다. dry-run은 쓰지 않는다
func orphanSkip(why string) {
	if dry {
		return
	}
	f := DATA + "/orphan-skip"
	if why == "" {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			say("경고 — 건너뜀 횟수를 지우지 못했다: %v", err)
		}
		return
	}
	n, since, sent := 0, strconv.FormatInt(time.Now().Unix(), 10), false
	b, err := os.ReadFile(f)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		say("경고 — 건너뜀 횟수를 읽지 못했다(모름, 그대로 둔다): %v", err)
		return
	}
	if c := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 4); len(c) == 4 && c[3] == why && isDigits(c[0]) && isDigits(c[1]) {
		n, since, sent = atoi10(c[0]), c[1], c[2] == "sent"
	}
	n++
	if n >= orphanAlarm && !sent {
		h := fnv.New32a()
		h.Write([]byte(why))
		msg := fmt.Sprintf("hc-fill 바퀴가 남은 자식 무리로 %d바퀴째 건너뜀: %s (기록 폴더 %s — 그 무리가 끝나거나 사람이 확인해 기록을 지울 때까지)", n, why, proc.RecordDir())
		sent = hcLaneReport("막힘", msg, "hc-fill", "", fmt.Sprintf("hf:orphans:%s:%08x", since, h.Sum32())) == 0
	}
	st := "-"
	if sent {
		st = "sent"
	}
	if err := os.WriteFile(f, []byte(fmt.Sprintf("%d\t%s\t%s\t%s\n", n, since, st, why)), 0o644); err != nil {
		say("경고 — 건너뜀 횟수를 쓰지 못했다: %v", err)
	}
}
