package main

// 「첫 실패」 이름표(2026-11-24 knob-test-leak): 검증 로그가 go test 출력(꾸러미 결과 줄 「ok <꾸러미>」·「FAIL <꾸러미>」이 있음)이면
// 실패한 꾸러미에 속한 최상위 「--- FAIL: <시험>」만 고른다. 병합기 계약 시험은 가짜 검증 출력(「--- FAIL: TestBad」)을 표준 출력에
// 찍어(tail5) 통과한 시험의 출력 속에 같은 꼴 줄이 섞인다 — 지휘가 그 이름표를 믿고 한 번 헛짚었다.
// 진짜 줄은 testing이 찍은 것이라 다음 줄이 그 시험의 본문(넷 칸 들여쓰기 — 「file_test.go:N:」·하위 시험)·「panic:」·꾸러미 끝 「FAIL」이다.
// 찍힌 가짜 줄 다음은 그 시험이 찍는 다른 줄이다. 결과 줄이 없는 로그(가짜 검증·bun 등)는 옛 규칙 그대로(첫 실패 줄).

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	pkgResult = regexp.MustCompile(`^(ok|FAIL)\s+(\S+)(\s|$)`)
	topFail   = regexp.MustCompile(`^--- FAIL: (\S+) \(`)
)

// pickFail: 줄들에서 이름표. old는 옛 규칙의 첫 실패 줄(빈 값이면 빈 값 — 실패 줄 없음·모름은 그대로), test면 시간 넘긴 꾸러미는 세지 않는다.
// 고른 시험이 없으면 꾸러미 줄(「FAIL <꾸러미> [build failed]」 등 — 「--- FAIL」 아닌 첫 실패 줄), 그것도 없으면 「이름 못 고름」
func pickFail(lines []string, old, path string, test bool) string {
	if old == "" {
		return ""
	}
	goOut := false
	for _, l := range lines {
		if m := pkgResult.FindStringSubmatch(l); m != nil && m[2] != "" {
			goOut = true
			break
		}
	}
	if !goOut {
		return old
	}
	type pkg struct {
		name  string
		tests []string
	}
	var got []pkg
	var pend []string
	timedOut := false
	for i, l := range lines {
		if strings.HasPrefix(l, "panic: test timed out") {
			timedOut = true
		}
		if m := topFail.FindStringSubmatch(l); m != nil {
			nx := ""
			if i+1 < len(lines) {
				nx = lines[i+1]
			}
			if strings.HasPrefix(nx, "    ") || strings.HasPrefix(nx, "panic:") || nx == "FAIL" {
				pend = append(pend, m[1])
			}
			continue
		}
		m := pkgResult.FindStringSubmatch(l)
		if m == nil || m[2] == "" {
			continue
		}
		if m[1] == "FAIL" && len(pend) > 0 && !envFail.MatchString(l) && !(test && timedOut) {
			got = append(got, pkg{m[2], uniq(pend)})
		}
		pend, timedOut = nil, false
	}
	if len(got) == 0 {
		if l := firstLine(lines, test, true); l != "" {
			return l
		}
		return "시험 실패(이름 못 고름 — 로그 끝 " + path + ")"
	}
	var parts []string
	for _, p := range got {
		s := p.name + ": " + strings.Join(p.tests[:min(3, len(p.tests))], ", ")
		if n := len(p.tests) - 3; n > 0 {
			s += fmt.Sprintf(" 외 %d", n)
		}
		parts = append(parts, s)
	}
	return bcut(200, flat(strings.Join(parts, " · ")))
}

func uniq(ss []string) []string {
	seen := map[string]bool{}
	var o []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			o = append(o, s)
		}
	}
	return o
}
