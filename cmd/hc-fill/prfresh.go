package main

// 새 PR은 살아 있는 레인의 새 보고에만(2026-11-24 pr-inbox-no-retro, docs/hc-fill/pr.md 「언제 열리나」): pr-inbox-lanes가 깔린 뒤 3분 안에
// 며칠 전에 끝난 레인 여섯에 PR이 열렸다 — 대기열 줄은 끝나도 「up」으로 남아(status의 「끝남」은 터미널이 없다는 뜻) opens가 600여 줄의
// 지난 보고를 다 훑었다. 이제 PR이 열려 있지 않은 레인은 (가) 그 이름 터미널이 이번 바퀴 표에 살아 있거나 병합 줄에 그 레인 제출이 서 있고
// (나) 그 보고가 이 기능이 그 프로젝트에 켜진 뒤(pr-on.tsv에 처음 본 때)에 온 것일 때만 연다. 모르면(표·병합 줄을 못 읽음) 이 바퀴는 열지
// 않고 다음 바퀴에 다시 본다. 이미 열린 PR의 뒤 사건(다시 완료·댓글·착지·닫기)은 그대로

import (
	"fmt"
	"os"
	"strings"
	"time"

	"herdr-tools/internal/events"
)

// prOnSince: pr-on.tsv(「프로젝트 종류 처음 본 때」)를 읽고 이번 바퀴에 켜진 것 중 처음 보는 것을 지금 시각으로 더한다. 종류 pr = 거울,
// inbox = 완료 없는 레인도(pr_inbox — pr과 함께 켜졌을 때만 적으니 그 시각은 pr 시각보다 앞서지 않는다). 꺼도 지우지 않는다 — 손잡이를 못
// 읽은 바퀴 하나가 시각을 늦춰 살아 있는 레인의 보고를 놓치지 않게(끈 동안 끝난 레인은 (가)가 막는다)
func prOnSince(projs []string) map[string]string {
	on := map[string]string{}
	s, _ := readFile(prFile("pr-on.tsv"))
	for _, l := range lines(s) {
		if i := strings.LastIndexByte(l, '\t'); i > 0 { // 「프로젝트\t종류」\t시각
			on[l[:i]] = l[i+1:]
		}
	}
	now := events.UTC(time.Now())
	kinds := []string{"pr"}
	if prInboxOn() {
		kinds = append(kinds, "inbox")
	}
	for _, p := range projs {
		for _, k := range kinds {
			if _, ok := on[p+"\t"+k]; ok {
				continue
			}
			on[p+"\t"+k] = now
			if err := appendLine(prFile("pr-on.tsv"), p+"\t"+k+"\t"+now); err != nil {
				fmt.Fprintf(os.Stderr, "hc-fill: PR 거울: %v\n", err)
			}
		}
	}
	return on
}

// fresh: 새 PR을 열어도 되나 — 보고가 켜진 뒤(나)이고 레인이 살아 있다(가). self = 완료 없는 레인(종류 inbox의 시각)
func (r *prRun) fresh(q Row, self bool, ev events.Event, t string) bool {
	p, kind := q.F(qProj), "pr"
	if self {
		kind = "inbox"
	}
	since, ok := r.on[p+"\t"+kind]
	if !ok || ev.Time < since {
		return false
	}
	if lalive(q.F(qAddr), q.F(qName)) {
		return true
	}
	return prQueued(p, q.F(qName), t)
}

// prQueued <프로젝트> <레인…>: 병합 줄(refs/merge-queue/<초>-<레인>)에 그 이름 제출이 서 있나. 줄을 못 읽으면 거짓(모름 — 열지 않고 다음 바퀴에)
func prQueued(p string, lanes ...string) bool {
	mqload(p)
	if inlist(p, MQBAD) {
		return false
	}
	for _, x := range MQS {
		if x.F(mqProj) != p {
			continue
		}
		if m := mqNameRe.FindStringSubmatch(x.F(mqRef)); m != nil && has(lanes, m[2]) {
			return true
		}
	}
	return false
}
