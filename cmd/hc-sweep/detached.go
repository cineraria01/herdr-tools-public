package main

// 분리·깨끗한 읽기 전용 작업 트리: 판정(detachedClean)과 닫은 뒤 지우기(worktreeRemove, 기본 켬 — HC_SWEEP_DETACHED_REMOVE=0이면
// 닫기만) — docs/hc-sweep/orphans.md

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
)

// keepScript: 작업 트리 맨 위($PWD)에서 지우면 안 되는 것 — 멈춘 git 작업(rebase·merge 등 — autostash 기록이 git 폴더에 있다,
// R-4A03)과 어느 깊이든 .qa 폴더(무시된 보관물은 status에도 worktree remove에도 안 걸린다 — R-4M01). 없으면 「없음」 한 줄, 있으면
// 까닭 한 줄, 확인 실패는 종료 2(모름)
const keepScript = `g=$(git rev-parse --absolute-git-dir) || exit 2
for f in rebase-merge rebase-apply MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD BISECT_LOG sequencer; do
  if [ -e "$g/$f" ] || [ -L "$g/$f" ]; then echo "멈춘 git 작업($f)"; exit 0; fi
done
q=$(find . -path ./.git -prune -o -type d -name .qa -print -prune) || exit 2
if [ -n "$q" ]; then echo "보관물 .qa 있음"; exit 0; fi
echo 없음`

// detachedClean: cwd가 분리 HEAD이고 변경 0(추적 안 된 파일 포함)이고 HEAD가 어느 가지(로컬·원격)에든 들었고 멈춘 git 작업·.qa
// 보관물이 없는 작업 트리면 (메인 저장소, 작업 트리 맨 위, ""). 아니면 ("", "", 까닭 — 분리임을 확인한 뒤에만, 가지 위·모름은 "").
// 분리는 종료 0의 출력으로만 본다(R-4M02 — hc exec가 스스로 실패해도 종료 1이다). 감사 스크립트가 `git worktree add --detach`로
// 직접 만든 읽기 전용 작업 트리의 끝난 터미널(2026-11-23 fable-audit 여섯)이 「작업 트리 남음」으로 남지 않게
func detachedClean(cwd string, r runner) (mainRepo, top, why string) {
	git := func(args ...string) (string, bool) {
		res := hcGit(r, cwd, 20, args...)
		return strings.TrimSpace(res.out), res.err == nil && res.rc == 0
	}
	if h, ok := git("rev-parse", "--symbolic-full-name", "HEAD"); !ok || h != "HEAD" { // refs/heads/… 가지 위, 그 밖·실패는 모름
		return "", "", ""
	}
	if s, ok := git("status", "--porcelain", "--untracked-files=all"); !ok || s != "" {
		return "", "", "분리지만 변경 있음 또는 모름"
	}
	if u, ok := git("rev-list", "-n", "1", "HEAD", "--not", "--branches", "--remotes"); !ok || u != "" {
		return "", "", "분리지만 가지에 없는 커밋 또는 모름"
	}
	p, ok := git("rev-parse", "--path-format=absolute", "--git-common-dir", "--show-toplevel") // 루트는 cwd가 아니라 맨 위(R-4A10)
	l := strings.Split(p, "\n")
	if !ok || len(l) != 2 || !filepath.IsAbs(l[0]) || !filepath.IsAbs(l[1]) {
		return "", "", "분리지만 저장소 위치 모름"
	}
	res := hcExec(r, l[1], 60, "sh", "-c", keepScript)
	if out := strings.TrimSpace(res.out); res.err != nil || res.rc != 0 || out == "" || strings.Contains(out, "\n") {
		return "", "", "분리지만 멈춘 git 작업·보관물 확인 못 함"
	} else if out != "없음" {
		return "", "", "분리지만 " + out
	}
	return filepath.Dir(l[0]), l[1], ""
}

// dirInUse: 지금 hc ls에서 이 기계의 live 터미널(self 말고 — 빈 글이면 아무도 빼지 않는다)이 dir 또는 그 아래를 cwd로 쓰면 그 까닭
// 한 줄, 아무도 안 쓰면 "". hc ls 실패·판독 실패·이 기계 모름 항목(hcjson Unknown)도 쓰는 중으로 본다(R-3M01 — 모르면 남김).
// 같은 폴더는 글자가 아니라 디스크로 본다(hcjson.Uses — 심볼릭 링크·대소문자·한글 정규화가 다른 철자, R-4A03). 띄운 폴더(ls cwd)로
// 아무도 안 쓰면 그 기계의 나머지 live 터미널마다 hc get 한 번(상한 30초)으로 지금 폴더(foreground.cwd)도 본다 — 못 얻으면 모름(남김)
func dirInUse(dir, self string, r runner) string {
	res := r(2*time.Minute, nil, "hc", "ls", "--json")
	if res.err != nil || res.rc != 0 {
		return "hc ls 실패 — 그 폴더를 누가 쓰는지 모름"
	}
	l, err := hcjson.ParseList([]byte(res.out))
	if err != nil {
		return "hc ls 판독 실패 — 그 폴더를 누가 쓰는지 모름"
	}
	var others []string
	for _, t := range l.Terminals {
		if (self != "" && t.Address == self) || strings.Contains(t.Address, "/") { // 자신·다른 기계(그 cwd는 이 기계 폴더가 아니다)
			continue
		}
		if t.Unknown != "" {
			return "hc ls에 읽지 못한 터미널 " + t.Address + " — 그 폴더를 쓰는지 모름"
		}
		if t.State == "live" && hcjson.Uses(t.CwdText, dir) {
			return "다른 터미널 " + t.Address + " 이 쓰는 중(" + t.Cwd + ")"
		}
		if t.State == "live" {
			others = append(others, t.Address)
		}
	}
	// 띄운 폴더로는 아무도 — 띄운 뒤 cd로 들어온 터미널도 본다: 지금 폴더(hc get foreground.cwd). 못 얻으면 모름(2026-11-23 cwd-now-inuse)
	for _, a := range others {
		res := r(30*time.Second, nil, "hc", "get", a, "--json")
		if res.err != nil {
			return "hc get " + a + " 실패 — 그 터미널의 지금 폴더 모름"
		}
		t, found, err := hcjson.ParseGet([]byte(hcjson.GetOut(res.out, res.errs, res.rc)), res.rc, a)
		switch {
		case err != nil:
			return "hc get " + a + " 실패 — 그 터미널의 지금 폴더 모름"
		case !found || t.State != "live": // 그사이 끝남·사라짐 — 그 폴더를 안 쓴다
		case t.NowCwd == "":
			return "터미널 " + a + " 의 지금 폴더 모름(hc get에 foreground.cwd 없음)"
		case hcjson.Uses(t.NowCwd, dir):
			return "다른 터미널 " + a + " 이 쓰는 중(지금 폴더 " + t.NowCwd + ")"
		}
	}
	return ""
}

// worktreeRemove: 닫은 터미널의 분리·깨끗한 작업 트리(맨 위 top)를 다시 확인하고 `git worktree remove`(--force 없이 — 그사이 변경이
// 생겼으면 git이 거절한다). git 검사를 먼저, 쓰는 터미널(dirInUse — 닫은 자신도 빼지 않는다, R-4M06)은 지우기 바로 앞에 본다(R-4A06).
// (지움, 실패 — git worktree remove가 실패함, 한 줄)
func worktreeRemove(mainRepo, top string, r runner) (ok, failed bool, msg string) {
	if m, t, _ := detachedClean(top, r); m != mainRepo || t != top {
		return false, false, "다시 보니 분리·깨끗이 아님 — 손대지 않음"
	}
	if why := dirInUse(top, "", r); why != "" {
		return false, false, why + " — 손대지 않음"
	}
	res := hcGit(r, mainRepo, 60, "worktree", "remove", top)
	if res.err != nil {
		return false, true, cutRunes(res.err.Error(), 200)
	}
	if res.rc != 0 {
		return false, true, cutRunes(strings.TrimSpace(res.errs+res.out), 200)
	}
	return true, false, "git worktree remove " + top
}

// hcGit: cwd에서 git(hc exec — launchd가 띄운 git은 ~/Documents에서 TCC에 멈춘다)
func hcGit(r runner, cwd string, secs int, args ...string) result {
	return hcExec(r, cwd, secs, "git", args...)
}

// hcExec: cwd에서 prog(hc exec, 상한 secs초)
func hcExec(r runner, cwd string, secs int, prog string, args ...string) result {
	a := append([]string{"exec", "--cwd", cwd, "--env", "LC_ALL=C", "--timeout", strconv.Itoa(secs), "--", prog}, args...)
	return r(time.Duration(secs+20)*time.Second, nil, "hc", a...)
}

// detachedRemoveOn: 분리 작업 트리 지우기는 기본 켬(gofix5 — 4바퀴 감사 R-4M01·R-4M02·R-4A03·R-4A04를 고친 뒤 다시 켬).
// HC_SWEEP_DETACHED_REMOVE=0이면 닫기만
func detachedRemoveOn() bool { return os.Getenv("HC_SWEEP_DETACHED_REMOVE") != "0" }

// detachedAfterClose: 닫은 터미널의 분리·깨끗 작업 트리 처리. (화면 한 줄, closed.log 까닭 — 빈 글이면 적지 않음, closed.log 끝 칸,
// 실패 — 종료 코드에 센다 R-4M07)
func detachedAfterClose(mainRepo, top string, r runner) (line, why, msg string, failed bool) {
	if !detachedRemoveOn() {
		return "작업 트리 남김(분리 지우기 꺼짐): " + top, "", "", false
	}
	ok, failed, msg := worktreeRemove(mainRepo, top, r)
	fail := ""
	if !ok {
		fail = " 실패"
	}
	return "작업 트리 정리" + fail + ": " + mainRepo + " — " + msg, "작업 트리 정리(분리·깨끗, git worktree remove)" + fail, msg, failed
}
