//go:build !windows

package main

// 통과 표식(mark.go, submit-fast): ① 전체 검증 통과 → 표식 = HEAD^{tree} ③ --cheap-only만 → 표식 없음 ④ 검증 실패 → 표식 없음
// ⑤ 검증 중 작업 트리가 바뀜(추적 안 된 파일·추적 파일 변경) → 표식 없음, 시작부터 더러움 → 표식 없음. 가짜 go(PATH 앞)로 돈다

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVerifiedTreeMark(t *testing.T) {
	B := t.TempDir()
	// 가짜 go: 조각(herdr-tools/a)에서 FAKE_TOUCH가 있으면 그 파일에 한 줄 더하고, FAKE_RC로 끝난다
	fake := "#!/bin/sh\ncase \"$*\" in\n  'list ./...') printf 'herdr-tools/a\\n' ;;\n" +
		"  *' herdr-tools/a') [ -z \"$FAKE_TOUCH\" ] || echo x >>\"$FAKE_TOUCH\"; exit \"${FAKE_RC:-0}\" ;;\nesac\nexit 0\n"
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "")
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("FAKE_TOUCH", "")
	t.Setenv("FAKE_RC", "")

	repo := func(t *testing.T) (string, func(...string) string) {
		R := tmp(t)
		git := func(a ...string) string {
			t.Helper()
			c := exec.Command("git", a...)
			c.Dir = R
			c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
			return strings.TrimSpace(string(out))
		}
		git("init", "-q")
		if err := os.WriteFile(R+"/f", []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "f")
		git("commit", "-q", "-m", "c")
		return R, git
	}
	run := func(t *testing.T, R string, args ...string) (int, string, string) {
		t.Helper()
		f, read := outFile(t)
		rc := verify(R, args, make(chan os.Signal), f)
		m, _ := os.ReadFile(R + "/.git/" + markName)
		return rc, read(), string(m)
	}

	t.Run("① 전체 통과 — 표식 = HEAD^{tree}", func(t *testing.T) {
		R, git := repo(t)
		rc, out, m := run(t, R)
		f := strings.Fields(m)
		if rc != 0 || len(f) != 2 || f[0] != git("rev-parse", "HEAD^{tree}") || !strings.Contains(out, "통과 표식") {
			t.Fatalf("rc=%d 표식=%q\n%s", rc, m, out)
		}
	})
	t.Run("③ --cheap-only만 — 표식 없음", func(t *testing.T) {
		R, _ := repo(t)
		if rc, out, m := run(t, R, "--cheap-only"); rc != 0 || m != "" {
			t.Fatalf("rc=%d 표식=%q\n%s", rc, m, out)
		}
	})
	t.Run("④ 검증 실패 — 표식 없음", func(t *testing.T) {
		R, _ := repo(t)
		t.Setenv("FAKE_RC", "1")
		if rc, out, m := run(t, R); rc != 1 || m != "" {
			t.Fatalf("rc=%d 표식=%q\n%s", rc, m, out)
		}
	})
	t.Run("⑤ 검증 중 바뀜 — 추적 안 된 파일·추적 파일 변경 모두 표식 없음", func(t *testing.T) {
		for _, name := range []string{"new", "f"} {
			R, _ := repo(t)
			t.Setenv("FAKE_TOUCH", R+"/"+name)
			if rc, out, m := run(t, R); rc != 0 || m != "" {
				t.Fatalf("%s: rc=%d 표식=%q\n%s", name, rc, m, out)
			}
		}
	})
	t.Run("시작부터 더러운 작업 트리 — 표식 없음", func(t *testing.T) {
		R, _ := repo(t)
		if err := os.WriteFile(R+"/f", []byte("b\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if rc, out, m := run(t, R); rc != 0 || m != "" {
			t.Fatalf("rc=%d 표식=%q\n%s", rc, m, out)
		}
	})
}
