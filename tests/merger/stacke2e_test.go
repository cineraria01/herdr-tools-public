package merger_test

// 쌓아 띄움 처음부터 끝까지(docs/hc-fill/stack.md): 레인 l-a가 제출하고, l-a의 제출 커밋 P 위에서 띄운 레인 l-b(HC_LANE_STACK=l-a@P)가
// 긴급으로 제출한다 — 지은 hc-lane이 제출 기록에 fork=P·stack=l-a@P를 싣고, 병합기는 긴급인 l-b를 앞세우지 않고 l-a → l-b 한 묶음으로 넣는다

import (
	"strings"
	"testing"
)

func TestStackE2ESubmitAndLand(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	k.set("HC_LANE_MERGER_BATCH=3")
	m.lane("a", "l-a", "a.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("선행 l-a 제출", m.rc == 0 && m.inQ("-l-a"), m.dl())

	k.sh(m.proj("b"), `git fetch -q origin refs/heads/lane/l-a && git checkout -q -B l-b FETCH_HEAD && printf 'b\n' >b.txt && git add b.txt && git commit -qm l-b -- b.txt`)
	m.land("b", "l-b", "HC_LANE_STACK=l-a@"+p, "HC_LANE_PRIORITY=긴급")
	k.check("쌓인 l-b 제출", m.rc == 0 && m.inQ("-l-b"), m.dl())
	var qb string
	for _, r := range lines(m.qrefs()) {
		if strings.HasSuffix(r, "-l-b") {
			qb = r
		}
	}
	mb := m.og("log", "-1", "--format=%B", strings.Replace(qb, "refs/merge-queue/", "refs/merge-queue-meta/", 1))
	k.check("제출 기록: fork=P · stack=l-a@P · 긴급", strings.Contains(mb, " fork="+p+" ") && strings.Contains(mb, " stack=l-a@"+p) &&
		strings.Contains(mb, "priority=긴급"), mb)

	m.merge()
	// main 로그만으로는 못 가린다(뒤가 선행 커밋을 싣고 먼저 들어도 같은 로그) — 묶음 구성원 차례를 본다(감사 atp4phf 낮음 1)
	k.check("병합기: l-a → l-b 차례로 한 번에 듦(긴급인 뒤가 앞서지 않음)", head(m.mainlog(), 3) == "l-b,l-a,base" && m.qrefs() == "" &&
		strings.Contains(m.mout, "착지 시작 — l-a+l-b ") && !strings.Contains(m.mout, "착지 시작 — l-b"), m.dm(), m.mainlog())
	j := read(m.MJ)
	k.check("병합기 일지: 쌓인 구성원 「쌓임 l-a」", strings.Contains(j, "l-b(쌓임 l-a)") || strings.Contains(j, "쌓임 l-a"), j)
	t.Logf("main:\n%s\n병합기 일지:\n%s\n병합기 출력:\n%s", m.mainlog(), j, m.mout)
}

// 선행이 기본 가지에 없는 동안 쌓인 제출은 제출 때 충돌 판정을 하지 않는다(지휘 t3yx 2026-11-27 안 A — 선행 없이 견주는 판정은
// 두 번 틀렸다: 선행 커밋까지 견줌·선행 변경을 기본 가지의 되돌림으로 읽음). 충돌은 병합기가 후보를 지을 때(--onto <앞 후보> P)
// 검증을 맡기기 전에 보고 그 제출만 돌려보낸다 — 선행·같은 묶음의 다른 구성원은 그대로 든다
func TestStackE2EConflictAtMerger(t *testing.T) {
	m := newM(t)
	k := m.K
	m.on()
	k.set("HC_LANE_MERGER_BATCH=3")
	m.lane("a", "l-a", "shared.txt", "a")
	p := k.git(m.proj("a"), "rev-parse", "HEAD")
	m.land("a", "l-a")
	k.check("선행 l-a 제출", m.rc == 0 && m.inQ("-l-a"), m.dl())
	// 그 뒤 기본 가지가 다른 파일 x.txt만 만듦(선행 파일 shared.txt는 안 건드림)
	k.sh(m.proj("m"), `git fetch -q origin && git checkout -q -B up origin/main && printf 'm\n' >x.txt && git add -A && git commit -qm up && git -c core.hooksPath=/dev/null push -q origin HEAD:main && git checkout -q --detach`)
	stack := func(name, file, val string) {
		k.sh(m.proj("b"), `git fetch -q origin refs/heads/lane/l-a && git checkout -q -B "$1" FETCH_HEAD && printf '%s\n' "$3" >"$2" && git add -- "$2" && git commit -qm "$1" -- "$2"`, name, file, val)
		m.land("b", name, "HC_LANE_STACK=l-a@"+p)
	}
	stack("l-b", "shared.txt", "b")
	k.check("뒤가 선행 파일을 이어 고침(기본 가지는 다른 파일만): 쌓인 제출은 받음", m.rc == 0 && m.inQ("-l-b"), m.dl())
	stack("l-c", "x.txt", "c")
	k.check("자기 커밋이 기본 가지와 부딪혀도 선행이 아직이면 제출은 받음(판정은 병합기 차례)", m.rc == 0 && m.inQ("-l-c"), m.dl())

	for i := 0; i < 3 && m.qrefs() != ""; i++ {
		m.merge()
	}
	k.check("병합기: l-a·l-b 듦, l-c만 돌려보냄", head(m.mainlog(), 3) == "l-b,l-a,up" && m.qrefs() == "" &&
		strings.Contains(m.failed(), "-l-c") && !strings.Contains(m.failed(), "-l-a") && !strings.Contains(m.failed(), "-l-b"), m.dm(), m.mainlog(), m.failed())
	k.check("l-c는 검증을 맡기기 전에 돌려보냄", m.cntV("verify l-c") == 0, read(m.verify))
	bl := strings.Join(m.ev("l-c", "막힘"), "\n")
	k.check("반송 사건: 「쌓인 제출 — 선행 l-a 위에 자기 커밋을 얹다 충돌: x.txt」·다시 맞추는 법", strings.Contains(bl, "쌓인 제출 — 선행 l-a 위에 자기 커밋을 얹다 충돌: x.txt") &&
		strings.Contains(bl, "git rebase --onto origin/main "+p[:8]), bl)
	k.check("선행·같은 묶음의 다른 구성원은 반송 사건 없음", len(m.ev("l-a", "막힘")) == 0 && len(m.ev("l-b", "막힘")) == 0, read(m.EV))
}
