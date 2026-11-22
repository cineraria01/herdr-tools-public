package main

// run(한 바퀴)·watch(바퀴를 늘) 명령 — 잠금과 신호 처리. 바퀴 본문은 run.go(runRound)

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
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
