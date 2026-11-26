package opsstate

// advise-merge-stall-age(2026-11-24): 「병합 줄이 안 움직임」의 시계는 줄에 선 가장 오래된 제출의 나이 — 병합기의 마지막 진행은
// 그 제출 뒤에 병합기가 나아간 것이 있나의 보조. 실제 일: 40분 비었던 줄에 4분 전 제출 1건이 「병합기 마지막 바퀴 40분 전」으로 울렸다.
// ht-merger-state-record(2026-11-26): 재료가 병합기 기록 한 장(Phase·LastRoundAgeS = 마지막 진행·Waits·Inner)으로 바뀌었다 —
// 장면과 기대는 그대로, 옛 재료(일지 줄 시각·spec 터미널 이름·갱신)를 기록 칸으로 옮겼다.

import (
	"strings"
	"testing"
)

var msth = Thresholds{WaitMin: 15, SlowMin: 30}

// mline: 병합기 기록이 있는 병합 줄(단계 round — 맨 앞 기다림이 아님). last = 마지막 진행 나이
func mline(oldest, last *int) MergeLine {
	return MergeLine{Project: "p", Length: ip(1), Merger: "alive", OldestAgeS: oldest, OldestLane: "ln-a", LastRoundAgeS: last,
		Lands: ip(3), Skipped: ip(0), Delegated: ip(3), Phase: "round", PhaseAgeS: ip(60)}
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
	// ① 줄이 40분 비었다가 4분 전 제출 1건 — 마지막 진행 40분 전은 「할 일이 없었다」
	if it := mergeItems(mline(ip(4*60), ip(40*60))); len(it) != 0 {
		t.Errorf("① 방금 찬 줄에 항목: %+v", it)
	}
	// ② 진짜 정체: 제출이 20분 서 있고 그 뒤 병합기 진행 없음(마지막 진행 40분 전 — 글은 뜻대로 「마지막 진행」, 「마지막 바퀴」가 아니다:
	// 병합기는 몇 초마다 바퀴를 돈다 — 2026-11-26 지휘 t3yx ⑦)
	it := mergeItems(mline(ip(20*60), ip(40*60)))
	if len(it) != 1 || it[0].Kind != "merge-stall" || it[0].Key != "merge-stall:p" || it[0].What != "병합 줄이 안 움직임: p" {
		t.Fatalf("② 안 움직임: %+v", it)
	}
	if !strings.Contains(it[0].Evidence, "줄 1 · 가장 오래 20분(ln-a)") || !strings.Contains(it[0].Evidence, "마지막 진행 40분 전") ||
		strings.Contains(it[0].Evidence, "마지막 바퀴") {
		t.Errorf("② 근거에 가장 오래된 제출의 나이·레인·마지막 진행: %q", it[0].Evidence)
	}
	// ②' 제출 뒤 병합기가 나아갔지만 그 뒤로 문턱 넘게 조용 — 안 움직임
	if it := mergeItems(mline(ip(60*60), ip(20*60))); len(it) != 1 || it[0].Kind != "merge-stall" {
		t.Errorf("②' 제출 뒤 진행도 문턱 넘게 전: %+v", it)
	}
	// ②'' 기록은 읽었는데 진행 시각이 없음 — 제출 나이로
	m := mline(ip(20*60), nil)
	if it := mergeItems(m); len(it) != 1 || it[0].Kind != "merge-stall" {
		t.Errorf("②'' 진행 시각 없음·제출 20분: %+v", it)
	}
	// 기록을 모름(옛 판 병합기 등 — 단계 빈 값) — 제출 나이만으로는 정하지 않는다
	m.Phase = ""
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("기록 모름인데 항목: %+v", it)
	}
	// ③ 제출이 오래됐지만 병합기가 그 제출 뒤 착지를 시작함(착지 자식이 살아 진행 — 문턱 안) — 항목 없음
	m = mline(ip(50*60), ip(30))
	m.Landing, m.LandingAgeS = "ln-a", ip(10*60)
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("③ 착지 시작 10분째에 항목: %+v", it)
	}
	// ③' 제출 뒤 진행(추측 띄움 등)이 문턱 안 — 항목 없음
	if it := mergeItems(mline(ip(50*60), ip(5*60))); len(it) != 0 {
		t.Errorf("③' 제출 뒤 5분 전 진행인데 항목: %+v", it)
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
	// ④' 착지 안쪽 단계(공개 선점 기다림) — 「착지 검증 N분째」가 아니라 그 단계와 그 나이
	m.Inner, m.InnerAgeS = "공개 선점 기다림 — 쥔 쪽 ln-x(box-2)", ip(12*60)
	if it := mergeItems(m); len(it) != 1 || it[0].Kind != "merge-slow" ||
		it[0].What != "병합 줄이 느림: p — 맨 앞 ln-a 공개 선점 기다림 — 쥔 쪽 ln-x(box-2) 12분째(착지 34분째)" || !strings.Contains(it[0].Cost, "그 단계") {
		t.Errorf("④' 안쪽 단계: %+v", it)
	}
	m.Inner, m.InnerAgeS = "", nil
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

// ht-stall-spec-alive(2026-11-25): 병합기가 맨 앞 추측 검증을 기다리는 중이고 그 검증이 살아서 진행 중이면 「안 움직임」이 아니다 —
// 검증 기계의 추측을 20분 넘게 기다리는 동안 검증은 진행 중이었는데 merge-stall이 올라갔다. 이제 진행 여부는 병합기가 기록에 남기고
// (마지막 진행 시각), hc-fill은 기다리는 대상의 주소가 살아 있는지만 본다.
func TestMergeStallSpecAlive(t *testing.T) {
	m := mline(ip(25*60), ip(30))
	m.Phase, m.PhaseWhy = "spec-wait", "추측 검증 기다림 — ln-a"
	m.Waits = []MergeWait{{Machine: "box-2", Addr: "box-2/t2h2", Role: "추측", Live: "live"}}
	// ① 살아 있고 진행 중 — 항목 없음
	if it := mergeItems(m); len(it) != 0 {
		t.Errorf("① 추측 검증 진행 중인데 항목: %+v", it)
	}
	// ② 살아 있으나 오래 멈춤 — 안 움직임, 근거에 그 터미널과 진행 없음
	m.LastRoundAgeS = ip(40 * 60)
	it := mergeItems(m)
	if len(it) != 1 || it[0].Kind != "merge-stall" || !strings.Contains(it[0].Evidence, "box-2/t2h2 진행 없음") {
		t.Errorf("② 멈춘 추측 검증: %+v", it)
	}
	// 살아 있는지 못 읽음(hc ls 실패) — 「진행 없음」·「터미널 없음」이 아니라 모름
	m.Waits[0].Live = "?"
	it = mergeItems(m)
	if len(it) != 1 || !strings.Contains(it[0].Evidence, "box-2/t2h2 살아 있는지 모름") || strings.Contains(it[0].Evidence, "t2h2 진행 없음") {
		t.Errorf("못 읽음: %+v", it)
	}
	// ③ 터미널 없음 — 안 움직임, 근거에 검증 기계 주소와 터미널 없음
	m.Waits[0].Live = "gone"
	it = mergeItems(m)
	if len(it) != 1 || it[0].Kind != "merge-stall" || !strings.Contains(it[0].Evidence, "box-2/t2h2 터미널 없음") {
		t.Errorf("③ 터미널 없음: %+v", it)
	}
	// 병합기가 주소를 못 찾음 — 기계로
	m.Waits[0].Addr = ""
	if it := mergeItems(m); len(it) != 1 || !strings.Contains(it[0].Evidence, "추측 box-2 터미널 모름") {
		t.Errorf("주소 모름: %+v", it)
	}
	// 추측 기다림이 아님(기다리는 대상 없음) — 지금 그대로
	if it := mergeItems(mline(ip(25*60), ip(40*60))); len(it) != 1 || strings.Contains(it[0].Evidence, "추측") {
		t.Errorf("기다림 아님: %+v", it)
	}
}
