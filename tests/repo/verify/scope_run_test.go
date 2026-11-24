//go:build !windows

package main

// 범위의 실행 계약(verify-scope) — 임시 모듈(scopeRepo + 시험이 돈 조각을 ATOMS에 적는 시험들)에 진짜 go로 verify를 돌린다:
// ① 처음 전부 돎·기록 ② 그대로 다시 — observe·on 모두 건너뜀(전부 기록과 같음) ③ 꾸러미 한 줄 — on은 그에 기대는 조각만, observe는 전부 돌되
// 「건너뛸 수 있었음」 ④ 도구 ⑤ 도구가 부르는 도구 ⑥ cmd/ 새 폴더 ⑦ config.example ⑧ 문서만 ⑨ 깨끗하지 않음 ⑩ 지문 실패 ⑪ 놓친 입력 ⑫ HERDR_VERIFY_ALL=1

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mark: 시험 소스에 넣는 ATOMS 기록 — FAIL_<이름>이 있으면 실패(지문에 안 드는 입력 = 놓친 입력)
func markTest(pkg, name, atom string) string {
	return "package " + pkg + "\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc " + name + "(t *testing.T) {\n" +
		"\tf, _ := os.OpenFile(os.Getenv(\"ATOMS\"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)\n\tf.WriteString(\"" + atom + "\\n\")\n\tf.Close()\n" +
		"\tif os.Getenv(\"FAIL_" + atom + "\") != \"\" {\n\t\tt.Fatal(\"FAIL\")\n\t}\n}\n"
}

func TestScopeRun(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	R := scopeRepo(t)
	for n, s := range map[string]string{
		"ta/m_test.go":                 markTest("ta", "TestMA", "ta"),
		"tl/m_test.go":                 markTest("tl", "TestML", "tl"),
		"tc/m_test.go":                 markTest("tc", "TestMC", "tc"),
		"td/m_test.go":                 markTest("td", "TestMD", "td"),
		"tr/m_test.go":                 markTest("tr", "TestMR", "rest"),
		"claude/hooks/tests/c_test.go": markTest("tests", "TestCotenantGuard", "cot"),
		"tests/repo/r_test.go":         "package repo\n\nimport \"testing\"\n\nfunc TestR(t *testing.T) {}\n",
	} {
		scopePut(t, R, n, s)
	}
	scopeCommit(t, R)
	C, A := t.TempDir(), filepath.Join(t.TempDir(), "atoms")
	old, oldSp := scopeCache, splits
	scopeCache = func() string { return C }
	splits = []split{{mod + "ta", nil, 0}, {mod + "tl", nil, 0}, {mod + "tc", nil, 0}, {mod + "td", nil, 0}}
	t.Cleanup(func() { scopeCache, splits = old, oldSp })
	t.Setenv("ATOMS", A)
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "4")
	t.Setenv("HERDR_VERIFY_RETRY", "0") // 실패 조각 다시 돌기(retry.go)는 retry_scope_test.go가 본다 — 여기는 돈 조각을 한 번씩 센다
	recs := func() int { e, _ := os.ReadDir(filepath.Join(C, "verify-pass")); return len(e) }
	const all = "cot,rest,ta,tc,td,tl"
	run := func(mode string) (int, string, string) { // 종료 코드, 돈 조각(이름순 ,), 출력
		t.Helper()
		t.Setenv("HERDR_VERIFY_SCOPE", mode)
		os.Remove(A)
		f, read := outFile(t)
		rc := verify(R, nil, make(chan os.Signal), f)
		b, _ := os.ReadFile(A)
		ran := strings.Fields(string(b))
		sort.Strings(ran)
		return rc, strings.Join(ran, ","), read()
	}
	expect := func(what, mode, want string, rcWant int, outHas ...string) string {
		t.Helper()
		rc, ran, out := run(mode)
		if rc != rcWant || ran != want {
			t.Errorf("%s(%s): 돈 조각 %q·종료 %d여야 %q·%d\n%s", what, mode, want, rcWant, ran, rc, out)
		}
		for _, s := range outHas {
			if !strings.Contains(out, s) {
				t.Errorf("%s(%s): 출력에 %q가 없다\n%s", what, mode, s, out)
			}
		}
		return out
	}
	change := func(file, body string) {
		t.Helper()
		scopePut(t, R, file, body)
		scopeCommit(t, R)
	}

	expect("① 처음", "", all, 0, "범위(observe): 조각 6개 중 0개 건너뛸 수 있었음")
	if recs() != 6 {
		t.Errorf("① 통과 기록 6개여야 %d", recs())
	}
	expect("② 그대로 다시", "observe", "", 0, "건너뜀 [홀로]", "건너뜀 [1/5]", "조각 6개 중 6개 건너뛸 수 있었음", "건너뜀 6개")
	expect("② 그대로 다시", "on", "", 0, "건너뜀 6개")

	change("lib/lib.go", "package lib\n\nfunc N() int { return 2 }\n")
	expect("③ 꾸러미 한 줄", "on", "rest,tl", 0, "조각 6개 중 4개 건너뛸 수 있었음")
	change("lib/lib.go", "package lib\n\nfunc N() int { return 3 }\n")
	out := expect("③ 꾸러미 한 줄", "observe", all, 0, "조각 6개 중 4개 건너뛸 수 있었음", "건너뜀 0개")
	if n := strings.Count(out, "건너뛸 수 있었음(입력이 통과 기록과 같음)"); n != 4 {
		t.Errorf("③ observe: 띄우는 줄 넷에 「건너뛸 수 있었음」이어야 %d\n%s", n, out)
	}

	change("cmd/tool-a/main.go", "package main\n\nimport \"os/exec\"\n\nfunc main() { exec.Command(\"tool-b\", \"x\").Run() }\n")
	expect("④ 도구", "on", "rest,ta", 0)
	change("cmd/tool-b/main.go", "package main\n\nfunc main() { println() }\n")
	expect("⑤ 도구가 부르는 도구", "on", "rest,ta", 0)
	change("cmd/tool-new/main.go", "package main\n\nfunc main() {}\n")
	expect("⑥ cmd/ 새 폴더", "on", all, 0)
	change("config.example/x.conf", "x 2\n")
	expect("⑦ config.example", "on", all, 0)
	change("docs/a.md", "# a2\n")
	expect("⑧ 문서만", "observe", "", 0, "건너뜀 6개")

	n0 := recs()
	scopePut(t, R, "tr/new.go", "package tr\n")
	expect("⑨ 추적 안 된 소스", "on", all, 0, "작업 트리가 깨끗하지 않다")
	os.Remove(filepath.Join(R, "tr/new.go"))
	scopePut(t, R, "tc/m_test.go", markTest("tc", "TestMC", "tc")+"\n// x\n")
	expect("⑨ 고친 추적 파일", "on", all, 0, "작업 트리가 깨끗하지 않다")
	if recs() != n0 {
		t.Errorf("⑨ 깨끗하지 않으면 기록을 안 써야: %d → %d", n0, recs())
	}
	scopeCommit(t, R)
	expect("⑨ 커밋 뒤", "on", "tc", 0)

	t.Setenv("GIT_DIR", filepath.Join(R, "no-such-git"))
	expect("⑩ 지문 실패", "on", all, 0, "알림 범위 지문 실패")
	os.Unsetenv("GIT_DIR")

	change("lib/lib.go", "package lib\n\nfunc N() int { return 4 }\n")
	t.Setenv("FAIL_ta", "1")
	expect("⑪ 건너뛸 조각이 실패", "observe", all, 1, "놓친 입력: [")
	t.Setenv("FAIL_ta", "")
	if b, err := os.ReadFile(filepath.Join(C, "verify-scope-misses.log")); err != nil || strings.Count(string(b), "\n") != 1 ||
		!strings.Contains(string(b), "herdr-tools/ta") {
		t.Errorf("⑪ 놓친 입력 기록 한 줄(ta)이어야: %v\n%s", err, b)
	}

	t.Setenv("HERDR_VERIFY_ALL", "1")
	expect("⑫ HERDR_VERIFY_ALL=1", "on", all, 0, "건너뜀 0개")
	t.Setenv("HERDR_VERIFY_ALL", "")
	expect("off", "off", all, 0)
}
