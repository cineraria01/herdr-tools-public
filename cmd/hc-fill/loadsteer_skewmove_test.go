//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// fill-polish-1124 가: 바퀴 끝 치우침 사건은 옮길 일이 있을 때만 — 대기 0이거나 대기 줄이 한가한 기계로 가면 사건 없음

func skewEvFix(t *testing.T) func() string {
	t.Helper()
	lpFix(t)
	ev := DATA + "/ev.log"
	if err := os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+ev+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	LOADS = map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 1, Cores: 8, OK: true}}
	return func() string { b, _ := os.ReadFile(ev); return string(b) }
}

// 대기 0 + 치우침 → 사건 없음(고치기 전 코드로 실패: 대기 레인 0인 채 보냈다)
func TestSkewEventNoWait(t *testing.T) {
	evs := skewEvFix(t)
	ROWS = nil
	t0 := time.Now()
	loadEnd(t0)
	loadEnd(t0.Add(11 * time.Minute))
	if e := evs(); e != "" {
		t.Fatalf("대기 줄 없이 치우침 사건: %q", e)
	}
	if loadRecs()["a"].hotSince == 0 {
		t.Fatal("바쁨은 load.tsv(상태 한 장)에 남아야 한다")
	}
}

// 대기 줄이 한가한 b로 갈 차례 → 사건 없음 · 바쁜 a로만 갈 줄 → 사건(근거: 줄 이름·a 도는 레인 수)
func TestSkewEventWaitRoute(t *testing.T) {
	evs := skewEvFix(t)
	ROWS = []Row{{"w1", "p", "/b", "-", "-", "wait"}}
	t0 := time.Now()
	loadEnd(t0)
	loadEnd(t0.Add(11 * time.Minute))
	if e := evs(); e != "" {
		t.Fatalf("한가한 b로 갈 줄인데 사건: %q", e)
	}
	ROWS = []Row{{"w1", "p", "/b", "-", "a", "wait"}}
	loadEnd(t0.Add(12 * time.Minute))
	if e := evs(); !strings.Contains(e, "바쁜 기계로만 갈 대기 줄 1(w1) · a 도는 레인 0") || !strings.Contains(e, "hc-fill-skew") {
		t.Fatalf("치우침 사건: %q", e)
	}
}
