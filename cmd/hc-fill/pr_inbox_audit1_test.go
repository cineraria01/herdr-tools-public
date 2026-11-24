//go:build !windows

package main

// pr-inbox-lanes 감사 1차 지적: 열린 받은편지 PR에 SHA가 바뀐 제출(rebase)이 와도 머리 가지를 옮기지 않는다 — 병합기의 머리 lease가
// 깨져 Merged 대신 닫히지 않게(중간, 고치기 전 코드로 실패: PATCH pr/w2 sha=2222… 한 번). 자기 감사 글에 높음·중간이 있으면 승인 없이 글만(낮음)

import (
	"strings"
	"testing"
)

func TestPRInboxSubmitKeepsHead(t *testing.T) {
	k := prSetup(t, "pr app")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋")
	prTick()
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA2)
	prTick()
	prTick()
	if k.n("git/refs/heads/pr/w2 -f sha="+prSHA2) != 0 || k.n("git/refs/heads/pr/w2 -f sha="+prSHA1) != 1 || k.n("pr create") != 1 {
		t.Fatalf("제출이 머리를 옮겼다:\n%s", k.logged())
	}
	if !strings.Contains(k.bodies(), "제출됨 — 병합기 줄에 넣음") || strings.Contains(k.bodies(), "다시 완료") {
		t.Fatalf("글:\n%s", k.bodies())
	}
}

func TestPRInboxSelfAuditNotClean(t *testing.T) {
	k := prSetup(t, "pr app", "pr_token_cmd echo tok-x")
	k.pinRow("w2", "-")
	k.ev("2026-11-24T01:05:00Z", "here", "w2", "중간", prSHA1, "구현·커밋")
	prTick()
	k.ev("2026-11-24T01:20:00Z", "here", "w2", "중간", prSHA1, "감사 완료 높음 0·중간 1·낮음 2")
	k.pinSubmit("2026-11-24T01:21:00Z", "w2", prSHA1)
	prTick()
	if k.n("pr review") != 0 || k.n("pr comment 7") != 2 || !strings.Contains(k.bodies(), "높음·중간 0을 읽지 못해 승인하지 않는다") {
		t.Fatalf("지적 있는 감사 글:\n%s\n%s", k.logged(), k.bodies())
	}
	for _, c := range []struct {
		s  string
		ok bool
	}{{"감사 완료 높음 0·중간 0·낮음 3", true}, {"감사 완료 높음 1·중간 0", false}, {"감사 통과", false}} {
		if prSelfClean(c.s) != c.ok {
			t.Errorf("prSelfClean(%q) != %v", c.s, c.ok)
		}
	}
}
