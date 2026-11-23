package main

// preverify(2026-11-24): report 완료는 병합기·추측 검증 기계 목록이 켜진 저장소에서만 refs/merge-pre/<레인> → 표식 커밋(부모 = 그 sha)을
// 올린다. 꺼진 저장소·--name 사건·중간은 올리지 않고, 표식을 못 올려도 보고는 성공이다

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

func TestReportPreMark(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	origin, repo, st := d+"/o.git", d+"/r", d+"/st"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+st, "HC_TERMINAL_ID=", "HC_LANE=lp", "HC_LANE_MACHINE=m",
		"HC_LANE_JOB=", "HC_LANE_OWNER=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	git := func(dir string, a ...string) string {
		c := exec.Command("git", a...)
		c.Dir, c.Env = dir, env
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", a, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	git(d, "init", "-q", "--bare", "-b", "main", origin)
	git(d, "init", "-q", "-b", "main", repo)
	git(repo, "commit", "-q", "--allow-empty", "-m", "base")
	git(repo, "remote", "add", "origin", origin)
	git(repo, "push", "-q", "origin", "main")
	git(repo, "fetch", "-q", "origin")
	git(repo, "remote", "set-head", "origin", "main")
	git(repo, "checkout", "-q", "-b", "lp")
	git(repo, "commit", "-q", "--allow-empty", "-m", "work")
	head := git(repo, "rev-parse", "HEAD")
	run := func(a ...string) (int, string, string) {
		c := exec.Command(bin, append([]string{"report"}, a...)...)
		c.Dir, c.Env = repo, env
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
	mark := func() string {
		c := exec.Command("git", "--git-dir="+origin, "rev-parse", "-q", "--verify", verified.PreRefs+"lp")
		b, _ := c.Output()
		return strings.TrimSpace(string(b))
	}
	// 병합기 꺼짐 — 표식 없음
	if rc, _, e := run("완료", "HEAD", "끝"); rc != 0 || mark() != "" {
		t.Fatalf("병합기 꺼진 저장소: rc %d 표식 %q %s", rc, mark(), e)
	}
	// 병합기 켬, 검증 기계 목록 없음 — 표식 없음
	git(repo, "push", "-q", "origin", "main:refs/herdr/merger/box")
	if rc, _, _ := run("완료", "HEAD", "끝"); rc != 0 || mark() != "" {
		t.Fatalf("검증 기계 목록 없음: rc %d 표식 %q", rc, mark())
	}
	et := git(repo, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := git(repo, "commit-tree", et, "-m", verified.FormatVerifiers([]verified.Verifier{{Machine: "v1", Path: "/x"}}))
	git(repo, "push", "-q", "origin", vc+":"+verified.VerifiersRef)
	// 중간·--name 사건은 올리지 않는다
	if rc, _, _ := run("중간", "HEAD", "반"); rc != 0 || mark() != "" {
		t.Fatalf("중간: rc %d 표식 %q", rc, mark())
	}
	if rc, _, _ := run("완료", "HEAD", "남", "--name", "lp"); rc != 0 || mark() != "" {
		t.Fatalf("--name: rc %d 표식 %q", rc, mark())
	}
	// 켬 — 표식 커밋(부모 = 완료 sha, 트리 같음, 제목 PreMsg)
	rc, o, e := run("완료", "HEAD", "끝")
	m := mark()
	if rc != 0 || m == "" || !strings.Contains(o, verified.PreRefs+"lp") {
		t.Fatalf("켬: rc %d 표식 %q 출력 %q %s", rc, m, o, e)
	}
	p := git(d, "--git-dir="+origin, "log", "-1", "--format=%P|%T|%s", m)
	f := strings.SplitN(p, "|", 3)
	lane, base, at, ok := verified.ParsePre(f[2])
	if f[0] != head || f[1] != git(repo, "rev-parse", "HEAD^{tree}") || !ok || lane != "lp" || base != "main" || at <= 0 {
		t.Fatalf("표식 꼴: %q", p)
	}
	// 표식을 못 올려도 보고는 성공(사건은 이미 적음)
	b0, _ := os.ReadFile(st + "/hc-lane/events.tsv")
	git(repo, "remote", "set-url", "--push", "origin", d+"/없음.git")
	rc, _, e = run("완료", "HEAD", "다시")
	b1, _ := os.ReadFile(st + "/hc-lane/events.tsv")
	if rc != 0 || !strings.Contains(e, "미리 검증 표식") || strings.Count(string(b1), "\n") != strings.Count(string(b0), "\n")+1 {
		t.Fatalf("표식 실패에도 보고 성공: rc %d %q", rc, e)
	}
}
