package main

// fill-named-cols(2026-11-24): 탭 구분 줄의 칸은 cols.go의 이름으로만 읽고 쓴다 — 칸 정의 파일 밖(시험 포함)에 숫자 칸 접근이 다시
// 들어오면 실패한다. 새 칸·새 파일 종류는 cols.go에 이름을 더하고 그것을 쓴다

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 숫자 칸 접근: Row.F(n)·Row.Set(n, …), 칸 번호·칸 수를 받는 도우미 evF·lcCut·auRest·lsSplit의 숫자(lsSplit 결과는 Row로 바꿔 이름으로 읽는다)
var numColRe = regexp.MustCompile(`\.F\([0-9]+\)|\.Set\([0-9]+,|\b(evF|lcCut|auRest|lsSplit)\([^()]*(\([^()]*\))?[^()]*,\s*[0-9]+\)`)

func TestNoNumericColumns(t *testing.T) {
	fs, err := filepath.Glob("*.go")
	if err != nil || len(fs) == 0 {
		t.Fatalf("꾸러미 파일 목록: %v %d", err, len(fs))
	}
	var hits []string
	for _, f := range fs {
		if f == "cols.go" || f == "cols_guard_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if numColRe.MatchString(l) {
				hits = append(hits, f+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(l))
			}
		}
	}
	if len(hits) > 0 {
		t.Fatalf("숫자 칸 접근 %d곳 — cols.go의 칸 이름을 쓴다(없으면 더한다):\n%s", len(hits), strings.Join(hits, "\n"))
	}
}

// 검사식이 실제로 잡는가(검사가 헛돌면 위 시험은 늘 통과한다)
func TestNoNumericColumnsPattern(t *testing.T) {
	for _, s := range []string{`r.F(1)`, `qrow(n).F(13)`, `r.Set(6, s)`, `evF(l, 5)`, `evF(runlast(a, b, c, s), 6)`, `lcCut(joblast(n, a, ""), 5)`, `auRest(r, 17)`, `lsSplit(l, 17)`, `lsSplit(r.Line(), 12)`} {
		if !numColRe.MatchString(s) {
			t.Errorf("못 잡음: %s", s)
		}
	}
	for _, s := range []string{`r.F(qName)`, `r.Set(qState, s)`, `evF(l, evState)`, `auRest(r, atNote)`, `lsSplit(l, qNF)`, `F1(f, 1)`, `fmt.Sprintf("%d", 1)`} {
		if numColRe.MatchString(s) {
			t.Errorf("잘못 잡음: %s", s)
		}
	}
}
