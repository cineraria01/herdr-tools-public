package main

// 레인 띄우기(launchMain)의 단계들 — 차례는 launchMain이 정하고, 단계마다 입력·출력을 인자·반환으로 드러낸다.
// 판정만 하는 것(spaceFor·laneKind·notifyArgs·hasBriefSlot·jobOK)은 읽기·쓰기 없이 단위 시험(launchsteps_test.go).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"herdr-tools/internal/proc"
)

// queueOnly: 대기열만 — 지휘 터미널이 쓰기 레인을 HC_LANE_FILL=1·--direct 없이 띄우면 아무것도 만들기 전에 거절(true면 종료 2)
func queueOnly(fill bool, tid string) bool {
	if read || dry || fill {
		return false
	}
	if direct {
		errf("hc-lane: 대기열 밖에서 띄움(--direct)\n")
	} else if os.Getenv("HC_LANE") == "" && tid != "" && coordIs(tid) {
		errf("hc-lane: 쓰기 레인은 hc-fill add로 대기열에 넣는다 — 자리가 나면 기계 차례대로 이어 뜬다(hc-fill add --name <레인> --project <프로젝트> --brief <지시문> [--after …] [--priority …] [--only <기계>]). 지금 꼭 이 자리에 띄워야 하면 --direct\n")
		return true
	}
	return false
}

// liveNameCheck: 살아 있는 터미널과 이름이 겹치면 hc run이 실패하니 아무것도 만들기 전에 막는다
func liveNameCheck() {
	if live, _ := outOf(nil, []string{"hc", "get", onp(name), "--json"}); strings.Contains(live, `"name":"`+name+`"`) && strings.Contains(live, `"state":"live"`) {
		die("살아 있는 hc 터미널이 이미 이름 %s 을 쓴다(hc ls). 다른 --name 을 쓴다", name)
	}
}

// notifyArgs: 설정 lane.notify → hc run 알림 인자(fill이면 --no-parent 더함). 모르는 값이면 ok=false
func notifyArgs(v string, fill bool) ([]string, bool) {
	var nn []string
	switch v {
	case "yes":
	case "no":
		nn = []string{"--no-notify"}
	default:
		return nil, false
	}
	if fill {
		nn = append(nn, "--no-parent")
	}
	return nn, true
}

// launchSettings: 시작 기다림·신뢰 질문·첫 턴 기다림·알림·확인 간격(전역 swait·strust·fwait·poll). 반환은 hc run 알림 인자
func launchSettings(fill bool) []string {
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
	n := cfg("lane.notify", "yes")
	nn, ok := notifyArgs(n, fill)
	if !ok {
		die("설정 lane.notify는 yes·no: %s", n)
	}
	poll = os.Getenv("HC_LANE_POLL")
	if poll == "" {
		poll = "2"
	}
	return nn
}

// hasBriefSlot: 실행 명령에 {brief} 자리가 있나
func hasBriefSlot(rest []string) bool {
	for _, a := range rest {
		if strings.Contains(a, "{brief}") {
			return true
		}
	}
	return false
}

// briefCommand: --brief면 지시문을 확인하고 실행 명령(없으면 설정 lane.run 틀)과 사본 자리(전역 runBrief)를 정한다. {brief} 자리와 --brief가 짝이 맞아야
func briefCommand(rest []string) []string {
	if brief != "" {
		if st, err := os.Stat(brief); err != nil || !st.Mode().IsRegular() {
			refuse("지시문 파일이 없다: %s", brief)
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
	k := hasBriefSlot(rest)
	if brief != "" && !k {
		refuse("실행 명령에 {brief} 자리가 없다(지시문 사본 경로를 넣을 곳)")
	}
	if brief == "" && k {
		refuse("실행 명령에 {brief} 자리가 있는데 --brief <지시문>이 없다")
	}
	return rest
}

// prepRepo: 실행 폴더(전역 cwd)를 정하고 메인 폴더를 앞당긴다. 쓰기 레인은 가지·원격 가지·작업 트리 폴더가 없고 기준이 있어야
func prepRepo(rdir string) {
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
		return
	}
	cwd = wt
	if !dry && remote && xfetch(repo, "-q", "origin") != 0 {
		die("git fetch 실패: %s", repo)
	}
	if revive { // 있어야 할 작업 트리·가지는 reviveGate가 봤다
		if !dry && remote {
			ffmain(repo, base)
		}
		return
	}
	if resume && !dry {
		parkLeftClean() // 주차 뒤 남은 작업 트리(launchresume.go)
	}
	again := "다른 이름은 --branch <이름>"
	// 이미 있나: 0 있음, 1 없음만 잇는다. 그 밖(hc 실패 125·시간 넘김)은 모름 — 띄우지 않는다(hc-exec-result)
	there := func(what string, args ...string) bool {
		switch r := xrun(nil, os.Stderr, args...); r {
		case 0:
			return true
		case 1:
			return false
		default:
			die("%s 가 이미 있는지 확인하지 못했다(종료 %d) — 띄우지 않았다", what, r)
			return false
		}
	}
	if there("로컬 브랜치 "+branch, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/heads/"+branch) {
		refuse("로컬 브랜치 %s 가 이미 있다. %s", branch, again)
	}
	if remote && there("원격 브랜치 origin/"+branch, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/remotes/origin/"+branch) {
		refuse("원격 브랜치 origin/%s 가 이미 있다. %s", branch, again)
	}
	if there("워크트리 폴더 "+wt, "test", "-e", wt) {
		refuse("워크트리 폴더가 이미 있다: %s. %s", wt, again)
	}
	if xrun(nil, os.Stderr, "git", "-C", repo, "rev-parse", "-q", "--verify", bref+"^{commit}") != 0 {
		die("%s 가 없다: %s", blab, repo)
	}
	if !dry && remote {
		ffmain(repo, base)
	}
}

// placeBrief: --brief면 지시문(+착지·정리 절차)을 대상 기계 사본 자리에 두고, 실행 명령의 {brief}를 그 경로로
func placeBrief(rest []string) []string {
	if brief == "" {
		return rest
	}
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
	return rest
}

// jobOK: HC_LANE_JOB 글자(a-z0-9~-)
func jobOK(jb string) bool {
	return strings.Trim(jb, "abcdefghijklmnopqrstuvwxyz0123456789~-") == ""
}

// laneEnvs: 레인 안에 줄 환경(env 인자)
func laneEnvs(tid string) []string {
	envs := []string{"HC_LANE=" + name, "LANE_REPO=" + repo}
	if read {
		envs = append(envs, "LANE_BASE="+base)
	} else {
		envs = append(envs, "LANE_WORKTREE="+wt, "LANE_BRANCH="+branch, "LANE_BASE="+base)
	}
	if reviveSess != "" { // 실행 명령 틀이 $HC_LANE_CONTINUE를 CLI 인자 자리에 둔다 — 한 낱말(zsh는 변수를 나누지 않는다, docs/hc-lane/revive.md)
		envs = append(envs, "HC_LANE_CONTINUE=--resume="+reviveSess)
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
		if !jobOK(jb) {
			die("HC_LANE_JOB 글자(a-z0-9~-): %s", jb)
		}
		envs = append([]string{"HC_LANE_JOB=" + jb}, envs...)
	}
	if pred, p := stackEnv(); p != "" { // 쌓아 띄움 — 레인의 land·verdict가 범위 P..HEAD로(stack.go)
		envs = append([]string{"HC_LANE_STACK=" + pred + "@" + p}, envs...)
	}
	if p := os.Getenv("HERDR_LAND_PRIORITY"); p != "" {
		envs = append([]string{"HERDR_LAND_PRIORITY=" + p}, envs...)
	}
	// 제출 우선(hc-fill이 대기열 줄의 우선으로 넣음): 레인의 hc-lane land가 큐 메타 priority=로 싣는다
	if p := os.Getenv("HC_LANE_PRIORITY"); p != "" {
		envs = append([]string{"HC_LANE_PRIORITY=" + p}, envs...)
	}
	return envs
}

// spaceFor: herdr-app ls 출력에서 기계 on의 공간 id(「<기계>/」 뺌). 못 찾거나 못 읽으면 lanes
func spaceFor(lsOut, on string) string {
	var v struct {
		Spaces []struct {
			ID      string  `json:"id"`
			Name    *string `json:"name"`
			Machine *string `json:"machine"`
		} `json:"spaces"`
	}
	if json.Unmarshal([]byte(lsOut), &v) == nil {
		for _, s := range v.Spaces {
			if s.Machine != nil && *s.Machine == on && s.Name != nil && *s.Name == on {
				return strings.TrimPrefix(s.ID, on+"/")
			}
		}
	}
	return "lanes"
}

// herdrSpace: 다른 기계 레인의 Herdr 공간(herdr-app ls 실패면 lanes)
func herdrSpace() string {
	out, r := outOf(nil, []string{"herdr-app", "ls"})
	if r != 0 {
		return "lanes"
	}
	return spaceFor(out, on)
}

// runArgs: hc run 앞 인자(env·실행 명령 앞까지)
func runArgs(nn []string) []string {
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

// launchDry: --dry-run — 할 일을 줄로만
func launchDry(nn, envs, rest []string, wfrom string) {
	if !norepo && remote {
		fmt.Printf("[dry-run] git -C %s fetch -q origin\n", q(repo))
		fmt.Printf("[dry-run] 메인 폴더 앞당김(기본 브랜치 %s·추적 변경 없음·merge/rebase 없을 때만): git -C %s merge --ff-only %s\n", base, q(repo), bref)
	}
	if revive {
		fmt.Printf("[dry-run] 이어 띄움: 작업 트리 %s%s 그대로(만들기·설정 훅·실어 온 자료 풀기 건너뜀), 임대 refs/hc-lanes/%s 이어 씀(%s)\n", onc(), q(wt), name, cut(reviveLease, 12))
	} else if !read {
		tk := ""
		if wfrom == bref {
			tk = "--track "
		}
		fmt.Printf("[dry-run] git -C %s worktree add %s-b %s %s %s\n", q(repo), tk, branch, q(wt), wfrom)
		fmt.Printf("[dry-run] (%s 에 .claude/herdr-setup.sh 가 있으면) 워크트리에서 bash .claude/herdr-setup.sh\n", blab)
		if carry != "" {
			fmt.Printf("[dry-run] 실어 보낼 묶음 %s 를 %s%s 에 풂(tar -xf)\n", carry, onc(), q(wt))
		}
		fmt.Printf("[dry-run] 임대 잡기 refs/hc-lanes/%s (없을 때만 — 이미 있으면 종료 11, 원격 없음이면 임대 없이)\n", name)
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
}

// makeWorktree: 쓰기 레인의 작업 트리·upstream·rr-cache·준비 훅(전역 logPath)
func makeWorktree(wfrom string) {
	wstart = wfrom
	// upstream을 기본 브랜치로 붙여 착지 뒤 branch -d가 통한다. 시작이 기본 브랜치면 --track, 가지가 아닌 시작(주차 가지 refs/hc-park/·
	// 쌓아 띄움의 커밋 P)은 --track이 거절하니(「starting point … is not a branch」) 만든 뒤 --set-upstream-to로 — 한 길
	track := wfrom == bref
	wa := []string{"git", "-C", repo, "worktree", "add", "-q", "-b", branch, wt, wfrom}
	if track {
		wa = []string{"git", "-C", repo, "worktree", "add", "-q", "--track", "-b", branch, wt, wfrom}
	}
	if xrun(os.Stdout, os.Stderr, wa...) != 0 {
		die("워크트리를 만들지 못했다: %s", wt)
	}
	if !track && xrun(os.Stdout, os.Stderr, "git", "-C", wt, "branch", "-q", "--set-upstream-to="+blab, branch) != 0 {
		die("upstream을 %s 로 두지 못했다(시작 %s). %s", blab, wfrom, rollback())
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

// takeLease: 쓰기 레인은 hc run 직전에 원격 refs/hc-lanes/<레인>을 없을 때만 만든다(모르면 안 띄움).
// 반환: 임대 sha(원격 없음이면 빈 값), 띄우지 않고 끝낼 종료 코드(0이면 잇는다)
func takeLease() (string, int) {
	leaseBranch = branch // 임대 주인 표식 refs/hc-lease-owners/<가지>(셸 lease_x의 ${branch-})
	lo, lr := leaseX("take", name, leaseWho())
	switch lr {
	case 0:
		return lo, 0
	case 4:
		if lo != "no-origin" {
			errf("hc-lane: 임대를 잡지 못했다(종료 4, 원격 없음 확인 못 함 — 기계 조회 실패일 수 있다). 레인을 띄우지 않았다. %s\n", rollback())
			return "", 1
		}
		return "", 0
	case 3: // 다시 해도 같음(launchaux.go rcRefuse) — 그 임대 값이 바뀌거나 없어질 때까지
		onf := ""
		if on != "" {
			onf = " --on " + on
		}
		sha, sub, _ := strings.Cut(lo, " ")
		errf("%s%s\n", leaseWatchPre, sha)
		errf("hc-lane: 이미 돎 — refs/hc-lanes/%s 를 %s. 레인을 띄우지 않았다(그 실행이 끝났으면 hc-lane lease drop%s --repo %s --name %s). %s\n", name, leaseHolder(sub), onf, q(repo), name, rollback())
		return "", rcRefuse
	default:
		errf("hc-lane: 임대를 잡지 못했다(git ls-remote·push origin 실패, 종료 %d). 레인을 띄우지 않았다 — 원격이 닿으면 다시. %s\n", lr, rollback())
		return "", 1
	}
}

// leaseWho: 임대 제목의 기계 칸 — --on이면 그 기계, 아니면 이 기계 hostname -s(모르면 -)
func leaseWho() string {
	if on != "" {
		return on
	}
	if h, r := outOf(nil, []string{"hostname", "-s"}); r == 0 {
		return h
	}
	return "-"
}

// hcRunLane: hc run --bg를 상한 안에서 돌려 표준 출력을 돌려준다. 실패면 거절(종료 1)·미확정(종료 5)으로 끝낸다
func hcRunLane(argv []string, lsha string) string {
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
	return trim(ob.String())
}

// laneKind: 레인 기록(lanes.tsv) 넷째 칸(탭 포함) — 읽기·대기열·직접, 그 밖은 없음
func laneKind(read, fill, direct bool) string {
	switch {
	case read:
		return "\tread"
	case fill:
		return "\tfill"
	case direct:
		return "\tdirect"
	}
	return ""
}

// recordLane: 레인 기록 — 이 레인의 지휘(전역 loOwner). HC_LANE_OWNER가 앞, 아니면 지휘 터미널에서 띄웠을 때 그 지휘, 모르면 -
func recordLane(tid string, fill bool) {
	loOwner = "-"
	if o := os.Getenv("HC_LANE_OWNER"); o != "" {
		loOwner = o
	} else if os.Getenv("HC_LANE_COORD") == "" && tid != "" && coordSet(tid, true) {
		loOwner = tid
	}
	lk := laneKind(read, fill, direct)
	if addr == "" {
		return
	}
	if err := appendLine(evdir()+"/lanes.tsv", addr+"\t"+name+"\t"+loOwner+lk+"\n"); err != nil {
		errf("hc-lane: 레인 기록을 쓰지 못했다: %s/lanes.tsv\n", evdir())
	}
	// 그 지휘가 이 저장소를 다룬다(watch-project-route — 주인 없는 레인 사건을 그 저장소 지휘에게만). 지휘 정보가 이 기계에 있을 때만
	if _, ok := readCoord(coordFile(loOwner)); ok && loOwner != "-" && loOwner != "*" {
		if u, rc := xoutQ("git", "-C", repo, "config", "--get", "remote.origin.url"); rc == 0 {
			coordRepoAdd(loOwner, repoID(u))
		}
	}
}

// launchRun: 실제로 띄운다 — 작업 트리 → 임대 → hc run → 레인 기록·감시·시작 확인. ok=false면 띄우지 않았다(code로 끝낸다)
func launchRun(nn, envs, rest []string, wfrom, tid string, fill bool) (code int, ok bool) {
	if revive { // 만들기·훅·풀기·임대 잡기 없이 — 임대는 reviveGate가 본 값을 이어 쓴다
		envs = append([]string{"HC_LANE_LEASE=" + reviveLease}, envs...)
		return launchHC(nn, envs, rest, reviveLease, tid, fill), true
	}
	if !read {
		makeWorktree(wfrom)
		carryIn()
		lsha, rc := takeLease()
		if rc != 0 {
			return rc, false
		}
		envs = append([]string{"HC_LANE_LEASE=" + lsha}, envs...)
		return launchHC(nn, envs, rest, lsha, tid, fill), true
	}
	return launchHC(nn, envs, rest, "", tid, fill), true
}

// launchHC: hc run 뒤 — 주소·레인 기록·감시·원격 표시·절차·시작 확인
func launchHC(nn, envs, rest []string, lsha, tid string, fill bool) int {
	code := 0
	argv := append(append(append(runArgs(nn), "env"), envs...), rest...)
	out := hcRunLane(argv, lsha)
	addr = firstAddr(out)
	if addr == "" {
		// 실행됐는지 모른다 — 성공(0)으로 끝내지 않는다. 임대·작업 트리는 그대로 둔다(gofix E09: hc-fill 밖에서 부른 쪽도 실패를 본다)
		errf("hc-lane: hc run 출력에서 주소를 읽지 못했다(hc ls --json 으로 확인 — 임대·작업 트리는 보존): %s\n", out)
		code = 5
	}
	recordLane(tid, fill)
	autoWatch()
	showRemote()
	anySteps(os.Stdout)
	if swait == "0" {
		start = "off"
	} else if addr != "" {
		code = startCheck(addr)
	}
	return code
}

// launchSummary: 끝 두 줄(사람 줄 + JSON)
func launchSummary() {
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
}
