package main

// 레인 띄우기(셸 판 bin/hc-lane 본문·lib/hc-lane/start.sh): 작업 트리 → 준비 훅 → 임대 → hc run --bg → 레인 기록·감시·시작 확인.
// 같은 인자·출력·종료 코드(0 띄움, 1 실패, 2 인자·대기열만, 5 시작 안 됨, 6 로그인, 7 키체인, 10 이미 돎).
// 단계 함수는 launchsteps.go — 차례는 launchMain 한 곳.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"herdr-tools/internal/atomic"
)

func init() { register("launch", launchMain, nil) }

var (
	swait, strust, poll, runBrief, cwd, hsp, addr, start, logPath, loOwner string
	fwait                                                                  int64
	tr, im                                                                 string
	ik                                                                     int
	itopSet                                                                bool
	itop, ihome                                                            string
)

func rmRule(w io.Writer) {
	fmt.Fprint(w, "- 임시 파일은 실행마다 `mktemp -d`로 새 폴더를 만들어 쓰고 지우지 않는다. `rm -rf`·`rm -f`에 변수(`$X/…`)나 와일드카드(`*`) 경로를 쓰지 않는다 — Claude Code가 권한 창을 띄워 무인 레인이 막힌다. 꼭 지워야 하면 글자 그대로의 경로나 `\"${X:?}\"` 꼴로.\n")
}

func printSteps(w io.Writer) {
	n := 0
	s := func(t string) { n++; fmt.Fprintf(w, "%d. %s\n", n, t) }
	fmt.Fprint(w, "### 착지·정리 (hc-lane이 채움)\n")
	fmt.Fprintf(w, "- 작업 폴더 %s (브랜치 %s, 기준 %s). 편집·명령은 이 폴더 안에서만 한다.\n", wt, branch, blab)
	fmt.Fprint(w, "- 커밋은 경로를 지정한다: git add -- <경로…> && git commit -m \"<메시지>\" -- <경로…> (add -A·commit -a·stash·reset --hard·강제 push 금지)\n")
	rmRule(w)
	fmt.Fprint(w, "- 알림: 중간·완료·착지·막힘 때 hc-lane report <상태> [<sha>] \"<한 줄>\"을 부른다(화면 줄과 보고 파일은 그대로 써도 됨). 지휘는 hc-lane wait로 이 사건만 기다린다\n")
	fmt.Fprint(w, "  예: hc-lane report 완료 HEAD \"<요약>\"(HEAD는 전체 해시로 풀림) ·아래 1이 착지하면 hc-lane report 착지 <sha8> · 막히면 hc-lane report 막힘 - \"<까닭>\"\n")
	s(fmt.Sprintf("cd %s && hc-lane land %s", q(wt), base))
	fmt.Fprint(w, "   # 이 한 줄을 그대로 돌린다. 나누거나 세미콜론으로 이어 쓰지 말 것. 앞 단계가 실패하면 push는 나가지 않는다\n")
	fmt.Fprint(w, "   # 저장소에 .claude/herdr-land.sh가 있으면 그 스크립트가 fetch·rebase·검증·push를 하고, 없으면 hc-lane이 같은 순서로 하며 .claude/herdr-verify.sh가 없으면 push하지 않고 실패한다\n")
	fmt.Fprintf(w, "   # rebase가 충돌로 멈추면(종료 3) 양쪽 내용을 살려 풀고 고친 파일을 git add 한 뒤 git rebase --continue, 그다음 이 줄을 처음부터 다시. 병합기 제출이 기본 브랜치와 충돌해 거절돼도 종료 3이다(rebase는 시작 전 — git rebase origin/%s 부터)\n", base)
	fmt.Fprint(w, "   # 검증이 실패하면 고쳐 커밋하고 이 줄을 다시. push 거절 횟수(종료 4)나 잠금 기다림 상한(종료 75)으로 끝나면 이 줄을 다시, 최대 20번\n")
	fmt.Fprint(w, "   # 병합기가 켜진 저장소면 이 줄은 큐에 제출만 하고 끝난다(「제출 … 이 레인은 여기서 끝」) — 착지는 병합기가 알리고, report 착지는 적히지 않는다. 아래 clean은 큐 표식(refs/merge-queue/*-레인이름)에 그대로 있는 가지를 「제출됨」으로 지워 자리를 바로 비운다\n")
	s(fmt.Sprintf("cd %s && hc-lane clean --repo %s --branch %s", q(repo), q(repo), branch))
	fmt.Fprint(w, "   # teardown·검사·워크트리·브랜치 정리를 한 번에. clean이 종료 1로 멈추면 출력된 이유를 고치고 같은 줄을 다시 돈다\n")
	fmt.Fprint(w, "   # 폴더를 rm -rf로 직접 지우지 않는다. 보고는 출력 그대로, 건너뜀은 통과가 아니다\n")
	fmt.Fprintf(w, "   # 끝내기 전 이 작업 트리 아래에서 도는 배경 프로세스가 없는지 확인한다(pgrep -fl %s). 자기 셸·에이전트는 빼고, 남은 것은 그 정확한 pid로 kill 한다. cwd로만 도는 것(명령 줄에 경로 없음)은 pgrep에 안 보인다\n", q(wt+"/"))
}

func readSteps(w io.Writer) {
	fmt.Fprint(w, "### 판정 알림 (hc-lane이 채움)\n")
	rmRule(w)
	fmt.Fprint(w, "- 판정이 나오면 보고 파일과 함께 이 한 줄을 부른다: hc-lane report 판정 <대상 커밋 40자 sha, 커밋이 아니면 -> \"<판정 한 줄(예: 감사 완료 높음 0·중간 0·낮음 n)>\". 위 지시문이 판정 줄 꼴을 정했으면 그 꼴이 앞선다\n")
	fmt.Fprint(w, "- 막히면 hc-lane report 막힘 - \"<까닭>\". 지휘는 hc-lane wait로 이 사건만 기다린다\n")
}

func anySteps(w io.Writer) {
	if read {
		readSteps(w)
	} else {
		printSteps(w)
	}
}

func isDigits(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

func launchMain(args []string) int {
	startSetup() // 배경 첫 턴 감시(__turn-check)도 사건을 셸 판 경로로 부른다
	if len(args) > 0 && args[0] == "__turn-check" {
		return turnCheckChild()
	}
	rest := parseStart("launch", args)
	checkArgs("launch", rest)
	rdir := resolveRepo("launch")
	selfTerm = os.Getenv("HC_TERMINAL_ID")
	if steps {
		anySteps(os.Stdout)
		return 0
	}
	fill := os.Getenv("HC_LANE_FILL") == "1"
	tid := os.Getenv("HC_TERMINAL_ID")
	if queueOnly(fill, tid) {
		return 2
	}
	liveNameCheck()
	nn := launchSettings(fill)
	rest = briefCommand(rest)
	prepRepo(rdir)
	wfrom := bref
	if resume {
		wfrom = resumeFrom() // --resume: 주차한 가지를 받아 그 참조에서 시작(launchresume.go)
	}
	rest = placeBrief(rest)
	envs := laneEnvs(tid)
	// 다른 기계 레인은 맥 Herdr 창의 그 기계 공간에 탭으로(hc run --label herdr.space=<id>)
	if on != "" {
		hsp = herdrSpace()
	}
	code := 0
	if dry {
		launchDry(nn, envs, rest, wfrom)
	} else {
		var ok bool
		if code, ok = launchRun(nn, envs, rest, wfrom, tid, fill); !ok {
			return code // 임대를 못 잡아 띄우지 않았다(1·10)
		}
	}
	launchSummary()
	return code
}

func atRepo() string {
	if on != "" {
		return on + ":" + repo
	}
	return repo
}

func onc() string {
	if on != "" {
		return on + ":"
	}
	return ""
}

func onOf() string {
	if on != "" {
		return on + " 의 "
	}
	return ""
}

func appendLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
	return atomic.Append(path, 0o666, line)
}
