package main

// clean(cleanMain)의 단계들 — 차례는 cleanMain 한 곳, 단계마다 입력·출력을 인자·반환으로.
// 판정만 하는 것(wtBlock·wtBlockProblem·submitHit·deleteOnly·mainWorktree·qaRead)은 읽기·쓰기 없이 단위 시험(cleansteps_test.go).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cleanHere: 지금 폴더(심볼릭 링크 풂) — 끝에 「지운 폴더 안이었다」 알림용
func cleanHere() string {
	here, _ := os.Getwd()
	if h, err := filepath.EvalSymlinks(here); err == nil {
		here = h
	}
	return here
}

// wtBlock: git worktree list --porcelain에서 작업 폴더 wt의 블록(「worktree …」 줄 뒤 다음 「worktree …」 전까지)과 등록 여부
func wtBlock(wl, wt string) ([]string, bool) {
	var blk []string
	reg, f := false, false
	for _, ln := range strings.Split(wl, "\n") {
		if ln == "worktree "+wt {
			reg, f = true, true
			continue
		}
		if f && strings.HasPrefix(ln, "worktree ") {
			f = false
		}
		if f {
			blk = append(blk, ln)
		}
	}
	return blk, reg
}

// blkVal: 블록에서 머리 k로 시작하는 첫 줄의 나머지(없으면 빈 값)
func blkVal(blk []string, k string) string {
	for _, ln := range blk {
		if strings.HasPrefix(ln, k) {
			return strings.TrimPrefix(ln, k)
		}
	}
	return ""
}

// wtBlockProblem: 등록된 작업 폴더가 이 브랜치의 것인지·잠기지 않았는지(br: 브랜치가 있음). 문제면 멈출 글, 없으면 빈 값
func wtBlockProblem(blk []string, br bool, branch, wt string) string {
	wb := blkVal(blk, "branch ")
	if wb != "" && wb != "refs/heads/"+branch {
		return "이 폴더는 다른 브랜치(" + strings.TrimPrefix(wb, "refs/heads/") + ")의 워크트리다: " + wt
	}
	if wb == "" && br {
		return "이 폴더는 분리 HEAD 워크트리인데 브랜치 " + branch + " 는 따로 있다(다른 레인의 폴더일 수 있다): " + wt
	}
	for _, ln := range blk {
		if strings.HasPrefix(ln, "locked") {
			return "잠긴 워크트리다(git worktree unlock 뒤 다시): " + wt
		}
	}
	return ""
}

// submitHit: ls-remote 줄 가운데 커밋 s를 가리키는 이 레인(sub)의 큐 표식 refs/merge-queue/<초>-<sub>(첫 것, 없으면 빈 값)
func submitHit(lrr []string, s, sub string) string {
	for _, ln := range lrr {
		fs := strings.Fields(ln)
		if len(fs) != 2 || fs[0] != s {
			continue
		}
		if m := cleanSubmitTell.FindStringSubmatch(fs[1]); m != nil && m[1] == sub {
			return fs[1]
		}
	}
	return ""
}

// laneMarks: 제출·주차·착지 표식(원격) — 처음 필요할 때 한 번 읽는다
type laneMarks struct {
	looked                                      bool
	lrr                                         []string
	receipt, receiptInfo, doneRef, parkRef, sub string
}

// load: 레인 소유·표식 이름을 정하고 원격 표식을 읽는다
func (m *laneMarks) load(lab string) {
	m.looked = true
	if !cleanOwner() {
		stop("레인 소유 기록 조회 실패: " + branch)
	}
	// 표식 이름: land·park가 가지 설정에 남긴 낸 이름(HC_LANE=<원래>로 낸 다시 맞춤·고침 레인), 없으면 레인 이름
	m.sub = owner
	switch v, r := xout("git", "-C", repo, "config", "--get", "branch."+branch+".hcLane"); {
	case r == 0 && lnameOK(v):
		m.sub = v
	case r != 1:
		stop("가지 설정 branch." + branch + ".hcLane 조회 실패(또는 꼴이 틀림): " + v)
	}
	m.doneRef, m.parkRef = "refs/merge-done/"+m.sub, "refs/park/"+m.sub
	out, r := xout("git", "-C", repo, "ls-remote", "origin", "refs/merge-queue/*", m.parkRef, m.doneRef, "refs/heads/lane/"+m.sub)
	if r != 0 {
		stop("git ls-remote 실패: " + lab + " · refs/merge-queue/*-" + m.sub)
	}
	m.lrr = strings.Split(out, "\n")
	for _, ln := range m.lrr {
		if fs := strings.Fields(ln); len(fs) == 2 && fs[1] == m.doneRef {
			m.receipt = fs[0]
		}
	}
}

// kept: 기준에 안 들어간 c를 레인 자신의 원격 표식(제출·주차·옛 판 주차·착지 완료 기록)이 쥐고 있나 — 쥐면 그 줄을 찍고 true
func (m *laneMarks) kept(c, lab, P string) bool {
	s, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", c+"^{commit}")
	if r != 0 {
		stop("커밋을 읽지 못했다: " + lab)
	}
	if hit := submitHit(m.lrr, s, m.sub); hit != "" {
		fmt.Printf("%s제출됨: %s 은 큐 표식 %s 에 그대로 있다(병합기가 넣는다)\n", P, lab, hit)
		return true
	}
	if contains(m.lrr, s+"\t"+m.parkRef) {
		fmt.Printf("%s주차됨: %s 은 원격 %s 에 그대로 있다(선행이 들어가면 hc-fill이 이어 띄운다)\n", P, lab, m.parkRef)
		return true
	}
	// 옛 판 주차(원격 lane/<레인>): 이 레인의 마지막 주차·제출 사건이 같은 SHA의 「주차」일 때만 — 반송된 제출 가지와 가른다.
	// --resume은 refs/park가 없으면 lane/<레인>을 받는다(gofix L07)
	if contains(m.lrr, s+"\trefs/heads/lane/"+m.sub) && legacyParked(m.sub, s) {
		fmt.Printf("%s주차됨: %s 은 원격 refs/heads/lane/%s(옛 판 주차)에 그대로 있다\n", P, lab, m.sub)
		return true
	}
	// 병합기가 먼저 rebase해 넣으면 원본은 완료 기록의 부모로 남는다
	if m.receipt == "" {
		return false
	}
	if m.receiptInfo == "" {
		if xrun(nil, nil, "git", "-C", repo, "cat-file", "-e", m.receipt+"^{commit}") != 0 {
			if dry {
				stop("원격 완료 기록을 받아야 판정할 수 있다: " + m.doneRef)
			}
			if xfetch(repo, "-q", "--no-tags", "--no-write-fetch-head", "origin", m.doneRef) != 0 {
				stop("완료 기록 받기 실패: " + m.doneRef)
			}
		}
		if m.receiptInfo, r = xout("git", "-C", repo, "show", "-s", "--format=%P %s", m.receipt); r != 0 {
			stop("완료 기록 조회 실패: " + m.receipt)
		}
	}
	if strings.HasPrefix(m.receiptInfo, s+" merge-queue lane="+m.sub+" ") && strings.Contains(m.receiptInfo, " orig="+s+" ") {
		fmt.Printf("%s착지됨: %s 에 %s 의 원본이 있다\n", P, m.doneRef, lab)
		return true
	}
	return false
}

// mergedCheck: 1) 브랜치와 작업 폴더 HEAD가 기준에 들어갔는지(폴더가 없어도 등록의 HEAD로 본다).
// 반환: --archive로 보관할 커밋, 브랜치 끝, 제출·주차·착지 표식이 쥐었나
func mergedCheck(blk []string, reg, br bool, P string) (arch []string, btip string, subm bool) {
	if remote {
		if dry {
			fmt.Printf("[dry-run] git fetch 건너뜀: 지금 있는 origin/%s 로 판정\n", base)
		} else if xfetch(repo, "-q", "origin") != 0 {
			stop("git fetch 실패: " + repo)
		}
	}
	if xrun(nil, os.Stderr, "git", "-C", repo, "rev-parse", "-q", "--verify", bref+"^{commit}") != 0 {
		stop(blab + " 가 없다: " + repo)
	}
	var targets []string
	if br {
		targets = append(targets, "refs/heads/"+branch)
	}
	if reg {
		h := blkVal(blk, "HEAD ")
		if h == "" {
			stop("워크트리 HEAD를 읽지 못했다: " + wt)
		}
		targets = append(targets, h)
	}
	if br {
		var r int
		if btip, r = xout("git", "-C", repo, "rev-parse", "-q", "--verify", "refs/heads/"+branch+"^{commit}"); r != 0 {
			stop("브랜치를 읽지 못했다: " + branch)
		}
	}
	var m laneMarks
	for _, c := range targets {
		lab := "워크트리 HEAD " + c[:min(12, len(c))]
		if strings.HasPrefix(c, "refs/heads/") {
			lab = "브랜치 " + branch
		}
		ia := xrun(nil, os.Stderr, "git", "-C", repo, "merge-base", "--is-ancestor", c, bref)
		if ia == 0 {
			continue
		}
		// 제출·주차한 레인 자신의 원격 표식이 원본을 쥘 때만 정리한다
		if ia == 1 && remote && br {
			if !m.looked {
				m.load(lab)
			}
			if m.kept(c, lab, P) {
				subm = true
				continue
			}
		}
		if ia == 1 && archive {
			if !remote {
				stop("원격(origin)이 없어 --archive로 보관할 수 없다: " + repo)
			}
			s, r := xout("git", "-C", repo, "rev-parse", "-q", "--verify", c+"^{commit}")
			if r != 0 {
				stop("커밋을 읽지 못했다: " + lab)
			}
			if !contains(arch, s) {
				arch = append(arch, s)
			}
			continue
		}
		lg, _ := xout("git", "-C", repo, "log", "--oneline", "-n", "10", bref+".."+c)
		stop(lab+" 이(가) "+blab+" 에 들어가지 않았다(착지 전이거나 다른 커밋). 착지 절차의 hc-lane land부터 다시 한다(다른 곳에서 rebase돼 착지했으면 --archive)", lg)
	}
	return arch, btip, subm
}

// deleteOnly: git status --porcelain 출력(빈 값 아님)이 추적 파일 지움(「 D 」)뿐인가와 그 수
func deleteOnly(st string) (int, bool) {
	ls := strings.Split(st, "\n")
	n := 0
	for _, ln := range ls {
		if strings.HasPrefix(ln, " D ") {
			n++
		}
	}
	return n, n == len(ls)
}

// wtLeftovers: 2) 지울 폴더에 추적 변경·무시되지 않은 untracked가 없는지. 추적 파일이 지워지기만 했으면(정리가 중간에 끊김) true — 이어서 지운다
func wtLeftovers(reg, dir bool, common, P string) bool {
	if !reg {
		if dir {
			leftCheck("등록이 지워진", common)
		}
		return false
	}
	if !dir {
		return false
	}
	delOnly := false
	st, r := xout("git", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if r != 0 {
		stop("git status 실패: " + wt)
	}
	if st != "" {
		n, ok := deleteOnly(st)
		if !ok {
			stop("워크트리에 커밋 안 된 변경이나 무시되지 않은 untracked가 있다: "+wt, st)
		}
		delOnly = true
		fmt.Printf("%s정리가 끊긴 자리: 추적 파일 %d개가 이미 지워졌다(변경은 지움뿐) — 이어서 지운다\n", P, n)
	}
	igs, _ := xout("git", "-c", "core.quotePath=false", "-C", wt, "status", "--porcelain", "--ignored", "--untracked-files=normal")
	ig := ""
	for _, ln := range strings.Split(igs, "\n") {
		if strings.HasPrefix(ln, "!! ") {
			ig += strings.TrimPrefix(ln, "!! ") + " "
		}
	}
	if ig != "" {
		fmt.Printf("%s무시된 것도 같이 지운다: %s\n", P, ig)
	}
	return delOnly
}

// archivePush: 2.5) --archive — 안 들어간 커밋을 원격 refs/archive/<가지>-<sha8>에 보관. 하나라도 실패하면 아무것도 지우지 않는다
func archivePush(arch []string) {
	for _, s := range arch {
		ar := "refs/archive/" + branch + "-" + s[:min(8, len(s))]
		if dry {
			fmt.Printf("[dry-run] git -C %s push origin %s:%s\n", q(repo), s, ar)
			continue
		}
		if zx(os.Stderr, os.Stderr, repo, "git", "push", "-q", "origin", s+":"+ar) != 0 {
			stop("보관 push 실패: " + ar + " (" + s + ")")
		}
		fmt.Printf("보관 %s (%s) — 되살리기: git fetch origin %s\n", ar, s, ar)
	}
}

// mainWorktree: worktree list의 첫 작업 트리(메인 폴더). bare이거나 그것이 wt 자신이면 빈 값
func mainWorktree(wl, wt string) string {
	mw := ""
	for i, ln := range strings.Split(wl, "\n") {
		if ln == "" {
			break
		}
		if i == 0 && len(ln) >= 9 {
			mw = ln[9:]
		}
		if strings.HasPrefix(ln, "bare") {
			mw = ""
		}
	}
	if mw == wt {
		mw = ""
	}
	return mw
}

// qaRead: .qa 보관 결과 줄(「c|s|d|f|n <경로>」) → .qa 폴더(정렬)·줄 수·새로·이미 같음·못 옮긴 것. 줄 꼴이 틀리면 bad에 그 줄(보관 결과를 모른다, R-11)
func qaRead(qa string) (ds []string, n, nc, ns int, dq []string, bad string) {
	ls := strings.Split(qa, "\n")
	seen := map[string]bool{}
	for _, l := range ls {
		if len(l) < 3 || l[1] != ' ' || !strings.ContainsRune("csdfn", rune(l[0])) {
			return nil, 0, 0, 0, nil, l
		}
		p := l[2:]
		if m := qaDirRe.FindStringSubmatch(p); m != nil {
			p = m[1]
		}
		if !seen[p] {
			seen[p] = true
			ds = append(ds, p)
		}
		switch l[0] {
		case 'c':
			nc++
		case 's':
			ns++
		case 'd':
			dq = append(dq, "다름(메인 것 둠) "+l[2:])
		case 'f':
			dq = append(dq, "복사 실패 "+l[2:])
		}
	}
	sort.Strings(ds)
	return ds, len(ls), nc, ns, dq, ""
}

// qaMove: 2.7) 무시된 보관물 .qa — 지우기 전 메인 폴더(첫 작업 트리)의 같은 자리로 복사(덮지 않음), 모두 같을 때만 지운다
func qaMove(wl string, reg bool) {
	mw := mainWorktree(wl, wt)
	gd := repo
	if reg {
		gd = wt
	}
	d := "0"
	if dry {
		d = "1"
	}
	qa, r := xout("sh", "-c", qaScript, "hc-lane", wt, mw, gd, d)
	switch r {
	case 0:
	case 3:
		stop("메인 폴더를 찾지 못해 무시된 .qa를 옮길 수 없다(손으로 옮긴 뒤 다시): " + wt)
	default:
		m := mw
		if m == "" {
			m = "?"
		}
		stop("무시된 .qa를 메인 폴더로 옮기지 못했다: " + wt + " → " + m)
	}
	if qa == "" {
		return
	}
	ds, n, nc, ns, dq, bad := qaRead(qa)
	if bad != "" || n == 0 {
		stop("무시된 .qa 보관 결과를 읽지 못했다(줄 꼴): "+wt, bad)
	}
	var qp []string
	for _, p := range ds {
		qp = append(qp, wt+"/"+p+" → "+mw+"/"+p)
	}
	if dry {
		fmt.Printf("[dry-run] 무시된 보관물 복사(덮지 않음) 뒤 지움: %s (파일 %d개)\n", strings.Join(qp, ", "), n)
		return
	}
	if len(dq) > 0 {
		stop("무시된 .qa 파일 일부를 메인 폴더로 옮기지 못했다(같은 이름의 다른 파일은 덮지 않는다) — 비교·정리한 뒤 다시. 아무것도 안 지웠다: "+wt, strings.Join(dq, "\n"))
	}
	fmt.Printf("옮김 %s (파일 %d개 — 새로 %d, 이미 같음 %d)\n", strings.Join(qp, ", "), n, nc, ns)
	addDid(".qa 메인 폴더로 복사")
}

// teardownStep: 3) teardown(있으면, 실패해도 계속) — 전역 td·did. 돌린 뒤 제거 직전 터미널을 다시 확인
func teardownStep(reg, dir bool) {
	if !reg || !dir || !xtest("-f", wt+"/.claude/herdr-teardown.sh") {
		return
	}
	if dry {
		fmt.Println("[dry-run] 워크트리에서 bash .claude/herdr-teardown.sh")
		td = "dry-run"
	} else if r, lg := hook("teardown"); r == 0 {
		td, did = "ran", "teardown"
	} else {
		tailLog(lg)
		errf("hc-lane clean: herdr-teardown.sh 실패(위는 기록 끝 20줄), 정리는 계속한다\n")
		td, did = "failed", "teardown"
	}
	if !dry {
		termGate("teardown 뒤 제거 직전 ")
	}
}

// removeWorktree: worktree remove — 등록으로 판정한다
func removeWorktree(delOnly bool) {
	if dry {
		fmt.Printf("[dry-run] git -C %s worktree remove %s\n", q(repo), q(wt))
		return
	}
	// 무시된 파일 때문에 "Directory not empty"로 끝나도 등록은 지워질 수 있다. 등록으로 판정한다.
	// 변경이 지움뿐(끊긴 정리)이면 git이 「modified」로 거절하니 --force — 위에서 지움뿐임을 확인했다
	ra := []string{"git", "-C", repo, "worktree", "remove", wt}
	if delOnly {
		ra = []string{"git", "-C", repo, "worktree", "remove", "--force", wt}
	}
	xrun(os.Stderr, os.Stderr, ra...)
	wl2, r := xout("git", "-C", repo, "worktree", "list", "--porcelain")
	if r != 0 {
		stop("git worktree list 실패: " + repo)
	}
	if contains(strings.Split(wl2, "\n"), "worktree "+wt) {
		stop("git worktree remove 실패: " + wt)
	}
	addDid("워크트리 등록 제거")
}

// removeLeftDir: 남은 무시 파일 폴더(등록이 없거나 remove 뒤 남은 것)
func removeLeftDir(reg, dir bool, common string) {
	if dry {
		if !reg && dir {
			fmt.Printf("[dry-run] rm -rf -- %s  # 무시된 파일만 남은 폴더\n", q(wt))
		}
		return
	}
	if !exists() {
		return
	}
	leftCheck("남은", common)
	if xrun(os.Stdout, os.Stderr, "rm", "-rf", "--", wt) != 0 {
		stop("남은 폴더를 지우지 못했다: " + wt)
	}
	addDid("남은 무시 파일 폴더 삭제")
}

// removeBranch: branch -d. 보관·제출했으면 병합 여부 대신 확인한 값(btip) 그대로일 때만 지운다
func removeBranch(kept bool, btip string) {
	if dry {
		fmt.Printf("[dry-run] git -C %s branch -d %s\n", q(repo), branch)
		return
	}
	if kept {
		if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "update-ref", "-d", "refs/heads/"+branch, btip) != 0 {
			stop("브랜치를 지우지 못했다(보관 뒤 값이 바뀌었나?): " + branch)
		}
	} else if xrun(os.Stderr, os.Stderr, "git", "-C", repo, "branch", "-q", "-d", branch) != 0 {
		stop("git branch -d 실패: " + branch)
	}
	addDid("브랜치 삭제")
}

// cleanSummary: 끝 줄(정리 끝·dry-run 계획)과 셸 위치 알림
func cleanSummary(here string) {
	tdk := "돌릴 예정"
	switch td {
	case "ran":
		tdk = "통과"
	case "none":
		tdk = "없음(건너뜀)"
	case "failed":
		tdk = "실패(정리는 계속)"
	}
	if dry {
		fmt.Printf("[dry-run] 정리할 것: 워크트리 %s, 브랜치 %s (teardown: %s)\n", wt, branch, tdk)
	} else {
		at := ""
		if on != "" {
			at = " @" + on
		}
		fmt.Printf("정리 끝: 워크트리 %s, 브랜치 %s%s (teardown: %s)\n", wt, branch, at, tdk)
	}
	if on == "" && !dry {
		if strings.HasPrefix(os.Getenv("PWD")+"/", wt+"/") || strings.HasPrefix(here+"/", wt+"/") {
			fmt.Printf("알림: 지금 셸 위치는 지운 폴더 안이었다. 다음 명령 전에 cd %s\n", q(repo))
		}
	}
}
