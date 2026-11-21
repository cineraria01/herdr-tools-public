package main

// 셸 판이 기대던 도구 의미 흉내: jq -r 출력·$(…) 잡기·bash `IFS=<한 글자> read -r`·macOS grep 문자 부류.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
)

// jqValues: jq처럼 입력의 JSON 값 흐름을 차례로 읽는다. err != nil이면 jq가 그 앞 값들의 출력을 낸 뒤 실패한 자리다.
func jqValues(in []byte) ([]any, error) {
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.UseNumber()
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
}

// jqPath: jq `.a.b` — null은 null로 이어지고, 객체도 null도 아닌 것을 고르면 jq 오류(false).
func jqPath(v any, keys ...string) (any, bool) {
	for _, k := range keys {
		switch x := v.(type) {
		case nil:
		case map[string]any:
			v = x[k]
		default:
			return nil, false
		}
	}
	return v, true
}

// jqTruthy: jq `//`가 대안으로 넘어가지 않는 값(null·false가 아님)
func jqTruthy(v any) bool { return v != nil && v != false }

// jqToString: jq tostring (문자열은 그대로, 나머지는 JSON 글)
func jqToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	}
	return jsonText(v, false)
}

// jqRaw: jq -r 한 값의 출력(줄바꿈 전). 객체·배열은 jq처럼 두 칸 들여 쓴 JSON
func jqRaw(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		return jsonText(v, true)
	}
	return jqToString(v)
}

func jsonText(v any, indent bool) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if indent {
		e.SetIndent("", "  ")
	}
	if err := e.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// jqCommand: 셸 판의 `cmd=$(jq -r '.tool_input.command // ""')` — ok=false는 jq가 실패해 `|| exit 0`으로 간 경우
func jqCommand(in []byte) (string, bool) {
	vals, err := jqValues(in)
	var out strings.Builder
	for _, v := range vals {
		c, ok := jqPath(v, "tool_input", "command")
		if !ok {
			return "", false
		}
		if !jqTruthy(c) {
			c = ""
		}
		out.WriteString(jqRaw(c))
		out.WriteByte('\n')
	}
	if err != nil {
		return "", false
	}
	return capture(out.String()), true
}

// capture: bash $(…)가 출력에 하는 일 — NUL을 버리고 끝 줄바꿈을 모두 뗀다
func capture(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\x00", ""), "\n")
}

// bashRead: bash 3.2 `IFS=<sep> read -r v1 … vn` 한 줄(줄바꿈 뺌)의 칸. sep은 공백이 아닌 한 글자.
// 마지막 칸은 나머지 전부(구분자 포함)지만, 나머지가 「한 낱말 + 구분자 하나」뿐이면 그 구분자는 뗀다(bash read 규칙).
func bashRead(line string, sep byte, n int) []string {
	f := make([]string, n)
	rest := line
	for i := 0; i < n-1; i++ {
		if j := strings.IndexByte(rest, sep); j >= 0 {
			f[i], rest = rest[:j], rest[j+1:]
		} else {
			f[i], rest = rest, ""
		}
	}
	if j := strings.IndexByte(rest, sep); j >= 0 && j == len(rest)-1 {
		rest = rest[:j]
	}
	f[n-1] = rest
	return f
}

// heredocLines: `while read …; done <<EOF\n$X\nEOF` 가 읽는 줄들(X 끝 줄바꿈은 그대로, here-doc이 하나를 더한다)
func heredocLines(x string) []string { return strings.Split(x, "\n") }

// asciiLower: C 로캘 tolower(awk·bash 패턴의 [Ll] 따위) — 영문만
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// grepAlnum·grepSpace: macOS grep -E의 [[:alnum:]]·[[:space:]] (로캘과 상관없이 UTF-8로 읽는다 — 2026-11-21 전수 확인)
func grepAlnum(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
	i := sort.Search(len(grepAlnumHigh), func(i int) bool { return grepAlnumHigh[i][1] >= r })
	return i < len(grepAlnumHigh) && grepAlnumHigh[i][0] <= r
}

func grepSpace(r rune) bool {
	if r < 0x80 {
		return r == ' ' || r == '\t' || r == '\v' || r == '\f' || r == '\r' || r == '\n'
	}
	for _, s := range grepSpaceHigh {
		if s == r {
			return true
		}
	}
	return false
}
