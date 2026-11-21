package main

// 띄우기·clean·gc·park의 도우미(hc-lane-go-start). 대상 기계 쪽 판정은 셸 판과 같은 sh -c 조각을 hc exec로 보낸다 — --on이면
// 그 기계에서 돌아야 해서(그 기계에 Go 판이 없을 수 있다). 공통(on·repo·core·die·q·plain·cfg·repoDefaults·ffmain·leaseX·지휘 판정)은
// common.go·hcx.go·ffmain.go·lease.go·coord.go(land·report 레인)의 것을 쓴다.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

var (
	branch, name, brief, swarg, integ, minage         string
	read, direct, resume, archive, dry, steps, rbFlag bool
	cmdName                                           string
	top, wt, rx, undo                                 string
	norepo                                            bool
	selfTerm                                          string // 부른 터미널($HC_TERMINAL_ID — clean이 자기 자신을 뺀다)
	selfPath                                          string // 셸 판 bin/hc-lane 절대 경로(셸 판 self — 감시·사건을 부른다)
)

// startSetup: 셸 판 self = $(cd "$(dirname "$0")" && pwd -P)/${0##*/}
func startSetup() {
	sh := os.Getenv("HC_LANE_SH")
	if sh == "" {
		sh, _ = exec.LookPath("hc-lane")
	}
	if sh != "" {
		if d, err := filepath.EvalSymlinks(filepath.Dir(sh)); err == nil {
			if a, err := filepath.Abs(d); err == nil {
				sh = a + "/" + filepath.Base(sh)
			}
		}
	}
	if sh == "" {
		sh = "hc-lane"
	}
	selfPath = sh
}

func errf(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }

// j: JSON 문자열 속 \ 와 " 만 막는다(셸 판 j와 같다)
func j(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }

func tf(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// xargv: 대상 기계에서 돌 명령 줄(xArgs)
func xargv(args []string) []string { return xArgs(args...) }

func command(argv []string) *exec.Cmd { return exec.Command(argv[0], argv[1:]...) }

// xrun: 대상 기계에서. stdout·stderr가 nil이면 버린다
func xrun(stdout, stderr io.Writer, args ...string) int {
	c := command(xargv(args))
	c.Stdout, c.Stderr = stdout, stderr
	return code(proc.Run(c)) // git·hc는 상한 안에서만(proc.Limit) — 넘기면 124(모름, gofix M3·E02)
}

// xout: $(x …) — 표준 출력(끝 줄바꿈 뺌)·종료 코드, 오류 출력은 화면. xoutQ는 오류 출력을 버림(2>/dev/null)
func xout(args ...string) (string, int)  { return outOf(os.Stderr, xargv(args)) }
func xoutQ(args ...string) (string, int) { return outOf(nil, xargv(args)) }

func outOf(stderr io.Writer, argv []string) (string, int) {
	c := command(argv)
	c.Stderr = stderr
	b, err := proc.Output(c) // 시간 넘김이면 출력 없이 124 — 반쪽 출력을 읽지 않는다
	return trim(string(b)), code(err)
}

// zx: 대상 기계의 그 폴더에서(zxArgs)
func zx(stdout, stderr io.Writer, dir string, args ...string) int {
	c := command(zxArgs(dir, args...))
	c.Stdout, c.Stderr = stdout, stderr
	return code(proc.Run(c))
}

// xi: 표준 입력을 넘긴다(--on이면 hc exec -i)
func xi(stdin io.Reader, args ...string) (string, int) {
	argv := args
	if on != "" {
		argv = append([]string{"hc", "exec", "-i", "--on", on, "--timeout", "600", "--"}, args...)
	}
	c := command(argv)
	c.Stdin, c.Stderr = stdin, os.Stderr
	b, err := proc.Output(c)
	return trim(string(b)), code(err)
}

// captArgv: secs초 안에 안 끝나면 끊는 명령 줄 — 셸 판 capt와 같은 꼴(perl alarm, 없으면 그대로). 계약 시험이 PATH의 가짜 perl로
// 넘긴 초를 본다
func captArgv(secs int64, args ...string) []string {
	if _, err := exec.LookPath("perl"); err == nil {
		return append([]string{"perl", "-e", "alarm shift; exec @ARGV", strconv.FormatInt(secs, 10)}, args...)
	}
	return args
}

// captP: $(capt <초> <명령…>) — 표준 입력 /dev/null, 오류 출력 버림
func captP(secs int64, args ...string) (string, int) { return outOf(nil, captArgv(secs, args...)) }

// now·sleepFor: 시계와 쉼도 셸 판과 같은 명령(date +%s·sleep) — 계약 시험이 PATH의 가짜 date·sleep으로 시계를 흉내 낸다
func now() int64 {
	if out, r := outOf(nil, []string{"date", "+%s"}); r == 0 {
		if n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil {
			return n
		}
	}
	return time.Now().Unix()
}

func sleepFor(s string) {
	if exec.Command("sleep", s).Run() != nil {
		sleepS(s)
	}
}

// xfetch: fetchr x git -C <저장소> fetch <인자…> — 대상 기계에서, 참조 잠금 경합이면 다시(fetchrCmd)
func xfetch(dir string, args ...string) int {
	return fetchrCmd(os.Stdout, os.Stderr, xargv(append([]string{"git", "-C", dir, "fetch"}, args...))...)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// coordIs: 살아 있고 레인·감시 터미널이 아니다(셸 판 coord_is)
func coordIs(addr string) bool {
	_, _, ll, r := tinfo(addr)
	return r == 0 && !ll
}

// localAddr: 대상 기계의 터미널이면 그 기계 쪽 주소(앞의 「<기계>/」를 뗀 것)
func localAddr(a string) (string, bool) {
	if on != "" {
		if !strings.HasPrefix(a, on+"/") {
			return "", false
		}
		a = strings.TrimPrefix(a, on+"/")
	}
	if strings.Contains(a, "/") {
		return "", false // 다른 기계
	}
	return a, true
}

func onp(a string) string {
	if on != "" {
		return on + "/" + a
	}
	return a
}

// termPairs: hc ls --json에서 터미널마다 「<주소> <cwd>」. 1 hc ls 실패, 2 꼴을 못 읽음(뽑은 수가 "cwd" 수와 다름) — 셸 판과 같은 판정
var (
	pairRe = regexp.MustCompile(`.*"address" *: *"([^"]*)".*"cwd" *: *"([^"]*)".*`)
	cwdRe  = regexp.MustCompile(`"cwd" *:`)
)

func termPairs() ([][2]string, int) {
	out, r := outOf(nil, []string{"hc", "ls", "--json"})
	if r != 0 {
		return nil, 1
	}
	if !json.Valid([]byte(out)) { // 잘린 목록(0으로 끝났어도)은 「터미널 없음」이 아니다 — 모르면 지우지 않는다(gofix E01)
		return nil, 2
	}
	var ps [][2]string
	for _, ch := range strings.Split(out, "{") {
		for _, ln := range strings.Split(ch, "\n") {
			if m := pairRe.FindStringSubmatch(ln); m != nil {
				ps = append(ps, [2]string{m[1], m[2]})
			}
		}
	}
	if len(cwdRe.FindAllStringIndex(out, -1)) != len(ps) {
		return nil, 2
	}
	return ps, 0
}

// busyTerms: 대상 기계의 터미널이고 cwd가 작업 폴더(wt) 안인 것 「<주소> <cwd>」. skipSelf면 부른 터미널 자신은 뺀다(clean)
func busyTerms(ps [][2]string, skipSelf bool) []string {
	var b []string
	for _, p := range ps {
		a, ok := localAddr(p[0])
		if !ok || (skipSelf && on == "" && a == selfTerm) {
			continue
		}
		if strings.HasPrefix(p[1]+"/", wt+"/") {
			b = append(b, onp(a)+" "+p[1])
		}
	}
	return b
}

// hook: 작업 폴더 안에서 .claude/herdr-<이름>.sh. 기록은 그 작업 폴더의 git 관리 폴더(worktree remove 때 같이 지워진다).
// 터미널 밖(launchd 등) 로컬이면 코어 문맥으로. 돌려준 값: 종료 코드와 기록 경로
func hook(which string) (int, string) {
	if on == "" && os.Getenv("HC_TERMINAL_ID") == "" {
		core = "1"
	}
	defer func() { core = "" }()
	gd, _ := xout("git", "-C", wt, "rev-parse", "--absolute-git-dir")
	log := gd + "/hc-lane-" + which + ".log"
	r := zx(os.Stdout, os.Stderr, wt, "env", "LANE_REPO="+repo, "LANE_WORKTREE="+wt, "LANE_BRANCH="+branch, "LANE_BASE="+base,
		"sh", "-c", `exec bash "$1" >"$2" 2>&1 </dev/null`, "hc-lane", ".claude/herdr-"+which+".sh", log)
	return r, log
}

// tailLog: x tail -n 20 <기록> >&2
func tailLog(log string) { xrun(os.Stderr, os.Stderr, "tail", "-n", "20", log) }

// xtest: x test -f|-e <경로> 가 0인가
func xtest(flag, p string) bool { return xrun(nil, os.Stderr, "test", flag, p) == 0 }
