package opsstate

// advise-merge-stall-age(2026-11-24): 「병합 줄이 안 움직임」의 시계는 줄에 선 가장 오래된 제출의 나이 — 병합기 일지 마지막 줄은
// 그 제출 뒤에 병합기가 한 일이 있나의 보조. 실제 일: 40분 비었던 줄에 4분 전 제출 1건이 「병합기 마지막 바퀴 40분 전」으로 울렸다.
// ①은 고치기 전 코드로 실패한다(항목이 나왔다). ④는 고치기 전 코드로 「느림」 항목이 없어 실패한다.

import (
	"strings"
	"testing"
)

var msth = Thresholds{WaitMin: 15, SlowMin: 30}

func mline(oldest, last *int) MergeLine {
	return MergeLine{Project: "p", Length: ip(1), Merger: "alive", OldestAgeS: oldest, OldestLane: "ln-a", LastRoundAgeS: last,
		Lands: ip(3), Skipped: ip(0), Delegated: ip(3)}
}

func mergeItems(m MergeLine) []Item {
	s := State{Schema: 1, At: t0, Queue: Queue{Counts: map[string]int{}}, Merge: []MergeLine{m}}
	var o []Item
	for _, x := range Advise(s, Facts{}, msth) {
		if strings.HasPrefix(x.Kind, "merge") {
			o = append(o, x)
		}
	}
	return o
}

func TestMergeStallAge(t *testing.T) {
	// ① 줄이 40분 비었다가 4분 전 제출 1건 — 일지 마지막 줄 40분 전은 「할 일이 없었다」
	if it := mergeItems(mline(ip(4*60), ip(40*60))); len(it) != 0 {
		t.Errorf("① 방금 찬 줄에 항목: %+v", it)
	}
	// ② 진짜 정체: 제출이 20분 서 있고 그 뒤 병합기 일지 줄 없음(마지막 바퀴 40분 전)
	it := mergeItems(mline(ip(20*60), ip(40*60)))
	if len(it) != 1 || it[0].Kind != "merge-stall" || it[0].Key != "merge-stall:p" || it[0].What != "병합 줄이 안 움직임: p" {
		t.Fatalf("② 안 움직임: %+v", it)
	}
	if !strings.Contains(it[0].Evidence, "줄 1 · 가장 오래 20분(ln-a)") {
		t.Errorf("② 근거에 가장 오래된 제출의 나이·레인: %q", it[0].Evidence)
	}
	// ②' 제출 뒤 병합기가 한 줄 남겼지만 그 뒤로 문턱 넘게 조용 — 안 움직임
	if it := mergeItems(mline(ip(60*60), ip(20*60))); len(it) != 1 || it[0].Kind != "merge-stall" {
		t.Errorf("②' 제출 뒤 줄도 문턱 넘게 전: %+v", it)
	}
	// ②'' 일지는 읽었는데 이 프로젝트 줄이 아직 없음(마지막 바퀴 모름) — 제출 나이로
	m := mline(ip(20*60), nil)
	if it := mergeItems(m); len(it) != 1 || it[0].Kind != "merge-stall" {
		t.Errorf("②'' 일지에 줄 없음·제출 20분: %+v", it)
	}
	// 일지를 못 읽음(다른 기계 병합기 — 마지막 바퀴·착지 수 모름) — 제출 나이만으로는 정하지 않는다(지금처럼)
	m.Lands, m.Skipped, m.Delegated = nil, nil, nil
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("일지 모름인데 항목: %+v", it)
	}
	// ③ 제출이 오래됐지만 병합기가 그 제출 뒤 착지를 시작함(문턱 안) — 항목 없음
	m = mline(ip(50*60), ip(40*60))
	m.Landing, m.LandingAgeS = "ln-a", ip(10*60)
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("③ 착지 시작 10분째에 항목: %+v", it)
	}
	// ③' 제출 뒤 일지 줄(추측 띄움 등)이 문턱 안 — 항목 없음
	if it := mergeItems(mline(ip(50*60), ip(5*60))); len(it) != 0 {
		t.Errorf("③' 제출 뒤 5분 전 일지 줄인데 항목: %+v", it)
	}
	// ④ 착지 검증이 느림 문턱(30분)을 넘김 — 「느림」, 열쇠가 안 움직임과 다르다
	m.LandingAgeS = ip(34 * 60)
	it = mergeItems(m)
	if len(it) != 1 || it[0].Kind != "merge-slow" || it[0].Key != "merge-slow:p" || it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 착지 검증 34분째" {
		t.Fatalf("④ 느림: %+v", it)
	}
	if !strings.Contains(it[0].Evidence, "가장 오래 50분(ln-a)") || len(it[0].Next) == 0 || it[0].Sure != SureJudge {
		t.Errorf("④ 근거·다음 명령: %+v", it[0])
	}
	// 느림 손잡이 0이면 끔
	if it := Advise(State{Merge: []MergeLine{m}}, Facts{}, Thresholds{WaitMin: 15}); len(it) != 0 {
		t.Errorf("advise_merge_slow 0인데 항목: %+v", it)
	}
	// ⑤ 병합기 죽음 — 병합기 항목이 잡고 이 항목은 가리지도 겹치지도 않는다
	d := mline(ip(50*60), ip(40*60))
	d.Merger = "gone"
	if it := mergeItems(d); len(it) != 1 || it[0].Kind != "merger" {
		t.Errorf("⑤ 병합기 죽음: %+v", it)
	}
}
