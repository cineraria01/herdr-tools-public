//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
)

// pull 잠금 경합(2026-11-24 지휘 알림): 가짜 git이 정한 횟수만큼 잠금 오류로 실패한 뒤 된다 — 잠금은 다시 해서 되고, 진짜 갈라짐은
// 바로 git 까닭 한 줄로 끝난다(다시 하지 않음), 잠금이 끝까지 풀리지 않으면 상한 뒤 까닭 한 줄
func TestPullFFRetriesLockOnly(t *testing.T) {
	d := t.TempDir()
	n := d + "/n"
	fake := "#!/bin/sh\nc=$(cat " + n + " 2>/dev/null || echo 0); c=$((c+1)); echo $c > " + n + "\n" +
		"case $FAKE_MODE in\n" +
		"  lock) [ $c -ge 3 ] && exit 0; echo \"fatal: Unable to create '/r/.git/index.lock': File exists.\" >&2; exit 128 ;;\n" +
		"  lockforever) echo \"error: cannot lock ref 'refs/remotes/origin/main': is at x but expected y\" >&2; exit 1 ;;\n" +
		"  diverge) echo 'hint: x' >&2; echo 'fatal: Not possible to fast-forward, aborting.' >&2; exit 128 ;;\n" +
		"esac\n"
	if err := os.WriteFile(d+"/git", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	old := pullWait
	pullWait = 0
	defer func() { pullWait = old }()
	count := func() string { b, _ := os.ReadFile(n); return strings.TrimSpace(string(b)) }

	t.Setenv("FAKE_MODE", "lock")
	if why := pullFF("/r"); why != "" || count() != "3" {
		t.Fatalf("잠금 두 번 뒤 됨: why=%q 부른 수 %s", why, count())
	}
	os.Remove(n)
	t.Setenv("FAKE_MODE", "diverge")
	if why := pullFF("/r"); why != "fatal: Not possible to fast-forward, aborting." || count() != "1" {
		t.Fatalf("갈라짐은 바로 끝: why=%q 부른 수 %s", why, count())
	}
	os.Remove(n)
	t.Setenv("FAKE_MODE", "lockforever")
	if why := pullFF("/r"); !strings.HasPrefix(why, "error: cannot lock ref") || count() != "5" {
		t.Fatalf("잠금이 안 풀리면 상한 뒤 까닭: why=%q 부른 수 %s", why, count())
	}
}
