package hcfill_test

// 글·TSV 도우미(셸 시험의 grep·awk·cut 자리).

import (
	"regexp"
	"strings"
	"time"
)

// lines: 글의 줄들(끝 줄바꿈 하나는 빼고, 빈 글은 줄 없음).
func lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// rows: TSV 파일의 줄마다 칸(awk -F'\t').
func rows(p string) [][]string {
	var r [][]string
	for _, l := range lines(read(p)) {
		r = append(r, strings.Split(l, "\t"))
	}
	return r
}

// col: 1부터 센 칸(없으면 "").
func col(r []string, c int) string {
	if c < 1 || c > len(r) {
		return ""
	}
	return r[c-1]
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// grep: 글에 정규식(Go regexp, 줄 단위 ^$)이 맞는지.
func grep(s, re string) bool { return regexp.MustCompile("(?m)" + re).MatchString(s) }

// grepLines: 정규식에 맞는 줄들(grep).
func grepLines(s, re string) []string {
	r := regexp.MustCompile(re)
	var l []string
	for _, x := range lines(s) {
		if r.MatchString(x) {
			l = append(l, x)
		}
	}
	return l
}

// fgrepLines: 그 글이 든 줄들(grep -F).
func fgrepLines(s, sub string) []string {
	var l []string
	for _, x := range lines(s) {
		if strings.Contains(x, sub) {
			l = append(l, x)
		}
	}
	return l
}

// hasF·hasFx·hasE: 파일에 grep -F · grep -Fx · 정규식.
func hasF(p, s string) bool  { return strings.Contains(read(p), s) }
func hasFx(p, s string) bool { return has(lines(read(p)), s) }
func hasE(p, re string) bool { return grep(read(p), re) }

// countFx·countF·countE: 파일에서 맞는 줄 수(grep -Fxc · -Fc · -c).
func countFx(p, s string) int {
	n := 0
	for _, l := range lines(read(p)) {
		if l == s {
			n++
		}
	}
	return n
}
func countF(p, s string) int  { return len(fgrepLines(read(p), s)) }
func countE(p, re string) int { return len(grepLines(read(p), re)) }

// lastLine: 마지막 줄.
func lastLine(s string) string {
	l := lines(s)
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// nowS: date +%s.
func nowS() int64 { return time.Now().Unix() }
