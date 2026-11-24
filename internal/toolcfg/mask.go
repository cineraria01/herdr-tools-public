package toolcfg

// 개인 값 가리기(2026-11-24 pr-surface): lint와 같은 무늬 파일(private-patterns.txt)·같은 바이트 규칙(byteRunes, 대소문자 무시)으로
// 걸린 곳을 「‹가림›」으로 바꾼다. 무늬를 못 읽으면(파일 없음·패턴 줄 없음·틀린 패턴) 오류 — 부른 쪽은 올리지 않는다(fail-closed)

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Hidden: 가린 자리 글
const Hidden = "‹가림›"

// Masker: 무늬 파일(dir/private-patterns.txt, 없으면 ~/.local/share/herdr-tools/private-patterns.txt)을 읽어 가리는 함수
func Masker(dir string) (func(string) string, error) {
	pf := dir + "/private-patterns.txt"
	if !isFile(pf) {
		pf = os.Getenv("HOME") + "/.local/share/herdr-tools/private-patterns.txt"
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return nil, fmt.Errorf("개인 값 패턴 파일을 읽지 못함: %w", err)
	}
	var res []*regexp.Regexp
	for _, p := range splitLines(data) {
		s := strings.TrimSuffix(string(p), "\r")
		if t := strings.TrimLeft(s, " \t\v\f\r"); t == "" || t[0] == '#' {
			continue
		}
		re, err := compileERE(s, true)
		if err != nil {
			return nil, fmt.Errorf("틀린 패턴 %s (%s)", s, pf)
		}
		res = append(res, re)
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("개인 값 패턴 줄 없음(%s)", pf)
	}
	return func(s string) string {
		s = strings.ReplaceAll(s, "\x00", "") // 가린 자리 표시와 겹치지 않게
		b := byteRunes([]byte(s))
		hit := false
		for _, re := range res {
			if re.MatchString(b) {
				b, hit = re.ReplaceAllLiteralString(b, "\x00"), true
			}
		}
		if !hit {
			return s
		}
		var o strings.Builder // byteRunes를 되돌린다(U+E080~U+E0FF → 바이트), 가린 자리는 Hidden
		for _, r := range b {
			switch {
			case r == 0:
				o.WriteString(Hidden)
			case r >= 0xE080 && r <= 0xE0FF:
				o.WriteByte(byte(r - 0xE000))
			default:
				o.WriteRune(r)
			}
		}
		return o.String()
	}, nil
}
