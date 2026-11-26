package main

// 워커 쪽 착지 실행(hc-lane merger --land-run) — 병합기 쪽 고르기·맡기기·기다리기는 landon.go

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

// landRunMain: hc-lane merger --land-run <기본 브랜치> <차례 커밋> — 워커 쪽(병합기가 hc run --on으로, 그 기계 클론에서 landWrap 안에).
// 차례 커밋의 부모(후보)를 <폴더>/<차례 12>(폴더 = HC_MERGER_LAND_DIR, 없으면 <클론>.land-wt)에 hc-merger-land/<레인>-<차례 12> 가지로 꺼내
// 준비 훅 → hc-lane land(병합기와 같은 환경 — 부른 쪽이 env로 — 를 그 기계의 셸 환경에서: 레인 준비 단계·추측 일꾼과 같은 shellCmd).
// 로그는 <폴더>/<차례 12>.log. 착지 단계에 들어섰으면 결과(종료 코드, 준비 훅이 명령 없음 127이면 「nocmd <명령>」)를
// HC_MERGER_LAND_RESULT 파일에 적는다(못 적은 끝은 감싸개가 「nostart」). 종료: 착지 종료 코드 · 75 준비 훅 실패 · 70 시작 못 함 · 2 인자
func landRunMain(a []string) int {
	rc, res := landRun(a)
	if r := os.Getenv("HC_MERGER_LAND_RESULT"); res != "" && r != "" {
		if err := os.WriteFile(r, []byte(res+"\n"), 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: --land-run — 결과 파일을 못 썼다(병합기는 시작 못 함으로 본다): %v\n", err)
		}
	}
	return rc
}

// landRun: 종료 코드와 결과 파일 줄(빈 값 = 착지를 시작 못 함)
func landRun(a []string) (int, string) {
	if len(a) != 2 || !plain(a[0]) || len(a[1]) != 40 || strings.Trim(a[1], "0123456789abcdef") != "" {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run <기본 브랜치> <차례 커밋 40자>")
		return 2, ""
	}
	base, cm := a[0], a[1]
	ln := os.Getenv("HC_LANE")
	top, rc := outQ("rev-parse", "--show-toplevel")
	if !laneOK(ln) || rc != 0 || os.Chdir(top) != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — git 클론 안에서 HC_LANE과 함께 부른다")
		return rcLandRunNot, ""
	}
	if orphansDefer("착지 작업 트리") {
		return rcLandRunNot, ""
	}
	dir := os.Getenv("HC_MERGER_LAND_DIR")
	if dir == "" {
		dir = top + ".land-wt"
	}
	landSweep(dir)
	ref := landClaims + base
	if fetchr(false, "-q", "origin", "+"+ref+":"+ref, "+refs/heads/"+base+":refs/remotes/origin/"+base) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 착지 차례를 받지 못했다")
		return rcLandRunNot, ""
	}
	// 착지 push가 함께 싣는 제출 기록 커밋(<기록>:refs/merge-done/<레인> — exbuild)을 이 클론에도 — 그 push가 지우는 기록 참조를 받기만
	// (FETCH_HEAD, 참조는 남기지 않음)
	metas := []string{"-q", "origin"}
	for _, f := range strings.Fields(os.Getenv("HERDR_LAND_PUSH_EXTRA")) {
		if r, ok := strings.CutPrefix(f, ":refs/merge-queue-meta/"); ok {
			metas = append(metas, "refs/merge-queue-meta/"+r)
		}
	}
	if len(metas) > 2 && fetchr(true, metas...) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 제출 기록을 받지 못했다")
		return rcLandRunNot, ""
	}
	defer runQ("update-ref", "-d", ref) // 이 클론에 차례 참조를 남기지 않는다
	if got, _ := outQ("rev-parse", "-q", "--verify", ref); got != cm {
		fmt.Fprintf(os.Stderr, "hc-lane merger: --land-run — 착지 차례가 바뀌었다(%s ≠ %s), 착지하지 않는다\n", first8(got), first8(cm))
		return rcLandRunNot, ""
	}
	wt, br := dir+"/"+cm[:12], landBranch(ln, cm)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 작업 폴더를 만들지 못했다")
		return rcLandRunNot, ""
	}
	lg := wt + ".log"
	defer runQ("branch", "-q", "-D", br)
	landRmWT(wt)
	if inLog("", lg, "worktree", "add", "-q", "-f", "-B", br, wt, cm+"^") != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --land-run — 작업 트리를 만들지 못했다")
		return rcLandRunNot, ""
	}
	defer landRmWT(wt)
	lf, err := os.OpenFile(lg, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return rcLandRunNot, ""
	}
	defer lf.Close()
	env := append(os.Environ(), "HERDR_MERGER=1")
	if id := os.Getenv("HC_MERGER_LAND_SPEC"); id != "" {
		if line, _, ok := specPartsGet(id, ""); ok && len(strings.Fields(line)) >= 2 && os.WriteFile(wt+".parts", []byte(line+"\n"), 0o666) == nil {
			env = append(env, "HERDR_VERIFY_PARTS_FROM="+wt+".parts")
			defer os.Remove(wt + ".parts")
		}
	}
	// 셸 환경의 PATH가 다른 hc-lane을 먼저 찾지 않게 지금 것의 절대 경로로
	self, err := exec.LookPath(gSelf)
	if err != nil {
		fmt.Fprintf(lf, "hc-lane merger: --land-run — %s 를 못 찾았다: %v\n", gSelf, err)
		return rcLandRunNot, ""
	}
	switch rc, hrc := setupHook(wt, top, base, lf, "", true); {
	case rc == rcNotRun:
		return rcLandRunNot, ""
	case hrc == 127: // 그 기계에 준비 훅의 도구가 없다 — 되풀이해도 같다. 병합기가 이 기계를 빼고 사건 한 번(landExec)
		lf.Sync()
		b, _ := os.ReadFile(lg)
		return rc, "nocmd " + missingCmd(string(b))
	case rc != 0:
		return rc, strconv.Itoa(rc)
	}
	c := shellCmd(wt, self, "land", base)
	c.Stdout, c.Stderr, c.Env = lf, lf, env
	rc, ran := proc.WaitFor(c, "", landMax(), landMax() <= 0)
	if !ran {
		return rcLandRunNot, ""
	}
	return rc, strconv.Itoa(rc)
}

// missingCmd: 준비 훅 로그에서 마지막으로 없던 명령(bash 「…: <명령>: command not found」 · zsh 「command not found: <명령>」), 모르면 「?」
func missingCmd(log string) string {
	m := missingRe.FindAllStringSubmatch(log, -1)
	if len(m) == 0 {
		return "?"
	}
	l := m[len(m)-1]
	if l[2] != "" {
		return l[2] // zsh 꼴 — 앞의 「zsh:」는 셸 이름
	}
	return l[1]
}

var missingRe = regexp.MustCompile(`([^\s:]+): command not found(?:: (\S+))?`)

// landBranch: 착지 작업 트리의 가지 — 실행(차례)마다 다른 이름. 레인 이름만이면 신호로 끊겨 defer가 못 돈 앞 실행의 작업 트리가 그
// 가지를 쥐어 같은 레인의 다음 실행이 시작도 못 했다(2026-11-26 ht-land-run-killed — 「used by worktree」 → nostart 70). 남은 것은
// landSweep이 하루 뒤 치운다. 「/」가 아니라 「-」로 잇는다 — 옛 이름 hc-merger-land/<레인> 가지와 참조 폴더·파일로 부딪히지 않게
func landBranch(ln, cm string) string { return "hc-merger-land/" + ln + "-" + cm[:12] }

// landRmWT: 착지 작업 트리를 지운다(git이 못 지우면 폴더째 — <폴더> 아래만), 등록은 prune
func landRmWT(wt string) {
	if _, err := os.Lstat(wt); err == nil && runQ("worktree", "remove", "-f", "-f", wt) != 0 {
		os.RemoveAll(wt)
	}
	runQ("worktree", "prune")
}

// landSweep: 시간 넘김(무리째 KILL)으로 defer가 못 돈 앞 실행의 잔재 — <폴더> 아래 하루 넘은 항목과, 어느 작업 트리도 쓰지 않는
// hc-merger-land/* 가지를 치운다(감사 atp2f36 낮음 1)
func landSweep(dir string) {
	es, _ := os.ReadDir(dir) // 없으면 치울 것 없음
	for _, e := range es {
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			if e.IsDir() {
				landRmWT(dir + "/" + e.Name())
			} else {
				os.Remove(dir + "/" + e.Name())
			}
		}
	}
	runQ("worktree", "prune")
	bs, _ := outQ("for-each-ref", "--format=%(refname:short)", "refs/heads/hc-merger-land/")
	for _, b := range strings.Fields(bs) {
		runQ("branch", "-q", "-D", b) // 작업 트리가 쓰는 가지는 git이 거절한다(도는 실행)
	}
}
