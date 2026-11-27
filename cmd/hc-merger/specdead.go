package main

// 맡긴 검증의 일꾼이 죽었으면 상한을 기다리지 않는다(ht-merger-dead-worker, 2026-11-28 — 검증 기계 한 대가 맨 앞 추측 도중 전원이 꺼졌는데
// 병합기는 그 프로젝트의 맨 앞 기다림 상한 1500초까지 「결과를 기다린다」를 되풀이했다. 지휘가 손잡이를 낮춰 약 17분 뒤 넘김).
//
//   - 바퀴마다 도는 추측(state=run, 병합기 기계 밖)이 있으면 hc machine ls 한 번. 죽음은 **확신**일 때만이고 그 줄을 「모름」(specLost,
//     까닭 끊김)으로 놓는다 — 그 뒤는 지금 모름 길 그대로(맨 앞: 한가한 다른 기계에 다시 맡김 → 없으면 손잡이 front_local, 앞 무리·범인
//     가리기·미리 줄·뒤 무리: 지금 모름 길). 새 길은 없다.
//     ① 그 기계가 연속으로 deadAfter초 넘게 안 닿음(상태 낱말 unreachable·disconnected). 연속 = 앞 관측에서 deadGap초 안에 다시 안 닿음을
//     봄 — 병합기가 한참 안 돌았으면(그 사이를 모른다) 지금부터 다시 센다. 처음 보는 기계도 지금부터(옛 판이 맡긴 state=run 기록 포함).
//     ② 그 기계가 닿는데(connected) 일꾼 터미널 spec-<아이디 8>(나눔이면 안 끝난 몫의 spec-<아이디 8>-<i>)이 없음·끝남이고, 일꾼이 떴다는
//     표식이 있고 띄운 지 deadMinAge초 넘었고, 결과(통과 기록·실패 기록·spec 참조 바뀜)가 지금 ls-remote에도 없음 — 바퀴 앞 ls-remote 뒤에
//     결과를 남기고 끝난 일꾼을 죽음으로 보지 않게.
//     hc 호출 실패·시간 넘김·모르는 상태 낱말·목록에 없는 기계(이름 바뀜)·터미널 못 읽음은 모름 = 지금처럼 기다린다(셈도 늘리지 않는다).
//   - 늦은 결과: 다시 맡겼으면 spec 참조가 바뀌어 옛 일꾼은 결과를 남기지 않는다(specWrite). 직접 검증으로 넘겼으면 그 바퀴에 판정이 끝나
//     늦은 통과·실패는 짝 없는 결과로 치운다(specClean) — 지금 상한 넘김 뒤 늦은 결과와 같다.
//   - 상태는 병합기 일지 폴더 deadFile 한 장(바퀴마다 새 일꾼 프로세스라 메모리로는 셀 수 없다): 「m <기계> <처음> <마지막>」(안 닿음 관측),
//     「n <spec 커밋>」(일지에 적은 끊김 — 같은 실행은 한 번).

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/verified"
)

const (
	deadFile   = "specdead"
	deadAfter  = 180 // 연속으로 안 닿은 지 이만큼(초) 넘으면 죽음 — 1분 안팎의 잠깐 끊김으로 놓지 않게, 합격 기준 5분 안
	deadGap    = 120 // 앞 관측과 이만큼(초) 넘게 떨어지면 연속이 아니다(병합기가 안 돈 사이를 안 닿음으로 세지 않음)
	deadMinAge = 60  // 띄운 지 이만큼(초) 안 된 일꾼은 터미널이 없어도 죽음으로 보지 않는다
	jDead      = "추측 일꾼 끊김"
)

// deadNote: 이 바퀴에 놓은 끊긴 일꾼(일지 한 줄 — spec()이 다음 길을 붙여 적는다)
type deadNote struct {
	commit, lanes, why string
	front              bool
}

// specDead: 도는 추측 가운데 일꾼이 죽었다고 확신하는 줄을 모름(끊김)으로 놓는다. groups는 일지의 레인 이름(없으면 아이디 8)
func (m *M) specDead(c *specCtx, groups []specGroup, now int64) {
	type spot struct{ mach, term, part string }
	type cand struct {
		r     *specRow
		spots []spot
	}
	var cs []cand
	sh, shOK := m.shardRead()
	for i := range c.rows {
		r := &c.rows[i]
		if r.state != specRun || len(r.id) < 8 {
			continue
		}
		var sp []spot
		if shOK && sh.id == r.id && sh.fin == "" {
			for _, p := range sh.parts {
				if !shardEnded(c.refs, r.id, p[0]) {
					sp = append(sp, spot{p[1], "spec-" + r.id[:8] + "-" + p[0], p[0]})
				}
			}
		} else {
			sp = []spot{{r.machine, "spec-" + r.id[:8], ""}}
		}
		var ok []spot
		for _, s := range sp {
			if s.mach != m.mme && addrOK(s.mach) {
				ok = append(ok, s)
			}
		}
		if len(ok) > 0 {
			cs = append(cs, cand{r, ok})
		}
	}
	if len(cs) == 0 {
		return
	}
	o, rc, _ := hcRun(hcLook, "machine", "ls", "--json")
	ms, err := hcjson.ParseMachines([]byte(o))
	if rc != 0 || err != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 검증 기계 연결을 못 읽었다(hc machine ls) — 일꾼 끊김은 이 바퀴에 모름")
		return
	}
	st := map[string]string{}
	for _, x := range ms {
		st[x.Name] = x.State
	}
	want, live := map[string]bool{}, map[string]bool{}
	for _, x := range cs {
		live[x.r.commit] = true
		for _, s := range x.spots {
			want[s.mach] = true
		}
	}
	down, noted := m.deadTrack(want, live, st, now)
	front := ""
	if w := specWalk(m.btree, groups, c.rows); len(w) > 0 && w[0].row != nil {
		front = w[0].row.id
	}
	for _, x := range cs {
		r, why := x.r, ""
		for _, s := range x.spots {
			if d := down[s.mach]; d > deadAfter {
				why = fmt.Sprintf("%s, 안 닿은 지 %d초", s.mach, d)
				break
			}
			if st[s.mach] != "connected" || c.refs[specStarts+r.id] == "" || now-r.at < deadMinAge {
				continue
			}
			t, found, err := hcGet(s.mach+"/"+s.term, "")
			if err == nil && (!found || t.State != "live") && specNoResult(r) {
				why = s.mach + ", 일꾼 터미널 없음"
				if s.part != "" {
					why = s.mach + ", 몫 " + s.part + " 일꾼 터미널 없음"
				}
				break
			}
		}
		if why == "" {
			continue
		}
		r.state, r.lost = specLost, "끊김"
		lanes := first8(r.id)
		for _, g := range groups {
			if g.key == r.group {
				lanes = g.lanes
			}
		}
		fmt.Printf("hc-lane merger: 추측 일꾼 끊김 — %s (%s) — 상한을 기다리지 않고 모름으로\n", lanes, why)
		if !noted[r.commit] {
			m.dead = append(m.dead, deadNote{commit: r.commit, lanes: lanes, why: why, front: r.id == front})
		}
	}
}

// specNoResult: 추측 r의 결과가 지금 원격에도 없나(통과 기록·실패 기록 없음, spec 참조 그대로). 못 읽으면 false(모름 — 놓지 않음)
func specNoResult(r *specRow) bool {
	refs := []string{specRefs + r.id, specFails + r.id, verified.Prefix + r.tree}
	o, rc := lsRemote(append([]string{"origin"}, refs...)...)
	if rc != 0 {
		return false
	}
	for _, l := range strings.Split(o, "\n") {
		sha, ref, _ := strings.Cut(l, "\t")
		switch {
		case ref == refs[0] && sha != r.commit, ref == refs[1], ref == refs[2]:
			return false
		}
	}
	return true
}

// deadTrack: 안 닿음 관측을 deadFile에 잇고, 볼 기계(want)마다 연속으로 안 닿은 초(안 닿지 않았거나 모르면 없음)와 이미 일지에 적은 spec 커밋
// (live — 지금 도는 추측 커밋 — 것만 남긴다). connected면 기록을 지우고, 모르는 낱말·목록에 없음이면 그대로 둔다(늘리지도 지우지도 않음)
func (m *M) deadTrack(want, live map[string]bool, st map[string]string, now int64) (map[string]int64, map[string]bool) {
	p := m.mj + "/" + deadFile
	b, _ := os.ReadFile(p) // 없으면 처음부터
	seen, noted := map[string][2]int64{}, map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) == 4 && f[0] == "m":
			a, e1 := strconv.ParseInt(f[2], 10, 64)
			z, e2 := strconv.ParseInt(f[3], 10, 64)
			if e1 == nil && e2 == nil && a <= z && z <= now {
				seen[f[1]] = [2]int64{a, z}
			}
		case len(f) == 2 && f[0] == "n" && live[f[1]]:
			noted[f[1]] = true
		}
	}
	down := map[string]int64{}
	for mach := range want {
		switch st[mach] {
		case "connected":
			delete(seen, mach)
		case "unreachable", "disconnected":
			v, ok := seen[mach]
			if !ok || now-v[1] > deadGap {
				v[0] = now
			}
			v[1] = now
			seen[mach], down[mach] = v, now-v[0]
		}
	}
	var o strings.Builder
	for mach, v := range seen {
		if now-v[1] <= deadGap {
			fmt.Fprintf(&o, "m %s %d %d\n", mach, v[0], v[1])
		}
	}
	for cm := range noted {
		fmt.Fprintf(&o, "n %s\n", cm)
	}
	if o.String() != string(b) {
		if err := writeVia(p, o.String()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 일꾼 끊김 기록을 적지 못했다(다음 바퀴에 처음부터 센다): %v\n", err)
		}
	}
	return down, noted
}

// deadJournal: spec()이 판정 뒤 — 이 바퀴에 놓은 끊긴 일꾼마다 일지 한 줄(같은 실행은 한 번)과 다음 길
func (m *M) deadJournal(d specDecision, now int64) {
	if len(m.dead) == 0 {
		return
	}
	defer func() { m.dead = nil }() // 한 바퀴에 spec()을 또 불러도 같은 줄을 두 번 적지 않게
	p := m.mj + "/" + deadFile
	b, _ := os.ReadFile(p)
	add := ""
	for _, x := range m.dead {
		next := "모름 — 그 자리의 모름 길"
		if x.front {
			switch {
			case m.wre != "":
				next = "다른 기계에 다시 맡김(" + m.wre + ")"
			case d == specLand:
				next = "직접 검증"
			case d == specWaitFront:
				next = "기다림(" + m.vw.why + ")"
			}
		}
		what := fmt.Sprintf("%s — %s (%s) → %s", jDead, x.lanes, x.why, next)
		fmt.Printf("hc-lane merger: %s\n", what)
		m.jmembers(m.b, jDead, what, "", m.r0, now)
		add += "n " + x.commit + "\n"
	}
	if err := writeVia(p, string(b)+add); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 일꾼 끊김 표식을 적지 못했다(다음 바퀴에 일지 줄이 또 나온다): %v\n", err)
	}
}
