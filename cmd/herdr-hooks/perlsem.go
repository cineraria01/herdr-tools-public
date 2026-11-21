package main

// 셸 판 perl 정규식(바이트 문자열, use utf8 없음: \w \s \d는 ASCII, \s에 \v 포함)을 Go regexp로 흉내 낸다.
// Go regexp는 앞뒤 살피기(lookaround)·되참조가 없어, 뒤 살피기는 시작 자리를 하나씩 옮기며 직접 보고(perlFind),
// 앞 살피기는 그 뒤를 받는 꼴로 정규식에 풀어 넣었다. Go regexp는 perl과 같은 왼쪽 우선(leftmost-first) 선택을 한다.

import (
	"regexp"
	"strings"
)

const (
	reS  = `[\t\n\x0B\f\r ]`  // perl \s
	reNS = `[^\t\n\x0B\f\r ]` // perl \S
)

func perlWord(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_'
}

func perlSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

// lbW: (?<![\w.\/-]) — 앞 글자가 이 부류가 아니면 참
func lbW(prev byte) bool { return !(perlWord(prev) || prev == '.' || prev == '/' || prev == '-') }

// perlFind: perl의 /g 찾기 한 번 — from부터 시작 자리를 하나씩 옮기며, 뒤 살피기 lb(앞 바이트)를 통과한 자리에서
// \A로 묶은 re가 맞는 첫 자리. 돌려주는 위치는 s 전체 기준(FindStringSubmatchIndex 꼴), 없으면 nil
func perlFind(s string, from int, lb func(byte) bool, re *regexp.Regexp) []int {
	for st := from; st <= len(s); st++ {
		if lb != nil && st > 0 && !lb(s[st-1]) {
			continue
		}
		if loc := re.FindStringSubmatchIndex(s[st:]); loc != nil {
			for i := range loc {
				if loc[i] >= 0 {
					loc[i] += st
				}
			}
			return loc
		}
	}
	return nil
}

// heredocStrip: s/(<<-?[ \t]*([\x27"]?)(\w+)\2[^\n]*)\n(?:.*?\n)?[ \t]*\3[ \t]*(?=\n|$)/$1/gs
// — heredoc 본문을 지우고 여는 줄은 남긴다. perl 되짚기 순서 그대로: 낱말은 긴 것부터, 본문은 한 줄 이상을 짧은 것부터, 맨 끝에 본문 없음.
func heredocStrip(s string) string {
	var b strings.Builder
	pos := 0
	for st := 0; st+1 < len(s); {
		if s[st] != '<' || s[st+1] != '<' {
			st++
			continue
		}
		if eol, end, ok := heredocAt(s, st); ok {
			b.WriteString(s[pos:eol])
			pos, st = end, end
			continue
		}
		st++
	}
	b.WriteString(s[pos:])
	return b.String()
}

// heredocAt: st의 << 에서 맞으면 여는 줄 끝(eol, \n 자리)과 맞은 끝(end)
func heredocAt(s string, st int) (eol, end int, ok bool) {
	j := st + 2
	if j < len(s) && s[j] == '-' {
		j++
	}
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	var q byte
	if j < len(s) && (s[j] == '\'' || s[j] == '"') {
		q = s[j]
		j++
	}
	wend := j
	for wend < len(s) && perlWord(s[wend]) {
		wend++
	}
	for l := wend; l > j; l-- {
		k := l
		if q != 0 { // 따옴표가 있으면 낱말 전체 뒤에 같은 따옴표만 (더 짧은 낱말 뒤는 낱말 글자라 따옴표가 아니다)
			if l != wend || k >= len(s) || s[k] != q {
				continue
			}
			k++
		}
		nl := strings.IndexByte(s[k:], '\n')
		if nl < 0 {
			return 0, 0, false
		}
		eol = k + nl
		w := s[j:l]
		for p := eol + 1; ; {
			n2 := strings.IndexByte(s[p:], '\n')
			if n2 < 0 {
				break
			}
			t := p + n2 + 1
			if e, ok := heredocTerm(s, t, w); ok {
				return eol, e, true
			}
			p = t
		}
		if e, ok := heredocTerm(s, eol+1, w); ok {
			return eol, e, true
		}
	}
	return 0, 0, false
}

// heredocTerm: t에서 [ \t]*w[ \t]* 뒤가 \n이나 끝이면 그 끝
func heredocTerm(s string, t int, w string) (int, bool) {
	if t > len(s) {
		return 0, false
	}
	i := t
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if !strings.HasPrefix(s[i:], w) {
		return 0, false
	}
	i += len(w)
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i == len(s) || s[i] == '\n' {
		return i, true
	}
	return 0, false
}

var (
	perlSQ      = regexp.MustCompile(`'[^']*'`)
	perlDQ      = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	perlComment = regexp.MustCompile(`(^|` + reS + `)#[^\n]*`)
	perlDQSub   = regexp.MustCompile("\\$\\(|`")
)

// quoteStrip: 작은따옴표 → ” , 큰따옴표 → ""(안에 $( 나 ` 가 있으면 실행되니 남김), 주석 지움 — 셸 판 perl의 세 줄 그대로
func quoteStrip(s string) string {
	s = perlSQ.ReplaceAllString(s, "''")
	s = perlDQ.ReplaceAllStringFunc(s, func(m string) string {
		if perlDQSub.MatchString(m[1 : len(m)-1]) {
			return m
		}
		return `""`
	})
	return perlComment.ReplaceAllString(s, "${1}")
}

// perlFields: grep { length } split /\s+/
func perlFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r < 0x80 && perlSpace(byte(r)) })
}
