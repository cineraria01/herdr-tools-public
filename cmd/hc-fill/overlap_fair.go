package main

import (
	"fmt"
	"strings"
)

// 겹치는 대기 줄끼리 줄 차례 지키기(ht-overlap-fair, 2026-11-27 — ht-merger-withdraw가 넓은 예약 cmd/hc-fill/로 겹침 대기하는 8시간 동안
// 그보다 늦게 넣은, 떠 있는 상대와는 안 겹치는 레인 여섯이 차례로 먼저 떠 같은 폴더를 다시 쥐었다).
// 이번 바퀴에 줄 차례(ROWS)가 앞이고 겹침만으로 못 뜬 대기 줄 A가 있으면, A와 겹치는(같은 ovcmp — 예약·실제, 단독은 프로젝트 전체)
// 뒤 줄 B도 기다린다. A가 다른 까닭으로도 못 뜨면(띄우기 거절로 멈춤·띄울 자리 없음 — 선행 미착지·능력 막힘은 겹침 판정 전에 빠진다)
// A는 기록되지 않아 B를 막지 않는다. A의 앞 레인이 B를 기다리면(--after 사슬) 막지 않는다(서로 기다리지 않게)

// ovqEnt: 이번 바퀴 겹침만으로 못 뜬 대기 줄
type ovqEnt struct {
	lane, proj string
	idx        int      // ROWS 차례
	keys       []Row    // 후보 키(candKeys — 「R|M|A 키」)
	solo       bool     // 단독(프로젝트 전체와 겹침)
	by         []string // 이 줄을 막은 앞 레인(쓰기 소유)
}

// OVQ: 이번 바퀴의 ovqEnt(launchRows가 비운다)
var OVQ []ovqEnt

func rowIdx(n string) int {
	for i, r := range ROWS {
		if r.F(qName) == n {
			return i
		}
	}
	return -1
}

// ovqRoom <레인> <프로젝트> <지시문> <기계제한>: 띄우기 거절로 멈추지 않았고, 자리 남는 정적 후보 기계(placeCands와 같은 거름 — 기계 제한·능력·
// 저장소·가벼운 기계·상태 안 잡힘 — 와 machRoom)가 있나. 바퀴 기록(mwhy·pin)은 건드리지 않는다
func ovqRoom(n, p, brief, only string) bool {
	if rejRow(n, RID, "reject") != nil {
		return false
	}
	need, light := needs(brief), false
	if r := qrow(n); r != nil {
		light, _ = laneWeight(r.Set(qBrief, brief))
	}
	for _, mc := range machines() {
		m := mc.Name
		if only != "-" && !inlist(m, only) || !mhas(m, need) || repoOf(p, m) == "" || !light && machLight(m) || strings.Contains(","+stuck, ","+m+"=") {
			continue
		}
		if k, _ := machRoom(mc, false); k > 0 {
			return true
		}
	}
	return false
}

// ovqAdd: 겹침만으로 못 뜬 줄을 적는다(자리가 없거나 멈춘 줄은 적지 않는다 — 굶음을 뒤 줄로 옮기지 않게)
func ovqAdd(n, p, brief, ovk, only string, keys []Row, by []string) {
	if !ovqRoom(n, p, brief, only) {
		return
	}
	OVQ = append(OVQ, ovqEnt{n, p, rowIdx(n), keys, ovk == "solo", by})
}

// ovqFront <레인> <프로젝트> <지시문> <겹침칸>: 이 줄이 뒤에 서야 할 앞선 대기 줄과 키(없으면 빈 값)
func ovqFront(n, p, brief, ovk string) (front, keys string, by []string) {
	i := rowIdx(n)
	if i < 0 {
		return "", "", nil
	}
	A, T := qGraph()
	for _, e := range OVQ {
		if e.proj != p || e.lane == n || e.idx < 0 || e.idx >= i {
			continue
		}
		waits := false
		for _, b := range e.by {
			waits = waits || reach(A, T, b)[n]
		}
		if waits {
			continue
		}
		if e.solo || ovk == "solo" {
			return e.lane, "단독", append(append([]string(nil), e.by...), e.lane)
		}
		if OVM != "block" {
			continue
		}
		if len(OVKEYS) == 0 {
			OVKEYS = candKeys(p, n, brief)
		}
		var own []Row
		for _, k := range e.keys {
			own = append(own, append(Row{e.lane}, k...))
		}
		for _, r := range ovcmp(own, OVKEYS) {
			if g := r.F(ovGrade); g == "예약" || g == "실제" {
				return e.lane, r.F(ovKeys), append(append([]string(nil), e.by...), e.lane)
			}
		}
	}
	return "", "", nil
}

// ovqHold: 앞선 대기 줄 뒤에 서면 겹침 대기로 적고(overlap.tsv 6칸 「줄」) 참
func ovqHold(n, p, brief, ovk, only string) bool {
	f, k, by := ovqFront(n, p, brief, ovk)
	if f == "" {
		return false
	}
	say("겹침 대기 %s: 앞선 대기 줄 %s (%s) — 줄 차례대로 그 줄이 먼저 뜬다", n, f, k)
	OVWAIT = append(OVWAIT, Row{n, f, k, dash(only), p, "줄"})
	ovqAdd(n, p, brief, ovk, only, OVKEYS, by)
	return true
}

// ovWaitNote <레인> <overlap.tsv 줄>: ls·status 비고 — 「겹침 대기(<앞 레인>: <키>)」·「겹침 대기(앞선 대기 줄 <A>: <키>)」와
// 앞에 선 줄의 「다음 차례(겹치는 뒤 줄 n개가 기다림)」
func ovWaitNote(n string, ws []Row) string {
	var o []string
	back := 0
	for _, w := range ws {
		switch {
		case w.F(wtLane) == n && len(o) == 0:
			pre := ""
			if w.F(wtQueue) == "줄" {
				pre = "앞선 대기 줄 "
			}
			o = append(o, "겹침 대기("+pre+w.F(wtFront)+": "+w.F(wtKey)+")")
		case w.F(wtFront) == n && w.F(wtQueue) == "줄":
			back++
		}
	}
	if back > 0 {
		o = append(o, fmt.Sprintf("다음 차례(겹치는 뒤 줄 %d개가 기다림)", back))
	}
	return strings.Join(o, " ")
}
