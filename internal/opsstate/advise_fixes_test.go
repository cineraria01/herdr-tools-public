package opsstate

// advise-fixes(2026-11-24): 손댈 것의 구멍 — ① 추린 상태에서 「감시 안 돎」이 사라짐 ② 겹침 풀기 권고가 rm && add ③ 멈춘 레인 권고가
// 맨 hc send ⑤ idle 0이 「문턱 0」. 고치기 전 코드: ①②③⑤ 실패(①은 Watch.Need가 없어 컴파일부터)

import (
	"strings"
	"testing"
)

// ① 지휘 터미널이 살아 있고 열린 줄이 있는데 감시가 없음 → 추린 상태로도 「감시 안 돎」(확실). 지휘가 사라진 흔적(Need 거짓)은 항목 없음 —
// 그 지휘의 사건은 다른 살아 있는 지휘의 감시가 받는다(hc-lane watch wfilter). 추린 출력의 감시 칸은 그대로(숨긴 수만)
func TestAdviseDeadWatchAfterTrim(t *testing.T) {
	s := aBase()
	s.Watch = []Watch{{Owner: "m/t1", Alive: bp(false), Need: true}, {Owner: "m/gone", Alive: bp(false)}, {Owner: "m/t2", Alive: bp(true), Need: true}}
	tr := Trim(s, 60)
	if len(tr.Watch) != 1 || tr.Hidden.Watch != 2 {
		t.Fatalf("추린 감시 칸이 바뀜: %+v hidden %+v", tr.Watch, tr.Hidden)
	}
	for _, st := range []State{tr, s} { // 추린 것·전체(--all) 둘 다 같은 항목
		w := kinds(Advise(st, Facts{}, ath), "watch")
		if len(w) != 1 || w[0].Key != "watch:m/t1" || w[0].Sure != SureYes || len(w[0].Next) != 1 || w[0].Next[0] != "hc-lane coord --watch m/t1" {
			t.Errorf("감시 안 돎: %+v", w)
		}
	}
	// 두 번째로 추려도(추린 것을 다시) 같은 항목
	if w := kinds(Advise(Trim(tr, 60), Facts{}, ath), "watch"); len(w) != 1 {
		t.Errorf("두 번 추림: %+v", w)
	}
}

// ② 겹침 대기의 명령은 edit --overlap-ok(rm 없음 — rm 뒤 add가 실패하면 줄이 사라지고 --solo 등 옵션이 안 돌아온다). 주차 중이어도
// edit는 줄 ID·주차 표식을 지키니 주차 중 줄도 같은 명령(cmd/hc-fill advise_keys_test가 주차 줄로 본다)
func TestAdviseFreeWaitEdit(t *testing.T) {
	f := Facts{Waits: []Wait{{Lane: "doc-a", Project: "p", AgeS: ip(30 * 60), Front: "lane-front", Keys: []string{"docs/x.md"}}}}
	it := kinds(Advise(aBase(), f, ath), "free-wait")
	want := "hc-fill edit doc-a --overlap-ok --why '겹침 풀기(hc-fill advise)'"
	if len(it) != 1 || len(it[0].Next) != 1 || it[0].Next[0] != want || it[0].Sure != SureYes {
		t.Fatalf("겹침 대기: %+v", it)
	}
	if strings.Contains(it[0].Next[0], "hc-fill rm") {
		t.Errorf("rm이 든 명령: %s", it[0].Next[0])
	}
}

// ③ 멈춘 레인(error)의 명령은 hc-fill nudge(주소·이름 대조·되풀이 막기) — 맨 hc send 없음
func TestAdviseLaneErrorNudge(t *testing.T) {
	s := aBase()
	s.Lanes[0].Stage = "작업 중(error)"
	it := kinds(Advise(s, Facts{Addr: map[string]string{"lane-front": "w/t5"}}, ath), "lane-error")
	if len(it) != 1 || strings.Join(it[0].Next, "\n") != "hc read w/t5\nhc-fill nudge lane-front --why '멈춘 레인(hc-fill advise)'" {
		t.Fatalf("멈춘 레인: %+v", it)
	}
}

// ⑤ idle 0이면 풀 수 있는 대기·사슬을 내지 않는다(문서 「0이면 끔」) — 1 이상은 지금처럼
func TestAdviseIdleZeroOff(t *testing.T) {
	f := Facts{Waits: []Wait{
		{Lane: "doc-a", Project: "p", AgeS: ip(30), Front: "lane-front", Keys: []string{"docs/x.md"}},
		{Lane: "c1", Project: "p", AgeS: ip(30), After: []string{"lane-front"}},
		{Lane: "c2", Project: "p", AgeS: ip(30), After: []string{"c1"}},
		{Lane: "nil-age", Project: "p", Front: "lane-front", Keys: []string{"docs/y.md"}}, // 나이 모름도 끔이면 내지 않음
	}}
	th := ath
	th.IdleMin = 0
	if it := Advise(aBase(), f, th); len(kinds(it, "free-wait"))+len(kinds(it, "chain")) != 0 {
		t.Errorf("idle 0인데 대기 항목: %+v", it)
	}
	th.IdleMin = 1 // 30초 줄은 아직, 나이 모름 줄은 지금처럼 항목
	if it := kinds(Advise(aBase(), f, th), "free-wait"); len(it) != 1 || it[0].Key != "free-wait:nil-age" {
		t.Errorf("idle 1: %+v", it)
	}
	for i := range f.Waits {
		if f.Waits[i].AgeS != nil {
			f.Waits[i].AgeS = ip(120)
		}
	}
	if it := Advise(aBase(), f, th); len(kinds(it, "free-wait")) != 3 || len(kinds(it, "chain")) != 1 {
		t.Errorf("idle 1·2분 기다림: %+v", it)
	}
}
