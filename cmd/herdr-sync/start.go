package main

// 시작: 기계 목록(hc), 잠금, 앞 실행이 남긴 자식 무리 확인

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

// onSignals: INT·TERM·HUP을 받으면 상한으로 따로 띄운 자식 무리(git push·commit, gh 등)를 거두고 임시 폴더를 지운 뒤 128+신호로
// 끝낸다 — 단말의 Ctrl-C는 앞쪽 무리에만 가서 따로 된 무리는 고아로 남는다(gofix-ops O-04). 이 처리기 하나만(proc.TakeSignals —
// 재감사 R-H04). 게시기(herdr-tools-publish)도 따로 무리로 기다리니 함께 TERM을 받는다 — 게시기는 제 push·pack 무리를 거두고 임시
// 폴더를 지운 뒤 끝나니(제 유예 proc.Grace) 그보다 넉넉히 기다린 뒤 KILL하고, 끝난 것을 본 뒤에 잠금을 놓는다(재감사 ops R-09)
func onSignals() {
	proc.TakeSignals()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s := <-ch
		cleanup()
		proc.StopCurrent(4*proc.Grace + 2*time.Second)
		code := 130
		switch s {
		case syscall.SIGTERM:
			code = 143
		case syscall.SIGHUP:
			code = 129
		}
		deployEndSig(code) // 신호로 멈춘 배포도 기록에 — 자식을 띄우지 않고 아는 판만(deploylog.go)
		os.Exit(code)
	}()
}

// machinesFromHC: hc machine ls의 기계 이름. hc가 없으면 빈 목록(맥이 아님). hc 실패·시간 넘김, JSON이 정확히 한 값이 아님,
// machines 칸 없음, 이름(빈 아닌 글자)이 없는 행은 오류(모름) — 목록 전체가 온전할 때만 쓴다(R-03)
func machinesFromHC() ([]string, error) {
	out, err := proc.Output(exec.Command("hc", "machine", "ls"))
	if errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hc machine ls: %w", err)
	}
	var v struct {
		Machines *[]map[string]any `json:"machines"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("hc machine ls 출력이 JSON 한 값이 아니다: %w", err)
	}
	if v.Machines == nil {
		return nil, errors.New("hc machine ls 출력에 machines가 없다")
	}
	var r []string
	for i, m := range *v.Machines {
		n, ok := m["name"].(string)
		if !ok || n == "" {
			return nil, fmt.Errorf("hc machine ls %d번째 행에 이름이 없다", i+1)
		}
		r = append(r, n)
	}
	return r, nil
}

// takeLock: STATE/lock을 비켜 잠근다(쥔 쪽이 있으면 75). 잠근 fd의 파일이 지금 그 경로의 파일과 같아야 한다 — 예전 셸 판 lockf(-k 없음)는
// 끝날 때 잠금 파일을 지워, 그 전에 연 fd에 잠근 실행과 새로 만든 파일에 잠근 실행이 함께 돌 수 있었다(gofix-ops A07). 다르면 다시 연다
func takeLock(p string) *os.File {
	for i := 0; i < 5; i++ {
		f, busy, err := lockx.TryLock(p)
		if busy {
			fmt.Fprintf(os.Stderr, "herdr-sync: 다른 herdr-sync가 돌고 있다 (%s)\n", p)
			os.Exit(75)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "herdr-sync: 잠금 실패 %s: %v\n", p, err)
			os.Exit(1)
		}
		if sameFile(f, p) {
			return f
		}
		f.Close()
	}
	fmt.Fprintf(os.Stderr, "herdr-sync: 잠금 파일이 자꾸 바뀐다 (%s) — 다른 herdr-sync가 돌고 있다\n", p)
	os.Exit(75)
	return nil
}

// sameFile: 열린 f가 지금 경로 p의 그 파일인가(지워졌거나 바뀌었으면 거짓)
func sameFile(f *os.File, p string) bool {
	a, err1 := f.Stat()
	b, err2 := os.Stat(p)
	return err1 == nil && err2 == nil && os.SameFile(a, b)
}

// orphansLeft: 앞 실행(부모가 죽음)의 자식 무리를 먼저 거둔 뒤에도 소유를 모르는 채 우두머리가 도는 것의 수(proc.Orphans — 옛 판은
// 거두기 전에 세어 거둘 수 있는 기록에도 멈췄다)
func orphansLeft() int { return proc.Orphans() }
