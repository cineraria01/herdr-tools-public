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

// arSetup: 원격(bare)과 두 클론, 감사 기계 HOME에 보고 하나 — 감사 낮음 후속(lows-1126b-ht)
func arSetup(t *testing.T, aid string) (root, a, b string) {
	t.Helper()
	root = t.TempDir()
	for _, kv := range [][2]string{{"GIT_CONFIG_GLOBAL", "/dev/null"}, {"GIT_AUTHOR_NAME", "t"}, {"GIT_AUTHOR_EMAIL", "t@example.invalid"},
		{"GIT_COMMITTER_NAME", "t"}, {"GIT_COMMITTER_EMAIL", "t@example.invalid"}, {"XDG_STATE_HOME", filepath.Join(root, "st")},
		{"HC_TERMINAL_ID", ""}, {"HC_LANE", ""}, {"HC_LANE_JOB", ""}, {"HC_LANE_OWNER", ""}} {
		t.Setenv(kv[0], kv[1])
	}
	bare := filepath.Join(root, "o.git")
	a, b = filepath.Join(root, "a"), filepath.Join(root, "b")
	arGit(t, root, "init", "-q", "--bare", bare)
	arGit(t, root, "clone", "-q", bare, a)
	arGit(t, root, "clone", "-q", bare, b)
	t.Setenv("HOME", filepath.Join(root, "ha"))
	rep := auditReportPath(aid)
	if err := os.MkdirAll(filepath.Dir(rep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rep, []byte("# 감사 "+aid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, a, b
}

// arStderr: f를 돌리는 동안의 표준 오류
func arStderr(t *testing.T, f func()) string {
	t.Helper()
	ef := filepath.Join(t.TempDir(), "err")
	w, err := os.Create(ef)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	f()
	os.Stderr = old
	w.Close()
	b, _ := os.ReadFile(ef)
	return string(b)
}

// 낮음 1: report 판정 「감사 완료 <ID> …」이 보고를 원격에 올린다(reportMain → auditPublish 연결)
func TestAuditReportVerdictPublishes(t *testing.T) {
	root, a, b := arSetup(t, "atx2-ab")
	t.Chdir(a)
	if rc := reportMain([]string{"판정", "-", "감사 완료 atx2-ab 높음 0·중간 0·낮음 0"}); rc != 0 {
		t.Fatalf("report 판정 종료 %d", rc)
	}
	t.Setenv("HOME", filepath.Join(root, "hb"))
	if rc := auditReportMain([]string{"atx2-ab", "--repo", b}); rc != 0 {
		t.Fatalf("판정 뒤 다른 기계 audit-report 종료 %d, 0이어야(판정이 올리지 않았다)", rc)
	}
}

// 낮음 2: 올리기가 실패해도 audit-report --publish <ID>로 감사 기계에서 다시 올리고, 경고·없음 글이 그 명령을 말한다
func TestAuditReportRepublish(t *testing.T) {
	root, a, b := arSetup(t, "atx3-ab")
	t.Chdir(a)
	arGit(t, a, "remote", "rename", "origin", "o2") // 올리기 실패 흉내
	if e := arStderr(t, func() { auditPublish("감사 완료 atx3-ab 낮음 1") }); !strings.Contains(e, "hc-lane audit-report --publish atx3-ab") {
		t.Fatalf("올리기 실패 경고에 다시 올리는 명령이 없다: %q", e)
	}
	t.Setenv("HOME", filepath.Join(root, "hb"))
	if e := arStderr(t, func() { auditReportMain([]string{"atx3-ab", "--repo", b}) }); !strings.Contains(e, "audit-report --publish atx3-ab") {
		t.Fatalf("없음 글에 감사 기계에서 다시 올리는 명령이 없다: %q", e)
	}
	arGit(t, a, "remote", "rename", "o2", "origin")
	t.Setenv("HOME", filepath.Join(root, "ha"))
	if rc := auditReportMain([]string{"--publish", "atx3-ab"}); rc != 0 {
		t.Fatalf("--publish 종료 %d", rc)
	}
	t.Setenv("HOME", filepath.Join(root, "hb"))
	if rc := auditReportMain([]string{"atx3-ab", "--repo", b}); rc != 0 {
		t.Fatalf("다시 올린 뒤 audit-report 종료 %d", rc)
	}
	// 보고가 없는 기계에서 --publish → 1
	if rc := auditReportMain([]string{"--publish", "atx3-zz", "--repo", a}); rc != 1 {
		t.Fatalf("보고 없는 --publish 종료 %d, 1이어야", rc)
	}
}

// 낮음 3: 저장소 밖·origin 없는 저장소는 「잠시 뒤 다시」(75)가 아니라 인자 오류(2)와 --repo 안내
func TestAuditReportNoRepo(t *testing.T) {
	root, a, _ := arSetup(t, "atx4-ab")
	t.Setenv("HOME", filepath.Join(root, "hb"))
	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	arGit(t, a, "remote", "remove", "origin")
	for _, dir := range []string{plain, a} {
		var rc int
		e := arStderr(t, func() { rc = auditReportMain([]string{"atx4-ab", "--repo", dir}) })
		if rc != 2 || !strings.Contains(e, "--repo") {
			t.Fatalf("%s: 종료 %d 글 %q — 2와 --repo 안내여야", dir, rc, e)
		}
	}
}
