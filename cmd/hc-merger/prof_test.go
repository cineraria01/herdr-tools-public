package main

import (
	"strings"
	"testing"
)

// fill-polish-1124: 머리 레인 — 제 머리 가지 먼저, 없으면 짝 참조의 원래 레인, 둘 다 없으면 그대로
func TestPRLane(t *testing.T) {
	a, b := strings.Repeat("1", 40), strings.Repeat("2", 40) // 비밀 꼴 검사에 안 걸리게 실행 중에
	list := a + "\trefs/heads/pr/orig\n" + b + "\trefs/herdr/pr-of/fix/orig\n" + b + "\trefs/heads/pr/own\n" + b + "\trefs/herdr/pr-of/own/orig"
	for lane, want := range map[string]string{"fix": "orig", "own": "own", "other": "other", "fi": "fi"} {
		if got := prLane(list, lane); got != want {
			t.Errorf("prLane(%s) = %s, 바람 %s", lane, got, want)
		}
	}
	m := &M{qs: list, b: []entry{{l: "fix", s: a}, {l: "other", s: b}}}
	if got := m.prHeads(); got != "orig="+a+"="+a {
		t.Errorf("prHeads = %q", got)
	}
}
