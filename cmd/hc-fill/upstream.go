package main

// 실물 점검 띄우기(ht-upstream-contract, docs/hc-sweep/upstream.md 「누가 언제」): 기계마다 hc-sweep이 적는 판 기록을 격리 점검 조각의
// 「probe upstream ok|hold <점검할 묶음|->」 줄(hc-lane merger --repo-read — 2분 조각)로 받아, 점검할 묶음이 있는 기계에 실물 점검
// (hc-sweep --upstream-probe)을 한 번에 한 대만 띄운다. 레인을 띄우는 길을 아는 쪽이 여기라서 — 실행 명령은 설정 upstream_launch(없으면
// launch 틀, {task} 자리에 점검 지시). 풀이 빈·모르는 기계(설정 worker.proxy가 있을 때 격리 점검의 풀 항목 — workermac.PoolUsable 판정 한
// 곳)와 격리된·연결 안 된 기계는 띄우지 않는다. 한 묶음에 상한 upCap번(못 띄움·결과 모름 — 결과가 나오면 묶음이 바뀌어 다시 0)이고, 상한에
// 닿으면 막힘 한 번. 기록 DATA/upstream.tsv는 멈춘 레인의 Escape(hang.go)도 읽는다(보류 기계는 보내지 않음)

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

// upCap: 한 판 묶음에 점검을 띄우는 상한 · upWait: 띄운 점검을 기다리는 초(hc-sweep 점검 상한 15분 + 여유)
const (
	upCap  = 3
	upWait = 25 * 60
)

func upFile() string { return DATA + "/upstream.tsv" }

func upRows() map[string]Row {
	m := map[string]Row{}
	for _, r := range readRows(upFile()) {
		if r.F(usMach) != "" {
			m[r.F(usMach)] = r
		}
	}
	return m
}

// upHeld: 그 기계에 확인 안 된·틀린 판이 있다(지난 조각 기준 — 모르면 거짓: 옛 판 워커는 지금 동작 그대로)
func upHeld(m string) bool { return upRows()[m].F(usSt) == "hold" }

// upRow: 이번 조각의 upstream 줄로 고친 줄(묶음이 바뀌면 띄운 수를 0부터)
func upRow(prev Row, m, line string) Row {
	f := strings.Fields(line)
	st, owed := "na", "-"
	switch {
	case len(f) == 2 && (f[0] == "ok" || f[0] == "hold"):
		st, owed = f[0], f[1]
	case len(f) > 0 && f[0] == "na":
	default: // unknown·꼴이 다름 — 앞 줄 그대로
		if prev != nil {
			return prev
		}
	}
	r := Row{m, st, owed, "0", "0", "-", "-"}
	if prev != nil && prev.F(usOwed) == owed {
		r[usTries-1], r[usAt-1], r[usTry-1], r[usCap-1] = prev.F(usTries), prev.F(usAt), prev.F(usTry), prev.F(usCap)
	}
	return r
}

// upPoolOK: 그 기계에 에이전트를 띄울 수 있나 — 풀 점검을 하는 설정이면 마지막 풀 판정이 좋음일 때만
func upPoolOK(m string) bool { return poolCmd() == "" || cdPool.load()[m].St == "ok" }

// upLaunchArgv: 점검 명령(그 기계 hc run 뒤)
func upLaunchArgv(m string) ([]string, bool) {
	tmpl := conf("upstream_launch")
	if tmpl == "" {
		tmpl = LAUNCH
	}
	saved := LAUNCH
	LAUNCH = tmpl
	args, ok := lcLaunchArgs("upstream-probe", "-", m, "-", "-", "-", "{probe}")
	LAUNCH = saved
	if !ok || len(args) == 0 {
		return nil, false
	}
	a := []string{"run", "--bg", "--rm", "--no-parent", "--name", "upstream-probe"}
	if m != LOCAL {
		a = append(a, "--on", m)
	}
	return append(append(a, "--", "zsh", "-lc", `exec hc-sweep --upstream-probe -- "$@"`, "_"), args...), true
}

// upstreamTick: 바퀴마다 한 번(cordonTick 뒤 — PROBE가 찬 뒤)
func upstreamTick() {
	if dry {
		return
	}
	prev := upRows()
	next := map[string]Row{}
	for m, r := range prev {
		next[m] = r
	}
	for m, p := range PROBE {
		if v, ok := p["upstream"]; ok {
			next[m] = upRow(prev[m], m, v)
		}
	}
	now := nowUnix()
	var ms []string
	busy := false
	for m, r := range next {
		ms = append(ms, m)
		at, _ := strconv.ParseInt(r.F(usAt), 10, 64)
		if r.F(usTry) == r.F(usOwed) && r.F(usOwed) != "-" && now-at < upWait {
			busy = true // 띄운 점검이 아직 결과를 안 냄 — 한 번에 한 대
		}
	}
	sort.Slice(ms, func(i, j int) bool { // 보류인 기계 먼저, 그다음 이름순
		hi, hj := next[ms[i]].F(usSt) == "hold", next[ms[j]].F(usSt) == "hold"
		return hi && !hj || hi == hj && ms[i] < ms[j]
	})
	for _, m := range ms {
		r := next[m]
		tries := atoi10(r.F(usTries))
		if busy || r.F(usOwed) == "-" || m != LOCAL && !connHas(m) || cordonOn(m) || !upPoolOK(m) {
			continue
		}
		if tries >= upCap {
			if r.F(usCap) != "1" && r.F(usSt) == "hold" {
				msg := fmt.Sprintf("실물 점검을 %d번 띄웠는데 결과가 없다(%s · %s) — 이 기계 이 판 터미널 닫기 보류가 이어진다(R3는 그대로). 그 기계에서 hc-sweep --upstream 으로 보고 손으로 hc-sweep --upstream-probe -- <실행 명령>",
					upCap, m, r.F(usOwed))
				if hcLaneReport("막힘", msg, "hc-upstream", "", "hc-upstream:"+m+":"+r.F(usOwed)+":cap") == 0 {
					r[usCap-1] = "1"
				}
			}
			continue
		}
		argv, ok := upLaunchArgv(m)
		if !ok {
			say("실물 점검 안 띄움 %s: 설정 upstream_launch·launch 틀을 인자로 나누지 못함", m)
			break
		}
		r[usTries-1], r[usTry-1] = strconv.Itoa(tries+1), r.F(usOwed)
		out, rc := Cmd{Name: "hc", Args: argv, Err: errMerge}.Run()
		switch rc {
		case 0, proc.TimeoutCode: // 시간 넘김은 떴을 수 있다(미확정) — 기다린다
			r[usAt-1] = strconv.FormatInt(now, 10)
			say("실물 점검 띄움 %s: %s (%d/%d)", m, r.F(usOwed), tries+1, upCap)
		default: // 일시(hc run 거절·같은 이름이 살아 있음 등 — docs/hc-lane/launch.md 「실패 종류」) — 다음 바퀴에 상한까지
			r[usAt-1] = "0"
			say("실물 점검 못 띄움 %s(종료 %d): %s", m, rc, lastLine(sub(out)))
		}
		next[m] = r
		break
	}
	var b strings.Builder
	for _, m := range ms {
		b.WriteString(next[m].Line() + "\n")
	}
	if len(ms) > 0 || exists(upFile()) {
		if err := writeAtomic(upFile(), b.String()); err != nil {
			say("알림: 실물 점검 기록을 못 썼다(%v)", err)
		}
	}
}
