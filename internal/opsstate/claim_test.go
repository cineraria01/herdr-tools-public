package opsstate

// ht-advise-claim(2026-11-26): 같은 손댈 것에 지휘 둘이 따로 손댔다(굶는 감사 → t3yx seats 3, 곧 t30x seats 2가 덮음).
// 항목 대상(레인·기계·프로젝트)에 최근 다른 지휘 조치가 일지에 있으면 Claim이 붙는다 — 막지 않는다

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/journal"
)

func TestClaims(t *testing.T) {
	now := time.Now()
	it := []Item{
		{Key: "audit-starve:lane-s", Kind: "audit-starve"}, // 레인 lane-s는 기계 box-1 — 손잡이 seats@box-1이 겨냥
		{Key: "free-wait:lane-b", Kind: "free-wait"},       // 오래된 조치만
		{Key: "merger:proj", Kind: "merger"},               // 실패한 조치·바퀴 스스로 한 조치는 조치함이 아니다
		{Key: "chain:lane-c", Kind: "chain"},               // 내가 한 조치는 「다른 지휘」가 아니다
		{Key: "unknown:source:journal", Kind: "unknown"},
	}
	s := State{Lanes: []Lane{{Name: "lane-s", Machine: "box-1"}}}
	es := []journal.Entry{
		{Time: now.Add(-50 * time.Minute), By: "w1:t1", Action: "edit", Target: "lane-b", Why: "선행 풀기", Result: "됨"},
		{Time: now.Add(-20 * time.Minute), By: "w1:t1", Action: "knob set", Target: "seats@box-1", Why: "감사 자리 없음 lane-s(hc-fill advise) 덧붙임 글이 길다", Result: "됨"},
		{Time: now.Add(-5 * time.Minute), By: "w1:t2", Action: "knob set", Target: "seats@box-1", Why: "되돌림", Result: "됨"},
		{Time: now.Add(-3 * time.Minute), By: "w1:t2", Action: "merger restart", Target: "proj", Result: "실패: 종료 1"},
		{Time: now.Add(-3 * time.Minute), By: "hc-fill 스스로", Action: "merger restart", Target: "proj", Result: "띄움"},
		{Time: now.Add(-2 * time.Minute), By: "w1:me", Action: "edit", Target: "lane-c", Why: "사슬", Result: "됨"},
	}
	Claims(it, s, es, "w1:me", now, 30*time.Minute)
	if c := it[0].Claim; !strings.HasPrefix(c, "w1:t2 5분 전 — knob set seats@box-1(되돌림) · 조치 2건") {
		t.Errorf("굶는 감사(기계 손잡이): %q", c)
	}
	for _, x := range it[1:] {
		if x.Claim != "" {
			t.Errorf("%s에 조치함: %q", x.Key, x.Claim)
		}
	}
	// 손잡이 0(끔)이면 붙이지 않는다 · 까닭은 앞부분만
	it[0].Claim = ""
	Claims(it[:1], s, es, "w1:me", now, 0)
	if it[0].Claim != "" {
		t.Errorf("끔인데 조치함: %q", it[0].Claim)
	}
	Claims(it[:1], s, es[:2], "w1:me", now, 30*time.Minute)
	if c := it[0].Claim; !strings.HasPrefix(c, "w1:t1 20분 전 — knob set seats@box-1(감사 자리 없음 lane-s(hc-fill advise) 덧붙임 글이 길…)") {
		t.Errorf("까닭 자름: %q", c)
	}
}

func TestItemLane(t *testing.T) {
	for _, c := range []struct{ kind, key, want string }{
		{"audit-starve", "audit-starve:a", "a"}, {"lane-done", "lane-done:b", "b"}, {"merger", "merger:p", ""}, {"deploy", "deploy:m", ""},
	} {
		if got := (Item{Key: c.key, Kind: c.kind}).Lane(); got != c.want {
			t.Errorf("%s: %q", c.key, got)
		}
	}
}
