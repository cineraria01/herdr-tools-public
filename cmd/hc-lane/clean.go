package main

// clean(셸 판 lib/hc-lane/clean.sh)과 park: 지우기 전에 다 확인하고, 하나라도 걸리면 아무것도 지우지 않는다.
// 「제출됨」은 큐 표식 refs/merge-queue/<UTC초>-<레인>이 그 커밋일 때만, 「주차됨」은 원격 refs/park/<레인>이 그 커밋일 때만
// (2026-11-21 hc-lane-go-start — 예전엔 원격 lane/<레인>을 봐서, 반송돼 큐에서 빠진 제출 가지·주차 가지가 같은 이름으로 섞였다).
// 정리가 중간에 끊긴 작업 폴더(추적 파일이 지워지기만 함)는 다시 부르면 이어서 지운다.
// 단계 함수는 cleansteps.go — 차례는 cleanMain 한 곳.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func init() {
	register("clean", cleanMain, nil)
	register("park", parkMain, noHelp) // --help·-h는 사용법(2026-11-23 park-help-guard — 예전엔 주차 글로 읽어 실제로 주차했다)
}

var (
	td, did         string
	owner, saved    string
	cleanSubmitTell = regexp.MustCompile(`^refs/merge-queue/[0-9]+-(.+)$`)
)

// cj: clean 결과 JSON 한 줄
func cj(state, reason string) {
	t := td
	if t == "" {
		t = "none"
	}
	fmt.Printf(`{"clean":"%s","reason":"%s","dry_run":%s,"on":"%s","repo":"%s","worktree":"%s","branch":"%s","base":"%s","teardown":"%s"}`+"\n",
		state, j(reason), tf(dry), j(on), j(repo), j(wt), j(branch), j(base), t)
}

// stop: 오류 출력에 알리고(자세한 목록은 30줄까지) JSON, 종료 1
func stop(msg string, detail ...string) {
	errf("hc-lane clean: %s\n", msg)
	if len(detail) > 0 && detail[0] != "" {
		ls := strings.Split(detail[0], "\n")
		for i, l := range ls {
			if i == 30 {
				break
			}
			errf("    %s\n", l)
		}
		if len(ls) > 30 {
			errf("    … (모두 %d줄)\n", len(ls))
		}
	}
	if did != "" {
		errf("hc-lane clean: 여기까지 했다: %s. 원인을 고친 뒤 같은 명령을 다시 돌린다\n", did)
	} else {
		errf("hc-lane clean: 아무것도 지우지 않았다. 원인을 고친 뒤 같은 명령을 다시 돌린다(폴더를 rm -rf로 직접 지우지 않는다)\n")
	}
	cj("stopped", msg)
	os.Exit(1)
}

// termGate: 작업 폴더에서 도는 살아 있는 다른 터미널이 없음을 확정한다 — 모르거나(hc ls 실패·판독 오류·기계 연결 모름) 있으면 멈춘다.
// 처음·보관 뒤·teardown 뒤(제거 직전)에 부른다 — 확인과 지우기 사이에 다시 붙은 터미널을 놓치지 않게(재감사2 lane/R-207)
func termGate(when string) {
	ps, r := termPairs()
	if r != 0 {
		stop(termWhy(r) + ": " + when + "이 폴더에서 도는 터미널을 확인하지 못했다")
	}
	busy := busyTerms(ps, true)
	// 이 레인 실행의 판정을 수거한 끝난 감사 터미널은 닫고(cleanaudit.go) 목록을 다시 본다 — 닫은 사이 붙은 터미널도 놓치지 않게
	if rest := auditCloseBusy(busy); len(rest) < len(busy) && !dry {
		if ps, r = termPairs(); r != 0 {
			stop(termWhy(r) + ": " + when + "감사 터미널을 닫은 뒤 이 폴더에서 도는 터미널을 확인하지 못했다")
		}
		busy = busyTerms(ps, true)
	} else {
		busy = rest
	}
	if len(busy) > 0 {
		stop(when+"살아 있는 터미널이 이 폴더에서 돈다(끝났으면 hc kill <주소> 뒤 다시): "+wt, strings.Join(busy, "\n"))
	}
}

func addDid(s string) {
	if did != "" {
		did += ", "
	}
	did += s
}

// exists: 있음·없음만 믿고, 그 밖(hc 연결 끊김 등)은 멈춘다
func exists() bool {
	switch xrun(nil, os.Stderr, "sh", "-c", `[ -e "$1" ] || [ -L "$1" ]`, "hc-lane", wt) {
	case 0:
		return true
	case 1:
		return false
	}
	stop("폴더를 확인하지 못했다(hc·연결?): " + wt)
	return false
}

func hasBranch() bool {
	switch xrun(nil, os.Stderr, "git", "-C", repo, "show-ref", "--verify", "-q", "refs/heads/"+branch) {
	case 0:
		return true
	case 1:
		return false
	}
	stop("브랜치를 확인하지 못했다: " + branch)
	return false
}

func leftCheck(what, common string) {
	g, r := xout("git", "-C", repo, "status", "--porcelain", "--untracked-files=all", "--", ":(glob)**/.gitignore")
	if r != 0 {
		stop("메인 폴더 git status 실패: " + repo)
	}
	if g != "" {
		stop("메인 폴더의 .gitignore에 커밋 안 된 변경이 있어 "+what+" 폴더의 무시 판정을 믿을 수 없다: "+repo, g)
	}
	left, ok := leftover(repo, wt, common)
	if !ok {
		stop(what + " 폴더를 검사하지 못했다: " + wt)
	}
	if left != "" {
		stop(what+" 폴더에 .gitignore로 무시되지 않는 것(추적 파일 포함)이 있다: "+wt, left)
	}
}

// qaScript: 무시된 보관물 .qa를 메인 폴더의 같은 자리로 복사(덮지 않음) — 「c|s|d|f|n <경로>」 줄. 3 메인 폴더 없음.
// 옆 임시 파일에 다 복사한 뒤 ln -P(이미 있으면 실패 — 덮지 않는 원자 넣기)로 넣는다: 두 작업 트리가 같은 경로를 동시에 보관해도
// 한쪽이 다른 쪽을 덮지 않고, 진 쪽은 다시 견줘 다르면 d(그 작업 트리 보존, gofix E04)
const qaScript = `cd "$1" || exit 2
      r=$(find . -path ./.git -prune -o -type d -name .qa -print -prune) || exit 2
      [ -n "$r" ] || exit 0
      f=$(printf "%s\n" "$r" | while IFS= read -r d; do find "$d" ! -type d || exit 2; done) || exit 2
      f=$(printf "%s\n" "$f" | sed "/^$/d; s|^\./||") || exit 2
      [ -n "$f" ] || exit 0
      i=$(printf "%s\n" "$f" | git -c core.quotePath=false -C "$3" check-ignore --stdin); [ $? -le 1 ] || exit 2
      [ -n "$i" ] || exit 0
      [ -n "$2" ] && [ -d "$2" ] || exit 3
      [ "$4" = 0 ] || { printf "%s\n" "$i" | sed "s/^/n /"; exit 0; }
      printf "%s\n" "$i" | while IFS= read -r p; do
        if [ -e "$2/$p" ] || [ -L "$2/$p" ]; then k=s; cmp -s "$p" "$2/$p" || k=d
        else
          t= k=f
          if mkdir -p "$2/$(dirname "$p")" && t=$(mktemp "$2/$(dirname "$p")/.hc-lane-qa.XXXXXX") && cp -pP "$p" "$t" && cmp -s "$p" "$t"; then
            if ln -P "$t" "$2/$p" 2>/dev/null; then
              # 그사이 목적지가 폴더가 됐으면 ln은 그 안에 넣고 0 — 목적지 자체가 이 파일일 때만 보관됨(R-12)
              if [ ! -d "$2/$p" ] && [ "$t" -ef "$2/$p" ]; then k=c
              else rm -f "$2/$p/${t##*/}"; k=f; fi
            elif [ -d "$2/$p" ]; then k=f
            elif [ -e "$2/$p" ] || [ -L "$2/$p" ]; then k=s; cmp -s "$p" "$2/$p" || k=d
            fi
          fi
          [ -z "$t" ] || rm -f "$t"
        fi
        echo "$k $p"
      done`

var qaDirRe = regexp.MustCompile(`^(.*\.qa)/.*`)

func cleanMain(args []string) int {
	startSetup()
	dieHook = func(msg string) { cj("stopped", msg) } // 셸 판 die: clean이면 결과 JSON도
	rest := parseStart("clean", args)
	checkArgs("clean", rest)
	resolveRepo("clean")
	selfTerm = os.Getenv("HC_TERMINAL_ID")
	P := dryP()
	td, did = "none", ""
	if branch == base {
		die("기본 브랜치(%s)는 정리하지 않는다", base)
	}
	here := cleanHere()
	common, r := xout("git", "-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if r != 0 {
		stop("git 관리 폴더를 찾지 못했다: " + repo)
	}
	wl, r := xout("git", "-C", repo, "worktree", "list", "--porcelain")
	if r != 0 {
		stop("git worktree list 실패: " + repo)
	}
	blk, reg := wtBlock(wl, wt)
	dir := exists()
	br := hasBranch()
	if !reg && !dir && !br {
		fmt.Printf("정리할 것 없음: 워크트리 %s 와 브랜치 %s 가 이미 없다\n", wt, branch)
		cleanLease()
		keepOffSelf()
		unblockNote()
		cj("nothing", "")
		return 0
	}
	// 0) 이 브랜치의 작업 폴더인지, 잠기지 않았는지, 살아 있는 다른 터미널이 그 폴더에서 돌지 않는지
	if reg {
		if msg := wtBlockProblem(blk, br, branch, wt); msg != "" {
			stop(msg)
		}
	}
	if dir {
		termGate("")
	}
	arch, btip, subm := mergedCheck(blk, reg, br, P) // 1) 기준에 들어갔나·표식이 쥐었나·보관할 것
	delOnly := wtLeftovers(reg, dir, common, P)      // 2) 지울 폴더에 남길 변경이 없나
	archivePush(arch)                                // 2.5) --archive
	if dir {
		qaMove(wl, reg) // 2.7) 무시된 보관물 .qa
	}
	// 2.9) 보관(.qa 복사·archive push)하는 사이 그 폴더에 터미널이 다시 붙었을 수 있다 — 지우기 전에 다시 확인(R-207)
	if dir && !dry {
		termGate("보관 뒤 ")
	}
	// 3) teardown(있으면, 실패해도 계속) → worktree remove → 남은 무시 파일 폴더 → branch -d
	teardownStep(reg, dir)
	if reg {
		removeWorktree(delOnly)
	}
	removeLeftDir(reg, dir, common)
	if br {
		removeBranch(len(arch) > 0 || subm, btip)
	}
	cleanSummary(here)
	cleanLease()
	keepOffSelf()
	unblockNote()
	if dry {
		cj("planned", "")
	} else {
		cj("done", "")
	}
	return 0
}
