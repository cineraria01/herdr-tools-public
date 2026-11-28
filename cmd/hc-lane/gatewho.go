// gatewho.go: 한 문(gate.go)의 받는 쪽 고르기와 그 재료(gworld). 받는 쪽은 한 명이다 — 주인(살아 있고 watch가 돎) → 그 저장소의
// 대표 지휘 → 전역 대표(손잡이 alert_coord@<프로젝트>·alert_coord, 없으면 저장소를 맡는다고 적힌 지휘 가운데 먼저 뜬 것 — pickRep).
// 9칸 --to도 주인 후보일 뿐이다. 주인·저장소 칸이 없는 사건(8칸 정리됨·지어낸 멈춤)은 그 터미널의 같은 레인 사건에서 물려받는다.
// 「남의 커서에 있나」로 정하지 않는다
package main

import (
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

// gworld: 한 바퀴 판정 재료 — 어느 지휘의 watch든 같다(커서 없음)
type gworld struct {
	now       time.Time
	hist      []events.Event                   // 이 바퀴 창(문맥 포함)
	knob      func(name string) string         // 손잡이 값(못 읽으면 기본)
	recv      func(addr, run string) tri.State // 그 지휘가 지금 받을 수 있나 — 살아 있음·같은 실행(run이 있으면)·watch가 돎
	reps      func(repo string) []string       // 대표 후보 차례(받을 수 있음·모름만), repo ""는 전역
	laneOwner func(term string) string         // 11칸이 없는 사건의 주인(레인 기록·hc parent), 모르면 -
	repo      map[string]string                // 레인 이름 → 13칸 저장소(laneRepos)
	first     func(key string) time.Time       // 시각 칸이 없는 사건(지어낸 멈춤)을 이 watch가 처음 본 때
	byLane    map[string][]int                 // hist 레인 이름 색인(laneIdx)
}

func (w *gworld) knobv(name string) string {
	if w.knob == nil {
		return ""
	}
	return w.knob(name)
}

// minutes: 분 손잡이(못 읽으면 0)
func (w *gworld) minutes(name string) time.Duration {
	n, _ := strconv.Atoi(w.knobv(name))
	return time.Duration(n) * time.Minute
}

// cand: 주인 후보 — run이 있으면 그 실행(created_ms)일 때만
type cand struct{ addr, run string }

// inherit: 주인·저장소 칸이 없는 사건(11칸 없음)은 같은 터미널·같은 레인의 가장 늦은 앞 사건(11칸 있음)에서 물려받는다
func (w *gworld) inherit(x events.Event) events.Event {
	if x.N >= 11 {
		return x
	}
	var best events.Event
	for _, i := range w.laneIdx()[x.Lane] {
		y := w.hist[i]
		if y.N >= 11 && lastSeg(y.Term) == lastSeg(x.Term) && y.State != "정리됨" && y.Time <= x.Time && y.Time >= best.Time {
			best = y
		}
	}
	if best.N == 0 {
		return x
	}
	x.To, x.Owner, x.Repo, x.N = best.To, best.Owner, best.Repo, best.N
	return x
}

// owners: 주인 후보 차례 — --to(쉼표), 11칸 지휘(@실행), 11칸이 없거나 -면 레인 기록·hc parent. 11칸 *는 주인 없음
func (w *gworld) owners(x events.Event) []cand {
	var cs []cand
	oa, orun := "", ""
	if o := x.Owner; o != "" && o != "-" && o != "*" {
		oa, orun, _ = strings.Cut(o, "@")
		oa = lastSeg(oa)
	}
	if t := x.To; t != "" && t != "-" {
		for _, a := range strings.Split(t, ",") {
			if a = lastSeg(strings.TrimSpace(a)); a != "" {
				c := cand{addr: a}
				if a == oa {
					c.run = orun
				}
				cs = append(cs, c)
			}
		}
	}
	switch {
	case oa != "":
		cs = append(cs, cand{oa, orun})
	case x.Owner != "*" && w.laneOwner != nil:
		if o := w.laneOwner(x.Term); o != "" && o != "-" {
			cs = append(cs, cand{addr: lastSeg(o)})
		}
	}
	return cs
}

// route: v에 받는 쪽 한 명·놓침 막기 다음 사람·한 구절을 채운다. 모름(지휘 확인 못 함)이 맨 앞이면 판단 필요는 모든 지휘에게
func (w *gworld) route(x events.Event, v *gverdict) {
	x = w.inherit(x)
	var list []string
	unk := false
	add := func(a string) {
		if a == "" || has(list, a) {
			return
		}
		list = append(list, a)
	}
	owner := ""
	oc := w.owners(x)
	v.hadOwner = len(oc) > 0
	for _, c := range oc {
		st := tri.No
		if w.recv != nil {
			st = w.recv(c.addr, c.run)
		}
		if st == tri.No {
			continue
		}
		owner, unk = c.addr, st == tri.Unknown
		break
	}
	add(owner)
	v.owner = owner
	r := x.Repo
	if !repoIDok(r) {
		r = w.repo[x.Lane]
	}
	if w.reps != nil && (owner == "" || v.br == brNow) { // 주인이 받는 소식에는 대표 차례가 필요 없다(지휘마다 hc get — 바퀴 비용)
		if r != "" {
			for _, a := range w.reps(r) {
				add(lastSeg(a))
			}
		}
		for _, a := range w.reps("") {
			add(lastSeg(a))
		}
	}
	if len(list) == 0 {
		v.why = strings.TrimPrefix(v.why+" · 받을 지휘 없음(events에 남음)", " · ")
		return
	}
	v.to = list[0]
	if owner == "" && w.recv != nil && w.recv(v.to, "") == tri.Unknown {
		unk = true
	}
	switch {
	case unk && v.br == brNow:
		v.all, v.tag = true, " → 모든 지휘(받는 쪽 "+v.to+" 확인 못 함)"
	case owner == "":
		v.tag = " → 대표 지휘 " + v.to
	}
	if v.br == brNow && len(list) > 1 && !v.all {
		v.next = list[1]
		base := v.at
		if !v.until.IsZero() {
			base = v.until
		}
		v.nextAt = base.Add(w.minutes("alert_escalate_min"))
		if w.minutes("alert_escalate_min") <= 0 {
			v.next = ""
		}
	}
}
