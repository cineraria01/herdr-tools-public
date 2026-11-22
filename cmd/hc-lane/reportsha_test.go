package main

// lane-report-sha(2026-11-22): report 완료·중간 <sha>는 이 작업 트리에서 전체 해시로 풀고, 못 풀면 종료 2(사건 안 씀),
// HEAD를 받아 풀고, 완료인데 HEAD가 아니면 경고만

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReportSHA(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	repo, st, out := d+"/r", d+"/st", d+"/out"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+st, "HC_TERMINAL_ID=", "HC_LANE=lr", "HC_LANE_MACHINE=m",
		"HC_LANE_JOB=", "HC_LANE_OWNER=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(a ...string) string {
		c := exec.Command("git", a...)
		c.Dir, c.Env = repo, env
		b, err := c.Output()
		if err != nil {
			t.Fatalf("git %v: %v", a, err)
		}
		return strings.TrimSpace(string(b))
	}
	os.MkdirAll(repo, 0o755)
	os.MkdirAll(out, 0o755)
	git("init", "-q", ".")
	git("commit", "-q", "--allow-empty", "-m", "a")
	first := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "b")
	head := git("rev-parse", "HEAD")
	evf := st + "/hc-lane/events.tsv"
	run := func(dir string, a ...string) (int, string, string) {
		c := exec.Command(bin, append([]string{"report"}, a...)...)
		c.Dir, c.Env = dir, env
		var o, e strings.Builder
		c.Stdout, c.Stderr = &o, &e
		err := c.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), o.String(), e.String()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, o.String(), e.String()
	}
	lastSHA := func() string {
		b, _ := os.ReadFile(evf)
		ls := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		f := strings.Split(ls[len(ls)-1], "\t")
		if len(f) < 6 {
			return ""
		}
		return f[5]
	}
	nlines := func() int {
		b, _ := os.ReadFile(evf)
		return strings.Count(string(b), "\n")
	}

	// 짧은 해시 풀기(중간은 HEAD가 아니어도 경고 없음)
	if rc, _, e := run(repo, "중간", first[:7], "앞 커밋"); rc != 0 || lastSHA() != first || e != "" {
		t.Fatalf("짧은 해시: rc %d sha %q 오류 %q", rc, lastSHA(), e)
	}
	// 뒤에 커밋 있음 — 완료는 경고 한 줄, 적기는 그대로
	if rc, _, e := run(repo, "완료", strings.ToUpper(first[:10]), "옛 커밋"); rc != 0 || lastSHA() != first ||
		!strings.Contains(e, "HEAD("+head[:12]+")가 아니다, 그 뒤 커밋 1개") {
		t.Fatalf("뒤 커밋 경고: rc %d sha %q 오류 %q", rc, lastSHA(), e)
	}
	// 없는 해시 거절 — 사건을 쓰지 않는다
	n := nlines()
	if rc, _, e := run(repo, "완료", "8b231930", "오타"); rc != 2 || nlines() != n ||
		e != "hc-lane report: 8b231930는 이 작업 트리에 없다 — HEAD는 "+head+"\n" {
		t.Fatalf("없는 해시: rc %d 줄 %d→%d 오류 %q", rc, n, nlines(), e)
	}
	// HEAD 받기
	if rc, o, e := run(repo, "완료", "HEAD", "끝"); rc != 0 || lastSHA() != head || e != "" || !strings.Contains(o, head) {
		t.Fatalf("HEAD: rc %d sha %q 출력 %q 오류 %q", rc, lastSHA(), o, e)
	}
	// 작업 트리 밖: 16진 sha는 옛 꼴 그대로(도구가 --name으로 적는 사건), HEAD는 거절
	if rc, _, _ := run(out, "완료", "abc1234", "밖", "--name", "x"); rc != 0 || lastSHA() != "abc1234" {
		t.Fatalf("밖 sha: rc %d sha %q", rc, lastSHA())
	}
	if rc, _, e := run(out, "완료", "HEAD", "밖"); rc != 2 || !strings.Contains(e, "HEAD를 풀 수 없다") {
		t.Fatalf("밖 HEAD: rc %d 오류 %q", rc, e)
	}
}
