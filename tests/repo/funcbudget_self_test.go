package repo_test

import (
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

// 판정 자체: 임시 저장소에서 함수 늘어남·새 긴 함수·예산 밖·사라진 함수·레인의 예산 올림이 걸리는지
func TestFuncBudgetSelf(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	R := T + "/r"
	fn := func(name string, body int) string { // body줄짜리 몸 → 함수 전체 body+2줄
		return "func " + name + "() {\n" + strings.Repeat("\t_ = 0\n", body) + "}\n"
	}
	src := func(fs ...string) string { return "package p\n\n" + strings.Join(fs, "\n") }
	clitest.Put(t, R+"/a.go", src(fn("big", 248), fn("small", 3), "type T struct{}\n\nfunc (t *T) "+strings.TrimPrefix(fn("M", 300), "func ")))
	clitest.Put(t, R+"/a_test.go", src(fn("huge", 900)))
	clitest.Put(t, R+"/tests/x/b.go", src(fn("huge", 900)))
	clitest.MustGit(t, R, "init", "-q", ".")
	clitest.MustGit(t, R, "add", ".")
	lens := func() map[string]int {
		t.Helper()
		m, err := funcLens(R)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	has := func(v []string, s string) bool {
		for _, l := range v {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	if m := lens(); m["a.go big"] != 250 || m["a.go small"] != 5 || m["a.go T.M"] != 302 || len(m) != 3 {
		t.Fatalf("함수 길이 재기(func 줄~닫는 }, 메서드는 형.이름, 시험·tests/ 빼고): %v", m)
	}
	b := map[string]int{"a.go big": 250, "a.go T.M": 302}
	if v := checkFuncBudget(lens(), b, nil, false); len(v) != 0 {
		t.Errorf("예산 그대로·시험 파일의 긴 함수는 통과여야: %v", v)
	}
	if !has(checkFuncBudget(lens(), map[string]int{"a.go T.M": 302}, nil, false), "새 긴 함수 a.go big 250줄") {
		t.Error("예산 없는 긴 함수는 실패여야")
	}
	clitest.Put(t, R+"/a.go", src(fn("big", 249), fn("small", 3), "type T struct{}\n\nfunc (t *T) "+strings.TrimPrefix(fn("M", 290), "func ")))
	v := checkFuncBudget(lens(), b, nil, false)
	if !has(v, "함수가 늘었다 a.go big 251줄") || has(v, "T.M") {
		t.Errorf("예산보다 늘면 실패·줄면 통과여야: %v", v)
	}
	if !has(checkFuncBudget(lens(), map[string]int{"a.go big": 260, "a.go T.M": 302, "a.go gone": 300}, nil, false), "예산에 없는 함수 a.go gone") {
		t.Error("사라진 함수의 예산 줄은 실패여야")
	}
	o := map[string]int{"a.go big": 260, "a.go T.M": 302}
	if !has(checkFuncBudget(lens(), map[string]int{"a.go big": 270, "a.go T.M": 302}, o, true), "레인이 함수 예산을 늘렸다 a.go big 270 (갈래점 260)") {
		t.Error("레인의 함수 예산 올림은 실패여야")
	}
	if v := checkFuncBudget(lens(), map[string]int{"a.go big": 270, "a.go T.M": 302}, o, false); len(v) != 0 {
		t.Errorf("레인 밖(지휘·사람)은 함수 예산을 올릴 수 있어야: %v", v)
	}
	if !has(checkFuncBudget(lens(), map[string]int{"a.go big": 260, "a.go T.M": 302, "a.go small": 5}, o, true), "레인이 함수 예산을 늘렸다 a.go small 5 (갈래점 없음)") {
		t.Error("레인이 새 예산 줄을 더하면 실패여야")
	}
	clitest.Put(t, R+"/a.go", "package p\nfunc (\n")
	if _, err := funcLens(R); err == nil {
		t.Error("구문 분석 실패가 통과로 읽힘")
	}
	if got := readFuncBudget("# 주석 12 a b\n300 a.go big\n12\tb.go T.M\n"); len(got) != 2 || got["a.go big"] != 300 || got["b.go T.M"] != 12 {
		t.Errorf("예산 줄 읽기: %v", got)
	}
}
