package main

// 분리·깨끗한 읽기 전용 작업 트리: 판정(detachedClean)과 닫은 뒤 지우기(worktreeRemove) — docs/hc-sweep/orphans.md

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
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

// dirInUse: 지금 hc ls에서 이 기계의 live 터미널(self 말고)이 dir 또는 그 아래를 cwd로 쓰면 그 까닭 한 줄, 아무도 안 쓰면 "".
// hc ls 실패·판독 실패·이 기계 모름 항목(hcjson Unknown)도 쓰는 중으로 본다(R-3M01 — 모르면 남김). 심볼릭 링크는 양쪽 다 푼다
// (11-23 끝난 감사 터미널을 닫으며 같은 폴더를 쓰던 확인 세션들의 작업 트리를 지운 일 뒤)
func dirInUse(dir, self string, r runner) string {
	res := r(2*time.Minute, nil, "hc", "ls", "--json")
	if res.err != nil || res.rc != 0 {
		return "hc ls 실패 — 그 폴더를 누가 쓰는지 모름"
	}
	l, err := hcjson.ParseList([]byte(res.out))
	if err != nil {
		return "hc ls 판독 실패 — 그 폴더를 누가 쓰는지 모름"
	}
	real := func(p string) string {
		if q, err := filepath.EvalSymlinks(p); err == nil {
			return q
		}
		return p
	}
	d := real(dir)
	for _, t := range l.Terminals {
		if t.Address == self || strings.Contains(t.Address, "/") { // 자신·다른 기계(그 cwd는 이 기계 폴더가 아니다)
			continue
		}
		if t.Unknown != "" {
			return "hc ls에 읽지 못한 터미널 " + t.Address + " — 그 폴더를 쓰는지 모름"
		}
		if t.State == "live" && (hcjson.Under(t.Cwd, dir) || hcjson.Under(real(t.Cwd), d)) {
			return "다른 터미널 " + t.Address + " 이 쓰는 중(" + t.Cwd + ")"
		}
	}
	return ""
}

// worktreeRemove: 닫은 터미널(self)의 분리·깨끗한 작업 트리를 다시 확인하고 `git worktree remove`(--force 없이 — 그사이 변경이 생겼으면
// git이 거절한다). 다른 터미널이 그 폴더를 쓰거나 모르면(dirInUse) 손대지 않는다. (성공, 한 줄)
func worktreeRemove(mainRepo, cwd, self string, r runner) (bool, string) {
	if why := dirInUse(cwd, self, r); why != "" {
		return false, why + " — 손대지 않음"
	}
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

// detachedRemoveOn: 분리 작업 트리 지우기는 HC_SWEEP_DETACHED_REMOVE=1일 때만(2026-11-23 sweep-detached-off — 4바퀴 감사
// R-4A02·03·04·08이 무시된 보관물·--only-under 밖·멈춘 rebase·정규화 다른 cwd를 지울 수 있다고 짚어 고칠 때까지 기본 끔). 닫기는 그대로
func detachedRemoveOn() bool { return os.Getenv("HC_SWEEP_DETACHED_REMOVE") == "1" }

// detachedAfterClose: 닫은 터미널의 분리·깨끗 작업 트리 처리. (화면 한 줄, closed.log 까닭 — 빈 글이면 적지 않음, closed.log 끝 칸)
func detachedAfterClose(mainRepo, cwd, self string, r runner) (line, why, msg string) {
	if !detachedRemoveOn() {
		return "작업 트리 남김(분리 지우기 꺼짐): " + cwd, "", ""
	}
	ok, msg := worktreeRemove(mainRepo, cwd, self, r)
	fail := ""
	if !ok {
		fail = " 실패"
	}
	return "작업 트리 정리" + fail + ": " + mainRepo + " — " + msg, "작업 트리 정리(분리·깨끗, git worktree remove)" + fail, msg
}
