//go:build !windows

package main

// 재감사 ops R-10·R-28: 실제 게시기가 pack|unpack 전송 중 TERM을 받으면, 두 git 무리(양쪽이 뜬 것을 확인한 뒤)를 다 거두고 자기 임시
// 폴더도 지운 뒤 143으로 끝난다(옛 판: 정상 종료 함수가 안 돌아 임시 폴더·블롭 사본이 남았다). SIGKILL로 남긴 임시 폴더는 다음 실행이
// owner(pid·시작 시각)로 끝난 실행임을 확인하고 지운다

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

func TestPublishTermReapsTransfer(t *testing.T) {
	pub := ct.Tool(t, "herdr-tools-publish", "HERDR_TOOLS_PUBLISH")
	T := ct.TempDir(t)
	ct.Isolate(t, T)
	for k, v := range map[string]string{"HOME": T + "/home", "HERDR_TOOLS_REPO": T + "/priv", "HERDR_TOOLS_CONFIG": T + "/conf",
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "priv", "GIT_AUTHOR_EMAIL": "priv-author@private.example",
		"GIT_COMMITTER_NAME": "priv", "GIT_COMMITTER_EMAIL": "priv-author@private.example", "TMPDIR": T + "/tmp",
		"HERDR_PROC_DIR": T + "/rec"} {
		t.Setenv(k, v)
	}
	for _, d := range []string{T + "/home", T + "/conf", T + "/tmp", T + "/fake"} {
		os.MkdirAll(d, 0o755)
	}
	put := func(p, s string) { ct.Put(t, p, s) }
	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/origin.git")
	ct.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/public.git")
	ct.MustGit(t, T, "clone", "-q", T+"/origin.git", T+"/priv")
	put(T+"/priv/docs/d.md", "doc\n")
	ct.MustGit(t, T+"/priv", "add", "-A")
	ct.MustGit(t, T+"/priv", "commit", "-qm", "first")
	ct.MustGit(t, T+"/priv", "push", "-q", "origin", "HEAD:main")
	ct.MustGit(t, T+"/priv", "fetch", "-q", "origin")
	put(T+"/conf/private-patterns.txt", "zzunusedzz\n")
	put(T+"/conf/config", "publish.repo    "+T+"/public.git\npublish.dir     ~/pubclone\npublish.author  Pub Bot <bot@public.example>\n")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	// 가짜 git: pack-objects는 TERM을 무시하고 멈춘다(KILL까지 가야 거둬진다), unpack-objects는 뜬 것을 적고 진짜 git으로
	put(T+"/fake/git", "#!/bin/sh\nfor a; do case $a in\n"+
		"  pack-objects) echo $$ >'"+T+"/pack.tmp'; mv '"+T+"/pack.tmp' '"+T+"/pack'; trap '' TERM; while :; do sleep 1; done ;;\n"+
		"  unpack-objects) echo $$ >'"+T+"/unpack.tmp'; mv '"+T+"/unpack.tmp' '"+T+"/unpack' ;;\n"+
		"esac; done\nexec '"+real+"' \"$@\"\n")
	ct.Chmod(t, T+"/fake/git", 0o755)
	// SIGKILL로 죽은 앞 실행이 남긴 임시 폴더(끝난 pid) — 이번 실행이 지운다. 도는 실행(이 시험)의 것은 둔다
	stale := T + "/tmp/herdr-tools-publish.stale1"
	live := T + "/tmp/herdr-tools-publish.live1"
	os.MkdirAll(stale+"/b", 0o700)
	os.MkdirAll(live, 0o700)
	put(stale+"/owner", "1999999\tMon Jan  1 00:00:00 2001\n")
	st, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	put(live+"/owner", strconv.Itoa(os.Getpid())+"\t"+st+"\n")

	c := exec.Command(pub)
	c.Env = append(os.Environ(), "PATH="+T+"/fake:"+os.Getenv("PATH"), "HERDR_TIMEOUT_GIT=120")
	var out strings.Builder
	c.Stdout, c.Stderr = &out, &out
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	pidOf := func(f string) int {
		for end := time.Now().Add(60 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			if b, err := os.ReadFile(f); err == nil {
				if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 1 {
					return p
				}
			}
		}
		c.Process.Kill()
		t.Fatalf("%s 가 뜨지 않았다\n%s", f, out.String())
		return 0
	}
	pk, up := pidOf(T+"/pack"), pidOf(T+"/unpack")
	defer syscall.Kill(pk, syscall.SIGKILL)
	defer syscall.Kill(up, syscall.SIGKILL)
	c.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err = <-done:
	case <-time.After(40 * time.Second):
		c.Process.Kill()
		t.Fatalf("TERM 뒤 끝나지 않았다\n%s", out.String())
	}
	if rc := proc.Code(err); rc != 143 {
		t.Fatalf("TERM이면 143: %d\n%s", rc, out.String())
	}
	gone := func(p int) bool {
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			if syscall.Kill(p, 0) == syscall.ESRCH {
				return true
			}
		}
		return false
	}
	if !gone(pk) || !gone(up) {
		t.Fatalf("pack(%d)·unpack(%d) 무리가 남았다", pk, up)
	}
	es, _ := os.ReadDir(T + "/tmp")
	var left []string
	for _, e := range es {
		left = append(left, e.Name())
	}
	if len(left) != 1 || left[0] != "herdr-tools-publish.live1" {
		t.Fatalf("임시 폴더: 이 실행 것·끝난 앞 실행 것은 지우고 도는 실행 것만 남아야: %v", left)
	}
}
