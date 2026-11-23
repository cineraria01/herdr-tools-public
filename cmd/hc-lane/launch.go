package main

// 레인 띄우기(셸 판 bin/hc-lane 본문·lib/hc-lane/start.sh): 작업 트리 → 준비 훅 → 임대 → hc run --bg → 레인 기록·감시·시작 확인.
// 같은 인자·출력·종료 코드(0 띄움, 1 실패, 2 인자·대기열만, 5 시작 안 됨, 6 로그인, 7 키체인, 10 이미 돎).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

func init() { register("launch", launchMain, nil) }

var (
	swait, strust, poll, runBrief, cwd, hsp, addr, start, logPath, loOwner string
	fwait                                                                  int64
	tr, im                                                                 string
	ik                                                                     int
	itopSet                                                                bool
	itop, ihome                                                            string
)

func rmRule(w io.Writer) {
	fmt.Fprint(w, "- 임시 파일은 실행마다 `mktemp -d`로 새 폴더를 만들어 쓰고 지우지 않는다. `rm -rf`·`rm -f`에 변수(`$X/…`)나 와일드카드(`*`) 경로를 쓰지 않는다 — Claude Code가 권한 창을 띄워 무인 레인이 막힌다. 꼭 지워야 하면 글자 그대로의 경로나 `\"${X:?}\"` 꼴로.\n")
}

func printSteps(w io.Writer) {
	n := 0
	s := func(t string) { n++; fmt.Fprintf(w, "%d. %s\n", n, t) }
	fmt.Fprint(w, "### 착지·정리 (hc-lane이 채움)\n")
	fmt.Fprintf(w, "- 작업 폴더 %s (브랜치 %s, 기준 %s). 편집·명령은 이 폴더 안에서만 한다.\n", wt, branch, blab)
	fmt.Fprint(w, "- 커밋은 경로를 지정한다: git add -- <경로…> && git commit -m \"<메시지>\" -- <경로…> (add -A·commit -a·stash·reset --hard·강제 push 금지)\n")
	rmRule(w)
	fmt.Fprint(w, "- 알림: 중간·완료·착지·막힘 때 hc-lane report <상태> [<sha>] \"<한 줄>\"을 부른다(화면 줄과 보고 파일은 그대로 써도 됨). 지휘는 hc-lane wait로 이 사건만 기다린다\n")
	fmt.Fprint(w, "  예: hc-lane report 완료 HEAD \"<요약>\"(HEAD는 전체 해시로 풀림) ·아래 1이 착지하면 hc-lane report 착지 <sha8> · 막히면 hc-lane report 막힘 - \"<까닭>\"\n")
	s(fmt.Sprintf("cd %s && hc-lane land %s", q(wt), base))
	fmt.Fprint(w, "   # 이 한 줄을 그대로 돌린다. 나누거나 세미콜론으로 이어 쓰지 말 것. 앞 단계가 실패하면 push는 나가지 않는다\n")
	fmt.Fprint(w, "   # 저장소에 .claude/herdr-land.sh가 있으면 그 스크립트가 fetch·rebase·검증·push를 하고, 없으면 hc-lane이 같은 순서로 하며 .claude/herdr-verify.sh가 없으면 push하지 않고 실패한다\n")
	fmt.Fprintf(w, "   # rebase가 충돌로 멈추면(종료 3) 양쪽 내용을 살려 풀고 고친 파일을 git add 한 뒤 git rebase --continue, 그다음 이 줄을 처음부터 다시. 병합기 제출이 기본 브랜치와 충돌해 거절돼도 종료 3이다(rebase는 시작 전 — git rebase origin/%s 부터)\n", base)
	fmt.Fprint(w, "   # 검증이 실패하면 고쳐 커밋하고 이 줄을 다시. push 거절 횟수(종료 4)나 잠금 기다림 상한(종료 75)으로 끝나면 이 줄을 다시, 최대 20번\n")
	fmt.Fprint(w, "   # 병합기가 켜진 저장소면 이 줄은 큐에 제출만 하고 끝난다(「제출 … 이 레인은 여기서 끝」) — 착지는 병합기가 알리고, report 착지는 적히지 않는다. 아래 clean은 큐 표식(refs/merge-queue/*-레인이름)에 그대로 있는 가지를 「제출됨」으로 지워 자리를 바로 비운다\n")
	s(fmt.Sprintf("cd %s && hc-lane clean --repo %s --branch %s", q(repo), q(repo), branch))
	fmt.Fprint(w, "   # teardown·검사·워크트리·브랜치 정리를 한 번에. clean이 종료 1로 멈추면 출력된 이유를 고치고 같은 줄을 다시 돈다\n")
	fmt.Fprint(w, "   # 폴더를 rm -rf로 직접 지우지 않는다. 보고는 출력 그대로, 건너뜀은 통과가 아니다\n")
	fmt.Fprintf(w, "   # 끝내기 전 이 작업 트리 아래에서 도는 배경 프로세스가 없는지 확인한다(pgrep -fl %s). 자기 셸·에이전트는 빼고, 남은 것은 그 정확한 pid로 kill 한다. cwd로만 도는 것(명령 줄에 경로 없음)은 pgrep에 안 보인다\n", q(wt+"/"))
}

func readSteps(w io.Writer) {
	fmt.Fprint(w, "### 판정 알림 (hc-lane이 채움)\n")
	rmRule(w)
	fmt.Fprint(w, "- 판정이 나오면 보고 파일과 함께 이 한 줄을 부른다: hc-lane report 판정 <대상 커밋 40자 sha, 커밋이 아니면 -> \"<판정 한 줄(예: 감사 완료 높음 0·중간 0·낮음 n)>\". 위 지시문이 판정 줄 꼴을 정했으면 그 꼴이 앞선다\n")
	fmt.Fprint(w, "- 막히면 hc-lane report 막힘 - \"<까닭>\". 지휘는 hc-lane wait로 이 사건만 기다린다\n")
}

func anySteps(w io.Writer) {
	if read {
		readSteps(w)
	} else {
		printSteps(w)
	}
}

func isDigits(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

func launchMain(args []string) int {
	startSetup() // 배경 첫 턴 감시(__turn-check)도 사건을 셸 판 경로로 부른다
	if len(args) > 0 && args[0] == "__turn-check" {
		return turnCheckChild()
	}
	rest := parseStart("launch", args)
	checkArgs("launch", rest)
	rdir := resolveRepo("launch")
	selfTerm = os.Getenv("HC_TERMINAL_ID")
	if steps {
		anySteps(os.Stdout)
		return 0
	}
	fill := os.Getenv("HC_LANE_FILL") == "1"
	tid := os.Getenv("HC_TERMINAL_ID")
	// 대기열만: 지휘 터미널이 쓰기 레인을 HC_LANE_FILL=1·--direct 없이 띄우면 아무것도 만들기 전에 거절(종료 2)
	if !read && !dry && !fill {
		if direct {
			errf("hc-lane: 대기열 밖에서 띄움(--direct)\n")
		} else if os.Getenv("HC_LANE") == "" && tid != "" && coordIs(tid) {
			errf("hc-lane: 쓰기 레인은 hc-fill add로 대기열에 넣는다 — 자리가 나면 기계 차례대로 이어 뜬다(hc-fill add --name <레인> --project <프로젝트> --brief <지시문> [--after …] [--priority …] [--only <기계>]). 지금 꼭 이 자리에 띄워야 하면 --direct\n")
			return 2
		}
	}
	// 살아 있는 터미널과 이름이 겹치면 hc run이 실패하니 아무것도 만들기 전에 막는다
	if live, _ := outOf(nil, []string{"hc", "get", onp(name), "--json"}); strings.Contains(live, `"name":"`+name+`"`) && strings.Contains(live, `"state":"live"`) {
		die("살아 있는 hc 터미널이 이미 이름 %s 을 쓴다(hc ls). 다른 --name 을 쓴다", name)
	}
	if swarg != "" {
		swait = swarg
		if !isDigits(swait) {
			die("--start-wait는 0 이상 정수(초): %s", swait)
		}
	} else {
		swait = cfg("lane.start_wait", "60")
		if !isDigits(swait) {
			die("설정 lane.start_wait는 0 이상 정수(초): %s", swait)
		}
	}
	strust = cfg("lane.auto_trust", "on")
	if strust != "on" && strust != "off" {
		die("설정 lane.auto_trust는 on·off: %s", strust)
	}
	fw := cfg("lane.first_turn_wait", "60")
	if !isDigits(fw) {
		die("설정 lane.first_turn_wait는 0 이상 정수(초): %s", fw)
	}
	fwait, _ = strconv.ParseInt(fw, 10, 64) // 08처럼 앞 0이 붙어도 십진수로
	var nn []string
	switch n := cfg("lane.notify", "yes"); n {
	case "yes":
	case "no":
		nn = []string{"--no-notify"}
	default:
		die("설정 lane.notify는 yes·no: %s", n)
	}
	if fill {
		nn = append(nn, "--no-parent")
	}
	poll = os.Getenv("HC_LANE_POLL")
	if poll == "" {
		poll = "2"
	}
	if brief != "" {
		if st, err := os.Stat(brief); err != nil || !st.Mode().IsRegular() {
			die("지시문 파일이 없다: %s", brief)
		}
		if len(rest) == 0 {
			tpl := cfg("lane.run", "")
			if tpl == "" {
				die("-- 뒤 실행 명령도 설정 lane.run(실행 명령 틀)도 없다")
			}
			rest = shellWords(tpl)
		}
		if bdir := cfg("lane.brief_dir", ""); bdir != "" {
			runBrief = strings.TrimSuffix(bdir, "/") + "/" + name + "-run.md"
		} else {
			bd, err := filepath.Abs(filepath.Dir(brief))
			if err == nil {
				bd, err = filepath.EvalSymlinks(bd)
			}
			if err != nil {
				die("지시문 폴더를 찾지 못했다: %s", brief)
			}
			runBrief = bd + "/" + strings.TrimSuffix(filepath.Base(brief), ".md") + "-run.md"
		}
	}
	k := false
	for _, a := range rest {
		if strings.Contains(a, "{brief}") {
			k = true
		}
	}
	if brief != "" && !k {
		die("실행 명령에 {brief} 자리가 없다(지시문 사본 경로를 넣을 곳)")
	}
	if brief == "" && k {
		die("실행 명령에 {brief} 자리가 있는데 --brief <지시문>이 없다")
	}

	if read {
		cwd, wt, branch = rdir, "", ""
		// 읽기 레인도 메인 폴더를 앞당긴다. fetch가 실패해도 레인은 띄운다
		if !dry && !norepo && remote {
			if xfetch(repo, "-q", "origin") == 0 {
				ffmain(repo, base)
			} else {
				errf("hc-lane: 경고 — 메인 폴더 앞당김 건너뜀: git fetch 실패 — 그대로 둠 (%s)\n", atRepo())
			}
		}
	} else {
		cwd = wt
		if !dry && remote && xfetch(repo, "-q", "origin") != 0 {
			die("git fetch 실패: %s", repo)
		}
		again := "다른 이름은 --branch <이름>"
		if xrun(nil, os.Stderr, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/heads/"+branch) == 0 {
			die("로컬 브랜치 %s 가 이미 있다. %s", branch, again)
		}
		if remote && xrun(nil, os.Stderr, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/remotes/origin/"+branch) == 0 {
			die("원격 브랜치 origin/%s 가 이미 있다. %s", branch, again)
		}
		if xtest("-e", wt) {
			die("워크트리 폴더가 이미 있다: %s. %s", wt, again)
		}
		if xrun(nil, os.Stderr, "git", "-C", repo, "rev-parse", "-q", "--verify", bref+"^{commit}") != 0 {
			die("%s 가 없다: %s", blab, repo)
		}
		if !dry && remote {
			ffmain(repo, base)
		}
	}
	// --resume: park가 원격 refs/park/<레인>에 둔 가지를 직접 받는다(제출 가지 lane/<레인>과 다른 이름 — 병합기·clean이 헷갈리지 않게).
	// 옛 판이 주차한 lane/<레인>은 refs/park/<레인>이 없을 때만 받는다(바꾼 날 주차해 둔 레인)
	wfrom := bref
	if resume {
		if !remote {
			die("--resume: 원격(origin)이 없는 저장소에는 주차한 가지가 없다: %s", repo)
		}
		wfrom = "refs/hc-park/" + name
		// 모의 실행도 원격을 본다(읽기만 — 참조를 쓰지 않고 시작점을 알린다, gofix L08)
		ls, r := xout("git", "-C", repo, "ls-remote", "origin", "refs/park/"+name, "refs/heads/lane/"+name)
		if r != 0 {
			die("--resume: 원격을 읽지 못했다(git ls-remote) — 주차한 가지 refs/park/%s 를 모른다: %s", name, repo)
		}
		// 응답 줄은 「<40자 SHA><탭><참조>」만 받는다 — 깨진 줄은 주차 SHA가 아니다(R-15)
		got := map[string]string{}
		for _, ln := range strings.Split(ls, "\n") {
			if ln == "" {
				continue
			}
			s, rf, ok := strings.Cut(ln, "\t")
			if !ok || !sha40.MatchString(s) || strings.ContainsAny(rf, " \t") {
				die("--resume: 원격 응답을 읽지 못했다(git ls-remote 줄 꼴: %q) — 주차한 가지를 모른다: %s", cut(ln, 80), repo)
			}
			got[rf] = s
		}
		src, ssha := "", ""
		if s, ok := got["refs/park/"+name]; ok {
			src, ssha = "refs/park/"+name, s
		} else if s, ok := got["refs/heads/lane/"+name]; ok {
			// 옛 판 주차: 같은 레인·SHA의 마지막 주차·제출 사건이 「주차」일 때만 — 반송된 옛 제출 가지에서 재개하지 않는다(R-15)
			if !legacyParked(name, s) {
				die("--resume: refs/park/%s 가 없고 lane/%s(%s)는 주차 사건이 없는 제출 가지다 — 재개하지 않는다: %s", name, name, cut(s, 12), repo)
			}
			src, ssha = "refs/heads/lane/"+name, s
		}
		switch {
		case src == "":
			die("--resume: 주차한 가지 refs/park/%s 가 없다: %s", name, repo)
		case dry:
			fmt.Printf("[dry-run] 주차한 가지 %s (%s) 에서 시작\n", src, cut(ssha, 12))
		case xfetch(repo, "-q", "origin", "+"+src+":"+wfrom) != 0:
			die("--resume: 주차한 가지 refs/park/%s 를 받지 못했다: %s", name, repo)
		default:
			// 받은 것이 판정한 그 SHA여야 한다(그사이 움직였으면 판정이 맞지 않는다)
			if v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", wfrom+"^{commit}"); r != 0 || v != ssha {
				die("--resume: 받은 주차 가지 %s 가 판정한 %s 와 다르거나 없다 — 같은 줄을 다시: %s", src, cut(ssha, 12), repo)
			}
		}
	}

	// --brief: 지시문(+착지·정리 절차)을 대상 기계 사본 자리에 두고, 실행 명령의 {brief}를 그 경로로
	if brief != "" {
		if dry {
			what := "착지·정리 절차"
			if read {
				what = "판정 알림"
			}
			fmt.Printf("[dry-run] 지시문 사본 %s%s ← %s + %s\n", onc(), runBrief, brief, what)
		} else {
			body, err := os.ReadFile(brief)
			if err != nil {
				die("지시문 파일이 없다: %s", brief)
			}
			var in bytes.Buffer
			in.Write(body)
			in.WriteString("\n")
			anySteps(&in)
			got, r := xi(&in, "sh", "-c", `f=$1
      case $f in "~"/*) f=$HOME/${f#"~/"} ;; esac
      mkdir -p "${f%/*}" && cat >"$f" && printf %s "$f"`, "hc-lane", runBrief)
			if r != 0 || got == "" {
				hint := ""
				if on != "" {
					hint = " (그 기계에 그 경로를 쓸 수 없으면 설정 lane.brief_dir)"
				}
				die("지시문 사본을 %s%s 에 두지 못했다%s", onOf(), runBrief, hint)
			}
			runBrief = got
		}
		for i, a := range rest {
			rest[i] = strings.ReplaceAll(a, "{brief}", runBrief)
		}
	}
	envs := []string{"HC_LANE=" + name, "LANE_REPO=" + repo}
	if read {
		envs = append(envs, "LANE_BASE="+base)
	} else {
		envs = append(envs, "LANE_WORKTREE="+wt, "LANE_BRANCH="+branch, "LANE_BASE="+base)
	}
	// 레인 안에서도 주인 지휘를 알게(사건 11칸). 지휘 터미널에서 바로 띄우면 그 지휘, 모르면 *(주인 없음)
	lenvO := os.Getenv("HC_LANE_OWNER")
	if lenvO == "" && os.Getenv("HC_LANE_COORD") == "" && tid != "" && !dry && coordIs(tid) {
		lenvO = tid
	}
	lenvR := ""
	if lenvO != "" && lenvO != "*" {
		if lenvO == os.Getenv("HC_LANE_OWNER") && os.Getenv("HC_LANE_OWNER_RUN") != "" {
			lenvR = os.Getenv("HC_LANE_OWNER_RUN")
		} else if !dry {
			if g, r := captP(10, "hc", "get", lenvO, "--json"); r == 0 {
				lenvR, _ = liveCreatedMs(g)
			}
		}
	}
	if lenvO == "" {
		lenvO = "*"
	}
	pre := []string{"HC_LANE_OWNER=" + lenvO}
	if lenvR != "" {
		pre = append(pre, "HC_LANE_OWNER_RUN="+lenvR)
	}
	envs = append(pre, envs...)
	if jb := os.Getenv("HC_LANE_JOB"); jb != "" {
		if strings.Trim(jb, "abcdefghijklmnopqrstuvwxyz0123456789~-") != "" {
			die("HC_LANE_JOB 글자(a-z0-9~-): %s", jb)
		}
		envs = append([]string{"HC_LANE_JOB=" + jb}, envs...)
	}
	if p := os.Getenv("HERDR_LAND_PRIORITY"); p != "" {
		envs = append([]string{"HERDR_LAND_PRIORITY=" + p}, envs...)
	}
	// 제출 우선(hc-fill이 대기열 줄의 우선으로 넣음): 레인의 hc-lane land가 큐 메타 priority=로 싣는다
	if p := os.Getenv("HC_LANE_PRIORITY"); p != "" {
		envs = append([]string{"HC_LANE_PRIORITY=" + p}, envs...)
	}

	// 다른 기계 레인은 맥 Herdr 창의 그 기계 공간에 탭으로(hc run --label herdr.space=<id>)
	if on != "" {
		hsp = "lanes"
		if out, r := outOf(nil, []string{"herdr-app", "ls"}); r == 0 {
			var v struct {
				Spaces []struct {
					ID      string  `json:"id"`
					Name    *string `json:"name"`
					Machine *string `json:"machine"`
				} `json:"spaces"`
			}
			if json.Unmarshal([]byte(out), &v) == nil {
				for _, s := range v.Spaces {
					if s.Machine != nil && *s.Machine == on && s.Name != nil && *s.Name == on {
						hsp = strings.TrimPrefix(s.ID, on+"/")
						break
					}
				}
			}
		}
	}
	runArgs := func() []string {
		a := append([]string{"hc", "run", "--bg", "--json"}, nn...)
		a = append(a, "--name", name, "--label", "hc-lane=1")
		if hsp != "" {
			a = append(a, "--label", "herdr.space="+hsp)
		}
		a = append(a, "--cwd", cwd)
		if on != "" {
			a = append(a, "--on", on)
		}
		return a
	}
	code := 0
	if dry {
		if !norepo && remote {
			fmt.Printf("[dry-run] git -C %s fetch -q origin\n", q(repo))
			fmt.Printf("[dry-run] 메인 폴더 앞당김(기본 브랜치 %s·추적 변경 없음·merge/rebase 없을 때만): git -C %s merge --ff-only %s\n", base, q(repo), bref)
		}
		if !read {
			tk := "--track "
			if resume {
				tk = ""
			}
			fmt.Printf("[dry-run] git -C %s worktree add %s-b %s %s %s\n", q(repo), tk, branch, q(wt), wfrom)
			fmt.Printf("[dry-run] (%s 에 .claude/herdr-setup.sh 가 있으면) 워크트리에서 bash .claude/herdr-setup.sh\n", blab)
			fmt.Printf("[dry-run] 임대 잡기 refs/hc-lanes/%s (없을 때만 — 이미 있으면 종료 10, 원격 없음이면 임대 없이)\n", name)
		}
		line := "[dry-run] hc run --bg --json"
		for _, n := range nn {
			line += " " + n
		}
		line += " --name " + name + " --label hc-lane=1"
		if hsp != "" {
			line += " --label herdr.space=" + hsp
		}
		line += " --cwd " + q(cwd)
		if on != "" {
			line += " --on " + on
		}
		for _, a := range append(append([]string{"env"}, envs...), rest...) {
			line += " " + q(a)
		}
		fmt.Println(line)
		anySteps(os.Stdout)
		autoWatch()
	} else {
		if !read {
			// --track: upstream을 붙여 착지 뒤 branch -d가 통한다. 주차한 가지(가지가 아닌 참조)에서는 아래에서 기본 브랜치로 붙인다
			wa := []string{"git", "-C", repo, "worktree", "add", "-q", "--track", "-b", branch, wt, wfrom}
			if resume {
				wa = []string{"git", "-C", repo, "worktree", "add", "-q", "-b", branch, wt, wfrom}
			}
			if xrun(os.Stdout, os.Stderr, wa...) != 0 {
				die("워크트리를 만들지 못했다: %s", wt)
			}
			if resume && xrun(os.Stdout, os.Stderr, "git", "-C", wt, "branch", "-q", "--set-upstream-to="+blab, branch) != 0 {
				die("--resume: upstream을 %s 로 두지 못했다. %s", blab, rollback())
			}
			// rerere: 저장소 설정에 쓰지 않고 공용 git 폴더에 rr-cache만 만든다(정해 둔 rerere.enabled는 그대로)
			if xrun(os.Stdout, os.Stderr, "sh", "-c", `d=$(git -C "$1" rev-parse --path-format=absolute --git-common-dir) && mkdir -p "$d/rr-cache"`, "_", repo) != 0 {
				errf("hc-lane: rerere 폴더(rr-cache)를 만들지 못했다(착지는 그대로 된다): %s\n", repo)
			}
			if xtest("-f", wt+"/.claude/herdr-setup.sh") {
				var r int
				if r, logPath = hook("setup"); r != 0 {
					tailLog(logPath)
					die("herdr-setup.sh 실패, 레인을 띄우지 않았다. %s", rollback())
				}
			}
		}
		// 레인 임대: 쓰기 레인은 hc run 직전에 원격 refs/hc-lanes/<레인>을 없을 때만 만든다(모르면 안 띄움)
		lsha := ""
		if !read {
			who := on
			if who == "" {
				if h, r := outOf(nil, []string{"hostname", "-s"}); r == 0 {
					who = h
				} else {
					who = "-"
				}
			}
			leaseBranch = branch // 임대 주인 표식 refs/hc-lease-owners/<가지>(셸 lease_x의 ${branch-})
			lo, lr := leaseX("take", name, who)
			switch lr {
			case 0:
				lsha = lo
			case 4:
				if lo != "no-origin" {
					errf("hc-lane: 임대를 잡지 못했다(종료 4, 원격 없음 확인 못 함 — 기계 조회 실패일 수 있다). 레인을 띄우지 않았다. %s\n", rollback())
					return 1
				}
			case 3:
				onf := ""
				if on != "" {
					onf = " --on " + on
				}
				errf("hc-lane: 이미 돎 — refs/hc-lanes/%s 를 다른 실행이 쥐고 있다(%s). 레인을 띄우지 않았다(그 실행이 끝났으면 hc-lane lease drop%s --repo %s --name %s). %s\n", name, lo, onf, q(repo), name, rollback())
				return 10
			default:
				errf("hc-lane: 임대를 잡지 못했다(git ls-remote·push origin 실패, 종료 %d). 레인을 띄우지 않았다 — 원격이 닿으면 다시. %s\n", lr, rollback())
				return 1
			}
			envs = append([]string{"HC_LANE_LEASE=" + lsha}, envs...)
		}
		argv := append(append(append(runArgs(), "env"), envs...), rest...)
		var ob, eb bytes.Buffer
		c := exec.Command(argv[0], argv[1:]...)
		c.Stdout, c.Stderr = &ob, &eb
		// 배경 시작(hc run --bg)도 상한 안에서(proc.Limit — 재감사 R-19: c.Run()이라 hc가 멈추면 끝없이 기다렸다). 넘기면 시작했을 수
		// 있어 미확정 — 아래 실패 갈래가 임대·롤백 안내를 그대로 낸다
		if err := proc.Run(c); err != nil {
			out := trim(ob.String() + "\n" + eb.String())
			errf("%s\n", out)
			leaseNote := ""
			if lsha != "" {
				leaseNote = " 임대 refs/hc-lanes/" + name + " 은 hc-lane lease show로 확인한다"
			}
			if runRefused(out) {
				rb := ""
				if wt != "" {
					rb = " " + rollback()
				}
				die("hc run이 실행을 거절했다.%s%s", rb, leaseNote)
			}
			kept := ""
			if wt != "" {
				kept = " 작업 트리 " + wt + " 를 보존했다."
			}
			if lsha != "" {
				kept += " 임대 refs/hc-lanes/" + name + " 도 보존했다."
			}
			// 거절이 확인되지 않은 실패는 실행됐을 수 있다 — 확정 실패(1)가 아니라 미확정(5): hc-fill이 그 기계·시도 기록을 지킨다(R-18)
			errf("hc-lane: hc run 결과를 확인하지 못했다(실행 미확정).%s hc ls --json으로 실행을 확인한다\n", kept)
			exit(5)
		}
		if s := trim(eb.String()); s != "" {
			errf("%s\n", s)
		}
		out := trim(ob.String())
		addr = firstAddr(out)
		if addr == "" {
			// 실행됐는지 모른다 — 성공(0)으로 끝내지 않는다. 임대·작업 트리는 그대로 둔다(gofix E09: hc-fill 밖에서 부른 쪽도 실패를 본다)
			errf("hc-lane: hc run 출력에서 주소를 읽지 못했다(hc ls --json 으로 확인 — 임대·작업 트리는 보존): %s\n", out)
			code = 5
		}
		// 레인 기록: 이 레인의 지휘. HC_LANE_OWNER가 앞, 아니면 지휘 터미널에서 띄웠을 때 그 지휘, 모르면 -
		loOwner = "-"
		if o := os.Getenv("HC_LANE_OWNER"); o != "" {
			loOwner = o
		} else if os.Getenv("HC_LANE_COORD") == "" && tid != "" && coordSet(tid, true) {
			loOwner = tid
		}
		lk := ""
		switch {
		case read:
			lk = "\tread"
		case fill:
			lk = "\tfill"
		case direct:
			lk = "\tdirect"
		}
		if addr != "" {
			if err := appendLine(evdir()+"/lanes.tsv", addr+"\t"+name+"\t"+loOwner+lk+"\n"); err != nil {
				errf("hc-lane: 레인 기록을 쓰지 못했다: %s/lanes.tsv\n", evdir())
			}
		}
		autoWatch()
		showRemote()
		anySteps(os.Stdout)
		if swait == "0" {
			start = "off"
		} else if addr != "" {
			code = startCheck(addr)
		}
	}
	pre2 := ""
	if dry {
		pre2 = "[dry-run] 띄울 "
	}
	at := ""
	if addr != "" {
		at = "(" + addr + ")"
	}
	if on != "" {
		at += " @" + on
	}
	if read {
		fmt.Printf("%s레인 %s%s: 읽기 전용, 폴더 %s 에서 실행\n", pre2, name, at, cwd)
	} else {
		fmt.Printf("%s레인 %s%s: 워크트리 %s, 브랜치 %s, 기준 %s\n", pre2, name, at, wt, branch, blab)
	}
	fmt.Printf(`{"lane":"%s","name":"%s","on":"%s","read":%s,"dry_run":%s,"repo":"%s","worktree":"%s","branch":"%s","base":"%s","brief":"%s","start":"%s"}`+"\n",
		j(addr), name, j(on), tf(read), tf(dry), j(repo), j(wt), j(branch), j(base), j(runBrief), start)
	return code
}

func atRepo() string {
	if on != "" {
		return on + ":" + repo
	}
	return repo
}

func onc() string {
	if on != "" {
		return on + ":"
	}
	return ""
}

func onOf() string {
	if on != "" {
		return on + " 의 "
	}
	return ""
}

func appendLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
