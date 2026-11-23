package main

import (
	"testing"
	"time"
)

// sweep-landed-keep: hc keep이 켜진 레인 터미널은 에이전트가 끝나면 셸로 바뀌어 status 없이 live로 남는다(2026-11-21 mini-m4
// land-claim-net 13시간). 그 레인이 착지(제출·착지 사건)로 끝났고 작업 트리가 없으면 닫는다. 사람이 켠 keep(~/.claude-tasks/keep),
// 레인이 아닌 셸 칸, 착지 증거 없는 레인(띄운 직후 상태 전)은 남긴다
func TestJudgeLandedKeepShell(t *testing.T) {
	now := float64(time.Now().UnixMilli())
	shell := func(extra ...any) map[string]any {
		m := map[string]any{"address": "t7", "name": "land-claim-net", "state": "live", "keep": true, "status": nil,
			"labels": map[string]any{"hc-lane": "1", "where": "app / 1"}, "created_ms": now - 14*60*minute,
			"foreground": map[string]any{"name": "zsh", "self": true}, "cwd": "/r.wt/land-claim-net", "shell": true}
		for i := 0; i+1 < len(extra); i += 2 {
			m[extra[i].(string)] = extra[i+1]
		}
		return m
	}
	tail := "% "
	ended := facts{worktree: no, unlanded: no, tail: &tail, submitted: true}
	none := map[string]bool{}

	if r, why := judge(shell(), ended, now, none); r == "" {
		t.Fatalf("착지로 끝난 keep 셸 레인 터미널을 남김: %s", why)
	}
	if r, why := judge(shell(), ended, now, map[string]bool{"land-claim-net": true}); r != "" || why != "보호: keep 목록" {
		t.Fatalf("사람 keep 목록 이름을 닫음: %q %q", r, why)
	}
	if r, _ := judge(shell(), ended, now, map[string]bool{"t7": true}); r != "" {
		t.Fatal("사람 keep 목록 주소를 닫음")
	}
	if r, _ := judge(shell(), facts{worktree: no, unlanded: no, tail: &tail}, now, none); r != "" {
		t.Fatal("착지 증거 없는 status 없는 레인(띄운 직후일 수 있음)을 닫음")
	}
	if r, _ := judge(shell(), facts{worktree: yes, unlanded: no, tail: &tail, submitted: true}, now, none); r != "" {
		t.Fatal("작업 트리가 남은 레인을 이 규칙으로 닫음(clean 규칙을 따라야 한다)")
	}
	if r, _ := judge(shell(), facts{worktree: missing, tail: &tail, submitted: true}, now, none); r != "" {
		t.Fatal("작업 트리를 모르는데 닫음")
	}
	if r, _ := judge(shell("labels", map[string]any{"where": "app / 1"}), ended, now, none); r != "" {
		t.Fatal("레인이 아닌 사람 셸 칸을 닫음")
	}
	// R-4A09: 전경이 셸 이름이어도 터미널 자신이 아니면(그 셸에서 스크립트가 돎) 남김 — 셸 칸 보호도 그대로
	if r, why := judge(shell("foreground", map[string]any{"name": "zsh", "self": false}), ended, now, none); r != "" || why != "에이전트 상태 없음" {
		t.Fatalf("keep 셸에서 스크립트가 도는데 닫음: %q %q", r, why)
	}
	if r, _ := judge(shell("keep", false), ended, now, none); r != "" {
		t.Fatal("keep이 아닌 status 없는 터미널을 닫음(셸로 바뀐 레인이 아니다)")
	}
	if r, _ := judge(shell("name", "d1-loop-x"), ended, now, none); r != "" {
		t.Fatal("보호 이름(loop)을 닫음")
	}
	if r, _ := judge(shell("state", "finished"), ended, now, none); r != "" {
		t.Fatal("live 아닌 터미널을 닫음")
	}
	// 에이전트가 살아 있는(status 있음) 레인은 지금 규칙 그대로: working이면 남김
	if r, _ := judge(shell("status", map[string]any{"app": "claude-code", "state": "working"}), ended, now, none); r != "" {
		t.Fatal("working 레인을 닫음")
	}
}

// [R-3M02] sameRun은 판정 때의 상태 꼴을 본다: 상태 없던 keep 셸은 그대로(없음·keep·같은 셸 전경)일 때만 같은 실행
func TestSameRunKeepShellShape(t *testing.T) {
	now := float64(time.Now().UnixMilli())
	term := func(extra ...any) map[string]any {
		m := map[string]any{"address": "t7", "name": "k", "state": "live", "keep": true, "status": nil,
			"created_ms": now, "status_ms": now, "foreground": map[string]any{"name": "zsh", "self": true}}
		for i := 0; i+1 < len(extra); i += 2 {
			m[extra[i].(string)] = extra[i+1]
		}
		return m
	}
	idle := map[string]any{"app": "claude-code", "state": "idle"}
	cases := []struct {
		g    map[string]any
		want bool
		why  string
	}{
		{term(), true, "그대로인 keep 셸"},
		{term("status", idle), false, "그사이 에이전트가 붙음"},
		{term("foreground", map[string]any{"name": "vim"}), false, "전경이 셸 아님"},
		{term("foreground", nil), false, "전경 모름"},
		{term("foreground", map[string]any{"name": "zsh", "self": false}), false, "전경 셸이 터미널 자신 아님(R-4A09)"},
		{term("keep", false), false, "keep 꺼짐"},
	}
	for _, c := range cases {
		if sameRun(term(), c.g) != c.want {
			t.Errorf("%s: sameRun=%v 기대 %v", c.why, !c.want, c.want)
		}
	}
	if sameRun(term("status", idle), term()) {
		t.Error("상태 있던 판정 뒤 상태가 사라졌는데 같은 실행으로 봄")
	}
	if !sameRun(term("status", idle), term("status", idle)) {
		t.Error("idle 그대로를 다른 실행으로 봄")
	}
}
