package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func arGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
}

// 감사 기계(HOME a)가 판정을 적을 때 보고를 원격에 올리고, 보고가 없는 다른 기계(HOME b)가 아이디만으로 읽는다
func TestAuditReportReach(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.invalid")
	bare, a, b := filepath.Join(root, "o.git"), filepath.Join(root, "a"), filepath.Join(root, "b")
	arGit(t, root, "init", "-q", "--bare", bare)
	arGit(t, root, "clone", "-q", bare, a)
	arGit(t, root, "clone", "-q", bare, b)

	ha, hb := filepath.Join(root, "ha"), filepath.Join(root, "hb")
	t.Setenv("HOME", ha)
	rep := auditReportPath("atx1-ab")
	if err := os.MkdirAll(filepath.Dir(rep), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# 감사\n- [낮음] x.go:1 무엇\n"
	if err := os.WriteFile(rep, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(a)
	auditPublish("감사 완료 atx1-ab 높음 0·중간 0·낮음 1")

	// 다른 기계 흉내: HOME b에는 보고가 없다 → 원격에서 가져와 로컬에 남긴다
	t.Setenv("HOME", hb)
	if rc := auditReportMain([]string{"atx1-ab", "--repo", b}); rc != 0 {
		t.Fatalf("audit-report 종료 %d, 0이어야", rc)
	}
	if got, _ := os.ReadFile(auditReportPath("atx1-ab")); string(got) != body {
		t.Fatalf("가져온 보고가 다르다: %q", got)
	}
	// 파일 이름 꼴(audit-<ID>.md)도 받는다 — 이번엔 로컬에서
	if rc := auditReportMain([]string{"~/.local/share/hc-fill/reports/audit-atx1-ab.md"}); rc != 0 {
		t.Fatalf("파일 이름 꼴 종료 %d", rc)
	}

	// 없는 아이디 → 종료 1과 알기 쉬운 글
	ef := filepath.Join(root, "err")
	f, err := os.Create(ef)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = f
	rc := auditReportMain([]string{"atx9-none", "--repo", b})
	os.Stderr = old
	f.Close()
	msg, _ := os.ReadFile(ef)
	if rc != 1 || !strings.Contains(string(msg), "감사 보고 atx9-none 가 없다") {
		t.Fatalf("없는 아이디: 종료 %d, 글 %q", rc, msg)
	}
	if rc := auditReportMain([]string{"a;b"}); rc != 2 {
		t.Fatalf("이상한 아이디 종료 %d, 2여야", rc)
	}
}
