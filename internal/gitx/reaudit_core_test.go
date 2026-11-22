package gitx

// gofix2(Go 전환 재감사 아스트라 1바퀴) — 고치기 전 코드에서 실패하던 시험

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// R-22: patch-id 계산기가 종료 0에 잘린 지문(세 글자)이나 여러 줄을 돌려주면 모름 — 승인 지문·큐 ref로 쓰이지 않게
func TestPatchIDRejectsTruncated(t *testing.T) {
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git 없음")
	}
	d := t.TempDir()
	for k, v := range map[string]string{"HOME": d, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	g := func(args ...string) string {
		c := exec.Command(real, args...)
		c.Dir = d
		o, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	g("init", "-q")
	if err := os.WriteFile(filepath.Join(d, "f"), []byte("a\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	g("add", "f")
	g("commit", "-qm", "a")
	a := g("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(d, "f"), []byte("b\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	g("commit", "-qam", "b")
	b := g("rev-parse", "HEAD")
	t.Chdir(d)
	if p, rc := PatchID(nil, a, b); rc != 0 || len(p) != 40 {
		t.Fatalf("진짜 git: 40자 patch-id와 0이어야: %q %d", p, rc)
	}
	fakeGit(t, `[ "$1" = patch-id ] && { cat >/dev/null; echo "abc 0"; exit 0; }; exec "`+real+`" "$@"`)
	if p, rc := PatchID(nil, a, b); rc == 0 {
		t.Fatalf("잘린 patch-id(abc)는 모름(0 아님)이어야: %q %d", p, rc)
	}
	x, y := strings.Repeat("a", 40), strings.Repeat("b", 40)
	fakeGit(t, `[ "$1" = patch-id ] && { cat >/dev/null; printf '%s %s\n%s %s\n' `+x+` `+y+` `+y+` `+x+`; exit 0; }; exec "`+real+`" "$@"`)
	if p, rc := PatchID(nil, a, b); rc == 0 {
		t.Fatalf("patch-id 결과가 두 줄이면 모름이어야: %q %d", p, rc)
	}
}
