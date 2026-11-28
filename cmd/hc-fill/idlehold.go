package main

// idlehold.go: 할 일 없는 빈 자리 알림 막기(ht-alert-route ⑩, docs/hc-fill/alert-route.md) — 띄우기 바퀴(waitRow)가 기다리면 풀리는
// 까닭으로 돌려보낸 줄을 WAITOK에 적고, 놀고 있음 알림(idleTick)의 「겹침 대기」 사유는 뜰 수 있는데 안 뜬 줄이 있을 때만 낸다

import (
	"fmt"
	"strings"
)

// WAITOK: 이번 바퀴 waitRow가 기다리면 풀리는 까닭(선행·주차·옛 실행 정리·자리 없음)으로 돌려보낸 줄 — 바퀴 시작(run2)에 비움
var WAITOK map[string]bool

// hold: 이번 바퀴에 그 줄을 기다리면 풀리는 까닭으로 돌려보냄(참) — 바퀴 밖에서 불려도(시험) 표를 만든다
func hold(name string, b bool) {
	if WAITOK == nil {
		WAITOK = map[string]bool{}
	}
	WAITOK[name] = b
}

// idleWaits: 기계 m이 받을 수 있는(기계 제한·그 기계 repo 줄) 겹침 대기(OVWAIT) 줄 수 — 겹침(ov)과, 그 가운데 선행을 기다리는 주차 줄
// (pk — 이 바퀴 목록 ROWS의 rowMode가 주차: 표 stStage0의 「대기(주차 — 선행 기다림)」와 같은 정의)을 갈라서
func idleWaits(m string) (ov, pk int) {
	mode := map[string]relMode{}
	for _, r := range ROWS {
		mode[r.F(qName)] = rowMode(r)
	}
	for _, o := range OVWAIT {
		oo, op := o.F(wtOnly), o.F(wtProj)
		if op == "" || (oo != "-" && !inlist(m, oo)) || repoOf(op, m) == "" {
			continue
		}
		if mode[o.F(wtLane)] == mParked {
			pk++
		} else {
			ov++
		}
	}
	return
}

// idleNote: 놀고 있음 알림의 기계 덧말 — 「(겹침 대기 n줄)」·「(선행 기다림 n줄)」·둘 다
func idleNote(ov, pk int) string {
	var o []string
	if ov > 0 {
		o = append(o, fmt.Sprintf("겹침 대기 %d줄", ov))
	}
	if pk > 0 {
		o = append(o, fmt.Sprintf("선행 기다림 %d줄", pk))
	}
	return "(" + strings.Join(o, " · ") + ")"
}

// idleWhy: 사유 ovl 묶음의 까닭(묶음 기계들의 합)
func idleWhy(ov, pk int) string {
	switch {
	case pk == 0:
		return "겹침 대기 — 앞 레인이 착지하면 뜬다(hc-fill ls)"
	case ov == 0:
		return "선행 기다림 — 선행이 착지하면 뜬다(hc-fill ls)"
	}
	return "겹침·선행 대기 — 앞 레인·선행이 착지하면 뜬다(hc-fill ls)"
}

// idleWorth: 기계 m이 받을 수 있는(기계 제한·그 기계 repo 줄) 대기 줄 가운데 이번 바퀴 waitRow가 기다리면 풀리는 까닭(WAITOK·겹침 대기
// OVWAIT)으로 돌려보내지 않은 것이 있나 — 있으면 뜰 수 있는데 안 뜬 것이라 알린다. 줄은 이 바퀴 목록(ROWS) 가운데 지금도 기다리는 것만 —
// 바퀴 도중 넣은 줄은 waitRow가 안 봤으니 다음 바퀴가 본다(넣자마자 「겹침 대기」 알림이 나갔다), 이번 바퀴에 뜬 줄은 뺀다
func idleWorth(m string) bool {
	ov := map[string]bool{}
	for _, o := range OVWAIT {
		ov[o.F(wtLane)] = true
	}
	still := map[string]bool{}
	for _, r := range qRows() {
		still[r.F(qName)] = r.F(qState) == "wait"
	}
	for _, r := range ROWS {
		n := r.F(qName)
		if r.F(qState) != "wait" || !still[n] || strings.HasPrefix(r.F(qAudit), "audit=") || WAITOK[n] || ov[n] {
			continue
		}
		if o := dash(r.F(qOnly)); (o == "-" || inlist(m, o)) && repoOf(r.F(qProj), m) != "" {
			return true
		}
	}
	return false
}
