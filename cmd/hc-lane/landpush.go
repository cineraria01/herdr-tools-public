package main

// landpush.go: 착지 push·로컬 올리기·착지 뒤 정리(land.go에서 나눔 — 줄 예산 600)

import (
	"fmt"
	"os"
	"strings"
)

// landPush: 원격 <기본>에 보낸다. 끝났으면 종료 코드(0 이상), 다른 쪽 착지가 앞서 거절됐으면 -1(처음부터 다시)
func landPush(cand, pu string) int {
	pu2, rc := gitOut("remote", "get-url", "--push", "--all", "origin")
	if rc != 0 {
		die("land: 검증 뒤 origin의 push 주소를 읽지 못했다 — 보내지 않았다. 같은 줄을 다시")
	}
	if pu2 != pu {
		die("land: 검증하는 동안 origin의 push 주소가 바뀌었다 — 보내지 않았다. 설정을 확인한 뒤 같은 줄을 다시")
	}
	pa, ex := []string{cand + ":refs/heads/" + base}, ""
	for _, e := range strings.Fields(px) {
		pa, ex = append(pa, e), "--atomic"
	}
	lmr := ""
	var mx []string
	if lmn, ok := lmname(); ok {
		lmr = "refs/hc-landed/" + lmn + "/" + unixNow()
		if mgx != "1" {
			mx, ex = []string{cand + ":" + lmr}, "--atomic"
			if lease := os.Getenv("HC_LANE_LEASE"); lease != "" {
				mx = append(mx, "--force-with-lease=refs/hc-lanes/"+lmn+":"+lease, ":refs/hc-lanes/"+lmn)
			}
		}
	}
	land := []string{"HERDR_LAND=1"}
	push1 := []string{"push"}
	if ex != "" {
		push1 = append(push1, ex)
	}
	push1 = append(append(append(push1, "origin"), pa...), mx...)
	ok := runCmd(cmdOpt{stdout: os.Stdout, stderr: os.Stderr, env: land}, "git", push1...) == 0
	if !ok && len(mx) > 0 {
		push2 := []string{"push"}
		if len(pa) > 1 {
			push2 = append(push2, "--atomic")
		}
		push2 = append(append(push2, "origin"), pa...)
		ok = runCmd(cmdOpt{stdout: os.Stdout, stderr: os.Stderr, env: land}, "git", push2...) == 0
	}
	if ok {
		m := ""
		if lmr != "" {
			m = " (착지 표식 " + lmr + ")"
		}
		if lrrn != 0 {
			m += fmt.Sprintf(" (rerere로 바뀐 경로 %d개)", lrrn)
		}
		fmt.Printf("hc-lane land: 착지 %s → origin/%s%s\n", cut(cand, 9), base, m)
		if lvd != "" {
			lverdictDone("")
		}
		ffmain(mainwt(), base)
		landAfter(base, lmr)
		return 0
	}
	// push 응답은 사라질 수 있다: 다시 보고 후보가 원격 기본에 들어가 있으면 레인 일은 끝. 견주지 못하면 「모름」으로 멈춘다
	rl, rc := gitOut("ls-remote", "origin", "refs/heads/"+base)
	if rc != 0 {
		die("land: push 결과 모름 — push가 실패로 돌아왔고 다시 볼 git ls-remote도 실패했다. 원격 %s 에 %s 가 들어갔는지 확인한 뒤 같은 줄을 다시", base, cut(cand, 9))
	}
	rss := lines(field2(rl, "refs/heads/"+base))
	if len(rss) != 1 {
		die("land: push 결과 모름 — git ls-remote에 refs/heads/%s 줄이 하나가 아니다. 들어갔는지 확인한 뒤 같은 줄을 다시", base)
	}
	rs := rss[0]
	if fetchr(os.Stdout, os.Stderr, "fetch", "-q", "origin") != 0 || gitRunQ("cat-file", "-e", rs+"^{commit}") != 0 {
		die("land: push 결과 모름 — 원격 %s(%s)를 가져오지 못했다(git fetch 실패). 들어갔는지 확인한 뒤 같은 줄을 다시", base, cut(rs, 9))
	}
	ia := gitRun("merge-base", "--is-ancestor", cand, rs)
	if ia == 0 {
		fmt.Printf("hc-lane land: push가 실패로 돌아왔지만 원격 %s 에 %s 가 들어가 있다(응답만 사라졌거나 같은 것이 먼저 들어감) — 착지 끝\n", base, cut(cand, 9))
		if lvd != "" {
			lverdictDone("")
		}
		ffmain(mainwt(), base)
		landAfter(base, lmr)
		return 0
	}
	if ia != 1 {
		die("land: push 결과 모름 — 원격 %s 와 견주지 못했다(git merge-base 종료 %d). 들어갔는지 확인한 뒤 같은 줄을 다시", base, ia)
	}
	// 후보가 원격에 없다(거절) — 함께 실은 lease 참조가 바뀌었으면(재제출) 기본 브랜치를 올리지 않고 종료 6
	for _, e := range strings.Fields(px) {
		r, isLease := strings.CutPrefix(e, "--force-with-lease=")
		if !isLease || r == "" || r[0] == ':' || !strings.Contains(r, ":") {
			continue
		}
		s := r[strings.Index(r, ":")+1:]
		r = r[:strings.Index(r, ":")]
		xr, rc := gitOut("ls-remote", "origin", r)
		if rc != 0 {
			die("land: push가 거절됐는데 lease 참조 %s 를 다시 보지 못했다(git ls-remote) — 같은 줄을 다시", r)
		}
		if field2(xr, r) != s {
			eprintf("hc-lane land: 함께 실은 lease 참조 %s 가 바뀌었다(다시 제출) — %s 를 올리지 않았다\n", r, base)
			exit(6)
		}
	}
	ia = gitRun("merge-base", "--is-ancestor", rs, cand)
	if ia == 0 {
		die("land: push 실패(다른 쪽 push가 앞선 것이 아니다 — 위 git 출력을 본다)")
	}
	if ia != 1 {
		die("land: push 결과 모름 — 원격 %s 와 견주지 못했다(git merge-base 종료 %d). 들어갔는지 확인한 뒤 같은 줄을 다시", base, ia)
	}
	return -1
}

// landLocal: origin이 없는 저장소 — 로컬 <기본>을 fast-forward로 올린다. 올렸으면 true, 다른 쪽이 앞서 못 올렸으면 false
func landLocal(cand string) bool {
	nw := cand
	old, rc := gitOut("rev-parse", "-q", "--verify", bref+"^{commit}")
	if rc != 0 {
		die("land: %s 를 읽지 못했다(git 실패) — 같은 줄을 다시", blab)
	}
	wl, rc := gitOut("worktree", "list", "--porcelain")
	if rc != 0 {
		die("land: git worktree list 실패 — 로컬 %s 가 체크아웃된 폴더를 모른다. 같은 줄을 다시", base)
	}
	at, w := "", ""
	for _, l := range lines(wl) {
		if strings.HasPrefix(l, "worktree ") {
			w = l[len("worktree "):]
		}
		if l == "branch "+bref {
			at = w
			break
		}
	}
	var ok bool
	if at != "" {
		ok = runCmd(cmdOpt{stdout: os.Stdout, stderr: os.Stderr, env: []string{"HERDR_LAND=1"}, noLimit: true}, "git", "-C", at, "merge", "--ff-only", "--no-squash", "-q", nw) == 0
	} else {
		ok = gitRun("merge-base", "--is-ancestor", old, nw) == 0 && gitRun("update-ref", bref, nw, old) == 0
	}
	if ok {
		m := ""
		if at != "" {
			m = " (폴더 " + at + ")"
		}
		fmt.Printf("hc-lane land: 착지 %s → 로컬 %s%s\n", cut(nw, 9), base, m)
		if lvd != "" {
			lverdictDone("")
		}
		return true
	}
	ia := gitRun("merge-base", "--is-ancestor", bref, nw)
	if ia == 0 {
		m := ""
		if at != "" {
			m = "(폴더 " + at + " 에 커밋 안 된 변경?)"
		}
		die("land: 로컬 %s 를 fast-forward로 올리지 못했다%s — 위 git 출력을 본다", base, m)
	}
	if ia != 1 {
		die("land: 로컬 %s 와 견주지 못했다(git merge-base 종료 %d) — 같은 줄을 다시", base, ia)
	}
	return false
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// countNonEmpty: grep -c .
func countNonEmpty(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			n++
		}
	}
	return n
}

// mgon: 원격 refs/herdr/merger/<기계>가 있으면 land는 제출하고 그 기계가 착지한다. 0 켜짐(mg에 기계), 1 꺼짐, 2 모름
func mgon() int {
	switch gitRunQ("remote", "get-url", "origin") {
	case 0:
	case 2:
		return 1
	default:
		eprintf("hc-lane land: 원격 설정(git remote get-url origin)을 읽지 못해 병합기가 켜졌는지 모른다 — 착지하지 않았다. 같은 줄을 다시\n")
		return 2
	}
	var mgt string
	for _, w := range waitsEnv("HC_LANE_MGON_WAITS") {
		var rc int
		if mgt, rc = gitOutQ("ls-remote", "origin", "refs/herdr/merger/*"); rc == 0 {
			break
		}
		if w == "-" {
			eprintf("hc-lane land: 원격을 읽지 못해 병합기가 켜졌는지 모른다 — 착지하지 않았다. 같은 줄을 다시\n")
			return 2
		}
		sleepS(w)
	}
	mg = ""
	for _, l := range lines(mgt) {
		if f := strings.Fields(l); len(f) == 2 {
			mg = strings.TrimPrefix(f[1], "refs/herdr/merger/")
			break
		}
	}
	if mg != "" {
		return 0
	}
	return 1
}

// tclean: 색인·추적 파일이 HEAD와 같은가. git status가 실패하면 「깨끗함」으로 읽지 않고 멈춘다
func tclean(when string) bool {
	st, rc := gitOut("status", "--porcelain", "--untracked-files=no")
	if rc != 0 {
		die("land: %s git status 실패 — 작업 트리를 모른다, 보내지 않았다. 같은 줄을 다시", when)
	}
	return st == ""
}

// committedHook: 실행하는 훅은 그 커밋의 파일이어야 한다(status는 새 파일과 assume-unchanged 파일을 놓친다)
func committedHook(rev, path string) {
	want, rc := gitOut("rev-parse", "--verify", rev+":"+path)
	if rc == 0 {
		var actual string
		if actual, rc = gitOut("hash-object", "--path="+path, "--", path); rc == 0 && actual == want {
			return
		}
	}
	die("land: %s 가 커밋 %s 의 파일과 일치하지 않아 실행하지 않았다", path, rev)
}

// landKnown: 기본 브랜치에 이미 들어간 경우에도 이 실행의 임대가 남아 있으면 표식과 함께 끝낸다
func landKnown(sha string) int {
	lease := os.Getenv("HC_LANE_LEASE")
	name, ok := lmname()
	if lease == "" || !ok {
		return 0
	}
	ref := "refs/hc-lanes/" + name
	rows, rc := gitOut("ls-remote", "origin", ref)
	if rc != 0 {
		eprintf("hc-lane land: 착지한 레인의 임대 조회 실패\n")
		return 75
	}
	if field2(rows, ref) != lease {
		return 0
	}
	marker := "refs/hc-landed/" + name + "/" + unixNow()
	if gitRun("push", "-q", "--atomic", "origin", sha+":"+marker, ":"+ref, "--force-with-lease="+ref+":"+lease) != 0 {
		return 75
	}
	return 0
}

// landAfter: main push 뒤 — 표식이 그 push에 못 실렸으면 따로, 임대가 남았으면 놓는다. 못 하면 알리기만(착지는 됨)
func landAfter(b, lmr string) {
	if mgx == "1" {
		return
	}
	if lmr != "" {
		la, _ := gitOutQ("ls-remote", "origin", lmr)
		if la == "" && !(gitRunQ("fetch", "-q", "origin") == 0 && gitRunQ("merge-base", "--is-ancestor", "HEAD", "refs/remotes/origin/"+b) == 0 &&
			gitRunQ("push", "-q", "origin", "HEAD:"+lmr) == 0) {
			eprintf("hc-lane land: 착지 표식 %s 를 두지 못했다(착지는 됨 — 기다리는 레인은 커밋 제목 규칙으로만 풀린다)\n", lmr)
		}
	}
	lease := os.Getenv("HC_LANE_LEASE")
	lmn, ok := lmname()
	if lease == "" || !ok {
		return
	}
	ld, rc := gitOutQ("ls-remote", "origin", "refs/hc-lanes/"+lmn)
	if rc != 0 {
		eprintf("hc-lane land: 임대를 확인하지 못했다(착지는 됨) — hc-lane clean이 놓는다\n")
		return
	}
	if firstWord(ld) != lease {
		return
	}
	if gitRunQ("push", "-q", "origin", "--force-with-lease=refs/hc-lanes/"+lmn+":"+lease, ":refs/hc-lanes/"+lmn) == 0 {
		fmt.Printf("hc-lane land: 임대 놓음 refs/hc-lanes/%s\n", lmn)
	} else {
		eprintf("hc-lane land: 임대를 놓지 못했다(착지는 됨) — hc-lane clean이 놓는다\n")
	}
}
