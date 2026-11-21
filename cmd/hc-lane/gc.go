package main

// gc(셸 판 lib/hc-lane/gc.sh): 끝난 레인의 작업 폴더·가지와 원격 중간 참조를 정리한다. 하나라도 맞지 않으면 「남김 <대상> <까닭>」,
// 지우지 않는다. 마지막 줄 「지움 n · 남김 m」, 오류(확인 못 함)가 있었으면 종료 1

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func init() { register("gc", gcMain, nil) }

var (
	gcDn, gcKn int
	gcErr      bool
	gcNow      int64
	gcKeep     []string
	gcP        string
)

func gcKeepf(t, why string) { fmt.Printf("%s남김 %s %s\n", gcP, t, why); gcKn++ }
func gcGone(t, what string) { fmt.Printf("%s지움 %s %s\n", gcP, t, what); gcDn++ }

const ginScript = `cd "$1" || exit 2; c=$(git rev-list --no-merges "$2..$3") || exit 2
    git log -p --no-merges --no-color --no-ext-diff "$3..$2" | git patch-id --verbatim | sed "s/^/U /"
    git log -p --no-merges --no-color --no-ext-diff "$2..$3" | git patch-id --verbatim | sed "s/^/B /"
    printf "%s\n" "$c" | sed "/^$/d; s/^/C /"`

// gin: 그 커밋이 기준에 다 들어갔나. 0 들어감(조상, 또는 병합 커밋 없이 커밋마다 공백까지 같은 패치가 있음), 1 아님(안 들어간 수), 2 확인 못 함
func gin(c string) (string, int) {
	switch xrun(nil, os.Stderr, "git", "-C", repo, "merge-base", "--is-ancestor", c, bref) {
	case 0:
		return "0", 0
	case 1:
	default:
		return "0", 2
	}
	gm, r := xout("git", "-C", repo, "rev-list", "--merges", bref+".."+c)
	if r != 0 {
		return "0", 2
	}
	if gm != "" {
		n, r := xout("git", "-C", repo, "rev-list", "--count", bref+".."+c)
		if r != 0 {
			n = "?"
		}
		return n, 1
	}
	out, r := xout("sh", "-c", ginScript, "hc-lane", repo, bref, c)
	if r != 0 {
		return "0", 2
	}
	u, b := map[string]bool{}, map[string]string{}
	var cs []string
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		switch {
		case len(f) >= 2 && f[0] == "U":
			u[f[1]] = true
		case len(f) >= 3 && f[0] == "B":
			b[f[2]] = f[1]
		case len(f) >= 2 && f[0] == "C":
			cs = append(cs, f[1])
		}
	}
	m := 0
	for _, x := range cs {
		if p, ok := b[x]; !ok || !u[p] {
			m++
		}
	}
	if m == 0 {
		return "0", 0
	}
	return strconv.Itoa(m), 1
}

// jdg: 들어갔고 --min-age보다 오래됐으면 true. 아니면 「남김」을 찍는다
func jdg(t, c string, ts int64) bool {
	switch gn, r := gin(c); r {
	case 0:
	case 1:
		d, _ := xout("git", "-C", repo, "show", "-s", "--format=%cs", c)
		gcKeepf(t, "안 들어감 "+gn+"개 · 마지막 "+d)
		return false
	default:
		gcKeepf(t, "들어갔는지 확인하지 못함")
		gcErr = true
		return false
	}
	if gcNow-ts < gcSecs {
		gcKeepf(t, fmt.Sprintf("새것(%d시간)", (gcNow-ts)/3600))
		return false
	}
	return true
}

// posixClass: 셸 case 무늬의 [:이름:] 묶음(Go 정규식도 같은 이름을 [ ] 안에서 받는다)
var posixClass = map[string]bool{"alnum": true, "alpha": true, "blank": true, "cntrl": true, "digit": true, "graph": true,
	"lower": true, "print": true, "punct": true, "space": true, "upper": true, "xdigit": true}

// globRe: 셸 case 무늬(* ? [..] \)를 정규식으로 — 글자(룬) 단위, * 는 / 도 맞는다. [ ] 안은 ! ^ 부정, 맨 앞 ] 는 글자,
// [:digit:] 같은 묶음. 해석할 수 없는 무늬(닫히지 않은 [·모르는 묶음)는 오류 — 부르는 쪽은 지우지 않고 멈춘다(gofix L02)
func globRe(p string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			b.WriteString("(?s:.*)")
		case '?':
			b.WriteString("(?s:.)")
		case '\\':
			if i+1 < len(rs) {
				i++
			}
			b.WriteString(regexp.QuoteMeta(string(rs[i])))
		case '[':
			j := i + 1
			b.WriteString("[")
			if j < len(rs) && (rs[j] == '!' || rs[j] == '^') {
				b.WriteString("^")
				j++
			}
			closed := false
			for first := true; j < len(rs); j, first = j+1, false {
				r := rs[j]
				if r == ']' && !first {
					closed = true
					break
				}
				if r == '[' && j+1 < len(rs) && rs[j+1] == ':' {
					k := j + 2
					for k+1 < len(rs) && !(rs[k] == ':' && rs[k+1] == ']') {
						k++
					}
					if k+1 >= len(rs) || !posixClass[string(rs[j+2:k])] {
						return nil, fmt.Errorf("모르는 문자 묶음: %s", p)
					}
					b.WriteString("[:" + string(rs[j+2:k]) + ":]")
					j = k + 1
					continue
				}
				if strings.ContainsRune(`\[]^`, r) {
					b.WriteString(`\`)
				}
				b.WriteRune(r)
			}
			if !closed {
				return nil, fmt.Errorf("닫히지 않은 [: %s", p)
			}
			b.WriteString("]")
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

var gcKeepRe []*regexp.Regexp

func kept(br string) bool {
	for _, re := range gcKeepRe {
		if re.MatchString(br) {
			return true
		}
	}
	return false
}

// rgc: 원격 <접두>* 가운데 들어갔고 커밋이 오래된 것을 ls-remote 때 값 그대로일 때만 지운다(refs/heads면 기본·남길 목록 빼고)
func rgc(prefix string) {
	ls, r := xout("git", "-C", repo, "ls-remote", "origin", prefix+"*")
	if r != 0 {
		die("gc: git ls-remote 실패: %s", repo)
	}
	type ref struct{ s, r string }
	var refs []ref
	for _, ln := range strings.Split(ls, "\n") {
		s, rf, ok := strings.Cut(ln, "\t")
		if ok && strings.HasPrefix(rf, prefix) && !strings.HasSuffix(rf, "^{}") {
			refs = append(refs, ref{s, rf})
		}
	}
	var miss []string
	for _, x := range refs {
		if b, ok := strings.CutPrefix(x.r, "refs/heads/"); ok && (b == base || kept(b)) {
			continue
		}
		if xrun(nil, nil, "git", "-C", repo, "cat-file", "-e", x.s+"^{commit}") != 0 {
			miss = append(miss, x.r)
		}
	}
	if len(miss) > 0 && xfetch(repo, append([]string{"-q", "--no-write-fetch-head", "--refmap=", "origin"}, miss...)...) != 0 {
		die("gc: 원격 참조 커밋을 받지 못했다")
	}
	for _, x := range refs {
		if b, ok := strings.CutPrefix(x.r, "refs/heads/"); ok {
			if b == base {
				continue
			}
			if kept(b) {
				gcKeepf(x.r, "남길 목록(gc.keep_branches)")
				continue
			}
		}
		cts, r := xout("git", "-C", repo, "show", "-s", "--format=%ct", x.s)
		if r != 0 {
			gcKeepf(x.r, "커밋 시각을 읽지 못함")
			gcErr = true
			continue
		}
		ct, _ := strconv.ParseInt(cts, 10, 64)
		if !jdg(x.r, x.s, ct) {
			continue
		}
		if dry {
			gcGone(x.r, x.s)
			continue
		}
		// 값이 ls-remote 때 그대로일 때만 지운다. 되돌리기: git push origin <sha>:<참조>
		if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "push", "-q", "--force-with-lease="+x.r+":"+x.s, "origin", ":"+x.r) != 0 {
			gcKeepf(x.r, "push 지우기 실패")
			gcErr = true
			continue
		}
		gcGone(x.r, x.s)
	}
}

const gcAgeScript = `m=$1; shift; for f; do [ -e "$f" ] || continue; o=$(find "$f" -prune -mmin -"$m") || exit 2
        [ -z "$o" ] || exit 1; done`

const gcProgScript = `for s in rebase-merge rebase-apply MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD BISECT_LOG; do
        [ ! -e "$1/$s" ] || echo "$s"; done`

const gcAttScript = `c=$(git -C "$1" rev-parse --path-format=absolute --git-common-dir) || exit 0
    cat "$c"/rebase-*/head-name "$c"/worktrees/*/rebase-*/head-name 2>/dev/null; exit 0`

var qaPathRe = regexp.MustCompile(`(^|/)\.qa(/|$)`)

func short12(s string) string { return s[:min(12, len(s))] }

func gcMain(args []string) int {
	startSetup()
	rest := parseStart("gc", args)
	checkArgs("gc", rest)
	resolveRepo("gc")
	gcP = dryP()
	gcNow = now()
	gcKeep = strings.Fields(cfg("gc.keep_branches", ""))
	gcKeepRe = nil
	for _, p := range gcKeep {
		re, err := globRe(p)
		if err != nil {
			die("gc: 설정 gc.keep_branches 무늬를 읽지 못했다(%v) — 아무것도 지우지 않았다", err)
		}
		gcKeepRe = append(gcKeepRe, re)
	}
	ps, r := termPairs()
	switch r {
	case 1:
		die("gc: hc ls 실패 — 작업 폴더를 쓰는 터미널을 확인하지 못해 아무것도 지우지 않았다")
	case 2:
		die("gc: hc ls --json 출력을 읽지 못했다(형식이 바뀌었나?) — 아무것도 지우지 않았다")
	}
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
	cur, r := xout("git", "-C", repo, "symbolic-ref", "-q", "--short", "HEAD")
	if r != 0 && r != 1 { // 1 = 분리 HEAD. 그 밖(시간 넘김 포함)은 현재 가지를 모른다 — 지우지 않는다
		die("gc: 현재 가지를 읽지 못했다(git symbolic-ref 종료 %d) — 아무것도 지우지 않았다", r)
	}
	wl, r := xout("git", "-C", repo, "worktree", "list", "--porcelain")
	if r != 0 {
		die("gc: git worktree list 실패: %s", repo)
	}
	type rec struct{ p, h, b, l string }
	var recs []rec
	var cr *rec
	for _, ln := range strings.Split(wl, "\n") {
		switch {
		case strings.HasPrefix(ln, "worktree "):
			recs = append(recs, rec{strings.TrimPrefix(ln, "worktree "), "-", "-", "-"})
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
	for _, x := range recs {
		if x.p == "" || x.p == repo {
			continue
		}
		wt, branch = x.p, strings.TrimPrefix(x.b, "refs/heads/")
		if !strings.HasPrefix(x.b, "refs/heads/") {
			branch = ""
		}
		h := x.h
		if x.l != "-" {
			gcKeepf(wt, "잠긴 작업 폴더")
			continue
		}
		if branch != "" && (branch == base || branch == cur) {
			gcKeepf(wt, "기본·현재 가지 "+branch)
			continue
		}
		switch _, r := gin(h); r {
		case 0:
		case 1:
			b := branch
			if b == "" {
				b = "분리 HEAD"
			}
			gcKeepf(wt, blab+" 에 안 들어감("+b+" "+short12(h)+")")
			continue
		default:
			gcKeepf(wt, "들어갔는지 확인하지 못함")
			gcErr = true
			continue
		}
		dir := false
		switch xrun(nil, os.Stderr, "sh", "-c", `[ -e "$1" ] || [ -L "$1" ]`, "hc-lane", wt) {
		case 0:
			dir = true
		case 1:
		default:
			gcKeepf(wt, "폴더를 확인하지 못함")
			gcErr = true
			continue
		}
		if dir {
			if busy := busyTerms(ps, false); len(busy) > 0 {
				var as []string
				for _, b := range busy {
					a, _, _ := strings.Cut(b, " ")
					as = append(as, a)
				}
				gcKeepf(wt, "살아 있는 터미널 "+strings.Join(as, " "))
				continue
			}
			adm, r := xout("git", "-C", wt, "rev-parse", "--absolute-git-dir")
			if r != 0 {
				gcKeepf(wt, "git 관리 폴더를 찾지 못함")
				gcErr = true
				continue
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
					continue
				default:
					gcKeepf(wt, "나이를 확인하지 못함")
					gcErr = true
					continue
				}
			}
			ip, r := xout("sh", "-c", gcProgScript, "hc-lane", adm)
			if r != 0 {
				gcKeepf(wt, "진행 상태를 확인하지 못함")
				gcErr = true
				continue
			}
			if ip != "" {
				gcKeepf(wt, "진행 중 "+strings.Join(strings.Fields(ip), " "))
				continue
			}
			st, r := xout("git", "--no-optional-locks", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
			if r != 0 {
				gcKeepf(wt, "git status 실패")
				gcErr = true
				continue
			}
			if st != "" {
				gcKeepf(wt, fmt.Sprintf("커밋 안 된 변경·untracked %d개", len(strings.Split(st, "\n"))))
				continue
			}
			ig, r := xout("git", "--no-optional-locks", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--ignored", "--untracked-files=normal")
			if r != 0 {
				gcKeepf(wt, "git status --ignored 실패")
				gcErr = true
				continue
			}
			var qa []string
			for _, ln := range strings.Split(ig, "\n") {
				if p, ok := strings.CutPrefix(ln, "!! "); ok && qaPathRe.MatchString(p) {
					qa = append(qa, p)
				}
			}
			if len(qa) > 0 {
				gcKeepf(wt, "무시된 보관물 "+strings.Join(qa, " ")+" (hc-lane clean이 메인 폴더로 옮기고 정리 — 손으로 지우지 말 것)")
				continue
			}
		}
		what := short12(h)
		if branch != "" {
			what = "가지 " + branch + " " + what
		}
		if dry {
			gcGone(wt, what)
			continue
		}
		if dir && xtest("-f", wt+"/.claude/herdr-teardown.sh") {
			if r, _ := hook("teardown"); r != 0 {
				errf("hc-lane gc: herdr-teardown.sh 실패(정리는 계속): %s\n", wt)
			}
		}
		// 셋(들어감·깨끗함·쓰는 터미널 없음)을 다 본 뒤라 강제로 지운다
		if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "worktree", "remove", "--force", wt) != 0 {
			gcKeepf(wt, "git worktree remove 실패")
			gcErr = true
			continue
		}
		if branch != "" {
			if v, _ := xout("git", "-C", repo, "rev-parse", "-q", "--verify", "refs/heads/"+branch); v == h {
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

	// 작업 폴더 없이 남은 로컬 가지: 기본·현재 HEAD·작업 폴더에 붙은 가지(멈춘 rebase의 가지 포함)는 빼고 같은 판정
	att := map[string]bool{}
	for _, x := range recs {
		att[x.b] = true
	}
	if a, _ := xout("sh", "-c", gcAttScript, "hc-lane", repo); a != "" {
		for _, ln := range strings.Split(a, "\n") {
			att[ln] = true
		}
	}
	bl, r := xout("git", "-C", repo, "for-each-ref", "--format=%(refname)%09%(objectname)%09%(committerdate:unix)", "refs/heads")
	if r != 0 {
		die("gc: git for-each-ref 실패: %s", repo)
	}
	for _, ln := range strings.Split(bl, "\n") {
		f := strings.Split(ln, "\t")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		rf, s := f[0], f[1]
		ct, _ := strconv.ParseInt(f[2], 10, 64)
		b := strings.TrimPrefix(rf, "refs/heads/")
		if b == base || b == cur || att[rf] {
			continue
		}
		if kept(b) {
			gcKeepf("가지 "+b, "남길 목록(gc.keep_branches)")
			continue
		}
		rl, r := xoutQ("git", "-C", repo, "reflog", "-1", "--date=unix", "--format=%gd", rf)
		if r != 0 { // 조회 실패(시간 넘김 포함)는 「기록 없음」(종료 0·빈 출력)이 아니다 — 나이를 모르니 남긴다(gofix E03)
			gcKeepf("가지 "+b, "가지 기록(reflog)을 읽지 못함")
			gcErr = true
			continue
		}
		if i := strings.LastIndex(rl, "@{"); i >= 0 {
			rl = rl[i+2:]
		}
		rl = strings.TrimSuffix(rl, "}")
		if n, err := strconv.ParseInt(rl, 10, 64); err == nil && isDigits(rl) && n > ct {
			ct = n
		}
		if !jdg("가지 "+b, s, ct) {
			continue
		}
		if dry {
			gcGone("가지 "+b, s)
			continue
		}
		// 판정한 값 그대로일 때만 지운다. 되돌리기: git branch <가지> <sha>
		if v, _ := xout("git", "-C", repo, "rev-parse", "-q", "--verify", rf); v != s {
			gcKeepf("가지 "+b, "그사이 움직임")
			continue
		}
		if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "branch", "-q", "-D", "--", b) != 0 {
			gcKeepf("가지 "+b, "git branch -D 실패")
			gcErr = true
			continue
		}
		gcGone("가지 "+b, s)
	}

	if (integ != "" || rbFlag) && !remote {
		fmt.Println("원격이 없어 --integ·--remote-branches 는 할 것 없음")
	} else {
		if integ != "" {
			rgc(integ)
		}
		if rbFlag {
			rgc("refs/heads/")
		}
	}
	fmt.Printf("%s지움 %d · 남김 %d\n", gcP, gcDn, gcKn)
	if gcErr {
		return 1
	}
	return 0
}
