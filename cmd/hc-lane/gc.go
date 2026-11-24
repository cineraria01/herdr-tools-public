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

// uniClass: UTF-8 로캘 셸이 보는 묶음(한글 등 ASCII 밖 글자 포함). digit·xdigit·blank·cntrl은 로캘과 상관없이 ASCII 그대로
var uniClass = map[string]string{"alpha": `\p{L}\p{M}`, "alnum": `\p{L}\p{M}\p{N}`, "upper": `\p{Lu}`, "lower": `\p{Ll}`,
	"punct": `\p{P}\p{S}`, "graph": `\p{L}\p{M}\p{N}\p{P}\p{S}`, "print": `\p{L}\p{M}\p{N}\p{P}\p{S}\p{Zs}`, "space": `\s\p{Z}`}

// globRe: 셸 case 무늬(* ? [..] \)를 정규식으로 — 글자(룬) 단위, * 는 / 도 맞는다. [ ] 안은 ! ^ 부정, 맨 앞 ] 는 글자,
// [:digit:] 같은 묶음(C 로캘 꼴, ASCII). 해석할 수 없는 무늬(닫히지 않은 [·모르는 묶음)는 오류 — 부르는 쪽은 지우지 않고 멈춘다(gofix L02)
func globRe(p string) (*regexp.Regexp, error) { return globRex(p, false) }

// globRex: uni면 [:alpha:] 같은 묶음을 UTF-8 로캘 꼴(유니코드)로
func globRex(p string, uni bool) (*regexp.Regexp, error) {
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
					if u, ok := uniClass[string(rs[j+2:k])]; uni && ok {
						b.WriteString(u)
					} else {
						b.WriteString("[:" + string(rs[j+2:k]) + ":]")
					}
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

// gcKeepRe: 무늬마다 C 로캘 꼴과 UTF-8 로캘 꼴 둘 — 기준 셸의 로캘을 모르니 어느 쪽이든 맞으면 남긴다(보호 쪽으로, R-05)
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

var (
	reflogTS = regexp.MustCompile(`\A[^\n]*@\{([0-9]+)\}\z`) // reflog -1 --date=unix --format=%gd 한 줄
)

func short12(s string) string { return s[:min(12, len(s))] }

// gcMain: 단계 함수는 gcsteps.go — 차례(시작 확인 → 작업 폴더 → 남은 로컬 가지 → 원격 참조 → prune)는 여기 한 곳
func gcMain(args []string) int {
	startSetup()
	rest := parseStart("gc", args)
	checkArgs("gc", rest)
	resolveRepo("gc")
	gcP = dryP()
	gcNow = now()
	gcKeep = strings.Fields(cfg("gc.keep_branches", ""))
	gcKeepRe = nil
	res, err := gcKeepPatterns(gcKeep)
	if err != nil {
		die("gc: 설정 gc.keep_branches 무늬를 읽지 못했다(%v) — 아무것도 지우지 않았다", err)
	}
	gcKeepRe = res
	ps, cur, wl := gcStart()
	recs := gcRecs(wl)
	for _, x := range recs {
		if x.p == "" || x.p == repo {
			continue
		}
		gcWorktree(x, ps, cur)
	}

	// 작업 폴더 없이 남은 로컬 가지: 기본·현재 HEAD·작업 폴더에 붙은 가지(멈춘 rebase의 가지 포함)는 빼고 같은 판정
	att := gcAttached(recs)
	bl, r := xout("git", "-C", repo, "for-each-ref", "--format=%(refname)%09%(objectname)%09%(committerdate:unix)", "refs/heads")
	if r != 0 {
		die("gc: git for-each-ref 실패: %s", repo)
	}
	for _, ln := range strings.Split(bl, "\n") {
		gcBranch(ln, att, cur)
	}

	gcRemotes()
	gcPrune()
	fmt.Printf("%s지움 %d · 남김 %d\n", gcP, gcDn, gcKn)
	if gcErr {
		return 1
	}
	return 0
}
