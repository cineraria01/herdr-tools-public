package main

// 미리 줄(preverify, 2026-11-24): 「hc-lane report 완료 <sha>」 뒤 — 병합기가 켜졌고(원격 refs/herdr/merger/*) 추측 검증 기계 목록
// (refs/herdr/spec-verifiers)이 있는 저장소면 refs/merge-pre/<레인> → 표식 커밋(부모 = 그 sha, 트리 = 그 트리, 제목 verified.PreMsg)을
// 올린다. 병합기가 감사 동안 빈 검증 기계에서 「기준 + 이 커밋」을 미리 검증해 둔다(cmd/hc-merger/specpre.go). 제출 큐와 다른 이름
// 공간이라 착지를 일으키지 않는다. 사건은 이미 적었다 — 여기서 실패해도 보고는 성공(제출 뒤 병합기가 지금처럼 검증한다)

import (
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/verified"
)

// prePost: 작업 트리 안에서 자기 레인 완료를 적은 뒤에만 부른다(sha는 reportSHA가 푼 전체 해시)
func prePost(lane, sha string) {
	if !lnameOK(lane) || mreserved(lane) || len(sha) != 40 {
		return
	}
	remote, b, ok := repoDefaults(".")
	if !remote || !ok || lane == b {
		return
	}
	o, rc := gitOutQ("ls-remote", "origin", "refs/herdr/merger/*", verified.VerifiersRef)
	if rc != 0 {
		eprintf("hc-lane report: 경고 — 원격을 읽지 못해 미리 검증 표식을 올리지 않았다(제출 뒤 병합기가 지금처럼 검증한다)\n")
		return
	}
	if !strings.Contains(o, "\trefs/herdr/merger/") || !strings.Contains(o, "\t"+verified.VerifiersRef) {
		return // 병합기·추측 검증이 꺼진 저장소 — 지금 그대로
	}
	mc, rc := gitOut2([]string{"GIT_AUTHOR_NAME=hc-lane", "GIT_AUTHOR_EMAIL=hc-lane@localhost",
		"GIT_COMMITTER_NAME=hc-lane", "GIT_COMMITTER_EMAIL=hc-lane@localhost"},
		"commit-tree", sha+"^{tree}", "-p", sha, "-m", verified.PreMsg(lane, b, time.Now().Unix()))
	if rc != 0 || len(mc) != 40 {
		eprintf("hc-lane report: 경고 — 미리 검증 표식 커밋을 만들지 못했다: %s\n", cut(mc, 200))
		return
	}
	ref := verified.PreRefs + lane
	if o, rc := gitOut2(nil, "push", "-q", "origin", "+"+mc+":"+ref); rc != 0 {
		eprintf("hc-lane report: 경고 — 미리 검증 표식(%s)을 올리지 못했다(제출 뒤 병합기가 지금처럼 검증한다): %s\n", ref, cut(o, 200))
		return
	}
	fmt.Printf("hc-lane report: 미리 검증 표식 %s → %s (감사 동안 병합기가 빈 검증 기계에서 미리 검증 — 착지·제출과는 무관)\n", ref, cut(sha, 12))
}
