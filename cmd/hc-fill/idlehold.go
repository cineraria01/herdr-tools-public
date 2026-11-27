package main

// idlehold.go: 할 일 없는 빈 자리 알림 막기(ht-alert-route ⑩, docs/hc-fill/alert-route.md) — 띄우기 바퀴(waitRow)가 기다리면 풀리는
// 까닭으로 돌려보낸 줄을 WAITOK에 적고, 놀고 있음 알림(idleTick)의 「겹침 대기」 사유는 뜰 수 있는데 안 뜬 줄이 있을 때만 낸다

import "strings"

// WAITOK: 이번 바퀴 waitRow가 기다리면 풀리는 까닭(선행·주차·옛 실행 정리·자리 없음)으로 돌려보낸 줄 — 바퀴 시작(run2)에 비움
var WAITOK map[string]bool

// hold: 이번 바퀴에 그 줄을 기다리면 풀리는 까닭으로 돌려보냄(참) — 바퀴 밖에서 불려도(시험) 표를 만든다
func hold(name string, b bool) {
	if WAITOK == nil {
		WAITOK = map[string]bool{}
	}
	WAITOK[name] = b
}

// idleWorth: 기계 m이 받을 수 있는(기계 제한·그 기계 repo 줄) 대기 줄 가운데 이번 바퀴 waitRow가 기다리면 풀리는 까닭(WAITOK·겹침 대기
// OVWAIT)으로 돌려보내지 않은 것이 있나 — 있으면 뜰 수 있는데 안 뜬 것이라 알린다
func idleWorth(m string) bool {
	ov := map[string]bool{}
	for _, o := range OVWAIT {
		ov[o.F(wtLane)] = true
	}
	for _, r := range qRows() {
		n := r.F(qName)
		if r.F(qState) != "wait" || strings.HasPrefix(r.F(qAudit), "audit=") || WAITOK[n] || ov[n] {
			continue
		}
		if o := dash(r.F(qOnly)); (o == "-" || inlist(m, o)) && repoOf(r.F(qProj), m) != "" {
			return true
		}
	}
	return false
}
