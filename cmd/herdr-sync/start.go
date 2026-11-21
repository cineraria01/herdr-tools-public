package main

// 시작: 기계 목록(hc), 잠금, 앞 실행이 남긴 자식 무리 확인

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

// onSignals: INT·TERM·HUP을 받으면 상한으로 따로 띄운 자식 무리(git push·commit, gh 등)를 거두고 임시 폴더를 지운 뒤 128+신호로
// 끝낸다 — 단말의 Ctrl-C는 앞쪽 무리에만 가서 따로 된 무리는 고아로 남는다(gofix-ops O-04). proc도 같은 신호로 무리를 거둔 뒤
// 그 신호로 끝낼 수 있어(어느 쪽이 먼저인지 모름) 임시 폴더부터 지운다 — 부모가 보는 끝은 130·143·129 또는 그 신호
func onSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s := <-ch
		cleanup()
		proc.StopCurrent(proc.Grace)
		code := 130
		switch s {
		case syscall.SIGTERM:
			code = 143
		case syscall.SIGHUP:
			code = 129
		}
		os.Exit(code)
	}()
}

// machinesFromHC: hc machine ls의 기계 이름. hc가 없거나 실패하면(워커 맥 등 — 셸 판과 같게) 빈 목록, 시간 넘김이면 오류(모름)
func machinesFromHC() ([]string, error) {
	out, err := proc.Output(exec.Command("hc", "machine", "ls"))
	if errors.Is(err, proc.ErrTimeout) {
		return nil, err
	}
	if err != nil {
		return nil, nil
	}
	var v struct {
		Machines []struct {
			Name string `json:"name"`
		} `json:"machines"`
	}
	if json.Unmarshal(out, &v) != nil {
		return nil, nil // 기계 목록을 내지 않는 hc(맥이 아닌 곳)
	}
	var r []string
	for _, m := range v.Machines {
		if m.Name != "" {
			r = append(r, m.Name)
		}
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

// orphansLeft: proc 고아 기록 가운데 부모가 죽었는데 남은 것(proc이 거두지 못함 — 모름)의 수
func orphansLeft() int {
	d := proc.RecordDir()
	if d == "" {
		return 0
	}
	es, err := os.ReadDir(d)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range es {
		b, err := os.ReadFile(filepath.Join(d, e.Name()))
		if err != nil {
			continue
		}
		f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
		if len(f) != 4 {
			continue
		}
		var pp int
		if _, err := fmt.Sscanf(f[2], "%d", &pp); err != nil || pp == os.Getpid() {
			continue
		}
		if !proc.Alive(pp) {
			n++
		}
	}
	return n
}
