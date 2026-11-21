package hcfill_test

// 셸 시험 10f(먼저 띄움)·10g(놀고 있음 알림).

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func init() { reg(41, "d2", secD2) }

var (
	dUntil = regexp.MustCompile(`^until `)
	dDo    = regexp.MustCompile(`^do `)
)

// dCond: awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' — 기다림 블록의 조건.
func dCond(p string) string {
	var l []string
	f := false
	for _, x := range lines(read(p)) {
		if dUntil.MatchString(x) {
			f = true
			x = dUntil.ReplaceAllString(x, "")
		}
		if f && dDo.MatchString(x) {
			break
		}
		if f {
			l = append(l, x)
		}
	}
	return strings.Join(l, "\n")
}

// dEg: 가짜 git(T/eg, log는 $T/subjects)을 PATH 앞에 두고 조건을 sh로 돌려 yes/no.
func (k *K) dEg(cond string, subjects ...string) string {
	write(k.t, k.T+"/subjects", strings.Join(subjects, "\n")+"\n")
	out, _ := k.exe(k.T, []string{"PATH=" + k.T + "/eg:" + k.env["PATH"]}, "sh", "-c", "if "+cond+"\nthen echo yes; else echo no; fi")
	return out
}

// dEln: ls 출력의 e1 줄.
func (k *K) dEln() string { return strings.Join(grepLines(k.out, "^e1 "), "\n") }

// dNf14: 칸 수 · 12 · 13 · 14칸.
func (k *K) dNf14(name string) string {
	var l []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			l = append(l, fmt.Sprintf("%d %s %s %s", len(r), col(r, 12), col(r, 13), col(r, 14)))
		}
	}
	return strings.Join(l, "\n")
}

// dEdit: idle.tsv 줄마다 f로 고쳐 다시 쓴다(awk … 1 >x && mv).
func (k *K) dEdit(f func(r []string) []string) {
	it := k.DATA + "/idle.tsv"
	var b strings.Builder
	for _, r := range rows(it) {
		b.WriteString(strings.Join(f(r), "\t") + "\n")
	}
	write(k.t, it, b.String())
}

func dPad(r []string, n int) []string {
	for len(r) < n {
		r = append(r, "")
	}
	return r
}

// dBack <칸> <초>: 시각을 앞당김.
func (k *K) dBack(c, d int) {
	k.dEdit(func(r []string) []string {
		r = dPad(r, c)
		v, _ := strconv.ParseInt(strings.TrimSpace(r[c-1]), 10, 64)
		r[c-1] = strconv.FormatInt(v-int64(d), 10)
		return r
	})
}

// dUnsent: 보낸 적 없는 것으로(셋째 칸 0).
func (k *K) dUnsent() { k.dEdit(func(r []string) []string { r = dPad(r, 3); r[2] = "0"; return r }) }

// dIt: idle.tsv에서 기계 m 줄의 c칸.
func (k *K) dIt(m string, c int) string {
	var l []string
	for _, r := range rows(k.DATA + "/idle.tsv") {
		if col(r, 1) == m {
			l = append(l, col(r, c))
		}
	}
	return strings.Join(l, "\n")
}

// dIdlek: 칸 수 · 보낸 사유.
func (k *K) dIdlek(m string) string {
	var l []string
	for _, r := range rows(k.DATA + "/idle.tsv") {
		if col(r, 1) == m {
			l = append(l, fmt.Sprintf("%d %s", len(r), col(r, 6)))
		}
	}
	return strings.Join(l, "\n")
}

// dCapn: 상한 사유 줄 수 · 보낸 횟수가 1이 아닌 줄 수.
func (k *K) dCapn() (n, bad int) {
	for _, r := range rows(k.DATA + "/idle.tsv") {
		if col(r, 6) != "cap" {
			continue
		}
		n++
		v := col(r, 4)
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			if f != 1 {
				bad++
			}
		} else if v != "1" {
			bad++
		}
	}
	return n, bad
}

func secD2(k *K) {
	RL := k.T + "/report.log"
	// # 10f. 먼저 띄움(2026-11-18 hcfill-early)
	k.fresh()
	k.add("e1", "--after", "pa,pc", "--only", "slow")
	k.run("run")
	k.check("먼저 띄움: 선행이 안 들어간 레인을 빈 자리에 띄우고 대기열 먼저 칸 early",
		k.where("e1") == "up slow slow/e1" && k.col14("e1") == "early" && strings.Contains(k.out, "먼저 띄움 e1 → slow slow/e1"), read(k.Q))
	rf := k.DATA + "/runs/e1-run.md"
	k.V["rf"] = rf
	k.check("먼저 띄움 절: run 지시문 = 지시문 + 착지 줄 + 먼저 띄움 절(기다림·기한·rebase)",
		hasF(rf, "# 지시문 e1") && hasF(rf, "### 착지 STEPS e1 slow") && hasE(rf, "^## 먼저 띄움") && hasF(rf, "선행 대기 초과") &&
			hasF(rf, "git rebase origin/main") && hasF(rf, "선행 레인(pa, pc)"), read(rf))
	joined := false
	for _, l := range fgrepLines(read(rf), "| grep -E '") {
		if regexp.MustCompile(`pa.*pc|pc.*pa`).MatchString(l) {
			joined = true
		}
	}
	k.check("먼저 띄움 절: 선행마다 판정 줄이 따로(두 선행을 한 줄로 이어 붙이지 않음)", countF(rf, "| grep -E '") == 2 && !joined, read(rf))
	// 기다림 블록의 조건을 그대로 돌려 본다: 가짜 git(fetch 성공, log는 $T/subjects)
	writeX(k.t, k.T+"/eg/git", "#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat \"$T_LOG/subjects\" ;; esac\n")
	cond := dCond(rf)
	k.check("먼저 띄움 기다림: 하나만 들어가면 아직", k.dEg(cond, "feat(x): pa 끝") == "no")
	k.check("먼저 띄움 기다림: 둘 다 제목 머리에 들어가면 풀림(after_ok와 같은 규칙)", k.dEg(cond, "feat(x): pa 끝", "pc: 고침") == "yes",
		k.dEg(cond, "feat(x): pa 끝", "pc: 고침"), cond)
	k.check("먼저 띄움 기다림: 문장 안 언급·이름 앞머리(pa-extra)는 안 셈", k.dEg(cond, "feat: pa-extra", "docs: pa 와 pc 언급", "fix(y): 고침 (pc)") == "no")
	k.run("ls")
	k.check("ls: 먼저 띄움(선행 …)", strings.Contains(k.dEln(), "먼저 띄움(선행 pa,pc)"))
	// 2026-11-19 fill-ls-early-state: 떠 있는 먼저 띄움 레인은 local 저장소의 지금 있는 origin/<기본>으로 선행을 본다
	k.lj("slow/e1:claude-code:e1")
	rm(k.T + "/git.log")
	k.run("ls")
	k.check("ls 먼저 띄움: 떠 있고 선행 모두 아직 → 「선행 기다림 pa,pc」, fetch 안 함",
		strings.Contains(k.dEln(), "먼저 띄움(선행 기다림 pa,pc)") && !hasF(k.T+"/git.log", "fetch"), read(k.T+"/git.log"))
	write(k.t, k.T+"/commits", "aaaa1111 feat(x): pa 끝\n")
	k.run("ls")
	k.check("ls 먼저 띄움: 일부 → 「선행 기다림 pc · 들어감 pa」", strings.Contains(k.dEln(), "먼저 띄움(선행 기다림 pc · 들어감 pa)"))
	appendFile(k.t, k.T+"/commits", "bbbb2222 pc: 고침\n")
	rm(k.T + "/git.log")
	k.run("ls")
	k.check("ls 먼저 띄움: 모두 들어감 → 「선행 들어감 — 본 작업」, fetch 안 함",
		strings.Contains(k.dEln(), "먼저 띄움(선행 들어감 — 본 작업)") && !hasF(k.T+"/git.log", "fetch"), read(k.T+"/git.log"))
	saved := read(k.conf())
	k.confDel(`^repo +app +here `)
	k.run("ls")
	write(k.t, k.conf(), saved)
	k.check("ls 먼저 띄움: 판정 못 함(local 저장소 줄 없음) → 지금처럼 「선행 pa,pc」", strings.Contains(k.dEln(), "먼저 띄움(선행 pa,pc)"))
	k.set("HC_FAKE_LOG_FAIL=1")
	k.run("ls")
	k.unset("HC_FAKE_LOG_FAIL")
	k.check("ls 먼저 띄움: 판정 못 함(origin/<기본> 없음 — git log 실패) → 「선행 pa,pc」", strings.Contains(k.dEln(), "먼저 띄움(선행 pa,pc)"))
	k.lj()
	k.run("ls")
	ended := false
	for _, l := range fgrepLines(k.dEln(), "끝남") {
		if strings.Contains(l, "먼저 띄움(선행 pa,pc)") {
			ended = true
		}
	}
	k.check("ls 먼저 띄움: 끝난 레인은 선행이 들어갔어도 그대로 「선행 pa,pc」", ended)
	k.fresh()
	k.add("e1", "--after", "pa", "--only", "slow")
	k.run("run", "--dry-run")
	k.check("먼저 띄움 dry-run: 찍기만", strings.Contains(k.out, "[dry-run] 먼저 띄울 e1 → slow") && k.state("e1") == "wait" && !exists(k.T+"/lane.log"))

	k.fresh()
	k.run("add", "--name", "e2", "--project", "app", "--brief", k.brief("e2"), "--after", "pa", "--no-early", "--only", "slow")
	k.check("add --no-early: 대기열 먼저 칸 no-early, 알림에 「먼저 띄움 끔」", k.rc == 0 && k.col14("e2") == "no-early" && strings.Contains(k.out, "먼저 띄움 끔"))
	k.run("run")
	k.check("--no-early: 자리가 비어도 먼저 띄우지 않음", k.state("e2") == "wait" && !exists(k.T+"/lane.log"))
	k.run("ls")
	k.check("ls: --no-early 줄에 「먼저 띄움 끔」", strings.Contains(strings.Join(grepLines(k.out, "^e2 "), "\n"), "먼저 띄움 끔"))

	k.fresh() // 띄울 수 있는 것이 먼저, 먼저 띄움은 우선순위·넣은 차례
	k.add("el", "--after", "pa", "--priority", "low", "--only", "slow")
	k.add("n1", "--only", "slow")
	k.add("eh", "--after", "pa", "--priority", "high", "--only", "slow")
	k.run("run")
	k.check("먼저 띄움은 띄울 수 있는 것(n1) 다음: slow 자리 1을 n1이 쓰고 먼저 띄움 없음",
		k.where("n1") == "up slow slow/n1" && k.state("eh") == "wait" && k.state("el") == "wait")
	k.fresh()
	k.add("el", "--after", "pa", "--priority", "low", "--only", "slow")
	k.add("eh", "--after", "pa", "--priority", "high", "--only", "slow")
	k.run("run")
	k.check("먼저 띄움 차례: 우선순위 높은 eh가 먼저", k.where("eh") == "up slow slow/eh" && k.state("el") == "wait")
	k.fresh()
	k.set("HC_FAKE_FETCH_FAIL=1")
	k.add("ef", "--after", "pa", "--only", "slow")
	k.run("run")
	k.unset("HC_FAKE_FETCH_FAIL")
	k.check("선행 판정 못 함(fetch 실패)은 먼저 띄우지 않음", k.state("ef") == "wait" && !exists(k.T+"/lane.log"))

	k.fresh() // 상한: 설정 early = 살아 있는 먼저 띄움 레인 수
	k.confSed(`^early$`, "early 1")
	k.add("ea", "--after", "pa", "--only", "box-1")
	k.add("eb", "--after", "pa", "--only", "box-1")
	k.run("run")
	k.check("먼저 띄움 상한 1: 하나만 띄우고 나머지는 대기, 한 줄",
		k.where("ea") == "up box-1 box-1/ea" && k.state("eb") == "wait" && strings.Contains(k.out, "먼저 띄움 상한 1개"))
	k.lj("box-1/ea:claude-code:ea")
	k.run("run")
	k.check("먼저 띄움 상한: 띄운 것이 살아 있으면 다음 바퀴에도 더 안 띄움", k.state("eb") == "wait")
	k.lj()
	k.run("run")
	k.check("먼저 띄움 상한: 띄운 것이 끝나면 다음 것을 띄움", k.where("eb") == "up box-1 box-1/eb")
	k.confSed(`^early 1$`, "early 0")
	k.fresh()
	k.add("ez", "--after", "pa", "--only", "slow")
	k.run("run")
	k.check("설정 early 0: 먼저 띄움 끔", k.state("ez") == "wait" && !exists(k.T+"/lane.log"))
	k.cpConf()

	// 감사 [중간] 1: 선행이 대기열에 wait로 남아 있으면 뒤 레인을 먼저 띄우지 않는다
	k.fresh() // 시나리오 1
	k.add("a1", "--after", "z1", "--only", "slow")
	k.add("b1", "--after", "a1", "--only", "slow", "--priority", "high")
	k.run("run")
	k.check("교착 1: 선행 a1(대기열 wait)을 기다리는 b1은 먼저 띄우지 않고 a1을 먼저 띄움",
		k.where("a1") == "up slow slow/a1" && k.state("b1") == "wait" && strings.Contains(k.out, "먼저 띄움 건너뜀 b1: 선행 a1"), read(k.Q))
	k.fresh() // 시나리오 2
	k.add("a2", "--after", "z2", "--only", "slow", "--no-early")
	k.add("b2", "--after", "a2", "--only", "slow")
	k.run("run")
	k.check("교착 2: --no-early 선행 a2가 대기열 wait면 b2를 먼저 띄우지 않음(자리 비움)",
		k.state("b2") == "wait" && k.state("a2") == "wait" && !exists(k.T+"/lane.log"))
	write(k.t, k.T+"/commits", "7777aaaa feat: z2 끝\n")
	k.run("run")
	k.check("교착 2: z2가 들어가면 a2가 자리를 잡음(b2는 그 뒤)", k.where("a2") == "up slow slow/a2" && k.state("b2") == "wait")
	k.fresh() // 같은 바퀴에 선행이 먼저 띄움으로 떴으면 뒤 레인도 후보(자리 2인 box-1)
	k.add("c1", "--after", "z3", "--only", "box-1", "--priority", "high")
	k.add("d1", "--after", "c1", "--only", "box-1")
	k.run("run")
	k.check("같은 바퀴에 선행 c1이 먼저 띄움으로 뜨면 d1도 먼저 띄움", k.where("c1") == "up box-1 box-1/c1" && k.where("d1") == "up box-1 box-1/d1")

	// 감사 [낮음] 6: 먼저 띄운 레인은 선행이 들어간 뒤에도 다시 뜨지 않는다, 옛 10·12칸 줄은 14칸으로 채움
	k.fresh()
	k.brief("o10")
	k.brief("o12")
	write(k.t, k.Q, "o10\tapp\t"+k.T+"/o10.md\tz4\tslow\twait\t-\t-\t-\t2026-01-01T00:00\n"+
		"o12\tapp\t"+k.T+"/o12.md\tz4\tbox-1\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\n")
	k.run("run")
	k.check("옛 10·12칸 줄도 먼저 띄우고 14칸(우선 normal, 먼저 early)으로 채움",
		k.where("o10") == "up slow slow/o10" && k.where("o12") == "up box-1 box-1/o12" &&
			k.dNf14("o10") == "14 - normal early" && k.dNf14("o12") == "14 - normal early", read(k.Q))
	write(k.t, k.T+"/commits", "8888bbbb fix: z4 끝\n")
	write(k.t, k.T+"/lane.log", "")
	k.lj("slow/o10:claude-code:o10", "box-1/o12:claude-code:o12")
	k.run("run")
	k.check("먼저 띄운 레인: 선행이 들어가도(터미널 살아 있음) 다시 안 띄움", k.launched("--name") == 0)
	k.lj()
	k.run("run")
	k.check("먼저 띄운 레인: 터미널이 끝난 뒤에도 다시 안 띄움", k.launched("--name") == 0 && k.state("o10") == "up")

	// # 10g. 놀고 있음 알림
	IT := k.DATA + "/idle.tsv"
	k.V["IT"] = IT
	nrl := func() int { return countE(RL, ".") } // grep -c .
	k.fresh()
	k.run("run")
	k.check("놀고 있음: 처음 빈 것은 시각만 적고 알리지 않음(대기열이 비어도 자리를 봄)",
		k.rc == 0 && !exists(RL) && hasE(IT, "^box-1\t") && hasE(IT, "^here\t"), read(IT))
	k.dBack(2, 660)
	k.run("run")
	k.check("놀고 있음: idle분 넘으면 사건 한 번 — 기계·자리 수·분, 레인 이름 hc-fill-idle, 터미널 밖으로",
		nrl() == 1 && hasF(RL, "report 중간 - box-1 자리 2개 11분째 빔") && hasF(RL, "here 자리 4개 11분째 빔") &&
			hasF(RL, "대기열에 띄울 것 없음 --name hc-fill-idle") && hasF(RL, "(터미널 없음)"), read(RL))
	k.check("놀고 있음: 대기 줄이 없으면 모든 지휘(--to 없음)", !hasF(RL, "--to"))
	k.run("run")
	k.check("놀고 있음: 30분 안에는 다시 안 보냄", nrl() == 1)
	// 2026-11-19 idle-once: 사유가 「대기열에 띄울 것 없음」이면 비기 시작한 뒤 한 번만
	k.dBack(3, 1860)
	k.run("run")
	k.check("놀고 있음(대기열 빔): 계속 비어도 30분 뒤 다시 안 보냄", nrl() == 1, read(RL), read(IT))
	k.dBack(3, 7300)
	k.run("run")
	k.check("놀고 있음(대기열 빔): 2시간 뒤에도 안 보냄", nrl() == 1, read(RL), read(IT))
	k.lj("box-1/x1:claude-code:a", "box-1/x2:claude-code:b")
	k.run("run")
	k.check("놀고 있음: 자리가 차면 그 기계 기록을 지움(다시 비면 처음부터)", !hasE(IT, "^box-1\t") && hasE(IT, "^box-2\t"))
	k.lj()
	k.run("run")
	k.dBack(2, 660)
	rm(RL)
	k.run("run")
	k.check("놀고 있음(대기열 빔): 자리가 찼다가 다시 비면 그 기계만 다시 한 번",
		nrl() == 1 && hasF(RL, "box-1 자리 2개 11분째 빔") && !hasF(RL, "box-2 자리"), read(RL), read(IT))
	k.dBack(3, 7300)
	rm(RL)
	k.run("run")
	k.check("놀고 있음(대기열 빔): 다시 보낸 뒤로도 한 번만", !nonEmpty(RL), read(RL))
	k.check("idle.tsv: 보낸 줄 여섯째 칸에 보낸 사유 none", k.dIdlek("box-1") == "6 none", read(IT))
	k.dEdit(func(r []string) []string { return dPad(r, 5)[:5] })
	k.dBack(3, 7300)
	rm(RL)
	k.run("run")
	k.check("idle.tsv: 옛 5칸 줄(보낸 적 있음)도 대기열 빔을 다시 안 보냄, 6칸으로 채움",
		!nonEmpty(RL) && k.dIdlek("box-1") == "6 none", read(RL), read(IT))
	// 2026-11-19 hcfill-idle-seats: 자리가 하나라도 차면 그 순간부터 다시 셈
	k.fresh()
	k.run("run")
	k.dBack(2, 660)
	k.lj("box-1/y1:claude-code:a")
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 빈 자리 수가 줄면(레인이 뜸) 처음부터 다시 셈 — 그 바퀴엔 알리지 않음",
		!hasF(RL, "box-1 자리") && hasF(RL, "box-2 자리 2개 11분째 빔") && k.dIt("box-1", 5) == "1", read(RL), read(IT))
	k.dBack(2, 660)
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 다시 센 뒤 idle분 넘으면 그때 알림(box-1 자리 1개 11분째)", hasF(RL, "box-1 자리 1개 11분째 빔"), read(RL), read(IT))
	s1 := k.dIt("box-1", 2)
	k.lj()
	k.run("run")
	k.check("놀고 있음: 빈 자리 수가 늘면(레인이 끝남) 시작 시각은 그대로", k.dIt("box-1", 2) == s1 && k.dIt("box-1", 5) == "2", read(IT))
	k.fresh()
	k.add("w1", "--after", "pa", "--no-early", "--only", "slow", "--coord", "tq1")
	k.add("w2", "--after", "pa", "--no-early", "--only", "slow", "--coord", "tq2")
	k.run("run")
	k.dBack(2, 700)
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 대기 줄 주인 지휘들에게만(--to tq1,tq2)", hasF(RL, "--to tq1,tq2"), read(RL))
	k.set("HC_FAKE_COORD_GONE=tq1")
	k.dUnsent()
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 사라진 주인은 빼고(--to tq2)", hasF(RL, "--to tq2 "), read(RL))
	k.set("HC_FAKE_COORD_GONE=tq1 tq2")
	k.dUnsent()
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 살아 있는 주인이 없으면 모든 지휘", nonEmpty(RL) && !hasF(RL, "--to"), read(RL))
	k.unset("HC_FAKE_COORD_GONE")
	k.add("w3", "--after", "pa", "--no-early", "--only", "slow")
	k.dUnsent()
	rm(RL)
	k.run("run")
	k.check("놀고 있음: 주인 없는 대기 줄이 있으면 모든 지휘", nonEmpty(RL) && !hasF(RL, "--to"), read(RL))
	k.set("HC_FAKE_REPORT_RC=1")
	k.dUnsent()
	rm(RL)
	k.run("run")
	ok := strings.Contains(k.out, "놀고 있음 알림 실패")
	if ok { // 셸 조건 안에서 unset 뒤 run run 하고 두 번째 보냄을 센다
		k.unset("HC_FAKE_REPORT_RC")
		k.run("run")
		ok = nrl() == 2
	}
	k.check("놀고 있음: 보내기(report) 실패면 한 줄, 보낸 시각을 안 적어 다음 바퀴에 다시", ok, read(RL))
	k.unset("HC_FAKE_REPORT_RC")
	k.fresh() // 감사 [낮음] 3: 세 번 보낸 뒤로는 2시간마다 — 되풀이는 사유가 먼저 띄움 상한일 때만
	k.confSed(`^early$`, "early 1")
	k.add("ca", "--after", "pa", "--only", "box-2")
	k.add("cb", "--after", "pa", "--only", "box-2")
	k.run("run")
	k.lj("box-2/ca:claude-code:ca")
	k.dBack(2, 700)
	k.run("run")
	k.dBack(3, 1900)
	k.run("run")
	k.dBack(3, 1900)
	k.run("run")
	k.check("되풀이 상한: 30분마다 세 번", countF(RL, "먼저 띄움 상한") == 3, read(RL), read(IT))
	k.dBack(3, 1900)
	k.run("run")
	k.check("되풀이 상한: 세 번 뒤로는 30분 지나도 안 보냄", countF(RL, "먼저 띄움 상한") == 3)
	k.dBack(3, 7300)
	k.run("run")
	k.check("되풀이 상한: 2시간 지나면 다시", countF(RL, "먼저 띄움 상한") == 4, read(IT))
	k.check("되풀이 상한: 넷 다 먼저 띄움 상한 사유", countF(RL, "먼저 띄움 상한(early 1)에 걸려 남김") == 4, read(RL))
	// 아스트라 2차 낮음 1: 상한 사유는 상한에 걸린 후보를 받을 기계(cb --only box-2)에만
	capBox1 := len(fgrepLines(strings.Join(fgrepLines(read(RL), "먼저 띄움 상한"), "\n"), "box-1 자리")) > 0
	idleBox1 := len(fgrepLines(strings.Join(fgrepLines(read(RL), "대기열에 띄울 것 없음"), "\n"), "box-1 자리"))
	k.check("되풀이 상한: 상한 사유는 그 후보를 받을 box-2에만, box-1은 「대기열에 띄울 것 없음」 한 번", !capBox1 && idleBox1 == 1, read(RL))
	k.cpConf()
	k.fresh() // 사유: 먼저 띄움 상한에 걸려 남긴 자리면 그렇게
	k.confSed(`^early$`, "early 0")
	k.run("run")
	k.dBack(2, 700)
	k.run("run")
	k.check("사유: 먼저 띄움 상한에 안 걸렸으면 「대기열에 띄울 것 없음」", hasF(RL, "대기열에 띄울 것 없음"))
	k.confSed(`^early 0$`, "early 1")
	k.add("ca", "--after", "pa", "--only", "box-1")
	k.add("cb", "--after", "pa", "--only", "box-2")
	rm(RL)
	k.run("run") // 상한 사유는 cb를 받을 box-2에(2차 낮음 1)
	k.check("사유: 대기열 빔으로 한 번 보낸 뒤 상한으로 바뀌면 30분 안이어도 그 사유로 다시 — 「먼저 띄움 상한(early 1)에 걸려 남김」",
		hasF(RL, "먼저 띄움 상한(early 1)에 걸려 남김") && !hasF(RL, "대기열에 띄울 것 없음"), read(RL))
	cn, cbad := k.dCapn()
	k.check("사유가 바뀌어 보내면 보낸 횟수는 1부터(감사 낮음 1 — 상한 사유도 30분 간격 세 번을 다 받게)", cbad == 0 && cn >= 1, read(IT))
	k.cpConf()
	k.fresh()
	k.run("run", "--dry-run")
	k.check("놀고 있음: dry-run은 기록·알림 없음", !exists(IT) && !exists(RL))
}
