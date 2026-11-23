//go:build !windows

package main

// conflict-fast 감사(atoxxc3-naygg0) 낮음 1: 제출 때 충돌 확인에서 같은 patch-id 조회가 실패해도 merge-tree 모름과 같은 꼴로
// 「판정하지 못했다 — 판정 없이 제출한다」 한 줄을 찍는다(동작 — 그대로 제출 — 은 그대로)

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSubmitConflictCherryUnknownSays(t *testing.T) {
	d := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = d
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(d+"/f", []byte("base\n"), 0o666)
	run("add", "f")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "lane")
	os.WriteFile(d+"/f", []byte("lane\n"), 0o666)
	run("commit", "-qam", "lane")
	run("checkout", "-q", "main")
	os.WriteFile(d+"/f", []byte("main\n"), 0o666)
	run("commit", "-qam", "main")
	ob, sh := run("rev-parse", "main"), run("rev-parse", "lane")
	// rev-list --cherry-mark만 실패하는 git
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	os.WriteFile(bin+"/git", []byte("#!/bin/sh\ncase \" $* \" in *' --cherry-mark '*) exit 128;; esac\nexec "+real+" \"$@\"\n"), 0o777)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Chdir(d)
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	submitConflict(ob, sh) // 충돌이면 거절하며 끝내는 길(die)로 가지 않아야 한다 — 그대로 돌아오면 「제출」
	os.Stderr = old
	w.Close()
	b, _ := io.ReadAll(r)
	if !strings.Contains(string(b), "판정하지 못했다") || !strings.Contains(string(b), "판정 없이 제출한다") {
		t.Fatalf("같은 patch-id 조회 실패 줄이 없다: %q", b)
	}
}
