package knob

import (
	"testing"
	"time"
)

// 묶음 손잡이(fill-load-steer): 「접두@값」만 찾고, 사본 Name은 저장 이름, 범위는 표 줄 그대로
func TestFamilyFind(t *testing.T) {
	d, ok := Find("seats@box-1")
	if !ok || d.Name != "seats@box-1" || d.Of() != "box-1" || d.Check("2") != nil || d.Check("100") == nil {
		t.Fatalf("seats@box-1 = %+v %v", d, ok)
	}
	for _, n := range []string{"seats@", "seats", "@box", "seats@a@b", "seats@a b", "nope@box"} {
		if _, ok := Find(n); ok {
			t.Errorf("%q가 찾아졌다", n)
		}
	}
	if d, _ := Find("idle"); d.Of() != "" {
		t.Errorf("낱개 손잡이 Of = %q", d.Of())
	}
	now := time.Unix(1_000_000, 0)
	if !(Entry{Name: "pause@box-1", Value: "1"}).Live(now) || (Entry{Name: "pause@box-1", Value: "2"}).Live(now) {
		t.Error("pause@ 값 범위")
	}
}
