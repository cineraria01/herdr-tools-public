package main

// 착지 뒤 판정·끊긴 앞 실행 정리·후보 치우기(bash mjudge·mrecover·mfin·mwtclean·mexisting·mtrans·mtry).
// 들어갔는지는 main push가 원자로 함께 실은 제출 ID 표식 refs/hc-landed/<제출 ID>로만 본다 — 그 표식이 있고 그 SHA가 main의 조상이면
// 완료(패치 일치로 판정하지 않는다). 제출 기록이 없는 큐 항목(손으로 넣음)만 후보 가지 끝으로 본다.

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// judge: 착지(종료 m.lr) 뒤. 종료 코드와 상관없이 먼저 원격이 후보를 담았는지 본다(응답 유실로 1이어도 들어갔을 수 있다).
// 못 보면 일지를 두고 쉰다(다음 바퀴의 recover가 같은 판정)
func (m *M) judge() {
	if !m.proofload(ids(m.b)) {
		m.mer, m.mq = true, ""
		return
	}
	if fetchr(false, "-q", "origin", "+refs/heads/"+m.base+":refs/remotes/origin/"+m.base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 착지 뒤 git fetch 실패 — 일지를 두고 다음 바퀴에 판정")
		m.mer, m.mq = true, ""
		return
	}
	var r1, r2 int
	m.O, r1 = out("rev-parse", "-q", "--verify", "refs/remotes/origin/"+m.base+"^{commit}")
	C, r2 := "", 1
	if r1 == 0 {
		C, r2 = out("rev-parse", "-q", "--verify", "refs/heads/"+m.br)
	}
	if r1 != 0 || r2 != 0 {
		die("merger: 착지 뒤 후보·origin/%s 를 읽지 못했다 — 다음 실행이 일지로 판정한다", m.base)
	}
	proven, firstProof := 0, ""
	if m.b[0].d != "" {
		for _, e := range m.b {
			switch m.proof(e.d) {
			case 0:
				if firstProof != "" && firstProof != m.hs {
					if !m.recover() {
						m.mer, m.mq = true, ""
					}
					return
				}
				proven++
				firstProof = m.hs
			case 1:
			default:
				m.mer, m.mq = true, ""
				return
			}
		}
		if proven > 0 && proven < len(m.b) {
			if !m.recover() {
				m.mer, m.mq = true, ""
			}
			return
		}
		if proven == 0 && ianc(C, m.O) {
			// 같은 후보를 다른 작성자가 넣었을 수 있다. 살아 있는 큐 항목만 다음 바퀴가 정리한다
			m.fin()
			return
		}
		if firstProof != "" {
			C = firstProof
		}
	}
	if proven > 0 || ianc(C, m.O) {
		why := "병합기 착지"
		if len(m.b) > 1 {
			why += fmt.Sprintf("(묶음 %d: %s)", len(m.b), m.mlab)
		}
		if !m.doneAll(C, why) {
			m.mer, m.mq = true, ""
			return
		}
		removeQuiet(m.mj + "/cap")
		m.fin()
		return
	}
	switch m.lr {
	case "0":
		m.lr = "0(그런데 " + m.base + " 에 없음)"
	case "6":
		fmt.Fprintln(os.Stderr, "hc-lane merger: 함께 실은 lease 참조가 바뀜(그새 다시 제출함) — 후보를 버리고 다음 바퀴에")
		m.fin()
		return
	}
	if m.trans() && m.try(m.b[0].q, m.b[0].l) {
		// 일시 실패(기다림 상한·push 거절 횟수·원격 받기 실패)는 큐에 그대로 두고 쉰다(3번 넘으면 돌려보냄)
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 일시 실패(종료 %s, 로그 %s) — 큐에 두고 다음 바퀴에\n", m.b[0].l, m.lr, m.lgf)
		m.fin()
		m.mq, m.mer = "", true
		return
	}
	// 묶음이 실패하면(일시 실패 아님) 아무도 돌려보내지 않고 마지막 것을 빼고 다시. 혼자일 때 실패해야 그 제출을 돌려보낸다
	if len(m.b) > 1 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음 %d(%s) 실패(종료 %s, 로그 %s) — 마지막 %s 빼고 다시\n", len(m.b), m.mlab, m.lr, m.lgf, m.b[len(m.b)-1].l)
		m.writeCap(len(m.b) - 1)
		if m.fin() {
			m.mq = "x"
		}
		return
	}
	if m.trans() {
		m.lr += ", 일시 실패가 3번 넘음"
	}
	if !m.fin() {
		return
	}
	removeQuiet(m.mj + "/cap")
	if !m.fail(m.b[0].q, m.b[0].s, m.b[0].l, m.b[0].m, m.lgf, "착지 실패(종료 "+m.lr+")") {
		m.mer = true
	}
}

func ids(b []entry) []string {
	s := make([]string, len(b))
	for i, e := range b {
		s[i] = e.d
	}
	return s
}

func removeQuiet(p string) {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 를 지우지 못했다: %v\n", p, err)
	}
}

// proofload: 제출 ID 표식을 먼저 읽고 그 뒤에 기본 브랜치를 받는다(확정된 표식이 가리키는 객체는 그 이력에 있어야 한다)
func (m *M) proofload(ids []string) bool {
	var refs []string
	for _, id := range ids {
		if id != "" {
			refs = append(refs, "refs/hc-landed/"+id)
		}
	}
	m.lk2 = ""
	if len(refs) == 0 {
		return true
	}
	var rc int
	if m.lk2, rc = out(append([]string{"ls-remote", "origin"}, refs...)...); rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 제출 ID 표식을 읽지 못했다 — 일지를 보존한다")
		return false
	}
	return true
}

// proof: 0 그 제출 ID 표식이 기본 브랜치 이력에 있음(m.hs = 그 SHA) · 1 표식 없음 · 2 모름
func (m *M) proof(id string) int {
	m.hs = refsha(m.lk2, "refs/hc-landed/"+id)
	if m.hs == "" {
		return 1
	}
	if run("merge-base", "--is-ancestor", m.hs, m.O) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "hc-lane merger: 제출 ID %s 표식이 %s 이력에 있는지 확인하지 못했다 — 일지를 보존한다\n", id, m.base)
	return 2
}

// recover: 일지가 있으면(앞 실행이 끊김) 먼저 정리. true 정리함(새로 골라도 됨) · false 원격을 못 봐 쉼
func (m *M) recover() bool {
	b, err := os.ReadFile(m.mj + "/cur")
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	head, rest, nl := strings.Cut(string(b), "\n")
	if err != nil || !nl {
		die("merger: 일지를 읽지 못했다: %s/cur", m.mj)
	}
	hf := strings.Fields(head)
	for len(hf) < 3 {
		hf = append(hf, "")
	}
	m.br = hf[0]
	jC := hf[2]
	if len(strings.Fields(head)) > 3 {
		jC = strings.Join(strings.Fields(head)[2:], " ")
	}
	m.b = nil
	for _, line := range strings.Split(rest, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		for len(f) < 4 {
			f = append(f, "")
		}
		e := entry{q: f[0], s: f[1], l: f[2], m: strings.Join(f[3:], " ")}
		mt, ok := readMeta(strings.TrimPrefix(e.m, "-"), e.s)
		if !ok {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 끊긴 앞 실행의 제출 기록(%s)을 읽지 못했다 — 일지를 두고 다음 바퀴에\n", e.l)
			m.mer = true
			return false
		}
		e.d = mt.id
		m.b = append(m.b, e)
	}
	m.mbn = fmt.Sprint(len(m.b))
	if !m.proofload(ids(m.b)) {
		m.mer = true
		return false
	}
	if fetchr(false, "-q", "origin", "+refs/heads/"+m.base+":refs/remotes/origin/"+m.base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 끊긴 앞 실행 판정에 git fetch 실패 — 다음 바퀴에")
		m.mer = true
		return false
	}
	var rc int
	if m.O, rc = out("rev-parse", "-q", "--verify", "refs/remotes/origin/"+m.base+"^{commit}"); rc != 0 {
		die("merger: origin/%s 가 없다", m.base)
	}
	for _, e := range m.b {
		if e.d != "" {
			switch m.proof(e.d) {
			case 0:
			case 1:
				continue
			default:
				m.mer = true
				return false
			}
		} else {
			C, cr := out("rev-parse", "-q", "--verify", "refs/heads/"+m.br)
			switch cr {
			case 0:
			case 1:
				C = jC
			default:
				fmt.Fprintf(os.Stderr, "hc-lane merger: 끊긴 앞 실행의 후보 가지를 읽지 못했다(종료 %d) — 일지를 두고 다음 바퀴에\n", cr)
				m.mer = true
				return false
			}
			if !ianc(C, m.O) {
				fmt.Printf("hc-lane merger: 끊긴 앞 실행의 %s(제출 기록 없음) — main에 없다, 처음부터\n", e.l)
				continue
			}
			m.hs = C
		}
		fmt.Printf("hc-lane merger: 끊긴 앞 실행의 %s 가 %s 에 있다(%s) — 완료 처리\n", e.l, m.base, first8(m.hs))
		if m.done(e.q, e.s, e.l, e.m, m.hs, "앞 병합기 실행이 넣었음", false) != 0 {
			m.mer = true
			return false
		}
	}
	if !m.fin() {
		return false
	}
	m.mq = "x" // 이 바퀴에 일을 했다(--once가 이어 돈다)
	return true
}

// doneAll: 담긴 제출을 모두 완료. 망 오류면 false(일지를 두고 다음 바퀴에)
func (m *M) doneAll(C, why string) bool {
	for _, e := range m.b {
		if m.done(e.q, e.s, e.l, e.m, C, why, false) != 0 {
			return false
		}
	}
	return true
}

// fin: 임시 작업 트리·후보 가지·일지를 치운다(클론의 작업 트리는 건드리지 않는다)
func (m *M) fin() bool {
	if !m.wtclean() {
		m.defer_("후보 작업 트리 정리 실패")
		return false
	}
	if m.br != "" && m.br != "-" {
		switch runO("show-ref", "--exists", "refs/heads/"+m.br) {
		case 0:
			if run("branch", "-q", "-D", m.br) != 0 {
				m.defer_("후보 가지 정리 실패")
				return false
			}
		case 2:
		default:
			m.defer_("후보 가지 조회 실패")
			return false
		}
	}
	if err := os.Remove(m.mj + "/cur"); err != nil && !errors.Is(err, os.ErrNotExist) {
		m.defer_("후보 일지 정리 실패")
		return false
	}
	return true
}

// wtclean: 병합기 자기 임시 작업 트리(<클론>.merger-wt/ 아래, 앞 판의 .git/hc-merger/wt/ 아래도)를 모두 지운다
func (m *M) wtclean() bool {
	list, rc := out("worktree", "list", "--porcelain")
	if rc != 0 {
		return false
	}
	for _, line := range strings.Split(list, "\n") {
		wt, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		if strings.HasPrefix(wt, m.mwr+"/") || strings.HasPrefix(wt, m.mj+"/wt/") {
			if run("worktree", "remove", "-f", "-f", wt) != 0 {
				return false
			}
		}
	}
	if run("worktree", "prune") != 0 {
		return false
	}
	// mktemp 뒤 등록 전에 중단된 후보의 빈 폴더도 회수한다
	des, err := os.ReadDir(m.mwr)
	if err != nil {
		return false
	}
	ok := true
	for _, d := range des {
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "c-") || len(d.Name()) != 8 {
			continue
		}
		p := m.mwr + "/" + d.Name()
		if sub, err := os.ReadDir(p); err == nil && len(sub) == 0 {
			if err := os.Remove(p); err != nil {
				ok = false
			}
		}
	}
	return ok
}

// existing: 이미 들어간 제출도 큐 삭제 전에 복구 일지를 쓴다(후보 가지는 없다)
func (m *M) existing(C, why string) {
	m.keep(0)
	m.mbn, m.br, m.C0 = "1", "-", C
	m.jwrite()
	switch m.done(m.b[0].q, m.b[0].s, m.b[0].l, m.b[0].m, C, why, true) {
	case 0, 2:
		m.fin()
	default:
		m.mer, m.mq = true, ""
	}
}

var transLog = regexp.MustCompile(`git fetch( origin)? 실패|push 결과 모름|push 실패\(다른 쪽`)

// trans: 다시 하면 될 착지 실패인가 — 종료 75(기다림 상한)·4(push 거절 횟수)·7(rebase로 기본 브랜치에 막 생긴 착지 스크립트를 만남),
// 원격 받기 실패·push 전송만 끊김(로그). 충돌·검증 실패는 아니다
func (m *M) trans() bool {
	switch m.lr {
	case "75", "4", "7":
		return true
	}
	b, err := os.ReadFile(m.lgf)
	return err == nil && transLog.Match(b)
}

// try: 그 제출의 일시 실패를 하나 세어 3번까지면 true(큐에 두고 쉼), 넘으면 false(돌려보냄)
func (m *M) try(q, ln string) bool {
	tf := m.mj + "/" + ln + ".tr"
	tc := 0
	if b, err := os.ReadFile(tf); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == q {
				if n, err := strconv.Atoi(f[1]); err == nil {
					tc = n
				}
			}
		}
	}
	tc++
	if err := os.WriteFile(tf, []byte(fmt.Sprintf("%s %d\n", q, tc)), 0o666); err != nil {
		die("merger: 일시 실패 횟수를 쓰지 못했다: %s", tf)
	}
	return tc <= 3
}
