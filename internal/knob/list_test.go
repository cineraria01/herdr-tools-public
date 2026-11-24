package knob

import "testing"

// 목록 손잡이(auto_submit — fill-auto-submit): 「-」 또는 쉼표로 나눈 이름만, 빈 이름·공백·셸 글자는 거절
func TestListKnobCheck(t *testing.T) {
	d, ok := Find("auto_submit")
	if !ok || !d.List || d.Default != "-" {
		t.Fatalf("auto_submit 등록: %+v %v", d, ok)
	}
	for _, v := range []string{"-", "app", "app,herdr-tools", "a_b.c-d"} {
		if err := d.Check(v); err != nil {
			t.Fatalf("%q 거절: %v", v, err)
		}
	}
	for _, v := range []string{"", ",", "app,", "a b", "a;b", "-,app"} {
		if d.Check(v) == nil {
			t.Fatalf("%q 받음", v)
		}
	}
	if m, ok := Find("auto_submit_max"); !ok || m.Check("120") != nil || m.Check("5") == nil || m.Check("601") == nil {
		t.Fatalf("auto_submit_max 범위: %+v", m)
	}
}
