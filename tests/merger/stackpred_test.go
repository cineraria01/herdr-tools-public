package merger_test

// 선행이 이미 착지한 쌓인 레인(docs/hc-fill/stack.md 「쌓임이 끝나는 때」): 레인이 선행 제출 P 위에 쌓여 떴고, 선행이 기본 가지에
// 다른 해시로 들어간 뒤 레인이 자기 커밋만 기본 가지 위로 다시 맞췄다(rebase --onto origin/main P). 옛 판은 「P가 조상이 아니다」로
// 착지를 거절했다(종료 2). 이제 「지금도 쌓인 레인인가」 한 물음이 — 선행 레인 표식 refs/hc-landed/<선행>/<초>의 착지가 P를 만든
// 뒤의 것이면 — 쌓임을 풀어 오늘의 안 쌓인 길(갈래점 merge-base·충돌 판정·제출 기록에 stack= 없음)로 낸다

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

const unstackNote = "선행 l-a 이 이미 착지해 쌓임을 풀고 기본 가지 기준으로 낸다"

// stackOn <레인> <P> <파일> <내용>: 클론 b에서 P 위 새 가지에 커밋 하나(쌓아 띄움 — 작업 트리 시작이 P)
func (m *M) stackOn(name, p, file, val string) {
	m.t.Helper()
	m.sh(m.proj("b"), `git fetch -q origin && git checkout -q -B "$1" "$2" && printf '%s\n' "$4" >"$3" && git add -- "$3" && git commit -qm "$1" -- "$3"`, name, p, file, val)
}

// ontoMain <레인> <P>: 그 가지의 자기 커밋만 origin/main 위로(사람 리뷰대로 선행 커밋을 뺌)
func (m *M) ontoMain(name, p string) {
	m.t.Helper()
	m.sh(m.proj("b"), `git fetch -q origin && git checkout -q "$1" && git rebase -q --onto origin/main "$2"`, name, p)
}

// mainUp <파일> <내용>: 기본 가지에 커밋 하나(선행이 다른 해시로 들게)
func (m *M) mainUp(file, val string) {
	m.t.Helper()
	m.sh(m.proj("m"), `git fetch -q origin && git checkout -q -B up origin/main && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "up $1" -- "$1" &&
git -c core.hooksPath=/dev/null push -q origin HEAD:main && git checkout -q --detach`, file, val)
}

// qmeta <레인>: 큐에 선 그 레인 제출 기록
func (m *M) qmeta(lane string) string {
	for _, r := range lines(m.qrefs()) {
		if strings.HasSuffix(r, "-"+lane) {
			return m.og("log", "-1", "--format=%B", strings.Replace(r, "refs/merge-queue/", "refs/merge-queue-meta/", 1))
		}
	}
	return ""
}

func TestStackPredLandedOtherHash(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	m.lane("a", "l-a", "a.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("선행 l-a 제출", m.rc == 0 && m.inQ("-l-a"), m.dl())
	st := "HC_LANE_STACK=l-a@" + p
	m.stackOn("l-b", p, "b.txt", "b")
	m.stackOn("l-c", p, "c.txt", "c")
	m.stackOn("l-d", p, "d.txt", "d")

	// ② 선행이 아직 안 들었는데 P를 뺐다 → 지금처럼 거절(까닭과 할 일)
	m.stackOn("l-x", p, "x2.txt", "x")
	m.ontoMain("l-x", p)
	m.land("b", "l-x", st)
	k.check("선행 미착지 + P 조상 아님: 거절(종료 2)·제출 없음·할 일", m.rc == 2 && !m.inQ("-l-x") &&
		strings.Contains(m.errS, "선행 l-a 이 아직 착지하지 않았다") && strings.Contains(m.errS, "선행 착지를 기다린다"), m.dl())

	// 선행이 다른 해시로 든다(기본 가지가 그새 움직임)
	m.mainUp("x.txt", "m")
	m.merge()
	la := m.og("log", "-1", "--format=%H", "--grep=^l-a$", "main")
	k.check("선행 l-a 가 다른 해시로 듦", la != "" && la != p && !m.ogOK("merge-base", "--is-ancestor", p, "main"), m.dm(), m.mainlog())
	marks := m.og("for-each-ref", "--format=%(objectname) %(refname)", "refs/hc-landed/l-a/")
	k.check("선행 레인 표식 refs/hc-landed/l-a/<초> 가 그 착지 커밋", strings.HasPrefix(marks, la+" refs/hc-landed/l-a/"), marks)

	// ① 「일어난 일」: 자기 커밋만 기본 가지 위로 다시 맞춘 뒤 land → 제출, stack= 없음, fork = merge-base, 알림 한 줄
	m.ontoMain("l-b", p)
	sb := k.git(m.proj("b"), "rev-parse", "HEAD")
	m.land("b", "l-b", st)
	mb := m.qmeta("l-b")
	k.check("선행이 다른 해시로 착지 + 기본 위로 다시 맞춤: 제출(종료 0)", m.rc == 0 && m.inQ("-l-b"), m.dl())
	k.check("제출 기록에 stack= 칸 없음 · fork = 기본 가지와의 갈래점", mb != "" && !strings.Contains(mb, "stack=") &&
		strings.Contains(mb, " fork="+m.og("merge-base", sb, "main")+" "), mb)
	all := m.out + m.errS
	k.check("화면 알림 한 줄(겹치지 않음)", strings.Count(all, unstackNote) == 1, m.dl())
	t.Logf("고친 뒤 l-b land 출력:\n%s\n%s", m.out, m.errS)

	// 두 번째 실행: 같은 land 다시 → 같은 답(이미 큐에), 알림 겹치지 않음
	m.land("b", "l-b", st)
	k.check("같은 land 두 번째: 이미 큐에 — 새로 내지 않음, 알림 0~1줄", m.rc == 0 && strings.Contains(m.out, "이미 병합기 큐에 있다") &&
		strings.Count(m.out+m.errS, unstackNote) <= 1, m.dl())

	// ⑦ 충돌 판정은 안 쌓인 길로 돈다(건너뛰지 않음): 기본 가지가 c.txt를 다르게 만든 뒤 → 제출 때 충돌로 거절(종료 3)
	m.ontoMain("l-c", p)
	m.mainUp("c.txt", "main")
	m.land("b", "l-c", st)
	k.check("쌓임 푼 제출도 기본 가지와의 충돌 판정을 함(종료 3, 큐에 안 넣음)", m.rc == 3 && !m.inQ("-l-c"), m.dl())

	// ③ P가 조상(다시 안 맞춤) + 선행 착지 → 지금 동작 그대로(쌓인 제출 stack=, fork=P)
	m.sh(m.proj("b"), `git checkout -q l-d`)
	m.land("b", "l-d", st)
	md := m.qmeta("l-d")
	k.check("P 조상 + 선행 착지: 지금처럼 쌓인 제출(stack=·fork=P), 알림 없음", m.rc == 0 && strings.Contains(md, " stack=l-a@"+p) &&
		strings.Contains(md, " fork="+p+" ") && !strings.Contains(m.out+m.errS, unstackNote), m.dl(), md)

	// 병합기가 둘 다 넣는다(l-b는 안 쌓인 제출, l-d는 쌓인 제출 — 선행 듦)
	for i := 0; i < 3 && m.qrefs() != ""; i++ {
		m.merge()
	}
	k.check("병합기: l-b·l-d 듦", m.inMain("l-b") && m.inMain("l-d") && m.qrefs() == "", m.dm(), m.mainlog())
}

// ⑧ 선행이 문서 접기로 다른 patch로 착지(조각 파일이 빠진 꼴 — 착지 push와 레인 표식을 손으로) + 레인이 plain `git rebase origin/main`
// → git이 선행 옛 커밋을 건너뛰지 않고 다시 얹는다(접혀 지워진 조각만 남은 커밋) → 쌓임을 풀지 않고 거절(감사 atp88r8 중간 1).
// 글의 할 일(다시 얹힌 가장 위 커밋 기준 --onto)을 그대로 따르면 다음 land는 제출된다(감사 atp89wu 중간 1 — P 기준 --onto로는 안 빠졌다)
func TestStackPredLandedReplayed(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	m.sh(m.proj("a"), `git fetch -q origin && git checkout -q -B l-a origin/main && printf 'a\n' >a.txt && printf 'frag\n' >frag-a.md && git add -- a.txt frag-a.md && git commit -qm l-a -- a.txt frag-a.md`)
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("선행 l-a 제출", m.rc == 0 && m.inQ("-l-a"), m.dl())
	st := "HC_LANE_STACK=l-a@" + p
	m.stackOn("l-y", p, "y.txt", "y")
	// 접힌 착지: a.txt만 든 커밋이 main에, 레인 표식 refs/hc-landed/l-a/<초>가 그 커밋을(병합기 done과 같은 꼴), 큐는 비움
	m.sh(m.proj("m"), `git fetch -q origin && git checkout -q -B up origin/main && printf 'a\n' >a.txt && printf 'l-a 접음\n' >CHANGELOG && git add -- a.txt CHANGELOG &&
git commit -qm l-a -- a.txt CHANGELOG && git -c core.hooksPath=/dev/null push -q origin HEAD:main && git push -q origin "HEAD:refs/hc-landed/l-a/$(date +%s)" && git checkout -q --detach`)
	m.delRefs("-l-a", []string{"refs/merge-queue/", "refs/merge-queue-meta/"}, "refs/heads/lane/l-a")
	m.sh(m.proj("b"), `git fetch -q origin && git checkout -q l-y && git rebase -q origin/main`)
	rp := k.git(m.proj("b"), "rev-parse", "HEAD~1")
	k.check("plain rebase가 선행 옛 커밋을 다시 얹음(조각 파일만)", k.git(m.proj("b"), "log", "-1", "--format=%s", rp) == "l-a" &&
		k.git(m.proj("b"), "diff", "--name-only", "origin/main", "HEAD") == "frag-a.md\ny.txt", k.git(m.proj("b"), "log", "--oneline", "origin/main..HEAD"))
	m.land("b", "l-y", st)
	k.check("선행 착지 + 선행 옛 커밋이 다시 얹힘: 거절(종료 2)·제출 없음·할 일(다시 얹힌 커밋 기준 --onto)", m.rc == 2 && !m.inQ("-l-y") &&
		strings.Contains(m.errS, "선행 l-a 의 커밋이 이 범위에 다시 얹혔다") && strings.Contains(m.errS, "git rebase --onto origin/main "+rp[:8]), m.dl())
	// 글의 할 일을 그대로
	m.sh(m.proj("b"), `git rebase -q --onto origin/main "$1"`, rp)
	m.land("b", "l-y", st)
	k.check("글대로 다시 맞춘 뒤 같은 줄: 제출·stack= 없음·조각 파일 없음", m.rc == 0 && m.inQ("-l-y") && !strings.Contains(m.qmeta("l-y"), "stack=") &&
		k.git(m.proj("b"), "diff", "--name-only", "origin/main", "HEAD") == "y.txt", m.dl())
}

// 바퀴 순서 반대(선행 착지 표식이 아직 원격에 안 보이는 찰나) → 거절, 표식이 보이면 다시 부르면 됨 · 원격 못 읽음 → 모름(75)
func TestStackPredLandedWindowAndUnknown(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	m.lane("a", "l-a", "a.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	st := "HC_LANE_STACK=l-a@" + p
	m.stackOn("l-b", p, "b.txt", "b")
	m.mainUp("x.txt", "m")
	m.merge()
	k.check("선행 l-a 듦", m.inMain("l-a"), m.dm())
	m.ontoMain("l-b", p)
	mk := strings.Fields(m.og("for-each-ref", "--format=%(objectname) %(refname)", "refs/hc-landed/l-a/"))
	k.check("표식 하나", len(mk) == 2, strings.Join(mk, " "))
	m.og("update-ref", "-d", mk[1])
	m.land("b", "l-b", st)
	k.check("표식이 아직 안 보이는 찰나: 거절(종료 2), 제출 없음", m.rc == 2 && !m.inQ("-l-b"), m.dl())
	m.og("update-ref", mk[1], mk[0])

	// ④ 원격 못 읽음 → 모름 규칙(75, 같은 줄을 다시) — 「들었다」로 읽지 않는다
	rg, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	m.fake(fmt.Sprintf("#!/bin/sh\ncase \" $* \" in *\" ls-remote \"*hc-landed*) echo \"fatal: unable to access\" >&2; exit 128 ;; esac\nexec %s \"$@\"\n", rg))
	m.landA("b", append(m.mfail(), "HC_LANE=l-b", st))
	k.check("선행 착지 조회 실패: 모름(75)·제출 없음", m.rc == 75 && !m.inQ("-l-b") && strings.Contains(m.errS, "같은 줄을 다시"), m.dl())

	m.land("b", "l-b", st)
	k.check("표식이 보인 뒤 같은 줄을 다시: 제출·stack= 없음", m.rc == 0 && m.inQ("-l-b") && !strings.Contains(m.qmeta("l-b"), "stack="), m.dl())
}

// ⑤ 같은 선행 이름의 옛 착지만 있음(이번 제출은 미착지) → 거절 · ⑥ 선행이 고쳐 다시 제출돼 새 커밋으로 착지 → ①과 같음
func TestStackPredLandedOldAndResubmit(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	// 옛 착지: l-a가 한 번 든다
	m.lane("a", "l-a", "old.txt", "old")
	m.land("a", "l-a")
	m.merge()
	k.check("옛 l-a 듦·표식 있음", m.inMain("l-a") && m.og("for-each-ref", "refs/hc-landed/l-a/") != "", m.dm())
	// 같은 이름이 다시 대기열에 — 이번 제출 P
	m.lane("a", "l-a", "a.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	st := "HC_LANE_STACK=l-a@" + p
	m.stackOn("l-b", p, "b.txt", "b")
	m.ontoMain("l-b", p)
	m.land("b", "l-b", st)
	k.check("옛 착지 표식만 있음: 거절(종료 2)", m.rc == 2 && !m.inQ("-l-b") && strings.Contains(m.errS, "아직 착지하지 않았다"), m.dl())

	// ⑥ 선행이 고쳐 새 커밋 P'로 다시 제출하고 그것이 든다
	m.sh(m.proj("a"), `printf 'a2\n' >a.txt && git commit -qm l-a --amend -- a.txt`)
	p2 := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("선행 다시 제출(새 커밋)", m.rc == 0 && p2 != p, m.dl())
	m.merge()
	k.check("선행 새 커밋 듦", m.qrefs() == "" && m.og("log", "-1", "--format=%s", "main") == "l-a" && m.og("diff", "main~1", "main", "--", "a.txt") != "" &&
		strings.Contains(m.og("show", "main:a.txt"), "a2"), m.dm(), m.mainlog())
	m.ontoMain("l-b", p)
	m.land("b", "l-b", st)
	k.check("선행이 고쳐 새 커밋으로 착지: 제출·stack= 없음·알림", m.rc == 0 && m.inQ("-l-b") && !strings.Contains(m.qmeta("l-b"), "stack=") &&
		strings.Count(m.out+m.errS, unstackNote) == 1, m.dl())
}

// 선행이 같은 해시로 착지 + 레인이 기본 가지 위로 다시 맞춤 → P가 조상이라 지금 길 그대로(쌓인 제출, 충돌 판정 함) → 듦
func TestStackPredLandedSameHash(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	m.lane("a", "l-a", "a.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	m.stackOn("l-b", p, "b.txt", "b")
	m.merge()
	k.check("선행 같은 해시로 듦", m.ogOK("merge-base", "--is-ancestor", p, "main"), m.dm(), m.mainlog())
	m.ontoMain("l-b", p)
	m.land("b", "l-b", "HC_LANE_STACK=l-a@"+p)
	k.check("같은 해시 착지 + 다시 맞춤: 제출(종료 0), 알림 없음", m.rc == 0 && m.inQ("-l-b") && !strings.Contains(m.out+m.errS, unstackNote), m.dl())
	m.merge()
	k.check("병합기: l-b 듦", m.inMain("l-b") && m.qrefs() == "", m.dm(), m.mainlog())
}
