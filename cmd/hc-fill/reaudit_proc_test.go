//go:build !windows

package main

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — hc-fill의 프로세스 지적(fill R-13) 고치기 전 실패 시험

import (
	"testing"
	"time"
)

// R-13: 로컬 onm(sh)도 원격과 같은 상한 안에서만 기다린다 — 넘기면 124(모름), 받은편지 해시 등이 멈춰도 run.lock을 쥔 채 서지 않는다
// (옛 판: proc.Limit이 sh를 몰라 상한 없이 기다렸다)
func TestGofix2OnmLocalLimit(t *testing.T) {
	old := LOCAL
	LOCAL = "here"
	t.Cleanup(func() { LOCAL = old })
	t.Setenv("HERDR_TIMEOUT_REMOTE", "1")
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	t0 := time.Now()
	_, rc := onm(nil, "here", "sleep 20", t.TempDir())
	if el := time.Since(t0); rc != 124 || el > 15*time.Second {
		t.Fatalf("로컬 sh도 상한(1초): rc=%d %v", rc, el)
	}
}
