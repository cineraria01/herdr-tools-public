package main

import (
	"reflect"
	"testing"
)

// long-func-split: launchMain에서 뗀 판정 단계 — 읽기·쓰기 없이 입력 → 결정
func TestLaunchStepDecisions(t *testing.T) {
	ls := `{"spaces":[{"id":"m1/a","name":"other","machine":"m1"},{"id":"m1/sp9","name":"m1","machine":"m1"},{"id":"x","name":null,"machine":null}]}`
	for _, c := range []struct{ out, on, want string }{
		{ls, "m1", "sp9"},
		{ls, "m2", "lanes"},
		{"not json", "m1", "lanes"},
		{`{"spaces":[{"id":"m3","name":"m3","machine":"m3"}]}`, "m3", "m3"}, // 「<기계>/」가 없으면 id 그대로
	} {
		if g := spaceFor(c.out, c.on); g != c.want {
			t.Errorf("spaceFor(%q, %q) = %q, 원하는 값 %q", c.out, c.on, g, c.want)
		}
	}

	for _, c := range []struct {
		r, f, d bool
		want    string
	}{
		{true, true, true, "\tread"}, {false, true, true, "\tfill"}, {false, false, true, "\tdirect"}, {false, false, false, ""},
	} {
		if g := laneKind(c.r, c.f, c.d); g != c.want {
			t.Errorf("laneKind(%v,%v,%v) = %q, 원하는 값 %q", c.r, c.f, c.d, g, c.want)
		}
	}

	for _, c := range []struct {
		v    string
		fill bool
		want []string
		ok   bool
	}{
		{"yes", false, nil, true},
		{"yes", true, []string{"--no-parent"}, true},
		{"no", true, []string{"--no-notify", "--no-parent"}, true},
		{"maybe", false, nil, false},
	} {
		g, ok := notifyArgs(c.v, c.fill)
		if ok != c.ok || !reflect.DeepEqual(g, c.want) {
			t.Errorf("notifyArgs(%q, %v) = %q %v, 원하는 값 %q %v", c.v, c.fill, g, ok, c.want, c.ok)
		}
	}

	if !hasBriefSlot([]string{"claude", "읽어라 {brief}"}) || hasBriefSlot([]string{"claude", "{bri"}) || hasBriefSlot(nil) {
		t.Error("hasBriefSlot: {brief} 자리 판정이 틀렸다")
	}
	if !jobOK("loop-1~a") || jobOK("Loop") || jobOK("a b") {
		t.Error("jobOK: HC_LANE_JOB 글자 판정이 틀렸다")
	}
}
