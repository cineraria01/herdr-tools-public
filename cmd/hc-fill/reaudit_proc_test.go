//go:build !windows

package main

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — hc-fill의 프로세스 지적(fill R-13) 고치기 전 실패 시험

import (
	"path/filepath"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// R-13: 로컬 onm(sh)도 원격과 같은 상한 안에서만 기다린다 — 넘기면 124(모름), 받은편지 해시 등이 멈춰도 run.lock을 쥔 채 서지 않는다
// (옛 판: proc.Limit이 sh를 몰라 상한 없이 기다렸다).
// 재감사 fill R-224: 시작 실패도 124라 sh가 안 떠도 통과했다 — sh가 시작 표식을 쓴 뒤 기다리게 하고, 상한만큼 기다렸는지(즉시 124가
// 아님)와 그 무리의 손자가 거둬졌는지(늦은 표식 없음)를 함께 본다. 경로는 따로 된 인자($1·$2)로
func TestGofix2OnmLocalLimit(t *testing.T) {
	old := LOCAL
	LOCAL = "here"
	t.Cleanup(func() { LOCAL = old })
	t.Setenv("HERDR_TIMEOUT_REMOTE", "1")
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	d := clitest.TempDir(t)
	start, late := filepath.Join(d, "start"), filepath.Join(d, "late")
	t0 := time.Now()
	_, rc := onm(nil, "here", `: >"$1"; (sleep 3; : >"$2") & sleep 20`, d, start, late) // 첫 인자는 폴더(X 머리말이 먹는다)
	el := time.Since(t0)
	clitest.WaitFile(t, start, time.Second) // sh가 정말 떴다(시작 실패의 124가 아님)
	if rc != 124 || el < 900*time.Millisecond || el > 15*time.Second {
		t.Fatalf("로컬 sh도 상한(1초)만큼 기다린 뒤 124: rc=%d %v", rc, el)
	}
	time.Sleep(time.Until(t0.Add(4500 * time.Millisecond)))
	if !clitest.Absent(t, late) {
		t.Error("상한에 걸린 sh의 손자가 살아남아 늦은 표식을 썼다 — 무리째 끄지 않음")
	}
}
