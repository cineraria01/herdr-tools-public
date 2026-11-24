package main

// PR 머리 가지(2026-11-24 pr-surface, docs/hc-fill/pr.md · cmd/hc-merger/prhead.go): 병합기 안 착지(HERDR_MERGER=1)가 받은
// HERDR_LAND_PR_HEADS(「<레인>=<제출 SHA>=<본 머리 값>」 빈칸으로)의 구성원마다, 기본 가지를 올리기 직전에 rebase된 그 구성원의 끝
// 커밋을 refs/heads/pr/<레인>에 본 값 lease로 올린다 — 기본 가지가 그 커밋을 담는 순간 GitHub가 PR을 Merged로 표시한다.
// 끝 커밋은 신원(작성자·작성 시각·메시지 — rebase가 지키는 것)으로 착지 후보에서 찾는다. 못 찾거나 push가 실패해도 착지는 그대로
// 한다(한 줄 알림 — 병합기 done이 「PR 머리 안 들어감」을 사건 글에 적고 hc-fill이 그 PR을 까닭 댓글 뒤 닫는다).
// 다시 시도(다른 쪽 착지가 앞서 처음부터)는 올린 값을 본 값으로 이어 간다. 착지 판정·관문은 손대지 않는다.
// 저장소 착지 스크립트(.claude/herdr-land.sh)에 맡기는 병합기 안 착지는 이 값을 그 스크립트에 그대로 넘기고 머리 가지는 스크립트가
// 올린다(계약 docs/hc-fill/pr.md 「착지 스크립트 계약」 — 도우미 hc-lane land-prheads가 같은 판정(prEnds)으로 push 인자를 찍는다).
// 계약을 구현하지 않은 스크립트면 지금처럼 Merged 표시 없음.

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/gitx"
)

func init() { register("land-prheads", landPRHeadsMain, noHelp) }

type prHead struct{ lane, sub, seen string }

var prh []prHead

// prHeadsTake: 환경 값을 읽고 지운다(검증 자식에 물려주지 않는다). 병합기 밖 착지·꼴이 틀린 항목은 버린다
func prHeadsTake() {
	v := os.Getenv("HERDR_LAND_PR_HEADS")
	os.Unsetenv("HERDR_LAND_PR_HEADS")
	prh = nil
	if os.Getenv("HERDR_MERGER") == "1" {
		prh = prParse(v)
	}
}

// prParse: 「<레인>=<제출 SHA>=<본 머리 값>」 빈칸으로 — 꼴이 틀린 항목은 버린다
func prParse(v string) []prHead {
	var o []prHead
	for _, f := range strings.Fields(v) {
		p := strings.Split(f, "=")
		if len(p) == 3 && p[0] != "" && !strings.ContainsAny(p[0], ":~^ ") && gitx.ObjID(p[1]) && gitx.ObjID(p[2]) {
			o = append(o, prHead{p[0], p[1], p[2]})
		}
	}
	return o
}

// prEnv: 착지 스크립트에 넘길 HERDR_LAND_PR_HEADS 값(받은 꼴 그대로 — 꼴이 맞는 항목만)
func prEnv(hs []prHead) string {
	o := make([]string, len(hs))
	for i, h := range hs {
		o[i] = h.lane + "=" + h.sub + "=" + h.seen
	}
	return strings.Join(o, " ")
}

const prSep, prEnd = "\x1f", "\x1e"

// prWho: 커밋 신원(작성자 이름·주소·작성 시각·메시지)
func prWho(rev string) (string, bool) {
	o, rc := gitOutQ("log", "-1", "--format=%an"+"%x1f%ae%x1f%at%x1f%B", rev)
	return o, rc == 0
}

// prEnds <착지 후보>: 구성원마다 후보 기록(최근 500개)에서 제출 SHA와 신원이 같은 가장 새 커밋(못 찾으면 빈 값).
// ok=false는 후보 기록을 못 읽음(모름). 내장 길(prHeadsPush)과 land-prheads가 같이 쓴다
func prEnds(cand string, hs []prHead) (ends []string, ok bool) {
	log, rc := gitOutQ("log", "-n", "500", "--format=%H%x1f%an%x1f%ae%x1f%at%x1f%B%x1e", cand)
	if rc != 0 {
		return nil, false
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
	ends = make([]string, len(hs))
	for i, h := range hs {
		if w, ok := prWho(h.sub); ok {
			ends[i] = who[strings.TrimRight(w, "\n")]
		}
	}
	return ends, true
}

// prSpec: 머리 가지를 c로 올리는 git push 인자 둘(refspec, 본 값 lease)
func prSpec(h prHead, c string) (refspec, lease string) {
	ref := "refs/heads/pr/" + h.lane
	return c + ":" + ref, "--force-with-lease=" + ref + ":" + h.seen
}

// prHeadsPush <착지 후보>: 구성원마다 머리 가지를 올린다(기본 가지 push 앞)
func prHeadsPush(cand string) {
	if len(prh) == 0 {
		return
	}
	ends, ok := prEnds(cand, prh)
	if !ok {
		fmt.Println("hc-lane land: PR 머리 못 올림 — 착지 후보 기록을 못 읽었다(git log) — 착지는 그대로")
		return
	}
	for i, h := range prh {
		c := ends[i]
		if c == "" {
			fmt.Printf("hc-lane land: PR 머리 못 올림 pr/%s — 착지 후보에서 제출 %s 의 끝 커밋을 못 찾았다 — 착지는 그대로\n", h.lane, cut(h.sub, 8))
			continue
		}
		if c == h.seen {
			continue
		}
		rs, ls := prSpec(h, c)
		if gitRun("push", "-q", "origin", rs, ls) != 0 {
			fmt.Printf("hc-lane land: PR 머리 못 올림 pr/%s(push 거절 — 본 값 %s) — 착지는 그대로\n", h.lane, cut(h.seen, 8))
			continue
		}
		prh[i].seen = c
		fmt.Printf("hc-lane land: PR 머리 pr/%s ← %s\n", h.lane, cut(c, 9))
	}
}

// landPRHeadsMain [<착지 후보>]: 저장소 착지 스크립트용 도우미 — 받은 HERDR_LAND_PR_HEADS와 후보(기본 HEAD)로 구성원마다
// 「<끝 커밋>:refs/heads/pr/<레인> --force-with-lease=refs/heads/pr/<레인>:<본 값>」 한 줄. 못 찾은 구성원은 건너뛰고 오류 출력에 한 줄,
// 이미 그 값인 구성원은 줄 없음. 종료 0, 후보·기록을 못 읽으면 1(모름 — 줄 없음), 인자 틀림 2(사용법)
func landPRHeadsMain(a []string) int {
	if len(a) > 1 || len(a) == 1 && (a[0] == "" || strings.HasPrefix(a[0], "-")) {
		usage()
	}
	cand := "HEAD"
	if len(a) == 1 {
		cand = a[0]
	}
	hs := prParse(os.Getenv("HERDR_LAND_PR_HEADS"))
	if len(hs) == 0 {
		return 0
	}
	c, rc := gitOutQ("rev-parse", "-q", "--verify", cand+"^{commit}")
	if rc != 0 || c == "" {
		eprintf("hc-lane land-prheads: 착지 후보 %s 를 읽지 못했다(git) — 줄 없음\n", cand)
		return 1
	}
	ends, ok := prEnds(c, hs)
	if !ok {
		eprintf("hc-lane land-prheads: 착지 후보 기록을 못 읽었다(git log) — 줄 없음\n")
		return 1
	}
	for i, h := range hs {
		switch e := ends[i]; {
		case e == "":
			eprintf("hc-lane land-prheads: pr/%s 건너뜀 — 착지 후보에서 제출 %s 의 끝 커밋을 못 찾았다\n", h.lane, cut(h.sub, 8))
		case e != h.seen:
			rs, ls := prSpec(h, e)
			fmt.Println(rs + " " + ls)
		}
	}
	return 0
}
