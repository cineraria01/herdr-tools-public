package main

import (
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// knobs-migrate: 닫는 문턱 R1·R2·R3가 손잡이(sweep_r1·r2·r3)를 따른다. 저장 파일 없음 → 지금 기본, 범위 밖 → 기본, 바꾸면 다음 쓸기에
func TestSweepKnobs(t *testing.T) {
	oh, ol, ob := home, lim, binDir
	t.Cleanup(func() { home, lim, binDir = oh, ol, ob })
	home = t.TempDir()
	binDir = t.TempDir() // 설정 도구(herdr-tools-config) 없이 — 이 기계 설정 sweep.knobs_repo를 읽지 않게
	now := time.Now()
	if l := sweepKnobs(now); l != (limits{30, 30, 360}) {
		t.Fatalf("손잡이 없음 = %+v", l)
	}
	if l := limitsFrom(map[string]string{"sweep_r1": "0", "sweep_r2": "x", "sweep_r3": "45"}); l != (limits{30, 30, 45}) {
		t.Fatalf("범위 밖은 기본 = %+v", l)
	}
	for n, v := range map[string]string{"sweep_r1": "5", "sweep_r2": "0", "sweep_r3": "90"} {
		if err := knob.Set(knob.Path(home), knob.Entry{Name: n, Value: v, By: "t", Why: "t", At: now.Unix()}, false, now); err != nil {
			t.Fatal(err)
		}
	}
	lim = sweepKnobs(now)
	if lim != (limits{5, 0, 90}) {
		t.Fatalf("저장 파일 = %+v", lim)
	}
	// 판정이 그 문턱을 쓴다
	nowMs, m := 1_000_000_000_000.0, minute
	tail := ""
	ok := facts{worktree: no, unlanded: no, tail: &tail}
	t0 := func(kv ...any) map[string]any {
		x := map[string]any{"address": "t1", "name": "claude-x", "state": "live", "status": map[string]any{"app": "claude-code", "state": "idle"},
			"created_ms": nowMs - 6*m, "status_ms": nowMs - 6*m + 500, "turn": 0.0, "parent": nil, "labels": map[string]any{}}
		for i := 0; i+1 < len(kv); i += 2 {
			x[kv[i].(string)] = kv[i+1]
		}
		return x
	}
	rule := func(x map[string]any) (string, string) { return judge(x, ok, nowMs, map[string]bool{}) }
	if r, _ := rule(t0()); r != "R1" {
		t.Fatalf("R1 5분: %q", r)
	}
	if r, _ := rule(t0("parent", "t0", "turn", 2.0, "status_ms", nowMs-1*m)); r != "R2" {
		t.Fatalf("R2 0분: %q", r)
	}
	if r, _ := rule(t0("turn", 5.0, "created_ms", nowMs-3*60*m, "status_ms", nowMs-91*m)); r != "R3" {
		t.Fatalf("R3 90분: %q", r)
	}
	if _, why := rule(t0("turn", 5.0, "created_ms", nowMs-3*60*m, "status_ms", nowMs-80*m)); why != "worker 아님, 쉰 지 80분(R3 90분 전)" {
		t.Fatalf("R3 전 까닭: %q", why)
	}
}
