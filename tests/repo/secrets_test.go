package repo_test

// 개인 값·비밀 꼴(2026-11-22 go-shell-remove — 옛 herdr-verify.sh의 `herdr-tools-config lint --strict` 단계와
// tests/publish-secrets.test.sh를 Go로). 줄 글은 옛것 그대로다.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
	"herdr-tools/internal/toolcfg"
)

// privateLint: 지금 폴더(git 저장소)의 추적 파일을 개인 값 패턴(개인 설정 private-patterns.txt, 없으면 ~/.local/share/herdr-tools/)으로.
// strict: 패턴 파일·패턴 줄이 없으면 실패(검사 없이 착지하지 않게). 걸려도 실패.
// 패턴이 없는 게 정당한 곳(공개 사본을 받아 돌리는 쪽)은 HERDR_VERIFY_NO_LINT=1로만 건너뛴다.
// 레인 안(HC_LANE)에서는 그 값을 무시한다: 무인 레인이 그 값으로 막힘을 우회해 착지하지 않게. 통과면 true
func privateLint(noLint, lane, cfg string, w io.Writer) bool {
	if noLint == "1" && lane != "" {
		fmt.Fprintf(w, "레인 안이라 HERDR_VERIFY_NO_LINT 무시 (HC_LANE=%s)\n", lane)
	}
	if noLint == "1" && lane == "" {
		fmt.Fprintln(w, "건너뜀 개인 값 검사 (HERDR_VERIFY_NO_LINT=1)")
		return true
	}
	if toolcfg.Lint(cfg, true, nil, w, w) == 0 {
		fmt.Fprintln(w, "통과 개인 값 검사")
		return true
	}
	fmt.Fprintln(w, "실패 개인 값 검사 (위 줄을 범용 값으로 바꾸거나 그 파일을 개인 저장소로. 패턴 파일 없음이면 herdr-sync로 맞춘다)")
	return false
}

func TestPrivateValues(t *testing.T) {
	t.Chdir(clitest.Root(t))
	var b bytes.Buffer
	ok := privateLint(os.Getenv("HERDR_VERIFY_NO_LINT"), os.Getenv("HC_LANE"), toolcfg.Dir(), &b)
	t.Log(strings.TrimSpace(b.String()))
	if !ok {
		t.Error(b.String())
	}
}

// 판정 자체(옛 tests/herdr-verify.test.sh 6·7절): 걸림·안 걸림·패턴 파일 없음·건너뛰기·레인 안 무시
func TestPrivateValuesSelf(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	t.Setenv("HOME", T+"/home") // 기계의 ~/.local/share/herdr-tools 패턴에 흔들리지 않게
	conf, none := T+"/conf", T+"/none"
	clitest.Put(t, conf+"/private-patterns.txt", "secret-word\n")
	R := T + "/r"
	clitest.Put(t, R+"/x.txt", "clean\n")
	clitest.MustGit(t, R, "init", "-q")
	clitest.MustGit(t, R, "add", ".")
	t.Chdir(R)
	run := func(noLint, lane, cfg string) (bool, string) {
		var b bytes.Buffer
		ok := privateLint(noLint, lane, cfg, &b)
		return ok, b.String()
	}
	if ok, out := run("", "", conf); !ok || !strings.Contains(out, "통과 개인 값 검사") {
		t.Errorf("개인 값 없음: 통과 개인 값 검사여야\n%s", out)
	}
	clitest.Put(t, R+"/x.txt", "clean\nsecret-word\n")
	if ok, out := run("", "", conf); ok || !strings.Contains(out, "실패 개인 값 검사") || !strings.Contains(out, "x.txt:2: secret-word") {
		t.Errorf("개인 값 있음: 실패·걸린 줄이어야\n%s", out)
	}
	clitest.Put(t, R+"/x.txt", "clean\n")
	if ok, out := run("", "", none); ok || !strings.Contains(out, "실패 개인 값 검사") || !strings.Contains(out, "herdr-sync") {
		t.Errorf("패턴 파일 없음: 실패·herdr-sync 안내여야\n%s", out)
	}
	if ok, out := run("1", "", none); !ok || !strings.Contains(out, "건너뜀 개인 값 검사") || strings.Contains(out, "통과 개인 값 검사") {
		t.Errorf("패턴 파일 없음 + HERDR_VERIFY_NO_LINT=1: 건너뜀 줄·통과여야\n%s", out)
	}
	if ok, out := run("1", "lane-x", none); ok || !strings.Contains(out, "레인 안이라 HERDR_VERIFY_NO_LINT 무시") ||
		!strings.Contains(out, "실패 개인 값 검사") || strings.Contains(out, "건너뜀 개인 값 검사") {
		t.Errorf("레인 안 + HERDR_VERIFY_NO_LINT=1: 무시 줄·실패여야\n%s", out)
	}
}

// 착지 전 비밀 꼴 검사(2026-11-21 publish-lint-go — Go 시험의 가짜 SHA 40자가 착지한 뒤 공개 내보내기에서야 걸려 릴리스가 막혔다).
// 추적 파일의 내용·경로를 herdr-tools-publish와 같은 scan으로 본다(cmd/herdr-tools-publish TestTrackedNoSecret, HERDR_SCAN_ROOT).
// 그 시험 바이너리를 한 번 지어 이 저장소와, 가짜 40자 hex를 추적한 임시 저장소(실패해야 한다)에 돌린다
func TestPublishSecrets(t *testing.T) {
	R := clitest.Root(t)
	T := clitest.TempDir(t)
	bin := filepath.Join(T, "publish.test")
	c := exec.Command("go", "test", "-c", "-o", bin, "./cmd/herdr-tools-publish")
	c.Dir, c.Env = R, append(os.Environ(), "GOFLAGS=")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("herdr-tools-publish 시험 짓기 실패: %v\n%s", err, out)
	}
	scan := func(root string) (string, error) {
		c := exec.Command(bin, "-test.count=1", "-test.run", "^TestTrackedNoSecret$")
		c.Dir, c.Env = filepath.Join(R, "cmd/herdr-tools-publish"), append(os.Environ(), "HERDR_SCAN_ROOT="+root)
		out, err := c.CombinedOutput()
		return string(out), err
	}
	if out, err := scan(R); err != nil {
		t.Errorf("추적 파일에 비밀 꼴(공개 내보내기가 막는다): %v\n%s", err, out)
	}
	clitest.Isolate(t, T)
	F := T + "/fake"
	clitest.Put(t, F+"/fake.txt", fmt.Sprintf("sha = \"%s%s\"\n", "abc1234000000000000", "000000000000000000000"))
	clitest.MustGit(t, F, "init", "-q")
	clitest.MustGit(t, F, "add", "--", "fake.txt")
	out, err := scan(F)
	switch {
	case err == nil:
		t.Errorf("가짜 40자 hex를 추적해도 통과\n%s", out)
	case !strings.Contains(out, "fake.txt:1: 비밀 꼴"):
		t.Errorf("가짜 40자 hex: 다른 까닭으로 실패\n%s", out)
	}
}
