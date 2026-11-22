package main

// 추측 병렬 검증 — 병합기 쪽(spec.go 머리말의 바퀴 1~4). 원격 참조가 일지다(로컬 상태 없음).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
var specLsPatterns = []string{verified.VerifiersRef, specRefs + "*", specFails + "*", specStarts + "*", verified.Prefix + "*"}

// specStartMax: HC_LANE_SPEC_START(초, 기본 300). 띄운 뒤 이만큼 지나도 일꾼의 떴다는 표식이 없으면 모름
func specStartMax() int64 {
	return int64(intSetting("HC_LANE_SPEC_START", os.Getenv("HC_LANE_SPEC_START"), 300, 0))
}

// specTimeout: HC_LANE_SPEC_TIMEOUT(초, 기본 3600, 60 이상). 넘긴 추측은 모름
func specTimeout() int64 {
	return int64(intSetting("HC_LANE_SPEC_TIMEOUT", os.Getenv("HC_LANE_SPEC_TIMEOUT"), 3600, 60))
}

// specCtx: 이번 바퀴에 읽은 원격 추측 상태
type specCtx struct {
	vers []verified.Verifier
	rows []specRow
	refs map[string]string // 추측·결과 참조 → SHA(ls-remote)
}

// spec: 바퀴 갈고리 — 후보(m.b)가 정해진 뒤, 착지 전에. 빠른 착지면 그 추측 트리도 돌려준다
func (m *M) spec() (specDecision, string) {
	c, ok := m.specLoad()
	if !ok {
		return specLand, ""
	}
	groups := m.specGroups()
	now := time.Now().Unix()
	busy := specBusy(c.rows)
	var st []specStep
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
	m.specClean(c, st)
	d := specFront(st)
	switch d {
	case specLandFast:
		fmt.Printf("hc-lane merger: 추측 검증 통과 — %s (트리 %s, %s) 착지가 검증을 건너뛴다\n", st[0].g.lanes, first8(st[0].row.tree), st[0].row.machine)
		return d, st[0].row.tree
	case specWaitFront:
		fmt.Printf("hc-lane merger: 추측 검증 도는 중 — %s (%s) 결과를 기다린다\n", st[0].g.lanes, st[0].row.machine)
	}
	return d, ""
}

// specLoad: m.qs(바퀴의 ls-remote)에서 추측 참조를 읽는다. 목록도 추측도 없거나 못 읽으면 ok=false(지금 그대로)
func (m *M) specLoad() (*specCtx, bool) {
	c := &specCtx{refs: map[string]string{}}
	vsha := ""
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
		}
	}
	if vsha == "" && len(c.refs) == 0 {
		return nil, false
	}
	// 추측 커밋(제목·트리)과 목록 커밋을 받는다. 원격에 없는 추측의 로컬 사본은 --prune이 거둔다
	args := []string{"-q", "--prune", "origin", "+" + specRefs + "*:" + specRefs + "*"}
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
		c.vers = verified.ParseVerifiers(msg)
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
	if c, ok := m.specLoad(); ok {
		m.specClean(c, nil)
	}
}

// specGroups: 맨 앞은 이번 묶음, 그 뒤는 큐 차례(이번 묶음·반송 중·다른 기본 브랜치 뺌)를 묶음 크기씩. 제출 ID 없는 손 제출은 혼자.
// 제출 기록을 못 읽는 것에서 멈춘다
func (m *M) specGroups() []specGroup {
	gs := []specGroup{newGroup(m.b)}
	n := m.batch
	if c, err := os.ReadFile(m.mj + "/cap"); err == nil {
		if k, err := strconv.Atoi(strings.TrimSpace(string(c))); err == nil && k > 0 && k < n {
			n = k
		}
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
