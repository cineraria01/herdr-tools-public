package main

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
)

// 지휘 기계 파일 붙이기의 비밀 꼴 판정(2026-11-22 go-secret-fp — 감사 보고서의 코드 글 「token = os.environ…」이
// 옛 이름=값 꼴에 걸려 gofix-fill이 안 떴다). 막는 것은 둘뿐:
//  1. 알려진 토큰 머리(어디에 있든)
//  2. 비밀스러운 이름(TOKEN·SECRET·KEY·PASSWORD·AUTH·COOKIE·SESSION… 대소문자 무관) = 무작위해 보이는 값
//     (영숫자가 16자 넘게 이어지고, 글자와 숫자가 섞이고, 엔트로피 3비트/자 이상)
//
// 그래서 `mode=block`·`HC_FILL_IMPL=bash`·`token = os.environ[…]`·`AUTH_FILE=~/.config/x.json` 같은 평범한 글은 지나간다.
var lcSecretHeads = []struct {
	name string
	re   *regexp.Regexp
}{
	{"개인 키", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"sk-", regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])sk-[A-Za-z0-9_-]{20,}`)}, // 「task-sk-…」 같은 낱말 속은 뺀다
	{"ghp_", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`)},
	{"github_pat_", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`)},
	{"AKIA", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"xox", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`)},
	{"AIza", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`)},
	{"eyJ(JWT)", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`)},
}

// 이름(비밀스러운 낱말을 품은 식별자) [따옴표] =|: [따옴표] [Bearer·Basic ] 값
var lcSecretNV = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:token|secret|key|passw(?:or)?d|pwd|auth|cookie|session|credential)[A-Za-z0-9_.-]*)["']?\s*[:=]\s*["']?(?:(?:bearer|basic)\s+)?([A-Za-z0-9+/=_.~-]+)`)

var lcAlnumRun = regexp.MustCompile(`[A-Za-z0-9]+`)

// lcRandomish: 값이 무작위 토큰 꼴인가 — 가장 긴 영숫자 줄기가 16자 이상이고 글자·숫자가 다 들고 엔트로피 3비트/자 이상
func lcRandomish(v []byte) bool {
	var run []byte
	for _, r := range lcAlnumRun.FindAll(v, -1) {
		if len(r) > len(run) {
			run = r
		}
	}
	if len(run) < 16 {
		return false
	}
	if !bytes.ContainsAny(run, "0123456789") || bytes.IndexFunc(run, func(r rune) bool { return r > '9' }) < 0 {
		return false
	}
	var n [256]int
	for _, c := range run {
		n[c]++
	}
	h := 0.0
	for _, k := range n {
		if k > 0 {
			p := float64(k) / float64(len(run))
			h -= p * math.Log2(p)
		}
	}
	return h >= 3.0
}

// lcSecretHit: 첫 비밀 꼴의 꼴 이름·줄 번호(1부터)·이름=값의 이름. 값은 돌려주지 않는다. 없으면 kind ""
func lcSecretHit(data []byte) (kind string, line int, name string) {
	for i, l := range bytes.Split(data, []byte("\n")) {
		for _, s := range lcSecretHeads {
			if s.re.Match(l) {
				return s.name, i + 1, ""
			}
		}
		for _, m := range lcSecretNV.FindAllSubmatch(l, -1) {
			if lcRandomish(m[2]) {
				nm := string(m[1])
				if len(nm) > 40 {
					nm = nm[:40] + "…"
				}
				return "이름=값", i + 1, nm
			}
		}
	}
	return "", 0, ""
}

// lcSecretWhy: 막는 까닭(값은 찍지 않는다)
func lcSecretWhy(kind string, line int, name string) string {
	w := fmt.Sprintf("비밀 꼴(%s)을 담았다 — %d번째 줄", kind, line)
	if name != "" {
		w += " 이름 " + name
	}
	return w
}
