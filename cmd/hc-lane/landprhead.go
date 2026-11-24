package main

// PR 머리 가지(2026-11-24 pr-surface, docs/hc-fill/pr.md · cmd/hc-merger/prhead.go): 병합기 안 착지(HERDR_MERGER=1)가 받은
// HERDR_LAND_PR_HEADS(「<레인>=<제출 SHA>=<본 머리 값>」 빈칸으로)의 구성원마다, 기본 가지를 올리기 직전에 rebase된 그 구성원의 끝
// 커밋을 refs/heads/pr/<레인>에 본 값 lease로 올린다 — 기본 가지가 그 커밋을 담는 순간 GitHub가 PR을 Merged로 표시한다.
// 끝 커밋은 신원(작성자·작성 시각·메시지 — rebase가 지키는 것)으로 착지 후보에서 찾는다. 못 찾거나 push가 실패해도 착지는 그대로
// 한다(한 줄 알림 — 병합기 done이 「PR 머리 안 들어감」을 사건 글에 적고 hc-fill이 그 PR을 까닭 댓글 뒤 닫는다).
// 다시 시도(다른 쪽 착지가 앞서 처음부터)는 올린 값을 본 값으로 이어 간다. 착지 판정·관문은 손대지 않는다.
// 저장소 착지 스크립트(.claude/herdr-land.sh)에 맡기는 착지는 이 길을 타지 않는다(Merged 표시 없음 — 문서).

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/gitx"
)

type prHead struct{ lane, sub, seen string }

var prh []prHead

// prHeadsTake: 환경 값을 읽고 지운다(검증 자식에 물려주지 않는다). 병합기 밖 착지·꼴이 틀린 항목은 버린다
func prHeadsTake() {
	v := os.Getenv("HERDR_LAND_PR_HEADS")
	os.Unsetenv("HERDR_LAND_PR_HEADS")
	prh = nil
	if os.Getenv("HERDR_MERGER") != "1" {
		return
	}
	for _, f := range strings.Fields(v) {
		p := strings.Split(f, "=")
		if len(p) == 3 && p[0] != "" && !strings.ContainsAny(p[0], ":~^ ") && gitx.ObjID(p[1]) && gitx.ObjID(p[2]) {
			prh = append(prh, prHead{p[0], p[1], p[2]})
		}
	}
}

const prSep, prEnd = "\x1f", "\x1e"

// prWho: 커밋 신원(작성자 이름·주소·작성 시각·메시지)
func prWho(rev string) (string, bool) {
	o, rc := gitOutQ("log", "-1", "--format=%an"+"%x1f%ae%x1f%at%x1f%B", rev)
	return o, rc == 0
}

// prHeadsPush <착지 후보>: 구성원마다 머리 가지를 올린다(기본 가지 push 앞)
func prHeadsPush(cand string) {
	if len(prh) == 0 {
		return
	}
	log, rc := gitOutQ("log", "-n", "500", "--format=%H%x1f%an%x1f%ae%x1f%at%x1f%B%x1e", cand)
	if rc != 0 {
		fmt.Println("hc-lane land: PR 머리 못 올림 — 착지 후보 기록을 못 읽었다(git log) — 착지는 그대로")
		return
	}
	who := map[string]string{} // 신원 → 가장 새 커밋
	for _, c := range strings.Split(log, prEnd) {
		c = strings.TrimLeft(c, "\n")
		h, rest, ok := strings.Cut(c, prSep)
		if ok && gitx.ObjID(h) {
			rest = strings.TrimRight(rest, "\n")
			if _, seen := who[rest]; !seen {
				who[rest] = h
			}
		}
	}
	for i, h := range prh {
		w, ok := prWho(h.sub)
		c := who[strings.TrimRight(w, "\n")]
		if !ok || c == "" {
			fmt.Printf("hc-lane land: PR 머리 못 올림 pr/%s — 착지 후보에서 제출 %s 의 끝 커밋을 못 찾았다 — 착지는 그대로\n", h.lane, cut(h.sub, 8))
			continue
		}
		if c == h.seen {
			continue
		}
		ref := "refs/heads/pr/" + h.lane
		if gitRun("push", "-q", "origin", c+":"+ref, "--force-with-lease="+ref+":"+h.seen) != 0 {
			fmt.Printf("hc-lane land: PR 머리 못 올림 pr/%s(push 거절 — 본 값 %s) — 착지는 그대로\n", h.lane, cut(h.seen, 8))
			continue
		}
		prh[i].seen = c
		fmt.Printf("hc-lane land: PR 머리 pr/%s ← %s\n", h.lane, cut(c, 9))
	}
}
