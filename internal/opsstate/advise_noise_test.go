package opsstate

// ht-advise-noise(2026-11-28): 손댈 것이 ① 끊긴 기계의 끝난 옛 줄 175건을 레인마다 lane-long으로 ② 끝남·정리됨 레인을 「같은 단계에 713분」으로
// ③ --then 이어 붙인 단계를 선행으로 가진 줄을 「풀 수 있음」으로 냈다. 고치기 전 코드로는 아래가 실패한다

import (
	"fmt"
	"strings"
	"testing"
)

// down: 기계 x가 끊김 — 끝난 옛 줄 여럿(착지·정리됨, 단계 「모름(연결 안 됨)」) + 살아 있는 줄 하나
func downState(n int) State {
	s := aBase()
	s.Machines = append(s.Machines, MachineUnknown("x", "연결 안 됨"))
	for i := range n {
		ev := "정리됨 - 정리"
		if i%2 == 0 {
			ev = fmt.Sprintf("착지 abc%05d 넣음", i)
		}
		s.Lanes = append(s.Lanes, Lane{Name: fmt.Sprintf("old-%d", i), Machine: "x", Stage: "모름(연결 안 됨)", StageAgeS: ip(7 * 24 * 3600), LastEvent: ev, Open: true})
	}
	s.Lanes = append(s.Lanes, Lane{Name: "alive-x", Machine: "x", Stage: "모름(연결 안 됨)", StageAgeS: ip(300 * 60), LastEvent: "중간 - 반쯤", Open: true})
	return s
}

// ① 끊긴 기계 + 끝난 줄 수백 + 살아 있는 줄 하나 → 그 기계 항목 하나에 살아 있는 레인 이름, lane-long 0
func TestAdviseNoiseDownMachineOneItem(t *testing.T) {
	s := downState(175)
	f := Facts{Addr: map[string]string{"alive-x": "x/t1"}}
	all := Advise(s, f, ath)
	if l := kinds(all, "lane-long"); len(l) != 0 {
		t.Fatalf("끊긴 기계 레인은 lane-long 0이어야: %d개 %+v", len(l), l[0])
	}
	m := kinds(all, "machine")
	if len(m) != 1 || m[0].Key != "machine:x" {
		t.Fatalf("기계 항목 하나: %+v", m)
	}
	if want := "기계 x 모름: 연결 안 됨 — 살아 있는 레인 1개(alive-x)"; m[0].What != want {
		t.Errorf("기계 항목:\n got %q\nwant %q", m[0].What, want)
	}
	if len(all) != 1 {
		t.Errorf("항목은 기계 하나뿐이어야: %+v", all)
	}
}

// 살아 있는 레인이 많으면 앞 다섯 + 외 k
func TestAdviseNoiseDownMachineManyLive(t *testing.T) {
	s := downState(0)
	for i := range 7 {
		s.Lanes = append(s.Lanes, Lane{Name: fmt.Sprintf("live-%d", i), Machine: "x", Stage: "모름(연결 안 됨)", Open: true})
	}
	m := kinds(Advise(s, Facts{}, ath), "machine")
	if len(m) != 1 || !strings.HasSuffix(m[0].What, "살아 있는 레인 8개(alive-x, live-0, live-1, live-2, live-3 외 3)") {
		t.Fatalf("많으면 앞 다섯 + 외 k: %+v", m)
	}
}

// ② 닿는 기계의 살아 있는 줄이 문턱을 넘으면 lane-long은 지금처럼 · ④ 기계가 돌아오면 레인별 판정 복귀
func TestAdviseNoiseReachableStillLong(t *testing.T) {
	s := downState(3)
	s.Machines[1] = Machine{Name: "x", Reach: "ok"} // 돌아옴
	s.Lanes = append(s.Lanes, Lane{Name: "stuck-a", Machine: "a", Stage: "작업 중(working)", StageAgeS: ip(120 * 60), Open: true})
	got := map[string]bool{}
	for _, it := range kinds(Advise(s, Facts{}, ath), "lane-long") {
		got[it.Key] = true
	}
	if !got["lane-long:stuck-a"] || !got["lane-long:alive-x"] || len(got) != 2 {
		t.Fatalf("닿는 기계의 살아 있는 줄만(끝난 옛 줄 빼고): %v", got)
	}
}

// ③ 끝남·정리됨·착지 줄은 터미널이 살아 있어도(또는 모름) 어떤 문턱에도 lane-long에 안 나온다
func TestAdviseNoiseEndedNeverLong(t *testing.T) {
	s := aBase()
	s.Lanes = append(s.Lanes,
		Lane{Name: "cleaned", Machine: "a", Stage: "감사 통과 받음", StageAgeS: ip(713 * 60), LastEvent: "정리됨 - 워크트리 정리", Open: true},
		Lane{Name: "landed", Machine: "a", Stage: "착지 abcd1234", StageAgeS: ip(500 * 60), LastEvent: "착지 abcd1234 넣음", Open: true},
		Lane{Name: "ended", Machine: "a", Stage: "끝남", StageAgeS: ip(500 * 60), Open: true})
	for _, th := range []Thresholds{ath, {StageMin: 1}} {
		for _, it := range kinds(Advise(s, Facts{}, th), "lane-long") {
			if it.Key != "lane-long:lane-front" { // 일하는 앞 레인은 문턱 1분이면 나오는 것이 맞다
				t.Errorf("문턱 %d분: 끝난 줄이 lane-long %+v", th.StageMin, it)
			}
		}
	}
	// 끝남 판정은 Finished와 한 정의 — 터미널이 끝난 같은 줄은 Finished
	for _, l := range s.Lanes[1:] {
		l.Open = false
		if !Ended(l) || !Finished(l) {
			t.Errorf("%s: Ended %v Finished %v", l.Name, Ended(l), Finished(l))
		}
	}
	if Ended(s.Lanes[0]) {
		t.Errorf("일하는 앞 레인이 끝남으로")
	}
}

// ⑤ --then 이어 붙인 단계를 --after로 가진 줄 → 「풀 수 있음」 아님, 근거 글은 「앞 레인 <받은 줄>의 이어서 단계 <이름> 대기」
// ⑥ 진짜로 대기열에 없는 선행 → 지금처럼 권고
func TestAdviseNoiseThenStep(t *testing.T) {
	s := aBase()
	f := Facts{Waits: []Wait{
		{Lane: "after-then", Project: "p", AgeS: ip(30 * 60), After: []string{"step-2"}},
		{Lane: "after-gone", Project: "p", AgeS: ip(30 * 60), After: []string{"lane-gone"}},
		{Lane: "head", Project: "p", AgeS: ip(30 * 60), After: []string{"step-2"}},
		{Lane: "tail", Project: "p", AgeS: ip(30 * 60), After: []string{"head"}},
	},
		Then: map[string]string{"step-2": "lane-front"},
		Pred: map[string]Lane{"lane-front@p": {Name: "lane-front", Machine: "a", Stage: "작업 중(working)", StageAgeS: ip(40 * 60)}}}
	by := map[string]Item{}
	for _, it := range Advise(s, f, ath) {
		by[it.Key] = it
	}
	if it, ok := by["free-wait:after-then"]; ok {
		t.Errorf("이어 붙인 단계 선행에 풀 수 있음: %+v", it)
	}
	if _, ok := by["free-wait:after-gone"]; !ok {
		t.Errorf("대기열에 없는 선행은 지금처럼 권고: %+v", by)
	}
	c, ok := by["chain:head"]
	if !ok || !strings.Contains(c.Evidence, "step-2 앞 레인 lane-front의 이어서 단계 step-2 대기(작업 중(working) 40분째(a))") {
		t.Errorf("사슬 머리 선행 글: %+v", c)
	}
	// 받은 줄이 끝났으면(이어서 단계가 착지 없이 사라짐) 지금처럼 권고 — 영영 안 풀리는 줄을 숨기지 않는다
	f.Pred["lane-front@p"] = Lane{Name: "lane-front", Machine: "a", Stage: "끝남", LastEvent: "정리됨 - 정리"}
	by = map[string]Item{}
	for _, it := range Advise(s, f, ath) {
		by[it.Key] = it
	}
	if _, ok := by["free-wait:after-then"]; !ok {
		t.Errorf("받은 줄이 끝난 이어서 단계는 권고: %+v", by)
	}
}

// 바퀴 순서 반대: 기계 연결을 모름(hc machine ls 못 읽음) → 끊김으로 단정하지 않고 판정 못 함 항목에 묶는다
func TestAdviseNoiseUnknownConnNotDown(t *testing.T) {
	s := downState(2)
	s.Machines[1] = MachineUnknown("x", "연결 모름(hc machine ls 못 읽음)")
	all := Advise(s, Facts{}, ath)
	if len(kinds(all, "lane-long")) != 0 || len(kinds(all, "machine")) != 0 {
		t.Fatalf("연결 모름은 끊김(machine)도 lane-long도 아님: %+v", all)
	}
	u := kinds(all, "unknown")
	if len(u) != 1 || !strings.Contains(u[0].What, "살아 있는 레인 1개(alive-x)") || strings.Contains(u[0].Summary(), "연결 안 됨") {
		t.Fatalf("판정 못 함 항목 하나에 묶음: %+v", u)
	}
}

// 외부 가정: 이름이 바뀐(설정에 없는) 기계의 끝난 옛 줄은 0건, 주인 없는 살아 있는 줄은 그대로
func TestAdviseNoiseRenamedMachine(t *testing.T) {
	s := aBase()
	s.Lanes = append(s.Lanes,
		Lane{Name: "old-renamed", Machine: "old-name", Stage: "모름(연결 안 됨)", StageAgeS: ip(7 * 24 * 3600), LastEvent: "착지 abcd1234 넣음", Open: true},
		Lane{Name: "live-renamed", Machine: "old-name", Stage: "모름(연결 안 됨)", StageAgeS: ip(200 * 60), LastEvent: "중간 - 반쯤", Open: true})
	l := kinds(Advise(s, Facts{}, ath), "lane-long")
	if len(l) != 1 || l[0].Key != "lane-long:live-renamed" {
		t.Fatalf("이름 바뀐 기계: 살아 있는 줄 하나만: %+v", l)
	}
}

// 두 번째 실행: 같은 재료 두 번 → 같은 항목·같은 열쇠
func TestAdviseNoiseStable(t *testing.T) {
	a, b := Advise(downState(5), Facts{}, ath), Advise(downState(5), Facts{}, ath)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("같은 재료 다른 항목:\n%+v\n%+v", a, b)
	}
}
