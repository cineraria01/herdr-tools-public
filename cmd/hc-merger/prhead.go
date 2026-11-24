package main

// PR 머리 가지(2026-11-24 pr-surface, docs/hc-fill/pr.md): hc-fill PR 거울이 레인 완료 때 연 PR의 머리 가지 refs/heads/pr/<레인>.
// 병합기는 그 가지가 있는 묶음 구성원만 착지 자식(hc-lane land)에 「레인=제출 SHA=본 머리 값」으로 넘기고(HERDR_LAND_PR_HEADS),
// 착지가 기본 가지를 올리기 직전에 rebase된 그 커밋을 머리 가지에 본 값 lease로 올린다(cmd/hc-lane/landprhead.go) — 기본 가지가
// 그 커밋을 담는 순간 GitHub가 PR을 Merged로 표시한다. 착지 판정·관문(트리 대조·patch-id)은 손대지 않는다.
// 레인 가지 lane/<레인>을 머리로 쓰지 않는 까닭: 착지 push가 그것을 제출 SHA lease로 함께 지운다(exbuild) — 머리로 옮기면 그 lease가
// 깨져 착지가 거절되고, 지우면 GitHub가 PR을 Merged가 아니라 닫힘으로 본다.
// 착지 뒤 done은 그 머리가 착지 커밋에 들었는지를 착지 사건 글 끝에 적는다(「 · PR 머리 <sha8> 들어감」·「 · PR 머리 안 들어감(…)」) —
// hc-fill이 그것으로 Merged인지 닫을지를 정한다. 머리 가지가 없으면 글이 전과 같다.

import "strings"

const prHeadPat = "refs/heads/pr/*"

func prRef(lane string) string { return "refs/heads/pr/" + lane }

// prHeads: 「<레인>=<제출 SHA>=<본 머리 값>」를 빈칸으로(머리 가지가 있는 구성원만)
func (m *M) prHeads() string {
	var o []string
	for _, e := range m.b {
		if h := refsha(m.qs, prRef(e.l)); h != "" {
			o = append(o, e.l+"="+e.s+"="+h)
		}
	}
	return strings.Join(o, " ")
}

// prNote <ls-remote 출력> <레인> <착지 커밋>: 착지 사건 글 끝 — 머리 가지가 없으면 빈 값
func prNote(rr, ln, C string) string {
	h := refsha(rr, prRef(ln))
	if h == "" {
		return ""
	}
	if runQ("merge-base", "--is-ancestor", h, C) == 0 { // 없는 객체·오류는 「안 들어감」(모름을 Merged로 치지 않는다)
		return " · PR 머리 " + first8(h) + " 들어감"
	}
	return " · PR 머리 안 들어감(가지 갱신 실패 — Merged 표시 없음)"
}
