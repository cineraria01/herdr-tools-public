package util_test

// 2026-11-21 Go 전환 전체 감사(go-final-audit base) B28: 제출 전 사전 검사(.claude/herdr-precheck.sh → herdr-verify.sh --cheap-only)의
// 싼 검사 목록(CHEAP) 파일이 레인에서 사라지면 조용히 빠졌다. 기준 브랜치(origin/main)에 있던 싼 검사 파일이 없으면 이름을 찍고 실패.
// 원격이 없는 가짜 저장소(tests/herdr-verify.test.sh)는 그대로 — 없는 파일은 건너뜀.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHerdrVerifyCheapMissing(t *testing.T) {
	T := tmpdir(t)
	e := isolated(T).With("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+T+"/home", "HERDR_TOOLS_CONFIG="+T+"/conf")
	write(t, T+"/conf/private-patterns.txt", "secret-word\n")
	R := T + "/r"
	mkdirs(t, R+"/.claude", R+"/bin", R+"/tests")
	for _, p := range []string{".claude/herdr-verify.sh", ".claude/herdr-precheck.sh", "bin/herdr-tools-config"} {
		b, err := os.ReadFile(filepath.Join(repo, p))
		if err != nil {
			t.Fatal(err)
		}
		write(t, R+"/"+p, string(b))
	}
	for _, n := range []string{"line-budget", "publish-secrets"} {
		script(t, R+"/tests/"+n+".test.sh", T, "#!/bin/bash\nexit 0\n")
	}
	bash(t, T, e, `R=$1 T=$2
git init -q --bare -b main "$T/origin.git" && git -C "$R" init -q -b main && git -C "$R" add -- .claude bin tests &&
  git -C "$R" commit -qm base && git -C "$R" remote add origin "$T/origin.git" && git -C "$R" push -q origin HEAD:main && git -C "$R" fetch -q origin`, R, T)
	pre := func() (string, int) { return run(R, e, "/bin/bash", R+"/.claude/herdr-precheck.sh") }
	out, rc := pre()
	if rc != 0 || !strings.Contains(out, "통과 tests/publish-secrets.test.sh") {
		t.Fatalf("준비 확인: 두 싼 검사가 있으면 통과여야 함 rc=%d\n%s", rc, out)
	}
	bash(t, T, e, `git -C "$1" rm -q -- tests/publish-secrets.test.sh && git -C "$1" commit -qm drop`, R)
	out, rc = pre()
	if rc == 0 || !strings.Contains(out, "tests/publish-secrets.test.sh") {
		t.Errorf("FAIL - [B28] 기준 브랜치에 있던 싼 검사 파일이 없으면 이름을 찍고 실패 rc=%d\n%s", rc, out)
	}
}
