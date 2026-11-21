package main

// park(2026-11-20 사용자 「노는 워크트리 없이」): 선행을 기다리는 레인이 깨끗한 작업 트리의 HEAD를 원격 refs/park/<레인>(지금 값에 lease)에
// 올리고, 같은 --atomic push로 이 레인 임대(HC_LANE_LEASE 값일 때만)를 놓은 뒤 「주차」 사건. 제출 가지 lane/<레인>과 다른 이름이라
// 병합기·clean이 주차를 제출로 읽지 않는다(2026-11-21 hc-lane-go-start). 종료 0 주차, 1 실패(같은 줄을 다시), 3 원격 없음

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"herdr-tools/internal/proc"
)

func lsVal(ls, ref string) (string, bool) {
	for _, ln := range strings.Split(ls, "\n") {
		if f := strings.Fields(ln); len(f) == 2 && f[1] == ref {
			return f[0], true
		}
	}
	return "", false
}

func parkMain(args []string) int {
	cmdName = "park"
	startSetup()
	pmsg := strings.Join(args, " ")
	git := func(a ...string) (string, int) { return outOf(os.Stderr, append([]string{"git"}, a...)) }
	gitQ := func(a ...string) (string, int) { return outOf(nil, append([]string{"git"}, a...)) }
	tp, r := gitQ("rev-parse", "--show-toplevel")
	if r != 0 || os.Chdir(tp) != nil {
		die("park: 레인 작업 폴더 안에서 부른다: %s", os.Getenv("PWD"))
	}
	ln := os.Getenv("HC_LANE")
	if ln == "" {
		ln, _ = gitQ("branch", "--show-current")
	}
	if ln == "" || !nameCharsOK(ln) {
		die("park: 레인 이름을 모른다(HC_LANE 또는 레인 가지)")
	}
	switch code(proc.Run(exec.Command("git", "remote", "get-url", "origin"))) {
	case 0:
	case 2:
		errf("hc-lane park: 원격(origin)이 없어 주차하지 못한다 — 지시문의 기다림 블록으로\n")
		return 3
	default:
		die("park: git remote get-url 실패")
	}
	for _, f := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD"} {
		gp, r := git("rev-parse", "--git-path", f)
		if r != 0 || gp == "" {
			die("park: git rev-parse --git-path 실패 — 진행 중인 rebase·merge가 있는지 모른다")
		}
		if _, err := os.Lstat(gp); err == nil {
			die("park: 진행 중인 rebase·merge가 있다(%s). 마친 뒤 다시", f)
		}
	}
	st, r := git("status", "--porcelain", "--untracked-files=normal")
	if r != 0 {
		die("park: git status 실패 — 작업 트리를 모른다")
	}
	if st != "" {
		die("park: 커밋 안 된 변경·새 파일이 있다(git status) — 준비를 커밋한 뒤 다시")
	}
	sh, r := git("rev-parse", "-q", "--verify", "HEAD")
	if r != 0 {
		die("park: HEAD를 읽지 못했다")
	}
	pr, pl := "refs/park/"+ln, "refs/hc-lanes/"+ln
	rs, r := git("ls-remote", "origin", pr, pl)
	if r != 0 {
		die("park: 원격을 읽지 못했다(git ls-remote) — 같은 줄을 다시")
	}
	cur, _ := lsVal(rs, pr)
	lv, _ := lsVal(rs, pl)
	lease := os.Getenv("HC_LANE_LEASE")
	if lv != lease && !(lv == "" && cur == sh) {
		die("park: 이 실행의 임대가 아니다 — 주차하지 않았다")
	}
	push := []string{"push", "-q", "--atomic", "--porcelain", "origin", sh + ":" + pr, "--force-with-lease=" + pr + ":" + cur}
	dl := lease != "" && lv == lease
	if dl {
		push = append(push, ":"+pl, "--force-with-lease="+pl+":"+lv)
	}
	po := ""
	if cur != sh || dl {
		c := exec.Command("git", push...)
		b, _ := proc.Combined(c) // 결과는 아래 ls-remote로 다시 본다
		po = string(b)
	}
	rb, r := git("ls-remote", "origin", pr, pl)
	if r != 0 {
		die("park: 원격을 다시 읽지 못했다 — 같은 줄을 다시(주차는 같은 값으로 모인다)")
	}
	if v, _ := lsVal(rb, pr); v != sh {
		errf("%s\n", trim(po))
		die("park: push 실패 — %s 이 이 커밋이 아니다. 같은 줄을 다시", pr)
	}
	lk := "임대 없음"
	if lease != "" {
		lk = "임대 남음(다른 값) — hc-lane clean이 놓는다"
	}
	if dl {
		if _, ok := lsVal(rb, pl); !ok {
			lk = "임대 놓음"
		} else {
			lk = "임대를 놓지 못했다 — hc-lane clean이 놓는다"
		}
	}
	msg := pr + " — 선행 착지 뒤 hc-fill이 이 가지에서 다시 띄운다"
	if pmsg != "" {
		msg += " · " + pmsg
	}
	rep := exec.Command(selfPath, "report", "주차", sh, msg, "--name", ln)
	rep.Stderr = os.Stderr
	if rep.Run() != nil {
		die("park: 주차 사건을 쓰지 못했다(가지는 %s 에 있다) — 같은 줄을 다시", pr)
	}
	short, _ := gitQ("rev-parse", "--short", sh)
	fmt.Printf("hc-lane park: 주차 %s → %s (%s). 다음: 착지·정리 절차의 hc-lane clean 줄 → 턴을 끝낸다\n", short, pr, lk)
	return 0
}
