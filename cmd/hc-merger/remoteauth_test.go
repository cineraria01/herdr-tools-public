//go:build !windows

package main

// ht-cordon-remote-auth(docs/hc-fill/cordon.md): 워커 쪽 원격 로그인 점검 한 줄(--spec-idle --remote-auth --repo-read …). 거절·모름은
// hc-fill이 가른다 — 여기서는 처음 있는 폴더·종료 코드·상한·호스트(자격 빼고)·오류 줄을 그대로 내는지만 본다

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func raGit(t *testing.T, a ...string) {
	t.Helper()
	if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", a, err, out)
	}
}

func TestRemoteAuthLine(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	d := t.TempDir()
	raGit(t, "init", "-q", "--bare", d+"/up.git")
	raGit(t, "init", "-q", d+"/w")
	raGit(t, "-C", d+"/w", "remote", "add", "origin", "file://"+d+"/up.git")
	// 처음 있는 폴더(2번째)에서 — 빈 원격은 --exit-code가 2(자격은 통과): 종료 그대로 싣는다
	if got := remoteAuth([]string{d + "/gone", d + "/w"}); got != "remote-auth 2 2 - -" {
		t.Fatalf("빈 원격: %q", got)
	}
	if got := remoteAuth([]string{d + "/gone"}); got != "remote-auth 0 none - -" {
		t.Fatalf("폴더 없음: %q", got)
	}
	// 원격이 응답하지 않음 → 상한 안에 timeout(모름 재료)
	raGit(t, "-C", d+"/w", "remote", "set-url", "origin", "ssh://git.example.test/o/r.git")
	t.Setenv("GIT_SSH_COMMAND", "sleep 30;")
	old := remoteAuthMax
	remoteAuthMax = 300 * time.Millisecond
	t.Cleanup(func() { remoteAuthMax = old })
	t0 := time.Now()
	if got := remoteAuth([]string{d + "/w"}); !strings.HasPrefix(got, "remote-auth 1 timeout git.example.test ") || time.Since(t0) > 5*time.Second {
		t.Fatalf("안 끝남: %q %s", got, time.Since(t0))
	}
	// 거절: 오류 줄들을 한 줄로(빈 줄 빼고)
	t.Setenv("GIT_SSH_COMMAND", "echo 'git@git.example.test: Permission denied (publickey).' >&2; exit 255;")
	got := remoteAuth([]string{d + "/w"})
	if !strings.HasPrefix(got, "remote-auth 1 128 git.example.test git@git.example.test: Permission denied (publickey). | fatal: Could not read from remote repository.") ||
		strings.Contains(got, "\n") {
		t.Fatalf("거절 글: %q", got)
	}
}

// 호스트만 — 주소 안의 자격은 싣지 않는다
func TestRemoteHost(t *testing.T) {
	for u, want := range map[string]string{
		"https://x-access-token:ghs_secret@git.example.test/o/r.git": "git.example.test",
		"https://git.example.test:8443/o/r":                          "git.example.test",
		"ssh://git@git.example.test:22/o/r.git":                      "git.example.test",
		"git@git.example.test:o/r.git":                               "git.example.test",
		"/srv/r.git":                                                 "-",
		"":                                                           "-",
	} {
		if got := remoteHost(u); got != want {
			t.Errorf("%q → %q, 바라는 것 %q", u, got, want)
		}
	}
}

// --remote-auth는 --repo-read 앞에만 — 혼자거나 다른 자리면 거절(종료 2, 옛 판과 같은 꼴 — hc-fill이 다음 꼴로 다시 부른다)
func TestSpecIdleRemoteAuthArgs(t *testing.T) {
	old := idleProbe
	idleProbe = func() (idleInfo, bool) { return idleInfo{load: 0.5, cpus: 8, gatesKnown: true}, true }
	t.Cleanup(func() { idleProbe = old })
	for _, a := range [][]string{{"--remote-auth"}, {"--remote-auth", "x"}} {
		if rc := specIdleMain(a); rc != 2 {
			t.Fatalf("%v: %d", a, rc)
		}
	}
	if rc := specIdleMain([]string{"--remote-auth", "--repo-read", t.TempDir() + "/gone"}); rc != 0 {
		t.Fatalf("점검 둘: %d", rc)
	}
}
