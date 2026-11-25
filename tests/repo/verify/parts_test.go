//go:build !windows

package main

// 통과 조각 이어 쓰기(parts.go, HERDR_VERIFY_PARTS): 병합기 추측 일꾼이 다른 기계에서 다시 맡은 꼴 — 기계마다 통과 기록(사용자 캐시)이
// 다르니 실행마다 빈 캐시로 돌린다. ① 한 조각 실패 → 나머지 통과 조각이 파일에 ② 같은 트리 다시 → 그 조각만 ③ 트리가 바뀜 → 전부

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPartsResume(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	R := scopeRepo(t)
	for n, s := range map[string]string{
		"ta/m_test.go":                 markTest("ta", "TestMA", "ta"),
		"tl/m_test.go":                 markTest("tl", "TestML", "tl"),
		"tr/m_test.go":                 markTest("tr", "TestMR", "rest"),
		"claude/hooks/tests/c_test.go": markTest("tests", "TestCotenantGuard", "cot"),
		"tests/repo/r_test.go":         "package repo\n\nimport \"testing\"\n\nfunc TestR(t *testing.T) {}\n",
	} {
		scopePut(t, R, n, s)
	}
	scopeCommit(t, R)
	A, P := filepath.Join(t.TempDir(), "atoms"), filepath.Join(t.TempDir(), "id.parts")
	old, oldSp := scopeCache, splits
	splits = []split{{mod + "ta", nil, 0}, {mod + "tl", nil, 0}}
	t.Cleanup(func() { scopeCache, splits = old, oldSp })
	t.Setenv("ATOMS", A)
	t.Setenv("HERDR_VERIFY_PARTS", P)
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "4")
	t.Setenv("HERDR_VERIFY_RETRY", "0")
	t.Setenv("HERDR_VERIFY_SCOPE", "")
	run := func() (int, string, string) {
		t.Helper()
		C := t.TempDir() // 다른 기계 — 이 기계의 통과 기록 없음
		scopeCache = func() string { return C }
		os.Remove(A)
		f, read := outFile(t)
		rc := verify(R, nil, make(chan os.Signal), f)
		b, _ := os.ReadFile(A)
		ran := strings.Fields(string(b))
		sort.Strings(ran)
		return rc, strings.Join(ran, ","), read()
	}
	parts := func() []string { // 첫 줄만(계약 — 일꾼은 첫 줄을 읽는다)
		b, _ := os.ReadFile(P)
		l, _, _ := strings.Cut(string(b), "\n")
		return strings.Fields(l)
	}

	t.Setenv("FAIL_ta", "1")
	if rc, ran, out := run(); rc != 1 || ran != "cot,rest,ta,tl" || len(parts()) != 4 {
		t.Fatalf("① ta 실패 · 나머지 셋 통과 조각이 파일에(트리 + 3): 종료 %d 돈 %q 파일 %q\n%s", rc, ran, parts(), out)
	}
	t.Setenv("FAIL_ta", "")
	if rc, ran, out := run(); rc != 0 || ran != "ta" || !strings.Contains(out, "앞 실행이 통과한 조각 3개 건너뜀") || len(parts()) != 5 {
		t.Fatalf("② 같은 트리 — ta만: 종료 %d 돈 %q 파일 %q\n%s", rc, ran, parts(), out)
	}
	scopePut(t, R, "docs/a.md", "# a3\n")
	scopePut(t, R, "tl/m_test.go", markTest("tl", "TestML", "tl")+"\n// x\n")
	scopeCommit(t, R)
	if rc, ran, out := run(); rc != 0 || ran != "cot,rest,ta,tl" || strings.Contains(out, "이어 쓰기") {
		t.Fatalf("③ 트리가 바뀌면 이어 쓰지 않는다: 종료 %d 돈 %q\n%s", rc, ran, out)
	}
}

// 바깥 검증의 조각 파일은 자식(go test·린트)에 물리지 않는다 — 픽스처로 verify()를 부르는 시험이 바깥 일꾼 파일을 픽스처 트리 줄로 덮던 꼴(감사 atp26g3 낮음 1)
func TestStartDropsParts(t *testing.T) {
	t.Setenv("HERDR_VERIFY_PARTS", "/outer.parts")
	for _, env := range [][]string{nil, {"X=1"}} {
		f, err := os.CreateTemp(t.TempDir(), "o")
		if err != nil {
			t.Fatal(err)
		}
		c, err := start(t.TempDir(), []string{"sh", "-c", `printf %s "${HERDR_VERIFY_PARTS-unset}"`}, env, f)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.Wait()
		if b, _ := os.ReadFile(f.Name()); string(b) != "unset" {
			t.Fatalf("env %v: 자식이 HERDR_VERIFY_PARTS=%q 를 물려받음", env, b)
		}
	}
}
