package opsstate

import (
	"strings"
	"testing"
)

// fill-polish-1124 가: 코어당 부하 치우침은 옮길 일이 있을 때만(대기 줄이 바쁜 기계로만 갈 때) 손댈 것으로 낸다

// skewBase: 바쁜 h(자리 남음, 도는 레인 2)와 한가한 c(자리 남음)
func skewBase() State {
	return State{Machines: []Machine{
		{Name: "h", Reach: "ok", Cores: ip(10), LoadPerCore: fp(3), Live: ip(2), Seats: ip(4), Lanes: ip(2), LaneCap: ip(4)},
		{Name: "c", Reach: "ok", Cores: ip(8), LoadPerCore: fp(0.1), Live: ip(0), Seats: ip(3), Lanes: ip(0), LaneCap: ip(3)},
	}}
}

// 대기 0 + 치우침 → 항목 없음(고치기 전 코드로 실패: 바쁨·한가만 보고 냈다)
func TestSkewNoWaitNoItem(t *testing.T) {
	if it := kinds(Advise(skewBase(), Facts{}, ath), "skew"); len(it) != 0 {
		t.Fatalf("대기 줄 없이 치우침 항목: %+v", it)
	}
}

// 대기 있음 + 다음 차례가 바쁜 기계(--only h) → 항목 있음, 근거에 대기 줄 수·바쁜 기계 도는 레인 수
func TestSkewWaitToHot(t *testing.T) {
	f := Facts{Waits: []Wait{{Lane: "w1", Project: "p", Can: []string{"h"}}}}
	it := kinds(Advise(skewBase(), f, ath), "skew")
	if len(it) != 1 {
		t.Fatalf("항목 %d개: %+v", len(it), it)
	}
	for _, w := range []string{"바쁨 h 3.00(도는 레인 2)", "한가 c 0.10", "대기 줄 1 가운데 바쁜 기계로만 갈 줄 1(w1)"} {
		if !strings.Contains(it[0].Evidence, w) {
			t.Errorf("근거에 %q 없음: %s", w, it[0].Evidence)
		}
	}
}

// 대기 있음 + 다음 차례가 한가한 기계(둘 다 갈 수 있음 · 바쁜 기계 자리 참) → 항목 없음
func TestSkewWaitToCool(t *testing.T) {
	f := Facts{Waits: []Wait{{Lane: "w1", Project: "p"}}} // Can nil = 모든 기계 — 한가한 c가 먼저
	if it := kinds(Advise(skewBase(), f, ath), "skew"); len(it) != 0 {
		t.Fatalf("한가한 기계로 갈 줄인데 항목: %+v", it)
	}
	s := skewBase()
	s.Machines[0].Live = ip(4) // h 자리 참 — h로 갈 줄이 없다
	f = Facts{Waits: []Wait{{Lane: "w1", Project: "p", Can: []string{"h"}}}}
	if it := kinds(Advise(s, f, ath), "skew"); len(it) != 0 {
		t.Fatalf("바쁜 기계 자리 참인데 항목: %+v", it)
	}
}

// 띄울 차례가 아닌 줄(거절·겹침·단독·대기 줄 선행)은 옮길 일로 세지 않는다
func TestSkewMovableNotReady(t *testing.T) {
	free, hot := map[string]int{"h": 1}, map[string]bool{"h": true}
	ws := []Wait{
		{Lane: "rj", Reject: "x"}, {Lane: "fr", Front: "y"}, {Lane: "so", Solo: true},
		{Lane: "head"}, {Lane: "kid", After: []string{"head@p"}},
	}
	if got := strings.Join(SkewMovable(ws, free, hot), ","); got != "head" {
		t.Fatalf("옮길 줄: %q", got)
	}
}

// ht-skew-verify-cause: 바쁜 기계 글에 도는 저장소 검증 수 — 레인 0·검증 1이면 「검증 1」, 모름이면 그 칸 없음, 0이면 지금 글 그대로.
// 상태 한 장 기계 줄도 같은 값(모름·0이면 없음)
func TestSkewVerifies(t *testing.T) {
	f := Facts{Waits: []Wait{{Lane: "w1", Project: "p", Can: []string{"h"}}}}
	for _, c := range []struct {
		lanes, ver *int
		want       string
	}{
		{ip(0), ip(1), "바쁨 h 3.00(도는 레인 0 · 검증 1) "},
		{ip(0), nil, "바쁨 h 3.00(도는 레인 0) "},
		{ip(2), ip(0), "바쁨 h 3.00(도는 레인 2) "},
	} {
		s := skewBase()
		s.Machines[0].Lanes, s.Machines[0].Verifies = c.lanes, c.ver
		it := kinds(Advise(s, f, ath), "skew")
		if len(it) != 1 || !strings.Contains(it[0].Evidence, c.want) {
			t.Errorf("근거에 %q 없음: %+v", c.want, it)
		}
		var b strings.Builder
		if err := Human(&b, s); err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(b.String(), "레인 0/4 · 검증 1")
		if has != (c.ver != nil && *c.ver > 0) || c.ver == nil && strings.Contains(b.String(), "검증") {
			t.Errorf("기계 줄 검증 칸(%v): %s", c.ver, b.String())
		}
	}
}
