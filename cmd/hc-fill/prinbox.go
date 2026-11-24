package main

// 「완료」 없이 제출하는 레인의 PR(2026-11-24 pr-inbox-lanes, docs/hc-fill/pr.md — 사용자 「그냥 피알 기능 켜면 바로 사용 되어야 하는거
// 아니야 ?」): 받은편지 레인·--no-audit 레인(감사 칸이 auto가 아닌 줄)은 자기 지시문대로 감사하고 「중간 <sha> …」 → 「제출 <sha>」로 끝나
// 완료 보고가 없어 PR이 안 열렸다. 그 레인은 커밋 SHA가 든 마지막 보고(중간·완료·제출)로 연다 — 첫 보고면 열기, SHA가 바뀌면 다시 완료,
// 제출 때 받은편지 레인의 자기 감사 글을 리뷰로 한 번. 손잡이 pr_inbox(기본 1, 0이면 그 레인들도 지금처럼 완료만)

import (
	"regexp"
	"strings"

	"herdr-tools/internal/events"
)

// prInboxOn: 손잡이 pr_inbox(바퀴마다 새로 읽는다)
func prInboxOn() bool { return knobInt("pr_inbox") != 0 }

// prLastSHA <레인> <주소>: 그 실행에서 커밋 SHA가 든 마지막 보고(중간·완료·제출). 실행 키를 확정했으면 그 실행만, 아니면 주소·이름으로
func prLastSHA(n, addr string) string {
	run, t := attemptRun(n, addr), ""
	if run != "" {
		t = auRunStart(run)
	}
	l := ""
	for _, x := range EV {
		e := events.Cols(x)
		if e.Lane != n || e.Term != addr || len(e.SHA) != 40 || (e.State != "중간" && e.State != "완료" && e.State != "제출") {
			continue
		}
		if run != "" {
			if e.Run != "" && e.Run != "-" {
				if e.Run != run {
					continue
				}
			} else if e.Time < t {
				continue
			}
		}
		l = x
	}
	return l
}

var prSelfAuditRe = regexp.MustCompile(`감사 (완료|통과)`)

// selfAudit: 받은편지 레인(자기 감사)의 제출 — 그 레인이 제출 전에 적은 마지막 감사 글(「감사 완료·통과」가 든 중간·완료 보고)을 리뷰(승인)로
// 한 번(작성자가 다른 계정일 때 진짜 리뷰 — 판정과 같은 규칙). 글이 없으면 승인 없이 댓글 한 줄. --no-audit·대기열(auto) 레인은 아무것도 안 함
func (r *prRun) selfAudit(e *prEnt, lane string, ev events.Event, key string) {
	if !prInboxOn() {
		return
	}
	self := false
	for _, q := range qRows() {
		if q.F(qName) == lane {
			_, _, inbox := replyOf(q.F(qReply))
			a := q.F(qAudit)
			self = inbox && a != "auto" && a != "no-audit" && !strings.HasPrefix(a, "audit=")
		}
	}
	if !self {
		return
	}
	msg := ""
	for _, x := range EV { // 그 실행(같은 터미널)이 PR을 연 뒤 제출 전까지 적은 것만 — 같은 이름의 앞 실행 글을 집지 않는다(감사 1차 낮음)
		if a := events.Cols(x); a.Lane == lane && a.Term == ev.Term && (a.State == "중간" || a.State == "완료") && a.Time >= e.Opened && a.Time <= ev.Time && prSelfAuditRe.MatchString(a.Msg) {
			msg = a.Msg
		}
	}
	id := ev.SubmitID()
	if msg == "" {
		r.comment(e, key+":self", "자기 감사 판정을 대기열 기계에서 못 읽음 — 보고서는 레인 기계 "+ev.Machine+" (제출 "+id+", "+head(ev.SHA, 8)+")")
		return
	}
	if !prSelfClean(msg) { // 높음·중간이 0으로 읽히지 않으면 승인하지 않고 글만
		r.comment(e, key+":self", "자기 감사(제출 "+id+", "+head(ev.SHA, 8)+") — "+msg+"\n\n높음·중간 0을 읽지 못해 승인하지 않는다 · 보고서는 레인 기계 "+ev.Machine)
		return
	}
	r.review(e, key+":self", true, "자기 감사 통과(제출 "+id+", "+head(ev.SHA, 8)+") — "+msg+"\n\n보고서는 레인 기계 "+ev.Machine)
}

var prHighRe, prMidRe = regexp.MustCompile(`높음 ([0-9]+)`), regexp.MustCompile(`중간 ([0-9]+)`)

// prSelfClean <감사 글>: 「높음 0」과 「중간 0」이 둘 다 읽히나
func prSelfClean(msg string) bool {
	h, m := prHighRe.FindStringSubmatch(msg), prMidRe.FindStringSubmatch(msg)
	return h != nil && m != nil && h[1] == "0" && m[1] == "0"
}
