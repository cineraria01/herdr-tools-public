package main

// cotenant-guard Go 판 — Bash 명령 판정: bash_git·bash_write·analyze·check_bash (cotenant.go에서 옮김)

import (
	"strings"
)

const forcePushMsg = "' 은 강제 push다. 원격에 있던 남의 커밋을 지울 수 있어 동거와 상관없이 늘 막는다. 대신 새 이름의 참조로 push하거나(git push origin HEAD:refs/<새 이름>), 그 참조를 git push origin --delete <참조> 로 지운 뒤 다시 push하라. 기대값을 적은 --force-with-lease=<참조>:<sha>(없을 때만 만들기는 <참조>: 로 빈 값)는 비교 후 교체라 된다. 그 참조를 다른 레인·사람이 쓰거나 판단이 서지 않으면 지휘(또는 사람)에게 알려라. 이 규칙을 끄는 것은 사람만 한다."

func (g *cg) bashGit(kind, dir, arg string) {
	if kind == "FORCEPUSH" {
		g.block("'" + arg + forcePushMsg)
	}
	if !g.phys(dir) || !g.tree(g.pd) {
		return
	}
	wt := "../" + baseName(g.root) + ".wt/land-<슬러그>"
	switch kind {
	case "ADDALL", "COMMITALL", "COMMITSTAGED", "AMEND", "DISCARD", "STASH", "RESETHARD", "CLEAN", "DYN":
		if !g.cotenant() {
			return
		}
		switch kind {
		case "ADDALL", "COMMITALL":
			g.block("'" + arg + "'은 같은 작업 트리의 다른 에이전트(" + g.who + ") 변경까지 담는다. 네 파일만 경로로 지정하라(git add -- <경로>), 커밋 전 git diff --cached로 확인.")
		case "COMMITSTAGED":
			g.block("경로 없는 '" + arg + "'은 같은 작업 트리의 다른 에이전트(" + g.who + ")가 그 사이 stage한 파일까지 커밋할 수 있어 막았다. 네 파일만 경로로 지정하라: git commit -m … -- <경로>")
		case "AMEND":
			g.block("'" + arg + "'은 같은 작업 트리의 다른 에이전트(" + g.who + ")가 그 사이 만든 커밋을 고쳐 쓸 수 있어 막았다. 새 커밋을 만들어라: git commit -m … -- <경로>")
		case "DYN":
			g.block("'" + arg + "'은 무엇을 담을지 미리 알 수 없어 같은 작업 트리에 다른 에이전트(" + g.who + ")가 있는 동안 막는다. 네 파일 경로를 글자로 적어라(git add -- <경로>).")
		default:
			g.block("'" + arg + "'은 같은 작업 트리의 다른 에이전트(" + g.who + ") 미커밋 변경을 지우거나 숨길 수 있어 막았다. 네 파일만 되돌리려면 그 파일을 Edit로 고치고, 트리 전체를 다루는 일은 자기 워크트리(git worktree add --detach " + wt + " HEAD)에서 하라.")
		}
	case "ADD", "DISCARDPATH":
		a := arg
		top, glob := false, false
		verb := "add"
		if kind == "DISCARDPATH" {
			verb = "checkout"
		}
		if strings.Contains(a, "$") {
			g.bashGit("DYN", dir, "git "+verb+" "+a)
			return
		}
		switch {
		case strings.HasPrefix(a, ":(") && strings.Contains(a[2:], ")"):
			m := a[:strings.IndexByte(a, ')')]
			m = strings.TrimPrefix(m, ":(")
			a = a[strings.IndexByte(a, ')')+1:]
			if strings.Contains(m, "exclude") {
				return
			}
			if strings.Contains(m, "top") {
				top = true
			}
			if !strings.Contains(m, "literal") && strings.ContainsAny(a, "*?[") {
				glob = true
			}
		case strings.HasPrefix(a, ":!") || strings.HasPrefix(a, ":^"):
			return
		case strings.HasPrefix(a, ":/"):
			a = a[2:]
			top = true
		case strings.ContainsAny(a, "*?["):
			glob = true
		}
		msg := "git add·checkout·commit에 넣지 말고 네 파일만 경로로 지정하라. 그 파일을 커밋하려면 그쪽이 직접 커밋하게 하거나, 그 에이전트 프로세스가 끝나 임대가 풀린 뒤에 하라."
		if glob {
			base := ""
			if !top {
				base = strings.TrimPrefix(strings.TrimPrefix(g.pd, g.root), "/")
			}
			a = strings.TrimPrefix(a, "./")
			if base != "" {
				a = base + "/" + a
			}
			g.foreignUnder(a, true, msg)
			return
		}
		if top {
			a = g.root + "/" + a
		}
		if !strings.HasPrefix(a, "/") {
			a = g.pd + "/" + a
		}
		if !g.relof(a) {
			return
		}
		g.foreignUnder(g.rel, false, msg)
	case "LAND":
		g.scanForeign()
		if g.fdirty == "" {
			return
		}
		g.block("다른 에이전트의 미커밋 파일(" + fdirtyNames(g.fdirty) + ")이 있는 작업 트리라 git " + arg + " 을 막았다. 커밋·rebase·검증·push(착지)는 따로 연 폴더에서 하라. 내 커밋만 올리려면 git worktree add --detach " + wt + " origin/<기본 브랜치> 뒤 그 폴더에서 git cherry-pick <내 커밋…> → 검증 → push. HEAD 기준(…add --detach " + wt + " HEAD)은 이 폴더의 미push 커밋을 모두 올리니 지휘가 모아 착지할 때만.")
	}
}

// bashWrite DIR ARG [noglob]: noglob = 글롭이 펼친 구체 경로라 다시 펼치지 않는다(셸 판도 같게 고침 — 2026-11-21 두 판 대조에서
// 발견: 다시 펼치면 닫히지 않은 [ 나 저를 다시 맞추는 이름에서 끝없이 되돌아 bash가 죽었다)
func (g *cg) bashWrite(dir, arg string, noglob bool) {
	a := arg
	if a == "" || strings.Contains(a, "$") || a == "-" {
		return
	}
	if !strings.HasPrefix(a, "/") {
		a = dir + "/" + a
	}
	if !noglob && strings.ContainsAny(a, "*?[") {
		// arg는 낱말 가르기가 낸 셸 낱말 하나라 안의 공백은 따옴표·\로 지킨 것 — 다시 가르지 않고 통째로 펼친다
		// (아스트라 E07: `tee "팀 문서"/*.txt`가 없는 「팀」에만 임대를 썼다. 셸 판 m=($a)는 그대로 가른다 — 알려진 차이)
		for _, f := range bashGlob(a) {
			g.bashWrite("/", f, true)
		}
		return
	}
	if !g.phys(a) || !g.tree(g.pd) {
		return
	}
	g.full = g.pd
	if g.prest != "" {
		g.full += "/" + g.prest
	}
	switch {
	case g.full == g.root:
		g.rel = ""
	case strings.HasPrefix(g.full, g.root+"/"):
		g.rel = g.full[len(g.root)+1:]
	default:
		return
	}
	if isDir(g.full) {
		what := g.rel
		if what == "" {
			what = "작업 트리 전체"
		}
		g.foreignUnder(g.rel, false, "'"+arg+"' 는 "+what+" 폴더를 바꾸므로 막았다. 그 파일을 빼고 네 파일만 경로로 지정하라.")
		return
	}
	g.lease(g.rel, "check")
	g.pend += g.pd + us + g.rel + "\n"
}

// analyze CMD CWD DEPTH REMOTE: 낱말 가르기의 찾은 것들 — SUB(bash -c·eval이 돌릴 줄)는 다시 가른다,
// RSUB(hc exec|run --on …, ssh <기계> …)는 다른 기계 명령이라 강제 push 규칙만, 그 안도 REMOTE
func (g *cg) analyze(cmd, cwd string, depth int, remote bool) {
	for _, l := range cgAnalyze(cmd, cwd, g.home) {
		f := bashRead(l, 0x1f, 3)
		kind, d, arg := f[0], f[1], f[2]
		switch kind {
		case "":
		case "FORCEPUSH":
			g.bashGit(kind, d, arg)
		case "SUB", "RSUB":
			if depth >= 5 {
				g.block("명령 겹침이 5겹을 넘어(bash -c·ssh·hc exec 등) 강제 push 여부를 판정할 수 없어 막았다. 겹침을 줄여 다시 하라.")
			}
			g.analyze(strings.ReplaceAll(arg, "\x1e", "\n"), d, depth+1, kind == "RSUB" || remote)
		case "WRITE":
			if !remote {
				g.bashWrite(d, arg, false)
			}
		default:
			if !remote {
				g.bashGit(kind, d, arg)
			}
		}
	}
}

func (g *cg) checkBash() {
	hit := false
	for _, w := range []string{"git", "sed", "cp", "mv", "rm", "tee", "unlink", ">"} {
		if strings.Contains(g.cmd, w) {
			hit = true
			break
		}
	}
	if !hit {
		return
	}
	g.pend = ""
	g.analyze(g.cmd, g.hcwd, 0, false)
	for _, l := range heredocLines(g.pend) { // 막힌 데가 없으면 파일 대상들에 임대를 남긴다
		f := bashRead(l, 0x1f, 2)
		if f[1] != "" && g.tree(f[0]) {
			g.lease(f[1], "0")
		}
	}
}
