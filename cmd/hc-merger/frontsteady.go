package main

// 맨 앞을 흔들지 않기(ht-merger-front-steady, 2026-11-26 사용자 「시간 지금 계속 낭비 되는 부분을 조사 해서 문제점 너가 책임 지고 수정해서
// 문제없게 만들어줘」 — 48시간 병합 줄 조사의 큰 차례 셋). 새 검증 길·새 통과 기록은 없다 — 차례와 버리는 때만 바꾼다.
//
//   A 얼림(손잡이 front_freeze, 기본 1): 맨 앞 무리를 기다린(추측 도는 중·통과 기다림) 바퀴는 그 구성원을 <일지 폴더>/front에 적고(「<기준>
//     <제출 SHA…>」), 다음 바퀴의 pickBatch는 그 무리를 그대로 고른다 — 새 제출·다시 넣은 제출·우선이 더 높은 제출은 다음 무리로(뒤 무리 추측
//     길 그대로 그 위에 추측). 고치기 전: 맨 앞 추측을 기다리는 사이 새 제출이 들어오면 묶음을 다시 지어 아이디가 바뀌고, 옛 추측이 그 기계를
//     쥔 채 「맡길 한가한 기계 없음」으로 처음부터 직접 검증했다(48시간 19건). 구성원이 빠졌거나(거둠·반송·이미 들어감)·기준이 움직였으면 지금처럼
//     다시 짓는다. 줄이기·반송·착지로 무리가 끝나면 지운다 — 그다음 바퀴는 줄 차례(우선 → 시각)대로.
//   B 옆으로 빼기(손잡이 aside, 기본 1 — 착지 스크립트 저장소만): 추측 실패는 반송 근거가 아니라 차례 근거다. 묶음 맨 앞 추측이 시험 실패면
//     직접 검증으로 다시 태우지 않고 곧바로 줄여(cap) 범인 가르기(앞 무리 함께 — culprit.go)를 추측으로 돈다. 의심 구성원(통과한 앞 무리 바로
//     다음·혼자 추측이 시험 실패한 맨 앞)은 확인 추측 없이 혼자 착지 스크립트 착지(직접 검증)로 보내고 — 실패면 지금 반송 규칙, 통과면 그대로
//     착지 — 그동안 뒤 무리는 그 제출 없이(그 자리를 실패로 보고 부모 그대로) 추측한다. 이 저장소에는 「검증만 하는 착지」가 없어(착지 스크립트가
//     rebase·검증·push를 한 번에) 혼자 착지 검증 = 혼자 착지 실행이다.
//   C 검증 기계 차례: 길에서 벗어난 도는 추측(옛 기준·바뀐 무리·거둔 제출)은 벗어난 그 바퀴 처음에 그 터미널을 정확한 주소로 끝내고 참조를
//     치운다(끝까지 돌게 두지 않는다). 맨 앞·범인 가르기가 맡길 기계를 못 얻었는데 미리 줄 추측이 기계를 쥐고 있으면 그것을 끝내고 그 기계를 쓴다.

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"herdr-tools/internal/verified"
)

const (
	jFreeze  = "얼림"      // 도는 맨 앞 무리를 그대로 두고 새 제출을 다음 무리로(구성원마다 한 줄, 새 제출마다 한 번)
	jSide    = "옆으로 뺌"   // 착지 스크립트 저장소 — 추측이 시험 실패한 의심 구성원을 혼자 착지 검증으로(맨 앞 제출마다 한 번)
	jPreKill = "미리 줄 끝냄" // 맨 앞에 검증 기계를 내주려 미리 줄 추측을 끝냄
)

// steady: M에 묻는 바퀴 상태
type steady struct {
	sided    bool // 이 바퀴 맨 앞을 옆으로 뺐다(일지 까닭 「직접 검증(옆으로 뺌)」)
	gpartial bool // specGroups가 제출 기록을 못 읽어 뒤 무리를 다 못 셈(길 밖 추측을 끝내지 않는다)
}

// freezeOn: 손잡이 front_freeze(HC_LANE_MERGER_FRONT_FREEZE, 기본 1). 0이면 지금 길(바퀴마다 줄 차례로 다시 고름)
func (m *M) freezeOn() bool { return m.knob("front_freeze", "HC_LANE_MERGER_FRONT_FREEZE") }

// sideline: 손잡이 aside(merger_aside, HC_LANE_MERGER_ASIDE, 기본 1). 0이면 착지 스크립트 저장소는 지금 길(추측 실패는 직접 검증 전
// 한가한 기계에 한 번 더, 그다음 묶음째 직접 검증)
func (m *M) sideline() bool { return m.knob("aside", "HC_LANE_MERGER_ASIDE") }

func (m *M) frontPath() string { return m.mj + "/front" }

// freezeRead: 얼린 무리 — 기준·구성원 SHA(큐 차례)·이미 일지에 적은 새 제출 레인
func (m *M) freezeRead() (base string, shas []string, noted []string, ok bool) {
	b, err := os.ReadFile(m.frontPath())
	if err != nil {
		return "", nil, nil, false
	}
	ls := strings.Split(strings.TrimSpace(string(b)), "\n")
	f := strings.Fields(ls[0])
	if len(f) < 2 {
		return "", nil, nil, false
	}
	return f[0], f[1:], ls[1:], true
}

// freezeWrite: 맨 앞 무리를 기다린 바퀴 — 기준 base 위 그 구성원을 얼린다(같은 무리면 일지에 적은 새 제출 레인은 이어 둔다)
func (m *M) freezeWrite(base string, noted []string) {
	if !m.freezeOn() {
		return
	}
	line := base + " " + strings.Join(shas(m.b), " ")
	if b, s, n, ok := m.freezeRead(); ok && noted == nil && b == base && slices.Equal(s, shas(m.b)) {
		noted = n
	}
	if err := writeVia(m.frontPath(), strings.Join(append([]string{line}, noted...), "\n")+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 얼린 맨 앞을 적지 못했다(다음 바퀴는 줄 차례로 다시 고른다): %v\n", err)
	}
}

func (m *M) freezeClear() { removeQuiet(m.frontPath()) }

// freezeAt: 추측 판정 뒤 — 맨 앞(또는 범인 가르기 앞 무리)이 도는 동안 기다리면 얼리고, 그 밖(착지·줄이기·반송, 손잡이 front_local 0으로
// 도는 것 없이 쉼 — 감사 atp3xrb 낮음 3)은 지운다
func (m *M) freezeAt(d specDecision) {
	if d == specWaitFront && m.wmach != "-" {
		m.freezeWrite(m.B, nil)
		return
	}
	m.freezeClear()
}

// freezeOrder: pickBatch가 고르기 전에 큐(*qp)·묶음 크기(*np)를 얼린 무리대로 바꾸고, 고른 뒤 부를 일지 함수를 돌려준다
func (m *M) freezeOrder(qp *[][2]string, np *int) func() {
	q0, n0 := *qp, *np
	var fz []string
	*qp, *np, fz = m.freezeHead(q0, n0)
	return func() { m.freezeNote(q0, n0, fz) }
}

// stepIDs: 단계들의 추측 아이디
func stepIDs(st []specStep) map[string]bool {
	k := map[string]bool{}
	for _, s := range st {
		if s.row != nil {
			k[s.row.id] = true
		}
	}
	return k
}

// sideStep: 짓기 고리 — 길의 마지막 자리가 옆으로 뺄 범인이면 추측 대신 실패 자리(sideVirtual)를 두고 true. 맨 앞이면 옆으로 뺌 일지
func (m *M) sideStep(c *specCtx, st []specStep) bool {
	last := st[len(st)-1]
	if !m.sideGroup(last.g) {
		return false
	}
	c.rows = append(c.rows, sideVirtual(last))
	if len(st) == 1 {
		_, why := m.suspectRec()
		m.sideNote(last.g, why)
	}
	return true
}

// freezeHead: 얼린 무리가 아직 그대로면(같은 기준, 구성원 모두 큐에 있고 반송 중 아님) 그 구성원을 적은 차례로 큐 맨 앞에 놓고 묶음 크기를
// 그 수로. 아니면 표식을 지우고 지금 그대로
func (m *M) freezeHead(q [][2]string, n int) ([][2]string, int, []string) {
	if !m.freezeOn() {
		return q, n, nil
	}
	base, fz, _, ok := m.freezeRead()
	if !ok {
		return q, n, nil
	}
	if base != refsha(m.qs, "refs/heads/"+m.base) {
		m.freezeClear() // 기준이 움직였다 — 그 추측은 어차피 안 쓰인다
		return q, n, nil
	}
	at := map[string]int{}
	for i, e := range q {
		at[e[1]] = i
	}
	in := map[string]bool{}
	var head [][2]string
	for _, s := range fz {
		i, found := at[s]
		if !found || m.retrying(q[i][0]) {
			m.freezeClear() // 구성원이 빠짐(거둠·반송·이미 들어감) — 다시 짓는다
			return q, n, nil
		}
		head, in[s] = append(head, q[i]), true
	}
	for _, e := range q {
		if !in[e[1]] {
			head = append(head, e)
		}
	}
	return head, len(fz), fz
}

func (m *M) retrying(mq string) bool {
	_, err := os.Stat(m.mj + "/fail/" + strings.TrimPrefix(mq, "refs/merge-queue/"))
	return err == nil
}

// freezeNote: pickBatch 뒤 — 고른 묶음이 얼린 무리와 다르면 표식을 지운다. 같으면 지금 길이었다면 맨 앞 무리에 들었을 제출(줄 차례 앞 n0개
// 가운데 얼린 무리 밖)을 「얼림 — 새 제출 <레인> 다음 무리로」로 일지에 한 번씩
func (m *M) freezeNote(q0 [][2]string, n0 int, fz []string) {
	if fz == nil {
		return
	}
	if !slices.Equal(shas(m.b), fz) {
		m.freezeClear()
		return
	}
	base, _, noted, _ := m.freezeRead()
	var lanes []string
	k := 0
	for _, e := range q0 {
		if m.retrying(e[0]) {
			continue
		}
		if k++; k > n0 {
			break
		}
		l := strings.TrimPrefix(e[0], "refs/merge-queue/")
		if len(l) > 11 {
			l = l[11:]
		}
		if !slices.Contains(fz, e[1]) && !slices.Contains(noted, l) {
			lanes = append(lanes, l)
		}
	}
	if len(lanes) == 0 {
		return
	}
	what := "얼림 — 새 제출 " + strings.Join(lanes, "·") + " 다음 무리로"
	fmt.Printf("hc-lane merger: %s(맨 앞 추측을 그대로 둔다)\n", what)
	m.jmembers(m.b, jFreeze, what, "", m.r0, time.Now().Unix())
	m.freezeWrite(base, append(noted, lanes...))
}

// sideGroup: 착지 스크립트 저장소에서 옆으로 뺄 무리인가 — 범인 표식(suspect)의 제출 혼자
func (m *M) sideGroup(g specGroup) bool {
	if m.hb == "" || !m.sideline() || len(g.shas) != 1 {
		return false
	}
	s, _ := m.suspectRec()
	return s != "" && s == g.shas[0]
}

// sideVirtual: 옆으로 뺀 무리의 자리 — 추측을 짓지 않고 「실패」로 둔다(원격에 안 올림, commit 빈 값). 길은 그 자리를 건너뛰어 뒤 무리를 그 제출
// 없이 부모 위에 추측한다. 맨 앞이면 착지 스크립트 착지로(specFrontLand)
func sideVirtual(s specStep) specRow {
	return specRow{id: specID(s.parent, s.g.key), tree: s.parent, parent: s.parent, group: s.g.key, machine: "-", state: specFail}
}

// sideNote: 맨 앞을 옆으로 뺀다 — 화면에 바퀴마다, 일지에 맨 앞 제출마다 한 줄(<일지 폴더>/sidenote에 그 SHA). test = 추측 실패의 첫 시험 줄
func (m *M) sideNote(g specGroup, test string) {
	m.sided = true
	what := fmt.Sprintf("옆으로 뺌 %s(추측 실패 %s) — 혼자 착지 검증", g.lanes, test)
	fmt.Printf("hc-lane merger: %s(뒤 무리는 그 제출 없이 추측)\n", what)
	p := m.mj + "/sidenote"
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == m.b[0].s {
		return
	}
	m.jmembers(m.b, jSide, what, "", m.r0, time.Now().Unix())
	if err := writeVia(p, m.b[0].s+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 옆으로 뺌 표식을 적지 못했다(다음 바퀴에 일지 줄이 또 나온다): %v\n", err)
	}
}

// termKill: 그 기계의 터미널 name을 정확한 주소(hc get이 준 address)로 끝낸다. 없거나 못 읽으면 아무것도 안 한다
func termKill(mach, name string) string {
	if !addrOK(mach) || !addrOK(name) {
		return ""
	}
	t, found, err := hcGet(mach+"/"+name, "")
	if err != nil || !found || t.State != "live" || t.Address == "" {
		return ""
	}
	addr := t.Address
	if !strings.Contains(addr, "/") {
		addr = mach + "/" + addr
	}
	if _, rc, e := hcRun(hcLook, "kill", addr); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 터미널 %s 를 끝내지 못했다(종료 %d): %s\n", addr, rc, strings.TrimSpace(e))
		return ""
	}
	return addr
}

// specSweep: 일꾼 — 작업 트리 폴더 wtr(<클론>.spec-wt) 아래에서 원격 추측 참조가 없는 아이디의 작업 트리를 치운다(감사 atp3xrb 중간 1 —
// specKill이 끝낸 일꾼은 defer가 안 돌아 작업 트리·worktree 등록이 남는다. hc kill이 KILL까지 가도 남지 않게 다음 일꾼이 거둔다).
// 폴더 이름은 <아이디 8>(나눔 몫이면 <아이디 8>-s<i>). 원격을 못 읽으면 아무것도 안 한다. 로그(.log)는 지금처럼 남긴다
func specSweep(wtr string) {
	ls, rc := lsRemote("origin", specRefs+"*")
	if rc != 0 {
		return
	}
	live := map[string]bool{}
	for _, l := range strings.Split(ls, "\n") {
		if f := strings.Fields(l); len(f) == 2 && len(f[1]) >= len(specRefs)+8 {
			live[strings.TrimPrefix(f[1], specRefs)[:8]] = true
		}
	}
	es, err := os.ReadDir(wtr)
	if err != nil {
		return
	}
	for _, e := range es {
		n := e.Name()
		if !e.IsDir() || len(n) < 8 || strings.Trim(n[:8], "0123456789abcdef") != "" || live[n[:8]] {
			continue
		}
		fmt.Printf("hc-lane merger: 끝난 추측의 남은 작업 트리를 치운다 — %s\n", n)
		specRmWT(wtr + "/" + n)
	}
}

// specKill: 도는 추측 r을 끝낸다 — 그 터미널(나눔이면 몫마다)을 정확한 주소로 끝내고 참조(spec·떴다는 표식·실패·조각·몫 끝)를 본 값 lease로
// 지운다. 줄은 c.rows에 남기되 gone(길이 건너뛴다 — 다른 단계가 쥔 포인터를 흔들지 않게). 일꾼은 결과를 쓰기 전에 spec 참조를 보니 끝내지
// 못한 일꾼의 결과도 남지 않는다
func (m *M) specKill(c *specCtx, r *specRow) string {
	terms := [][2]string{{r.machine, "spec-" + r.id[:8]}}
	if s, ok := m.shardRead(); ok && s.id == r.id {
		for _, p := range s.parts {
			terms = append(terms, [2]string{p[1], "spec-" + r.id[:8] + "-" + p[0]})
		}
	}
	var killed []string
	for _, t := range terms {
		if a := termKill(t[0], t[1]); a != "" {
			killed = append(killed, a)
		}
	}
	var args []string
	for ref, sha := range c.refs {
		if ref == specRefs+r.id || ref == specStarts+r.id || ref == specFails+r.id ||
			strings.HasPrefix(ref, specParts+r.id) || strings.HasPrefix(ref, specShards+r.id) {
			args = append(args, "--force-with-lease="+ref+":"+sha, ":"+ref)
			delete(c.refs, ref)
		}
	}
	if len(args) > 0 && runO(append([]string{"push", "-q", "origin"}, args...)...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 끝낸 추측 참조를 다 치우지 못했다 — 다음 바퀴에 다시")
	}
	r.state, r.lost, r.gone = specLost, "끝냄", true
	if len(killed) == 0 {
		return "터미널 없음"
	}
	return strings.Join(killed, "·")
}

// offPathKill: 이 바퀴에 짓기 전(줄이 빈 바퀴는 미리 줄을 챙긴 뒤) — 길(st0)·맨 앞 혼자·앞 무리·미리 줄 어디에도 없는 도는 추측을 끝낸다
// (C — 끝까지 돌게 두지 않는다). 무리를 다 못 읽은 바퀴(specGroups가 중간에 멈춤)는 건너뛴다(길이 짧게 보여 쓸 추측을 끝내지 않게)
func (m *M) offPathKill(c *specCtx, keep map[string]bool) {
	if m.gpartial {
		return
	}
	for i := range c.rows {
		r := &c.rows[i]
		if r.state != specRun || r.gone || keep[r.id] {
			continue
		}
		fmt.Printf("hc-lane merger: 길에서 벗어난 추측 %s 끝냄(%s, %s)\n", first8(r.id), r.machine, m.specKill(c, r))
	}
}

// ownIDs: 진짜 줄의 추측 — 길·맨 앞 혼자·이번 묶음의 앞 무리(범인 가르기). 미리 줄 추측과 아이디가 같아도 이쪽이다(감사 atp3xrb 낮음 4)
func (m *M) ownIDs(c *specCtx, st []specStep) map[string]bool {
	k := stepIDs(append(st[:len(st):len(st)], m.soloKeep(c)...))
	for j := range m.b {
		k[specID(m.btree, newGroup(m.b[:j+1]).key)] = true
	}
	return k
}

// keepIDs: 바퀴 처음의 지킬 추측 — ownIDs와 미리 줄(기준 + 그 sha)
func (m *M) keepIDs(c *specCtx, st []specStep) map[string]bool {
	k := m.ownIDs(c, st)
	for _, p := range c.pre {
		k[specID(m.btree, p.sha)] = true
	}
	return k
}

// preempt: 맨 앞·범인 가르기가 맡길 기계를 못 얻었다 — 길 밖의 도는 미리 줄 추측(가장 늦게 띄운 것)을 끝내고 그 기계를 맡긴다. 없으면 빈 값
func (m *M) preempt(c *specCtx, busy map[string]bool, st []specStep) verified.Verifier {
	lane := map[string]string{}
	for _, p := range c.pre {
		lane[p.sha] = p.lane
	}
	on := m.ownIDs(c, st)
	var best *specRow
	for i := range c.rows {
		r := &c.rows[i]
		if r.state == specRun && !r.gone && !on[r.id] && lane[r.group] != "" && r.machine != m.mme && (best == nil || r.at > best.at) {
			best = r
		}
	}
	if best == nil {
		return verified.Verifier{}
	}
	mach := best.machine
	how := m.specKill(c, best)
	delete(busy, mach)
	if c.idle == nil {
		c.idle = map[string]int{}
	}
	c.idle[mach] = idIdle // 우리 추측이 쥐고 있던 기계 — 일꾼의 바쁨 확인이 실제로 비었는지 한 번 더 본다
	what := fmt.Sprintf("미리 줄 추측 끝냄(%s) — 맨 앞에 내줌", mach)
	fmt.Printf("hc-lane merger: %s(미리 줄 %s, %s)\n", what, lane[best.group], how)
	m.jmembers(m.b, jPreKill, what+"(미리 줄 "+lane[best.group]+")", "", m.r0, time.Now().Unix())
	return m.specFree(c, busy)
}

// frontFree: 맨 앞·범인 가르기 몫의 기계 — 한가한 기계(병합 검증 자리), 없으면 미리 줄 추측을 끝내고 그 기계
func (m *M) frontFree(c *specCtx, busy map[string]bool, st []specStep) verified.Verifier {
	if v := m.specFree(c, busy); v.Machine != "" {
		return v
	}
	return m.preempt(c, busy, st)
}
