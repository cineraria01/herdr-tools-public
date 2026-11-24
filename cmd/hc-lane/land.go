package main

// land: 레인 작업 폴더에서 착지(셸 lib/hc-lane/land.sh land_cmd와 같은 순서·출력·종료 코드).
// 종료 코드: 0 착지(또는 이미 들어감·제출) · 1 멈춤(die) · 2 승인과 다름 · 3 rebase 충돌 · 4 push 거절 횟수 · 6 함께 실은 lease 참조가
// 바뀜(병합기) · 7 병합기 안 착지인데 전용 착지 스크립트가 생김 · 75 다시(병합기 켬 모름·임대 조회 실패·큐에서 다시)

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"herdr-tools/internal/tri"
)

func init() {
	register("land", landMain, can("land"))
	argSpecs["land"] = argSpec{
		head: func(a []string) bool { return len(a) > 0 && a[0] != "" && !strings.HasPrefix(a[0], "-") }, // <기본 브랜치>
		flags: map[string]ownFlag{
			"--force-old": {},
			"--verdict": {val: true, bad: func(v string) string {
				if v == "" || strings.Trim(v, alnum+"_-") != "" {
					return "--verdict 판정 ID 글자: " + v
				}
				return ""
			}},
		},
		check: func(o *opts, rds bool) *parseErr {
			if o.on+o.name+o.branch+o.repo != "" || !rds || len(o.rest) > 0 {
				return die1("land는 레인 작업 폴더 안에서 인자 없이(또는 기본 브랜치 하나만) 부른다: hc-lane land [<기본 브랜치>]")
			}
			return nil
		},
	}
}

// 착지 상태(셸 판의 같은 이름 변수)
var (
	base, lbase, lvd string
	remote           bool
	lforceOld        bool   // --force-old: 원격 lane/<레인>의 조상(옛 커밋)도 제출
	bref, blab       string // 착지 기준 참조와 그 이름(알림용)
	lpid, lorig      string // 승인 착지: 승인 변경 patch-id, 대조 원본 커밋
	lmb, lrrf        string // 승인 원본과 기준의 갈림점, rerere 기록 파일
	llog, qa0        string // 착지 로그, 검증 전 .qa 폴더 목록
	px, mgx          string // HERDR_LAND_PUSH_EXTRA, HERDR_MERGER
	lrrn             int    // rerere로 바뀐 경로 수(LRRN)
	mg               string // 병합기 기계
)

const lf = ".claude/herdr-land.sh"

func landMain(a []string) int {
	o := mustParse("land", a)
	lbase, lvd, lforceOld = o.head, o.own["--verdict"], o.own["--force-old"] != ""
	return landCmd()
}

func landCmd() int {
	timeoutExit = 75 // 바깥 명령 시간 넘김 뒤의 die는 「다시」(gofix M01)
	top, rc := gitOutQ("rev-parse", "--show-toplevel")
	if rc != 0 || os.Chdir(top) != nil {
		wd := os.Getenv("PWD")
		if wd == "" {
			wd, _ = os.Getwd()
		}
		die("land: git 작업 폴더 안에서 부른다(레인 워크트리): %s", wd)
	}
	os.Setenv("PWD", top)
	if lbase != "" {
		base = lbase
	} else if b, ok := originBase("."); ok {
		base = b
	} else {
		die("land: 원격·기본 브랜치 조회 실패")
	}
	if !plain(base) {
		die("land: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: %s", base)
	}
	// 병합기가 켜졌으면 큐에 제출한다. 켬 상태를 모르면 종료 75로 재시도를 기다린다
	if os.Getenv("HERDR_MERGER") != "1" {
		switch mgon() {
		case tri.Yes:
			submitCmd()
		case tri.Unknown:
			exit(75)
		case tri.No:
		}
	}
	// 이 착지의 인자는 이 착지만 쓴다 — 검증 자식에 물려주지 않는다
	px, mgx = os.Getenv("HERDR_LAND_PUSH_EXTRA"), os.Getenv("HERDR_MERGER")
	prHeadsTake() // PR 머리 가지(landprhead.go) — 병합기 안 착지만
	specTreeTake(mgx == "1")
	os.Unsetenv("HERDR_LAND_PUSH_EXTRA")
	os.Unsetenv("HERDR_MERGER")
	qa0 = strings.Join(qadirs(), "\n")
	llogInit()
	var ob, obl string
	switch gitRunQ("remote", "get-url", "origin") {
	case 0:
		ob, obl = "refs/remotes/origin/"+base, "origin/"+base
		if fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin") != 0 {
			die("land: git fetch 실패")
		}
	case 2:
		ob, obl = "refs/heads/"+base, "로컬 "+base
	default:
		die("land: git remote get-url 실패 — 착지 기준을 확인하지 못했다")
	}
	bref, blab, remote = ob, obl, ob != "refs/heads/"+base
	lpid = ""
	if pid := os.Getenv("HERDR_LAND_PATCHID"); lvd == "" && pid != "" {
		if pid != "-" && strings.Trim(pid, "0123456789abcdef-") != "" {
			die("land: HERDR_LAND_PATCHID 꼴: %s", pid)
		}
		lpid = pid
		v := os.Getenv("HERDR_LAND_VERDICT")
		if v == "" {
			v = "?"
		}
		fmt.Printf("hc-lane land: 승인 착지(판정 %s) — 변경 patch-id %s\n", v, cut(lpid, 12))
		var rc int
		if lorig, rc = gitOut("rev-parse", "--verify", "-q", "HEAD"); rc != 0 {
			die("land: HEAD를 읽지 못했다(git 실패) — 착지하지 않았다")
		}
		if s := os.Getenv("HERDR_LAND_SHA"); s != "" && lorig != s {
			eprintf("hc-lane land: 승인 착지인데 HEAD가 제출 SHA가 아니다(HEAD %s, 제출 %s) — 착지하지 않았다\n", cut(lorig, 8), cut(s, 8))
			exit(2)
		}
	} else if lvd != "" {
		lverdictStart()
		lorig = lvs
		lverdictRecover()
	}
	os.Unsetenv("HERDR_LAND_PATCHID")
	os.Unsetenv("HERDR_LAND_VERDICT")
	os.Unsetenv("HERDR_LAND_SHA")
	os.Unsetenv("HERDR_LAND_HOOK_REV") // 착지 훅에만 hc-lane이 정해 넘긴다(호출자 값이 내장 길 검증에 새지 않게)
	if lpid != "" {
		var rc int
		if lmb, rc = gitOut("merge-base", lorig, bref); rc != 0 {
			die("land: 승인 커밋과 %s 의 갈림점을 못 찾았다 — 착지하지 않았다", blab)
		}
		if lrrf, rc = gitOut("rev-parse", "--absolute-git-dir"); rc != 0 || lrrf == "" {
			die("land: git 폴더를 읽지 못했다 — 착지하지 않았다")
		}
		lrrf += "/hc-lane-rerere"
		lo, ok := lpatch(lmb, lorig)
		if !ok {
			die("land: 승인 원본 patch-id를 못 구했다(git 실패) — 착지하지 않았다")
		}
		if lo != lpid {
			eprintf("hc-lane land: 착지할 커밋(%s)이 승인 변경이 아니다(patch-id %s ≠ 승인 %s) — 착지하지 않았다. 지금 커밋으로 hc-lane report 완료(새 감사)\n", cut(lorig, 8), cut(lo, 12), cut(lpid, 12))
			exit(2)
		}
	}
	// 병합기의 전용 착지 훅은 기본 브랜치에서 읽은 blob으로 실행한다
	lb := func(rev string) (string, int) { return gitOut("ls-tree", "--format=%(objectname)", rev, "--", lf) }
	basehook, rc := lb(ob)
	if rc != 0 {
		die("land: %s 착지 훅 조회 실패", obl)
	}
	if mgx == "1" && basehook != "" {
		tmpd := os.Getenv("TMPDIR")
		if tmpd == "" {
			tmpd = "/tmp"
		}
		f, err := os.CreateTemp(tmpd, "hc-merger-land.*")
		if err != nil {
			die("land: 기본 브랜치의 %s 임시 파일 생성 실패", lf)
		}
		mls := f.Name()
		cleanups = append(cleanups, func() { os.Remove(mls) })
		if runCmd(cmdOpt{stdout: f, stderr: os.Stderr}, "git", "cat-file", "blob", basehook) != 0 {
			f.Close()
			die("land: 기본 브랜치의 %s 를 꺼내지 못했다", lf)
		}
		f.Close()
		fmt.Printf("hc-lane land: 병합기 — %s 판 %s 에 맡긴다\n", obl, lf)
		if lpid != "" {
			lverdictPrecheck()
		}
		brev, rc := gitOut("rev-parse", "--verify", "-q", ob+"^{commit}")
		if rc != 0 || brev == "" {
			die("land: %s 커밋 조회 실패 — 착지 훅을 실행하지 않았다", obl)
		}
		// HERDR_LAND_HOOK_REV: 지금 도는 착지 스크립트 blob이 나온 커밋(저장소 스크립트가 이 트리로 도구를 짓는다)
		argv := []string{"env", "HERDR_LAND=1", "HERDR_LAND_HOOK_REV=" + brev}
		if px != "" {
			argv = append(argv, "HERDR_LAND_PUSH_EXTRA="+px)
		}
		verifiedEnv("") // 저장소 착지 스크립트가 rebase 뒤 검증 스크립트에 그대로 물려준다(트리 대조는 검증 스크립트가)
		rc = lrun(append(argv, "bash", mls, base)...)
		if rc != 0 {
			lfail(rc)
		}
		exit(rc)
	}
	mb, rc1 := gitOut("merge-base", "HEAD", ob)
	headhook, forkhook := "", ""
	rc2, rc3 := 1, 1
	if rc1 == 0 {
		if headhook, rc2 = lb("HEAD"); rc2 == 0 {
			forkhook, rc3 = lb(mb)
		}
	}
	if rc1 != 0 || rc2 != 0 || rc3 != 0 {
		die("land: 착지 훅의 갈래점·커밋 조회 실패")
	}
	if headhook == forkhook && forkhook != basehook {
		lerr(fmt.Sprintf("▸ hc-lane land: 가지의 %s 가 %s 보다 낡음 → 먼저 rebase", lf, obl))
		if !tclean("먼저 rebase 전") {
			die("land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시")
		}
		if rb(ob) != 0 {
			lerr(fmt.Sprintf("hc-lane land: rebase 충돌 — 양쪽 내용을 살려 풀고 git add → git rebase --continue 한 뒤 착지 줄(hc-lane land %s)을 다시", base))
			exit(3)
		}
		if headhook, rc = lb("HEAD"); rc != 0 {
			die("land: rebase 뒤 착지 훅 조회 실패")
		}
	}
	if headhook != "" && !isFile(lf) {
		die("land: 커밋의 %s 가 작업 트리에 없다", lf)
	}
	if mgx == "1" && isFile(lf) {
		eprintf("hc-lane land: 기본 브랜치에 전용 착지 스크립트(.claude/herdr-land.sh)가 생겼다 — 병합기 MVP-1은 그 스크립트에 맡기지 않는다\n")
		exit(7)
	}
	if isFile(lf) {
		exit(landHook())
	}
	return landBuiltin()
}

// landHook: 저장소의 .claude/herdr-land.sh에 맡긴다(rebase·push는 그 스크립트가). 착지 성공 직후 메인 폴더를 앞당긴다
func landHook() int {
	fmt.Printf("hc-lane land: .claude/herdr-land.sh %s 에 맡긴다\n", base)
	if lpid != "" {
		lverdictPrecheck()
	}
	committedHook("HEAD", lf)
	// 전용 훅도 main·착지 표식·이 실행의 임대 삭제를 같은 원자 push에 싣는다
	lmr, lpx := "", ""
	if lmn, ok := lmname(); remote && ok {
		lmr = "refs/hc-landed/" + lmn + "/" + unixNow()
		lpx = "HEAD:" + lmr
		if lvd != "" {
			lpx += " HEAD:refs/hc-verdict/" + os.Getenv("HC_LANE") + "/" + lvd
		}
		if lease := os.Getenv("HC_LANE_LEASE"); lease != "" {
			lref := "refs/hc-lanes/" + lmn
			lh, rc := gitOut("ls-remote", "origin", lref)
			if rc != 0 {
				lerr("hc-lane land: 임대 조회 실패 — 착지 훅을 실행하지 않았다")
				exit(75)
			}
			if firstWord(lh) == lease {
				lpx += " :" + lref + " --force-with-lease=" + lref + ":" + lease
			}
		}
	}
	ax := lpx
	if px != "" {
		ax = px
		if lpx != "" {
			ax += " " + lpx
		}
	}
	lp := os.Getenv("HERDR_LAND_PRIORITY")
	if lp == "" || strings.Trim(lp, "0123456789") != "" {
		lp = "0"
	}
	hrev, rc := gitOut("rev-parse", "--verify", "-q", "HEAD")
	if rc != 0 || hrev == "" {
		die("land: HEAD를 읽지 못했다(git 실패) — 착지 훅을 실행하지 않았다")
	}
	argv := []string{"env", "HERDR_LAND=1", "HERDR_LAND_PRIORITY=" + lp, "HERDR_LAND_HOOK_REV=" + hrev}
	if ax != "" {
		argv = append(argv, "HERDR_LAND_PUSH_EXTRA="+ax)
	}
	rc = lrun(append(argv, "bash", ".claude/herdr-land.sh", base)...)
	if rc != 0 {
		lfail(rc)
		return rc
	}
	switch gu := gitRunQ("remote", "get-url", "origin"); gu {
	case 0:
		ffmain(mainwt(), base)
		landAfter(base, lmr)
		if lvd != "" {
			lverdictDone("")
		}
	case 2:
		if lvd != "" {
			lverdictDone("")
		}
	default:
		if lvd != "" {
			lverdictDone("")
		}
		eprintf("hc-lane: 경고 — 메인 폴더 앞당김 건너뜀: 원격 origin을 읽지 못했다(git remote get-url 종료 %d) — 그대로 둠(착지 표식·임대 놓기도 건너뜀)\n", gu)
	}
	return 0
}

// landBuiltin: 검증(.claude/herdr-verify.sh) → push를 여기서
func landBuiltin() int {
	// 원격 없음(git remote get-url 종료 2)과 조회 실패를 가른다
	switch gitRunQ("remote", "get-url", "origin") {
	case 0:
		remote = true
	case 2:
		remote = false
	default:
		die("land: 원격 origin을 읽지 못했다(git remote get-url 실패) — 원격 없음으로 읽지 않는다. 같은 줄을 다시")
	}
	var pu string
	if remote {
		var rc int
		if pu, rc = gitOut("remote", "get-url", "--push", "--all", "origin"); rc != 0 {
			die("land: origin의 push 주소를 읽지 못했다(git remote get-url --push --all 실패) — 모르면 착지하지 않는다. 같은 줄을 다시")
		}
		if countNonEmpty(pu) != 1 {
			die("land: origin의 push 주소가 하나가 아니다 — 여러 push 주소(remote.origin.pushurl·url 여러 줄)는 지원하지 않는다. 하나로 줄인 뒤 같은 줄을 다시")
		}
		switch ic := gitRunQ("config", "--get-regexp", `^includeif\.onbranch:`); ic {
		case 1:
		case 0:
			die("land: 브랜치별 조건 설정(includeIf onbranch)은 지원하지 않는다 — push 주소가 HEAD 상태에 따라 바뀔 수 있다. 그 설정을 뺀 뒤 같은 줄을 다시")
		default:
			die("land: git 설정을 읽지 못했다(git config --get-regexp 종료 %d) — 브랜치별 조건 설정이 있는지 몰라 착지하지 않는다. 같은 줄을 다시", ic)
		}
	}
	if remote {
		bref, blab = "refs/remotes/origin/"+base, "origin/"+base
	} else {
		bref, blab = "refs/heads/"+base, "로컬 "+base
	}
	tries, err := strconv.Atoi(os.Getenv("HERDR_LAND_TRIES"))
	if err != nil || tries <= 0 || strings.Trim(os.Getenv("HERDR_LAND_TRIES"), "0123456789") != "" {
		tries = 5
	}
	for i := 1; ; i++ {
		if !tclean("시작 때") {
			die("land: 커밋 안 된 변경이나 멈춘 rebase가 있다(git status). 커밋하거나 rebase를 마친 뒤 같은 줄을 다시")
		}
		if remote && fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin") != 0 {
			die("land: git fetch 실패")
		}
		// 기준 SHA를 이 바퀴에 고정해 그 SHA로 rebase
		bs, rc := gitOut("rev-parse", "-q", "--verify", bref+"^{commit}")
		if rc != 0 {
			die("land: %s 가 없다", blab)
		}
		if rb(bs) != 0 {
			if lpid != "" {
				lerr("hc-lane land: 승인 착지 중 rebase 충돌 — 풀면 내용이 승인과 달라진다. 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 커밋으로 hc-lane report 완료 <sha>(새 감사)")
			} else {
				lerr(fmt.Sprintf("hc-lane land: rebase 충돌. 양쪽 내용을 살려 풀고 git add → git rebase --continue 뒤 같은 줄(hc-lane land %s)을 다시", base))
			}
			exit(3)
		}
		if mgx == "1" && isFile(lf) {
			eprintf("hc-lane land: 기본 브랜치에 전용 착지 스크립트(.claude/herdr-land.sh)가 생겼다 — 병합기 MVP-1은 내장 길로 넣지 않는다\n")
			exit(7)
		}
		// 보내는 것은 검증한 커밋 그 자체
		cand, rc := gitOut("rev-parse", "-q", "--verify", "HEAD")
		if rc != 0 || cand == "" {
			die("land: rebase 뒤 HEAD를 읽지 못했다(git 실패) — 아무것도 보내지 않았다. 같은 줄을 다시")
		}
		if gitRunQ("cat-file", "-e", cand+"^{commit}") != 0 {
			die("land: rebase 뒤 HEAD 값(%s)이 이 저장소의 커밋이 아니다(git 조회 이상) — 아무것도 보내지 않았다. 같은 줄을 다시", cand)
		}
		if !tclean("rebase 뒤") {
			die("land: rebase 뒤 색인·추적 파일이 후보와 다르다(git status) — 보내지 않았다. 같은 줄을 다시")
		}
		if lpid != "" {
			lverdictCheck(bref)
		}
		if cand == bs && px == "" {
			fmt.Printf("hc-lane land: 이미 들어가 있다 %s = %s (보낼 것 없음)\n", cut(cand, 9), blab)
			if remote {
				if rc := landKnown(cand); rc != 0 {
					return rc
				}
			}
			if lvd != "" {
				lverdictDone(cand)
			}
			if remote {
				ffmain(mainwt(), base)
			}
			return 0
		}
		if !isFile(".claude/herdr-verify.sh") {
			die("land: .claude/herdr-verify.sh 가 없어 push하지 않았다(검증 없는 착지는 없다). 저장소에 검증 스크립트(또는 .claude/herdr-land.sh)를 두거나 지휘에 알린다")
		}
		committedHook(cand, ".claude/herdr-verify.sh")
		verifiedEnv(cand)
		if rc := lrun("bash", ".claude/herdr-verify.sh"); rc != 0 {
			lfail(rc)
			die("land: 검증 실패, push하지 않았다. 고쳐 커밋한 뒤 같은 줄을 다시")
		}
		h, rc := gitOut("rev-parse", "-q", "--verify", "HEAD")
		if rc != 0 {
			die("land: 검증 뒤 HEAD를 읽지 못했다(git 실패) — 보내지 않았다. 같은 줄을 다시")
		}
		if h != cand {
			die("land: 검증하는 동안 HEAD가 바뀌었다(검증한 %s → 지금 %s) — 검증하지 않은 커밋은 보내지 않았다. 같은 줄을 다시", cut(cand, 9), cut(h, 9))
		}
		if !tclean("검증 뒤") {
			die("land: 검증이 추적 파일·색인을 바꿨다(git status) — 검증한 파일과 보낼 커밋이 달라 보내지 않았다. 검증 스크립트를 고치거나 바뀐 것을 커밋한 뒤 같은 줄을 다시")
		}
		if lpid != "" {
			lverdictCheck(bref)
		}
		if lvd != "" {
			lverdictSave(cand, "builtin")
		}
		if remote {
			if rc := landPush(cand, pu); rc >= 0 {
				return rc
			}
		} else if landLocal(cand) {
			return 0
		}
		if i >= tries {
			eprintf("hc-lane land: 다른 쪽 착지가 먼저 들어가 %d번 거절됐다. 같은 줄을 다시\n", tries)
			exit(4)
		}
		eprintf("hc-lane land: 다른 쪽 착지가 먼저 들어가 거절됨, 처음부터 다시 (%d/%d)\n", i+1, tries)
	}
}
