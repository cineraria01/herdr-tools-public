package merger_test

// 병합기 계약 시험(옛 tests/hc-lane-merger.test.sh — 121단언, 이름·차례 그대로): 임시 bare origin + 레인 클론 둘(a·b) + 병합기 전용
// 클론(m), 가짜 hc. 묶음 안에서는 앞 절의 큐·main·사건이 다음 절로 이어진다. 묶음(TestMergerContractA~E)은 저마다 newM으로 새로
// 준비하니 혼자서도·어떤 차례로도 돈다(2026-11-23 contract-split — 한 시험이던 때 검증의 가장 긴 조각이었다). 새 절은 기대는 상태가
// 있는 묶음에 넣는다(A 끝의 「병합기 일지」 확인은 A의 l-a·l-d·l-live를 본다).
// bash 병합기(HC_LANE_MERGER_IMPL=bash) 갈래는 그 판이 없어져 버렸다(재제출 경합·우선 순서는 Go 판 단언만).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// M: 이 시험의 셸 전역(W·O·out·rc·mout·mrc …)
type M struct {
	*K
	W, O          string
	EV, LE, MJ    string
	out, errS     string
	rc            int
	mout, merr    string
	mrc           int
	verify, failG string // $T/verify.log, $T/fail/git
}

func (m *M) og(args ...string) string { return m.git(m.O, args...) }
func (m *M) ogOK(args ...string) bool { return m.gitOK(m.O, args...) }
func (m *M) qrefs() string            { return m.og("for-each-ref", "--format=%(refname)", "refs/merge-queue/") }
func (m *M) mainlog() string          { return m.og("log", "--format=%s", "main") }
func (m *M) inMain(s string) bool     { return hasLine(m.mainlog(), s) }
func (m *M) inQ(s string) bool        { return strings.Contains(m.qrefs(), s) }
func (m *M) proj(c string) string     { return filepath.Join(m.W, c, "proj") }
func (m *M) mg() string               { return filepath.Join(m.W, "m/proj/.git/hc-merger") }
func (m *M) failed() string {
	return m.og("for-each-ref", "--format=%(refname)", "refs/merge-failed/")
}
func (m *M) vcount() int { return countNonEmpty(read(m.verify)) }

// lane <클론> <레인> <파일> <내용>: 그 클론에서 origin/main 위 새 레인 가지에 커밋 하나
func (m *M) lane(c, name, file, content string) {
	m.t.Helper()
	m.sh(m.proj(c), `git fetch -q origin && git checkout -q -B "$1" origin/main && printf '%s\n' "$3" >"$2" && git add -- "$2" && git commit -qm "$1" -- "$2"`, name, file, content)
}

// land <클론> <레인> [VAR=값…] [-- 인자…]: out·rc·errS
func (m *M) land(c, name string, extra ...string) {
	m.landA(c, append([]string{"HC_LANE=" + name}, extra...))
}

func (m *M) landA(c string, extra []string, args ...string) {
	r := m.K.lane(m.proj(c), extra, append([]string{"land"}, args...)...)
	m.out, m.rc, m.errS = r.Out, r.RC, r.Err
}

// merge [인자…]: 병합기 한 바퀴(--once). mergeE: 이 부름에만 VAR=값
func (m *M) merge(args ...string) { m.mergeE(nil, 0, args...) }

func (m *M) mergeE(extra []string, max time.Duration, args ...string) {
	p := m.proj("m")
	r := m.merger(p, extra, max, append([]string{"--repo", p, "--once"}, args...)...)
	m.mout, m.mrc, m.merr = r.Out, r.RC, r.Err
}

// mfail: PATH 앞에 가짜 git($T/fail)
func (m *M) mfail() []string {
	return []string{"PATH=" + filepath.Join(m.T, "fail") + ":" + m.env["PATH"]}
}

// fake: $T/fail/git — 셸 printf 꼴 본문(%[1]s = 실제 git)
func (m *M) fake(body string) { m.stub(m.failG, body) }

func (m *M) ev(lane, state string) []string { return evs(m.EV, lane, state) }

func (m *M) on()  { m.og("update-ref", "refs/herdr/merger/box", "main") }
func (m *M) off() { m.og("update-ref", "-d", "refs/herdr/merger/box") }

func (m *M) dm() string { return "rc=" + strconv.Itoa(m.mrc) + "\n" + m.merr + "\n" + m.mout }
func (m *M) dl() string { return "rc=" + strconv.Itoa(m.rc) + "\n" + m.errS + "\n" + m.out }

// delRefs: 그 접두 아래 이름에 s가 든 원격 참조와 덧붙인 참조를 지운다
func (m *M) delRefs(s string, prefixes []string, more ...string) {
	for _, r := range lines(m.og(append([]string{"for-each-ref", "--format=%(refname)"}, prefixes...)...)) {
		if strings.Contains(r, s) {
			m.og("update-ref", "-d", r)
		}
	}
	for _, r := range more {
		m.og("update-ref", "-d", r)
	}
}

// newM: 묶음 하나의 준비 — 시험 폴더·가짜 hc·검증 스크립트·bare origin과 클론 a·b·m. 켬 표식은 없다(꺼짐)
func newM(t *testing.T) *M {
	k := newK(t)
	m := &M{K: k, W: filepath.Join(k.T, "공백 있는 폴더")}
	m.O = filepath.Join(m.W, "origin.git")
	m.EV = filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
	m.LE = filepath.Join(k.T, ".local/state/hc-lane/land-events.tsv")
	m.MJ = filepath.Join(k.T, ".local/state/hc-lane/merger-journal.tsv")
	m.verify, m.failG = filepath.Join(k.T, "verify.log"), filepath.Join(k.T, "fail/git")
	T, W := k.T, m.W
	mkdirs(t, W, filepath.Join(T, "vf"), filepath.Join(T, "fail"))
	k.stub(filepath.Join(T, "bin/hc"), `#!/bin/bash
case $1 in
  get) case $2 in
         t-live) echo '{"foreground":{"name":"node"},"name": "l-live","state":"live","created_ms":11}'; exit 0 ;;
         t-far|box2/t-far) echo '{"name": "l-far","state":"live","created_ms":12}'; exit 0 ;;
         t-fg) echo '{"foreground":{"name":"l-fg"},"name":"other","state":"live","created_ms":13}'; exit 0 ;;   # 레인 이름은 foreground에만
       esac; echo '{"error":{"kind":"not_found"}}' >&2; exit 3 ;;
  ls) echo '{"terminals":[]}' ;;
  send) [ ! -e "$T/hcfail" ] || exit 1; printf '%s\t%s\n' "$2" "$3" >>"$T/hc.send" ;;
  *) exit 0 ;;
esac
`)
	// 병합기 MVP-1은 한 건씩. 켬 표식 조회 재시도는 기다리지 않게. 묶음은 아래 「묶음」에서 HC_LANE_MERGER_BATCH=3으로
	k.set("W="+W, "O="+m.O, "HC_LANE_MGON_WAITS=0", "HC_LANE_MERGER_BATCH=1")
	k.detail = func() string { return "" }

	// origin: 검증 스크립트는 가지 이름의 실패 표지($T/vf/<가지>)가 있으면 실패하고, 돈 가지를 verify.log에 적는다
	write(t, filepath.Join(W, "seed/.claude/herdr-verify.sh"), `#!/bin/bash
b=$(git rev-parse --abbrev-ref HEAD)
echo "verify $b" >>"$T/verify.log"
[ ! -x "$T/vhook.$b" ] || "$T/vhook.$b"   # 시험이 검증 도중의 일을 끼워 넣는다(같은 레인 다시 제출 등)
[ ! -e "$T/vf/$b" ] || { echo "vfail $b"; exit 1; }
[ ! -e bad.txt ] || { echo "vfail bad.txt"; exit 1; }   # 내용으로 실패(묶음 시험 — 어느 가지 이름으로 돌든)
`)
	k.sh(W, `git init -q --bare -b main "$1" && cd seed && git init -q -b main && printf 'base\n' >shared.txt && git add -A && git commit -qm base &&
git remote add origin "$1" && git push -q origin main && cd .. && git clone -q "$1" a/proj && git clone -q "$1" b/proj && git clone -q "$1" m/proj`, m.O)
	return m
}

// cntV: verify.log에서 그 줄(「verify <가지>」)의 수
func (m *M) cntV(s string) int {
	n := 0
	for _, l := range lines(read(m.verify)) {
		if l == s {
			n++
		}
	}
	return n
}

// TestMergerContractA: 끔·제출·병합기 기본·충돌·검증 실패·이어 받기·잠금·더러움·git 실패·응답 유실·이름 꼴·clean(1~15),
// 결과 알림(37)·병합기 일지
func TestMergerContractA(t *testing.T) {
	m := newM(t)
	k := m.K
	T, rg, cntV := k.T, k.realGit, m.cntV

	// 1. 설정이 없으면 land는 예전 그대로(제출 없이 main에 착지)
	m.lane("a", "l-old", "old.txt", "old")
	m.land("a", "l-old")
	k.check("끔: land가 그대로 착지", m.rc == 0 && head(m.mainlog(), 1) == "l-old" && m.qrefs() == "", m.dl())

	// 2. 켬(원격 참조 refs/herdr/merger/<기계>): land가 제출로. 개인 설정(lane.*)에 무엇이 있어도 원격 참조만 본다
	write(t, filepath.Join(T, ".config/herdr-tools/config"), "lane.merger nothing=here\n")
	m.on()
	m.lane("a", "l-a", "a.txt", "a")
	sa := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("제출: 종료 0, main은 그대로", m.rc == 0 && head(m.mainlog(), 1) == "l-old" && strings.Contains(m.out, "병합기(box)"), m.dl())
	q := m.qrefs()
	k.check("제출: lane/l-a와 큐 표식이 같은 SHA", m.og("rev-parse", "refs/heads/lane/l-a") == sa && len(lines(q)) == 1 &&
		m.og("rev-parse", q) == sa && grepE(q, `^refs/merge-queue/[0-9]{10}-l-a$`), q)
	fk := k.git(m.proj("a"), "merge-base", "HEAD", "origin/main")
	meta := strings.Replace(q, "refs/merge-queue/", "refs/merge-queue-meta/", 1)
	mb := m.og("log", "-1", "--format=%B", meta)
	k.check("제출: 기록 refs/merge-queue-meta/<같은 이름> — 부모 = 원본, 원본·갈래점·제출 ID", m.og("rev-parse", meta+"^") == sa &&
		strings.Contains(mb, "orig="+sa+" fork="+fk) && grepE(mb, `lane=l-a id=[^ ]+-[0-9]{8}T[0-9]{6}-[0-9]+ `), mb)
	// 기록은 병합기 일지와 기존 report 사건만(land-metrics의 착지 사건 파일은 이 가지에 없다)
	k.check("제출: 안내 줄, 착지 사건 파일은 안 씀", strings.Contains(m.out, "hc-lane land: 제출") && !strings.Contains(read(m.LE), sa), m.out)
	// 3. 같은 레인이 다시 제출하면 큐에는 새 것 하나
	k.sh(m.proj("a"), `printf 'a2\n' >a.txt && git commit -qam l-a2`)
	sa = k.git(m.proj("a"), "rev-parse", "HEAD")
	time.Sleep(time.Second)
	m.land("a", "l-a")
	q = m.qrefs()
	k.check("다시 제출: 큐 표식은 새 SHA 하나, lane 가지도 새 SHA", m.rc == 0 && len(lines(q)) == 1 && m.og("rev-parse", q) == sa &&
		m.og("rev-parse", "refs/heads/lane/l-a") == sa, m.dl())
	// 4. 둘째 레인 제출 → 병합기가 순서대로 둘 다 착지, 큐·lane 가지 지움, 착지 사건 --name
	m.lane("b", "l-b", "b.txt", "b")
	m.land("b", "l-b")
	v0 := m.vcount()
	m.merge()
	k.check("병합기: 둘 다 제출 순서대로 착지", m.mrc == 0 && head(m.mainlog(), 2) == "l-b,l-a2", m.dm())
	k.check("병합기: 큐·기록·lane 가지가 비고 실패 표식 없음", m.qrefs() == "" &&
		m.og("for-each-ref", "refs/heads/lane/", "refs/merge-failed/", "refs/merge-queue-meta/") == "")
	k.check("다시 제출: 옛 기록도 지워져 기록이 쌓이지 않음(위 줄)·착지 사건에 제출 ID", anyHas(m.ev("l-a", "착지"), "제출 ID"))
	k.check("병합기: 레인 이름으로 착지 사건", len(m.ev("l-a", "착지")) > 0 && len(m.ev("l-b", "착지")) > 0)
	vl := lines(read(m.verify))
	k.check("병합기: 하나씩 — 검증 두 번(가지 = 레인 이름)", m.vcount() == v0+2 && strings.Join(vl[len(vl)-2:], ",") == "verify l-a,verify l-b", read(m.verify))
	n := 0
	for _, r := range rows(m.EV) {
		if col(r, 5) == "착지" && strings.Contains(strings.Join(r, "\t"), "병합기 착지") {
			n++
		}
	}
	k.check("병합기: 착지 사건은 레인마다 「병합기 착지」", n == 2)
	// 접수가 확정되면 사건 상태 「제출」(한 줄 첫 낱말이 제출 ID)
	k.check("제출 사건: 레인마다 상태 「제출」, 「<ID> → <큐>」", grepE(strings.Join(m.ev("l-b", "제출"), "\n"),
		`\t[^ \t]+-[0-9]{8}T[0-9]{6}-[0-9]+ → refs/merge-queue/[0-9]{10}-l-b`))
	k.check("병합기: 클론 작업 트리는 그대로·깨끗함, 로컬 레인 가지·임시 작업 트리 없음", k.git(m.proj("m"), "status", "--porcelain") == "" &&
		!k.gitOK(m.proj("m"), "show-ref", "-q", "--verify", "refs/heads/l-a") && countNonEmpty(k.git(m.proj("m"), "worktree", "list")) == 1)

	// 5. 충돌 제출은 merge-failed로, 다음 제출은 계속
	m.lane("a", "l-c", "shared.txt", "c-side")
	m.lane("b", "l-d", "shared.txt", "d-side")
	m.land("a", "l-c")
	m.land("b", "l-d")
	m.lane("a", "l-e", "e.txt", "e")
	m.land("a", "l-e")
	m.merge()
	k.check("충돌: 먼저 낸 l-c는 착지", m.og("show", "main:shared.txt") == "c-side", m.dm())
	k.check("충돌: l-d는 refs/merge-failed로, lane/l-d는 남음", grepE(m.failed(), `^refs/merge-failed/[0-9]{10}-l-d$`) &&
		m.ogOK("rev-parse", "-q", "--verify", "refs/heads/lane/l-d"))
	k.check("충돌: 다음 제출 l-e는 계속 착지", m.inMain("l-e") && m.qrefs() == "")
	ld := m.ev("l-d", "막힘")
	k.check("충돌: l-d 막힘 사건(까닭·원본·기준·제출 ID·로그 경로, Go 판은 충돌 파일도)", func() bool {
		for _, l := range ld {
			if grepE(l, `착지 실패\(종료 3(\)|, 충돌 shared\.txt\))`) && strings.Contains(l, "원본 ") && strings.Contains(l, "기준 ") &&
				strings.Contains(l, "제출 ID") && strings.Contains(l, "merger-logs") {
				return true
			}
		}
		return false
	}(), strings.Join(ld, "\n"))
	k.check("충돌: 제출 기록은 refs/merge-failed-meta로", grepE(m.og("for-each-ref", "--format=%(refname)", "refs/merge-failed-meta/"),
		`^refs/merge-failed-meta/[0-9]{10}-l-d$`) && m.og("for-each-ref", "refs/merge-queue-meta/") == "")
	// 반송된 제출은 원격 lane/l-d가 그 SHA 그대로여도 큐 표식이 없어 「제출됨」이 아니다 — clean이 가지를 지우지 않는다
	k.sh(m.proj("b"), `git checkout -q --detach`)
	c := m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-d")
	k.check("clean: 반송된 제출(lane 가지만 남음)은 지우지 않음(멈춤)", c.RC == 1 && k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-d") &&
		!strings.Contains(c.Out+c.Err, "제출됨"), c.Out, c.Err)

	// 6. 검증 실패도 merge-failed
	m.lane("a", "l-f", "f.txt", "f")
	m.land("a", "l-f")
	touch(t, filepath.Join(T, "vf/l-f"))
	m.merge()
	k.check("검증 실패: merge-failed·막힘, main에 안 들어감", !m.inMain("l-f") && strings.Contains(m.failed(), "l-f") && len(m.ev("l-f", "막힘")) > 0, m.dm())

	// 7. 이미 main에 들어간 제출은 정리만(다시 착지하지 않음)
	m.og("update-ref", "refs/merge-queue/1000000000-l-in", m.og("rev-parse", "main~1"))
	m.merge()
	k.check("이미 들어감: 큐 표식만 지우고 착지 사건, main 그대로", m.qrefs() == "" && head(m.mainlog(), 1) == "l-e" && anyHas(m.ev("l-in", "착지"), "이미"), m.dm())

	// 8. 끊긴 앞 실행(구조 A — 일지 하나): 일지의 후보가 이미 main에 들어갔으면, 새 제출이 둘 와 있어도 묶음을 고르기 전에 그 제출을
	//    정리만 한다. main이 움직인 뒤 rebase한 후보라 원본 SHA는 main의 조상이 아니다
	m.lane("a", "l-h", "h.txt", "h")
	m.land("a", "l-h")
	qh := m.qrefs()
	sh := m.og("rev-parse", qh)
	m.lane("b", "l-hx", "hx.txt", "hx")
	m.K.lane(m.proj("b"), []string{"HERDR_MERGER=1", "HC_LANE=l-hx"}, "land") // main을 움직인다
	bh := m.og("rev-parse", "main")
	k.sh(m.proj("m"), `git fetch -q origin && git checkout -q -B l-h "$1"`, sh)
	m.K.lane(m.proj("m"), []string{"HERDR_MERGER=1", "HC_LANE=l-h"}, "land")
	write(t, m.mg()+"/cur", fmt.Sprintf("%s %s %s\n%s %s %s %s\n", "l-h", bh, sh, qh, sh, "l-h", "-"))
	k.sh(m.proj("m"), `git checkout -q --detach`)
	m.og("update-ref", "-d", qh) // 그 착지의 main push가 큐 표식을 함께 지운 것처럼
	m.lane("a", "l-h2", "h2.txt", "h2")
	m.land("a", "l-h2")
	m.lane("b", "l-h3", "h3.txt", "h3")
	m.land("b", "l-h3")
	vb := cntV("verify l-h")
	m.merge()
	k.check("이어 받기: 앞 실행이 넣은 제출은 다시 넣지 않고 정리(착지 사건 「앞 병합기 실행이 넣었음」), 새 둘도 들어감",
		!m.ogOK("merge-base", "--is-ancestor", sh, "main") && strings.Count("\n"+m.mainlog()+"\n", "\nl-h\n") == 1 && cntV("verify l-h") == vb &&
			anyHas(m.ev("l-h", "착지"), "앞 병합기 실행이 넣었음") && m.inMain("l-h2") && m.inMain("l-h3") && m.qrefs() == "" && !exists(m.mg()+"/cur"), m.dm())

	// 9. 병합기는 한 클론에 하나(구조 C — 잠금 파일의 flock): 누가 쥐고 있으면 종료 75
	// 잠금 이름은 셸 시험처럼 클론 경로의 글자마다 _(tr -c, UTF-8 로캘). 병합기는 바이트마다 _ 꼴도 함께 쥔다
	ck := strings.Map(func(r rune) rune {
		if r < 128 && (r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return r
		}
		return '_'
	}, m.proj("m"))
	lockP := filepath.Join(T, ".local/state/hc-lane/merger."+ck+".lock")
	_, rel := holdLock(t, lockP)
	m.merge()
	k.check("잠금: 이미 돌면 종료 75", m.mrc == 75 && strings.Contains(m.merr, "아직 돈다"), m.dm())
	rel()

	// 10. 제출 전 커밋 안 된 변경이면 멈춤, 아무것도 안 보냄
	m.lane("a", "l-dirty", "x.txt", "x")
	k.sh(m.proj("a"), `echo dirty >>x.txt`)
	m.land("a", "l-dirty")
	k.check("더러움: 제출 안 함", m.rc == 1 && m.qrefs() == "" && !m.ogOK("rev-parse", "-q", "--verify", "refs/heads/lane/l-dirty") &&
		strings.Contains(m.errS, "커밋 안 된"), m.dl())
	k.sh(m.proj("a"), `git checkout -q -- x.txt`)

	// 11. git 실패를 「없음」으로 읽지 않는다: 제출 중 ls-remote가 실패하면 멈추고 아무것도 안 보냄
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" ls-remote \"*merge-queue*) echo \"fatal: unable to access\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.landA("a", append(m.mfail(), "HC_LANE=l-dirty"))
	k.check("원격 못 읽음: 제출 멈춤(모름 — 75, 같은 줄을 다시)", m.rc == 75 && m.qrefs() == "" && strings.Contains(m.errS, "원격을 읽지 못했다"), m.dl())
	// 병합기 바퀴에서 ls-remote가 실패하면 큐가 빈 것으로 읽지 않고 다음 바퀴(아무것도 안 지움)
	m.land("a", "l-dirty")
	m.mergeE(m.mfail(), 0)
	k.check("병합기 원격 못 읽음: 큐 그대로·안내, --once는 75(빈 큐와 가림 — 1차 중간 4)", m.mrc == 75 && len(lines(m.qrefs())) == 1 &&
		strings.Contains(m.merr, "원격 큐를 읽지 못했다"), m.dm())
	m.merge()

	// 12. 제출 push 응답 유실: 서버는 받았는데 실패로 돌아와도 두 참조가 그 SHA면 제출로
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" push \"*--atomic*) %[1]s \"$@\" >/dev/null 2>&1; echo \"fatal: the remote end hung up unexpectedly\" >&2; exit 128 ;; esac\nexec %[1]s \"$@\"\n", rg))
	m.lane("a", "l-lost", "l.txt", "l")
	m.landA("a", append(m.mfail(), "HC_LANE=l-lost"))
	k.check("응답 유실: 원격을 다시 읽어 제출로", m.rc == 0 && m.inQ("l-lost"), m.dl())
	m.merge()
	k.check("응답 유실 뒤 병합기가 넣음", m.inMain("l-lost"), m.dm())

	// 13. 병합기 안 착지(HERDR_MERGER=1)는 설정이 켜져 있어도 제출하지 않고 착지
	m.lane("a", "l-direct", "d.txt", "d")
	m.landA("a", []string{"HERDR_MERGER=1", "HC_LANE=l-direct"})
	k.check("HERDR_MERGER=1: 착지 그대로", m.rc == 0 && head(m.mainlog(), 1) == "l-direct" && !m.inQ("l-direct"), m.dl())

	// 14. 레인 이름 꼴이 아니면 제출하지 않는다
	m.lane("a", "Bad_Name", "bn.txt", "bn")
	m.land("a", "Bad_Name")
	k.check("레인 이름 꼴: 제출 안 함", m.rc == 1 && m.qrefs() == "" && strings.Contains(m.errS, "레인 이름"), m.dl(), m.qrefs())

	// 15. 제출한 가지는 큐 표식에 그 SHA 그대로 있을 때 clean이 지운다. 원격 참조가 지워졌는데 낡은 추적 참조만 남았으면 지우지 않는다
	m.lane("b", "l-cl", "cl.txt", "cl")
	m.land("b", "l-cl")
	m.lane("b", "l-cs", "cs.txt", "cs")
	m.land("b", "l-cs")
	m.delRefs("l-cs", []string{"refs/merge-queue/", "refs/merge-queue-meta/"}, "refs/heads/lane/l-cs")
	k.sh(m.proj("b"), `git checkout -q --detach`)
	c = m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-cl")
	k.check("clean: 큐 표식에 그대로 있는 제출 가지는 지움(제출됨)", c.RC == 0 && !k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-cl") &&
		strings.Contains(c.Out+c.Err, "제출됨: 브랜치 l-cl 은 큐 표식 refs/merge-queue/"), c.Out, c.Err)
	c = m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-cs")
	k.check("clean: 원격 표식이 지워지고 낡은 추적 참조만 있으면 지우지 않음(멈춤)", c.RC == 1 && k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-cs"), c.Out, c.Err)
	m.lane("b", "l-ns", "ns.txt", "ns")
	k.sh(m.proj("b"), `git checkout -q --detach`)
	c = m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-ns")
	k.check("clean: 제출 안 한 가지는 멈춤(그대로)", c.RC == 1 && k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-ns"), c.Out, c.Err)
	// 이름을 바꿔 낸 제출(다시 맞춤·고침 레인이 HC_LANE=<원래>로): 큐 표식은 <원래> 이름이라도 land가 가지 설정에 남긴 낸 이름으로 찾는다
	m.lane("b", "l-rn-fix", "rn.txt", "rn")
	m.land("b", "l-rn")
	k.sh(m.proj("b"), `git checkout -q --detach`)
	c = m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-rn-fix")
	k.check("clean: 이름을 바꿔 낸 제출도 제출됨으로 지움", m.rc == 0 && c.RC == 0 && !k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-rn-fix") &&
		strings.Contains(c.Out+c.Err, "제출됨: 브랜치 l-rn-fix 은 큐 표식 refs/merge-queue/") && strings.Contains(c.Out+c.Err, "-l-rn "), m.dl(), c.Out, c.Err)
	m.delRefs("l-rn", []string{"refs/merge-queue/", "refs/merge-queue-meta/"}, "refs/heads/lane/l-rn")

	// 37. 결과 알림: 제출한 레인 터미널이 이 기계에 살아 있고 이름이 맞으면 hc send, 사건은 제출 때 적은 지휘(--to)로
	m.lane("a", "l-live", "live.txt", "live")
	m.landA("a", []string{"HC_TERMINAL_ID=t-live", "HC_LANE_OWNER=t-own", "HC_LANE_JOB=j1120-abc", "HC_LANE=l-live"})
	m.merge()
	sent := read(T + "/hc.send")
	k.check("알림: 레인 터미널에 hc send, 착지 사건 --to 지휘", func() bool {
		for _, l := range lines(sent) {
			if strings.Contains(l, "t-live\t") && strings.Contains(l, "착지") {
				for _, r := range rows(m.EV) {
					if col(r, 4) == "l-live" && col(r, 5) == "착지" && col(r, 9) == "t-own" {
						return true
					}
				}
			}
		}
		return false
	}(), sent, strings.Join(m.ev("l-live", "착지"), "\n"))

	// 병합기 일지: 제출마다 12칸 한 줄
	k.check("병합기 일지: 착지는 후보 40자·묶음 크기·착지 시작/끝·작업 ID, 돌려보냄은 후보 -", func() bool {
		a, l, d := false, false, false
		for _, r := range rows(m.MJ) {
			switch {
			case col(r, 3) == "l-a" && len(r) == 12 && col(r, 10) == "착지" && grepE(col(r, 11), `^[0-9a-f]{40}$`) && col(r, 7) == "1" && col(r, 8) != "-" && col(r, 9) != "-":
				a = true
			}
			if col(r, 3) == "l-live" && col(r, 5) == "j1120-abc" && col(r, 4) != "-" {
				l = true
			}
			if col(r, 3) == "l-d" && col(r, 10) == "돌려보냄" && col(r, 11) == "-" {
				d = true
			}
		}
		return a && l && d
	}(), read(m.MJ))

	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// TestMergerContractB: 재제출 경합·큐 받기 실패·켬 모름·선행·인자·report 착지·이름·일시 실패·착지 뒤 fetch 실패·rebase 충돌·멈춤·
// 고아·SIGKILL(17~30)
func TestMergerContractB(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	T, rg, cntV := k.T, k.realGit, m.cntV

	// 17. 재제출 경합: Go 병합기는 착지 직전 원격 refs/herdr/landing/<큐>를 올려 착지 중 재제출을 hc-lane land가 종료 2로 거절한다 —
	//     옛 제출이 막힘 없이 들어가고 큐는 빈다
	m.merge() // 앞 시험(15)이 남긴 l-cl 제출을 먼저 넣는다
	m.lane("a", "l-rs", "rs.txt", "rs1")
	m.land("a", "l-rs")
	k.stub(filepath.Join(T, "vhook.l-rs"), "#!/bin/sh\nrm -f \"$T/vhook.l-rs\"\ncd \"$W/a/proj\" && git checkout -q -B l-rs origin/main && printf \"rs2\\n\" >rs2.txt && git add rs2.txt && git commit -qm l-rs2 rs2.txt && env -u HERDR_MERGER HC_LANE=l-rs \"$LANE\" land >/dev/null 2>\"$T/rs.err\"\necho $? >\"$T/rs.rc\"\nexit 0\n")
	m.merge()
	k.check("재제출 경합(Go 병합기): 착지 중 재제출은 종료 2·안내, 옛 제출이 막힘 없이 착지, 큐 빔", m.mrc == 0 &&
		strings.TrimSpace(read(T+"/rs.rc")) == "2" && strings.Contains(read(T+"/rs.err"), "l-rs은 병합기가 지금 착지 중") &&
		m.ogOK("cat-file", "-e", "main:rs.txt") && !m.ogOK("cat-file", "-e", "main:rs2.txt") && len(m.ev("l-rs", "막힘")) == 0 && m.qrefs() == "",
		m.dm(), read(T+"/rs.err"), m.qrefs())

	// 18. 큐 표식을 받지 못하면(그새 바뀜·망) 그 바퀴는 쉰다 — --once는 끝나고(몰아치며 되풀이하지 않음) 큐는 그대로
	m.lane("a", "l-ff", "ff.txt", "ff")
	m.land("a", "l-ff")
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" fetch \"*refs/merge-queue/*) echo \"fatal: could not fetch\" >&2; exit 1 ;; esac\nexec %s \"$@\"\n", rg))
	m.mergeE(m.mfail(), 60*time.Second)
	k.check("큐 받기 실패: --once가 75로 끝나고(몰아치지 않음) 큐 그대로", m.mrc == 75 && m.inQ("l-ff") && strings.Contains(m.merr, "git fetch 실패"), m.dm())
	m.merge()

	// 19. 켬 표식을 읽지 못하면(켜졌는지 모름) 다시 보고, 끝내 모르면 착지하지 않는다 — 종료 75, main·큐·lane 가지 그대로
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" ls-remote \"*herdr/merger*) echo \"fatal: unable to access\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.lane("a", "l-unk", "unk.txt", "unk")
	m0 := m.og("rev-parse", "main")
	m.landA("a", append(m.mfail(), "HC_LANE=l-unk"))
	k.check("켬 모름: 착지하지 않고 75, main·큐 그대로", m.rc == 75 && m.og("rev-parse", "main") == m0 && !m.inQ("l-unk") && strings.Contains(m.errS, "켜졌는지 모른다"), m.dl())
	k.check("켬 모름: lane 가지도 안 보냄", !m.ogOK("rev-parse", "-q", "--verify", "refs/heads/lane/l-unk"))
	m.land("a", "l-unk")
	m.merge() // 정리: 이제 제출·착지

	// 20. 선행(HERDR_LAND_AFTER)은 최소판에서 뺐다 — 주면 제출하지 않는다(조용히 무시하지 않음)
	m.lane("a", "l-dep", "dep.txt", "dep")
	m.landA("a", []string{"HERDR_LAND_AFTER=l-pre", "HC_LANE=l-dep"})
	k.check("선행: 주면 제출 안 함(안내)", m.rc == 1 && !m.inQ("l-dep") && strings.Contains(m.errS, "HERDR_LAND_AFTER"), m.dl())

	// 21. --batch는 모르는 인자
	m.merge("--batch", "3")
	k.check("MVP-1: --batch는 없는 인자(종료 1)", m.mrc == 1 && strings.Contains(m.merr, "알 수 없는 인자: --batch"), m.dm())
	// 병합기가 넘긴 착지 인자(HERDR_LAND_PUSH_EXTRA·HERDR_MERGER)는 검증 자식에 물려주지 않는다
	m.lane("a", "l-env", "env.txt", "env")
	m.land("a", "l-env")
	k.stub(T+"/vhook.l-env", "#!/bin/sh\nprintf \"%s|%s\\n\" \"${HERDR_LAND_PUSH_EXTRA-없음}\" \"${HERDR_MERGER-없음}\" >\"$T/env.l-env\"\nrm -f \"$T/vhook.l-env\"\n")
	m.merge()
	k.check("검증 자식 환경: HERDR_LAND_PUSH_EXTRA·HERDR_MERGER 없음, 착지는 됨", strings.TrimSpace(read(T+"/env.l-env")) == "없음|없음" && m.inMain("l-env") && m.qrefs() == "",
		read(T+"/env.l-env"), m.dm())

	// 24. 제출한 커밋의 report 착지는 적지 않는다. 착지는 병합기가 --name으로
	m.lane("a", "l-rp", "rp.txt", "rp")
	m.land("a", "l-rp")
	srp := k.git(m.proj("a"), "rev-parse", "--short", "HEAD")
	rr := m.K.lane(m.proj("a"), []string{"HC_LANE=l-rp"}, "report", "착지", srp, "넣음")
	k.check("제출 뒤 report 착지: 종료 0, 안 적고 안내", rr.RC == 0 && strings.Contains(rr.Out+rr.Err, "제출만") && len(m.ev("l-rp", "착지")) == 0, rr.Out, rr.Err)
	k.check("제출 안내 줄: report 착지는 적히지 않음", strings.Contains(m.out, "report 착지는 부르지 않아도"), m.out)
	m.merge()
	k.check("제출 뒤 착지 사건은 병합기 것 하나", len(m.ev("l-rp", "착지")) == 1)

	// 25. 병합기가 쓰는 가지 이름(기본 브랜치·묶음 가지)은 레인 이름으로 받지 않는다
	m.lane("a", "merge-batch", "mbn.txt", "mbn")
	m.land("a", "merge-batch")
	k.check("레인 이름 merge-batch: 제출 안 함", m.rc == 1 && m.qrefs() == "" && strings.Contains(m.errS, "병합기가 쓰는 가지 이름"), m.dl())
	k.sh(m.proj("a"), `git checkout -q --detach && git branch -q -D merge-batch`)

	// 26. 일시 실패(착지 안 git fetch 실패·종료 75·4)는 돌려보내지 않고 큐에 두고 쉰다 — 3번 넘으면 그때 돌려보냄
	m.lane("a", "l-tr", "tr.txt", "tr")
	m.land("a", "l-tr")
	m.fake(fmt.Sprintf("#!/bin/sh\n[ \"$*\" != \"fetch -q origin\" ] || { echo \"fatal: unable to access\" >&2; exit 128; }\nexec %s \"$@\"\n", rg))
	mfk := func() { m.mergeE(m.mfail(), 0) }
	mfk()
	k.check("일시 실패: 큐 그대로·merge-failed·막힘 없음, --once 75", m.mrc == 75 && m.inQ("l-tr") && !strings.Contains(m.failed(), "l-tr") &&
		len(m.ev("l-tr", "막힘")) == 0 && strings.Contains(m.merr, "일시 실패(종료 1"), m.dm())
	mfk()
	mfk()
	mfk()
	k.check("일시 실패가 3번 넘으면 돌려보냄(merge-failed·막힘)", strings.Contains(m.failed(), "l-tr") && anyHas(m.ev("l-tr", "막힘"), "일시 실패가 3번") && !m.inQ("l-tr"), m.dm())
	k.check("일시 실패 횟수 파일은 정리됨", !exists(m.mg()+"/l-tr.tr"))

	// 27. 착지 뒤 fetch가 실패해도 병합기는 멈추지 않고, 다음 바퀴에 일지로 정리한다
	m.lane("a", "l-nf", "nf.txt", "nf")
	m.land("a", "l-nf")
	m.fake(fmt.Sprintf("#!/bin/sh\n[ \"$*\" != \"fetch -q origin +refs/heads/main:refs/remotes/origin/main\" ] || { echo \"fatal: unable to access\" >&2; exit 128; }\nexec %s \"$@\"\n", rg))
	mfk()
	k.check("착지 뒤 fetch 실패: 멈추지 않고 일지를 두고 쉼(--once 75), main엔 들어감", m.mrc == 75 && m.inMain("l-nf") && exists(m.mg()+"/cur") &&
		strings.Contains(m.merr, "일지를 두고"), m.dm())
	m.merge()
	k.check("착지 뒤 fetch 실패: 다음 바퀴가 정리(다시 넣지 않음, 착지 사건 하나)", m.qrefs() == "" && strings.Count("\n"+m.mainlog()+"\n", "\nl-nf\n") == 1 &&
		len(m.ev("l-nf", "착지")) == 1, m.dm())

	// 28. 착지가 rebase 충돌(종료 3)로 끝나도 병합기는 멈추지 않는다: 검증 도중 원격 main이 충돌하게 움직이면 — 그 제출은 merge-failed,
	//     다음 제출은 들어간다, 클론에 멈춘 rebase가 남지 않는다
	k.stub(T+"/vhook.l-m1", "#!/bin/bash\nrm -f \"$T/vhook.l-m1\"\nd=$(mktemp -d \"$T/ph.XXXXXX\") && git clone -q \"$O\" \"$d/c\" && cd \"$d/c\" && printf 'zz\\n' >m3.txt && git add m3.txt && git commit -qm l-m3x && git push -q origin main\n")
	m.lane("a", "l-m1", "m3.txt", "m1")
	m.land("a", "l-m1")
	m.lane("a", "l-m2", "m4.txt", "m2")
	m.land("a", "l-m2")
	m.merge()
	k.check("rebase 충돌: 병합기 계속, l-m1은 merge-failed, l-m2는 착지", m.mrc == 0 && strings.Contains(m.failed(), "l-m1") && m.inMain("l-m2") &&
		m.inMain("l-m3x") && m.qrefs() == "", m.dm())
	k.check("rebase 충돌: 클론에 멈춘 rebase 없음", !exists(m.proj("m")+"/.git/rebase-merge") && !exists(m.proj("m")+"/.git/rebase-apply"))

	// 29. 병합기가 멈추면(die) 막힘 사건 --name merger
	touch(t, m.proj("m")+"/stray.txt")
	m.merge()
	k.check("병합기 멈춤: 종료 1, 막힘 사건 「병합기 멈춤」", m.mrc == 1 && anyHas(m.ev("merger", "막힘"), "병합기 멈춤"), m.dm())
	os.Remove(m.proj("m") + "/stray.txt")

	// 30a. 병합기 잠금은 착지·검증 자식에 물리지 않는다 — 검증이 남긴 고아가 살아 있어도 병합기가 끝나면 새 병합기가 바로 잠금을 얻는다
	m.lane("a", "l-orph", "orph.txt", "orph")
	m.land("a", "l-orph")
	k.stub(T+"/vhook.l-orph", "#!/bin/sh\nsleep 30 >/dev/null 2>&1 </dev/null &\necho $! >\"$T/orph.pid\"\nrm -f \"$T/vhook.l-orph\"\n")
	m.merge()
	m.lane("a", "l-orph2", "orph2.txt", "orph2")
	m.land("a", "l-orph2")
	m.merge()
	opid, _ := strconv.Atoi(strings.TrimSpace(read(T + "/orph.pid")))
	alive := opid > 0 && syscall.Kill(opid, 0) == nil
	k.check("고아: 검증이 남긴 배경 프로세스가 살아 있어도 다음 병합기는 잠금을 얻어 착지(75 아님)", m.inMain("l-orph") && alive && m.mrc == 0 && m.inMain("l-orph2"), m.dm())
	if opid > 0 {
		syscall.Kill(opid, syscall.SIGTERM)
	}

	// 30. SIGKILL(구조 A): 검증 중 병합기만 죽으면 잠금은 바로 놓인다. 살아 있는 착지는 끝까지 가고, 그 뒤 다음 병합기가 일지로 완료
	m.lane("a", "l-kill", "kill.txt", "kill")
	m.land("a", "l-kill")
	// 기다림 상한 3분 — 시험이 꼬여도 검증 손자가 끝없이 남지 않게
	k.stub(T+"/vhook.l-kill", "#!/bin/sh\n: >\"$T/inverify\"\ni=0; while [ ! -e \"$T/go\" ] && [ $i -lt 900 ]; do sleep 0.2; i=$((i + 1)); done\nrm -f \"$T/vhook.l-kill\"\n")
	t.Cleanup(func() { touch(t, T+"/go") })
	bg := k.mergerCmd(m.proj("m"), nil, T+"/mk.out", "--repo", m.proj("m"), "--once")
	if err := bg.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(300, 100*time.Millisecond, func() bool { return exists(T + "/inverify") })
	bg.Process.Kill()
	bg.Wait()
	locks, _ := filepath.Glob(filepath.Join(T, ".local/state/hc-lane/merger.*.lock"))
	free := len(locks) > 0
	for _, p := range locks {
		free = free && tryLock(p)
	}
	k.check("SIGKILL: 착지 자식이 살아 있어도 병합기 잠금은 바로 놓인다", exists(T+"/inverify") && !exists(T+"/go") && free, read(T+"/mk.out"))
	touch(t, T+"/go")
	waitFor(300, 100*time.Millisecond, func() bool { return m.inMain("l-kill") })
	for i := 0; i < 100; i++ {
		m.merge()
		if m.mrc != 75 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	lk := m.ev("l-kill", "착지")
	k.check("SIGKILL: 그 착지는 들어갔고 다음 병합기가 일지로 완료(검증 한 번, 착지 사건 하나 — 「앞 병합기 실행」)", m.inMain("l-kill") && m.mrc == 0 &&
		m.qrefs() == "" && cntV("verify l-kill") == 1 && len(lk) == 1 && anyHas(lk, "앞 병합기 실행"), m.dm())
	os.Remove(T + "/inverify")
	os.Remove(T + "/go")

	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// TestMergerContractC: 다른 기본 브랜치·빠른 병합기·이름 다른 제출·report 착지·새 파일·묶음 크기 1·보낼 함·옮기기 끊김(31~39),
// 아스트라 2차 지적
func TestMergerContractC(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	T, rg := k.T, k.realGit

	// 31. 다른 기본 브랜치로 낸 제출은 건드리지 않는다
	m.og("update-ref", "refs/heads/release", "main")
	m.lane("a", "l-rel", "rel.txt", "rel")
	k.sh(m.proj("a"), `git fetch -q origin`)
	m.landA("a", []string{"HC_LANE=l-rel"}, "release")
	m0, r0 := m.og("rev-parse", "main"), m.og("rev-parse", "release")
	m.merge()
	k.check("다른 기본 브랜치: 큐에 그대로, 건너뜀 안내, main·release 그대로", m.rc == 0 && m.inQ("l-rel") && strings.Contains(m.merr, "기본 브랜치 release 로 낸 제출") &&
		m.og("rev-parse", "main") == m0 && m.og("rev-parse", "release") == r0, m.dl(), m.dm())
	m.delRefs("l-rel", []string{"refs/merge-queue/", "refs/merge-queue-meta/"}, "refs/heads/lane/l-rel", "refs/heads/release")

	// 32. 접수 확인 사이에 병합기가 처리해 세 참조가 사라져도 접수로 본다(제출 기록이 refs/merge-done/<레인>에)
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \"$1 $*\" in\n  push*refs/heads/lane/l-fast*) \"%[1]s\" \"$@\"; r=$?; (cd \"%[2]s\" && PATH=\"%[3]s\" %[4]s --repo \"%[2]s\" --once >/dev/null 2>&1); exit $r ;;\nesac\nexec \"%[1]s\" \"$@\"\n",
		rg, m.proj("m"), k.env["PATH"], k.mergerSh()))
	m.lane("a", "l-fast", "fast.txt", "fast")
	m.landA("a", append(m.mfail(), "HC_LANE=l-fast"))
	k.check("빠른 병합기: 제출은 종료 0(접수), main에 들어감", m.rc == 0 && m.inMain("l-fast"), m.dl())

	// 33. 가지 이름과 레인 이름이 달라도 제출은 레인 이름으로. 병합기가 넣은 뒤에는 clean이 그 가지를 지운다(main에 듦)
	m.lane("b", "l-brx", "brx.txt", "brx")
	m.land("b", "l-bry")
	m.merge()
	k.sh(m.proj("b"), `git checkout -q --detach && git fetch -q origin`)
	c := m.K.lane(T, nil, "clean", "--repo", m.proj("b"), "--branch", "l-brx")
	k.check("clean: 레인 이름이 다른 제출도 병합기가 넣은 뒤엔 지움", m.rc == 0 && m.inMain("l-brx") && c.RC == 0 &&
		!k.gitOK(m.proj("b"), "show-ref", "-q", "--verify", "refs/heads/l-brx"), c.Out, c.Err)

	// 34. 제출만 한 커밋은 report 착지 --name으로도, 추적 참조가 없어도(push 응답 유실) 착지로 안 적는다
	m.lane("a", "l-rq", "rq.txt", "rq")
	m.land("a", "l-rq")
	srq := k.git(m.proj("a"), "rev-parse", "--short", "HEAD")
	r1 := m.K.lane(m.proj("a"), nil, "report", "착지", srq, "x", "--name", "l-rq")
	k.git(m.proj("a"), "update-ref", "-d", "refs/remotes/origin/lane/l-rq")
	r2 := m.K.lane(m.proj("a"), []string{"HC_LANE=l-rq"}, "report", "착지", srq, "x")
	k.check("report 착지: --name·추적 참조 없음도 안 적음", strings.Contains(r1.Out+r1.Err, "제출만") && strings.Contains(r2.Out+r2.Err, "제출만") &&
		len(m.ev("l-rq", "착지")) == 0, r1.Out+r1.Err, r2.Out+r2.Err)
	m.merge()

	// 35. 무시되지 않은 새 파일이 있으면 제출하지 않는다
	m.lane("a", "l-ut", "ut.txt", "ut")
	touch(t, m.proj("a")+"/stray-new.txt")
	m.land("a", "l-ut")
	k.check("새 파일: 제출 안 함", m.rc == 1 && !m.inQ("l-ut") && strings.Contains(m.errS, "새 파일"), m.dl())
	os.Remove(m.proj("a") + "/stray-new.txt")
	m.land("a", "l-ut")
	m.merge()

	// 36. HC_LANE_MERGER_BATCH=1이면 제출 둘을 따로 검증한다
	m.lane("a", "l-d1", "d1.txt", "d1")
	m.land("a", "l-d1")
	m.lane("a", "l-d2", "d2.txt", "d2")
	m.land("a", "l-d2")
	v0 := m.vcount()
	m.merge()
	k.check("묶음 크기 1: 검증 두 번", m.mrc == 0 && m.vcount()-v0 == 2 && m.qrefs() == "", m.dm())

	// 38. 보낼 함: 원격을 고친 뒤 사건을 쓰기 전에 죽어도 다음 바퀴가 보낸다
	write(t, m.mg()+"/out/1700000000-l-ob", "착지\n-\n보낼 함 시험\nl-ob\n-\n-\n-\n")
	m.merge()
	k.check("보낼 함: 남은 사건을 다음 바퀴가 보내고 지움", anyHas(m.ev("l-ob", "착지"), "보낼 함 시험") && !exists(m.mg()+"/out/1700000000-l-ob"), m.dm())

	// 39. 돌려보내기를 옮기다 원격이 끊겨도 병합기는 멈추지 않고 다른 제출을 넣는다, 다음 바퀴에 다시 옮긴다
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \"$1 $*\" in push*refs/merge-failed/*) echo \"fatal: unable to access\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.lane("a", "l-mf", "mf.txt", "mf")
	m.land("a", "l-mf")
	touch(t, T+"/vf/l-mf")
	m.lane("a", "l-mg", "mg.txt", "mg")
	m.land("a", "l-mg")
	m.mergeE(m.mfail(), 0)
	k.check("옮기기 끊김: 멈추지 않음(75), l-mg는 착지, l-mf는 큐에 남고 막힘은 아직", m.mrc == 75 && m.inMain("l-mg") && m.inQ("l-mf") && len(m.ev("l-mf", "막힘")) == 0, m.dm())
	m.merge()
	k.check("옮기기 끊김 뒤: 다음 바퀴에 merge-failed로 옮기고 막힘", strings.Contains(m.failed(), "l-mf") && len(m.ev("l-mf", "막힘")) > 0 && !m.inQ("l-mf"), m.dm())

	// ---- 아스트라 2차 지적 ----
	// 높음 5: 원격 설정(git remote get-url)을 읽지 못하면 「꺼짐」이 아니라 모름 — 착지하지 않고 75
	m.fake(fmt.Sprintf("#!/bin/sh\n[ \"$*\" != \"remote get-url origin\" ] || { echo \"fatal: bad config\" >&2; exit 128; }\nexec %s \"$@\"\n", rg))
	m.lane("a", "l-gu", "gu.txt", "gu")
	m0 = m.og("rev-parse", "main")
	m.landA("a", append(m.mfail(), "HC_LANE=l-gu"))
	k.check("원격 설정 못 읽음: 착지·제출 안 하고 75", m.rc == 75 && m.og("rev-parse", "main") == m0 &&
		k.git(m.proj("a"), "rev-parse", "main") != k.git(m.proj("a"), "rev-parse", "HEAD") && !m.inQ("l-gu") && strings.Contains(m.errS, "원격 설정"), m.dl())
	m.land("a", "l-gu")
	m.merge()
	// 중간 1: 제출 push 직후 병합기가 main push로 큐 표식을 지웠어도(lane·기록만 남음) 접수로 본다
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n  *push*--atomic*refs/heads/lane/l-acc*) \"%[1]s\" \"$@\"; r=$?; for q in $(\"%[1]s\" -C \"%[2]s\" for-each-ref --format='%%(refname)' 'refs/merge-queue/*-l-acc'); do \"%[1]s\" -C \"%[2]s\" update-ref -d \"$q\"; done; exit $r ;;\nesac\nexec \"%[1]s\" \"$@\"\n", rg, m.O))
	m.lane("a", "l-acc", "acc.txt", "acc")
	m.landA("a", append(m.mfail(), "HC_LANE=l-acc"))
	k.check("접수: 큐 표식이 먼저 지워져도 기록이 있으면 제출(종료 0)", m.rc == 0 && strings.Contains(m.out, "hc-lane land: 제출"), m.dl())
	m.delRefs("l-acc", []string{"refs/merge-queue-meta/"}, "refs/heads/lane/l-acc")
	// 중간 2: 끊긴 앞 실행을 정리할 때 후보 가지를 읽다 git이 실패하면(종료 128) 일지를 두고 쉰다
	m.lane("a", "l-rv", "rv.txt", "rv")
	m.land("a", "l-rv")
	srv := k.git(m.proj("a"), "rev-parse", "HEAD")
	var qrv string
	for _, l := range lines(m.qrefs()) {
		if strings.Contains(l, "l-rv") {
			qrv = l
		}
	}
	k.sh(m.proj("m"), `git fetch -q origin "$1" && git branch -f l-rv FETCH_HEAD`, qrv)
	write(t, m.mg()+"/cur", fmt.Sprintf("%s %s %s\n%s %s %s %s\n", "l-rv", m.og("rev-parse", "main"), srv, qrv, srv, "l-rv", "-"))
	m.fake(fmt.Sprintf("#!/bin/sh\n[ \"$*\" != \"rev-parse -q --verify refs/heads/l-rv\" ] || { echo \"fatal: bad object\" >&2; exit 128; }\nexec %s \"$@\"\n", rg))
	m.mergeE(m.mfail(), 0)
	k.check("복구 중 후보 조회 실패: 일지를 두고 쉼(75), 큐 그대로", m.mrc == 75 && exists(m.mg()+"/cur") && m.inQ("l-rv") && strings.Contains(m.merr, "후보 가지를 읽지 못했다"), m.dm())
	m.merge()
	k.check("복구 중 후보 조회 실패 뒤: 다음 바퀴가 넣음", m.inMain("l-rv") && m.qrefs() == "" && !exists(m.mg()+"/cur"), m.dm())
	// 중간 4: 돌려보낼 기록이 잘려(빈 파일) 남아도 병합기는 멈추지 않고 그 기록만 버린다
	write(t, m.mg()+"/fail/1700000001-l-zz", "")
	m.merge()
	k.check("잘린 돌려보낼 기록: 멈추지 않고(0) 버림", m.mrc == 0 && !exists(m.mg()+"/fail/1700000001-l-zz") && strings.Contains(m.merr, "깨졌다"), m.dm())
	// 중간 8: report 착지는 제출 때의 기본 브랜치로 판정한다 — release로 낸 커밋이 main에만 들었으면 아직 착지 아님
	m.og("update-ref", "refs/heads/release", "main")
	m.lane("a", "l-rb", "rb.txt", "rb")
	k.sh(m.proj("a"), `git fetch -q origin`)
	m.landA("a", []string{"HC_LANE=l-rb"}, "release")
	srb := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.og("update-ref", "refs/heads/main", srb)
	k.sh(m.proj("a"), `git fetch -q origin`)
	rr := m.K.lane(m.proj("a"), nil, "report", "착지", srb, "x", "--name", "l-rb")
	k.check("report 착지: release로 낸 제출은 main에 들어도 안 적음", m.rc == 0 && strings.Contains(rr.Out+rr.Err, "제출만") && len(m.ev("l-rb", "착지")) == 0, rr.Out, rr.Err)
	m.delRefs("l-rb", []string{"refs/merge-queue/", "refs/merge-queue-meta/"}, "refs/heads/lane/l-rb", "refs/heads/release")
	k.sh(m.proj("m"), `git fetch -q origin && git checkout -q --detach origin/main`)
	// 중간 10: 켬 표식이 다른 기계면 병합기는 멈춘다. 검증 중에 켬 표식이 사라져도 그 착지는 들어가고, 다음 바퀴부터 쉰다
	m.off()
	m.og("update-ref", "refs/herdr/merger/box2", "main")
	m.merge()
	k.check("켬 표식이 다른 기계: 병합기 멈춤(1)", m.mrc == 1 && strings.Contains(m.merr, "다른 기계"), m.dm())
	m.og("update-ref", "-d", "refs/herdr/merger/box2")
	m.on()
	k.stub(T+"/vhook.l-mk", "#!/bin/sh\nrm -f \"$T/vhook.l-mk\"\ngit -C \"$O\" update-ref -d refs/herdr/merger/box\n")
	m.lane("a", "l-mk", "mk.txt", "mk")
	m.land("a", "l-mk")
	m.lane("a", "l-mk2", "mk2.txt", "mk2")
	m.land("a", "l-mk2")
	m.merge()
	k.check("검증 중 켬 표식이 사라짐: 그 착지는 들어가고 다음 제출은 쉼(75)", m.mrc == 75 && m.inMain("l-mk") && m.inQ("l-mk2") && strings.Contains(m.merr, "꺼짐"), m.dm())
	m.on()
	m.merge()
	k.check("다시 켜면 넣음", m.inMain("l-mk2") && m.qrefs() == "", m.dm())
	// 중간 12: 제출 때의 작업 ID로 착지 표식 refs/hc-landed/<작업 ID>를 main push에 함께
	m.lane("a", "l-job", "job.txt", "job")
	m.landA("a", []string{"HC_LANE_JOB=j-777", "HC_LANE=l-job"})
	m.merge()
	k.check("작업 표식: refs/hc-landed/j-777 = 넣은 커밋", m.og("rev-parse", "-q", "--verify", "refs/hc-landed/j-777") == m.og("rev-parse", "main") &&
		m.og("log", "-1", "--format=%s", "main") == "l-job", m.dm(), m.og("for-each-ref", "refs/hc-landed/"))
	// 중간 13: 가지 이름이 못 되는 레인 이름(「-audit」)은 제출하지 않고, 큐에 들어온 그런 항목은 그것만 반송(병합기는 계속)
	m.lane("a", "l-bq", "bq.txt", "bq")
	m.land("a", "-audit")
	k.check("레인 이름 「-audit」: 제출 안 함", m.rc == 1 && !m.inQ("-audit") && strings.Contains(m.errS, "레인 이름"), m.dl())
	k.sh(m.proj("a"), `git push -q origin HEAD:refs/heads/tmp-bq`)
	m.og("update-ref", "refs/merge-queue/1000000002--audit", m.og("rev-parse", "tmp-bq"))
	m.og("update-ref", "-d", "refs/heads/tmp-bq")
	m.merge()
	k.check("큐의 「-audit」: 그 항목만 반송, 병합기 계속(0)", m.mrc == 0 && m.ogOK("rev-parse", "-q", "--verify", "refs/merge-failed/1000000002--audit") && !m.inQ("-audit"), m.dm())
	// 중간 15: 다른 기계 레인 터미널(<기계>/<터미널>)에도 알리고, 알림이 끊기면 보낼 함에 두고 다음 바퀴에 다시(사건은 한 번)
	m.lane("a", "l-far", "far.txt", "far")
	m.landA("a", []string{"HC_LANE_MACHINE=box2", "HC_TERMINAL_ID=t-far", "HC_LANE=l-far"})
	touch(t, T+"/hcfail")
	m.merge()
	os.Remove(T + "/hcfail")
	outHas := func(s string) bool {
		es, _ := os.ReadDir(m.mg() + "/out")
		for _, e := range es {
			if strings.Contains(e.Name(), s) {
				return true
			}
		}
		return false
	}
	k.check("원격 레인 알림 실패: 보낼 함에 남음, 착지 사건은 하나", m.inMain("l-far") && outHas("l-far") && len(m.ev("l-far", "착지")) == 1, m.dm())
	m.merge()
	k.check("원격 레인 알림: 다음 바퀴에 box2/t-far 로 hc send, 사건은 그대로 하나", grepE(read(T+"/hc.send"), "^box2/t-far\t") && !outHas("l-far") &&
		len(m.ev("l-far", "착지")) == 1, read(T+"/hc.send"), m.dm())
	// 내장 착지의 push 응답 유실(구조 A): main에 들고 큐 표식도 지워졌는데 실패로 돌아오면 원격이 후보를 담았는지 먼저 보고 착지로
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" push --atomic origin \"*\":refs/heads/main \"*) echo x >>\"$T/inj\"; %[1]s \"$@\"; echo \"fatal: the remote end hung up unexpectedly\" >&2; exit 128 ;; esac\nexec %[1]s \"$@\"\n", rg))
	m.lane("a", "l-lost3", "lost3.txt", "lost3")
	m.land("a", "l-lost3")
	os.Remove(T + "/inj")
	m.mergeE(m.mfail(), 0)
	// 가짜 git이 실제로 응답 유실을 한 번 넣었는지도 본다
	k.check("내장 착지 응답 유실: 주입 한 번, 착지 처리(merge-failed 아님)", countNonEmpty(read(T+"/inj")) == 1 && m.mrc == 0 && m.inMain("l-lost3") &&
		m.qrefs() == "" && !strings.Contains(m.failed(), "l-lost3") && len(m.ev("l-lost3", "착지")) > 0, m.dm())

	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// TestMergerContractD: 아스트라 3차 지적(철회된 제출·제출 ID 표식·git-path·임시 작업 트리·다시 land·큐 이름·이미 든 제출·제출 기록·
// push 끊김·알림·이름·순서·우선·작업 표식)
func TestMergerContractD(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	T, rg := k.T, k.realGit

	// ---- 아스트라 3차 지적 ----
	// 높음 4: 재시작 때 제출 ID 표식으로만 완료를 본다 — 다시 제출해 철회된 옛 제출이 같은 패치로 main에 있어도 완료로 치지 않는다
	m.lane("a", "l-wd", "wd.txt", "wd")
	m.land("a", "l-wd")
	swd := k.git(m.proj("a"), "rev-parse", "HEAD")
	var qwd string
	for _, l := range lines(m.qrefs()) {
		if strings.Contains(l, "l-wd") {
			qwd = l
		}
	}
	nwd := strings.TrimPrefix(qwd, "refs/merge-queue/")
	k.sh(m.proj("m"), `git fetch -q origin '+refs/merge-queue-meta/*:refs/hc-merger/meta/*'`)
	mwd := k.git(m.proj("m"), "rev-parse", "refs/hc-merger/meta/"+nwd)
	k.sh(m.proj("b"), `git fetch -q origin && git checkout -q -B tmp-wd origin/main && printf 'wd\n' >wd.txt && git add wd.txt && git commit -qm l-wd-same && git push -q origin HEAD:main`)
	m.og("update-ref", "-d", qwd)
	m.og("update-ref", "-d", "refs/merge-queue-meta/"+nwd) // 레인이 다시 제출해 옛 표식이 바뀐(철회된) 꼴
	write(t, m.mg()+"/cur", fmt.Sprintf("%s %s %s\n%s %s %s %s\n", "l-wd", m.og("rev-parse", "main~1"), swd, qwd, swd, "l-wd", mwd))
	m.merge()
	k.check("철회된 옛 제출: 같은 패치가 main에 있어도 착지로 안 침", m.mrc == 0 && len(m.ev("l-wd", "착지")) == 0 && !exists(m.mg()+"/cur"), m.dm())
	m.og("update-ref", "-d", "refs/heads/lane/l-wd")
	// 높음 4(정상 길): main push에 제출 ID 표식이 원자로 함께 실린다
	m.lane("a", "l-sid", "sid.txt", "sid")
	m.land("a", "l-sid")
	var sidv string
	for _, r := range rows(m.EV) {
		if col(r, 4) == "l-sid" && col(r, 5) == "제출" {
			sidv, _, _ = strings.Cut(col(r, 7), " ")
		}
	}
	m.merge()
	k.check("제출 ID 표식: refs/hc-landed/<제출 ID> = 넣은 커밋", sidv != "" && m.og("rev-parse", "-q", "--verify", "refs/hc-landed/"+sidv) == m.og("rev-parse", "main"),
		"sid="+sidv, m.og("for-each-ref", "refs/hc-landed/"))
	// 높음 5: git이 관리 경로(--git-path)를 못 알려 주면 진행 중인 rebase가 있는지 모르니 제출하지 않는다
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" rev-parse --git-path \"*) echo \"fatal: bad\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.lane("a", "l-gp", "gp.txt", "gp")
	m.landA("a", append(m.mfail(), "HC_LANE=l-gp"))
	k.check("git-path 조회 실패: 제출 안 함(1)", m.rc == 1 && !m.inQ("l-gp") && strings.Contains(m.errS, "git rev-parse --git-path 실패"), m.dl())
	m.land("a", "l-gp")
	m.merge()
	// 높음 6: 후보는 임시 작업 트리에서 — 검증하는 동안에도 병합기 클론의 작업 트리(HEAD·파일)는 그대로다. 앞 실행이 남긴 임시 작업 트리는
	// 다음 바퀴가 지운다
	hm0 := k.git(m.proj("m"), "rev-parse", "HEAD")
	k.stub(T+"/vhook.l-wt", "#!/bin/sh\nrm -f \"$T/vhook.l-wt\"\nprintf \"%s|%s\\n\" \"$(git -C \"$W/m/proj\" rev-parse HEAD)\" \"$(git -C \"$W/m/proj\" status --porcelain | wc -l | tr -d \" \")\" >\"$T/wt.seen\"\n")
	m.lane("a", "l-wt", "wt.txt", "wt")
	m.land("a", "l-wt")
	k.sh(m.proj("m"), `mkdir -p .git/hc-merger/wt && git worktree add -q --detach "$PWD/.git/hc-merger/wt/c.left" HEAD && : >.git/hc-merger/wt/c.left/half-written &&
mkdir -p "$PWD.merger-wt" && git worktree add -q --detach "$PWD.merger-wt/c-left2" HEAD`)
	m.merge()
	k.check("임시 작업 트리: 검증 중 병합기 클론 HEAD 그대로·깨끗함, 남은 작업 트리는 지움", strings.TrimSpace(read(T+"/wt.seen")) == hm0+"|0" && m.inMain("l-wt") &&
		countNonEmpty(k.git(m.proj("m"), "worktree", "list")) == 1, read(T+"/wt.seen"), k.git(m.proj("m"), "worktree", "list"), m.dm())
	// 중간 1: 같은 커밋을 다시 land하면 새 제출을 만들지 않는다 — 큐에 있으면 그대로, 넣었으면 할 것 없음
	qOf := func(s string) string {
		for _, l := range lines(m.qrefs()) {
			if strings.Contains(l, s) {
				return l
			}
		}
		return ""
	}
	m.lane("a", "l-dup", "dup.txt", "dup")
	m.land("a", "l-dup")
	qd0 := qOf("l-dup")
	m.land("a", "l-dup")
	k.check("다시 land: 이미 큐에 있으면 새로 내지 않음(큐 이름 그대로)", m.rc == 0 && qOf("l-dup") == qd0 && strings.Contains(m.out, "이미 병합기 큐에 있다"), m.dl())
	k.sh(m.proj("b"), `git fetch -q origin && git checkout -q -B tmp-dd origin/main && printf 'dd\n' >dd.txt && git add dd.txt && git commit -qm l-dd-move && git push -q origin HEAD:main`)
	m.merge()
	m.land("a", "l-dup")
	k.check("다시 land: 병합기가 rebase해 넣었으면 할 것 없음(새 제출 없음)", m.rc == 0 && !m.inQ("l-dup") && strings.Contains(m.out, "이미 main 에 넣었다"), m.dl(), m.qrefs())
	// 중간 2: 큐 이름은 반송된 이름(refs/merge-failed/)과도 겹치지 않는다
	now := time.Now().Unix()
	m.lane("a", "l-col", "col.txt", "col")
	for d := int64(0); d < 4; d++ {
		m.og("update-ref", fmt.Sprintf("refs/merge-failed/%d-l-col", now+d), "main")
	}
	m.land("a", "l-col")
	qc := qOf("l-col")
	k.check("큐 이름: 반송된 이름과 겹치지 않음", m.rc == 0 && qc != "" && !m.ogOK("rev-parse", "-q", "--verify", "refs/merge-failed/"+strings.TrimPrefix(qc, "refs/merge-queue/")), m.qrefs())
	m.merge()
	// 중간 3·4: 이미 main에 든 제출의 정리가 실패하면 --once는 75로 쉬고, 정리되면 그 제출의 작업 표식도 남긴다
	m.lane("a", "l-ear", "ear.txt", "ear")
	m.landA("a", []string{"HC_LANE_JOB=j-ear", "HC_LANE=l-ear"})
	sear := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.og("update-ref", "refs/heads/main", sear) // 병합기 밖에서 먼저 들어감
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" push \"*refs/merge-done/l-ear*) echo \"fatal: unable to access\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.mergeE(m.mfail(), 60*time.Second)
	k.check("이미 든 제출 정리 실패: --once 75(되풀이 없음), 큐 그대로", m.mrc == 75 && m.inQ("l-ear"), m.dm())
	m.merge()
	k.check("이미 든 제출 정리: 작업 표식 refs/hc-landed/j-ear = 그 제출", m.og("rev-parse", "-q", "--verify", "refs/hc-landed/j-ear") == sear && !m.inQ("l-ear"),
		m.og("for-each-ref", "refs/hc-landed/"), m.dm())
	// 중간 7: 착지 뒤 정리할 때 제출 기록을 못 읽으면(git log 실패 — 고를 때는 읽혔다) 제출 ID 없이 끝내지 않고 쉰다
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" log -1 --format=%%B \"*) [ ! -e \"$T/mmfail\" ] || { echo \"fatal: bad object\" >&2; exit 128; } ;; esac\nexec %s \"$@\"\n", rg))
	k.stub(T+"/vhook.l-mm", "#!/bin/sh\nrm -f \"$T/vhook.l-mm\"\n: >\"$T/mmfail\"\n")
	m.lane("a", "l-mm", "mm.txt", "mm")
	m.land("a", "l-mm")
	m.mergeE(m.mfail(), 0)
	k.check("제출 기록 못 읽음: 착지·반송 없이 쉼(75)", m.mrc == 75 && m.inMain("l-mm") && len(m.ev("l-mm", "착지")) == 0 && len(m.ev("l-mm", "막힘")) == 0, m.dm())
	os.Remove(T + "/mmfail")
	m.merge()
	k.check("제출 기록 읽히면: 착지 사건에 제출 ID", anyHas(m.ev("l-mm", "착지"), "제출 ID"), m.dm())
	// 중간 8: main push 전송만 끊기고(원격은 안 받음) 큐 lease가 그대로면 일시 실패 — 한 번 만에 반송하지 않는다
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" push --atomic origin \"*\":refs/heads/main \"*) [ -e \"$T/pf\" ] && { rm -f \"$T/pf\"; echo \"fatal: unable to access\" >&2; exit 128; } ;; esac\nexec %s \"$@\"\n", rg))
	m.lane("a", "l-pf", "pf.txt", "pf")
	m.land("a", "l-pf")
	touch(t, T+"/pf")
	m.mergeE(m.mfail(), 0)
	k.check("push 전송 끊김: 반송하지 않고 큐에 둠(75)", m.mrc == 75 && m.inQ("l-pf") && !strings.Contains(m.failed(), "l-pf"), m.dm())
	m.merge()
	// 중간 12: 알림은 최상위 name·state live로 본다 — 레인 이름이 다른 터미널의 foreground에만 있으면 보내지 않는다
	m.lane("a", "l-fg", "fg.txt", "fg")
	m.landA("a", []string{"HC_TERMINAL_ID=t-fg", "HC_LANE=l-fg"})
	m.merge()
	k.check("알림: foreground 이름만 맞는 터미널에는 hc send 안 함", m.inMain("l-fg") && !grepE(read(T+"/hc.send"), "^t-fg\t"), read(T+"/hc.send"))
	// 중간 13: 레인 이름 규칙은 레인을 띄울 때와 같다 — 「_audit」은 제출·착지된다
	m.lane("a", "_audit", "au.txt", "au")
	m.land("a", "_audit")
	m.merge()
	k.check("레인 이름 「_audit」: 제출·착지", m.rc == 0 && m.inMain("_audit"), m.dl(), m.dm())
	// 시험 중간 1: 먼저 낸 것이 먼저 — 이름이 사전순으로 뒤인 레인을 먼저 내도 그 순서로 넣는다
	m.lane("a", "l-zz", "zz.txt", "zz")
	m.land("a", "l-zz")
	time.Sleep(time.Second)
	m.lane("b", "l-aa", "aa.txt", "aa")
	m.land("b", "l-aa")
	m.merge()
	k.check("순서: 먼저 낸 l-zz 다음 l-aa(이름순 아님)", head(m.mainlog(), 2) == "l-aa,l-zz", head(m.mainlog(), 3))
	// 우선: 제출 기록에 priority=<HC_LANE_PRIORITY, 없으면 보통>. 우선 → 제출 시각 순
	m.lane("a", "l-pl", "pl.txt", "pl")
	m.land("a", "l-pl")
	time.Sleep(time.Second)
	m.lane("b", "l-pu", "pu.txt", "pu")
	m.landA("b", []string{"HC_LANE_PRIORITY=긴급", "HC_LANE=l-pu"})
	pmeta := func(l string) string {
		for _, q := range lines(m.qrefs()) {
			if strings.HasSuffix(q, "-"+l) {
				return m.og("log", "-1", "--format=%B", "refs/merge-queue-meta/"+strings.TrimPrefix(q, "refs/merge-queue/"))
			}
		}
		return ""
	}
	k.check("우선: 제출 기록에 priority=긴급·보통(없으면 보통)", m.rc == 0 && strings.Contains(pmeta("l-pu"), "priority=긴급") && strings.Contains(pmeta("l-pl"), "priority=보통"),
		m.dl(), pmeta("l-pu"))
	m.merge()
	k.check("우선: 늦게 낸 긴급이 먼저 착지(Go 판)", m.mrc == 0 && head(m.mainlog(), 2) == "l-pl,l-pu", head(m.mainlog(), 3), m.dm())
	// 시험 중간 3: 작업 표식은 rebase해 넣은 커밋을 가리키고, 표식 push가 거절되면(그새 다른 값) main도 그대로
	k.stub(T+"/vhook.l-jr", "#!/bin/sh\nrm -f \"$T/vhook.l-jr\"\nd=$(mktemp -d \"$T/jr.XXXXXX\") && git clone -q \"$O\" \"$d/c\" && cd \"$d/c\" && printf \"jr\\n\" >jr-other.txt && git add jr-other.txt && git commit -qm l-jr-other && git push -q origin main\n")
	m.lane("a", "l-jr", "jr.txt", "jr")
	m.landA("a", []string{"HC_LANE_JOB=j-jr", "HC_LANE=l-jr"})
	sjr := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.merge()
	jr := m.og("rev-parse", "-q", "--verify", "refs/hc-landed/j-jr")
	k.check("작업 표식: rebase해 넣은 커밋(원본 아님)", jr == m.og("rev-parse", "main") && jr != sjr && m.og("log", "-1", "--format=%s", "main") == "l-jr",
		m.og("for-each-ref", "refs/hc-landed/"), head(m.mainlog(), 3))
	m.lane("a", "l-jx", "jx.txt", "jx")
	m.landA("a", []string{"HC_LANE_JOB=j-jx", "HC_LANE=l-jx"})
	k.stub(T+"/vhook.l-jx", "#!/bin/sh\nrm -f \"$T/vhook.l-jx\"\ngit -C \"$O\" update-ref refs/hc-landed/j-jx \"$(git -C \"$O\" commit-tree \"$(git -C \"$O\" hash-object -t tree -w /dev/null)\" -m other)\"\n")
	mj0 := m.og("rev-parse", "main")
	m.mergeE(nil, 120*time.Second)
	k.check("작업 표식 push 거절: main 그대로·큐 그대로", m.og("rev-parse", "main") == mj0 && m.inQ("l-jx"), m.dm())
	m.og("update-ref", "-d", "refs/hc-landed/j-jx")
	m.merge()

	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

// TestMergerContractE: 끄기(16)·묶음·착지 스크립트(2단계)·준비 훅·이미 든 내용·격리·사전 검사
func TestMergerContractE(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	T := k.T

	// 16. 끄기: 원격 참조를 지우면 다음 land부터 예전처럼 착지
	m.off()
	m.lane("a", "l-off2", "off2.txt", "off2")
	m.land("a", "l-off2")
	k.check("끄기: 표식을 지우면 착지 그대로", m.rc == 0 && head(m.mainlog(), 1) == "l-off2" && !m.inQ("l-off2"), m.dl())

	// 묶음(최대 3): 앞 구성원 위에 차례로 얹고 → 검증 한 번 → push 한 번(CAS). 실패면 마지막 것 빼고 다시
	m.on()
	bmerge := func() { m.mergeE([]string{"HC_LANE_MERGER_BATCH=3"}, 0) }
	for _, s := range []string{"1", "2", "3"} {
		m.lane("a", "l-b"+s, "b"+s+".txt", "b"+s)
		m.land("a", "l-b"+s)
	}
	v0 := m.vcount()
	bmerge()
	k.check("묶음 3: 셋이 검증 한 번·push 한 번으로 main에(제출 순서), 큐 비고 merge-done 셋, 일지 묶음 크기 3", m.mrc == 0 && head(m.mainlog(), 3) == "l-b3,l-b2,l-b1" &&
		m.vcount() == v0+1 && m.qrefs() == "" && len(lines(m.og("for-each-ref", "refs/merge-done/l-b1", "refs/merge-done/l-b2", "refs/merge-done/l-b3"))) == 3 &&
		func() bool {
			for _, r := range rows(m.MJ) {
				if col(r, 3) == "l-b3" && col(r, 7) == "3" && col(r, 10) == "착지" {
					return true
				}
			}
			return false
		}(), m.dm())
	nb := 0
	for _, l := range []string{"l-b1", "l-b2", "l-b3"} {
		nb += countHas(m.ev(l, "착지"), "묶음 3")
	}
	k.check("묶음 3: 착지 사건 셋(「묶음 3」), 2번째부터도 레인 표식", nb == 3 && m.og("for-each-ref", "refs/hc-landed/l-b3/") != "")
	// 앞 구성원 위에 얹으면 충돌하는 구성원은 이번 묶음에서 빼고 큐에 그대로 — 맨 앞이 되면 혼자(main과 충돌이면 그때 반송)
	m.lane("a", "l-c1", "cx.txt", "one")
	m.land("a", "l-c1")
	m.lane("b", "l-c2", "cx.txt", "two")
	m.land("b", "l-c2")
	m.lane("a", "l-c3", "c3.txt", "c3")
	m.land("a", "l-c3")
	bmerge()
	k.check("묶음: 충돌하는 구성원은 빼고 나머지 둘이 검증 한 번으로, 뺀 것은 다음에 혼자 → 반송", m.mrc == 0 && m.inMain("l-c1") && m.inMain("l-c3") && !m.inMain("l-c2") &&
		strings.Contains(m.failed(), "l-c2") && m.qrefs() == "", m.dm())
	// 묶음 검증이 실패하면 마지막 것을 빼고 다시 — 실패한 제출만 혼자일 때 반송, 나머지는 들어감
	m.lane("a", "l-f1", "f1.txt", "f1")
	m.land("a", "l-f1")
	m.lane("a", "l-f2", "f2.txt", "f2")
	m.land("a", "l-f2")
	m.lane("a", "l-f3", "bad.txt", "f3")
	m.land("a", "l-f3")
	v0 = m.vcount()
	bmerge()
	k.check("묶음 실패: 셋(실패) → 둘(통과) → 셋째 혼자(실패, 반송), 검증 세 번", m.mrc == 0 && m.inMain("l-f1") && m.inMain("l-f2") && !m.inMain("l-f3") &&
		strings.Contains(m.failed(), "l-f3") && m.vcount() == v0+3 && !exists(m.mg()+"/cap") && m.qrefs() == "", fmt.Sprintf("v=%d/%d", m.vcount(), v0), m.dm())
	// 「한 묶음에 하나만」 경로(기본 브랜치의 .claude/herdr-merge-solo): 그 경로를 건드리는 구성원은 묶음에 하나만
	write(t, T+"/solo.src", "docs/frag/\n")
	opush := func(path, src string) {
		k.sh(T, `d=$(mktemp -d "$T/op.XXXXXX") && git clone -q "$O" "$d/c" && cd "$d/c" && mkdir -p "$(dirname "$1")" && cp "$2" "$1" && git add -- "$1" && git commit -qm "o-$1" && git push -q origin main`, path, src)
	}
	opush(".claude/herdr-merge-solo", T+"/solo.src")
	frag := func(c, l, f string, extra string) {
		k.sh(m.proj(c), `git fetch -q origin && git checkout -q -B "$1" origin/main && mkdir -p docs/frag && printf '%s\n' "$2" >"docs/frag/$2.md" && git add "docs/frag/$2.md" && git commit -qm "$1"`+extra, l, f)
	}
	frag("a", "l-s1", "s1", "")
	m.land("a", "l-s1")
	frag("b", "l-s2", "s2", "")
	m.land("b", "l-s2")
	m.lane("a", "l-s3", "s3.txt", "s3")
	m.land("a", "l-s3")
	v0 = m.vcount()
	bmerge()
	k.check("한 묶음에 하나만 경로: 첫 묶음은 l-s1+l-s3, l-s2는 다음에 혼자 — 검증 두 번", m.mrc == 0 && m.inMain("l-s1") && m.inMain("l-s2") && m.inMain("l-s3") &&
		m.vcount() == v0+2 && head(m.mainlog(), 1) == "l-s2", fmt.Sprintf("v=%d/%d", m.vcount(), v0), head(m.mainlog(), 5), m.dm())
	// 그 경로를 더했다 지운 제출(앞서 접은 커밋)도 건드린 것으로 본다
	frag("a", "l-s4", "s4", ` && git rm -q docs/frag/s4.md && printf 's4\n' >s4.txt && git add s4.txt && git commit -qm l-s4-fold`)
	m.land("a", "l-s4")
	frag("b", "l-s5", "s5", "")
	m.land("b", "l-s5")
	v0 = m.vcount()
	bmerge()
	k.check("한 묶음에 하나만 경로: 더했다 지운(접은) 제출도 건드림 — l-s4·l-s5 따로, 검증 두 번", m.mrc == 0 && m.inMain("l-s4-fold") && m.inMain("l-s5") && m.vcount() == v0+2,
		fmt.Sprintf("v=%d/%d", m.vcount(), v0), head(m.mainlog(), 5), m.dm())
	m.off()

	// 2단계: 전용 착지 스크립트(.claude/herdr-land.sh)가 있는 저장소도 받는다 — 착지는 그 스크립트의 기본 브랜치 판.
	write(t, T+"/land-main.sh", `#!/bin/bash
echo "LAND_MAIN $(git rev-parse --abbrev-ref HEAD)" >>"$T/landscript.log"
git fetch -q origin && git rebase -q "origin/$1" || exit 3
pa=("HEAD:refs/heads/$1"); for e in ${HERDR_LAND_PUSH_EXTRA-}; do pa+=("$e"); done
HERDR_LAND=1 git push -q --atomic origin "${pa[@]}"
`)
	write(t, T+"/land-lane.sh", "#!/bin/bash\necho \"LAND_LANE $(git rev-parse --abbrev-ref HEAD)\" >>\"$T/landscript.log\"\n")
	m.on()
	// 기본 브랜치에 없는 착지 스크립트를 제출이 새로 들이면 반송 — 그 스크립트는 돌지 않는다
	addLandScript := func(l string) {
		k.sh(m.proj("a"), `git fetch -q origin && git checkout -q -B "$1" origin/main && cp "$T/land-lane.sh" .claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm "$1"`, l)
	}
	addLandScript("l-add")
	m.land("a", "l-add")
	m.merge()
	k.check("기본 브랜치에 없는 착지 스크립트를 들이는 제출: 반송, 그 스크립트는 안 돎", m.mrc == 0 && strings.Contains(m.failed(), "l-add") && !m.inMain("l-add") &&
		!strings.Contains(read(T+"/landscript.log"), "LAND_LANE") && anyHas(m.ev("l-add", "막힘"), "들이는 제출"), m.dm())
	// 검증하는 사이 기본 브랜치에 착지 스크립트가 생기면 그 바퀴는 일시 실패(종료 7 — 반송 아님), 다음 바퀴가 기본 브랜치 판으로 넣는다
	k.stub(T+"/vhook.l-nh", "#!/bin/sh\nrm -f \"$T/vhook.l-nh\"\nd=$(mktemp -d \"$T/nh.XXXXXX\") && git clone -q \"$O\" \"$d/c\" && cd \"$d/c\" && cp \"$T/land-main.sh\" .claude/herdr-land.sh && git add .claude/herdr-land.sh && git commit -qm o-land && git push -q origin main\n")
	m.lane("a", "l-nh", "nh.txt", "nh")
	m.land("a", "l-nh")
	m.merge()
	k.check("검증 중 기본 브랜치에 생긴 착지 스크립트: 그 바퀴는 일시 실패(75), 큐에 그대로, 반송 아님", m.mrc == 75 && m.inQ("l-nh") && !strings.Contains(m.failed(), "l-nh") && !m.inMain("l-nh"), m.dm())
	m.merge()
	k.check("다음 바퀴: 기본 브랜치 판 착지 스크립트로 넣음(큐 lease·제출 ID 표식 함께)", m.mrc == 0 && m.inMain("l-nh") && hasLine(read(T+"/landscript.log"), "LAND_MAIN l-nh") &&
		m.qrefs() == "" && m.ogOK("rev-parse", "-q", "--verify", "refs/merge-done/l-nh") && m.og("for-each-ref", "refs/hc-landed/") != "", m.dm())
	// 레인이 착지 스크립트를 고친 제출: 기본 브랜치 판으로 넣는다(제출 트리 속 판은 안 돎), 고친 판은 내용으로 들어간다
	write(t, T+"/landscript.log", "")
	addLandScript("l-hs")
	m.land("a", "l-hs")
	m.merge()
	ls := read(T + "/landscript.log")
	k.check("착지 스크립트를 고친 제출: 기본 브랜치 판으로 착지, 레인 판은 안 돎", m.mrc == 0 && m.inMain("l-hs") && hasLine(ls, "LAND_MAIN l-hs") && !strings.Contains(ls, "LAND_LANE") &&
		m.qrefs() == "", m.dm(), ls)
	hs := lines(m.og("for-each-ref", "--format=%(refname) %(objectname)", "refs/hc-landed/l-hs/"))
	k.check("착지 스크립트 길도 착지 뒤 레인 표식 refs/hc-landed/<레인>/<초> = main에 든 커밋(fill-dup-guard)", len(hs) > 0 &&
		m.ogOK("merge-base", "--is-ancestor", hs[0][strings.Index(hs[0], " ")+1:], "main"), m.og("for-each-ref", "refs/hc-landed/"))
	opush(".claude/herdr-land.sh", T+"/land-main.sh") // 다음 시험을 위해 기본 브랜치 판으로 되돌림

	// 새 임시 작업 트리마다 준비 훅(.claude/herdr-setup.sh)을 먼저 — 메인 폴더(LANE_REPO)는 병합기 클론, 코드 색인 도구는 PATH에서 빠짐.
	// 실패하면 일시 실패(큐에 그대로)
	write(t, T+"/setup.sh", "#!/bin/bash\necho \"$LANE_REPO $LANE_BASE cbm=$(command -v codebase-memory-mcp || echo none)\" >>\"$T/setup.log\"\nprintf \"%s\\n\" \"$PWD\" >\"$T/setup.pwd\"\n[ ! -e \"$T/setupfail\" ]\n")
	opush(".claude/herdr-setup.sh", T+"/setup.sh")
	k.stub(T+"/cbm/codebase-memory-mcp", "#!/bin/sh\nexit 0\n")
	touch(t, T+"/setupfail")
	m.lane("a", "l-su", "su.txt", "su")
	m.land("a", "l-su")
	m.mergeE([]string{"PATH=" + T + "/cbm:" + k.env["PATH"]}, 0)
	mtop := m.proj("m")
	k.check("준비 훅 실패: 일시 실패(75), 큐에 그대로, 착지 안 함", m.mrc == 75 && m.inQ("l-su") && !m.inMain("l-su") && grepE(read(T+"/setup.log"), "^"+regexp.QuoteMeta(mtop)+" main "),
		m.dm(), read(T+"/setup.log"))
	// 후보 작업 트리는 클론 옆 <클론>.merger-wt/ 아래 — 경로에 .git 마디·점으로 시작하는 마디 없음(Vite fs.deny 403)
	sp := strings.TrimSpace(read(T + "/setup.pwd"))
	sl := lines(read(T + "/setup.log"))
	k.check("후보 작업 트리: 클론 옆 <클론>.merger-wt/ 아래 — 경로에 .git 마디·점으로 시작하는 마디 없음, 준비 훅에 색인 도구 안 보임",
		strings.HasPrefix(sp, mtop+".merger-wt/") && !strings.Contains("/"+strings.TrimPrefix(sp, filepath.Dir(mtop)+"/"), "/.") &&
			len(sl) > 0 && strings.Contains(sl[len(sl)-1], "cbm=none"), sp, read(T+"/setup.log"))
	os.Remove(T + "/setupfail")
	m.merge()
	k.check("준비 훅 성공: 착지", m.mrc == 0 && m.inMain("l-su") && m.qrefs() == "", m.dm())

	// 제출 내용이 이미 기본 브랜치에 있으면(같은 변경이 다른 커밋으로 먼저 들어감) 준비·검증 없이 정리만
	m.lane("a", "l-dup", "same-dup.txt", "samedup")
	m.land("a", "l-dup")
	write(t, T+"/dup.src", "samedup\n")
	opush("same-dup.txt", T+"/dup.src")
	write(t, T+"/setup.log", "")
	v0 = m.vcount()
	m.merge()
	k.check("이미 든 내용의 제출: 검증·준비 없이 큐 정리, merge-done, 착지 사건 「이미」", m.mrc == 0 && m.qrefs() == "" && m.ogOK("rev-parse", "-q", "--verify", "refs/merge-done/l-dup") &&
		m.vcount() == v0 && read(T+"/setup.log") == "" && anyHas(m.ev("l-dup", "착지"), "합친 트리가 같음"), m.dm())

	// 격리: 바깥 병합기 잠금을 fd 19로 물려받아 쥔 채 돌려도 병합기는 시험 HOME 아래 자기 잠금을 쓰고 끝난다
	m.off()
	m.og("update-ref", "refs/herdr/merger/box", "main")
	_, relOuter := holdLock(t, T+"/outer.lock")
	m.lane("a", "l-iso", "iso.txt", "iso")
	m.land("a", "l-iso")
	f19, err := os.OpenFile(T+"/outer.lock", os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	k.extraFiles19 = f19
	m.mergeE(nil, 120*time.Second)
	k.extraFiles19 = nil
	f19.Close()
	relOuter()
	sl2, _ := filepath.Glob(T + "/.local/state/hc-lane/merger.*.lock")
	k.check("격리: 바깥 잠금 fd 19를 쥔 채여도 병합기는 시험 HOME 아래 자기 잠금으로 큐를 처리(잠금에 안 막힘)", m.mrc == 0 && m.qrefs() == "" && !strings.Contains(m.merr, "아직 돈다") && len(sl2) > 0, m.dm())
	// 셸 시험 틀(isolate.sh)의 격리를 Go 시험 틀(clitest.Isolate)이 지키는지: T 밖 저장소를 못 찾고, 자식에 fd 19가 물리지 않는다
	k.check("격리: 시험은 T 밖 저장소를 못 찾는다(GIT_CEILING_DIRECTORIES), 물려받은 fd 19는 닫힘", !k.gitOK(T, "rev-parse", "--git-dir") &&
		k.shr(T, nil, `{ : >&19; } 2>/dev/null`).RC != 0)

	// 제출 전 싼 사전 검사: 저장소의 .claude/herdr-precheck.sh가 실패하면 큐에 넣지 않고 그 출력과 함께 종료 1, 통과하면 제출
	m.lane("a", "l-pc", "pc.txt", "pc")
	k.sh(m.proj("a"), `printf 'echo precheck-ran; [ ! -e "$T/pcfail" ] || { echo precheck-no; exit 3; }\n' >.claude/herdr-precheck.sh && git add -- .claude/herdr-precheck.sh && git commit -qm pc -- .claude/herdr-precheck.sh`)
	touch(t, T+"/pcfail")
	m.land("a", "l-pc")
	k.check("사전 검사: 실패면 제출 안 함(큐·lane 가지 없음), 출력과 까닭, 종료 1", m.rc == 1 && m.qrefs() == "" && !m.ogOK("rev-parse", "-q", "--verify", "refs/heads/lane/l-pc") &&
		hasLine(m.out+"\n"+m.errS, "precheck-no") && strings.Contains(m.errS, "사전 검사"), m.dl())
	os.Remove(T + "/pcfail")
	m.land("a", "l-pc")
	k.check("사전 검사: 통과면 제출", m.rc == 0 && hasLine(m.out, "precheck-ran") && grepE(m.qrefs(), `^refs/merge-queue/[0-9]{10}-l-pc$`), m.dl())

	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}
