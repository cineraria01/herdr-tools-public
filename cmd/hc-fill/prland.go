package main

// 착지 뒤 Merged를 원격 사실로(2026-11-24 pr-inbox-lanes 지휘 추가 지시 8): 병합기 글 「PR 머리 … 들어감」만 보고 merged로 적었는데,
// 착지 스크립트가 머리와 기준 가지를 같은 push로 올린 저장소에서 GitHub는 그 PR을 닫았다(Closed — 로그·pr.tsv만 merged).
// 그래서 gh로 그 PR의 실제 상태를 한 번 읽는다 — MERGED면 지금처럼, CLOSED면 closed와 한 줄 댓글, OPEN(아직 반영 전)이면 다음 바퀴에 다시

import "herdr-tools/internal/events"

// landedState: 병합기가 머리 가지를 착지 커밋에 뒀다고 알린 착지 — PR의 실제 상태로 적는다
func (r *prRun) landedState(e *prEnt, ev events.Event, key string) {
	if r.stop {
		return
	}
	st, err := r.gh.State(e.repo(), e.num())
	if !r.do("PR 상태 "+e.Lane, err) {
		return // 모름 — 다음 바퀴에 다시(merged로 적지 않는다)
	}
	switch st {
	case "MERGED":
		e.State = "merged"
		say("PR #%s 착지 %s — Merged(병합기가 머리 가지를 착지 커밋에 둠)", e.Num, e.Lane)
	case "CLOSED":
		if r.comment(e, key+":closed", "착지 "+head(ev.SHA, 8)+" — 변경은 기준 가지에 들어갔다(표시만 Closed)") {
			e.State = "closed"
			say("PR #%s 착지 %s — 닫힘(GitHub가 Merged로 표시하지 않음)", e.Num, e.Lane)
		}
	default:
		// ponytail: OPEN이 이어지면 바퀴마다 gh 한 번(pr_max 안) — 오래 남는 일이 보이면 횟수 상한과 알림을 더한다
		say("PR #%s 착지 %s — GitHub가 아직 OPEN, 다음 바퀴에 다시 본다", e.Num, e.Lane)
	}
}
