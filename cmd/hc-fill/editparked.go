package main

// edit의 주차 맞추기(ht-edit-parked, 2026-11-26): 먼저 띄운 레인이 hc-lane park로 「주차」를 보고하고 차례를 끝냈는데 run 터미널이
// done으로 남아 있으면, run 바퀴의 parkTick이 줄을 대기·parked로 되돌리기 전까지 edit --brief가 「이미 떴다(up)」로 거절됐다
// (지휘가 터미널을 손으로 닫아야 했다). edit은 그 줄에 parkTick과 같은 판정을 바로 한다 — run.lock·대기열 잠금 안이라 바퀴와 겹치지 않는다

import (
	"strconv"
	"strings"
)

// editParked <줄> <안 뜬 줄만 바꾸는 인자인가>: 떠 있음(up)·먼저 띄움(early)·다시 잡힌(parked) 줄의 지금 run 터미널이 차례를 끝냈고
// (done·idle, 또는 없음) 그 실행의 마지막 보고(정리됨 빼고)가 「주차」면 같은 줄 ID로 대기·parked로 되돌린 줄.
// 아니면(일하는 중·주차 보고 없음·터미널이나 사건을 모름) r 그대로 — 부르는 쪽이 지금처럼 거절한다
func editParked(r Row, need bool) Row {
	n, a, m, ef := r.F(qName), r.F(qAddr), r.F(qMach), r.F(qEarly)
	if !need || r.F(qState) != "up" || (ef != "early" && ef != "parked") || dash(a) == "-" {
		return r
	}
	t, found, err := hcGet(a)
	live := found && t.State != "finished"
	if err != nil || live && !t.HasName {
		return r
	}
	run := attemptRun(n, a) // 터미널이 없거나 주소가 다른 이름에 다시 쓰였으면 띄울 때 확정한 실행 키(없으면 주소·이름만)
	if live && t.Name == n {
		if t.Status != "done" && t.Status != "idle" || t.Created <= 0 {
			return r
		}
		run = a[strings.LastIndexByte(a, '/')+1:] + "@" + strconv.FormatInt(t.Created, 10)
	}
	ev, ok := laneEvents(m)
	if !ok {
		return r
	}
	saved := EV
	defer func() { EV = saved }()
	EV = nil
	for _, e := range ev {
		if splitRow(e).F(evState) != "정리됨" {
			EV = append(EV, e)
		}
	}
	l := lastev(n, a, "")
	if run != "" {
		l = runlast(n, a, run, "")
	}
	if evF(l, evState) != "주차" || qset(true, r.F(qRID), n, "wait", "-", "-", r.F(qStuck), "parked", "") != 0 {
		return r
	}
	say("주차 %s: %s 가 주차를 보고하고 차례를 끝냄(%s) — 줄을 대기·주차로 되돌림(run 바퀴의 주차 되돌리기를 먼저)", n, a, dash(t.Status))
	if x := qrow(n); x != nil {
		return x
	}
	return r
}
