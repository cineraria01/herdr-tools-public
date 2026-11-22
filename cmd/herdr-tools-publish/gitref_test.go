package main

// gofix3-gitref(재감사 아스트라 2바퀴): ops/R-220 공개 쪽 ls-remote 응답 판독 · ops/R-213 소유 기록 뒤에만 블롭

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

// ops/R-213: 시작 시각 조회가 실패하면 소유 기록 없는 임시 폴더를 남기지 않고 오류(게시를 시작하지 않는다)
func TestGitrefOwnedTmp(t *testing.T) {
	d := t.TempDir()
	t.Setenv("TMPDIR", d)
	old := startTime
	t.Cleanup(func() { startTime = old })
	startTime = func(int) (string, error) { return "", errors.New("ps 실패") }
	if p, err := mkOwnedTmp(); err == nil {
		t.Fatalf("소유 기록을 못 만들었는데 임시 폴더를 돌려줬다: %s", p)
	}
	if es, _ := os.ReadDir(d); len(es) != 0 {
		t.Fatalf("빈 자기 임시 폴더는 지워야: %v", es)
	}
	startTime = old
	p, err := mkOwnedTmp()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p + "/owner"); err != nil || !strings.HasPrefix(string(b), strconv.Itoa(os.Getpid())+"\t") {
		t.Fatalf("owner: %q %v", b, err)
	}
}

// ops/R-220: 공개 main 조회가 SHA 앞 12자만 내고 0으로 끝나면 모름 — 「같음」·결과 파일을 쓰지 않는다
func TestGitrefPublicLsRemoteTrunc(t *testing.T) {
	pub := ct.Bin(t, "herdr-tools-publish")
	T := ct.TempDir(t)
	ct.Isolate(t, T)
	for k, v := range map[string]string{"HOME": T + "/home", "HERDR_TOOLS_REPO": T + "/priv", "HERDR_TOOLS_CONFIG": T + "/conf",
		"GIT_CONFIG_NOSYSTEM": "1", "TMPDIR": T + "/tmp"} {
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
	ct.MustGit(t, T+"/priv", "-c", "user.name=p", "-c", "user.email=p@p.example", "commit", "-qm", "first")
	ct.MustGit(t, T+"/priv", "push", "-q", "origin", "HEAD:main")
	put(T+"/conf/private-patterns.txt", "zzunusedzz\n")
	put(T+"/conf/config", "publish.repo    "+T+"/public.git\npublish.dir     ~/pubclone\npublish.author  Pub Bot <bot@public.example>\n")
	res := T + "/res"
	r := ct.Cmd{Env: []string{"HERDR_TOOLS_PUBLISH_RESULT=" + res}, Combined: true}.Run(pub)
	if r.RC != 0 || !strings.Contains(r.Out, "내보냄") {
		t.Fatalf("첫 게시(공개 main 없음 — 종료 2): %d\n%s", r.RC, r.Out)
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	head := ct.Git(T, "--git-dir="+T+"/public.git", "rev-parse", "main")
	put(T+"/fake/git", "#!/bin/sh\nfor a; do [ \"$a\" = ls-remote ] && { echo "+head[:12]+"; exit 0; }; done\nexec '"+real+"' \"$@\"\n")
	ct.Chmod(t, T+"/fake/git", 0o755)
	os.Remove(res)
	r = ct.Cmd{Env: []string{"HERDR_TOOLS_PUBLISH_RESULT=" + res, "PATH=" + T + "/fake:" + os.Getenv("PATH")}, Combined: true}.Run(pub)
	if r.RC == 0 || strings.Contains(r.Out, "같음") {
		t.Fatalf("잘린 ls-remote 응답을 받았다: %d\n%s", r.RC, r.Out)
	}
	if _, err := os.Stat(res); !os.IsNotExist(err) {
		t.Fatalf("결과 파일을 쓰면 안 된다: %v", err)
	}
	if got := ct.Git(T, "--git-dir="+T+"/public.git", "rev-parse", "main"); got != head {
		t.Fatalf("공개 main이 바뀌었다: %s", got)
	}
}
