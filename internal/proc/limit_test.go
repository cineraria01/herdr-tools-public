//go:build !windows

package proc

// 바깥 명령 시간 상한 시험(go-followups-2): 가짜 git이 sleep으로 멈추면 상한 안에 무리째 거두고 TimeoutCode(모름)로 끝난다.

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLimitTable(t *testing.T) {
	t.Setenv("HERDR_TIMEOUT_GIT", "")
	t.Setenv("HERDR_TIMEOUT_REMOTE", "")
	t.Setenv("HERDR_TIMEOUT_LANE", "")
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"git", "-C", "/r", "remote", "get-url", "origin"}, 60},
		{[]string{"/usr/bin/git", "-c", "a=b", "fetch", "-q", "origin"}, 120},
		{[]string{"git", "remote", "update"}, 120},
		{[]string{"hc", "get", "x", "--json"}, 120},
		{[]string{"hc", "wait", "x"}, 0},
		{[]string{"hc", "run", "--name", "x", "cmd"}, 0},
		{[]string{"hc", "run", "--bg", "--name", "x", "cmd"}, 120},
		{[]string{"hc", "exec", "--timeout", "600", "--on", "m", "--", "sh"}, 0},
		{[]string{"ssh", "m", "true"}, 120},
		{[]string{"hc-lane", "coord"}, 600},
		{[]string{"/x/bin/hc-lane", "land", "main"}, 0},
		{[]string{"sh", "-c", "sleep 99"}, 0},
		// M04·F13: 옵션 값·넘기는 명령의 인자는 하위 명령·옵션으로 읽지 않는다
		{[]string{"hc-lane", "--name", "watch", "--repo", "repo", "--", "true"}, 600},
		{[]string{"hc-lane", "--name", "land", "--repo", "r", "--", "true"}, 600},
		{[]string{"hc", "exec", "--on", "m", "--", "hc-lane", "wait", "--timeout", "0"}, 120},
		{[]string{"hc", "exec", "--on", "m", "hc-lane", "wait", "--timeout", "0"}, 120},
		{[]string{"hc", "run", "--name", "x", "--", "sleep", "--bg"}, 0},
		{[]string{"hc", "exec", "--on", "--bg", "true"}, 120},
		{[]string{"hc", "send", "x", "--timeout=5", "--wait"}, 0},
	}
	for _, c := range cases {
		if got := Limit(c.args); got != time.Duration(c.want)*time.Second {
			t.Errorf("Limit(%v) = %v, 기대 %ds", c.args, got, c.want)
		}
	}
	t.Setenv("HERDR_TIMEOUT_GIT", "0")
	if got := Limit([]string{"git", "status"}); got != 0 {
		t.Errorf("HERDR_TIMEOUT_GIT=0 이면 상한 없음이어야: %v", got)
	}
}

// M10: 진단에는 명령·하위 명령만 — 사건 본문·주소·설정 값은 적지 않는다
func TestHeadHidesValues(t *testing.T) {
	cases := map[string][]string{
		"hc-lane report":                 {"hc-lane", "report", "중간", "-", "감사용본문"},
		"hc get":                         {"/opt/bin/hc", "get", "감사용주소", "--json"},
		"git -C /r remote get-url":       {"git", "-C", "/r", "remote", "get-url", "origin"},
		"git -c … fetch":                 {"git", "-c", "http.extraheader=비밀", "fetch", "https://u:비밀@x/r"},
		"git push":                       {"git", "push", "origin", "HEAD:refs/heads/main"},
		"ssh":                            {"ssh", "machine", "cat 비밀"},
		"hc-lane":                        {"hc-lane", "--name", "x"},
		"git -C /nowhere remote get-url": {"git", "-C", "/nowhere", "remote", "get-url", "origin"},
		"curl":                           {"curl", "-H", "Authorization: 비밀", "https://x"},
		"gh api":                         {"gh", "api", "repos/x"},
	}
	for want, args := range cases {
		if got := head(args); got != want {
			t.Errorf("head(%v) = %q, 기대 %q", args, got, want)
		}
	}
}

// fakeGit: PATH 앞에 sleep으로 멈추는 git(손자 sleep도 남긴다 — 무리째 거두는지)
func fakeGit(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	pidf := d + "/grandchild"
	sc := "#!/bin/sh\nsleep 60 & echo $! >" + pidf + "\nsleep 60\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	return pidf
}

func TestRunTimeout(t *testing.T) {
	pidf := fakeGit(t)
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	c := exec.Command("git", "-C", "/nowhere", "remote", "get-url", "origin")
	t0 := time.Now()
	out, err := Output(c)
	if el := time.Since(t0); el > 10*time.Second {
		t.Fatalf("상한 1초인데 %v 걸림", el)
	}
	var te *TimeoutError
	if !errors.As(err, &te) || out != nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("TimeoutError(ErrTimeout)·빈 출력이어야: err=%v out=%q", err, out)
	}
	if te.Error() != "시간 넘김 git -C /nowhere remote get-url 1" {
		t.Errorf("줄 꼴: %q", te.Error())
	}
	if Code(err) != TimeoutCode {
		t.Errorf("Code = %d, 기대 %d", Code(err), TimeoutCode)
	}
	b, rerr := os.ReadFile(pidf)
	if rerr != nil {
		t.Fatal(rerr)
	}
	gc, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if gc <= 1 || !gone(gc) {
		t.Errorf("손자 sleep %d 가 남았다 — 무리째 거두지 않음", gc)
	}
}

// M06·M07: 무리 밖 프로세스가 오류 관을 쥐고 계속 써도, Run은 출력 복사가 끝난 뒤에만 돌아오고(경합 없음 — -race) 결과는 모름이다
func TestRunHeldPipeNoRace(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	// 무리를 벗어난(perl setpgrp) 손자가 오류 관을 쥐고 4초 동안 쓴다 — 이 시험이 띄운 것이고 스스로 끝난다
	sc := "#!/bin/sh\nperl -e 'setpgrp(0,0); for (1..40) { print STDERR \"x\" x 100, \"\\n\"; select(undef,undef,undef,0.1) }' &\necho $! >" + d + "/helper\nexit 0\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	old := Grace
	Grace = 200 * time.Millisecond
	defer func() { Grace = old }()
	out, err := Combined(exec.Command("git", "status"))
	if !errors.Is(err, ErrTimeout) || out != nil {
		t.Fatalf("출력 관이 안 닫히면 모름(ErrTimeout)·빈 출력: err=%v len=%d", err, len(out))
	}
	if h := waitFile(t, d+"/helper"); h > 1 {
		gone(h) // 스스로 끝날 때까지 기다린다(4초)
	}
	if n := len(bounded); n != 0 {
		t.Fatalf("끝난 무리가 bounded에 남았다: %d", n)
	}
}

// T02: 이 저장소의 .git에 기대지 않는다 — 임시 저장소에서
func TestRunFastUnchanged(t *testing.T) {
	t.Setenv("HERDR_TIMEOUT_GIT", "5")
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	out, err := Output(exec.Command("git", "--version"))
	if err != nil || !strings.HasPrefix(string(out), "git version") {
		t.Fatalf("빠른 git은 그대로: %q %v", out, err)
	}
	r := t.TempDir()
	if b, err := exec.Command("git", "init", "-q", r).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	if rc := Code(Run(exec.Command("git", "-C", r, "rev-parse", "--verify", "-q", "refs/heads/없는가지"))); rc != 1 {
		t.Errorf("종료 코드 그대로여야: %d", rc)
	}
}
