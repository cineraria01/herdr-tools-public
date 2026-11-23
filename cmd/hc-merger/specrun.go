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
var specLsPatterns = []string{verified.VerifiersRef, specRefs + "*", specFails + "*", specStarts + "*", verified.Prefix + "*", verified.PreRefs + "*"}

// specStartMax: HC_LANE_SPEC_START(초, 기본 300). 띄운 뒤 이만큼 지나도 일꾼의 떴다는 표식이 없으면 모름
func specStartMax() int64 {
	return int64(intSetting("HC_LANE_SPEC_START", os.Getenv("HC_LANE_SPEC_START"), 300, 0))
}

// specTimeout: HC_LANE_SPEC_TIMEOUT(초, 기본 3600, 60 이상). 넘긴 추측은 모름
func specTimeout() int64 {
	return int64(intSetting("HC_LANE_SPEC_TIMEOUT", os.Getenv("HC_LANE_SPEC_TIMEOUT"), 3600, 60))
}

// specFrontWait: HC_LANE_SPEC_FRONT_WAIT(초, 기본 600, 60 이상). 맨 앞 무리의 추측이 띄운 때(at)부터 이만큼 넘게 「도는 중」이면
// 맨 앞 판정은 모름 — 병합기 기계가 직접 검증한다(preverify-wait-cap). 미리 줄·큐 어느 쪽에서 온 추측이든 같다. 뒤쪽 무리와
// 일꾼 쪽 상한(HC_LANE_SPEC_TIMEOUT)은 그대로
func specFrontWait() int64 {
	return int64(intSetting("HC_LANE_SPEC_FRONT_WAIT", os.Getenv("HC_LANE_SPEC_FRONT_WAIT"), 600, 60))
}

// specCtx: 이번 바퀴에 읽은 원격 추측 상태
type specCtx struct {
	vers   []verified.Verifier
	rows   []specRow
	refs   map[string]string // 추측·결과 참조 → SHA(ls-remote)
	pre    []preMark         // 미리 줄 표식(specpre.go)
	preOff bool              // 검증 기계 목록이 미리 줄을 껐다
	preDel [][2]string       // 치울 표식(참조, 본 값)
}

// spec: 바퀴 갈고리 — 후보(m.b)가 정해진 뒤, 착지 전에. 빠른 착지면 그 추측 트리도 돌려준다
func (m *M) spec() (specDecision, string) {
	m.vw = vwhy{why: "모름"}
	c, ok := m.specLoad()
	if !ok {
		return specLand, ""
	}
	groups := m.specGroups()
	now := time.Now().Unix()
	busy := specBusy(c.rows)
	st := specWalk(m.btree, groups, c.rows)
	if m.specCulpritWhy(c, st) { // 뒤 추측은 반송할 맨 앞을 품었다 — 더 짓지 않고 치운다(미리 줄 추측은 지킨다)
		m.specClean(c, append(st[:len(st):len(st)], m.specPre(c, busy, now)...))
		return specCulprit, ""
	}
	m.specSolo(c, busy, now)
	for {
		st = specWalk(m.btree, groups, c.rows)
		last := st[len(st)-1]
		v := m.specFree(c.vers, busy)
		if last.row != nil || v.Machine == "" {
			break
		}
		if len(st) == 1 {
			v = verified.Verifier{} // 맨 앞은 병합기가 직접 검증 — 짓기만(뒤 추측의 부모)
		}
		r, ok := m.specBuild(st, v, now)
		if !ok {
			break
		}
		c.rows = append(c.rows, r)
		if r.state == specRun {
			busy[v.Machine] = true
		}
	}
	// 진짜 줄 추측을 다 챙긴 뒤 남은 빈 검증 기계에 미리 줄(specpre.go)
	m.specClean(c, append(st[:len(st):len(st)], m.specPre(c, busy, now)...))
	d := specFront(st)
	over := false
	if w := specFrontWait(); d == specWaitFront && now-st[0].row.at > w {
		over = true
		// 기다릴 값어치는 남은 시간이 직접 검증보다 짧을 때뿐 — 검증 기계가 죽었거나 바쁘거나 느리면 그 뒤 큐 전체가 멈춘다.
		// 그 추측은 통과도 실패도 아니다(반송·범인 판정에 안 씀): 지금 「모름」 길 그대로, 뒤늦은 결과는 specClean이 거둔다
		fmt.Printf("hc-lane merger: 미리 검증이 %d초 넘게 도는 중 — 병합기가 직접 검증한다(%s)\n", w, st[0].row.machine)
		d = specLand
	}
	m.vw.why = frontWhy(st, c.rows, over)
	switch d {
	case specLandFast:
		fmt.Printf("hc-lane merger: 추측 검증 통과 — %s (트리 %s, %s) 착지가 검증을 건너뛴다\n", st[0].g.lanes, first8(st[0].row.tree), st[0].row.machine)
		m.vw = vwhy{why: "통과", id: first8(st[0].row.id), mach: st[0].row.machine}
		return d, st[0].row.tree
	case specWaitFront:
		fmt.Printf("hc-lane merger: 추측 검증 도는 중 — %s (%s) 결과를 기다린다\n", st[0].g.lanes, st[0].row.machine)
	}
	return d, ""
}

// specSolo: 묶음이 실패해 줄인 바퀴(cap)에 앞 k−1(≥2)을 병합기가 직접 검증하는 동안 「맨 앞 혼자」를 빈 검증 기계에 먼저 맡긴다
// (merger-batch-culprit). 아이디는 다음 바퀴 맨 앞이 혼자일 때의 무리(기준 트리 + 맨 앞)와 같아, 그 바퀴 specWalk가 결과를 그대로 읽는다.
// 기계가 없거나 바쁘거나 이미 그 추측이 있으면 아무것도 안 한다(지금 그대로)
func (m *M) specSolo(c *specCtx, busy map[string]bool, now int64) {
	if m.capN() == 0 || len(m.b) < 2 {
		return
	}
	g := newGroup(m.b[:1])
	id := specID(m.btree, g.key)
	for _, r := range c.rows {
		if r.id == id {
			return
		}
	}
	v := m.specFree(c.vers, busy)
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
		m.jmembers(m.b[:1], jSolo, fmt.Sprintf("묶음 실패 뒤 맨 앞 혼자를 %s 에 추측 검증으로 맡김(앞 %d개는 병합기가 직접)", v.Machine, len(m.b)), "", m.r0, now)
	}
}

// specCulpritWhy: 묶음이 실패해 줄인 바퀴(cap)의 맨 앞 혼자 추측이 같은 기준에서 시험 실패로 끝났나 — 그렇다면 반송 까닭(m.cwhy·m.clog)을
// 채우고 true. 실패 기록에 시험 실패 근거(test=1)가 없거나(옛 판·환경 문제와 가를 수 없음) 그 기록을 못 읽으면 false(지금처럼 직접 검증)
func (m *M) specCulpritWhy(c *specCtx, st []specStep) bool {
	if m.capN() == 0 || len(st) == 0 || st[0].row == nil || st[0].row.state != specFail || len(st[0].g.shas) != 1 {
		return false
	}
	if b, err := os.ReadFile(m.mj + "/solo"); err != nil || strings.TrimSpace(string(b)) != st[0].row.id {
		fmt.Printf("hc-lane merger: 맨 앞 %s 의 실패한 추측은 이 cap 바퀴에 지은 혼자 추측이 아니다(미리 줄 등) — 지금처럼 직접 검증\n", st[0].g.lanes)
		return false
	}
	ref := specFails + st[0].row.id
	sha := c.refs[ref]
	if fetchr(true, "-q", "origin", ref) != 0 { // 받기만(FETCH_HEAD) — 로컬 참조를 남기지 않는다
		fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 혼자 추측의 실패 기록을 받지 못했다 — 지금처럼 직접 검증")
		return false
	}
	body, rc := out("cat-file", "commit", sha)
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 맨 앞 혼자 추측의 실패 기록을 읽지 못했다 — 지금처럼 직접 검증")
		return false
	}
	_, msg, _ := strings.Cut(body, "\n\n")
	subj, test, _ := strings.Cut(msg, "\n\n")
	kv := map[string]string{}
	for _, f := range strings.Fields(subj) {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	test = strings.TrimSpace(test)
	if !strings.HasPrefix(subj, "hc-merger-spec-fail ") || kv["test"] != "1" || test == "" || kv["exit"] == "" {
		fmt.Printf("hc-lane merger: 맨 앞 %s 혼자 추측 실패는 시험 실패와 환경 문제를 가를 수 없다 — 지금처럼 직접 검증\n", st[0].g.lanes)
		return false
	}
	lg := ""
	if _, rest, ok := strings.Cut(subj, " log="); ok {
		lg, _, _ = strings.Cut(rest, " exit=")
	}
	m.clog = kv["machine"] + ":" + lg
	m.cwhy = fmt.Sprintf("착지 실패(종료 %s, 추측 검증 — %s 에서 맨 앞 혼자 기준 %s 위 시험 실패, 첫 실패 「%s」)", kv["exit"], kv["machine"], first8(m.B), test)
	fmt.Printf("hc-lane merger: 맨 앞 %s 혼자 추측이 시험 실패(%s) — 병합기 검증 없이 반송\n", st[0].g.lanes, kv["machine"])
	return true
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
			strings.HasPrefix(f[1], verified.Prefix):
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
		if r := &c.rows[i]; r.state == specRun && c.refs[specStarts+r.id] == "" && now-r.at >= start {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 %s — %s 일꾼이 %d초 안에 뜨지 않았다(모름)\n", first8(r.id), r.machine, start)
			r.state = specLost
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
	m.specClean(c, keep)
}

// specGroups: 맨 앞은 이번 묶음, 그 뒤는 큐 차례(이번 묶음·반송 중·다른 기본 브랜치 뺌)를 묶음 크기씩. 제출 ID 없는 손 제출은 혼자.
// 제출 기록을 못 읽는 것에서 멈춘다
func (m *M) specGroups() []specGroup {
	gs := []specGroup{newGroup(m.b)}
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
			break
		}
		mt, ok := readMeta(refsha(m.qs, "refs/merge-queue-meta/"+mn), ms)
		if !ok {
			break
		}
		if mt.base != "" && mt.base != m.base {
			continue
		}
		if mt.id == "" {
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
	return gs
}

// specFree: 도는 추측이 없는 검증 기계(목록 차례). 병합기 기계 자신은 뺀다
func (m *M) specFree(vs []verified.Verifier, busy map[string]bool) verified.Verifier {
	for _, v := range vs {
		if !busy[v.Machine] && v.Machine != m.mme {
			return v
		}
	}
	return verified.Verifier{}
}

// specBuild: 길의 마지막 자리(줄 없음)의 추측을 임시 작업 트리에서 짓고 push한다. v가 있으면 그 기계에 띄운다(못 띄우면 state=lost).
// 지을 수 없으면(git 실패·push 거절) ok=false — 이 바퀴는 더 짓지 않는다
func (m *M) specBuild(st []specStep, v verified.Verifier, now int64) (specRow, bool) {
	s, mch := st[len(st)-1], v.Machine
	pc := m.B // 부모 커밋: 앞으로 나아간 마지막 추측 커밋(없으면 기준)
	for _, p := range st[:len(st)-1] {
		switch p.row.state {
		case specPass, specRun, specLost:
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
	cm, ok := commitTree("", tree, pc, specMsg(s.parent, s.g.key, machine, now, state))
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
		r.state = specRun
		if !m.specDispatch(v, id) {
			// 못 띄웠다 — 모름으로 고쳐 둔다(고치기도 못 하면 시간 넘김 뒤 모름)
			r.dispatchX, r.state, r.machine = true, specLost, "-"
			if lc, ok := commitTree("", tree, pc, specMsg(s.parent, s.g.key, "-", now, "lost")); ok &&
				runO("push", "-q", "--force-with-lease="+ref+":"+cm, "origin", lc+":"+ref) == 0 {
				r.commit = lc
			}
		} else {
			fmt.Printf("hc-lane merger: 추측 검증 띄움 — %s → %s (부모 트리 %s)\n", s.g.lanes, mch, first8(s.parent))
		}
	}
	return r, true
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
		mf, rc := out("merge-base", s, pc)
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

// specDispatch: 검증 기계에 일꾼을 띄운다(hc run --on, 상한 60초)
func (m *M) specDispatch(v verified.Verifier, id string) bool {
	_, rc, e := boundedRun(60*time.Second, "hc", nil, "run", "--bg", "--json", "--no-parent", "--rm", "--on", v.Machine,
		"--name", "spec-"+id[:8], "--label", "hc-merger-spec=1", "--cwd", v.Path, "hc-lane", "merger", "--spec-verify", id, m.base)
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 검증을 %s 에 띄우지 못했다(종료 %d): %s — 그 무리는 착지 때 직접 검증\n", v.Machine, rc, strings.TrimSpace(e))
		return false
	}
	return true
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
