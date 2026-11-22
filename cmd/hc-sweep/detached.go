package main

// 분리·깨끗한 읽기 전용 작업 트리: 판정(detachedClean)과 닫은 뒤 지우기(worktreeRemove) — docs/hc-sweep/orphans.md

import (
	"path/filepath"
	"strings"
)

// detachedClean: cwd가 분리 HEAD이고 변경 0(추적 안 된 파일 포함)이고 HEAD가 어느 가지(로컬·원격)에든 든 작업 트리면 메인 저장소,
// 아니면(가지 위·변경·가지에 없는 커밋·git 실패) "". 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리의
// 끝난 터미널(2026-11-23 fable-audit 여섯)이 「작업 트리 남음」으로 남지 않게
func detachedClean(cwd string, r runner) string {
	git := func(args ...string) (string, int) {
		res := hcGit(r, cwd, 20, args...)
		if res.err != nil {
			return "", -1
		}
		return strings.TrimSpace(res.out), res.rc
	}
	if _, rc := git("symbolic-ref", "-q", "HEAD"); rc != 1 { // 0 가지 위, 그 밖은 모름
		return ""
	}
	if s, rc := git("status", "--porcelain", "--untracked-files=all"); rc != 0 || s != "" {
		return ""
	}
	if u, rc := git("rev-list", "-n", "1", "HEAD", "--not", "--branches", "--remotes"); rc != 0 || u != "" {
		return ""
	}
	common, rc := git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if rc != 0 || !strings.HasPrefix(common, "/") {
		return ""
	}
	return filepath.Dir(common)
}

// worktreeRemove: 닫은 터미널의 분리·깨끗한 작업 트리를 다시 확인하고 `git worktree remove`(--force 없이 — 그사이 변경이 생겼으면
// git이 거절한다). (성공, 한 줄)
func worktreeRemove(mainRepo, cwd string, r runner) (bool, string) {
	if detachedClean(cwd, r) != mainRepo {
		return false, "다시 보니 분리·깨끗이 아님 — 손대지 않음"
	}
	res := hcGit(r, mainRepo, 60, "worktree", "remove", cwd)
	if res.err != nil {
		return false, cutRunes(res.err.Error(), 200)
	}
	if res.rc != 0 {
		return false, cutRunes(strings.TrimSpace(res.errs+res.out), 200)
	}
	return true, "git worktree remove " + cwd
}
