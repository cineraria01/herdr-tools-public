package hcfill_test

// 10z. 착지 표식(셸 시험 1940~2051줄): 선행은 커밋 제목 머리 규칙 또는 그 선행 줄을 넣은 뒤의 착지 표식
// refs/hc-landed/<선행>/<UTC 초>(hc-lane land가 main과 같은 원자 push에)의 sha가 origin/<기본>의 조상일 때 들어감. git 오류면 아직.
// 지시문이 내는 기다림 셸 글(until … do)을 뽑아 sh·zsh·bash로 직접 돌려 본다. 원격 없는 master·git log 부분 실패도.
// 10k. 선행 사슬(셸 시험 2052~2170줄, 2026-11-19 fill-dep-needs 사용자 「① 선행 사슬 기다림 (추천)」): 선행 먼저·HERDR_LAND_PRIORITY,
// 먼저 띄움 기계의 선행 상태(after.tsv), 기다림 블록을 가짜 시계로 그대로 돌리기, 기계 능력(필요: 줄), 빈 지시문 거절.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() { reg(70, "g", secG) }

// gMark <레인> <sha> <UTC 초>: 가짜 git ls-remote가 읽는 착지 표식 한 줄.
func (k *K) gMark(lane, sha string, sec int64) {
	appendFile(k.t, k.T+"/landed/"+lane, sha+"\trefs/hc-landed/"+lane+"/"+strconv.FormatInt(sec, 10)+"\n")
}

// gEdit: TSV 파일을 줄마다 f로 고쳐 쓴다(awk -F'\t' -v OFS='\t' '…' 1 꼴).
func (k *K) gEdit(p string, f func(r []string) []string) {
	var b strings.Builder
	for _, r := range rows(p) {
		b.WriteString(strings.Join(f(r), "\t") + "\n")
	}
	write(k.t, p, b.String())
}

// gPut: r의 c칸(1부터)을 v로 — 모자라면 빈 칸으로 늘린다(awk $c=v).
func gPut(r []string, cv map[int]string) []string {
	for c, v := range cv {
		for len(r) < c {
			r = append(r, "")
		}
		r[c-1] = v
	}
	return r
}

// gSetQ: 대기열에서 이름이 names 가운데 하나인 줄의 칸들을 바꾼다. v 안의 {n}은 그 줄 이름.
func (k *K) gSetQ(cv map[int]string, names ...string) {
	k.gEdit(k.Q, func(r []string) []string {
		if !has(names, col(r, 1)) {
			return r
		}
		m := map[int]string{}
		for c, v := range cv {
			m[c] = strings.ReplaceAll(v, "{n}", col(r, 1))
		}
		return gPut(r, m)
	})
}

// gUntil: awk '/^until /{f=1; sub(/^until /, "")} f && /^do /{exit} f' — 첫 기다림 조건(until 뗌, do 줄 앞까지).
// keep이면 awk '/^until /{f=1} f; f && /^do /{exit}' — until 줄부터 do 줄까지 그대로(블록).
func gUntil(s string, keep bool) string {
	var o []string
	f := false
	for _, l := range lines(s) {
		if strings.HasPrefix(l, "until ") {
			f = true
			if !keep {
				l = strings.TrimPrefix(l, "until ")
			}
		}
		if f && strings.HasPrefix(l, "do ") {
			if keep {
				o = append(o, l)
			}
			break
		}
		if f {
			o = append(o, l)
		}
	}
	return strings.Join(o, "\n")
}

// gShBlock: awk '/^```sh$/ {f=1; next} f && /^```$/ {exit} f' — 첫 ```sh 블록 안.
func gShBlock(s string) string {
	var o []string
	f := false
	for _, l := range lines(s) {
		if !f && l == "```sh" {
			f = true
			continue
		}
		if f && l == "```" {
			break
		}
		if f {
			o = append(o, l)
		}
	}
	return strings.Join(o, "\n")
}

// gLine: 출력에서 prefix로 시작하는 줄 가운데 sub가 든 줄이 있나(grep "^<prefix>" | grep -Fq <sub>).
func gLine(out, prefix, sub string) bool {
	for _, l := range lines(out) {
		if strings.HasPrefix(l, prefix) && strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// gCmp: cmp -s a b.
func gCmp(a, b string) bool {
	x, e1 := os.ReadFile(a)
	y, e2 := os.ReadFile(b)
	return e1 == nil && e2 == nil && string(x) == string(y)
}

func secG(k *K) {
	T := k.T
	later := nowS() + 600
	cl := k.state
	// 실제 사례(carousel-save-race ← main-split): 선행 커밋 제목이 「feat(frontend): main-split-1 …」 꼴이라 제목 머리 규칙에 안 걸려 기한 초과
	k.fresh()
	rm(T + "/landed")
	k.add("main-split")
	k.add("carousel-save-race", "--after", "main-split", "--no-early")
	write(k.t, T+"/commits", "aaa1111 feat(frontend): main-split-1 화면 나눔\nbbb2222 test(frontend): main-split-1 시험\n")
	k.run("run")
	k.check("표식: 제목 머리가 레인 이름이 아닌 커밋(main-split-1)만 있고 표식이 없으면 아직",
		k.where("carousel-save-race") == "wait - -" && strings.Contains(k.out, "선행 carousel-save-race ← main-split: 아직"))
	k.gMark("main-split", "aaa11110000", 1)
	k.run("run")
	k.check("표식: 그 선행 줄을 넣기 전의 옛 표식(같은 이름의 옛 실행)은 안 셈 — 아직", k.where("carousel-save-race") == "wait - -")
	k.gMark("main-split", "aaa11112222", later)
	k.runE([]string{"HC_FAKE_LSREMOTE_FAIL=1"}, "run")
	k.check("표식: ls-remote git 오류면 판정 보류(풀지 않음)",
		k.where("carousel-save-race") == "wait - -" && strings.Contains(k.out, "선행 carousel-save-race ← main-split: 판정 못 함"))
	k.runE([]string{"HC_FAKE_MB_RC=1"}, "run")
	k.check("표식: 표식 sha가 origin/main의 조상이 아니면(merge-base 1) 아직", k.where("carousel-save-race") == "wait - -")
	k.runE([]string{"HC_FAKE_MB_RC=128"}, "run")
	k.check("표식: merge-base git 오류(128)도 아직", k.where("carousel-save-race") == "wait - -")
	k.run("run")
	k.check("표식: 제목이 「feat(frontend): main-split-1 …」이어도 줄을 넣은 뒤의 표식이 조상이면 들어감(carousel-save-race 사례)",
		cl("carousel-save-race") == "up" && strings.Contains(k.out, "선행 carousel-save-race ← main-split: 들어감 (aaa11112 — 착지 표식)"))
	k.fresh()
	rm(T + "/landed")
	k.add("qx", "--after", "zz8", "--no-early")
	k.gMark("zz8", "cccc0000", later)
	k.run("run")
	k.check("표식: 대기열에 없는 선행은 표식을 안 보고 커밋 제목 머리 규칙만(줄을 넣은 시각이 없다)", k.where("qx") == "wait - -")
	write(k.t, T+"/commits", "dddd0000 zz8: 끝\n")
	k.run("run")
	k.check("표식: 제목 머리 규칙은 그대로(표식 없이도 들어감)", cl("qx") == "up" && strings.Contains(k.out, "선행 qx ← zz8: 들어감 (dddd0000"))
	// ls는 마지막 바퀴가 검증한 병합기 표식도 보인다. 진행 중이라는 go만으로는 풀지 않는다.
	k.fresh()
	rm(T + "/landed")
	k.add("main-split", "--only", "box-1")
	k.add("show-mark", "--after", "main-split", "--only", "slow")
	k.run("run")
	k.lj("box-1/main-split:claude-code:main-split", "slow/show-mark:claude-code:show-mark")
	k.run("run")
	k.run("ls")
	k.check("ls는 단순 진행 중 상태를 선행 착지로 표시하지 않음", gLine(k.out, "show-mark ", "선행 기다림 main-split"))
	write(k.t, T+"/commits", "abcd1111 feat(frontend): main-split-1 구현\n")
	k.gMark("main-split", "abcd1111", later)
	k.run("run")
	write(k.t, T+"/git.log", "")
	k.run("ls")
	k.check("ls는 조회한 원격 표식의 로컬 조상을 확인해 실제 착지를 표시",
		gLine(k.out, "show-mark ", "선행 들어감") && !hasF(T+"/git.log", "ls-remote"))
	// 표식 조회 실패는 선행 미착지와 다르다. 먼저 띄울 수 있는 줄도 그 바퀴에는 보류한다.
	for _, fault := range []string{"목록", "객체"} {
		k.fresh()
		rm(T + "/landed")
		k.add("needs-mark", "--only", "box-1")
		k.add("later-mark", "--after", "needs-mark", "--only", "slow")
		if fault == "목록" {
			k.runE([]string{"HC_FAKE_LSREMOTE_FAIL=1"}, "run")
		} else {
			k.gMark("needs-mark", "abc12345", later)
			k.runE([]string{"HC_FAKE_MB_RC=128"}, "run")
		}
		k.check("표식 "+fault+" 조회 실패 때 먼저 띄우기를 보류", k.where("later-mark") == "wait - -")
	}
	// 같은 분의 오래된 표식도 새 대기열 실행의 증거가 될 수 없다.
	k.fresh()
	rm(T + "/landed")
	k.add("same-min")
	k.add("wait-min", "--after", "same-min", "--no-early")
	mt, err := time.ParseInLocation("2006-01-02T15:04:05", "2026-11-20T10:00:00", time.Local)
	if err != nil {
		k.t.Fatal(err)
	}
	rowsec := mt.Unix() + 45
	k.gSetQ(map[int]string{10: "2026-11-20T10:00", 17: "r" + strconv.FormatInt(rowsec, 10) + "-1-2"}, "same-min")
	k.gMark("same-min", "abc1111", rowsec-1)
	k.run("run")
	k.check("새 줄을 넣기 1초 전의 같은 분 표식은 보류", k.where("wait-min") == "wait - -")
	k.gMark("same-min", "abc2222", rowsec)
	k.run("run")
	k.check("줄을 넣은 초부터의 표식으로 후속을 풂", cl("wait-min") == "up")
	secGEarly(k, later)
	secGLocal(k)
	secG2(k)
}

// secGEarly: 먼저 띄운 레인의 기다림 줄도 같은 판정 — 그 줄을 가짜 git 응답으로 실제로 돌린다(표식 있음·없음·옛 표식·ls-remote 오류·merge-base 1·128).
func secGEarly(k *K, later int64) {
	T := k.T
	k.fresh()
	rm(T + "/landed")
	k.add("ea", "--only", "box-1")
	k.add("eb", "--after", "ea,ec", "--only", "slow")
	k.run("run")
	rf := k.DATA + "/runs/eb-run.md"
	k.check("먼저 띄움 기다림 줄: 대기열 선행(ea)은 제목 머리 또는 refs/hc-landed/ea/* 표식, 대기열 밖(ec)은 제목 머리만",
		hasF(rf, "git ls-remote origin 'refs/hc-landed/ea/*'") && !hasF(rf, "refs/hc-landed/ec/"), read(rf))
	writeX(k.t, T+"/eg2/git", `#!/bin/sh
case $1 in fetch) exit 0 ;; log) cat "$T_LOG/subjects" ;; ls-remote) [ ! -f "$T_LOG/lsrfail" ] || exit 128; cat "$T_LOG/lsr" 2>/dev/null; exit 0 ;; merge-base) exit "$(cat "$T_LOG/mbrc" 2>/dev/null || echo 0)" ;; esac
`)
	cond := gUntil(read(rf), false)
	eg2 := []string{"PATH=" + T + "/eg2:" + k.env["PATH"]}
	// ew <표식 초|-> <mb 종료> <lsr 실패 1|-> <제목…>: 기다림 조건을 돌려 yes|no
	ew := func(mark, mb, lsrfail string, subj ...string) string {
		rm(T+"/lsr", T+"/lsrfail", T+"/mbrc")
		if mark != "-" {
			write(k.t, T+"/lsr", "9999aaaa0000\trefs/hc-landed/ea/"+mark+"\n")
		}
		if mb != "0" {
			write(k.t, T+"/mbrc", mb+"\n")
		}
		if lsrfail != "-" {
			write(k.t, T+"/lsrfail", "")
		}
		write(k.t, T+"/subjects", strings.Join(subj, "\n")+"\n")
		o, _ := k.exe(T, eg2, "sh", "-c", "if "+cond+"\nthen echo yes; else echo no; fi")
		return o
	}
	ls := strconv.FormatInt(later, 10)
	k.check("먼저 띄움 기다림: ea 표식(줄을 넣은 뒤)이 조상이면 ea 제목이 레인 이름이 아니어도 풀림(+ ec 제목)",
		ew(ls, "0", "-", "feat(frontend): ea-1 다른 제목", "ec: 고침") == "yes", cond)
	k.check("먼저 띄움 기다림: 표식이 없으면(빈 답·종료 0) 아직", ew("-", "0", "-", "feat(frontend): ea-1 다른 제목", "ec: 고침") == "no")
	k.check("먼저 띄움 기다림: 줄을 넣기 전의 옛 표식은 안 셈", ew("1", "0", "-", "ec: 고침") == "no")
	k.check("먼저 띄움 기다림: ls-remote 오류면 아직", ew(ls, "0", "1", "ec: 고침") == "no")
	k.check("먼저 띄움 기다림: merge-base 1(조상 아님)·128(오류)이면 아직",
		ew(ls, "1", "-", "ec: 고침") == "no" && ew(ls, "128", "-", "ec: 고침") == "no")
	k.check("먼저 띄움 기다림: 제목 머리 규칙은 그대로(ea: …, 표식 없이)", ew("-", "0", "-", "ea: 끝", "ec: 고침") == "yes")
	// 2026-11-20 fill-early-wait-zsh: 표식 판정이 파이프 끝 { …; exit 1; }라 zsh(워커 Bash 도구의 셸)에서는 그 exit가 셸째 끝내
	// 기다림이 바로 빠졌다. 블록 전체(until … done)를 zsh·bash로 돌린다 — 가짜 sleep이 한 바퀴 뒤 ea 제목을 넣어 다음 판정에서 풀린다
	writeX(k.t, T+"/eg2/sleep", "#!/bin/sh\necho slept; printf \"ea: 끝\\n\" >>\"$T_LOG/subjects\"\n")
	blk := gUntil(read(rf), true)
	eb := func(sh string) string {
		rm(T+"/lsr", T+"/lsrfail", T+"/mbrc")
		write(k.t, T+"/subjects", "ec: 고침\n")
		o, _ := k.exe(T, eg2, sh, "-c", blk+"\necho rc=$?")
		return o
	}
	for _, sh := range []string{"zsh", "bash"} {
		o := eb(sh)
		k.check("먼저 띄움 기다림 블록("+sh+"): 선행 안 들어감이면 셸이 끝나지 않고 기다림 계속 → 들어가면 풀림",
			strings.Join(lines(o), " ") == "slept rc=0", o, blk)
	}
}

// secGLocal: 원격 없는 저장소의 로컬 master, git log 부분 출력 뒤 실패.
func secGLocal(k *K) {
	T := k.T
	// 원격 없는 저장소의 로컬 master에서 선행을 판정하고 같은 참조로 기다린다.
	k.fresh()
	k.cpConf()
	repoDir := T + "/로컬 저장소"
	k.set("HC_REAL_REPO=" + repoDir)
	if o, rc := k.exe(T, nil, k.REALGIT, "init", "-qb", "master", repoDir); rc != 0 {
		k.t.Fatal("git init: " + o)
	}
	localCommit := func(msg string) {
		if o, rc := k.exe(T, nil, k.REALGIT, "-C", repoDir, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid",
			"commit", "-qm", msg, "--allow-empty"); rc != 0 {
			k.t.Fatal("git commit: " + o)
		}
	}
	localCommit("시작")
	k.confSed(regexp.QuoteMeta("/Users/me/src/app"), strings.ReplaceAll(repoDir, "$", "$$"))
	k.add("wait-local", "--after", "prep", "--only", "here")
	k.add("next-local", "--after", "prep", "--only", "here", "--no-early")
	k.run("run")
	rf := k.DATA + "/runs/wait-local-run.md"
	cond := gUntil(read(rf), false)
	localWait := func() bool {
		_, rc := k.exe(repoDir, []string{"PATH=" + strings.TrimPrefix(k.env["PATH"], T+"/bin:")}, "sh", "-c", cond)
		return rc == 0
	}
	k.check("원격 없는 master: 준비만 띄우고 기다림 조건은 거짓", cond != "" && !localWait() && k.where("next-local") == "wait - -")
	localCommit("prep: 준비 완료")
	k.run("run")
	k.check("원격 없는 master: 선행 착지 뒤 대기열과 생성한 기다림 조건 모두 풀림",
		cond != "" && localWait() && k.where("next-local") == "up here next-local" && hasE(rf, "git rebase refs/heads/master"))
	k.fresh()
	k.confAppend("audit " + k.EXA)
	k.add("check-local", "--only", "here")
	k.run("run")
	k.lj("check-local:claude-code:check-local:done:/")
	write(k.t, T+"/events", "2026-11-20T10:00:00Z\there\tcheck-local\tcheck-local\t완료\tabc1234\t끝\t1\n")
	k.run("run")
	briefs, _ := filepath.Glob(k.DATA + "/briefs/audit/*.md")
	found := false
	for _, b := range briefs {
		found = found || hasF(b, "diff refs/heads/master...")
	}
	k.check("원격 없는 master: 감사 지시문도 로컬 기준으로 대조", found)
	k.unset("HC_REAL_REPO")
	k.cpConf()

	// Git이 제목 일부를 출력하고 128로 끝나면 대기열과 생성한 기다림 블록 모두 보류한다.
	k.fresh()
	k.add("partial", "--after", "need", "--only", "slow")
	write(k.t, T+"/commits", "abcd1234 need: 제목\n")
	k.runE([]string{"HC_FAKE_PARTIAL_LOG=1"}, "run")
	k.check("선행 git log 부분 출력 뒤 실패: 본 작업과 먼저 띄우기를 모두 보류",
		k.where("partial") == "wait - -" && !nonEmpty(T+"/lane.log"))
	k.fresh()
	k.add("partial", "--after", "need", "--only", "slow")
	k.run("run")
	rf = k.DATA + "/runs/partial-run.md"
	cond = gUntil(read(rf), false)
	writeX(k.t, T+"/partial-bin/git", "#!/bin/sh\ncase $1 in log) echo \"need: 제목\"; exit 128 ;; esac\n")
	_, rc := k.exe(T, []string{"PATH=" + T + "/partial-bin:" + k.env["PATH"]}, "sh", "-c", cond)
	k.check("생성한 기다림 조건: git log가 제목 뒤 실패하면 기다림", cond != "" && rc != 0)
}

// gAF: $HOME/.local/share/hc-fill/after.tsv.
func (k *K) gAF() string { return k.DATA + "/after.tsv" }

// gAfv: 선행의 판정 까닭 — awk '$1==n {print $2 " " $3; exit}' after.tsv.
func (k *K) gAfv(n string) string {
	for _, r := range rows(k.gAF()) {
		if col(r, 1) == n {
			return col(r, 2) + " " + col(r, 3)
		}
	}
	return ""
}

// gAF4: [ "$(awk -F"\t" "{print \$4}" after.tsv)" -gt 0 ] — 한 줄이고 넷째 칸이 양의 정수.
func (k *K) gAF4() bool {
	r := rows(k.gAF())
	if len(r) != 1 {
		return false
	}
	n, err := strconv.Atoi(col(r[0], 4))
	return err == nil && n > 0
}

// gNb <이름> <능력> [add 인자…]: 「- 필요: <능력>」 줄이 든 지시문으로 add.
func (k *K) gNb(name, need string, extra ...string) {
	p := k.T + "/" + name + ".md"
	write(k.t, p, "# 지시문 "+name+"\n본문\n- 필요: "+need+"\n")
	k.fill(nil, append([]string{"add", "--name", name, "--project", "app", "--brief", p}, extra...)...)
}

// secG2: 10k 선행 사슬 ① 기다리는 줄이 있는 선행을 같은 우선순위 안에서 먼저(많을수록 먼저), 그 수를 레인 환경 HERDR_LAND_PRIORITY로.
func secG2(k *K) {
	T, lp := k.T, k.T+"/landprio.log"
	cl := k.state
	k.cpConf()
	k.fresh()
	k.add("x1", "--only", "slow")
	k.add("p1", "--only", "slow")
	k.add("q1", "--after", "p1", "--only", "slow", "--no-early")
	k.add("q2", "--after", "p1", "--only", "slow", "--no-early")
	k.run("run")
	k.check("선행 먼저: 기다리는 줄이 있는 p1이 먼저 넣은 x1보다 먼저", k.where("p1") == "up slow slow/p1" && cl("x1") == "wait")
	k.check("HERDR_LAND_PRIORITY: 띄울 때 레인 환경에 기다리는 줄 수", hasFx(lp, "p1 2"), read(lp))
	k.lj()
	write(k.t, lp, "")
	k.gSetQ(map[int]string{6: "sent"}, "p1")
	k.run("run")
	k.check("HERDR_LAND_PRIORITY: 기다리는 줄이 없으면 0", hasFx(lp, "x1 0"), read(lp))
	k.fresh()
	k.add("pa1", "--only", "slow")
	k.add("pb1", "--only", "slow")
	k.add("w1", "--after", "pa1", "--only", "slow", "--no-early")
	k.add("w2", "--after", "pb1", "--only", "slow", "--no-early")
	k.add("w3", "--after", "pb1", "--only", "slow", "--no-early")
	k.run("run")
	k.check("선행 먼저: 기다리는 줄이 많은 pb1(2)이 pa1(1)보다 먼저", k.where("pb1") == "up slow slow/pb1" && cl("pa1") == "wait")
	k.fresh()
	k.add("pc1", "--only", "slow")
	k.add("wc", "--after", "pc1", "--only", "slow", "--no-early")
	k.add("hx", "--only", "slow", "--priority", "high")
	k.run("run")
	k.check("선행 먼저는 같은 우선순위 안에서만: 높음 hx가 먼저", k.where("hx") == "up slow slow/hx" && cl("pc1") == "wait")
	k.fresh() // 떠 있는 먼저 띄움 레인도 기다리는 줄로 센다
	k.add("z3", "--only", "slow")
	k.add("p3", "--only", "slow")
	k.add("e3", "--after", "p3", "--only", "box-1")
	k.gSetQ(map[int]string{6: "up", 7: "box-1", 8: "box-1/e3", 14: "early"}, "e3")
	k.lj("box-1/e3:claude-code:e3")
	k.run("run")
	k.check("선행 먼저: 떠 있는 먼저 띄움 레인(e3)이 기다리는 p3이 먼저", k.where("p3") == "up slow slow/p3" && hasFx(lp, "p3 1"), read(k.Q))
	k.fresh() // --then으로 이어 붙인 이름을 기다리는 줄은 그 이름을 품은 줄의 몫
	k.add("x4", "--only", "slow")
	k.add("p4", "--only", "slow")
	k.fill(nil, "add", "--name", "p4b", "--project", "app", "--brief", k.brief("p4b"), "--then", "p4")
	k.add("w4", "--after", "p4b", "--only", "slow", "--no-early")
	k.run("run")
	k.check("선행 먼저: --then 이름(p4b)을 기다리는 줄이 있으면 그 이름을 품은 p4가 먼저",
		k.where("p4") == "up slow slow/p4" && cl("x4") == "wait", read(k.Q))
	k.fresh() // 자동 감사 줄도 그 선행의 차례(감사 줄끼리는 모두 긴급)
	k.brief("ad-a")
	k.brief("ad-b")
	k.brief("c5")
	write(k.t, k.Q, "audit-a5\tapp\t"+T+"/ad-a.md\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\turgent\tno-early\taudit=a5\n"+
		"audit-b5\tapp\t"+T+"/ad-b.md\t-\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\turgent\tno-early\taudit=b5\n"+
		"c5\tapp\t"+T+"/c5.md\tb5\tslow\twait\t-\t-\t-\t2026-01-01T00:00\t-\t-\tnormal\tno-early\t-\n")
	k.run("run")
	k.check("선행 먼저: 기다리는 줄이 있는 b5의 감사 줄이 a5의 감사 줄보다 먼저",
		k.where("audit-b5") == "up slow slow/audit-b5" && cl("audit-a5") == "wait", read(k.Q))
	secG2After(k)
	secG2Block(k)
	secG2Needs(k)
}

// secG2After: ② 먼저 띄움 레인 기계에 선행 상태(after.tsv: 선행 go|stop 까닭 쓴시각)를 바퀴마다 써 준다.
func secG2After(k *K) {
	T, AF := k.T, k.gAF()
	k.fresh()
	k.add("pp", "--only", "box-1")
	k.add("ee", "--after", "pp", "--only", "slow")
	k.run("run")
	k.check("먼저 띄움 준비: pp는 box-1, ee는 slow에 먼저 띄움", k.where("pp") == "up box-1 box-1/pp" && k.where("ee") == "up slow slow/ee")
	rm(AF)
	write(k.t, T+"/hc.log", "")
	k.lj("box-1/pp:claude-code:pp", "slow/ee:claude-code:ee")
	k.run("run")
	k.check("선행 상태: 선행이 진행 중(working)이면 go, 먼저 띄움 레인 기계(slow)에 hc exec로 씀",
		strings.HasPrefix(k.gAfv("pp"), "go 진행 중") && hasE(T+"/hc.log", `^exec.*--on slow`) && k.gAF4(), read(AF), read(T+"/hc.log"))
	k.lj("box-1/pp:claude-code:pp:idle:/w/app.wt/pp", "slow/ee:claude-code:ee")
	write(k.t, T+"/events", "2026-11-19T09:00:00Z\tm\tbox-1/pp\tpp\t막힘\t-\t시험 실패\t1\n")
	k.run("run")
	k.check("선행 상태: 선행이 쉬고 마지막 사건이 막힘이면 stop 「막힘: …」", strings.HasPrefix(k.gAfv("pp"), "stop 막힘: 시험 실패"), read(AF))
	appendFile(k.t, T+"/events", "2026-11-19T09:10:00Z\tm\tbox-1/pp\tpp\t완료\t1234abcd\t다 됨\t2\n")
	k.run("run")
	k.check("선행 상태: 쉬어도 마지막 사건이 완료면(감사·착지 기다림) go", strings.HasPrefix(k.gAfv("pp"), "go "), read(AF))
	rm(T + "/events")
	k.run("run")
	k.check("선행 상태: 사건 없이 막 쉬기 시작했으면 go(쉼)", strings.HasPrefix(k.gAfv("pp"), "go 쉼"), read(AF))
	k.gEdit(k.HELD, func(r []string) []string {
		if col(r, 1) == "box-1/pp" {
			n, _ := strconv.ParseInt(col(r, 2), 10, 64)
			r = gPut(r, map[int]string{2: strconv.FormatInt(n-61*60, 10)})
		}
		return r
	})
	k.run("run")
	k.check("선행 상태: 사건 없이 hold분 넘게 쉬면 stop", strings.HasPrefix(k.gAfv("pp"), "stop "), read(AF), read(k.HELD))
	k.lj("slow/ee:claude-code:ee")
	k.run("run")
	k.check("선행 상태: 선행 터미널이 사라졌으면 stop 「터미널 없음」", grep(k.gAfv("pp"), `^stop .*터미널 없음`), read(AF))
	write(k.t, T+"/commits", "abcd1234 pp: 끝\n")
	k.run("run")
	k.check("선행 상태: 선행이 들어갔으면 go 「들어감」", strings.HasPrefix(k.gAfv("pp"), "go 들어감"), read(AF))
	write(k.t, T+"/commits", "")
	if b, err := os.ReadFile(AF); err == nil {
		write(k.t, T+"/af.before", string(b))
	}
	k.run("run", "--dry-run")
	k.check("선행 상태: dry-run은 쓰지 않음", gCmp(AF, T+"/af.before"))
	k.fresh() // 선행이 대기열에서 기다림(아직 안 뜸)이면 go, 대기열에 없으면 stop
	k.add("pw", "--only", "box-1", "--after", "zz", "--no-early")
	k.add("ew", "--after", "pw", "--only", "slow")
	k.add("ex", "--after", "nope9", "--only", "slow")
	k.gSetQ(map[int]string{6: "up", 7: "slow", 8: "slow/{n}", 14: "early"}, "ew", "ex")
	k.lj("slow/ew:claude-code:ew", "slow/ex:claude-code:ex")
	k.run("run")
	k.check("선행 상태: 대기열에서 기다리는 선행은 go, 대기열에 없는 선행은 stop",
		strings.HasPrefix(k.gAfv("pw"), "go 대기열") && strings.HasPrefix(k.gAfv("nope9"), "stop 대기열에 없음"), read(AF))
}

// secG2Block: ③ 기다림 블록을 그대로 돌린다(가짜 시계: date +%s가 기한 뒤, 가짜 sleep이 두 번째에 선행을 넣음, early-wait는 진짜 hc-lane).
func secG2Block(k *K) {
	T, AF := k.T, k.gAF()
	k.fresh()
	k.add("pp", "--only", "box-1")
	k.add("ee", "--after", "pp", "--only", "slow")
	k.run("run")
	rf := k.DATA + "/runs/ee-run.md"
	blk := gShBlock(read(rf))
	k.check("먼저 띄움 절: 기한에 닿으면 hc-lane early-wait <선행>으로 선행 상태를 본다",
		strings.Contains(blk, "hc-lane early-wait pp") && hasF(rf, "선행이 살아 진행 중"), read(rf))
	mkdirs(k.t, T+"/clk")
	rm(T + "/clk/hc-lane")
	if err := os.Symlink(k.HL, T+"/clk/hc-lane"); err != nil {
		k.t.Fatal(err)
	}
	writeX(k.t, T+"/clk/date", "#!/bin/sh\n[ \"$1\" = +%s ] && { echo 9999999000; exit 0; }\nexec /bin/date \"$@\"\n")
	writeX(k.t, T+"/clk/sleep", "#!/bin/sh\nn=$(cat \"$T_LOG/sleeps\" 2>/dev/null || echo 0); n=$((n + 1)); echo $n >\"$T_LOG/sleeps\"\n"+
		"[ $n -lt 2 ] || echo \"pp: 끝\" >>\"$T_LOG/subjects\"\n")
	writeX(k.t, T+"/clk/git", "#!/bin/sh\ncase $1 in fetch) exit 0 ;; log) cat \"$T_LOG/subjects\" ;; esac\n")
	blkrun := func() {
		rm(T + "/sleeps")
		write(k.t, T+"/subjects", "")
		k.out, _ = k.exe(T, []string{"PATH=" + T + "/clk:" + k.env["PATH"]}, "perl", "-e", "alarm 20; exec @ARGV", "bash", "-c", blk)
	}
	write(k.t, AF, "pp\tgo\t진행 중(working)\t9999998990\n")
	blkrun()
	k.check("기다림 블록: 기한이 지나도 선행이 진행 중이면 멈추지 않고, 선행이 들어가면 끝남",
		!strings.Contains(k.out, "선행 대기 초과") && strings.TrimRight(read(T+"/sleeps"), "\n") == "2", blk)
	write(k.t, AF, "pp\tstop\t막힘: 시험 실패\t9999998990\n")
	blkrun()
	k.check("기다림 블록: 기한에 선행이 막혔으면 까닭 한 줄과 「선행 대기 초과」로 멈춤",
		strings.Contains(k.out, "pp: 막힘: 시험 실패") && strings.Contains(k.out, "선행 대기 초과") && !exists(T+"/sleeps"))
}

// secG2Needs: 능력 기계(필요: 줄)와 빈 지시문.
func secG2Needs(k *K) {
	T := k.T
	// 능력이 있어도 프로젝트 경로나 쓰기 자리가 없는 기계에는 배정할 수 없다.
	for _, invalid := range []string{"저장소없음", "상한0"} {
		k.fresh()
		k.cpConf()
		switch invalid {
		case "저장소없음":
			k.confDel(`^repo .*app .*slow `)
		case "상한0":
			k.confSed(`^(machine +slow +)1`, "${1}0")
		}
		k.confAppend("machine-has slow xcode")
		write(k.t, T+"/need.md", "본문\n필요: xcode\n")
		k.fill(nil, "add", "--name", "need", "--project", "app", "--brief", T+"/need.md")
		k.run("run")
		k.check("능력 기계가 "+invalid+": 주인에게 막힘을 알림", k.where("need") == "wait - -" && hasF(T+"/report.log", "필요 xcode"))
	}
	k.cpConf()

	// ④ 기계 능력: 지시문의 「필요: <능력>,…」 줄과 설정 machine-has <기계> <능력,…>을 맞춰 능력이 없는 기계에는 띄우지 않는다
	rep := T + "/report.log"
	k.fresh()
	k.confAppend("machine-has slow railway,xcode", "machine-has box-2 xcode")
	k.gNb("nr", "railway")
	k.gNb("nx", "xcode")
	k.gNb("nk", "kube")
	k.add("plain")
	k.run("run")
	k.check("필요 railway: 가진 slow에만(앞 기계 box-1이 비어도)", k.where("nr") == "up slow slow/nr")
	k.check("필요 xcode: 없는 box-1은 건너뛰고 box-2", k.where("nx") == "up box-2 box-2/nx")
	k.check("필요 줄 없는 레인은 그대로 첫 기계", k.where("plain") == "up box-1 box-1/plain")
	k.check("필요 kube: 가진 기계 없음 → 대기 그대로, 막힘 한 줄, 주인 지휘에 막힘 사건 한 번",
		k.state("nk") == "wait" && grep(k.out, `막힘 nk.*kube`) && countE(rep, `막힘 .*--name nk`) == 1, read(rep))
	k.run("run")
	k.check("필요 막힘 알림은 바퀴마다 되풀이하지 않음", countE(rep, `막힘 .*--name nk`) == 1, read(rep))
	k.run("ls")
	k.check("ls: 필요 막힘 비고", gLine(k.out, "nk ", "필요 kube"))
	k.confAppend("machine-has box-1 kube")
	k.run("run")
	k.check("설정에 능력을 더하면 다음 바퀴에 뜸", k.where("nk") == "up box-1 box-1/nk")
	k.gNb("no", "railway", "--only", "box-1")
	k.run("run")
	k.check("필요 railway + --only box-1: 가진 기계가 기계 제한 밖이면 막힘", k.state("no") == "wait" && grep(k.out, `막힘 no`))
	write(k.t, T+"/np.md", "# 지시문 np\n본문\n필요: 설치와 확인(사람)\n")
	k.fill(nil, "add", "--name", "np", "--project", "app", "--brief", T+"/np.md", "--only", "box-2")
	k.run("run")
	k.check("필요: 뒤가 능력 이름 목록이 아니면(문장) 필요 줄로 안 봄", k.where("np") == "up box-2 box-2/np")
	k.cpConf()

	// ⑤ 빈 지시문(2026-11-16 travel-ta: 본문 없이 착지 부록만 든 run 지시문으로 레인이 떴다): 제목·부록 절 말고 글 줄이 없으면 거절
	k.fresh()
	write(k.t, T+"/ap.md", "\n### 착지·정리 (hc-lane이 채움)\n- 작업 폴더 /x (브랜치 x)\n1. hc-lane land main\n")
	k.run("add", "--name", "ap", "--project", "app", "--brief", T+"/ap.md")
	k.check("빈 지시문: 부록(착지·정리 절)만 든 지시문은 add 거절", k.rc != 0 && k.where("ap") == "" && strings.Contains(k.out, "본문 없음"))
	write(k.t, T+"/ho.md", "# 제목만\n\n## 감사·착지 (hc-fill 자동)\n1. x\n")
	k.run("add", "--name", "ho", "--project", "app", "--brief", T+"/ho.md")
	k.check("빈 지시문: 제목과 감사·착지 절만이면 거절", k.rc != 0 && k.where("ho") == "")
	k.add("ok1", "--only", "slow")
	copyFile(k.t, T+"/ap.md", k.DATA+"/briefs/added/ok1.md")
	k.run("run")
	k.check("빈 지시문: 대기열 사본이 부록만 되면 run도 띄우지 않음",
		k.state("ok1") == "wait" && !exists(T+"/lane.log") && strings.Contains(k.out, "실패 ok1"))
}
