package main

// preverify-wait-cap: hc-lane clean은 정리한 레인의 미리 검증 표식(refs/merge-pre/<레인>)을 본 값 lease로 지운다 — 남의 표식은 그대로,
// 표식이 없거나 지우기에 실패해도 정리는 성공

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

func TestCleanDropsPreMark(t *testing.T) {
	d := t.TempDir()
	bin := buildLane(t, d)
	origin, repo := d+"/o.git", d+"/r"
	env := append(os.Environ(), "HOME="+d, "XDG_STATE_HOME="+d+"/st", "HC_TERMINAL_ID=", "HC_LANE=", "LANE_BRANCH=", "HC_LANE_MACHINE=m",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
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
	git(repo, "branch", "lp") // 착지한 레인 가지(기준에 들어감)
	head := git(repo, "rev-parse", "HEAD")
	for _, l := range []string{"lp", "other"} {
		c := git(repo, "commit-tree", head+"^{tree}", "-p", head, "-m", verified.PreMsg(l, "main", 1))
		git(repo, "push", "-q", "origin", c+":"+verified.PreRefs+l)
	}
	has := func(l string) bool {
		return exec.Command("git", "--git-dir="+origin, "rev-parse", "-q", "--verify", verified.PreRefs+l).Run() == nil
	}
	c := exec.Command(bin, "clean", "--repo", repo, "--branch", "lp")
	c.Dir, c.Env = repo, env
	o, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("clean: %v %s", err, o)
	}
	if has("lp") || !has("other") || !strings.Contains(string(o), "미리 검증 표식 지움: "+verified.PreRefs+"lp") {
		t.Fatalf("정리한 레인의 표식만 지운다: lp %v other %v\n%s", has("lp"), has("other"), o)
	}
}
