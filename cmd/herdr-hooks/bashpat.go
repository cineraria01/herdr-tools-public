package main

// bash 패턴 맞추기(case 패턴·글롭 한 칸)와 글롭 펼치기(nullglob, 숨은 파일은 패턴이 .으로 시작할 때만) — cotenant-guard의
// `case $f in $1|$1/*)`(*가 /도 넘는다)와 `shopt -s nullglob; m=($a)`를 흉내 낸다. 글자 단위(UTF-8 로캘).

import (
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// bashMatch: s가 bash 패턴 p에 통째로 맞는가 (* ? [..] [!..] [^..] [:class:] \x)
func bashMatch(s, p string) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if p == "" {
				return true
			}
			for i := 0; i <= len(s); {
				if bashMatch(s[i:], p) {
					return true
				}
				if i == len(s) {
					break
				}
				_, w := utf8.DecodeRuneInString(s[i:])
				i += w
			}
			return false
		case '?':
			if s == "" {
				return false
			}
			_, w := utf8.DecodeRuneInString(s)
			s, p = s[w:], p[1:]
		case '[':
			if s == "" {
				return false
			}
			r, w := utf8.DecodeRuneInString(s)
			ok, n, valid := bracket(p, r)
			if !valid { // 닫는 ]가 없으면 [는 글자 그대로
				if s[0] != '[' {
					return false
				}
				s, p = s[1:], p[1:]
				continue
			}
			if !ok {
				return false
			}
			s, p = s[w:], p[n:]
		case '\\':
			if len(p) > 1 {
				p = p[1:]
			}
			fallthrough
		default:
			r, w := utf8.DecodeRuneInString(p)
			if s == "" {
				return false
			}
			r2, w2 := utf8.DecodeRuneInString(s)
			if r != r2 {
				return false
			}
			s, p = s[w2:], p[w:]
		}
	}
	return s == ""
}

// bracket: p[0]=='['인 묶음이 r에 맞는가, 묶음 길이, 묶음이 닫혔는가
func bracket(p string, r rune) (match bool, n int, valid bool) {
	i := 1
	neg := false
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
		neg = true
		i++
	}
	first := true
	for i < len(p) {
		if p[i] == ']' && !first {
			return match != neg, i + 1, true
		}
		first = false
		if strings.HasPrefix(p[i:], "[:") {
			if e := strings.Index(p[i+2:], ":]"); e >= 0 {
				if charClass(p[i+2:i+2+e], r) {
					match = true
				}
				i += e + 4
				continue
			}
		}
		lo, w := utf8.DecodeRuneInString(p[i:])
		if lo == '\\' && i+1 < len(p) {
			i++
			lo, w = utf8.DecodeRuneInString(p[i:])
		}
		i += w
		hi := lo
		if i+1 < len(p) && p[i] == '-' && p[i+1] != ']' {
			i++
			hi, w = utf8.DecodeRuneInString(p[i:])
			if hi == '\\' && i+1 < len(p) {
				i++
				hi, w = utf8.DecodeRuneInString(p[i:])
			}
			i += w
		}
		if lo <= r && r <= hi {
			match = true
		}
	}
	return false, 0, false
}

func charClass(name string, r rune) bool {
	switch name {
	case "alpha":
		return unicode.IsLetter(r)
	case "digit":
		return r >= '0' && r <= '9'
	case "alnum":
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	case "upper":
		return unicode.IsUpper(r)
	case "lower":
		return unicode.IsLower(r)
	case "space":
		return unicode.IsSpace(r)
	case "blank":
		return r == ' ' || r == '\t'
	case "punct":
		return unicode.IsPunct(r) || unicode.IsSymbol(r)
	case "xdigit":
		return strings.ContainsRune("0123456789abcdefABCDEF", r)
	case "cntrl":
		return unicode.IsControl(r)
	case "print":
		return unicode.IsPrint(r)
	case "graph":
		return unicode.IsGraphic(r) && !unicode.IsSpace(r)
	case "word":
		return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return false
}

// hasGlob: bash가 글롭 패턴으로 보는가 — 이스케이프 안 된 * ? 또는 [ 뒤에 ] (닫히지 않은 [는 패턴이 아니다)
func hasGlob(s string) bool {
	open := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '*', '?':
			return true
		case '[':
			open = true
		case ']':
			if open {
				return true
			}
		}
	}
	return false
}

func dequote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// bashGlob: nullglob 펼치기 한 낱말 — 패턴이 아니면 그대로, 맞는 것이 없으면 빈 목록
func bashGlob(w string) []string {
	if !hasGlob(w) {
		return []string{w}
	}
	parts := strings.Split(w, "/")
	start := ""
	if strings.HasPrefix(w, "/") {
		start, parts = "/", parts[1:]
	}
	out := globWalk(start, parts, false)
	sort.Strings(out)
	return out
}

func globWalk(base string, parts []string, globbed bool) []string {
	if len(parts) == 0 {
		if _, err := os.Lstat(base); globbed && err != nil {
			return nil
		}
		return []string{base}
	}
	p, rest := parts[0], parts[1:]
	join := func(name string) string {
		if base == "" || strings.HasSuffix(base, "/") {
			return base + name
		}
		return base + "/" + name
	}
	if p == "" { // 겹친 / 나 끝 /
		next := base
		if !strings.HasSuffix(next, "/") {
			next += "/"
		}
		return globWalk(next, rest, globbed)
	}
	if !hasGlob(p) {
		return globWalk(join(dequote(p)), rest, globbed)
	}
	dir := base
	if dir == "" {
		dir = "."
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := []string{}
	if strings.HasPrefix(p, ".") {
		names = append(names, ".", "..")
	}
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(p, ".") {
			continue
		}
		if !bashMatch(n, p) {
			continue
		}
		full := join(n)
		if len(rest) > 0 && !isDir(full) {
			continue
		}
		out = append(out, globWalk(full, rest, true)...)
	}
	return out
}
