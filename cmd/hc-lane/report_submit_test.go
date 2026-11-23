package main

// fix-inbox-submit-id(2026-11-23): 「제출」은 hc-lane land만 적는다 — 제출 ID 키(--key submit:<ID>) 없는 손 report 제출은 종료 2(사건 안 씀).
// 손으로 적은 제출의 첫 낱말이 제출 ID로 읽혀 받은편지 회신이 이틀 멈췄다

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReportSubmitNeedsSubmitID(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	st := d + "/st"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+st, "HC_TERMINAL_ID=", "HC_LANE=lr", "HC_LANE_MACHINE=m",
		"HC_LANE_JOB=", "HC_LANE_OWNER=")
	run := func(a ...string) (int, string) {
		c := exec.Command(bin, append([]string{"report"}, a...)...)
		c.Dir, c.Env = d, env
		var e strings.Builder
		c.Stderr = &e
		err := c.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), e.String()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, e.String()
	}
	nlines := func() int {
		b, _ := os.ReadFile(st + "/hc-lane/events.tsv")
		return strings.Count(string(b), "\n")
	}
	for _, a := range [][]string{
		{"제출", "-", "감사 완료(낮음 4) → 병합기에 넘김"},
		{"제출", "-", "box-20261121T010203-4242 손으로"},                 // 키 없이 ID 꼴 첫 낱말도 손이다
		{"제출", "-", "x", "--key", "submit:audit"},                   // 키의 ID가 꼴이 아님
		{"제출", "-", "x", "--key", "other:box-20261121T010203-4242"}, // submit: 키가 아님
	} {
		if rc, e := run(a...); rc != 2 || !strings.Contains(e, "hc-lane land") || nlines() != 0 {
			t.Fatalf("손 제출 %q: rc %d 사건 %d 오류 %q — 종료 2·사건 없음·land 안내여야", a, rc, nlines(), e)
		}
	}
	sid := "box-20261121T010203-4242"
	if rc, e := run("제출", "-", sid+" → refs/merge-queue/1-lr", "--key", "submit:"+sid); rc != 0 || nlines() != 1 {
		t.Fatalf("land가 적는 꼴의 제출: rc %d 사건 %d 오류 %q", rc, nlines(), e)
	}
	if rc, e := run("중간", "-", "감사 완료(낮음 4)"); rc != 0 || nlines() != 2 { // 대조: 다른 상태는 그대로
		t.Fatalf("중간: rc %d 사건 %d 오류 %q", rc, nlines(), e)
	}
}
