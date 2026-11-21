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
	if !errors.As(err, &te) || out != nil {
		t.Fatalf("TimeoutError·빈 출력이어야: err=%v out=%q", err, out)
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

func TestRunFastUnchanged(t *testing.T) {
	t.Setenv("HERDR_TIMEOUT_GIT", "5")
	out, err := Output(exec.Command("git", "--version"))
	if err != nil || !strings.HasPrefix(string(out), "git version") {
		t.Fatalf("빠른 git은 그대로: %q %v", out, err)
	}
	if rc := Code(Run(exec.Command("git", "rev-parse", "--verify", "-q", "refs/heads/없는가지-"+strconv.Itoa(os.Getpid())))); rc != 1 {
		t.Errorf("종료 코드 그대로여야: %d", rc)
	}
}

func TestWaitMax(t *testing.T) {
	c := exec.Command("sh", "-c", "sleep 60")
	t0 := time.Now()
	rc := WaitMax(c, "", 500*time.Millisecond)
	if el := time.Since(t0); el > 8*time.Second || rc != 128+15 {
		t.Fatalf("상한에 TERM으로 끝나야: rc=%d %v", rc, el)
	}
}
