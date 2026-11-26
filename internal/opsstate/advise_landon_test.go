package opsstate

// ht-advise-land-remote(2026-11-26): 병합기가 착지를 워커에서 돌리는 동안 「병합 줄이 안 움직임」이 올라갔다(워커 착지 18분째).
// 워커 착지가 살아서 진행 중이면 안 움직임이 아니라 느림과 견주고, 멈췄으면 근거에 「워커 착지 멈춤 <기계> n분」.
// ht-merger-state-record(2026-11-26): 워커 착지 단계·워커·진행은 병합기 기록이 알린다(일지 마지막 「착지 실행」 줄로 짐작하던
// MergeJournal LandOn은 걷었다 — 기록이 단계 land-on을 쓰는 것은 hc-merger staterec_test.go가 본다).

import (
	"strings"
	"testing"
)

// mlandon: 워커 착지 단계 — 워커 box-2의 터미널 box-2/t5, 착지 나이 land, 마지막 진행 나이 last
func mlandon(land, last int) MergeLine {
	m := mline(ip(18*60), ip(last))
	m.Phase, m.PhaseWhy, m.LandOn = "land-on", "워커 착지 실행 — ln-a(box-2)", "box-2"
	m.Waits = []MergeWait{{Machine: "box-2", Addr: "box-2/t5", Role: "착지 실행", Live: "live"}}
	m.Landing, m.LandingAgeS = "ln-a", ip(land)
	return m
}

func TestMergeStallLandOn(t *testing.T) {
	// 워커 착지 살아 있고 진행 중 — 문턱 안이면 항목 없음, 넘으면 느림(기계가 보임)
	if it := mergeItems(mlandon(17*60, 30)); len(it) != 0 {
		t.Errorf("워커 착지 진행 중인데 항목: %+v", it)
	}
	if it := mergeItems(mlandon(34*60, 30)); len(it) != 1 || it[0].Kind != "merge-slow" || it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 착지 검증 34분째(box-2)" {
		t.Errorf("워커 착지 느림: %+v", it)
	}
	// 멈춤(진행 17분 전) — 안 움직임, 근거에 워커 착지 멈춤 <기계> n분과 그 터미널
	m := mlandon(17*60, 17*60)
	it := mergeItems(m)
	if len(it) != 1 || it[0].Kind != "merge-stall" || !strings.Contains(it[0].Evidence, "워커 착지 멈춤 box-2 17분") || !strings.Contains(it[0].Evidence, "box-2/t5") {
		t.Fatalf("워커 착지 멈춤: %+v", it)
	}
	// 터미널 없음 · 못 읽음 — 근거에 그 사실
	m.Waits[0].Live = "gone"
	if it := mergeItems(m); len(it) != 1 || !strings.Contains(it[0].Evidence, "워커 착지 멈춤 box-2 17분") || !strings.Contains(it[0].Evidence, "box-2/t5 터미널 없음") {
		t.Errorf("터미널 없음: %+v", it)
	}
	m.Waits[0].Live = "?"
	if it := mergeItems(m); len(it) != 1 || !strings.Contains(it[0].Evidence, "box-2/t5 살아 있는지 모름") {
		t.Errorf("못 읽음: %+v", it)
	}
	// 병합기 기계 착지(LandOn 없음) — 지금 그대로
	if it := mergeItems(mline(ip(20*60), ip(40*60))); len(it) != 1 || strings.Contains(it[0].Evidence, "워커 착지") {
		t.Errorf("워커 착지 아님: %+v", it)
	}
	l := mline(ip(50*60), ip(30))
	l.Landing, l.LandingAgeS = "ln-a", ip(34*60)
	if it := mergeItems(l); len(it) != 1 || it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 착지 검증 34분째" {
		t.Errorf("병합기 기계 느림: %+v", it)
	}
}
