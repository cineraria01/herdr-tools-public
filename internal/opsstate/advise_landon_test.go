package opsstate

// ht-advise-land-remote(2026-11-26): 병합기가 착지를 워커(land-<차례 8> 터미널)에서 돌리는 동안 일지에는 「착지 실행」 줄 하나뿐이라
// 「병합 줄이 안 움직임」이 올라갔다(워커 착지 18분째). 일지 마지막 줄이 워커 착지 실행 결정이면 그 기계·레인·시각을 알고,
// 워커 착지가 살아서 갱신 중이면(hc-fill이 Landing을 채움) 안 움직임이 아니며, 멈췄으면 근거에 「워커 착지 멈춤 <기계> n분」.
// 고치기 전 코드에는 LandOn 칸이 없어 컴파일부터 실패한다.

import (
	"strings"
	"testing"
	"time"
)

func jl(at, lane, result, what string) string {
	return strings.Join([]string{at, "1-" + lane, lane, "-", "-", "-", "1", at, "-", result, "-", what + " — 구성원 " + lane}, "\t")
}

func TestMergeJournalLandOn(t *testing.T) {
	all := func(string) bool { return true }
	t1, t2 := "2026-11-26T03:40:00Z", "2026-11-26T03:41:00Z"
	// 마지막 줄이 워커 착지 실행 결정 — 그 기계·레인·시각
	f := MergeJournal([]string{jl(t1, "a", "착지", "a 넣음"), jl(t2, "b", "착지 실행", "box-2(검증 자리 남음 · 코어당 부하 0.43)")}, 20, all)
	if want, _ := time.Parse(time.RFC3339, t2); f.LandOn != "box-2" || f.LandOnLane != "b" || !f.LandOnAt.Equal(want) {
		t.Fatalf("워커 착지 중: %+v", f)
	}
	// 그 뒤 결과 줄(착지·반송 등) — 끝남
	if f := MergeJournal([]string{jl(t1, "b", "착지 실행", "box-2(검증 자리 남음)"), jl(t2, "b", "착지", "b 넣음")}, 20, all); f.LandOn != "" {
		t.Errorf("착지 뒤: %+v", f)
	}
	// 병합기 기계에서(지금처럼 child 기록이 잡는다) · 끊김 줄 — 워커 착지 중 아님
	for _, w := range []string{"box-1(병합기 기계 · 손잡이 local)", "box-2 착지 실행 끊김(닿지 않음) — 일지로 판정하고 다음 바퀴에 다른 워커로"} {
		if f := MergeJournal([]string{jl(t2, "b", "착지 실행", w)}, 20, all); f.LandOn != "" {
			t.Errorf("%q: %+v", w, f)
		}
	}
	// 다른 프로젝트 줄은 keep이 뺀다
	f = MergeJournal([]string{jl(t1, "b", "착지 실행", "w2(검증 자리 남음)"), jl(t2, "x", "착지", "x 넣음")}, 20, func(l string) bool { return l != "x" })
	if f.LandOn != "w2" {
		t.Errorf("다른 프로젝트 줄: %+v", f)
	}
}

func TestMergeStallLandOn(t *testing.T) {
	// 워커 착지 살아 있고 갱신 중 — hc-fill이 Landing을 채운다 → 문턱 안이면 항목 없음, 넘으면 느림(기계가 보임)
	m := mline(ip(18*60), ip(17*60))
	m.LandOn, m.LandOnTerm, m.LandOnFresh, m.LandOnAgeS = "box-2", "box-2/t5", true, ip(17*60)
	m.Landing, m.LandingAgeS = "ln-a", ip(17*60)
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("워커 착지 진행 중인데 항목: %+v", it)
	}
	m.LandingAgeS = ip(34 * 60)
	if it := mergeItems(m); len(it) != 1 || it[0].Kind != "merge-slow" || it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 착지 검증 34분째(box-2)" {
		t.Errorf("워커 착지 느림: %+v", it)
	}
	// 멈춤(갱신 없음) — 안 움직임, 근거에 워커 착지 멈춤 <기계> n분
	m.Landing, m.LandingAgeS, m.LandOnFresh = "", nil, false
	it := mergeItems(m)
	if len(it) != 1 || it[0].Kind != "merge-stall" || !strings.Contains(it[0].Evidence, "워커 착지 멈춤 box-2 17분") || !strings.Contains(it[0].Evidence, "box-2/t5") {
		t.Fatalf("워커 착지 멈춤: %+v", it)
	}
	// 터미널 없음 · 못 읽음 — 근거에 그 사실
	m.LandOnTerm = ""
	if it := mergeItems(m); len(it) != 1 || !strings.Contains(it[0].Evidence, "워커 착지 멈춤 box-2 17분") || !strings.Contains(it[0].Evidence, "land 터미널 없음") {
		t.Errorf("터미널 없음: %+v", it)
	}
	m.LandOnTerm, m.LandOnWhy = "box-2/t5", "워커 착지 갱신 못 읽음(box-2 종료 255)"
	if it := mergeItems(m); len(it) != 1 || !strings.Contains(it[0].Evidence, "box-2 종료 255") {
		t.Errorf("못 읽음: %+v", it)
	}
	// 병합기 기계 착지(local — LandOn 없음) — 지금 그대로
	if it := mergeItems(mline(ip(20*60), ip(40*60))); len(it) != 1 || strings.Contains(it[0].Evidence, "워커 착지") {
		t.Errorf("워커 착지 아님: %+v", it)
	}
	l := mline(ip(50*60), ip(40*60))
	l.Landing, l.LandingAgeS = "ln-a", ip(34*60)
	if it := mergeItems(l); len(it) != 1 || it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 착지 검증 34분째" {
		t.Errorf("병합기 기계 느림: %+v", it)
	}
}
