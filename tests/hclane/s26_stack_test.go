package hclane_test

// 쌓아 띄움(docs/hc-fill/stack.md)의 띄우기 길을 진짜 git으로(hc만 가짜): HC_LANE_STACK=<선행>@<P>면 원격 lane/<선행>을 받아 P에서
// 작업 트리를 만든다. 2026-11-27 ht-stack-launch-track — 시작이 가지가 아닌 참조(refs/hc-stack/<레인>)인데 --track을 붙여
// 「cannot set up tracking information」으로 레인이 못 떴고 그 참조가 클론에 남았다(옛 시험은 띄우기 전까지만 돌았다)

import "strings"

func init() { reg(92, "s26-stack", s26Stack) }

// s26pred: seed 클론에서 기본 가지 위에 커밋 하나(file=content)를 지어 원격 lane/<선행>으로 올린다. 돌려준 값은 그 커밋 P
func s26pred(k *K, pred, file, content string) string {
	k.t.Helper()
	sd := k.D + "/seed"
	k.sh(sd, `git checkout -q --detach && mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$3: 선행" -- "$1" &&
		git push -q origin "HEAD:refs/heads/lane/$3" && git checkout -q "$4"`, file, content, pred, k.B)
	return k.git(k.D+"/origin.git", "rev-parse", "refs/heads/lane/"+pred)
}

func s26noref(k *K, name string) bool {
	return !k.gitOK(k.R, "rev-parse", "-q", "--verify", "refs/hc-stack/"+name)
}

func s26Stack(k *K) {
	// ① P에서 작업 트리, upstream은 기본 가지, 레인 환경 HC_LANE_STACK, 참조는 남지 않음, 착지 뒤 branch -d가 통함
	k.mk("t26")
	p := s26pred(k, "st-a", "p.txt", "선행")
	k.runE([]string{"HC_LANE_STACK=st-a@" + p}, "--name", "st-b", "--repo", k.R, "--", "claude")
	wt := k.R + ".wt/st-b"
	k.check("쌓아 띄움: 작업 트리가 선행 승인 커밋 P에서(진짜 git worktree add), 종료 0", k.rc == 0 && isDir(wt) && k.git(wt, "rev-parse", "HEAD") == p)
	k.check("쌓아 띄움: upstream은 origin/main", k.git(wt, "rev-parse", "--abbrev-ref", "@{u}") == "origin/main")
	k.check("쌓아 띄움: 레인 환경에 HC_LANE_STACK=<선행>@<P>", k.has("HC_LANE_STACK=st-a@"+p))
	k.check("쌓아 띄움: 띄운 뒤 refs/hc-stack/<레인>이 남지 않음", s26noref(k, "st-b"))
	k.sh(wt, `echo b >b2.txt && git add -- b2.txt && git commit -qm st-b -- b2.txt && git push -q origin HEAD:main && git fetch -q origin`)
	k.sh(k.R, `git worktree remove --force "$1"`, wt)
	k.check("쌓아 띄움: 선행·자기 커밋이 착지한 뒤 git branch -d가 통함(upstream 기준)", k.gitOK(k.R, "branch", "-d", "st-b"))

	// ② --resume + 쌓아 띄움: 주차 가지에서(레인이 P 위로 rebase), upstream은 origin/main
	k.mk("t26r")
	p = s26pred(k, "st-a", "p.txt", "선행")
	k.sh(k.D+"/seed", `git checkout -q --detach && echo c >c.txt && git add -- c.txt && git commit -qm "st-c: 준비" -- c.txt && git push -q origin HEAD:refs/park/st-c && git checkout -q "$1"`, k.B)
	ps := k.git(k.D+"/origin.git", "rev-parse", "refs/park/st-c")
	k.runE([]string{"HC_LANE_STACK=st-a@" + p}, "--resume", "--name", "st-c", "--repo", k.R, "--", "claude")
	wt = k.R + ".wt/st-c"
	k.check("--resume + 쌓아 띄움: 주차 가지에서 작업 트리, upstream origin/main, 참조 안 남음", k.rc == 0 && k.git(wt, "rev-parse", "HEAD") == ps &&
		k.git(wt, "rev-parse", "--abbrev-ref", "@{u}") == "origin/main" && k.has("HC_LANE_STACK=st-a@"+p) && s26noref(k, "st-c"))

	// ③ 작업 트리를 만든 뒤 단계(준비 훅)가 실패하면 작업 트리·가지·refs/hc-stack/<레인>이 모두 없다
	k.mk("t26f")
	p = s26pred(k, "st-a", ".claude/herdr-setup.sh", "exit 5")
	k.runE([]string{"HC_LANE_STACK=st-a@" + p}, "--name", "st-f", "--repo", k.R, "--", "claude")
	k.check("쌓아 띄움 준비 훅 실패: 띄우지 않고 작업 트리·가지·참조를 모두 지움", k.rc != 0 && !exists(k.R+".wt/st-f") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/st-f") && s26noref(k, "st-f") && !strings.Contains(strings.Join(k.hclog(""), "\n"), "st-f") &&
		strings.Contains(k.errf(), "지웠다"))

	// ④ 선행 가지가 P가 아니면 거절(종료 11), 아무것도 안 남음
	k.mk("t26x")
	s26pred(k, "st-a", "p.txt", "선행")
	other := k.git(k.R, "rev-parse", "origin/main")
	k.runE([]string{"HC_LANE_STACK=st-a@" + other}, "--name", "st-x", "--repo", k.R, "--", "claude")
	k.check("쌓아 띄움: 선행 가지가 P가 아니면 거절(11), 작업 트리·가지·참조 없음", k.rc == 11 && !exists(k.R+".wt/st-x") &&
		!k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/st-x") && s26noref(k, "st-x"))
}
