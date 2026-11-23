package main

// park-help-guard(2026-11-23): `hc-lane park --help`가 도움말 대신 실제로 주차했다(관문 nil + 인자 전부를 주차 글로).
// 도움말·옵션 꼴 글은 아무것도 하지 않고(원격 refs/park 없음·사건 없음) 끝나고, 정상 주차는 그대로

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParkHelpGuard(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	repo, bare, st := d+"/r", d+"/o.git", d+"/st"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+st, "HC_TERMINAL_ID=", "HC_LANE=pk", "HC_LANE_LEASE=",
		"HC_LANE_MACHINE=m", "HC_LANE_JOB=", "HC_LANE_OWNER=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(dir string, a ...string) string {
		c := exec.Command("git", a...)
		c.Dir, c.Env = dir, env
		b, err := c.Output()
		if err != nil {
			t.Fatalf("git %v: %v", a, err)
		}
		return strings.TrimSpace(string(b))
	}
	os.MkdirAll(repo, 0o755)
	git(d, "init", "-q", "--bare", bare)
	git(repo, "init", "-q", "-b", "pk", ".")
	git(repo, "commit", "-q", "--allow-empty", "-m", "a")
	git(repo, "remote", "add", "origin", bare)
	head := git(repo, "rev-parse", "HEAD")
	evf := st + "/hc-lane/events.tsv"
	run := func(a ...string) (int, string) {
		c := exec.Command(bin, append([]string{"park"}, a...)...)
		c.Dir, c.Env = repo, env
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
	parked := func() string { return git(d, "--git-dir", bare, "for-each-ref", "refs/park/") }
	events := func() string { b, _ := os.ReadFile(evf); return string(b) }

	for _, a := range [][]string{{"--help"}, {"-h"}, {"준비", "--help"}} {
		rc, e := run(a...)
		if rc != 2 || !strings.HasPrefix(e, "사용법: hc-lane ") || parked() != "" || events() != "" {
			t.Errorf("park %q: 종료 2·사용법·주차 없음·사건 없음이어야 — 종료 %d, 원격 %q, 사건 %q, 오류 %.60q", a, rc, parked(), events(), e)
		}
	}
	// 옵션 꼴(오타 난 옵션)은 주차 글로 읽지 않는다
	for _, a := range [][]string{{"--mesage", "준비"}, {"준비", "--resume"}} {
		rc, e := run(a...)
		if rc != 1 || !strings.Contains(e, "알 수 없는 인자") || parked() != "" || events() != "" {
			t.Errorf("park %q: 종료 1·알 수 없는 인자·주차 없음이어야 — 종료 %d, 원격 %q, 사건 %q, 오류 %.80q", a, rc, parked(), events(), e)
		}
	}
	// 정상 주차는 그대로
	if rc, e := run("준비", "끝"); rc != 0 || !strings.Contains(parked(), head+" commit\trefs/park/pk") ||
		!strings.Contains(events(), "주차") || !strings.Contains(events(), "· 준비 끝") {
		t.Fatalf("park 준비 끝: 종료 %d, 원격 %q, 사건 %q, 오류 %s", rc, parked(), events(), e)
	}
	// -로 시작하는 글은 -- 뒤에
	if rc, e := run("--", "-x", "글"); rc != 0 || !strings.Contains(events(), "· -x 글") {
		t.Errorf("park -- -x 글: 종료 %d, 사건 %q, 오류 %s", rc, events(), e)
	}
}
