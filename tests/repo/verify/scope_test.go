//go:build !windows

package main

// 범위 지문(scope.go)의 계약 — 임시 모듈(git 저장소)로: 꾸러미 소스 한 줄은 그 꾸러미에 기대는 조각만, 시험이 지어 돌리는 도구와
// 그 도구가 부르는 도구도 입력, 주석에만 나온 도구 이름은 입력이 아님, cmd/ 새 폴더·config.example·.claude/는 전부, 문서만은 아무것도
// (Go 소스가 경로로 가리킨 문서는 전부), testdata는 그 꾸러미, argv·env가 다르면 다른 지문, 작업 트리가 깨끗하지 않으면 dirty,
// go list가 실패하면 색인 오류.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scopeRepo: 임시 모듈 — ta는 tool-a를 지어 돌리고(tool-a는 tool-b를 부른다), tl은 lib를 가져오고, tc는 주석에만 tool-c
func scopeRepo(t *testing.T) string {
	t.Helper()
	R := tmp(t)
	files := map[string]string{
		"go.mod":                  "module herdr-tools\n\ngo 1.24\n",
		".claude/herdr-verify.sh": "#!/bin/bash\n",
		"config.example/x.conf":   "x 1\n",
		"docs/a.md":               "# a\n",
		"docs/ref.md":             "# ref\n",
		"README.md":               "# r\n",
		"lib/lib.go":              "package lib\n\nfunc N() int { return 1 }\n",
		"cmd/tool-a/main.go":      "package main\n\nimport \"os/exec\"\n\nfunc main() { exec.Command(\"tool-b\").Run() }\n",
		"cmd/tool-b/main.go":      "package main\n\nfunc main() {}\n",
		"cmd/tool-c/main.go":      "package main\n\nfunc main() {}\n",
		"ta/ta_test.go":           "package ta\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { _ = \"./cmd/\" + \"tool-a\" }\n",
		"ta/testdata/in.txt":      "1\n",
		"tl/tl_test.go":           "package tl\n\nimport (\n\t\"testing\"\n\n\t\"herdr-tools/lib\"\n)\n\nfunc TestL(t *testing.T) { _ = lib.N() }\n",
		"tc/tc_test.go": "package tc\n\nimport \"testing\"\n\n// tool-c를 부르지 않는다 — rlRepo(t)는 가짜 저장소\n" +
			"func TestC(t *testing.T) { _ = \"tool-c이 정리한다 tool-c.log README.md\" }\n",
		"td/td_test.go":             "package td\n\nimport \"testing\"\n\nfunc TestD(t *testing.T) { _ = \"docs/ref.md\" }\n",
		"tests/repo/verify/main.go": "package main\n\nfunc main() {}\n",
	}
	for n, s := range files {
		scopePut(t, R, n, s)
	}
	scopeGitT(t, R, "init", "-q")
	scopeCommit(t, R)
	return R
}

func scopePut(t *testing.T, R, n, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(R, n)), 0o755)
	if err := os.WriteFile(filepath.Join(R, n), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scopeGitT(t *testing.T, R string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	c.Dir = R
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func scopeCommit(t *testing.T, R string) {
	t.Helper()
	scopeGitT(t, R, "add", "-A")
	scopeGitT(t, R, "commit", "-qm", "x", "--allow-empty")
}

// scopeKeys: ta·tl·tc·td 조각의 지문
func scopeKeys(t *testing.T, R string) map[string]string {
	t.Helper()
	x, err := scopeIndex(R)
	if err != nil {
		t.Fatal(err)
	}
	if x.dirty != "" {
		t.Fatalf("깨끗해야 dirty=%q", x.dirty)
	}
	m := map[string]string{}
	for _, p := range []string{"ta", "tl", "tc", "td"} {
		k, err := x.key([]string{"go", "test", "-race", mod + p}, nil)
		if err != nil {
			t.Fatal(err)
		}
		m[p] = k
	}
	return m
}

// changed: before와 견줘 지문이 바뀐 조각(이름순 , 로)
func changed(before, after map[string]string) string {
	var out []string
	for _, p := range []string{"ta", "tl", "tc", "td"} {
		if before[p] != after[p] {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

func TestScopeKeys(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	R := scopeRepo(t)
	k0 := scopeKeys(t, R)
	if again := scopeKeys(t, R); changed(k0, again) != "" {
		t.Fatalf("그대로면 같은 지문이어야: %s", changed(k0, again))
	}
	for _, c := range []struct {
		name, file, body, want string
	}{
		{"꾸러미 소스 한 줄 — 그 꾸러미에 기대는 조각만", "lib/lib.go", "package lib\n\nfunc N() int { return 2 }\n", "tl"},
		{"시험이 지어 돌리는 도구", "cmd/tool-a/main.go", "package main\n\nimport \"os/exec\"\n\nfunc main() { exec.Command(\"tool-b\", \"x\").Run() }\n", "ta"},
		{"도구가 부르는 도구", "cmd/tool-b/main.go", "package main\n\nfunc main() { println() }\n", "ta"},
		{"주석에만 나온 도구는 입력이 아님", "cmd/tool-c/main.go", "package main\n\nfunc main() { println() }\n", ""},
		{"testdata는 그 꾸러미", "ta/testdata/in.txt", "2\n", "ta"},
		{"문서만 — 아무것도", "docs/a.md", "# a2\n", ""},
		{"뿌리 README — 아무것도", "README.md", "# r2\n", ""},
		{"Go 소스가 가리킨 문서 — 전부", "docs/ref.md", "# ref2\n", "ta,tl,tc,td"},
		{"config.example — 전부", "config.example/x.conf", "x 2\n", "ta,tl,tc,td"},
		{".claude/ 검증 스크립트 — 전부", ".claude/herdr-verify.sh", "#!/bin/bash\n:\n", "ta,tl,tc,td"},
		{"검증 본문 — 전부", "tests/repo/verify/main.go", "package main\n\nfunc main() { println() }\n", "ta,tl,tc,td"},
		{"cmd/ 새 폴더 — 전부", "cmd/tool-new/main.go", "package main\n\nfunc main() {}\n", "ta,tl,tc,td"},
		{"go.mod — 전부", "go.mod", "module herdr-tools\n\ngo 1.24\n\n// x\n", "ta,tl,tc,td"},
		{"도구의 시험 파일 — 아무것도", "cmd/tool-a/a_test.go", "package main\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) { _ = \"tool-c\" }\n", ""},
		{"도구 시험 파일·문장에만 나온 도구 — 아무것도", "cmd/tool-c/main.go", "package main\n\nfunc main() { println(1) }\n", ""},
		{"뿌리를 읽는 시험이 생김 — README가 공통 입력이 되어 전부", "td/td_test.go", "package td\n\nimport \"testing\"\n\nfunc Repo(t *testing.T) string { return \"\" }\n\n" +
			"func TestD(t *testing.T) { _ = \"docs/ref.md\"; _ = Repo(t) + \"/README.md\" }\n", "ta,tl,tc,td"},
		{"뿌리를 읽는 시험에 나온 README — 전부", "README.md", "# r3\n", "ta,tl,tc,td"},
		{"주석에 문서 경로", "lib/lib.go", "package lib\n\n// docs/a.md 참고\nfunc N() int { return 5 }\n", "tl"},
		{"주석에만 나온 문서 — 아무것도", "docs/a.md", "# a3\n", ""},
		{"검증 본문의 시험 파일 — 아무것도", "tests/repo/verify/main_test.go", "package main\n", ""},
		{"config.example 안의 .md — 전부(문서 아님)", "config.example/z.md", "# z\n", "ta,tl,tc,td"},
		{".claude/ 안의 .md — 전부(문서 아님)", ".claude/rule.md", "# r\n", "ta,tl,tc,td"},
		{".gitignore — 전부", ".gitignore", "cmd/.omc/\n", "ta,tl,tc,td"},
		{"cmd/ 아래 무시된 점 폴더(세션 훅) — 아무것도", "cmd/.omc/state", "x\n", ""},
	} {
		before := scopeKeys(t, R)
		scopePut(t, R, c.file, c.body)
		scopeCommit(t, R)
		if got := changed(before, scopeKeys(t, R)); got != c.want {
			t.Errorf("%s: 바뀐 조각 %q여야 %q", c.name, c.want, got)
		}
	}
	x, err := scopeIndex(R)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := x.key([]string{"go", "test", "-run", "^TestA$", mod + "ta"}, nil)
	b, _ := x.key([]string{"go", "test", "-run", "^TestB$", mod + "ta"}, nil)
	c, _ := x.key([]string{"go", "test", "-run", "^TestA$", mod + "ta"}, []string{"HCFILL_SEC=a"})
	if a == b || a == c || b == c {
		t.Errorf("조각 표(argv·env)가 바뀌면 지문도 바뀌어야")
	}
	if _, err := x.key([]string{"go", "test", "./..."}, nil); err == nil {
		t.Errorf("모르는 꾸러미(./...)면 오류여야")
	}
	if k, err := x.key([]string{"go", "test", "./ta/"}, nil); err != nil || k == "" {
		t.Errorf("./폴더 꼴도 꾸러미로: %v", err)
	}
}

// 작업 트리가 깨끗하지 않으면 dirty(추적 파일 변경·문서 밖 추적 안 된 파일). 추적 안 된 문서는 깨끗. go list 실패면 색인 오류
func TestScopeDirty(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	R := scopeRepo(t)
	dirty := func() string {
		x, err := scopeIndex(R)
		if err != nil {
			t.Fatal(err)
		}
		return x.dirty
	}
	scopePut(t, R, "docs/new.md", "x\n")
	if d := dirty(); d != "" {
		t.Errorf("추적 안 된 문서는 깨끗해야: %q", d)
	}
	scopePut(t, R, "tl/new.go", "package tl\n")
	if d := dirty(); !strings.Contains(d, "tl/new.go") {
		t.Errorf("추적 안 된 소스면 dirty여야: %q", d)
	}
	os.Remove(filepath.Join(R, "tl/new.go"))
	scopePut(t, R, "lib/lib.go", "package lib\n\nfunc N() int { return 3 }\n")
	if d := dirty(); !strings.Contains(d, "lib/lib.go") {
		t.Errorf("고친 추적 파일이면 dirty여야: %q", d)
	}
	scopePut(t, R, "go.mod", "this is not a go.mod\n")
	if _, err := scopeIndex(R); err == nil {
		t.Errorf("go list 실패면 오류여야")
	}
}

// 기록: 넣으면 있고, 오래된 것·상한 넘는 것은 지운다. 놓친 입력은 한 줄씩 쌓인다
func TestScopePass(t *testing.T) {
	C := t.TempDir()
	old := scopeCache
	scopeCache = func() string { return C }
	t.Cleanup(func() { scopeCache = old })
	if passHas("k1") {
		t.Fatal("처음엔 없어야")
	}
	if err := passPut("k1", "[1/2] x", 7); err != nil || !passHas("k1") {
		t.Fatalf("넣으면 있어야: %v", err)
	}
	if s, ok := passSecs("k1"); !ok || s != 7 {
		t.Errorf("기록의 초 7이어야: %d %v", s, ok)
	}
	if err := missLog("abc", "k1", "[1/2] x"); err != nil {
		t.Fatal(err)
	}
	missLog("abc", "k2", "[2/2] y") //nolint:errcheck
	b, _ := os.ReadFile(filepath.Join(C, "verify-scope-misses.log"))
	if n := strings.Count(string(b), "\n"); n != 2 || !strings.Contains(string(b), "\tabc\tk2\t[2/2] y\n") {
		t.Errorf("놓친 입력 두 줄이어야\n%s", b)
	}
	now := time.Now()
	passPut("stale", "x", 0) //nolint:errcheck
	os.Chtimes(filepath.Join(C, "verify-pass", "stale"), now, now.Add(-passAge-time.Hour))
	for i := 0; i < passMax+2; i++ {
		passPut(fmt.Sprintf("n%04d", i), "x", 0) //nolint:errcheck
		ts := now.Add(time.Duration(i-passMax-2) * time.Second)
		os.Chtimes(filepath.Join(C, "verify-pass", fmt.Sprintf("n%04d", i)), ts, ts)
	}
	passPrune(now)
	ents, _ := os.ReadDir(filepath.Join(C, "verify-pass"))
	if len(ents) != passMax || passHas("stale") || passHas("n0000") || passHas("n0001") || !passHas(fmt.Sprintf("n%04d", passMax+1)) {
		t.Errorf("오래된 것·상한 넘는 오래된 것을 지워 %d개여야 %d개", passMax, len(ents))
	}
}
