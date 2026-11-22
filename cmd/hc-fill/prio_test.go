package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// prioMeta: 이미 낸 레인의 큐 메타 제목 priority=를 바꾼다(트리·부모·날짜·나머지 글 그대로), 다른 레인(이름 꼬리가 같은 것)은 안 건드림,
// 큐에 없으면 1, lease 실패면 그대로 두고 2
func TestPrioMeta(t *testing.T) {
	setData(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git 없음")
	}
	git := func(dir string, a ...string) string {
		t.Helper()
		c := exec.Command(realGit, a...)
		c.Dir = dir
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "t",
		"GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
		"GIT_AUTHOR_DATE": "1700000000 +0900", "GIT_COMMITTER_DATE": "1700000000 +0900"} {
		t.Setenv(k, v)
	}
	o, w := DATA+"/o.git", DATA+"/w"
	git(DATA, "init", "-q", "--bare", "-b", "main", o)
	git(DATA, "init", "-q", "-b", "main", w)
	git(w, "commit", "-q", "--allow-empty", "-m", "base")
	git(w, "remote", "add", "origin", o)
	et := git(w, "hash-object", "-t", "tree", "-w", "/dev/null")
	mk := func(qn, lane string) string {
		mc := git(w, "commit-tree", et, "-p", "HEAD", "-m", "merge-queue lane="+lane+" id=x-1 created_ms=5 priority=보통\n\n둘째 줄")
		git(w, "push", "-q", "origin", "HEAD:refs/merge-queue/"+qn, mc+":refs/merge-queue-meta/"+qn)
		return mc
	}
	m1, m2 := mk("100-l-a", "l-a"), mk("200-x-l-a", "x-l-a")
	confLines = lines("local here\nrepo app here " + w + "\n")
	LOCAL = "here"
	t.Setenv("GIT_COMMITTER_DATE", "1800000000 +0000") // 다시 만들 때 날짜가 바뀌면 안 된다

	if rc := prioMeta("l-a", "app", "긴급"); rc != 0 {
		t.Fatalf("prioMeta rc %d", rc)
	}
	n1 := git(w, "ls-remote", "origin", "refs/merge-queue-meta/100-l-a")
	n1 = strings.Fields(n1)[0]
	git(w, "fetch", "-q", "origin", "refs/merge-queue-meta/100-l-a")
	if got := git(w, "log", "-1", "--format=%s", n1); got != "merge-queue lane=l-a id=x-1 created_ms=5 priority=긴급" {
		t.Fatalf("제목: %q", got)
	}
	for _, f := range []string{"%T %P %an %ae %ad %cn %ce %cd", "%b"} {
		if a, b := git(w, "log", "-1", "--format="+f, m1), git(w, "log", "-1", "--format="+f, n1); a != b {
			t.Errorf("%s 바뀜: %q → %q", f, a, b)
		}
	}
	if s := strings.Fields(git(w, "ls-remote", "origin", "refs/merge-queue-meta/200-x-l-a"))[0]; s != m2 {
		t.Errorf("다른 레인(x-l-a) 메타를 건드림")
	}
	if rc := prioMeta("l-a", "app", "긴급"); rc != 0 { // 같은 값이면 그대로
		t.Fatalf("같은 값 rc %d", rc)
	}
	if rc := prioMeta("l-b", "app", "긴급"); rc != 1 {
		t.Fatalf("큐에 없음 rc %d", rc)
	}
	// lease 실패: push 직전에 누가 그 메타를 바꿨다 — 그대로 두고 2
	fake := DATA + "/bin"
	os.MkdirAll(fake, 0o755)
	os.WriteFile(fake+"/git", []byte("#!/bin/sh\ncase \" $* \" in *\" push \"*) \""+realGit+"\" -C "+o+" update-ref refs/merge-queue-meta/100-l-a "+m1+" ;; esac\nexec \""+realGit+"\" \"$@\"\n"), 0o755)
	t.Setenv("PATH", fake+":"+os.Getenv("PATH"))
	if rc := prioMeta("l-a", "app", "낮음"); rc != 2 {
		t.Fatalf("lease 실패 rc %d", rc)
	}
	if s := strings.Fields(git(w, "ls-remote", "origin", "refs/merge-queue-meta/100-l-a"))[0]; s != m1 {
		t.Fatalf("lease 실패인데 바뀜: %s", s)
	}
}
