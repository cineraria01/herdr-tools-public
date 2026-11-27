package main

import "testing"

// claudeProj: 2026-11-28 실측 — 「/이어 가기.A」 폴더의 대화 기록이 ~/.claude/projects/…-------A 에 생겼다(영숫자 밖 글자마다 -)
func TestClaudeProj(t *testing.T) {
	for in, want := range map[string]string{
		"/x/scratchpad/이어 가기.A":         "-x-scratchpad-------A",
		"/Users/u/app/herdr-tools.wt/x": "-Users-u-app-herdr-tools-wt-x",
		"/a/😀b":                         "-a---b",
	} {
		if got := claudeProj(in); got != want {
			t.Errorf("claudeProj(%q) = %q, 바람 %q", in, got, want)
		}
	}
}

// leaseOwnerOK: 임대 제목 「hc-lane lease <레인> <기계> <UTC>」의 레인·기계가 같아야
func TestLeaseOwnerOK(t *testing.T) {
	for _, c := range []struct {
		sub, lane, who string
		ok             bool
	}{
		{"hc-lane lease rv-a w1 2026-11-28T00:00:00Z", "rv-a", "w1", true},
		{"hc-lane lease rv-a box 2026-11-28T00:00:00Z", "rv-a", "w1", false},
		{"hc-lane lease rv-b w1 2026-11-28T00:00:00Z", "rv-a", "w1", false},
		{"?", "rv-a", "w1", false},
		{"hc-lane lease rv-a w1", "rv-a", "w1", false},
	} {
		if got := leaseOwnerOK(c.sub, c.lane, c.who); got != c.ok {
			t.Errorf("leaseOwnerOK(%q, %q, %q) = %v", c.sub, c.lane, c.who, got)
		}
	}
}
