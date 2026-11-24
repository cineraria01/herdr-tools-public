//go:build !windows

package main

// ⑦ 흔들려 통과한 조각은 통과 기록(scope.go)을 남기지 않는다 — 임시 모듈에 진짜 go로 verify: tl의 시험은 표식 파일이 없으면 만들고
// 실패한다(첫 번 실패·다시 통과). 처음 검증은 통과·흔들림 줄, 통과 기록은 tl만 빠지고, 흔들림 기록은 통과 기록 곁(scopeCache)에.
// 같은 트리로 on 다시 — 기록이 있는 조각은 건너뛰고 tl만 다시 돈다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetryFlakyNoPassRecord(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	R := scopeRepo(t)
	flaky := "package tl\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestShakyTL(t *testing.T) {\n" +
		"\tp := os.Getenv(\"FLAKY_MARK\")\n\tif _, err := os.Stat(p); err != nil {\n\t\tos.WriteFile(p, nil, 0o644)\n\t\tt.Fatal(\"첫 번\")\n\t}\n}\n"
	for n, s := range map[string]string{
		"ta/m_test.go":                 "package ta\n\nimport \"testing\"\n\nfunc TestMA(t *testing.T) {}\n",
		"tl/m_test.go":                 flaky,
		"claude/hooks/tests/c_test.go": "package tests\n\nimport \"testing\"\n\nfunc TestCotenantGuard(t *testing.T) {}\n",
		"tests/repo/r_test.go":         "package repo\n\nimport \"testing\"\n\nfunc TestR(t *testing.T) {}\n",
	} {
		scopePut(t, R, n, s)
	}
	scopeCommit(t, R)
	C := t.TempDir()
	old, oldSp := scopeCache, splits
	scopeCache = func() string { return C }
	splits = []split{{mod + "ta", nil, 0}, {mod + "tl", nil, 0}}
	t.Cleanup(func() { scopeCache, splits = old, oldSp })
	t.Setenv("FLAKY_MARK", filepath.Join(t.TempDir(), "mark"))
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_RETRY", "")
	t.Setenv("HERDR_VERIFY_JOBS", "4")
	recs := func() string {
		e, _ := os.ReadDir(filepath.Join(C, "verify-pass"))
		var l []string
		for _, x := range e {
			b, _ := os.ReadFile(filepath.Join(C, "verify-pass", x.Name()))
			l = append(l, string(b))
		}
		return strings.Join(l, "")
	}
	run := func(mode string) (int, string) {
		t.Helper()
		t.Setenv("HERDR_VERIFY_SCOPE", mode)
		f, read := outFile(t)
		return verify(R, nil, make(chan os.Signal), f), read()
	}

	rc, out := run("")
	r := recs()
	if rc != 0 || !strings.Contains(out, "herdr-tools/tl — TestShakyTL · 혼자 다시 돌려 통과") || strings.Count(r, "\n") != 3 ||
		strings.Contains(r, "herdr-tools/tl") || !strings.Contains(r, "herdr-tools/ta") {
		t.Fatalf("흔들림: 통과·tl 말고 셋(홀로·ta·나머지)만 통과 기록이어야 rc=%d\n기록:\n%s\n%s", rc, r, out)
	}
	if b, _ := os.ReadFile(filepath.Join(C, "verify-flaky.log")); !strings.Contains(string(b), "\tTestShakyTL\t") {
		t.Errorf("흔들림 기록은 통과 기록 곁(scopeCache)에: %q", b)
	}
	rc, out = run("on")
	if rc != 0 || !strings.Contains(out, "건너뛸 수 있었음") || !strings.Contains(out, "건너뜀 3개") || strings.Contains(out, "\n흔들림: ") ||
		!strings.Contains(recs(), "herdr-tools/tl") {
		t.Errorf("다음 검증(on): tl만 다시 돌고 이번엔 통과 기록이어야 rc=%d\n%s", rc, out)
	}
}
