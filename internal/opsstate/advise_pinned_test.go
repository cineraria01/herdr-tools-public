package opsstate

import (
	"strings"
	"testing"
)

// ht-advise-only-stuck(2026-11-25): --only로 묶은 대기 줄이 묶인 기계 상한 때문에 advise_pinned분 넘게 못 뜨고 다른 기계에 빈 자리가 있으면
// 손댈 것 「묶인 기계 때문에 못 뜸」. 고치기 전 코드: Wait에 Pin 칸이 없어 컴파일부터 실패

func pinBase() State {
	return State{Machines: []Machine{
		{Name: "a", Reach: "ok", Live: ip(3), Seats: ip(3), Free: ip(0)},
		{Name: "b", Reach: "ok", Live: ip(0), Seats: ip(3), Free: ip(3)},
	}}
}

func pinWait(minutes int) Wait {
	return Wait{Lane: "w1", Project: "p", Can: []string{"a"}, Wider: []string{"b"}, Pin: "a 상한 — 부하 18.9/10", PinS: ip(minutes * 60)}
}

var pth = Thresholds{IdleMin: 10, PinMin: 10}

func TestPinnedItem(t *testing.T) {
	it := kinds(Advise(pinBase(), Facts{Waits: []Wait{pinWait(11)}}, pth), "pinned")
	if len(it) != 1 || it[0].Key != "pinned:w1" || len(it[0].Next) != 1 ||
		it[0].Next[0] != "hc-fill edit w1 --only a,b --why '묶인 기계 넓히기(hc-fill advise)'" {
		t.Fatalf("항목: %+v", it)
	}
	for _, w := range []string{"묶인 기계 a 상한 — 부하 18.9/10", "다른 기계 빈 자리 3(b 3)"} {
		if !strings.Contains(it[0].Evidence, w) {
			t.Errorf("근거에 %q 없음: %s", w, it[0].Evidence)
		}
	}
	if !strings.Contains(it[0].What, "다른 기계에 자료를 두고") {
		t.Errorf("무엇: %s", it[0].What)
	}
}

func TestPinnedNoItem(t *testing.T) {
	full := pinBase()
	full.Machines[1].Live, full.Machines[1].Free = ip(3), ip(0)
	for name, c := range map[string]struct {
		s State
		w Wait
		t Thresholds
	}{
		"다른 기계 빈 자리 없음": {full, pinWait(11), pth},
		"문턱 안":          {pinBase(), pinWait(5), pth},
		"묶음 없음":         {pinBase(), Wait{Lane: "w1", Project: "p"}, pth},
		"손잡이 0(끔)":      {pinBase(), pinWait(60), Thresholds{IdleMin: 10}},
	} {
		if it := kinds(Advise(c.s, Facts{Waits: []Wait{c.w}}, c.t), "pinned"); len(it) != 0 {
			t.Errorf("%s: 항목 %+v", name, it)
		}
	}
}
