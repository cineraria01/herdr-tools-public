package main

// 따로 바뀐 바이너리(sync-bin-conflict, 2026-11-23 — 지휘가 손으로 옛 릴리스로 되돌린 hc-fill·hc-lane을 다음 herdr-sync가 다시 덮어
// hc-fill이 또 멈췄다): 기계의 ~/.local/bin/<도구>가 bin.tsv에 적힌 「마지막으로 깔아 준 sha」와 다르면 installSh가 덮지 않고 C(충돌)를 낸다.
// --take <기계>:.local/bin/<도구>(또는 <기계>:*)면 덮고, 덮기 전 판은 그 기계 ~/.local/state/herdr-sync/bak/<시각>/.local/bin/<도구>에.

import "strings"

// binKey: sync.list 바이너리 줄의 맥 경로(.local/bin/<도구>)인가 — --take 경로로 받는다
func binKey(k string) bool {
	for _, b := range BINS {
		if k == ".local/bin/"+b.tool {
			return true
		}
	}
	return false
}

// takeSh: 그 기계에서 --take로 집은 도구(충돌이어도 덮음)와 백업 시각을 installSh 앞 변수 줄로. 없으면 빈 글
func takeSh(m string, tools []string) string {
	var tk []string
	for _, t := range tools {
		if spec(takes, m, ".local/bin/"+t) {
			tk = append(tk, strings.ReplaceAll(t, "'", `'\''`))
		}
	}
	if len(tk) == 0 {
		return ""
	}
	return "take=' " + strings.Join(tk, " ") + " ' stamp=" + STAMP + "\n"
}
