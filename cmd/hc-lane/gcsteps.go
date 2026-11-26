package main

// gc(gcMain)의 단계들 — 차례는 gcMain 한 곳, 단계마다 입력·출력을 인자·반환으로.
// 판정만 하는 것(gcKeepPatterns·gcRecs·reflogTime)은 읽기·쓰기 없이 단위 시험(gcsteps_test.go).

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// gcKeepPatterns: 설정 gc.keep_branches 무늬마다 C 로캘 꼴과 UTF-8 로캘 꼴(R-05). 하나라도 못 읽으면 오류
func gcKeepPatterns(pats []string) ([]*regexp.Regexp, error) {
	var res []*regexp.Regexp
	for _, p := range pats {
		for _, uni := range []bool{false, true} {
			re, err := globRex(p, uni)
			if err != nil {
				return nil, err
			}
			res = append(res, re)
		}
	}
	return res, nil
}

// gcRec: worktree list의 작업 폴더 하나 — 경로·HEAD·가지 참조·잠김(x)·없으면 -
type gcRec struct{ p, h, b, l string }

// gcRecs: git worktree list --porcelain → 작업 폴더들(locked·bare는 l=x)
func gcRecs(wl string) []gcRec {
	var recs []gcRec
	var cr *gcRec
	for _, ln := range strings.Split(wl, "\n") {
		switch {
		case strings.HasPrefix(ln, "worktree "):
			recs = append(recs, gcRec{strings.TrimPrefix(ln, "worktree "), "-", "-", "-"})
			cr = &recs[len(recs)-1]
		case cr == nil:
		case strings.HasPrefix(ln, "HEAD "):
			cr.h = ln[5:]
		case strings.HasPrefix(ln, "branch "):
			cr.b = ln[7:]
		case strings.HasPrefix(ln, "locked"), strings.HasPrefix(ln, "bare"):
			cr.l = "x"
		}
	}
	return recs
}

// gcStart: 판정 전 확인 — 터미널 목록·fetch·기준·현재 가지·작업 폴더 목록. 하나라도 모르면 아무것도 지우지 않고 끝낸다
func gcStart() (ps [][2]string, cur, wl string) {
	ps, r := termPairs()
	if r != 0 {
		die("gc: %s — 작업 폴더를 쓰는 터미널을 확인하지 못해 아무것도 지우지 않았다", termWhy(r))
	}
	gcNoteUnk(ps)
	if remote {
		if dry {
			fmt.Printf("[dry-run] git fetch 건너뜀: 지금 있는 origin/%s 로 판정\n", base)
		} else if xfetch(repo, "-q", "origin") != 0 {
			die("gc: git fetch 실패: %s", repo)
		}
	}
	if xrun(nil, os.Stderr, "git", "-C", repo, "rev-parse", "-q", "--verify", bref+"^{commit}") != 0 {
		die("gc: %s 가 없다: %s", blab, repo)
	}
	cur, r = xout("git", "-C", repo, "symbolic-ref", "-q", "--short", "HEAD")
	if r != 0 && r != 1 { // 1 = 분리 HEAD. 그 밖(시간 넘김 포함)은 현재 가지를 모른다 — 지우지 않는다
		die("gc: 현재 가지를 읽지 못했다(git symbolic-ref 종료 %d) — 아무것도 지우지 않았다", r)
	}
	wl, r = xout("git", "-C", repo, "worktree", "list", "--porcelain")
	if r != 0 {
		die("gc: git worktree list 실패: %s", repo)
	}
	return ps, cur, wl
}

// gcDirKept: 있는 작업 폴더를 남길 까닭(쓰는 터미널·나이·진행 중·변경·.qa)이 있으면 「남김」을 찍고 true
func gcDirKept(ps [][2]string) bool {
	if busy := busyTerms(ps, false); len(busy) > 0 {
		var as []string
		for _, b := range busy {
			a, _, _ := strings.Cut(b, " ")
			as = append(as, a)
		}
		gcKeepf(wt, "살아 있는 터미널 "+strings.Join(as, " "))
		return true
	}
	adm, r := xout("git", "-C", wt, "rev-parse", "--absolute-git-dir")
	if r != 0 {
		gcKeepf(wt, "git 관리 폴더를 찾지 못함")
		gcErr = true
		return true
	}
	// 나이는 status보다 먼저 본다(status가 index를 다시 쓰면 묵은 폴더가 새것으로 보인다)
	if gcMins != 0 {
		switch xrun(nil, os.Stderr, "sh", "-c", gcAgeScript, "hc-lane", strconv.FormatInt(gcMins, 10), adm+"/HEAD", adm+"/index", adm+"/logs/HEAD", wt) {
		case 0:
		case 1:
			ma := minage
			if ma == "" {
				ma = "24h"
			}
			gcKeepf(wt, "새것(마지막 git 활동이 --min-age "+ma+" 안)")
			return true
		default:
			gcKeepf(wt, "나이를 확인하지 못함")
			gcErr = true
			return true
		}
	}
	ip, r := xout("sh", "-c", gcProgScript, "hc-lane", adm)
	if r != 0 {
		gcKeepf(wt, "진행 상태를 확인하지 못함")
		gcErr = true
		return true
	}
	if ip != "" {
		gcKeepf(wt, "진행 중 "+strings.Join(strings.Fields(ip), " "))
		return true
	}
	if !carryDrop("") { // 바뀌지 않은 실은 파일(--carry)은 레인 산출물이 아니다(carry.go)
		gcKeepf(wt, "실어 보낸 자료를 거두지 못함")
		gcErr = true
		return true
	}
	st, r := xout("git", "--no-optional-locks", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if r != 0 {
		gcKeepf(wt, "git status 실패")
		gcErr = true
		return true
	}
	if st != "" {
		gcKeepf(wt, fmt.Sprintf("커밋 안 된 변경·untracked %d개", len(strings.Split(st, "\n"))))
		return true
	}
	// clean과 같은 탐색(어느 깊이든 .qa 폴더 + check-ignore) — status --ignored 는 무시된 폴더를 접어 그 안 .qa를 놓친다(R-3A20)
	qa, ok := qaDirs(wt)
	if !ok {
		gcKeepf(wt, "무시된 보관물(.qa)을 확인하지 못함")
		gcErr = true
		return true
	}
	if len(qa) > 0 {
		gcKeepf(wt, "무시된 보관물 "+strings.Join(qa, " ")+" (hc-lane clean이 메인 폴더로 옮기고 정리 — 손으로 지우지 말 것)")
		return true
	}
	return false
}

// gcWorktree: 작업 폴더 하나(전역 wt·branch를 그것으로) — 들어갔고 깨끗하고 쓰는 터미널이 없을 때만 지운다(가지도). 아니면 「남김」
func gcWorktree(x gcRec, ps [][2]string, cur string) {
	wt, branch = x.p, strings.TrimPrefix(x.b, "refs/heads/")
	if !strings.HasPrefix(x.b, "refs/heads/") {
		branch = ""
	}
	h := x.h
	if x.l != "-" {
		gcKeepf(wt, "잠긴 작업 폴더")
		return
	}
	if branch != "" && (branch == base || branch == cur) {
		gcKeepf(wt, "기본·현재 가지 "+branch)
		return
	}
	// HEAD 0000…: 가지가 정말 없으면(종료 1) 깨진 작업 트리로 남긴다(지우지 않음). 조회 실패는 아래 gin이 확인 못 함(오류)으로
	if strings.Trim(h, "0") == "" && branch != "" {
		if _, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", x.b); r == 1 {
			gcKeepf("깨진 작업 트리 "+wt, "— HEAD가 없는 가지 "+branch+"을 가리킴(사람이 본다)")
			return
		}
	}
	switch _, r := gin(h); r {
	case 0:
	case 1:
		b := branch
		if b == "" {
			b = "분리 HEAD"
		}
		gcKeepf(wt, blab+" 에 안 들어감("+b+" "+short12(h)+")")
		return
	default:
		gcKeepf(wt, "들어갔는지 확인하지 못함")
		gcErr = true
		return
	}
	dir := false
	switch xrun(nil, os.Stderr, "sh", "-c", `[ -e "$1" ] || [ -L "$1" ]`, "hc-lane", wt) {
	case 0:
		dir = true
	case 1:
	default:
		gcKeepf(wt, "폴더를 확인하지 못함")
		gcErr = true
		return
	}
	if dir && gcDirKept(ps) {
		return
	}
	what := short12(h)
	if branch != "" {
		what = "가지 " + branch + " " + what
	}
	if dry {
		gcGone(wt, what)
		return
	}
	if dir && xtest("-f", wt+"/.claude/herdr-teardown.sh") {
		if r, _ := hook("teardown"); r != 0 {
			errf("hc-lane gc: herdr-teardown.sh 실패(정리는 계속): %s\n", wt)
		}
	}
	// 지우기 직전 대상 기계의 사용 여부를 다시 본다 — 그사이 붙은 터미널·끊긴 기계는 남긴다(R-07)
	if dir {
		if ps2, r := termPairs(); r != 0 {
			gcKeepf(wt, "지우기 직전 터미널 재확인 실패("+termWhy(r)+")")
			gcErr = true
			return
		} else if len(busyTerms(ps2, false)) > 0 {
			gcKeepf(wt, "지우기 직전 살아 있는 터미널")
			return
		}
	}
	// 셋(들어감·깨끗함·쓰는 터미널 없음)을 다 본 뒤라 강제로 지운다
	if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "worktree", "remove", "--force", wt) != 0 {
		gcKeepf(wt, "git worktree remove 실패")
		gcErr = true
		return
	}
	if branch != "" {
		if v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", "refs/heads/"+branch); r != 0 && r != 1 {
			errf("hc-lane gc: 가지 %s 를 다시 확인하지 못해 남겼다(git rev-parse 종료 %d)\n", branch, r)
			gcErr = true
		} else if r == 0 && v == h {
			if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "branch", "-q", "-D", branch) != 0 {
				errf("hc-lane gc: 가지 %s 를 지우지 못했다\n", branch)
				gcErr = true
			}
		} else {
			errf("hc-lane gc: 가지 %s 가 그사이 움직여 남겼다\n", branch)
		}
	}
	gcGone(wt, what)
}

// gcAttached: 작업 폴더에 붙은 가지 참조(멈춘 rebase의 가지 포함)
func gcAttached(recs []gcRec) map[string]bool {
	att := map[string]bool{}
	for _, x := range recs {
		att[x.b] = true
	}
	if a, _ := xout("sh", "-c", gcAttScript, "hc-lane", repo); a != "" {
		for _, ln := range strings.Split(a, "\n") {
			att[ln] = true
		}
	}
	return att
}

// reflogTime: 가지 나이 — 커밋 시각 ct와 reflog -1 --date=unix --format=%gd 출력 rl 가운데 늦은 것.
// 빈 출력만 「기록 없음」(커밋 시각). 꼴·시각이 깨졌으면 남길 까닭(why)을 준다(R-08)
func reflogTime(rl string, ct int64) (int64, string) {
	if rl == "" {
		return ct, ""
	}
	m := reflogTS.FindStringSubmatch(rl)
	if m == nil {
		return 0, "가지 기록(reflog) 꼴을 읽지 못함"
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, "가지 기록(reflog) 시각을 읽지 못함"
	}
	if n > ct {
		return n, ""
	}
	return ct, ""
}

// gcBranch: 작업 폴더 없이 남은 로컬 가지 한 줄(for-each-ref 「참조\tsha\t커밋 시각」) — 같은 판정으로 지우거나 남긴다
func gcBranch(ln string, att map[string]bool, cur string) {
	f := strings.Split(ln, "\t")
	if len(f) < 3 || f[0] == "" {
		return
	}
	rf, s := f[0], f[1]
	ct, _ := strconv.ParseInt(f[2], 10, 64)
	b := strings.TrimPrefix(rf, "refs/heads/")
	if b == base || b == cur || att[rf] {
		return
	}
	if kept(b) {
		gcKeepf("가지 "+b, "남길 목록(gc.keep_branches)")
		return
	}
	if !isDigits(f[2]) {
		gcKeepf("가지 "+b, "커밋 시각을 읽지 못함")
		gcErr = true
		return
	}
	rl, r := xoutQ("git", "-C", repo, "reflog", "-1", "--date=unix", "--format=%gd", rf)
	if r != 0 { // 조회 실패(시간 넘김 포함)는 「기록 없음」(종료 0·빈 출력)이 아니다 — 나이를 모르니 남긴다(gofix E03)
		gcKeepf("가지 "+b, "가지 기록(reflog)을 읽지 못함")
		gcErr = true
		return
	}
	ct, why := reflogTime(rl, ct)
	if why != "" {
		gcKeepf("가지 "+b, why)
		gcErr = true
		return
	}
	if !jdg("가지 "+b, s, ct) {
		return
	}
	if dry {
		gcGone("가지 "+b, s)
		return
	}
	// 판정한 값 그대로일 때만 지운다. 되돌리기: git branch <가지> <sha>
	// 종료 0과 같은 SHA가 함께 확인된 때만(실패한 조회의 출력을 믿지 않는다, R-10)
	if v, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", rf); r != 0 {
		gcKeepf("가지 "+b, "지우기 직전 재조회 실패")
		gcErr = true
		return
	} else if v != s {
		gcKeepf("가지 "+b, "그사이 움직임")
		return
	}
	if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "branch", "-q", "-D", "--", b) != 0 {
		gcKeepf("가지 "+b, "git branch -D 실패")
		gcErr = true
		return
	}
	gcGone("가지 "+b, s)
}

// gcRemotes: --integ·--remote-branches 원격 참조 정리
func gcRemotes() {
	if (integ != "" || rbFlag) && !remote {
		fmt.Println("원격이 없어 --integ·--remote-branches 는 할 것 없음")
		return
	}
	if integ != "" {
		rgc(integ)
	}
	if rbFlag {
		rgc("refs/heads/")
	}
}
