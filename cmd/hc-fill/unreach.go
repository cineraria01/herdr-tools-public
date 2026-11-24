package main

// 기계 닿지 않음(2026-11-24 worker-health-sweep, docs/hc-fill/unreach.md): 기계가 잠들거나 꺼지면 그 기계의 hc-sweep은 알릴 수 없다 —
// 지휘 쪽(이 바퀴)에서 본다. 살아 있는 레인이 있는 기계가 손잡이 unreach분(기본 10, 0이면 끔) 넘게 연결 안 됨이면 그 레인 주인 지휘에
// 막힘 한 번(운영 사건 hc-fill-unreach), 알린 뒤 다시 붙으면 한 줄(중간).
// 재기: 이 바퀴에 이미 읽은 CONN(목록 앞뒤 두 번 다 연결이어야 연결)·TERMS·LANEADDR·대기열 — 원격 호출을 늘리지 않는다.
// 레인: 연결된 동안 그 기계에 떠 있던 hc-lane 라벨 터미널과 대기열 up 줄의 터미널. 끊긴 뒤에는 마지막으로 본 것(처음 본 때 이미 끊겼으면
// 대기열 up 줄).
// hc-lane watch의 「연결 안 됨 <기계>」(끊길 때마다 바로 — 그 기계 사건을 못 읽는다는 뜻)는 그대로 둔다. 이 사건은 그것이 주지 않는
// 오래 이어짐·영향 레인·회복만 맡는다(같은 사실을 되풀이하지 않게 문턱 전에는 내지 않는다).
// 기록 DATA/reach.tsv: 기계 상태(up|down) 그 상태 시작(초) 알림(0|1) 레인(쉼표, 없으면 -) — hc-fill machine·status가 읽는다.

import (
	"fmt"
	"sort"
	"strings"
)

type reachRow struct {
	M, St   string
	Since   int64
	Alerted bool
	Lanes   []string
}

func reachFile() string { return DATA + "/reach.tsv" }

func reachLoad() map[string]reachRow {
	o := map[string]reachRow{}
	for _, r := range readRows(reachFile()) {
		if r.F(1) == "" {
			continue
		}
		var ls []string
		if r.F(5) != "-" {
			ls = words(r.F(5))
		}
		o[r.F(1)] = reachRow{r.F(1), r.F(2), int64(atoi10(r.F(3))), r.F(4) == "1", ls}
	}
	return o
}

func (x reachRow) line() string {
	a := "0"
	if x.Alerted {
		a = "1"
	}
	return strings.Join([]string{x.M, x.St, fmt.Sprint(x.Since), a, dash(strings.Join(x.Lanes, ","))}, "\t")
}

// reachLanes: 이 바퀴 그 기계에 떠 있는 레인(라벨 터미널·대기열 up 줄의 터미널) — 연결된 기계만 뜻이 있다
func reachLanes(m string, q []Row) []string {
	var o []string
	for _, t := range TERMS {
		if t.M != m || t.Name == "" || t.Name == "-" {
			continue
		}
		if has(LANEADDR, t.Addr) {
			o = append(o, t.Name)
			continue
		}
		for _, r := range q {
			if r.F(6) == "up" && r.F(8) == t.Addr && r.F(1) == t.Name {
				o = append(o, t.Name)
			}
		}
	}
	o = uniq(o)
	sort.Strings(o)
	return o
}

// reachQueued: 그 기계의 대기열 up 줄 이름(끊긴 채 처음 본 때 — 떠 있는지 모르니 줄로)
func reachQueued(m string, q []Row) []string {
	var o []string
	for _, r := range q {
		if r.F(6) == "up" && r.F(7) == m && r.F(1) != "" {
			o = append(o, r.F(1))
		}
	}
	o = uniq(o)
	sort.Strings(o)
	return o
}

// reachOwners: 레인들의 주인 지휘(줄 없음 = 「-」 = 모든 지휘)
func reachOwners(lanes []string) []string {
	var o []string
	for _, n := range lanes {
		o = append(o, qown(n))
	}
	o = uniq(o)
	sort.Strings(o)
	return o
}

// unreachTick: stallTick 머리에서(readTables 다음 — TERMS·CONN·LANEADDR를 이 바퀴에 읽었다). 알림이 실패하면 그 알림만 다음 바퀴에
// 다시(hc-lane이 키 「hf:unreach:<기계>:<끊긴 시각>[:up]:<주인>」로 한 번만 적는다)
func unreachTick() {
	lim := knobInt("unreach")
	prev, now := reachLoad(), nowUnix()
	var q []Row
	if isFile(Q) {
		q = qRows()
	}
	ms := map[string]bool{}
	for m := range prev {
		ms[m] = true
	}
	for _, t := range TERMS {
		ms[t.M] = true
	}
	for _, r := range q {
		if r.F(6) == "up" && r.F(7) != "" {
			ms[r.F(7)] = true
		}
	}
	var names []string
	for m := range ms {
		if m != "" && m != LOCAL {
			names = append(names, m)
		}
	}
	sort.Strings(names)
	var out []reachRow
	for _, m := range names {
		pv, seen := prev[m]
		x := reachRow{M: m, St: "up", Since: now}
		if connHas(m) {
			x.Lanes = reachLanes(m, q)
			if seen && pv.St == "up" {
				x.Since = pv.Since
			}
			if seen && pv.St == "down" && pv.Alerted {
				msg := fmt.Sprintf("기계 %s 다시 닿음 — %s 끊겼다가 붙음(그동안의 레인 %s)", m, stDur(now-pv.Since), dash(strings.Join(pv.Lanes, ",")))
				say("%s", msg)
				if !reachSend("중간", msg, fmt.Sprintf("hf:unreach:%s:%d:up", m, pv.Since), pv.Lanes) {
					x = pv // 다음 바퀴에 다시(끊김 기록 그대로)
				}
			}
			out = append(out, x) // 레인이 없어도 적는다 — 끊긴 뒤 「처음 본 기계」로 읽혀 끝난 대기열 줄로 알리지 않게
			continue
		}
		x.St = "down"
		switch {
		case seen && pv.St == "down":
			x.Since, x.Alerted, x.Lanes = pv.Since, pv.Alerted, pv.Lanes
		case seen:
			x.Lanes = pv.Lanes
		default:
			x.Lanes = reachQueued(m, q)
		}
		mins := (now - x.Since) / 60
		if len(x.Lanes) > 0 && lim > 0 && mins >= int64(lim) && !x.Alerted {
			msg := fmt.Sprintf("기계 %s %d분째 닿지 않음(hc machine ls 연결 안 됨) — 그 기계 레인 %s. 잠들었거나 꺼졌거나 Tailscale이 끊겼을 수 있다"+
				"(붙은 뒤 그 기계 hc-sweep --health로 잠자기 끔·키체인 확인)", m, mins, strings.Join(x.Lanes, ","))
			x.Alerted = reachSend("막힘", msg, fmt.Sprintf("hf:unreach:%s:%d", m, x.Since), x.Lanes)
		}
		out = append(out, x)
	}
	if dry {
		return
	}
	if len(out) == 0 && !exists(reachFile()) {
		return
	}
	var b strings.Builder
	for _, x := range out {
		b.WriteString(x.line() + "\n")
	}
	if err := writeAtomic(reachFile(), b.String()); err != nil {
		say("기계 닿지 않음 기록을 못 씀: %v", err)
	}
}

// reachSend: 레인 주인 지휘마다 한 줄(막힘은 aesc, 회복은 중간). 모두 남겼으면 참
func reachSend(state, msg, key string, lanes []string) bool {
	ok := true
	for _, own := range reachOwners(lanes) {
		k := key + ":" + own
		if state == "막힘" {
			ok = aesc("hc-fill-unreach", own, msg, k) == 0 && ok
			continue
		}
		to := own
		if to == "-" {
			to = ""
		}
		if dry {
			fmt.Println(P + "지휘로 넘길 hc-fill-unreach: " + msg)
		} else if hcLaneReport("중간", msg, "hc-fill-unreach", to, k) != 0 {
			ok = false
		}
	}
	return ok
}

// reachNotes: 기계 → 「끊김 <얼마>째(레인 …)」 — hc-fill machine·status 기계 줄 끝에 붙인다(run 바퀴가 적은 기록, 원격을 더 읽지 않는다)
func reachNotes() map[string]string {
	o := map[string]string{}
	now := nowUnix()
	for m, x := range reachLoad() {
		if x.St != "down" {
			continue
		}
		s := "  끊김 " + stDur(now-x.Since) + "째"
		if len(x.Lanes) > 0 {
			s += "(레인 " + strings.Join(x.Lanes, ",") + ")"
		}
		if x.Alerted {
			s += " 알림 보냄"
		}
		o[m] = s
	}
	return o
}
