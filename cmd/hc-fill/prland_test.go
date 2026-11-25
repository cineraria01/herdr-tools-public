//go:build !windows

package main

// 착지 뒤 PR의 실제 상태(prland.go): MERGED → merged, CLOSED → closed + 한 줄 댓글(고치기 전 코드로 실패 — merged로 적었다),
// OPEN → 그대로 열림, 다음 바퀴에 다시 읽고 그때 MERGED면 merged. 착지 뒤 prOpenMax 넘게 OPEN이면 까닭 댓글 뒤 닫는다
// (2026-11-26 ht-reland-pr-of — GitHub가 머리 가지 갱신을 PR에 반영하지 않아 원래 PR이 착지 뒤 4시간 OPEN, 지휘가 손으로 닫음)

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

func prLandState(t *testing.T, st string) (*prT, string) {
	return prLandStateAt(t, st, events.UTC(time.Now()))
}

// prLandStateAt: 착지 사건 시각을 정해서(OPEN이 얼마나 이어졌나)
func prLandStateAt(t *testing.T, st, landAt string) (*prT, string) {
	t.Helper()
	k := prSetup(t, "pr app")
	t.Setenv("HC_T_PRSTATE", st)
	k.ev("2026-11-24T01:00:00Z", "here", "w1", "완료", prSHA1, "끝")
	prTick()
	k.ev(landAt, "mg", "w1", "착지", prSHA2, "병합기 착지 — 제출 ID x · PR 머리 22222222 들어감")
	out := captured(t, prTick)
	return k, out
}

func TestPRLandedRemoteState(t *testing.T) {
	k, out := prLandState(t, "MERGED")
	if m, _ := prLoad(); m["w1"].State != "merged" || k.n("pr view 7") != 1 || !strings.Contains(out, "Merged") {
		t.Fatalf("MERGED:\n%s\n%s", k.logged(), out)
	}

	k, out = prLandState(t, "CLOSED")
	prTick()
	if m, _ := prLoad(); m["w1"].State != "closed" || k.n("pr view 7") != 1 || k.n("pr close") != 0 {
		t.Fatalf("CLOSED 상태:\n%s", k.logged())
	}
	if !strings.Contains(out, "PR #7 착지 w1 — 닫힘(GitHub가 Merged로 표시하지 않음)") || strings.Count(k.bodies(), "착지 22222222 — 변경은 기준 가지에 들어갔다(표시만 Closed)") != 1 {
		t.Fatalf("CLOSED 글:\n%s\n%s", out, k.bodies())
	}

	k, _ = prLandState(t, "OPEN")
	if m, _ := prLoad(); m["w1"].State != "open" {
		t.Fatalf("OPEN인데 %q", m["w1"].State)
	}
	t.Setenv("HC_T_PRSTATE", "MERGED")
	prTick()
	prTick()
	if m, _ := prLoad(); m["w1"].State != "merged" || k.n("pr view 7") != 2 || k.n("pr comment 7") != 1 {
		t.Fatalf("OPEN 뒤 다시:\n%s", k.logged())
	}

	// 착지 뒤 prOpenMax 넘게 OPEN — 까닭 댓글 뒤 닫는다(한 번만)
	k, out = prLandStateAt(t, "OPEN", events.UTC(time.Now().Add(-prOpenMax-time.Minute)))
	prTick()
	if m, _ := prLoad(); m["w1"].State != "closed" || k.n("pr close") != 1 {
		t.Fatalf("오래 OPEN인데 안 닫음:\n%s\n%s", k.logged(), out)
	}
	if !strings.Contains(k.bodies(), "착지 22222222 — 변경은 기준 가지에 들어갔는데 GitHub가 ") || !strings.Contains(out, "PR #7 닫음 w1") {
		t.Fatalf("오래 OPEN 글:\n%s\n%s", out, k.bodies())
	}

	k, _ = prLandState(t, "weird")
	if m, _ := prLoad(); m["w1"].State != "open" || k.n("pr view 7") != 1 {
		t.Fatalf("모름:\n%s", k.logged())
	}
}
