package main

// 쌓인 제출(docs/hc-fill/stack.md 「병합기 규칙 넷」): 제출 기록에 stack=<선행>@<P>가 있는 제출 — 선행 레인이 감사 통과로 낸 커밋 P 위에
// 자기 커밋을 얹은 것(P는 아직 기본 가지에 없을 수 있다). 칸이 없는 제출은 지금 길 그대로다.
//   1 차례: queue()가 쌓인 제출을 선행 바로 뒤로 옮긴다(우선·이름 순보다 앞서는 걸림). 고르기는 모두 그 차례의 앞 무리라 선행이 빠진 무리에
//     뒤가 들 수 없고, 선행을 건너뛰면(pickBatch의 건너뜀·keep·stack()의 충돌 빼기·추측 길의 실패 자리) 뒤도 뺀다.
//   2 선행이 빠지면 주차: 선행 큐(<레인> = 선행, SHA = P)가 없고 들지도 않았고 착지·풀이 중도 아니면 refs/merge-parked로 옮긴다(중간 사건).
//   3 자기 커밋만 얹음: 갈래점은 P(forkOf). 맨 앞이 쌓인 제출이면 혼자여도 늘 stack() 길(m.rewrote) — 얹다 부딪히면 검증 전에 그 제출만 반송.

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/landing"
	"herdr-tools/internal/tri"
)

type stackRec struct {
	pred, p string
	done    bool // 선행이 이미 들었다(merge-done 원본 = P 또는 기준이 P를 담음) — 차례 걸림 없음
}

// stackM: M에 묻는 상태 — 이번 큐의 쌓인 제출(제출 SHA → 선행·P, queue()가 바퀴마다 다시 셈), 이 바퀴 맨 앞이 쌓인 제출(병합기가 후보를 짓는다)
type stackM struct {
	stk     map[string]stackRec
	rewrote bool
}

// qLane: 큐 이름 <UTC초 10자리>-<레인>의 레인(꼴이 아니면 빈 값)
func qLane(n string) string {
	if len(n) > 11 && digits.MatchString(n[:10]) && n[10] == '-' {
		return n[11:]
	}
	return ""
}

type stackSub struct {
	q, s, ln, rec string
	mt            meta
	why           string // 반송 까닭(꼴이 틀림)
}

// stackOrder: queue()가 정렬한 큐 q에 규칙 1·2를 건다 — 쌓인 제출을 선행 바로 뒤로, 선행이 아직 착지·풀이 중이면 이 바퀴는 빼고, 선행이
// 빠졌으면 주차, 꼴이 틀리면 반송. 쌓인 제출이 없으면 q 그대로(git 부름은 제출 기록 제목 읽기 한 번). 원격·git을 모르면 ok=false(쉼)
func (m *M) stackOrder(q [][2]string) ([][2]string, bool) {
	m.stk = map[string]stackRec{}
	if len(q) == 0 || !strings.Contains(m.qs, "refs/merge-queue-meta/") {
		return q, true
	}
	subj, rc := out("for-each-ref", "--format=%(objectname) %(contents:subject)", "refs/hc-merger/meta/")
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 쌓인 제출을 읽지 못했다(git for-each-ref) — 다음 바퀴에")
		return nil, false
	}
	marked := map[string]bool{}
	for _, l := range strings.Split(subj, "\n") {
		if sha, s, ok := strings.Cut(l, " "); ok && (strings.HasPrefix(s, "stack=") || strings.Contains(s, " stack=")) {
			marked[sha] = true
		}
	}
	var subs []stackSub
	for _, e := range q {
		n := strings.TrimPrefix(e[0], "refs/merge-queue/")
		rec := refsha(m.qs, "refs/merge-queue-meta/"+n)
		if !marked[rec] {
			continue
		}
		// 기록을 못 읽으면 차례의 판정(pickBatch가 거기서 멈춤)에 맡긴다
		if mt, ok := readMeta(rec, e[1]); ok && mt.pred+mt.sbad != "" {
			subs = append(subs, stackSub{q: e[0], s: e[1], ln: qLane(n), rec: rec, mt: mt})
		}
	}
	if len(subs) == 0 {
		return q, true
	}
	if !m.stackFetch(subs) {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 쌓인 제출의 선행 커밋을 받지 못했다(git fetch) — 다음 바퀴에")
		return nil, false
	}
	at := map[string]int{} // 「<레인> <SHA>」 → q 자리
	for i, e := range q {
		at[qLane(strings.TrimPrefix(e[0], "refs/merge-queue/"))+" "+e[1]] = i
	}
	skip, dep, deps := map[string]bool{}, map[string]bool{}, map[string][]int{}
	qi := map[string]int{}
	for i, e := range q {
		qi[e[0]] = i
	}
	var parks, bads []stackSub
	for _, s := range subs {
		r, st := m.stackClass(&s)
		switch {
		case s.why != "":
			bads, skip[s.q] = append(bads, s), true
			continue
		case st == tri.Unknown:
			fmt.Fprintf(os.Stderr, "hc-lane merger: 쌓인 제출 %s 의 선행 %s 를 확인하지 못했다(git 실패) — 이 바퀴는 뺀다\n", s.ln, r.pred)
			skip[s.q] = true
			continue
		}
		m.stk[s.s] = r
		if r.done {
			continue
		}
		if i, ok := at[r.pred+" "+r.p]; ok {
			deps[q[i][0]], dep[s.q] = append(deps[q[i][0]], qi[s.q]), true
			continue
		}
		skip[s.q] = true
		if !m.predBusy(r) {
			parks = append(parks, s)
		}
	}
	var o [][2]string
	var emit func(e [2]string)
	emit = func(e [2]string) {
		o = append(o, e)
		for _, j := range deps[e[0]] {
			emit(q[j])
		}
	}
	for _, e := range q {
		if !skip[e[0]] && !dep[e[0]] {
			emit(e)
		}
	}
	for _, s := range bads {
		if !m.fail(s.q, s.s, s.ln, s.rec, "-", s.why) {
			m.mer = true
		}
		m.qs = dropRefs(m.qs, s.q, "refs/merge-queue-meta/"+strings.TrimPrefix(s.q, "refs/merge-queue/"))
	}
	for _, s := range parks {
		if m.park(s) {
			m.qs = dropRefs(m.qs, s.q, "refs/merge-queue-meta/"+strings.TrimPrefix(s.q, "refs/merge-queue/"))
		} else {
			m.mer = true
		}
	}
	return o, true
}

// stackFetch: 판정에 쓸 객체(제출·P·기준·선행의 merge-done 기록)가 이 클론에 없으면 받는다 — 기준은 따로 둔 참조로(바퀴의 origin/<기본>을
// 움직이지 않게)
func (m *M) stackFetch(subs []stackSub) bool {
	B := refsha(m.qs, "refs/heads/"+m.base)
	need := []string{B}
	specs := []string{"-q", "origin", "+refs/heads/" + m.base + ":refs/hc-merger/stack/base", "+refs/merge-queue/*:refs/hc-merger/q/*"}
	for _, s := range subs {
		need = append(need, s.s, s.mt.pp)
		if d := refsha(m.qs, "refs/merge-done/"+s.mt.pred); d != "" && laneOK(s.mt.pred) {
			need = append(need, d)
			if sp := "+refs/merge-done/" + s.mt.pred + ":refs/hc-merger/stack/done/" + s.mt.pred; !slices.Contains(specs, sp) {
				specs = append(specs, sp)
			}
		}
	}
	for _, x := range need {
		if x != "" && sha40.MatchString(x) && runQ("cat-file", "-e", x+"^{commit}") != 0 {
			return fetchr(true, specs...) == 0
		}
	}
	return true
}

// stackClass: 쌓인 제출 하나의 꼴(s.why — 반송 까닭)과 선행이 들었는지
func (m *M) stackClass(s *stackSub) (stackRec, tri.State) {
	r := stackRec{pred: s.mt.pred, p: s.mt.pp}
	switch {
	case s.mt.sbad != "":
		s.why = s.mt.sbad
	case r.pred == s.ln:
		s.why = "쌓임 기록 꼴(선행이 자기 레인)"
	case r.p == s.s:
		s.why = "쌓임 기록 꼴(선행 커밋 P가 제출 자신)"
	}
	if s.why != "" {
		return r, tri.No
	}
	switch runQ("merge-base", "--is-ancestor", r.p, s.s) {
	case 0:
	case 1:
		s.why = "쌓임 기록 꼴(선행 커밋 P " + first8(r.p) + " 가 제출의 조상이 아님)"
		return r, tri.No
	default:
		return r, tri.Unknown
	}
	st := m.predDone(r)
	r.done = st == tri.Yes
	return r, st
}

// predDone: 선행이 들었나 — 원격 refs/merge-done/<선행> 기록의 원본(부모)이 P이거나, 기본 가지가 P를 담음
func (m *M) predDone(r stackRec) tri.State {
	if d := refsha(m.qs, "refs/merge-done/"+r.pred); d != "" {
		p, rc := outQ("rev-parse", "-q", "--verify", d+"^")
		if rc != 0 {
			return tri.Unknown
		}
		if p == r.p {
			return tri.Yes
		}
	}
	switch runQ("merge-base", "--is-ancestor", r.p, refsha(m.qs, "refs/heads/"+m.base)) {
	case 0:
		return tri.Yes
	case 1:
		return tri.No
	}
	return tri.Unknown
}

// predBusy: 선행 제출(P)이 큐 밖이지만 아직 살아 있나 — 착지 중 표식·병합 쪽 풀이(그동안 큐 목록에서 뺀다 — resolve.go)
func (m *M) predBusy(r stackRec) bool {
	for _, line := range strings.Split(m.qs, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == r.p && strings.HasPrefix(f[1], landing.Prefix) && qLane(strings.TrimPrefix(f[1], landing.Prefix)) == r.pred {
			return true
		}
	}
	des, _ := os.ReadDir(m.mj + "/resolve")
	for _, d := range des {
		if rs, ok := loadR(m.mj + "/resolve/" + d.Name()); ok && rs.ln == r.pred && rs.s == r.p {
			return true
		}
	}
	return false
}

// park: 선행이 빠진(반송·거둠) 쌓인 제출을 큐·기록에서 refs/merge-parked/<이름>·refs/merge-parked-meta/<이름>으로 — 한 번의 원자 push(lease).
// 실패가 아니다: 반송 횟수·범인·다시 맞춤에 들지 않고 일지 「주차」·중간 사건만(hc-fill이 선행이 다시 서면 다시 쌓는다). false = 못 옮김
// ponytail: 옮긴 뒤 사건을 적기 전에 죽으면 사건이 빠진다(큐에서 빠져 다시 보지 않는다) — 주차 참조가 정본이라 hc-fill은 그대로 다시 쌓는다
func (m *M) park(s stackSub) bool {
	n := strings.TrimPrefix(s.q, "refs/merge-queue/")
	pr, pm, qm := "refs/merge-parked/"+n, "refs/merge-parked-meta/"+n, "refs/merge-queue-meta/"+n
	p := []string{s.s + ":" + pr, ":" + s.q, "--force-with-lease=" + pr + ":", "--force-with-lease=" + s.q + ":" + s.s,
		s.rec + ":" + pm, ":" + qm, "--force-with-lease=" + pm + ":", "--force-with-lease=" + qm + ":" + s.rec}
	if runQ(append([]string{"push", "-q", "--atomic", "--porcelain", "origin"}, p...)...) != 0 {
		rb, rc := lsRemote("origin", pr, pm)
		if rc != 0 || refsha(rb, pr) != s.s || refsha(rb, pm) != s.rec { // 응답만 끊김이 아니면 다음 바퀴에(큐가 바뀌었으면 그때 다시 판정)
			fmt.Fprintf(os.Stderr, "hc-lane merger: 쌓인 제출 %s 를 %s 로 옮기지 못했다 — 다음 바퀴에\n", s.ln, pr)
			return false
		}
	}
	why := fmt.Sprintf("주차(선행 반송) %s — %s 제출 %s", s.mt.pred, s.ln, first8(s.s))
	msg := fmt.Sprintf("병합기: %s, %s%s, %s 로 옮김(선행 %s 가 큐에 없고 들지 않음 — 실패 아님, 선행이 다시 서면 다시 쌓는다)",
		why, s.mt.key(n), s.mt.infoTail(), pr, first8(s.mt.pp))
	if !m.jonce(s.mt, n, s.ln, "주차", "-", why) || !m.outbox(s.mt, n, "중간", "-", msg, s.ln) {
		return true // 옮김은 끝났다 — 사건만 빠짐
	}
	m.send(n)
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s\n", why)
	return true
}

// stackSkip: pickBatch의 건너뜀 — 다른 기본 브랜치로 낸 제출(그 브랜치의 병합기 몫)·선행이 아직 들지 않았는데 이번 묶음에 없는 쌓인 제출
func (m *M) stackSkip(mt meta, ln, s string) bool {
	if mt.base != "" && mt.base != m.base {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 건너뜀 — 기본 브랜치 %s 로 낸 제출(이 병합기는 %s)\n", ln, mt.base, m.base)
		return true
	}
	return !m.predIn(s, m.b)
}

// predIn: 제출 s를 b 뒤에 담아도 되나 — 쌓인 제출이 아니거나, 선행이 들었거나, 선행 제출(P)이 b에 있음
func (m *M) predIn(s string, b []entry) bool {
	r, ok := m.stk[s]
	return !ok || r.done || slices.Contains(shas(b), r.p)
}

// stackKeep: 묶음을 줄인 뒤(keep) 선행이 빠진 쌓인 구성원도 뺀다(큐에 그대로). 맨 앞은 늘 남긴다(pickBatch가 이미 걸렀다)
func (m *M) stackKeep(b []entry) []entry {
	o := b[:0:0]
	for _, e := range b {
		if len(o) > 0 && !m.predIn(e.s, o) {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(선행 %s 가 이번 묶음에서 빠짐, 큐에 그대로)\n", e.l, m.stk[e.s].pred)
			continue
		}
		o = append(o, e)
	}
	return o
}

// predKept: stack()이 구성원 i를 얹기 전에 — 선행이 앞에서 얹혔나(k = 얹은 구성원 자리)
func (m *M) predKept(i int, k []int) bool {
	b := make([]entry, 0, len(k))
	for _, j := range k {
		b = append(b, m.b[j])
	}
	return m.predIn(m.b[i].s, b)
}

// forkOf: 제출 s의 갈래점 — 쌓인 제출이면 P(자기 커밋만), 아니면 지금처럼 merge-base(s, base)
func (m *M) forkOf(s, base string) (string, int) {
	if r, ok := m.stk[s]; ok {
		return r.p, 0
	}
	return out("merge-base", s, base)
}

// stackSolo: 이 바퀴 맨 앞이 쌓인 제출인가 — 그러면 혼자여도 늘 stack() 길로 P..제출만 얹고(P가 기준에 있든, 다시 쓰여 들었든 같은 길),
// 얹다 부딪히면 검증을 맡기기 전에 그 제출만 돌려보낸다(stackFail — 제출 쪽은 선행이 들기 전 충돌을 보지 않는다, 지휘 t3yx 안 A).
// 착지에는 판정 대신 기본 가지 lease(m.rewrote — landEnv)
func (m *M) stackSolo() bool {
	_, m.rewrote = m.stk[m.b[0].s]
	return m.rewrote
}

// frontArgs: 후보 맨 앞을 기준 위로 다시 얹는 rebase — 쌓인 제출이면 P 뒤 커밋만(--onto)
func (m *M) frontArgs() []string {
	a := []string{"rebase", "-q", "--reapply-cherry-picks"}
	if r, ok := m.stk[m.b[0].s]; ok {
		return append(a, "--onto", m.B, r.p)
	}
	return append(a, m.B)
}

// stackConflictWhy: 쌓인 맨 앞을 기준 위에 얹다 부딪힌 반송 까닭 — 다시 맞춤 길(「착지 실패(종료 3, 충돌 …)」)과 자기 커밋만 옮기는 법
func (m *M) stackConflictWhy(e entry) string {
	r := m.stk[e.s]
	// 파일은 P부터(자기 커밋만) — 맨 앞이면 선행은 이미 들었으니 P→기준은 그 사이 기본 가지 변경뿐이다(자동 갈래점은 선행 파일까지 섞는다)
	fs := "파일 모름(로그)"
	var o bytes.Buffer // 충돌(종료 1)의 출력이 필요하다 — Out은 0이 아니면 출력을 버린다
	if gitx.Run("", &o, nil, "-c", "core.quotePath=false", "merge-tree", "--write-tree", "--name-only", "--no-messages", "--merge-base="+r.p, m.B, e.s) == 1 {
		if _, names, ok := strings.Cut(o.String(), "\n"); ok && strings.TrimSpace(names) != "" {
			fs = strings.Join(strings.Fields(names), " ")
		}
	}
	return "착지 실패(종료 3, 충돌 " + fs + ") — 쌓인 제출 — 선행 " + r.pred + " 위에 자기 커밋을 얹다 충돌: " + fs +
		". 다시 맞추기: git rebase --onto origin/" + m.base + " " + first8(r.p) + " 로 자기 커밋만 옮겨 풀고 --verdict 없이 다시 낸다"
}

// stackFail: 쌓인 맨 앞을 기준 위에 못 얹음(충돌·승인과 다름) — 후보를 치우고 반송(혼자 착지 실패와 같은 길)
func (m *M) stackFail(why string) {
	if !m.fin() {
		return
	}
	m.capEnd()
	if !m.fail(m.b[0].q, m.b[0].s, m.b[0].l, m.b[0].m, m.lgf, why) {
		m.mer = true
	}
}

// stackOrphan: 추측 길의 마지막 자리가 선행이 길에 없는(앞 무리 실패로 건너뜀) 쌓인 제출을 품었나 — 그러면 짓지 않고 실패 자리로(sideStep)
func (m *M) stackOrphan(st []specStep) bool {
	if len(st) < 2 || len(m.stk) == 0 {
		return false
	}
	on := map[string]bool{}
	for _, s := range st[:len(st)-1] {
		if s.row == nil {
			continue
		}
		switch s.row.state {
		case specPass, specRun, specLost, specBusyX: // specWalk가 앞으로 나아간 자리
			for _, x := range s.g.shas {
				on[x] = true
			}
		}
	}
	for _, x := range st[len(st)-1].g.shas {
		if r, ok := m.stk[x]; ok && !r.done && !on[r.p] {
			return true
		}
		on[x] = true
	}
	return false
}

// stackTag: 일지 까닭에 붙일 「쌓임 <선행>」(쌓인 제출이 아니면 빈 값)
func (m *M) stackTag(s string) string {
	if r, ok := m.stk[s]; ok {
		return "쌓임 " + r.pred
	}
	return ""
}
