package main

// 추측 병렬 검증 — 병합기 쪽(spec.go 머리말의 바퀴 1~4). 원격 참조가 일지다(로컬 상태 없음).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/verified"
)

const (
	specRefs   = "refs/hc-merger/spec/"
	specFails  = "refs/hc-merger/spec-fail/"
	specStarts = "refs/hc-merger/spec-run/" // 일꾼이 떴다는 표식(못 뜬 일꾼을 한 시간 기다리지 않게)
)

// specLsPatterns: 바퀴의 ls-remote에 함께 싣는 추측 참조(round.go)
var specLsPatterns = []string{verified.VerifiersRef, specRefs + "*", specFails + "*", specStarts + "*", specParts + "*", specShards + "*", verified.Prefix + "*", verified.PreRefs + "*"}

// specStartMax: HC_LANE_SPEC_START(초, 기본 300). 띄운 뒤 이만큼 지나도 일꾼의 떴다는 표식이 없으면 모름
func specStartMax() int64 {
	return int64(intSetting("HC_LANE_SPEC_START", kenv("merger_spec_start", "HC_LANE_SPEC_START"), 300, 0))
}

// specTimeout: HC_LANE_SPEC_TIMEOUT(초, 기본 3600, 60 이상). 넘긴 추측은 모름
func specTimeout() int64 {
	return int64(intSetting("HC_LANE_SPEC_TIMEOUT", kenv("merger_spec_timeout", "HC_LANE_SPEC_TIMEOUT"), 3600, 60))
}

// specFrontWait: HC_LANE_SPEC_FRONT_WAIT(초, 기본 600, 60 이상). 맨 앞 무리의 추측이 띄운 때(at)부터 이만큼 넘게 「도는 중」이면
// 맨 앞 판정은 모름 — 병합기 기계가 직접 검증한다(preverify-wait-cap). 미리 줄·큐 어느 쪽에서 온 추측이든 같다. 뒤쪽 무리와
// 일꾼 쪽 상한(HC_LANE_SPEC_TIMEOUT)은 그대로
func specFrontWait() int64 {
	return int64(intSetting("HC_LANE_SPEC_FRONT_WAIT", kenv("merger_front_wait", "HC_LANE_SPEC_FRONT_WAIT"), 600, 60))
}

// specCtx: 이번 바퀴에 읽은 원격 추측 상태
type specCtx struct {
	vers    []verified.Verifier
	rows    []specRow
	refs    map[string]string  // 추측·결과 참조 → SHA(ls-remote)
	pre     []preMark          // 미리 줄 표식(specpre.go)
	preOff  bool               // 검증 기계 목록이 미리 줄을 껐다
	preDel  [][2]string        // 치울 표식(참조, 본 값)
	idle    map[string]int     // 이 바퀴에 잰 검증 기계 한가함(idUnknown·idIdle·idBusy·idGate — verifierpick.go), 처음 고를 때 채운다
	perCore map[string]float64 // 잰 기계의 코어당 부하(병합 검증 자리 차례)
	gpick   map[string]bool    // 이 바퀴에 병합 검증 자리로 고른 기계 — 추측 기다림 일지 줄의 「병합 검증 자리(<기계>)」
}

// spec: 바퀴 갈고리 — 후보(m.b)가 정해진 뒤, 착지 전에. 빠른 착지면 그 추측 트리도 돌려준다
func (m *M) spec() (specDecision, string) {
	d, t := m.specDecide()
	m.deadJournal(d, time.Now().Unix())
	m.freezeAt(d) // 기다리는 맨 앞 무리는 얼리고, 그 밖(착지·줄이기·반송)은 풀어 다음 바퀴는 줄 차례대로(frontsteady.go A)
	m.sparts = ""
	if d == specLand && m.sfid != "" { // 병합기 기계가 직접 검증 — 맨 앞 추측의 통과 조각을 넘긴다(쓸지는 검증 스크립트가)
		m.sparts = m.directParts(m.sfid)
	}
	return d, t
}

func (m *M) specDecide() (specDecision, string) {
	m.vw, m.wre, m.sfid, m.vers, m.sided = vwhy{why: "모름"}, "", "", nil, false
	c, ok := m.specLoad()
	if ok {
		m.vers = c.vers // 착지 실행 기계 후보(landon.go)
	} else {
		if m.vw.why == "모름" {
			m.vw.why = "추측 참조 못 읽음"
		}
		return specLand, ""
	}
	groups := m.specGroups()
	now := time.Now().Unix()
	m.specDead(c, groups, now) // 일꾼이 죽었다고 확신하는 도는 추측은 상한을 기다리지 않고 모름(specdead.go)
	if st := specWalk(m.btree, groups, c.rows); len(st) > 0 {
		shardSettle(c, st[0].row)          // 길에서 벗어난 나눔 추측은 몫이 다 끝나면 모름(specshards.go)
		m.offPathKill(c, m.keepIDs(c, st)) // 길에서 벗어난 도는 추측은 이 바퀴에 끝낸다(frontsteady.go C)
	}
	busy := specBusy(c.rows)
	st := specWalk(m.btree, groups, c.rows)
	if m.specCulpritWhy(c, st) { // 뒤 추측은 반송할 맨 앞을 품었다 — 더 짓지 않고 치운다(미리 줄 추측은 지킨다)
		m.specClean(c, append(st[:len(st):len(st)], m.specPre(c, busy, now)...))
		return specReject, ""
	}
	disp := m.frontDispatch()
	// 빈 검증 기계의 차례: 맨 앞 → 맨 앞 혼자(cap 바퀴) → 뒤 무리. 옛 길(맨 앞은 병합기가 직접)은 혼자를 먼저(감사 atozd0z-zklwye 중간 —
	// 혼자가 하나뿐인 기계를 먼저 가져가 맨 앞 k−1을 병합기가 직접 검증했다)
	par := m.parCap(disp) // 묶음 실패로 줄인 바퀴 — 앞 무리를 함께 가른다(culprit.go)
	soloDone := !disp || par
	if !disp {
		m.specSolo(c, busy, now, false)
	}
	var ff frontFail
	if disp { // 맨 앞이 실패·모름이면 뒤 추측보다 먼저 다시 띄운다(specfront.go — 한 번 더 확인 · 손잡이 front_local 0의 다시 띄움)
		ff = m.specFrontRedo(c, specWalk(m.btree, groups, c.rows), busy, now)
	}
	var ps []prefix
	if par {
		ps = m.prefixScan(c)
		if d, ok := m.culpritJump(c, ps, busy, now); ok { // 범인을 가렸거나 가운데가 시험 실패 — 뒤 무리를 짓기 전에 줄인다
			return d, ""
		}
		m.prefixDispatch(c, ps, busy, now)
	}
	for {
		st = specWalk(m.btree, groups, c.rows)
		if !soloDone && st[0].row != nil { // 맨 앞 자리를 정했다 — 혼자 다음, 뒤 무리는 그 뒤
			m.specSolo(c, busy, now, true)
			soloDone = true
			continue
		}
		last := st[len(st)-1]
		if last.row != nil {
			break
		}
		if m.sideStep(c, st) { // 옆으로 뺄 범인 — 추측하지 않고 그 자리를 실패로, 뒤 무리는 그 제출 없이(frontsteady.go B)
			continue
		}
		v := m.specFree(c, busy) // 실제로 한가한 기계(verifierpick.go — 이 바퀴 처음 고를 때 후보를 함께 잰다)
		if v.Machine == "" && len(st) == 1 && disp {
			v = m.preempt(c, busy, st) // 맨 앞이 먼저 — 미리 줄 추측을 끝내고 그 기계(frontsteady.go C)
		}
		if v.Machine == "" {
			break
		}
		if len(st) == 1 && !disp {
			v = verified.Verifier{} // 손잡이 front_dispatch 0 — 맨 앞은 병합기가 직접 검증, 짓기만(뒤 추측의 부모)
		}
		// 맨 앞 무리도 뒤 무리와 같은 꼴로 검증 기계에 맡긴다(merger-verify-dispatch) — 병합기 기계는 기록을 보고 넣기만.
		// 맨 앞은 손잡이 merger_shards대로 여러 기계에 나눠(specshards.go)
		var more []verified.Verifier
		if len(st) == 1 {
			v, more = m.shardMore(c, busy, v)
		}
		r, ok := m.specBuild(st, v, now, more...)
		if !ok {
			break
		}
		c.rows = append(c.rows, r)
		if r.state == specRun {
			busy[v.Machine] = true
		}
	}
	if !soloDone { // 맨 앞을 못 지음(빈 기계 없음·짓기 실패) — 지금 그대로 혼자에 기회를
		m.specSolo(c, busy, now, true)
	}
	// 진짜 줄 추측을 다 챙긴 뒤 남은 빈 검증 기계에 미리 줄(specpre.go). 맨 앞 혼자 추측은 길 밖이어도 지킨다 — 묶음이 검증 기계에서
	// 실패해 줄이는 바퀴(specShrink)는 혼자 추측이 끝난 뒤라, 치우면 범인 반송 근거를 잃고 다시 돌린다
	m.specClean(c, append(append(append(st[:len(st):len(st)], m.specPre(c, busy, now)...), m.soloKeep(c)...), prefixKeep(c, ps)...))
	if st[0].row != nil && st[0].row.commit != "" { // 옆으로 뺀 자리(원격 추측 없음)는 넘길 조각도 없다
		m.sfid = st[0].row.id
	}
	if par {
		if d, ok := m.prefixWait(c, ps, st[0].row, now); ok {
			return d, ""
		}
	}
	if disp {
		m.shardStep(c, st, busy, now) // 나눔 중인 맨 앞 — 몫이 다 끝났거나 상한이면 마무리(specshards.go)
	}
	d, over, stall := m.frontOver(c, st, specFront(st), busy, now, disp) // 맨 앞 기다림 상한(specfront.go)
	m.vw.why = frontWhy(st, c.rows, over, disp)
	if over && stall != "" {
		m.vw.why += "·" + stall // 상한에서 직접 검증한 까닭(갱신 없음·터미널 없음·못 읽음)
	}
	if m.sided {
		m.vw.why = "옆으로 뺌"
	}
	m.wmach = ""
	switch {
	case d == specWaitFront:
		m.wmach, m.wrow = st[0].row.machine, *st[0].row
		if c.gpick[m.wmach] {
			m.wmach = "*" + m.wmach // 병합 검증 자리로 맡김(jround.go specWaitNote가 같은 기다림 동안 잇는다)
		}
	case d == specLand && disp:
		d = m.specFrontLand(c, st, ff) // 반송·줄이기·직접 검증·기다림(손잡이 front_local 0)
	}
	switch {
	case d == specLandFast:
		fmt.Printf("hc-lane merger: 추측 검증 통과 — %s (트리 %s, %s) 착지가 검증을 건너뛴다\n", st[0].g.lanes, first8(st[0].row.tree), st[0].row.machine)
		m.vw = vwhy{why: "통과", id: first8(st[0].row.id), mach: st[0].row.machine}
		return d, st[0].row.tree
	case d == specWaitFront && m.wmach != "-":
		fmt.Printf("hc-lane merger: 추측 검증 도는 중 — %s (%s) 결과를 기다린다\n", st[0].g.lanes, st[0].row.machine)
	}
	return d, ""
}

// specCulpritWhy: 묶음이 실패해 줄인 바퀴(cap)의 맨 앞 혼자 추측이 같은 기준에서 시험 실패로 끝났나 — 그렇다면 반송 까닭(m.cwhy·m.clog)을
// 채우고 true. 실패 기록에 시험 실패 근거(test=1)가 없거나(옛 판·환경 문제와 가를 수 없음) 그 기록을 못 읽으면 false(지금처럼 직접 검증)
func (m *M) specCulpritWhy(c *specCtx, st []specStep) bool {
	if m.capN() == 0 || len(st) == 0 || st[0].row == nil || st[0].row.state != specFail || len(st[0].g.shas) != 1 {
		return false
	}
	if m.hb != "" { // 착지 스크립트 저장소 — 추측 실패만으로 반송하지 않는다(반송은 착지 스크립트의 실패로만, specfront.go)
		return false
	}
	if b, err := os.ReadFile(m.mj + "/solo"); err != nil || strings.TrimSpace(string(b)) != st[0].row.id {
		fmt.Printf("hc-lane merger: 맨 앞 %s 의 실패한 추측은 이 cap 바퀴에 지은 혼자 추측이 아니다(미리 줄 등) — 지금처럼 직접 검증\n", st[0].g.lanes)
		return false
	}
	kv, test, ok := m.specFailRec(c, st[0].row.id)
	if !ok || kv["test"] != "1" || test == "" || kv["exit"] == "" {
		fmt.Printf("hc-lane merger: 맨 앞 %s 혼자 추측 실패는 시험 실패와 환경 문제를 가를 수 없다 — 지금처럼 직접 검증\n", st[0].g.lanes)
		return false
	}
	m.clog = kv["machine"] + ":" + kv["log"]
	m.cwhy = fmt.Sprintf("착지 실패(종료 %s, 추측 검증 — %s 에서 맨 앞 혼자 기준 %s 위 시험 실패, 첫 실패 「%s」)", kv["exit"], kv["machine"], first8(m.B), test)
	fmt.Printf("hc-lane merger: 맨 앞 %s 혼자 추측이 시험 실패(%s) — 병합기 검증 없이 반송\n", st[0].g.lanes, kv["machine"])
	return true
}

// specFailRec: 추측 실패 기록(spec-fail 참조)의 제목 칸(id·machine·at·log·exit·test)과 본문(첫 실패 시험 줄). 못 받거나 못 읽거나 꼴이
// 다르면 ok=false(모름)
func (m *M) specFailRec(c *specCtx, id string) (kv map[string]string, test string, ok bool) {
	ref := specFails + id
	if fetchr(true, "-q", "origin", ref) != 0 { // 받기만(FETCH_HEAD) — 로컬 참조를 남기지 않는다
		fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 추측의 실패 기록을 받지 못했다(모름)")
		return nil, "", false
	}
	body, rc := out("cat-file", "commit", c.refs[ref])
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 추측의 실패 기록을 읽지 못했다(모름)")
		return nil, "", false
	}
	_, msg, _ := strings.Cut(body, "\n\n")
	subj, test, _ := strings.Cut(msg, "\n\n")
	if !strings.HasPrefix(subj, "hc-merger-spec-fail ") {
		return nil, "", false
	}
	kv = map[string]string{}
	for _, f := range strings.Fields(subj) {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	kv["log"] = ""
	if _, rest, ok := strings.Cut(subj, " log="); ok { // 로그 경로에 빈칸이 있을 수 있다
		kv["log"], _, _ = strings.Cut(rest, " exit=")
	}
	return kv, strings.TrimSpace(test), true
}

// specLoad: m.qs(바퀴의 ls-remote)에서 추측 참조를 읽는다. 목록도 추측도 없거나 못 읽으면 ok=false(지금 그대로)
func (m *M) specLoad() (*specCtx, bool) {
	c := &specCtx{refs: map[string]string{}}
	vsha, pre := "", map[string]string{}
	for _, line := range strings.Split(m.qs, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		switch {
		case f[1] == verified.VerifiersRef:
			vsha = f[0]
		case strings.HasPrefix(f[1], specRefs), strings.HasPrefix(f[1], specFails), strings.HasPrefix(f[1], specStarts),
			strings.HasPrefix(f[1], specParts), strings.HasPrefix(f[1], specShards), strings.HasPrefix(f[1], verified.Prefix):
			c.refs[f[1]] = f[0]
		case strings.HasPrefix(f[1], verified.PreRefs):
			pre[f[1]] = f[0]
		}
	}
	if vsha == "" && len(c.refs) == 0 && len(pre) == 0 {
		m.vw.why = "추측 없음"
		return nil, false
	}
	// 추측 커밋(제목·트리)과 목록 커밋을 받는다. 원격에 없는 추측의 로컬 사본은 --prune이 거둔다
	args := []string{"-q", "--prune", "origin", "+" + specRefs + "*:" + specRefs + "*", "+" + verified.PreRefs + "*:" + preLocal + "*"}
	if vsha != "" {
		args = append(args, "+"+verified.VerifiersRef+":"+verified.VerifiersRef)
	}
	if fetchr(true, args...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 추측 참조를 받지 못했다(git fetch) — 이 바퀴는 추측 없이")
		return nil, false
	}
	if vsha != "" {
		body, rc := out("cat-file", "commit", vsha)
		if rc != 0 {
			return nil, false
		}
		_, msg, _ := strings.Cut(body, "\n\n")
		c.vers, c.preOff = verified.ParseVerifiers(msg), verified.PreOff(msg)
	}
	var ok bool
	if c.pre, ok = preRead(pre); !ok {
		return nil, false
	}
	list, rc := out("for-each-ref", "--format=%(refname) %(objectname) %(tree) %(contents:subject)", specRefs)
	if rc != 0 {
		return nil, false
	}
	for _, line := range strings.Split(list, "\n") {
		f := strings.SplitN(line, " ", 4)
		if len(f) != 4 || c.refs[f[0]] != f[1] {
			continue // 받는 사이 바뀐 것은 다음 바퀴에
		}
		r, ok := parseSpec(f[3])
		if !ok {
			continue
		}
		r.id, r.commit, r.tree = strings.TrimPrefix(f[0], specRefs), f[1], f[2]
		c.rows = append(c.rows, r)
	}
	failed, ver := map[string]bool{}, map[string]bool{}
	for ref := range c.refs {
		if id, ok := strings.CutPrefix(ref, specFails); ok {
			failed[id] = true
		} else if t, ok := strings.CutPrefix(ref, verified.Prefix); ok {
			ver[t] = true
		}
	}
	now, start := time.Now().Unix(), specStartMax()
	specResolve(c.rows, ver, failed, now, specTimeout())
	for i := range c.rows {
		r := &c.rows[i]
		switch {
		case r.state == specRun && c.refs[specStarts+r.id] == "" && now-r.at >= start:
			fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 %s — %s 일꾼이 %d초 안에 뜨지 않았다(모름)\n", first8(r.id), r.machine, start)
			r.state, r.lost = specLost, "안 뜸"
		case r.dispatchX && c.refs[specStarts+r.id] != "": // 일꾼이 떴는데 state=lost로 끝냄(준비 실패·검증이 트리를 바꿈)
			r.lost = "준비 실패"
		case r.dispatchX && r.machine == "-":
			r.lost = "못 띄움"
		}
	}
	return c, true
}

// specIdle: 큐가 비었다 — 도는 중이 아닌 추측과 짝 없는 결과를 치운다(추측 참조가 없으면 원격 호출 없음)
func (m *M) specIdle() {
	c, ok := m.specLoad()
	if !ok {
		return
	}
	var keep []specStep
	if len(c.pre) > 0 { // 미리 줄은 큐가 빈 바퀴에도 본다 — 기준을 못 받으면 이 바퀴는 치우지도 않는다(미리 줄 추측을 길 밖으로 읽지 않게)
		if !m.preBase() {
			return
		}
		keep = m.specPre(c, specBusy(c.rows), time.Now().Unix())
	}
	m.offPathKill(c, stepIDs(keep)) // 줄이 비었다 — 미리 줄 말고 도는 추측은 쓸 데가 없다(frontsteady.go C)
	m.specClean(c, keep)
}

// specGroups: 맨 앞은 이번 묶음, 그 뒤는 큐 차례(이번 묶음·반송 중·다른 기본 브랜치 뺌)를 묶음 크기씩. 제출 ID 없는 손 제출은 혼자.
// 제출 기록을 못 읽는 것에서 멈춘다
func (m *M) specGroups() []specGroup {
	gs := []specGroup{newGroup(m.b)}
	m.gpartial = true // 끝까지 셌을 때만 거짓
	n := m.batch
	if k := m.capN(); k > 0 && k < n {
		n = k
	}
	in := map[string]bool{}
	for _, e := range m.b {
		in[e.q] = true
	}
	q, ok := m.queue()
	if !ok {
		return gs
	}
	sus := m.suspect(q)
	var cur []entry
	flush := func() {
		if len(cur) > 0 {
			gs = append(gs, newGroup(cur))
			cur = nil
		}
	}
	for _, e := range q {
		mq, ms := e[0], e[1]
		mn := strings.TrimPrefix(mq, "refs/merge-queue/")
		if in[mq] {
			continue
		}
		if _, err := os.Stat(m.mj + "/fail/" + mn); err == nil {
			continue
		}
		ln := ""
		if len(mn) > 11 && digits.MatchString(mn[:10]) && mn[10] == '-' {
			ln = mn[11:]
		}
		if !laneOK(ln) || m.reserved(ln) {
			flush()
			return gs
		}
		mt, ok := readMeta(refsha(m.qs, "refs/merge-queue-meta/"+mn), ms)
		if !ok {
			flush()
			return gs
		}
		if mt.base != "" && mt.base != m.base {
			continue
		}
		if mt.id == "" || ms == sus { // 손 제출·앞 무리로 가린 범인(pickBatch와 같이 혼자)
			flush()
			cur = []entry{{q: mq, s: ms, l: ln}}
			flush()
			continue
		}
		cur = append(cur, entry{q: mq, s: ms, l: ln})
		if len(cur) >= n {
			flush()
		}
	}
	flush()
	m.gpartial = false
	return gs
}

// specBuild: 길의 마지막 자리(줄 없음)의 추측을 임시 작업 트리에서 짓고 push한다. v가 있으면 그 기계에 띄운다(못 띄우면 state=lost).
// 지을 수 없으면(git 실패·push 거절) ok=false — 이 바퀴는 더 짓지 않는다
func (m *M) specBuild(st []specStep, v verified.Verifier, now int64, more ...verified.Verifier) (specRow, bool) {
	s, mch := st[len(st)-1], v.Machine
	pc := m.B // 부모 커밋: 앞으로 나아간 마지막 추측 커밋(없으면 기준)
	for _, p := range st[:len(st)-1] {
		switch p.row.state {
		case specPass, specRun, specLost, specBusyX:
			pc = p.row.commit
		}
	}
	id := specID(s.parent, s.g.key)
	tree, state := s.parent, "local"
	if mch != "" {
		state = "run"
	}
	merges, rc := out(append(append([]string{"rev-list", "-1", "--merges"}, s.g.shas...), "^"+pc)...)
	if rc != 0 {
		return specRow{}, false
	}
	if merges != "" {
		state = "conflict" // 병합 풀이가 있는 제출은 추측하지 않는다(착지가 원본 그대로 판정)
	} else {
		t, ok, built := m.specStack(pc, s.g.shas)
		if !ok {
			return specRow{}, false
		}
		if built {
			tree = t
		} else {
			state = "conflict"
		}
	}
	machine := mch
	if state != "run" {
		machine = "-"
	}
	r := specRow{id: id, tree: tree, parent: s.parent, group: s.g.key, machine: machine, at: now}
	msg := specMsg(s.parent, s.g.key, machine, now, state)
	if s.note != "" {
		msg += "\n\n" + s.note
	}
	cm, ok := commitTree("", tree, pc, msg)
	if !ok {
		return specRow{}, false
	}
	ref := specRefs + id
	if runO("push", "-q", "--force-with-lease="+ref+":", "origin", cm+":"+ref) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 %s 참조를 올리지 못했다 — 이 바퀴는 더 짓지 않는다\n", first8(id))
		return specRow{}, false
	}
	r.commit = cm
	switch state {
	case "conflict":
		r.conflict, r.state = true, specConflict
		fmt.Printf("hc-lane merger: 추측 %s — 부모 위에 얹으면 충돌(또는 병합 커밋), 건너뛴다\n", s.g.lanes)
	case "local":
		r.dispatchX, r.state = true, specLost
	case "run":
		m.specLaunch(&r, v, s.g.lanes, now, more...)
	}
	return r, true
}

// specLaunch: 원격에 state=run으로 올린 추측 r을 기계 v에(more가 있으면 v와 more에 몫으로 나눠 — specshards.go) 띄운다.
// 못 띄우면 모름(state=lost)으로 고쳐 둔다(고치기도 못 하면 시간 넘김 뒤 모름)
func (m *M) specLaunch(r *specRow, v verified.Verifier, lanes string, now int64, more ...verified.Verifier) {
	r.state = specRun
	if len(more) > 0 {
		if m.shardLaunch(r, append([]verified.Verifier{v}, more...), lanes, now) {
			return
		}
	} else if m.specDispatch(v, r.id, "") {
		fmt.Printf("hc-lane merger: 추측 검증 띄움 — %s → %s (부모 트리 %s)\n", lanes, v.Machine, first8(r.parent))
		return
	}
	ref, cm := specRefs+r.id, r.commit
	r.dispatchX, r.state, r.machine, r.lost = true, specLost, "-", "못 띄움"
	if lc, ok := commitTree("", r.tree, cm+"^", specMsg(r.parent, r.group, "-", now, "lost")); ok &&
		runO("push", "-q", "--force-with-lease="+ref+":"+cm, "origin", lc+":"+ref) == 0 {
		r.commit = lc
	}
}

// specStack: 부모 커밋 위에 제출들을 차례로 얹은 트리(착지의 rebase --reapply-cherry-picks와 같은 명령). 충돌이면 built=false
func (m *M) specStack(pc string, shas []string) (tree string, ok, built bool) {
	wt, err := mktempDir(m.mwr)
	if err != nil {
		return "", false, false
	}
	defer func() {
		if run("worktree", "remove", "-f", "-f", wt) != 0 {
			os.RemoveAll(wt) // 다음 wtclean이 등록을 거둔다
		}
	}()
	lg := m.lgf // 이 바퀴의 착지 로그 뒤에
	if inLog("", lg, "worktree", "add", "-q", "-f", "--detach", wt, pc) != 0 {
		return "", false, false
	}
	cur := pc
	for _, s := range shas {
		mf, rc := m.forkOf(s, pc) // 쌓인 제출은 P 뒤 커밋만(stack.go)
		if rc != 0 {
			return "", false, false
		}
		if inLog(wt, lg, "rebase", "-q", "--reapply-cherry-picks", "--onto", cur, mf, s) != 0 {
			inLog(wt, lg, "rebase", "--abort")
			return "", true, false
		}
		if cur, rc = gitx.Out(wt, os.Stderr, "rev-parse", "HEAD"); rc != 0 {
			return "", false, false
		}
	}
	t, rc := gitx.Out(wt, os.Stderr, "rev-parse", "HEAD^{tree}")
	if rc != 0 {
		return "", false, false
	}
	return t, true, true
}

// specClean: 길에 없고 도는 중도 아닌 추측과, 남는 추측 어디에도 안 맞는 결과(spec-fail·verified)를 지운다(본 값 lease, 실패는 다음 바퀴에)
func (m *M) specClean(c *specCtx, st []specStep) {
	stale := map[string]bool{}
	for _, r := range specStale(st, c.rows) {
		stale[r.id] = true
	}
	keepID, keepTree := map[string]bool{}, map[string]bool{}
	for _, r := range c.rows {
		if !stale[r.id] {
			keepID[r.id], keepTree[r.tree] = true, true
		}
	}
	var args []string
	del := func(ref, sha string) { args = append(args, "--force-with-lease="+ref+":"+sha, ":"+ref) }
	for _, p := range c.preDel {
		del(p[0], p[1])
	}
	for _, r := range c.rows {
		if stale[r.id] && c.refs[specRefs+r.id] == r.commit {
			del(specRefs+r.id, r.commit)
		}
	}
	for ref, sha := range c.refs {
		if id, ok := strings.CutPrefix(ref, specFails); ok && !keepID[id] {
			del(ref, sha)
		} else if id, ok := strings.CutPrefix(ref, specStarts); ok && !keepID[id] {
			del(ref, sha)
		} else if id, ok := strings.CutPrefix(ref, specParts); ok && !keepID[shardBase(id)] {
			del(ref, sha)
		} else if id, ok := strings.CutPrefix(ref, specShards); ok && !keepID[shardBase(id)] {
			del(ref, sha)
		} else if t, ok := strings.CutPrefix(ref, verified.Prefix); ok && !keepTree[t] {
			del(ref, sha)
		}
	}
	if len(args) == 0 {
		return
	}
	if runO(append([]string{"push", "-q", "origin"}, args...)...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 끝난 추측 참조를 다 치우지 못했다 — 다음 바퀴에 다시")
	}
}

// commitTree: 고정 작성자로 커밋을 만든다(parent가 빈 값이면 부모 없음)
func commitTree(dir, tree, parent, msg string) (string, bool) {
	args := []string{"commit-tree", tree, "-m", msg}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=hc-merger", "GIT_AUTHOR_EMAIL=hc-merger@localhost",
		"GIT_COMMITTER_NAME=hc-merger", "GIT_COMMITTER_EMAIL=hc-merger@localhost")
	var o bytes.Buffer
	c.Stdout, c.Stderr = &o, os.Stderr
	if proc.Code(proc.Run(c)) != 0 {
		return "", false
	}
	return strings.TrimSpace(o.String()), true
}
