package hcfill_test

// 셸 시험 10h+(최종 감사)·10i(--then)·10h(마지막 보고) — 겹침(10j~)은 sec_f2.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func init() { reg(60, "f", secF) }

// --- 이 절(f·f2) 도우미 ---

// fQset: 대기열에서 1칸이 name인 줄의 c칸을 v로(awk '$1==n {$c = v} 1').
func (k *K) fQset(name string, c int, v string) {
	var b strings.Builder
	for _, r := range rows(k.Q) {
		if col(r, 1) == name {
			for len(r) < c {
				r = append(r, "")
			}
			r[c-1] = v
		}
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	write(k.t, k.Q, b.String())
}

// fQsum: 셸의 qsum(대기열 + briefs/added/*.md를 이어 붙인 글 — cksum 대신 글 그대로 비교).
func (k *K) fQsum() string {
	s := read(k.Q)
	l, _ := filepath.Glob(k.DATA + "/briefs/added/*.md")
	for _, p := range l {
		s += read(p)
	}
	return s
}

// fLrl: 셸의 lrl — 출력에서 「<이름> 」으로 시작하는 줄들.
func (k *K) fLrl(name string) string {
	var l []string
	for _, x := range lines(k.out) {
		if strings.HasPrefix(x, name+" ") {
			l = append(l, x)
		}
	}
	return strings.Join(l, "\n")
}

// fCut: cut -d" " -f<i…>을 줄마다.
func fCut(s string, idx ...int) string {
	var out []string
	for _, x := range lines(s) {
		f := strings.Split(x, " ")
		var p []string
		for _, i := range idx {
			if i <= len(f) {
				p = append(p, f[i-1])
			}
		}
		out = append(out, strings.Join(p, " "))
	}
	return strings.Join(out, "\n")
}

// fWhereU: for x in …; do where $x; done | sort -u.
func (k *K) fWhereU(names ...string) string {
	seen := map[string]bool{}
	var l []string
	for _, n := range names {
		for _, x := range lines(k.where(n)) {
			if !seen[x] {
				seen[x] = true
				l = append(l, x)
			}
		}
	}
	sort.Strings(l)
	return strings.Join(l, "\n")
}

// fBoth: grep -F a 파일 | grep -Fq b.
func fBoth(p, a, b string) bool {
	for _, x := range fgrepLines(read(p), a) {
		if strings.Contains(x, b) {
			return true
		}
	}
	return false
}

// fNonEmpty: grep -c . (빈 줄이 아닌 줄 수).
func fNonEmpty(p string) int {
	n := 0
	for _, x := range lines(read(p)) {
		if x != "" {
			n++
		}
	}
	return n
}

// fOR: 겹침 시험 저장소($T/orepo).
func (k *K) fOR() string { return k.T + "/orepo" }

var (
	reRepoHere = regexp.MustCompile(`^(repo[[:space:]]+app[[:space:]]+here[[:space:]]+).*`)
	reOverlap  = regexp.MustCompile(`^overlap[[:space:]]`)
)

// fOfresh: 셸의 ofresh [warn|block|off|none] — 저장소 경로(repo app here)를 $OR로, overlap 줄을 그 값으로(none이면 뺌), 사건 파일 비움.
func (k *K) fOfresh(mode string) {
	k.fresh()
	rm(k.T + "/events")
	if mode == "" {
		mode = "warn"
	}
	var out []string
	for _, l := range lines(read(k.EX)) {
		if m := reRepoHere.FindStringSubmatch(l); m != nil {
			l = m[1] + k.fOR()
		}
		if reOverlap.MatchString(l) {
			if mode == "none" {
				continue
			}
			l = "overlap  " + mode
		}
		out = append(out, l)
	}
	write(k.t, k.conf(), strings.Join(out, "\n")+"\n")
}

// fOadd <이름> <본문> [인자…]: 지시문 「# 지시문 <이름>\n<본문>\n」으로 add(프로젝트 app, 출력 버림).
func (k *K) fOadd(name, body string, extra ...string) {
	p := k.T + "/" + name + ".md"
	write(k.t, p, "# 지시문 "+name+"\n"+body+"\n")
	k.fill(nil, append([]string{"add", "--name", name, "--project", "app", "--brief", p}, extra...)...)
}

// fBack <칸> <초>: idle.tsv의 그 칸 시각을 앞당김(셸의 back).
func (k *K) fBack(c, d int) {
	p := k.DATA + "/idle.tsv"
	var b strings.Builder
	for _, r := range rows(p) {
		for len(r) < c {
			r = append(r, "")
		}
		v, _ := strconv.Atoi(r[c-1])
		r[c-1] = strconv.Itoa(v - d)
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	write(k.t, p, b.String())
}

func fHead(s string, n int) string {
	l := lines(s)
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, "\n")
}

func fTailN(s string, n int) string {
	l := lines(s)
	if n > len(l) {
		return ""
	}
	return l[len(l)-n]
}

func fUTC(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

func secF(k *K) {
	// 10h+. 최종 감사: 설정 audit_final <안내>가 있으면 run 지시문 감사 절 3번·ls가 「최종 감사 통과 뒤에만 착지」
	copyFile(k.t, k.conf(), k.T+"/conf.keep")
	mkdirs(k.t, k.W+"/app.wt/wf")
	k.afresh()
	k.confAppend("audit_final 최종 감사(지휘 자동 감사기)")
	k.add("wf", "--only", "slow")
	k.run("run")
	rf := k.DATA + "/runs/wf-run.md"
	k.check("audit_final: run 지시문 감사 절 3번은 「감사 통과여도 착지하지 않는다 — 최종 감사(<안내>) 통과 알림을 기다린다」",
		hasF(rf, "감사 통과 …」여도 착지하지 않는다 — 최종 감사(최종 감사(지휘 자동 감사기))의 통과 알림을 기다린다") && !hasF(rf, "3. 출력이 「감사 통과 …」면 그 출력의 착지 줄"), read(rf))
	k.lj("slow/wf:claude-code:wf:done:" + k.W + "/app.wt/wf")
	k.ev("slow/wf", "wf", "완료", "abc1234", "끝")
	k.run("run")
	WF := k.aidof("wf")
	k.lj("slow/wf:claude-code:wf:done:"+k.W+"/app.wt/wf", "slow/audit-"+WF+":claude-code:audit-"+WF+":done:"+k.W+"/app.wt/wf")
	k.verd("slow/audit-"+WF, WF, "0", "0", "1")
	k.run("run")
	write(k.t, k.VD()+"/wf/"+WF+".ack", "landing\tx\twf@1700000000000\tp\n")
	k.run("run")
	o, _ := k.exe(k.T, nil, k.FILL, "ls")
	ok := false
	for _, x := range lines(o) {
		if strings.HasPrefix(x, "wf ") && strings.Contains(x, "감사 통과 받음(최종 대기)") {
			ok = true
		}
	}
	k.check("audit_final: ls 비고 「감사 통과 받음(최종 대기)」", ok, o)
	copyFile(k.t, k.T+"/conf.keep", k.conf())

	// 10i. --then: 같은 일의 다음 단계는 새 줄 없이 대기 중인 선행 줄 지시문에 이어 붙임
	k.fresh()
	k.add("p1")
	pb := k.DATA + "/briefs/added/p1.md"
	k.run("add", "--name", "n1", "--project", "app", "--brief", k.brief("n1"), "--then", "p1")
	k.check("--then: 종료 0, 대기열 줄 수 그대로, 출력 한 줄", k.rc == 0 && fNonEmpty(k.Q) == 1 && has(lines(k.out), "이어 붙임 n1 → p1 (새 레인 없음, 선행 지시문 "+pb+")"), read(k.Q))
	pbs := read(pb)
	k.check("--then: 선행 지시문 = 원래 본문 + 「## 이어서: n1」 절·커밋 머리 약속·본문",
		strings.Contains(fHead(pbs, 2), "# 지시문 p1") && grep(pbs, `^## 이어서: n1 \(hc-fill add --then, 20[0-9-]+T[0-9:]+\)$`) &&
			strings.Contains(pbs, "커밋 제목 머리는 `n1:`") && strings.Contains(pbs, "위 일의 완료 보고·착지는 하지 않는다 — 이 절까지 끝낸 뒤 한 번만 report 완료") &&
			fTailN(pbs, 2) == "# 지시문 n1" && fTailN(pbs, 1) == "본문", pbs)
	s0 := k.fQsum()
	k.run("add", "--name", "n1", "--project", "app", "--brief", k.T+"/n1.md", "--then", "p1")
	k.check("--then: 같은 이름 두 번 → 종료 2, 그대로", k.rc == 2 && strings.Contains(k.out, "n1은 이미 p1에 이어 붙였다") && k.fQsum() == s0)
	k.run("add", "--name", "n2", "--project", "app", "--brief", k.brief("n2"), "--then", "n1")
	k.check("--then: 이어 붙인 이름을 선행으로 → 종료 2 + 실제 줄 안내", k.rc == 2 && strings.Contains(k.out, "n1은 p1에 이어 붙였다 — --then p1") && k.fQsum() == s0)
	k.run("add", "--name", "n2", "--project", "app", "--brief", k.T+"/n2.md", "--then", "nope")
	k.check("--then: 선행이 대기열에 없음 → 종료 2", k.rc == 2 && strings.Contains(k.out, "선행이 대기열에 없다: nope") && k.fQsum() == s0)
	k.run("add", "--name", "n1", "--project", "app", "--brief", k.T+"/n2.md")
	k.check("add: 이어 붙인 이름을 새 줄로 → 거절, 그대로", k.rc != 0 && strings.Contains(k.out, "n1은 이미 p1에 이어 붙였다(--then)") && k.fQsum() == s0)
	for _, o := range []string{"--after p1", "--only box-1", "--priority 높음", "--no-early", "--no-audit", "--coord t0"} {
		k.run(append([]string{"add", "--name", "n3", "--project", "app", "--brief", k.brief("n3"), "--then", "p1"}, strings.Fields(o)...)...)
		first, _, _ := strings.Cut(o, " ")
		k.check("--then과 "+o+" 함께 → 종료 2, 그대로", k.rc == 2 && strings.Contains(k.out, "--then은 "+first+"와 함께 쓰지 않는다") && k.fQsum() == s0)
	}
	write(k.t, k.T+"/empty.md", "")
	k.run("add", "--name", "n3", "--project", "app", "--brief", k.T+"/empty.md", "--then", "p1")
	k.check("--then: 빈 지시문 → 종료 2", k.rc == 2 && strings.Contains(k.out, "지시문을 읽지 못함(빈 파일)") && k.fQsum() == s0)
	// 감사 [낮음] 2: 사본 폴더 밖 지시문(사본 기능 전에 넣은 옛 줄)은 원본을 고치지 않게 거절
	copyFile(k.t, k.Q, k.Q+".keep")
	k.fQset("p1", 3, k.T+"/p1.md")
	s1 := read(k.T + "/p1.md")
	k.run("add", "--name", "n3", "--project", "app", "--brief", k.T+"/n3.md", "--then", "p1")
	k.check("--then: 선행 지시문이 사본 폴더 밖 → 종료 2, 원본 그대로", k.rc == 2 && strings.Contains(k.out, "p1: 지시문이 사본이 아니다("+k.T+"/p1.md) — rm 뒤 다시 add") && read(k.T+"/p1.md") == s1)
	if err := os.Rename(k.Q+".keep", k.Q); err != nil {
		k.t.Fatal(err)
	}
	qb := filepath.Dir(pb) + "/q1.md"
	copyFile(k.t, k.T+"/n1.md", qb) // 다른 프로젝트(tools)의 대기 줄 q1
	var qs strings.Builder
	for _, r := range rows(k.Q) {
		if col(r, 1) == "p1" {
			q := []string{"q1", "tools", qb}
			for c := 4; c <= 15; c++ {
				q = append(q, col(r, c))
			}
			qs.WriteString(strings.Join(q, "\t") + "\n")
		}
		qs.WriteString(strings.Join(r, "\t") + "\n")
	}
	write(k.t, k.Q, qs.String())
	s0 = k.fQsum()
	k.run("add", "--name", "n3", "--project", "app", "--brief", k.T+"/n3.md", "--then", "q1")
	k.check("--then: 프로젝트가 다름 → 종료 2", k.rc == 2 && strings.Contains(k.out, "q1: 프로젝트가 다르다(tools)") && k.fQsum() == s0)
	for _, x := range []string{"up|6|이미 떴다(up)", "sent|6|이미 떴다(sent)", "box-1|inbox11|받은편지 줄에는", "audit=p9|15|자동 감사 줄에는", "box-1=box-1/p1|9|상태 안 잡힘으로 남긴 터미널이 있다"} {
		f := strings.Split(x, "|")
		v, c, why := f[0], f[1], f[2]
		copyFile(k.t, k.Q, k.Q+".keep")
		switch c {
		case "6":
			k.fQset("p1", 6, v)
		case "9":
			k.fQset("p1", 9, v)
		case "inbox11":
			k.fQset("p1", 11, v+":/in/done/p1.md")
		default:
			k.fQset("p1", 15, v)
		}
		s0 = k.fQsum()
		k.run("add", "--name", "n4", "--project", "app", "--brief", k.brief("n4"), "--then", "p1")
		k.check("--then: 선행이 "+v+"("+c+") → 종료 2 「"+why+"」, 그대로", k.rc == 2 && strings.Contains(k.out, "p1: "+why) && k.fQsum() == s0)
		if err := os.Rename(k.Q+".keep", k.Q); err != nil {
			k.t.Fatal(err)
		}
	}
	k.fQset("p1", 6, "up")
	k.run("add", "--name", "n4", "--project", "app", "--brief", k.T+"/n4.md", "--then", "p1")
	k.check("--then: 떠 있는 선행 → 「이미 떴다(up) — --after p1」", strings.Contains(k.out, "이미 떴다(up) — --after p1으로 넣거나 그 레인에 hc send"))
	k.fQset("p1", 6, "wait")
	k.run("ls")
	k.check("ls: 선행 줄 비고에 「이어서 n1」", grep(k.out, `^p1 .*이어서 n1`))
	// q1은 다른 프로젝트(tools) 레인 — 2026-11-22부터 --after-any-project 없이는 거절(sec_r)
	k.run("add", "--name", "a1", "--project", "app", "--brief", k.brief("a1"), "--after", "p1,q1", "--after-any-project")
	k.check("--after에 대기 중 같은 프로젝트 레인 → 알림 한 줄 + 정상 추가, 다른 프로젝트는 알림 없음", k.rc == 0 && k.where("a1") != "" &&
		has(lines(k.out), "알림: p1가 아직 대기 중 — 같은 일의 다음 단계면 --after 대신 --then p1로 한 레인에 이어 붙인다") && !strings.Contains(k.out, "q1가 아직"))
	k.fQset("p1", 6, "up")
	k.run("add", "--name", "a2", "--project", "app", "--brief", k.brief("a2"), "--after", "p1")
	k.check("--after에 떠 있는 레인 → 알림 없음", k.rc == 0 && !strings.Contains(k.out, "아직 대기 중"))
	k.fresh()
	k.add("p2")
	k.run("add", "--name", "n5", "--project", "app", "--brief", k.brief("n5"), "--then", "p2")
	k.run("run")
	rf = k.DATA + "/runs/p2-run.md"
	k.check("run: 선행을 띄울 때 run 지시문에 이어 붙인 절이 들어감", k.state("p2") == "up" && hasF(rf, "## 이어서: n5 (hc-fill add --then") && hasF(rf, "# 지시문 n5") && k.where("n5") == "", read(rf))
	k.fresh()
	k.add("p3")
	// 감사 [낮음] 5: 시간 비교 없이 순서로 — 배경이 run.lock을 잡은 뒤(표식) --then을 부르고, 배경이 잠금 안에서 남긴 줄이 --then 줄보다 먼저
	mark, ord := k.T+"/then.mark", k.T+"/then.ord"
	write(k.t, ord, "")
	lk := exec.Command("lockf", "-k", k.DATA+"/run.lock", "sh", "-c", `: >"$1"; sleep 1; echo run >>"$2"`, "_", mark, ord)
	lk.Dir, lk.Env = k.T, k.environ(nil)
	if err := lk.Start(); err != nil {
		k.t.Fatal(err)
	}
	for i := 0; !exists(mark) && i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	k.run("add", "--name", "n6", "--project", "app", "--brief", k.brief("n6"), "--then", "p3")
	appendFile(k.t, ord, "then\n")
	lk.Wait()
	k.check("--then: 도는 run(run.lock)이 끝난 뒤에 붙인다 — run이 지시문을 읽고 up으로 바꾸는 사이에 끼지 않게", exists(mark) && k.rc == 0 && strings.Join(lines(read(ord)), ",") == "run,then", read(ord))

	// 10h. 마지막 보고: run이 바퀴마다 레인 사건에서 대기열 레인마다 마지막 사건을 $DATA/last.tsv에 적고, ls는 그 파일만 읽는다
	LR := k.DATA + "/last.tsv"
	k.fresh()
	rm(k.T+"/events", k.T+"/events.log")
	k.cpConf()
	k.add("lr1", "--only", "box-1")
	k.add("lr2", "--only", "slow")
	k.run("run")
	k.lj("box-1/lr1:claude-code:lr1:done:/", "slow/lr2:claude-code:lr2:working:/")
	write(k.t, k.T+"/events", "2026-11-19T08:30:00Z\tm\tbox-1/lr1\tlr1\t완료\t1372684aa\t다 됨\t1\n2026-11-19T08:38:00Z\tm\tbox-1/lr1\tlr1\t착지\t8bccc0d11\tmain에 넣음\t2\n2026-11-19T08:39:00Z\tm\tbox-1/other\tlr1\t막힘\t-\t남의 터미널\t1\n")
	rm(k.T + "/events.log")
	utc := []string{"TZ=UTC"}
	k.runE(utc, "run")
	el := k.T + "/events.log"
	k.check("마지막 보고: run이 hc-lane events를 한 번만(기계마다 --on 묶어) 읽음", fNonEmpty(el) == 1 && hasF(el, "--on box-1") && hasF(el, "--on slow"), read(el))
	rm(el, k.T+"/hc.log")
	k.run("ls")
	k.check("ls 마지막 보고: 그 터미널의 마지막 사건 착지 sha 8자·시각(남의 터미널 사건은 안 셈)", strings.Contains(k.fLrl("lr1"), "착지 8bccc0d1 08:38 main에 넣음"), read(LR))
	k.check("ls 마지막 보고: 사건 없는 레인은 「보고 없음」", strings.Contains(k.fLrl("lr2"), "보고 없음"))
	k.check("ls는 원격 호출 안 함(hc-lane events·hc exec 없음)", !exists(el) && !hasE(k.T+"/hc.log", "^exec"), read(el), read(k.T+"/hc.log"))
	k.mj("box-2", "shared", "slow")
	rm(el)
	k.run("run")
	k.run("ls")
	k.check("연결 끊긴 기계: 지난 값을 두고 「(옛 값 HH:MM)」, 그 기계는 events에 --on 안 함", strings.Contains(k.fLrl("lr1"), "착지 8bccc0d1 08:38") &&
		grep(k.fLrl("lr1"), `\(옛 값 [0-9]{2}:[0-9]{2}\)`) && !strings.Contains(k.fLrl("lr2"), "옛 값") && !hasF(el, "--on box-1"), read(LR), read(el))
	k.mj("box-1", "box-2", "shared", "slow")
	k.run("run")
	k.run("ls")
	var r17 []string
	for _, r := range rows(k.Q) {
		if col(r, 1) == "lr1" {
			r17 = append(r17, col(r, 17))
		}
	}
	rid := strings.Join(r17, "\n")
	f8 := false
	for _, r := range rows(LR) {
		if col(r, 1) == "lr1" && col(r, 8) == rid {
			f8 = true
		}
	}
	k.check("last.tsv 여덟째 칸 = 대기열 줄 ID", rid != "" && f8, read(LR))
	k.fQset("lr1", 17, "rnew") // 같은 이름의 새 줄(rm·다시 add)
	k.mj("box-2", "shared", "slow")
	k.run("run")
	k.run("ls")
	k.check("기존 6: 끊긴 기계의 지난 값은 줄 ID가 같을 때만 — 새 동명 줄에 옛 착지 sha가 안 붙음", !strings.Contains(k.fLrl("lr1"), "8bccc0d1"), read(LR))
	k.mj("box-1", "box-2", "shared", "slow")
	k.run("run")
	k.run("ls")
	k.check("다시 연결되면 옛 값 표시 없음", strings.Contains(k.fLrl("lr1"), "착지 8bccc0d1") && !strings.Contains(k.fLrl("lr1"), "옛 값"))
	before := read(LR)
	appendFile(k.t, k.T+"/events", "2026-11-19T09:00:00Z\tm\tslow/lr2\tlr2\t막힘\t-\t테스트 실패\t1\n")
	k.run("run", "--dry-run")
	k.check("dry-run은 last.tsv를 쓰지 않음", exists(LR) && read(LR) == before, read(LR))
	k.runE(utc, "run")
	k.run("ls")
	k.check("막힘은 sha 없이 「막힘 09:00 <앞부분>」", strings.Contains(k.fLrl("lr2"), "막힘 09:00 테스트 실패"))
	// ht-clean-unstick(2026-11-25): clean 멈춤 막힘 뒤 clean이 성공해 「막힘 풀림」 중간을 적으면 마지막 보고는 더는 막힘이 아니다
	appendFile(k.t, k.T+"/events", "2026-11-19T09:05:00Z\tm\tslow/lr2\tlr2\t중간\t-\t막힘 풀림: hc-lane clean 성공 — lr2 작업 트리·가지 정리 끝\t2\n")
	k.runE(utc, "run")
	k.run("ls")
	k.check("clean 성공 뒤 「막힘 풀림」이면 마지막 보고에 막힘이 남지 않음", !strings.Contains(k.fLrl("lr2"), "막힘 09:00") && strings.Contains(k.fLrl("lr2"), "막힘 풀림"), k.fLrl("lr2"))
}
