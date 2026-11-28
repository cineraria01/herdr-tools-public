// gateexplain.go: hc-lane events --explain — 사건마다 한 문(gate.go)의 판정 한 줄(읽기 전용). 지휘가 「이 알림 왜 나한테 왔지」
// 「그 막힘 왜 안 왔지」를 이 한 줄로 본다: 받는 쪽·갈래·종류·까닭, 보류면 언제까지, 안 보냈으면 왜, 공용 보냄 기록에 있나
package main

import (
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

// restedOf: 보냄 기록 줄 가운데 쉼(멈춤 … 쉼 turn n)을 알린 터미널·레인(주소 끝 t…\x1c레인)
func restedOf(ls []string) map[string]bool {
	m := map[string]bool{}
	for _, l := range ls {
		if f := strings.Split(l, tab); len(f) == 4 && f[0] == "멈춤" && strings.HasPrefix(f[3], "쉼 ") {
			m[lastSeg(f[1])+"\x1c"+f[2]] = true
		}
	}
	return m
}

// explain: all(시각 순 사건 — 판정 재료 전부)의 사건 가운데 --since·--lane·--kind에 드는 것마다 한 줄
func (e *ev) explain(all []string) {
	e.lsLanes()
	e.win, e.lrepo = all, laneRepos(all)
	e.occ, e.rcv, e.reps = map[string]tri.State{}, map[string]tri.State{}, map[string][]string{}
	w := e.world(nil)
	sl, _ := readLines(e.ed + "/sent")
	sent := map[string]bool{}
	for _, l := range sl {
		sent[nk(l)] = true
	}
	rested, kind := restedOf(sl), e.lkinds()
	end := map[string]string{}
	for _, l := range all {
		x := events.Cols(l)
		t2 := x.Term + "\x1c" + x.Lane
		show := x.Time >= e.since && e.laneOK(x)
		head := clock(evTime(x)) + " " + x.Lane + " " + x.State
		if x.SHA != "-" && x.SHA != "" {
			head += " " + short8(x.SHA)
		}
		head += " 「" + digestCut(x.Msg, 40) + "」"
		if x.State == "정리됨" {
			_, rec := kind[[2]string{lastSeg(x.Term), x.Lane}]
			name, ok := closeName(end[t2], x.Msg, rested[lastSeg(x.Term)+"\x1c"+x.Lane])
			switch {
			case !rec:
				name, ok = "레인 기록에 없는 터미널", false
			case ok:
				x.State, x.SHA, x.Msg = "멈춤", "-", name
			}
			if !ok {
				if show {
					fmt.Println(head + " — " + brMute.String() + " · 정리됨 · " + name)
				}
				continue
			}
		} else if x.State != "중간" {
			end[t2] = x.State
		}
		if !show {
			continue
		}
		v := w.judge(x)
		fmt.Println(head + " — " + explainTail(v, sent[lineKey(x.Raw())]))
	}
}

// explainTail: 판정 한 구절 「<갈래> → <받는 쪽> · <종류> · <까닭>[ · 놓침 막기 …][ · 보냄 기록 있음]」
func explainTail(v gverdict, sent bool) string {
	to := v.to
	switch {
	case v.all:
		to = "모든 지휘"
	case to == "":
		to = "-"
	}
	s := v.br.String()
	switch {
	case v.br == brMute || v.br == brHold:
	case v.all:
		s += v.tag
	case strings.Contains(v.tag, "대표 지휘"):
		s += " → " + to + "(대표)"
	default:
		s += " → " + to
	}
	s += " · " + v.kind
	if v.why != "" {
		s += " · " + v.why
	}
	switch {
	case v.stale && staleKeep(v, time.Now()):
		s += " · 묵었지만 안 풀림 — 시각을 붙여 받는 쪽에게"
	case v.stale && v.br == brNow && v.open:
		s += " · 묵었고 안 풀렸지만 안 보냄(상한 " + mins(staleMax) + " 넘음)"
	}
	if v.next != "" && !v.stale {
		s += " · 놓침 막기 " + clock(v.nextAt) + "부터 " + v.next
	}
	if sent {
		s += " · 보냄 기록 있음"
	}
	return s
}
