//go:build !windows

package main

// ht-probe-cordon(docs/hc-fill/cordon.md 「점검 항목」): 워커 쪽 점검 줄 — 쓰기 자격(--dry-run, 원격에 참조가 안 생김)·키체인·임시 폴더·풀

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func prRepo(t *testing.T) (d string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	d = t.TempDir()
	raGit(t, "init", "-q", "--bare", d+"/up.git")
	raGit(t, "init", "-q", d+"/w")
	raGit(t, "-C", d+"/w", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	raGit(t, "-C", d+"/w", "remote", "add", "origin", "file://"+d+"/up.git")
	return d
}

// 쓰기 자격: 받아 주는 원격은 종료 0(좋음 재료) — --dry-run이라 원격에 참조가 생기지 않는다. 거절 글은 그대로 싣는다
func TestRemoteWriteLine(t *testing.T) {
	d := prRepo(t)
	ls := remoteAuth([]string{d + "/w"})
	if len(ls) != 2 || ls[1] != "remote-write 1 0 - -" {
		t.Fatalf("받아 주는 원격: %q", ls)
	}
	if out, err := exec.Command("git", "-C", d+"/up.git", "for-each-ref").CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("원격에 참조가 생겼다: %v %q", err, out)
	}
	raGit(t, "-C", d+"/w", "remote", "set-url", "origin", "ssh://git.example.test/o/r.git")
	t.Setenv("GIT_SSH_COMMAND", "echo 'ERROR: Permission to o/r.git denied to someone.' >&2; exit 1;")
	if w := remoteAuth([]string{d + "/w"})[1]; !strings.HasPrefix(w, "remote-write 1 128 git.example.test ERROR: Permission to o/r.git denied to someone. | fatal: Could not read from remote repository.") {
		t.Fatalf("거절 글: %q", w)
	}
}

func prFake(t *testing.T, body string) string {
	t.Helper()
	p := t.TempDir() + "/pool"
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProbePool(t *testing.T) {
	old := probeMax
	probeMax = 500 * time.Millisecond
	t.Cleanup(func() { probeMax = old })
	for body, want := range map[string]string{
		`[ "$1" = status ] && printf 'Server: running\nUsable now:     2/6 accounts\n  account acct-secret-name\n'`: "probe pool ok 쓸 계정 2",
		`printf 'Usable now:     0/6 accounts\n  account acct-secret-name\n'; exit 1`:                               "probe pool bad 쓸 계정 0",
		`echo 'Server: stopped'`:       "probe pool unknown 쓸 계정 수를 못 읽음(0)",
		`sleep 5`:                      "probe pool unknown 500ms 안에 안 끝남",
		`echo 'Usable now: x'; exit 3`: "probe pool unknown 쓸 계정 수를 못 읽음(exit status 3)",
	} {
		if got := probePool(prFake(t, body)); got != want {
			t.Errorf("%s → %q, 바라는 것 %q", body, got, want)
		}
	}
	if got := probePool(t.TempDir() + "/none"); got != "probe pool unknown 명령 없음" && !strings.HasPrefix(got, "probe pool unknown") {
		t.Errorf("명령 없음: %q", got)
	}
	if got := probePool("no-such-pool-cmd-x"); got != "probe pool unknown 명령 없음" {
		t.Errorf("PATH에 없음: %q", got)
	}
}

func TestProbeKeychain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if got := probeKeychain(); got != "probe keychain na macOS 아님" {
			t.Fatalf("해당 없음: %q", got)
		}
		return
	}
	old, om := keychainArgv, probeMax
	t.Cleanup(func() { keychainArgv, probeMax = old, om })
	probeMax = 500 * time.Millisecond
	for rc, want := range map[string]string{"0": "probe keychain ok 열림", "36": "probe keychain bad rc 36 User interaction is not allowed",
		"51": "probe keychain unknown exit status 51"} {
		keychainArgv = func() []string { return []string{"/bin/sh", "-c", "exit " + rc} }
		if got := probeKeychain(); got != want {
			t.Errorf("종료 %s → %q", rc, got)
		}
	}
	keychainArgv = func() []string { return []string{"/bin/sleep", "5"} }
	if got := probeKeychain(); got != "probe keychain unknown 500ms 안에 안 끝남" {
		t.Errorf("안 끝남은 모름: %q", got)
	}
}

func TestProbeTmp(t *testing.T) {
	oldR, oldT, om := tmpRoot, tmpTryFn, probeMax
	t.Cleanup(func() { tmpRoot, tmpTryFn, probeMax = oldR, oldT, om })
	d := t.TempDir()
	tmpRoot = func() (string, bool) { return d + "/root", true }
	if got := probeTmp(); got != "probe tmp ok "+d+"/root" {
		t.Fatalf("좋음: %q", got)
	}
	if es, _ := os.ReadDir(d + "/root"); len(es) != 0 {
		t.Fatalf("만든 파일을 지우지 않음: %v", es)
	}
	if err := os.WriteFile(d+"/file", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tmpRoot = func() (string, bool) { return d + "/file/root", true }
	if got := probeTmp(); !strings.HasPrefix(got, "probe tmp bad ") {
		t.Fatalf("쓰기 오류는 나쁨: %q", got)
	}
	tmpRoot = func() (string, bool) { return "off", true }
	if got := probeTmp(); got != "probe tmp na 작업 임시 폴더 끔" {
		t.Fatalf("끔: %q", got)
	}
	tmpRoot = func() (string, bool) { return "", false }
	if !strings.HasPrefix(probeTmp(), "probe tmp unknown ") {
		t.Fatalf("설정 못 읽음은 모름")
	}
	probeMax = 200 * time.Millisecond
	tmpRoot = func() (string, bool) { return d + "/root", true }
	tmpTryFn = func(string) string { time.Sleep(3 * time.Second); return "ok" }
	if got := probeTmp(); got != "probe tmp bad "+d+"/root 쓰기가 200ms 안에 안 끝남" {
		t.Fatalf("안 끝남은 나쁨: %q", got)
	}
}

// --spec-idle --remote-auth --repo-read: 한가함 → 점검 줄(키체인·임시·풀) → 원격 읽기·쓰기 → 읽기 줄들 → 끝 줄
func TestSpecIdleProbeOrder(t *testing.T) {
	old, oldR := idleProbe, tmpRoot
	idleProbe = func() (idleInfo, bool) { return idleInfo{load: 0.5, cpus: 8, gatesKnown: true}, true }
	d := t.TempDir()
	tmpRoot = func() (string, bool) { return d + "/tmp", true }
	t.Cleanup(func() { idleProbe, tmpRoot = old, oldR })
	t.Setenv("HC_LANE_PROBE_POOL", prFake(t, `echo 'Usable now: 1/1 accounts'`))
	r, w, _ := os.Pipe()
	so := os.Stdout
	os.Stdout = w
	rc := specIdleMain([]string{"--remote-auth", "--repo-read", d + "/gone"})
	os.Stdout = so
	w.Close()
	b, _ := io.ReadAll(r)
	var heads []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		f := strings.Fields(l)
		heads = append(heads, f[0]+map[bool]string{true: " " + f[1]}[f[0] == "probe"])
	}
	if got := strings.Join(heads, ","); rc != 0 || got != "load=0.50,probe keychain,probe tmp,probe upstream,probe pool,remote-auth,remote-write,repo-read,repo-read" {
		t.Fatalf("차례: %d %s\n%s", rc, got, b)
	}
}
