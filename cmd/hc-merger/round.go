package main

// 한 바퀴(bash mround): 일지 정리 → 큐 읽기 → 묶음 고르기 → 후보 작업 트리 → 착지 부르기 → 판정(judge.go).
// 일지 하나($mj/cur): 착지를 부르기 **전에** 「<가지> <기준 B> <후보 C>」 한 줄 + 담긴 제출마다 「<큐 참조> <제출 SHA> <레인> <기록 SHA|->」.
// 바퀴마다 새 제출을 고르기 전에 그 일지부터 정리한다(recover). 큐 표식은 기본 브랜치 push가 lease로 함께 지운다.

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// entry: 묶음에 담긴 제출 하나
type entry struct {
	q, s, l, m, d string // 큐 참조 · 제출 SHA · 레인 · 기록 SHA(없으면 -) · 제출 ID
	v, p          string // 감사 판정 · 승인 patch-id(판정 없으면 둘 다 빈 값). 일지(cur)에는 안 적는다 — 복구는 착지하지 않는다
	ow, owr       string // 제출 기록의 지휘·지휘 실행(없으면 빈 값) — 착지가 내는 중간 사건의 주인(ownerEnv)
}

// M: 병합기 일꾼의 상태(bash 판의 전역 변수들)
type M struct {
	parent                          int
	top, base, mj, mwr, ml, mme     string
	batch                           int
	mq                              string // 처리한 큐 참조(일을 했으면 빈 값 아님 — 바로 다음 바퀴)
	mer                             bool   // 원격 오류 등으로 못 본 것이 있음(--once는 75)
	mbn                             string // 그 착지에 든 제출 수
	ls0, ls1, r0                    int64  // 착지 시작·끝(유닉스 초, 0 = 모름) · 이 바퀴 시작
	qs                              string // 이번 바퀴의 원격 큐 목록(ls-remote)
	b, sfront                       []entry
	bj, bd, bv, bp                  string // 맨 앞 제출의 작업 표식·제출 ID·판정·patch-id
	B, btree, hb                    string // 기준(origin/<기본>)·그 트리·기준에 착지 스크립트가 있음
	solo                            []string
	mjx, ex, br, lgf, C0, mwt, mlab string
	lr                              string // 착지 종료(뒤에 설명이 붙을 수 있음)
	O, lk2, hs                      string
	deferred                        string // 이 바퀴가 쉰 까닭(defer_)
	stree                           string // 맨 앞 추측이 통과한 트리(착지에 HERDR_SPEC_TREE로)
	vw                              vwhy   // 이번 착지의 검증 길(일지 까닭 칸 — why.go)
	wmach                           string // 맨 앞을 기다리는 바퀴의 검증 기계(손잡이 0으로 맡기지 못해 기다리면 -) — 추측 기다림 일지 줄
	cwhy, clog                      string // 맨 앞 혼자 추측의 시험 실패로 반송할 까닭·로그(<기계>:<경로>) — specCulpritWhy
}

const mhook = "기본 브랜치에 없는 전용 착지 스크립트(.claude/herdr-land.sh)를 들이는 제출 — 병합기는 제출 트리 속 착지 코드를 실행하지 않는다(그 스크립트는 레인이 직접 착지해 먼저 넣는다)"

func (m *M) reserved(l string) bool {
	switch l {
	case m.base, "merge-batch", "batch", "main", "master":
		return true
	}
	return false
}

// laneOK: 레인 이름은 hc-fill과 같은 글자(a-z 0-9 _ -)·32자까지, 맨 앞 - 아님
func laneOK(l string) bool {
	return l != "" && l[0] != '-' && len(l) <= 32 && strings.Trim(l, "abcdefghijklmnopqrstuvwxyz0123456789_-") == ""
}

// ianc: a가 b에 들어갔나. git이 모르면(0·1 밖) 멈춘다(「모름」을 「아니오」로 읽지 않는다)
func ianc(a, b string) bool {
	switch rc := run("merge-base", "--is-ancestor", a, b); rc {
	case 0:
		return true
	case 1:
		return false
	default:
		dieGit(rc, "merger: git merge-base --is-ancestor 실패(종료 %d)", rc)
	}
	return false
}

// sameTree: 기준과 합친 트리가 기준 트리와 같은가. 0 예 · 1 아니오 · 2 모름
func (m *M) sameTree(s string) int {
	tree, rc := out("merge-tree", "--write-tree", m.B, s)
	switch rc {
	case 0:
		if tree == m.btree {
			return 0
		}
		return 1
	case 1:
		return 1
	}
	return 2
}

// touch: 갈래점 뒤 커밋 하나라도 「한 묶음에 하나만」 경로를 건드렸나. 0 예 · 1 아니오 · 2 모름
func (m *M) touch(s string) int {
	if len(m.solo) == 0 {
		return 1
	}
	hit, rc := out(append([]string{"rev-list", "-n", "1", m.B + ".." + s, "--"}, m.solo...)...)
	if rc != 0 {
		return 2
	}
	if hit != "" {
		return 0
	}
	return 1
}

func (m *M) keep(idx ...int) {
	nb := make([]entry, 0, len(idx))
	for _, i := range idx {
		nb = append(nb, m.b[i])
	}
	m.b = nb
}

// jwrite: 일지(cur) — 후보 가지·기준·후보 첫 커밋 한 줄, 담긴 제출마다 한 줄
func (m *M) jwrite() {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s %s\n", m.br, m.B, m.C0)
	for _, e := range m.b {
		fmt.Fprintf(&sb, "%s %s %s %s\n", e.q, e.s, e.l, e.m)
	}
	if err := writeVia(m.mj+"/cur", sb.String()); err != nil {
		die("merger: 일지를 쓰지 못했다")
	}
}

// capN: 앞 묶음이 실패해 줄인 크기($mj/cap). 없거나 꼴이 틀리면 0
func (m *M) capN() int {
	if c, err := os.ReadFile(m.mj + "/cap"); err == nil {
		if k, err := strconv.Atoi(strings.TrimSpace(string(c))); err == nil && k > 0 {
			return k
		}
	}
	return 0
}

func (m *M) writeCap(n int) {
	if err := writeVia(m.mj+"/cap", fmt.Sprintf("%d\n", n)); err != nil {
		die("merger: 묶음 크기를 적지 못했다")
	}
}

func (m *M) rbabort() bool {
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		p, rc := out("rev-parse", "--git-path", state)
		if rc != 0 || p == "" {
			return false
		}
		if _, err := os.Stat(p); err == nil {
			return run("rebase", "--abort") == 0
		}
	}
	return true
}

// prio: 제출 우선(요구 4) — 긴급 0 · 높음 1 · 보통 2(없거나 모르는 값) · 낮음 3
func prio(subject string) int {
	for _, kv := range strings.Fields(subject) {
		if v, ok := strings.CutPrefix(kv, "priority="); ok {
			switch v {
			case "긴급":
				return 0
			case "높음":
				return 1
			case "낮음":
				return 3
			}
		}
	}
	return 2
}

// queue: 큐를 우선 → 제출 시각(이름의 UTC 초) 순으로. 우선은 받아 둔 제출 기록의 제목에서 읽는다(못 읽으면 ok=false — 그 바퀴는 쉼)
func (m *M) queue() (q [][2]string, ok bool) {
	for _, line := range strings.Split(m.qs, "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/merge-queue/") {
			q = append(q, [2]string{f[1], f[0]})
		}
	}
	rank := map[string]int{}
	if len(q) > 1 && strings.Contains(m.qs, "refs/merge-queue-meta/") {
		subj, rc := out("for-each-ref", "--format=%(objectname) %(contents:subject)", "refs/hc-merger/meta/")
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "hc-lane merger: 제출 우선을 읽지 못했다(git for-each-ref) — 다음 바퀴에")
			return nil, false
		}
		bySha := map[string]int{}
		for _, line := range strings.Split(subj, "\n") {
			if sha, s, found := strings.Cut(line, " "); found {
				bySha[sha] = prio(s)
			}
		}
		for _, e := range q {
			mm := refsha(m.qs, "refs/merge-queue-meta/"+strings.TrimPrefix(e[0], "refs/merge-queue/"))
			r, found := bySha[mm]
			if !found {
				r = 2
			}
			rank[e[0]] = r
		}
	}
	sort.SliceStable(q, func(i, j int) bool {
		ri, rj := rank[q[i][0]], rank[q[j][0]]
		if _, f := rank[q[i][0]]; !f {
			ri = 2
		}
		if _, f := rank[q[j][0]]; !f {
			rj = 2
		}
		if ri != rj {
			return ri < rj
		}
		return q[i][0]+" "+q[i][1] < q[j][0]+" "+q[j][1]
	})
	return q, true
}

// pickBatch: 큐 앞에서 묶음 크기만큼 담는다. 계속하면 true, 이 바퀴를 여기서 끝내면 false
func (m *M) pickBatch() bool {
	n := m.batch
	if k := m.capN(); k > 0 && k < n { // 앞 묶음이 실패해 줄인 크기
		n = k
	}
	q, ok := m.queue()
	if !ok {
		m.mer = true
		return false
	}
	m.b = nil
	for _, e := range q {
		mq, ms := e[0], e[1]
		mn := strings.TrimPrefix(mq, "refs/merge-queue/")
		if _, err := os.Stat(m.mj + "/fail/" + mn); err == nil {
			continue // 돌려보내기를 다시 하는 중(fretry)
		}
		ln := ""
		if len(mn) > 11 && digits.MatchString(mn[:10]) && mn[10] == '-' {
			ln = mn[11:]
		}
		mm := refsha(m.qs, "refs/merge-queue-meta/"+mn)
		dash := func(s string) string {
			if s == "" {
				return "-"
			}
			return s
		}
		// 레인 이름이 가지 이름으로 못 쓰는 꼴이면 그 항목만 반송. 묶음 뒤쪽에서 걸리면 거기서 끊는다
		if !laneOK(ln) {
			if len(m.b) > 0 {
				break
			}
			if !m.fail(mq, ms, dash(ln), dash(mm), "-", "큐 이름 꼴이 아니다(<UTC초 10자리>-<레인>, 레인은 가지 이름으로 쓸 수 있는 a-z 0-9 - _)") {
				m.mer = true
			}
			m.mq = "x"
			return false
		}
		mt, ok := readMeta(mm, ms)
		if !ok {
			if len(m.b) > 0 {
				break
			}
			fmt.Fprintf(os.Stderr, "hc-lane merger: 제출 기록 %s 을 읽지 못했다(또는 원본과 안 맞음) — 다음 바퀴에\n", mn)
			m.mer, m.mq = true, ""
			return false
		}
		// 다른 기본 브랜치로 낸 제출은 그 브랜치의 병합기 몫
		if mt.base != "" && mt.base != m.base {
			fmt.Fprintf(os.Stderr, "hc-lane merger: %s 건너뜀 — 기본 브랜치 %s 로 낸 제출(이 병합기는 %s)\n", ln, mt.base, m.base)
			continue
		}
		if m.reserved(ln) {
			if len(m.b) > 0 {
				break
			}
			m.mq = mq
			if !m.fail(mq, ms, ln, dash(mm), "-", "병합기가 쓰는 가지 이름") {
				m.defer_("예약 이름 반송 실패")
			}
			return false
		}
		// 제출 ID가 없는 손 제출은 혼자. 판정이 붙은 제출도 담는다 — 구성원마다 자기 승인 patch-id를 filterBatch(approved)가 확인하고,
		// 다르면 그 구성원만 이번 묶음에서 뺀다
		batchable := mt.id != ""
		if !batchable && len(m.b) > 0 {
			break
		}
		if len(m.b) == 0 {
			m.bj, m.bd, m.bv, m.bp = mt.jobref, mt.id, mt.verdict, mt.patch
		}
		m.b = append(m.b, entry{q: mq, s: ms, l: ln, m: dash(mm), d: mt.id, v: mt.verdict, p: mt.patch, ow: mt.owner, owr: mt.ownerRun})
		if !batchable || len(m.b) >= n {
			break
		}
	}
	return len(m.b) > 0
}

// round: 한 바퀴(bash mround)
func (m *M) round() {
	m.mq, m.mer, m.mbn, m.ls0, m.ls1, m.r0, m.sfront = "", false, "", 0, 0, time.Now().Unix(), nil
	defer m.specWaitEarly()
	m.flush()
	// 병합기 클론은 병합기만 쓴다 — 앞 실행이 남긴 멈춘 rebase는 풀고, 그래도 깨끗하지 않으면 멈춘다(모르는 변경을 지우지 않음)
	if !m.rbabort() {
		m.defer_("클론의 rebase 상태 확인·복구 실패")
		return
	}
	st, rc := out("status", "--porcelain", "--untracked-files=all")
	if rc != 0 {
		dieGit(rc, "merger: git status 실패 — 클론 상태를 모른다")
	}
	if st != "" {
		die("merger: 병합기 클론에 커밋 안 된 변경·새 파일이 있다 — 살펴 치운 뒤 다시 띄운다: %s", m.top)
	}
	if !m.wtclean() {
		m.defer_("후보 작업 트리 정리 실패")
		return
	}
	if !m.recover() {
		return
	}
	m.fretry()
	m.qs, rc = lsRemote(append([]string{"origin", "refs/heads/" + m.base, "refs/merge-queue/*", "refs/merge-queue-meta/*", "refs/merge-done/*", "refs/heads/lane/*", "refs/herdr/merger/*", "refs/herdr/landing/*"}, specLsPatterns...)...)
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 원격 큐를 읽지 못했다(git ls-remote) — 다음 바퀴에")
		m.mer = true
		return
	}
	// 켬 표식이 이 기계인지 바퀴마다 본다 — 길 안내일 뿐 정확성 조건이 아니다(main에 쓰는 모든 길이 main CAS)
	if refsha(m.qs, "refs/herdr/merger/"+m.mme) == "" {
		for _, line := range strings.Split(m.qs, "\n") {
			if f := strings.Fields(line); len(f) == 2 && strings.HasPrefix(f[1], "refs/herdr/merger/") {
				die("merger: 켬 표식이 다른 기계(refs/herdr/merger/%s)다 — 이 병합기(%s)는 멈춘다(기계를 옮겼거나 이름이 다르면 HC_LANE_MACHINE=<표식의 기계>로)", strings.TrimPrefix(f[1], "refs/herdr/merger/"), m.mme)
			}
		}
		fmt.Fprintf(os.Stderr, "hc-lane merger: 켬 표식 refs/herdr/merger/%s 가 없다(꺼짐) — 큐를 건드리지 않고 쉰다\n", m.mme)
		m.mer = true
		return
	}
	m.landUnmark() // 일지를 정리한 뒤라 이 병합기는 착지 중이 아니다(landing.go)
	// 제출 기록(레인이 같은 push로 올림)을 한 번에 받는다. 못 받으면 이 바퀴는 쉰다
	if strings.Contains(m.qs, "refs/merge-queue-meta/") {
		if fetchr(true, "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*") != 0 {
			fmt.Fprintln(os.Stderr, "hc-lane merger: 제출 기록을 받지 못했다(git fetch) — 다음 바퀴에")
			m.mer = true
			return
		}
	}
	m.prescan() // 줄 전체를 지금 기준과 견줘 실제 rebase로도 충돌하는 것을 차례 전에 반송(prescan.go)
	if !m.pickBatch() {
		if m.mq == "" && !m.mer {
			m.specIdle()
		}
		return
	}
	m.mq, m.sfront = m.b[0].q, m.b[:1:1]
	if fetchr(false, "-q", "origin", "+refs/heads/"+m.base+":refs/remotes/origin/"+m.base, "+refs/merge-queue/*:refs/hc-merger/q/*") != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: git fetch 실패 — 다음 바퀴에")
		m.mer, m.mq = true, ""
		return
	}
	if m.B, rc = out("rev-parse", "-q", "--verify", "refs/remotes/origin/"+m.base+"^{commit}"); rc != 0 {
		dieGit(rc, "merger: origin/%s 가 없다", m.base)
	}
	// 이미 기본 브랜치에 든 제출은 착지 없이 정리만
	if ianc(m.b[0].s, m.B) {
		m.existing(m.b[0].s, "제출이 이미 "+m.base+" 에 있음")
		return
	}
	if m.btree, rc = out("rev-parse", "--verify", m.B+"^{tree}"); rc != 0 {
		m.defer_("기준 트리 조회 실패")
		return
	}
	switch m.sameTree(m.b[0].s) {
	case 0:
		m.existing(m.B, "제출 내용이 이미 "+m.base+" 에 있음(합친 트리가 같음)")
		return
	case 2:
		m.defer_("git merge-tree 실패")
		return
	}
	// 착지 스크립트는 기본 브랜치 판을 쓴다. 기본 브랜치에 없는 스크립트를 제출이 새로 들이면 반송
	hl, rc1 := out("ls-tree", "--name-only", m.b[0].s, "--", ".claude/herdr-land.sh")
	hb, rc2 := out("ls-tree", "--name-only", m.B, "--", ".claude/herdr-land.sh")
	if rc1 != 0 || rc2 != 0 {
		dieGit(max(rc1, rc2), "merger: git ls-tree 실패(%s · %s)", m.b[0].s, m.B)
	}
	m.hb = hb
	if hl != "" && hb == "" {
		if !m.fail(m.b[0].q, m.b[0].s, m.b[0].l, m.b[0].m, "-", mhook) {
			m.mer = true
		}
		return
	}
	if !m.filterBatch() {
		return
	}
	m.mjx = ""
	if m.bj != "" && m.bj != "refs/hc-landed/"+m.bd {
		hk, rc := lsRemote("origin", m.bj)
		if rc != 0 {
			fmt.Fprintln(os.Stderr, "hc-lane merger: 작업 표식을 보지 못했다(git ls-remote) — 다음 바퀴에")
			m.mer, m.mq = true, ""
			return
		}
		if hk == "" {
			m.mjx = "HEAD:" + m.bj
		}
	}
	m.exbuild()
	m.br = m.b[0].l
	m.lgf = fmt.Sprintf("%s/%s-%s-%d.log", m.ml, m.b[0].l, time.Now().Format("20060102-150405"), os.Getpid())
	m.C0 = m.b[0].s
	m.jwrite() // 행동 전에(작업 트리를 만들기 전에)
	// 후보는 매번 새 임시 작업 트리에서 레인 이름 가지로 — 클론의 작업 트리는 건드리지 않는다
	var err error
	if m.mwt, err = mktempDir(m.mwr); err != nil {
		die("merger: 후보 작업 트리를 만들지 못했다(%s)", m.br)
	}
	if rc := run("worktree", "add", "-q", "-f", "-B", m.br, m.mwt, m.C0); rc != 0 {
		dieGit(rc, "merger: 후보 작업 트리를 만들지 못했다(%s)", m.br)
	}
	if len(m.b) > 1 && !m.stack() {
		return
	}
	// 추측 병렬 검증(specrun.go) — 맨 앞 추측이 검증 기계에서 도는 중이면 착지하지 않고 쉰다
	var sd specDecision
	if sd, m.stree = m.spec(); sd == specWaitFront {
		m.specWaitNote()
		m.fin()
		m.mq, m.mer = "", true
		return
	}
	// 기다림이 끝났다 — 이 바퀴의 착지·반송 줄보다 먼저
	m.specWaitEnd()
	if sd == specShrink { // 묶음 착지 실패와 같은 길(judge) — 병합기 검증 없이
		m.jround(jBatchFail, m.cwhy, ", 로그 "+m.clog)
		m.writeCap(len(m.b) - 1)
		if m.fin() {
			m.mq = "x"
		}
		return
	}
	if sd == specReject { // 혼자 착지가 실패했을 때와 같은 길(judge 끝) — 검증 한 바퀴를 더 쓰지 않는다
		if !m.fin() {
			return
		}
		m.capEnd()
		if !m.fail(m.b[0].q, m.b[0].s, m.b[0].l, m.b[0].m, m.clog, m.cwhy) {
			m.mer = true
		}
		return
	}
	labs := make([]string, len(m.b))
	for i, e := range m.b {
		labs[i] = e.l
	}
	m.mlab = strings.Join(labs, "+")
	more := ""
	if len(m.b) > 1 {
		more = fmt.Sprintf(", 묶음 %d", len(m.b))
	}
	m.landMark()
	fmt.Printf("hc-lane merger: 착지 시작 — %s (기준 %s%s)\n", m.mlab, first8(m.B), more)
	m.mbn, m.ls0 = fmt.Sprint(len(m.b)), time.Now().Unix()
	// 상주 부모가 끝났으면 새 착지를 시작하지 않는다. 이미 시작한 착지의 결과는 다음 병합기가 일지로 수거한다
	if !m.parentAlive() {
		return
	}
	lrc := m.runCandidate()
	if !m.parentAlive() {
		return
	}
	if lrc == rcNotRun { // 명령은 안 돌았다 — 후보만 치우고 큐는 그대로 쉰다(이어지면 deferNote가 막힘 사건)
		if m.fin() {
			// 까닭은 바퀴마다 같은 글(로그 경로처럼 바뀌는 값을 넣지 않는다 — deferNote가 같은 까닭으로 센다)
			m.defer_("준비 훅·착지를 띄우지 못했다(모름 — 명령은 안 돎: 고아 기록·시작 시각을 확정 못 함 또는 실행 파일 없음, 자세한 것은 착지 로그)")
		}
		return
	}
	m.lr = fmt.Sprint(lrc)
	m.ls1 = time.Now().Unix()
	tail5(m.lgf)
	m.judge()
}

// filterBatch: 묶음 뒤쪽(2번째부터)에서 맨 앞이 될 때 따로 처리할 것을 뺀다 — 이미 든 것·없는 착지 스크립트를 들이는 것·
// 「한 묶음에 하나만」 경로(기본 브랜치의 .claude/herdr-merge-solo)를 앞 구성원과 함께 건드리는 것. 쉬면 false
func (m *M) filterBatch() bool {
	m.solo = nil
	if len(m.b) > 1 {
		merges, rc := out(append(append([]string{"rev-list", "-1", "--merges"}, shas(m.b)...), "^"+m.B)...)
		if rc != 0 {
			m.defer_("병합 커밋 조회 실패")
			return false
		}
		if merges != "" {
			m.keep(0) // 병합 풀이가 있는 제출은 착지에서 원본 그대로 판정한다
		}
	}
	if len(m.b) > 1 {
		sf, rc := out("ls-tree", "--name-only", m.B, "--", ".claude/herdr-merge-solo")
		if rc != 0 {
			m.defer_("묶음 규칙 조회 실패")
			return false
		}
		if sf != "" {
			body, rc := out("show", m.B+":"+sf)
			if rc != 0 {
				m.defer_("묶음 규칙 읽기 실패")
				return false
			}
			for _, l := range strings.Split(body, "\n") {
				if l != "" && !strings.HasPrefix(l, "#") {
					m.solo = append(m.solo, l)
				}
			}
		}
	}
	// 맨 앞의 승인이 지금 변경과 다르면(또는 모르면) 혼자 — 착지가 그 판정으로 지금처럼 다시 보고 돌려보낸다
	// 혼자여도 먼저 본다 — 모름이면 묶음을 줄이지도 혼자 착지(그 판정 대조가 또 git 실패면 일시 실패로 세다 반송)로 넘기지도 않고 쉰다(R-H02)
	if m.b[0].v != "" {
		switch m.approved(m.b[0]) {
		case apDiffer:
			m.keep(0)
		case apUnknown:
			m.defer_("승인 patch-id 확인 실패")
			return false
		}
	}
	su := false
	switch m.touch(m.b[0].s) {
	case 0:
		su = true
	case 2:
		m.defer_("묶음 경로 이력 조회 실패")
		return false
	}
	if len(m.b) > 1 {
		k := []int{0}
		for i := 1; i < len(m.b); i++ {
			s := m.b[i].s
			if ianc(s, m.B) {
				continue
			}
			switch m.sameTree(s) {
			case 0:
				continue
			case 2:
				m.defer_("git merge-tree 실패")
				return false
			}
			hl, rc := out("ls-tree", "--name-only", s, "--", ".claude/herdr-land.sh")
			if rc != 0 {
				m.defer_("착지 훅 조회 실패")
				return false
			}
			if hl != "" && m.hb == "" {
				continue
			}
			if m.approved(m.b[i]) != apOK {
				continue // 다름·모름 모두 큐에 그대로(반송 아님) — 맨 앞이 되면 다시 확인한다(모름이면 그때 쉰다)
			}
			switch m.touch(s) {
			case 0:
				if su {
					continue
				}
				su = true
			case 2:
				m.defer_("묶음 경로 이력 조회 실패")
				return false
			}
			k = append(k, i)
		}
		m.keep(k...)
	}
	return true
}

func shas(b []entry) []string {
	s := make([]string, len(b))
	for i, e := range b {
		s[i] = e.s
	}
	return s
}

func first8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
