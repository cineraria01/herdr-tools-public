package main

// 감사 보고에서 [낮음] 지적 원문 뽑기(go-lows-followup) — 보고 꼴은 감사마다 조금씩 다르다(2026-11-22 실제 보고 26개):
//   ### [낮음] 1. 제목 …(본문은 다음 같은·윗 단계 제목까지)   ### [낮음]1 제목
//   ## 낮음 1 — 제목   ### 낮음 2 · 파일:줄 — 제목
//   1. **낮음 · `파일:줄`** — …   - [낮음]1 …(목록 꼴 — 이어지는 줄은 들여 쓴 줄)
// 제목 꼴이 하나라도 있으면 제목 꼴만 쓴다(목록 꼴 「- [낮음]1 … 반영 확인」은 앞 차수 지적 확인일 때가 많다).
// 뽑은 수가 판정 줄의 낮음 수와 다르면 쓰지 않는다(lowsFor) — 꼴을 잘못 읽은 것으로 본다.

import (
	"regexp"
	"strings"
)

var (
	lpHeadRe  = regexp.MustCompile(`^(#{1,6})[ \t]+(.*)$`)
	lpItemRe  = regexp.MustCompile(`^([ \t]*)(?:[-*+]|[0-9]+[.)])[ \t]+(.*)$`)
	lpGradeRe = regexp.MustCompile(`^(?:\*\*)?\[?낮음(?:\]|\*\*|[ \t]*[0-9]|[ \t]*[·:|—–-])`)
	lpFenceRe = regexp.MustCompile("^[ \t]*(```|~~~)")
)

// lowsItems: 보고 글에서 [낮음] 지적마다 원문(제목·첫 줄부터 그 지적 끝까지, 끝 빈 줄 뗌)
func lowsItems(text string) []string {
	ls := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// 코드 울타리 안 줄은 제목·목록으로 보지 않는다
	fenced := make([]bool, len(ls))
	in := false
	for i, l := range ls {
		if lpFenceRe.MatchString(l) {
			in = !in
			fenced[i] = true
			continue
		}
		fenced[i] = in
	}
	heading := func(i int) int { // 제목 단계(아니면 0)
		if fenced[i] {
			return 0
		}
		if m := lpHeadRe.FindStringSubmatch(ls[i]); m != nil {
			return len(m[1])
		}
		return 0
	}
	var heads, items []string
	for i, l := range ls {
		if fenced[i] {
			continue
		}
		if lv := heading(i); lv > 0 {
			if !lpGradeRe.MatchString(lpHeadRe.FindStringSubmatch(l)[2]) {
				continue
			}
			j := i + 1
			for ; j < len(ls); j++ {
				if h := heading(j); h > 0 && h <= lv {
					break
				}
			}
			heads = append(heads, lpBlock(ls[i:j]))
			continue
		}
		m := lpItemRe.FindStringSubmatch(l)
		if m == nil || !lpGradeRe.MatchString(m[2]) {
			continue
		}
		ind := lpIndent(m[1])
		j := i + 1
		for ; j < len(ls); j++ {
			if heading(j) > 0 {
				break
			}
			if !fenced[j] && strings.TrimSpace(ls[j]) != "" && lpIndent(lpLead(ls[j])) <= ind {
				break
			}
		}
		items = append(items, lpBlock(ls[i:j]))
	}
	if len(heads) > 0 {
		return heads
	}
	return items
}

func lpLead(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \t"))] }

// lpIndent: 들여쓰기 폭(탭은 4)
func lpIndent(s string) int {
	n := 0
	for _, c := range s {
		if c == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return n
}

func lpBlock(ls []string) string { return strings.TrimRight(strings.Join(ls, "\n"), " \t\n") }
