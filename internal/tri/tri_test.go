package tri

import "testing"

// 0값이 모름 — 빠뜨린 대입·깜빡 돌려준 0값이 있음·없음이 되지 않는다
func TestZeroIsUnknown(t *testing.T) {
	var s State
	if s != Unknown || s.String() != "모름" {
		t.Fatalf("0값 = %d %q, 바람 모름", s, s)
	}
	for v, w := range map[State]string{Yes: "있음", No: "없음", Unknown: "모름", State(9): "모름"} {
		if v.String() != w {
			t.Errorf("%d.String() = %q, 바람 %q", v, v.String(), w)
		}
	}
	if Of(true) != Yes || Of(false) != No {
		t.Fatal("Of: 참 있음 · 거짓 없음")
	}
}
