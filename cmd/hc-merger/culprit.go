package main

// 묶음 실패 뒤 범인을 한 번에 가르기(ht-merger-culprit-parallel, 2026-11-26 사용자 「병합기 병목을 해결할수 있는 가장 좋은 방법」 → 「1,2,3
// 모두 해줘」의 3번 — 11-25~26 한 사슬이 묶음 5 → 4 → 3 → 2 → 맨 앞 혼자로 검증 네 바퀴를 차례로 태우는 동안 main이 섰다).
//
// 묶음이 시험 실패해 줄인 바퀴(cap — 맨 앞 무리 = 앞 c)에 앞 무리 c·c−1·…·1을 한가한 검증 기계에 추측 검증으로 함께 맡긴다. 아이디는 지금
// spec 식 그대로(기준 트리 + 앞 j의 열쇠)라 앞 j로 줄인 바퀴의 맨 앞 추측과 같다 — 이미 결과가 있는 앞 무리는 다시 돌지 않는다. 기계가 모자라면
// 이분 차례(bisect), 하나도 없으면 지금 길. 바퀴마다 결과로 lo = 통과한 가장 긴 앞 무리(없으면 0), hi = lo 뒤 첫 시험 실패(없으면 c+1 —
// 줄이기 전 묶음)를 보고:
//   - lo = c: 맨 앞이 통과 — 지금 길(그 기록으로 착지).
//   - hi = lo+1(범인 = lo+1번째): lo ≥ 1이면 cap을 lo로(다음 바퀴 앞 lo가 그 통과 기록으로 착지 — 착지 관문은 그대로), 범인은 표식
//     (<일지 폴더>/suspect)으로 큐에서 혼자 묶이고, 「앞 lo 트리 + 범인」 추측을 첫 실패 기계가 아닌 한가한 기계에 한 번 더 확인으로 띄운다
//     (본문에 첫 실패 — specfront.go 한 번 더 확인과 같은 꼴). 앞 lo 착지 뒤 맨 앞 혼자가 된 범인의 아이디가 그 추측과 같아, 시험 실패면
//     두 기계 근거로 반송(ffReject), 통과면 착지(흔들림 — 반송 없음). 뒤 구성원은 큐에 남는다. lo = 0이면 cap을 1로 — 지금의 맨 앞 혼자 길.
//   - hi ≤ c(가운데 시험 실패, 그 앞은 아직): cap을 hi−1로.
//   - 그 밖: 맨 앞이 도는 중이면 지금 기다림 길, 앞 무리가 도는 중이면 기다림(상한 merger_front_wait — 넘기면 지금 길), 아니면 지금 길.
//
// 시험 실패(실패 기록 test=1)가 아닌 것 — 모름·환경 실패·바쁨·도는 중·충돌 — 은 통과도 실패도 아니다(범인 근거가 아님). 손잡이 culprit
// serial, 맨 앞을 맡기지 않음(front_dispatch 0), 착지 스크립트 저장소면 지금 길(마지막 빼고 다시 + 맨 앞 혼자 추측).

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// jPrefix: 묶음 실패 뒤 앞 무리들을 검증 기계에 함께 맡긴 바퀴(구성원마다 한 줄)
const jPrefix = "앞 무리 추측"

// 앞 무리 하나의 결과
const (
	pOther = iota // 도는 중·모름·환경 실패·바쁨·충돌·줄 없음
	pPass
	pFail // 시험 실패(실패 기록 test=1)
)

type prefix struct {
	g    specGroup
	id   string
	st   int
	mach string            // 맡은 기계
	kv   map[string]string // 시험 실패 기록의 칸(pFail)
	test string            // 첫 실패 시험 줄(pFail)
}

// culpritPar: 손잡이 culprit(parallel 기본 | serial — HC_LANE_MERGER_CULPRIT)
func (m *M) culpritPar() bool {
	switch v := m.knobStr("culprit", "HC_LANE_MERGER_CULPRIT"); v {
	case "", "parallel":
		return true
	case "serial":
		return false
	default:
		fmt.Fprintf(os.Stderr, "hc-lane merger: 손잡이 culprit %q 를 모른다 — parallel로\n", v)
		return true
	}
}

// parCap: 이 바퀴가 앞 무리를 함께 가르는 바퀴인가. 착지 스크립트 저장소는 옆으로 빼기(손잡이 aside)가 켜졌을 때만 — 추측 실패를
// 반송 근거가 아니라 차례 근거로 쓴다(범인은 반송하지 않고 혼자 착지 검증으로, frontsteady.go B)
func (m *M) parCap(disp bool) bool {
	return disp && (m.hb == "" || m.sideline()) && len(m.b) >= 2 && m.capN() > 0 && m.culpritPar()
}

func rowOf(c *specCtx, id string) *specRow {
	for i := range c.rows {
		if c.rows[i].id == id {
			return &c.rows[i]
		}
	}
	return nil
}

// prefixScan: 앞 1…c(c = len(m.b))의 추측과 결과
func (m *M) prefixScan(c *specCtx) []prefix {
	ps := make([]prefix, len(m.b))
	for j := range ps {
		p := prefix{g: newGroup(m.b[:j+1])}
		p.id = specID(m.btree, p.g.key)
		if r := rowOf(c, p.id); r != nil {
			p.mach = r.machine
			switch r.state {
			case specPass:
				p.st = pPass
			case specFail:
				if kv, test, ok := m.specFailRec(c, p.id); ok && kv["test"] == "1" && test != "" && kv["exit"] != "" {
					p.st, p.kv, p.test = pFail, kv, test
				}
			}
		}
		ps[j] = p
	}
	return ps
}

// culpritBounds: lo = 통과한 가장 긴 앞 무리(없으면 0), hi = lo 뒤 첫 시험 실패(없으면 len+1)
func culpritBounds(st []int) (lo, hi int) {
	for j := len(st); j >= 1; j-- {
		if st[j-1] == pPass {
			lo = j
			break
		}
	}
	hi = len(st) + 1
	for j := lo + 1; j <= len(st); j++ {
		if st[j-1] == pFail {
			return lo, j
		}
	}
	return lo, hi
}

func bounds(ps []prefix) (int, int) {
	st := make([]int, len(ps))
	for i, p := range ps {
		st[i] = p.st
	}
	return culpritBounds(st)
}

// bisect: lo와 hi 사이(둘 다 뺌) 앞 무리 길이를 이분 차례로 — 가운데, 왼쪽 반의 가운데, 오른쪽 반의 가운데, …
func bisect(lo, hi int) []int {
	o := []int{}
	for q := [][2]int{{lo, hi}}; len(q) > 0; q = q[1:] {
		a, b := q[0][0], q[0][1]
		if b-a < 2 {
			continue
		}
		mid := (a + b) / 2
		o = append(o, mid)
		q = append(q, [2]int{a, mid}, [2]int{mid, b})
	}
	return o
}

// culpritJump: 결과로 범인을 가렸거나 가운데가 시험 실패면 cap을 줄인다(specShrink — m.capTo·m.cwhy·m.clog). 그 밖은 ok=false
func (m *M) culpritJump(c *specCtx, ps []prefix, busy map[string]bool, now int64) (specDecision, bool) {
	lo, hi := bounds(ps)
	n := len(ps)
	switch {
	case lo == n || hi > n:
		return specLand, false
	case hi == lo+1:
		m.culpritFound(c, ps, lo, busy, now)
		return specShrink, true
	}
	f := ps[hi-1]
	m.capTo, m.clog = hi-1, f.kv["machine"]+":"+f.kv["log"]
	m.markSuspect(m.b[hi-1], f.test) // 범인 후보 — 앞 hi−1이 착지하면 혼자 묶인다(다시 묶음 사슬을 타지 않게, 감사 낮음 1)
	m.cwhy = fmt.Sprintf("묶음 %d 추측 검증 — 앞 %d 시험 실패(%s, 첫 실패 「%s」), 그 앞은 아직 — 앞 %d로 줄여 다시", n, hi, f.kv["machine"], f.test, hi-1)
	fmt.Printf("hc-lane merger: %s\n", m.cwhy)
	return specShrink, true
}

// culpritFound: 범인 = 앞 lo+1의 lo+1번째(앞 lo 통과 · 앞 lo+1 시험 실패)
func (m *M) culpritFound(c *specCtx, ps []prefix, lo int, busy map[string]bool, now int64) {
	f, e := ps[lo], m.b[lo]
	m.clog = f.kv["machine"] + ":" + f.kv["log"]
	if lo == 0 { // 맨 앞 — 혼자로 줄이면 지금의 맨 앞 혼자 길(앞 1 추측 = 맨 앞 혼자 추측, specCulpritWhy)
		m.capTo = 1
		if s, _ := m.suspectRec(); s != "" && slices.Contains(shas(m.b), s) {
			removeQuiet(m.mj + "/suspect") // 앞서 적은 이 묶음의 범인 「후보」는 확정이 아니다 — 옆으로 빼지 않게(감사 atp3xrb 낮음 1)
		}
		m.cwhy = fmt.Sprintf("묶음 %d 추측 검증 — 앞 1 시험 실패(%s, 첫 실패 「%s」) — 범인 맨 앞 %s, 혼자로 줄여 다시", len(ps), f.kv["machine"], f.test, e.l)
		fmt.Printf("hc-lane merger: %s\n", m.cwhy)
		return
	}
	m.capTo = lo
	m.markSuspect(e, f.test)
	chk := "옆으로 뺌 — 앞 착지 뒤 혼자 착지 검증" // 착지 스크립트 저장소: 확인 추측 없이 착지 스크립트가 판정(frontsteady.go B)
	if m.hb == "" {
		chk = m.culpritRecheck(c, ps[lo-1], e, f, busy, now)
	}
	m.cwhy = fmt.Sprintf("묶음 %d 추측 검증 — 앞 %d 통과(%s)·앞 %d 시험 실패(%s, 첫 실패 「%s」) — 앞 %d 착지 · 범인 %s, %s",
		len(ps), lo, ps[lo-1].mach, lo+1, f.kv["machine"], f.test, lo, e.l, chk)
	fmt.Printf("hc-lane merger: %s\n", m.cwhy)
}

// culpritRecheck: 「앞 lo 트리 + 범인」 추측을 첫 실패 기계가 아닌 한가한 검증 기계(없으면 그 기계)에 띄운다 — 본문에 첫 실패(한 번 더 확인).
// 앞 lo가 착지하면 맨 앞 혼자가 된 범인의 추측이 바로 이것이다. 일지에 덧붙일 글을 돌려준다
func (m *M) culpritRecheck(c *specCtx, pass prefix, e entry, f prefix, busy map[string]bool, now int64) string {
	pr := rowOf(c, pass.id)
	g := newGroup([]entry{e})
	if rowOf(c, specID(pr.tree, g.key)) != nil {
		return "범인 확인 추측이 이미 있음"
	}
	v := m.specPick(c, busy, nil, f.kv["machine"], true)
	if v.Machine == "" {
		return "범인 확인은 앞 착지 뒤 맨 앞 혼자 길(빈 검증 기계 없음)"
	}
	note := fmt.Sprintf("recheck machine=%s exit=%s log=%s\n%s", f.kv["machine"], f.kv["exit"], f.kv["log"], f.test)
	r, ok := m.specBuild([]specStep{{row: pr}, {g: g, parent: pr.tree, note: note}}, v, now)
	if !ok {
		return "범인 확인 추측을 못 지음(앞 착지 뒤 맨 앞 혼자 길)"
	}
	c.rows = append(c.rows, r)
	if r.state != specRun {
		return "범인 확인을 못 띄움(앞 착지 뒤 맨 앞 혼자 길)"
	}
	busy[v.Machine] = true
	return "범인 확인 " + v.Machine + " 에 맡김"
}

// prefixDispatch: 아직 가를 앞 무리(lo와 hi 사이)를 이분 차례로 빈 검증 기계에 맡긴다(맨 앞 무리 = 앞 c도 그 하나). 앞 1을 맡기면 맨 앞
// 혼자 표식(solo)도 — 앞 1 실패로 혼자로 줄인 바퀴가 지금의 맨 앞 혼자 반송 길(specCulpritWhy)을 탄다
func (m *M) prefixDispatch(c *specCtx, ps []prefix, busy map[string]bool, now int64) {
	lo, hi := bounds(ps)
	var gave []string
	for _, j := range bisect(lo, hi) {
		p := &ps[j-1]
		if rowOf(c, p.id) != nil {
			continue
		}
		v := m.frontFree(c, busy, nil) // 범인 가르기는 뒤 무리·미리 줄보다 먼저(frontsteady.go C)
		if v.Machine == "" {
			break
		}
		r, ok := m.specBuild([]specStep{{g: p.g, parent: m.btree}}, v, now)
		if !ok {
			break
		}
		c.rows = append(c.rows, r)
		if r.state != specRun {
			continue
		}
		busy[v.Machine], p.mach = true, v.Machine
		gave = append(gave, fmt.Sprintf("앞 %d(%s)", j, v.Machine))
		if j == 1 {
			if err := writeVia(m.mj+"/solo", p.id+"\n"); err != nil {
				fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 혼자 추측 표식을 적지 못했다 — 실패해도 한 번 더 확인 뒤 반송한다")
			}
		}
	}
	if len(gave) > 0 {
		m.jmembers(m.b, jPrefix, "묶음 실패 뒤 앞 무리를 검증 기계에 함께 맡김 — "+strings.Join(gave, "·"), "", m.r0, now)
	}
}

// prefixWait: 맨 앞이 아직 안 맡겨졌고(이분 차례) 가를 앞 무리가 도는 중이면 기다린다(m.wmach = 도는 기계들). 가장 오래 도는 것이
// merger_front_wait를 넘겼거나 도는 것이 없으면 ok=false(지금 길)
func (m *M) prefixWait(c *specCtx, ps []prefix, front *specRow, now int64) (specDecision, bool) {
	if front != nil && front.state == specRun {
		return specLand, false // 맨 앞 기다림은 지금 길(상한·진행 확인 — specfront.go)
	}
	lo, hi := bounds(ps)
	var ms []string
	var run []*specRow
	old := now
	for _, p := range ps[lo : hi-1] {
		if r := rowOf(c, p.id); r != nil && r.state == specRun {
			ms, old, run = append(ms, r.machine), min(old, r.at), append(run, r)
		}
	}
	if len(ms) == 0 || now-old > specFrontWait() && !m.anyFresh(run) {
		return specLand, false
	}
	m.wmach = strings.Join(ms, "·")
	fmt.Printf("hc-lane merger: 앞 무리 추측 도는 중(%s) — 결과를 기다린다\n", m.wmach)
	return specWaitFront, true
}

// anyFresh: 상한을 넘긴 앞 무리 가운데 하나라도 그 기계에서 진행 중인가(specFresh — 맨 앞 상한 넘김과 같은 규칙, 감사 atp3lz5-lg7y9e 중간 1).
// 진행 중이면 merger_spec_timeout까지 더 기다린다(넘기면 specResolve가 모름으로 — 도는 것이 없어 지금 길)
func (m *M) anyFresh(rs []*specRow) bool {
	for _, r := range rs {
		if f, _ := m.specFresh(r); f {
			fmt.Printf("hc-lane merger: 앞 무리 추측이 기다림 상한을 넘겼지만 %s 에서 진행 중 — merger_spec_timeout까지 더 기다린다\n", r.machine)
			return true
		}
	}
	return false
}

// prefixKeep: 치우기에서 앞 무리 추측을 길처럼 지킨다(통과·실패 결과를 다음 바퀴 판정에 쓴다)
func prefixKeep(c *specCtx, ps []prefix) []specStep {
	var o []specStep
	for _, p := range ps {
		if r := rowOf(c, p.id); r != nil {
			o = append(o, specStep{row: r})
		}
	}
	return o
}

// specSolo: 묶음이 실패해 줄인 바퀴(cap)에 앞 k−1(≥2)을 검증하는 동안(disp면 검증 기계에서, 아니면 병합기가 직접) 「맨 앞 혼자」를 빈
// 검증 기계에 맡긴다(merger-batch-culprit — 손잡이 culprit serial·front_dispatch 0의 길, parallel은 prefixDispatch가 앞 1도 맡는다).
// 아이디는 다음 바퀴 맨 앞이 혼자일 때의 무리(기준 트리 + 맨 앞)와 같아, 그 바퀴 specWalk가 결과를 그대로 읽는다. 기계가 없거나 바쁘거나
// 이미 그 추측이 있으면 아무것도 안 한다(지금 그대로)
func (m *M) specSolo(c *specCtx, busy map[string]bool, now int64, disp bool) {
	if m.capN() == 0 || len(m.b) < 2 {
		return
	}
	g := newGroup(m.b[:1])
	id := specID(m.btree, g.key)
	if rowOf(c, id) != nil {
		return
	}
	v := m.specFree(c, busy)
	if v.Machine == "" && disp {
		v = m.preempt(c, busy, nil) // 맨 앞 혼자는 미리 줄보다 먼저(frontsteady.go C)
	}
	if v.Machine == "" {
		return
	}
	r, ok := m.specBuild([]specStep{{g: g, parent: m.btree}}, v, now)
	if !ok {
		return
	}
	c.rows = append(c.rows, r)
	if r.state == specRun {
		busy[v.Machine] = true
		// 반송 근거로 믿는 것은 이렇게 cap 바퀴에 지은 추측뿐(같은 아이디의 미리 줄 추측은 아님 — specCulpritWhy, 감사 atoz3el-ese56i)
		if err := writeVia(m.mj+"/solo", id+"\n"); err != nil {
			fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 혼자 추측 표식을 적지 못했다 — 실패해도 지금처럼 직접 검증한다")
		}
		where := "병합기가 직접"
		if disp {
			where = "검증 기계에"
		}
		m.jmembers(m.b[:1], jSolo, fmt.Sprintf("묶음 실패 뒤 맨 앞 혼자를 %s 에 추측 검증으로 맡김(앞 %d개는 %s)", v.Machine, len(m.b), where), "", m.r0, now)
	}
}

// markSuspect: 범인 표식 — 첫 줄 제출 SHA, 둘째 줄 그 근거(추측 실패의 첫 시험 줄 — 옆으로 뺌 일지에)
func (m *M) markSuspect(e entry, why string) {
	if err := writeVia(m.mj+"/suspect", e.s+"\n"+why+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 범인 표식을 적지 못했다(범인이 다음 묶음에 다시 묶일 수 있다): %v\n", err)
	}
}

// suspectRec: 범인 표식의 제출 SHA와 근거(없으면 빈 값)
func (m *M) suspectRec() (string, string) {
	b, err := os.ReadFile(m.mj + "/suspect")
	if err != nil {
		return "", ""
	}
	s, why, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(s), strings.TrimSpace(why)
}

// suspect: 범인 표식(<일지 폴더>/suspect)의 제출 SHA — 큐에서 혼자 묶인다(pickBatch·specGroups). 큐에 없으면 표식을 지우고 빈 값
func (m *M) suspect(q [][2]string) string {
	s, _ := m.suspectRec()
	if s == "" {
		return ""
	}
	for _, e := range q {
		if e[1] == s {
			return s
		}
	}
	removeQuiet(m.mj + "/suspect")
	return ""
}
