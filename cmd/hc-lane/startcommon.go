package main

// 띄우기·clean·gc·park의 도우미(hc-lane-go-start). 대상 기계 쪽 판정은 셸 판과 같은 sh -c 조각을 hc exec로 보낸다 — --on이면
// 그 기계에서 돌아야 해서(그 기계에 Go 판이 없을 수 있다). 공통(on·repo·core·die·q·plain·cfg·repoDefaults·ffmain·leaseX·지휘 판정)은
// common.go·hcx.go·ffmain.go·lease.go·coord.go(land·report 레인)의 것을 쓴다.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/tri"
)

var (
	branch, name, brief, swarg, integ, minage         string
	carry                                             string // --carry <이 기계 tar>: 작업 트리를 만든 직후 그 안에 푼다(hc-fill — 지휘 기계 .qa 자료)
	read, direct, resume, archive, dry, steps, rbFlag bool
	revive                                            bool // --revive: 있는 작업 트리에 이어 띄움(launchrevive.go)
	cmdName                                           string
	top, wt, rx, undo                                 string
	norepo                                            bool
	selfTerm                                          string // 부른 터미널($HC_TERMINAL_ID — clean이 자기 자신을 뺀다)
	selfPath                                          string // 이 바이너리 절대 경로(감시·사건을 부른다)
)

// startSetup: selfPath = 이 바이너리(링크를 따른 실제 경로)
func startSetup() { selfPath = selfExe() }

func errf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprint(os.Stderr, msg)
	oplog.Log("hc-lane", "", oplog.Warn, -1, strings.TrimSuffix(msg, "\n"))
}

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
	if on != "" || core != "" {
		return xjson(stdout, stderr, args)
	}
	c := command(args)
	c.Stdout, c.Stderr = stdout, stderr
	return code(proc.Run(c)) // git·hc는 상한 안에서만(proc.Limit) — 넘기면 124(모름, gofix M3·E02)
}

// rcHc: hc exec 자신의 실패·꼴이 다른 응답·잘린 출력 — 모름. 프로그램의 종료 코드(0 있음·1 없음 …)와 겹치지 않는 값
const rcHc = 125

// xjson: hc exec --json으로 돌려 hcjson.ParseExec로 읽는다(hc-exec-result, 2026-11-23 — --json 없이는 hc 실패도 종료 1이라
// 「없음」과 못 갈랐다). 결과면 프로그램의 출력을 쓰고 그 종료 코드(시간 넘김 124, 신호 128+신호 — --json 없을 때와 같은 값),
// hc 실패·꼴이 다름·잘림이면 rcHc
func xjson(stdout, stderr io.Writer, args []string) int {
	a := xargv(args)
	c := command(append([]string{"hc", "exec", "--json"}, a[2:]...))
	var ob, eb bytes.Buffer
	c.Stdout, c.Stderr = &ob, &eb
	err := proc.Run(c)
	if errors.Is(err, proc.ErrTimeout) {
		return 124
	}
	r, perr := hcjson.ParseExec(ob.Bytes(), eb.Bytes(), code(err))
	if stderr == nil {
		stderr = io.Discard
	}
	if perr != nil {
		fmt.Fprintf(stderr, "hc-lane: %v\n", perr)
		fmt.Fprintln(stderr, "hc-lane: 다음: hc machine ls로 그 기계가 연결됐는지 본다(끊겼으면 붙은 뒤 같은 줄을 다시, 오래면 다른 기계로)") // agent-skill-guides
		return rcHc
	}
	stderr.Write(r.Stderr)
	if r.Truncated {
		fmt.Fprintln(stderr, "hc-lane: hc exec가 출력을 잘랐다 — 모름")
		return rcHc
	}
	if stdout != nil {
		stdout.Write(r.Stdout)
	}
	switch {
	case r.TimedOut:
		return 124
	case r.Code < 0:
		return 128 + r.Signal
	}
	return r.Code
}

// xout: $(x …) — 표준 출력(끝 줄바꿈 뺌)·종료 코드, 오류 출력은 화면. xoutQ는 오류 출력을 버림(2>/dev/null)
func xout(args ...string) (string, int)  { return xoutTo(os.Stderr, args) }
func xoutQ(args ...string) (string, int) { return xoutTo(nil, args) }

// xoutHc: xout과 같되 hc 자신의 실패(rcHc)면 그 한 줄로 끝낸다 — 뒤에 「git 저장소가 아니다」 같은 엉뚱한 까닭을 잇지 않게
func xoutHc(args ...string) (string, int) {
	var eb bytes.Buffer
	out, r := xoutTo(&eb, args)
	e := strings.TrimRight(eb.String(), "\n")
	if r != rcHc {
		os.Stderr.WriteString(eb.String())
		return out, r
	}
	i := strings.LastIndex(e, "\n") + 1 // hc 실패 줄은 끝 줄 — 앞(프로그램의 오류 출력)은 그대로
	os.Stderr.WriteString(e[:i])
	die("%s", strings.TrimPrefix(e[i:], "hc-lane: "))
	return "", r
}

func xoutTo(stderr io.Writer, args []string) (string, int) {
	if on == "" && core == "" {
		return outOf(stderr, args)
	}
	var b bytes.Buffer
	rc := xjson(&b, stderr, args)
	return trim(b.String()), rc
}

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
	return r == tri.Yes && !ll
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

// termPairs: hc ls --json에서 터미널마다 「<주소> <cwd>」. 1 hc ls 실패, 2 꼴을 못 읽음, 3 대상 기계(--on)가 연결돼 있지 않음.
// 2·3은 「터미널 없음」이 아니다 — 모르면 지우지 않는다(gofix E01, 재감사 R-06·R-07)
func termPairs() ([][2]string, int) {
	nowSeen = map[string]string{} // 새 목록이면 지금 폴더도 새로 본다(지우기 직전 재확인)
	out, r := outOf(nil, []string{"hc", "ls", "--json"})
	if r != 0 {
		return nil, 1
	}
	return parsePairs(out, on)
}

// jstr: JSON 문자열 값이면 그 글(null·숫자·빠짐은 false) — hcx.go jlistOK가 쓴다
func jstr(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// unkCwd: 모름 항목의 cwd 자리 표시 — 절대 경로가 아니라 실제 cwd와 겹치지 않는다
const unkCwd = "?(hc ls 항목을 못 읽음)"

// parsePairs: hcjson.ParseList — 입력 전체가 JSON 값 하나, 주소 없는 항목이 있으면 2. 주소는 읽히는데 다른 칸이 계약을 어긴 항목
// (살아 있는 터미널의 cwd가 절대 경로가 아님 포함 — 재감사2 lane/R-202)은 cwd를 unkCwd로 — 쓰는 중으로 본다(R-3M01). 끝난 터미널은 cwd가 없어도 되고(빈 글 — 작업 폴더를 쓰지 않는다). mach가 있으면 machines에 그 기계가 connected로
// 있어야 한다(목록에 그 기계 터미널이 빠진 것을 「아무도 안 씀」으로 읽지 않게) — 아니면 3
func parsePairs(out, mach string) ([][2]string, int) {
	l, err := hcjson.ParseList([]byte(out))
	if err != nil {
		return nil, 2
	}
	ps := make([][2]string, 0, len(l.Terminals))
	for _, t := range l.Terminals {
		c := t.CwdText // 원문 — busyTerms(hcjson.Uses)가 `..`를 디스크로 푼다(A3)
		if t.Unknown != "" {
			c = unkCwd // 모름 항목(R-3M01): cwd를 몰라 busyTerms가 어느 작업 폴더든 쓰는 것으로 본다
		}
		ps = append(ps, [2]string{t.Address, c})
	}
	if mach != "" {
		if m, ok := l.Machine(mach); !ok || m.State != "connected" {
			return nil, 3
		}
	}
	return ps, 0
}

// termWhy: termPairs의 0 아닌 판정을 사람이 읽을 한 줄로
func termWhy(r int) string {
	switch r {
	case 1:
		return "hc ls 실패"
	case 3:
		return "대상 기계 " + on + " 가 hc ls 기계 목록에 연결(connected)로 없다 — 그 기계의 터미널을 모른다"
	}
	return "hc ls --json 출력을 읽지 못했다(잘림·형식 — 터미널마다 address·cwd가 있어야 한다)"
}

// unkTerms: 대상 기계의 모름 항목 주소들(R-3M01)
func unkTerms(ps [][2]string) []string {
	var u []string
	for _, p := range ps {
		if a, ok := localAddr(p[0]); ok && p[1] == unkCwd {
			u = append(u, onp(a))
		}
	}
	return u
}

// gcNoteUnk: 대상 기계에 읽지 못한 항목(R-3M01 모름)이 있으면 그 터미널이 어느 작업 트리를 쓰는지 모른다 — busyTerms가 모든
// 작업 트리를 남기니 까닭을 한 줄로 찍는다(gc)
func gcNoteUnk(ps [][2]string) {
	if u := unkTerms(ps); len(u) > 0 {
		fmt.Printf("hc-lane gc: hc ls에 읽지 못한 터미널 항목(%s) — 어느 작업 트리를 쓰는지 몰라 이 기계의 작업 트리는 고르지 않는다(모름 때문에 건너뜀)\n", strings.Join(u, " "))
	}
}

// busyTerms: 대상 기계의 터미널이고 cwd가 작업 폴더(wt) 안인 것 「<주소> <cwd>」. skipSelf면 부른 터미널 자신은 뺀다(clean).
// 이 기계면 디스크로 같은 폴더인지까지 본다(hcjson.Uses — 다른 철자 cwd, gofix5 lane/R-4A02). --on은 글자로만(그 기계 경로는 여기서
// stat할 수 없다 — 한계, docs/hc-lane/gc.md). 띄운 폴더로 아무도 안 쓰면 대상 기계의 나머지 터미널마다 지금 폴더(hc get
// foreground.cwd — 띄운 뒤 cd로 들어온 터미널, 2026-11-23 cwd-now-inuse)도 본다. 못 얻으면 「<주소> ?(지금 폴더 모름: …)」 — 쓰는 중으로
func busyTerms(ps [][2]string, skipSelf bool) []string {
	var b, rest []string
	under := hcjson.Uses
	if on != "" {
		under = hcjson.Under
	}
	for _, p := range ps {
		a, ok := localAddr(p[0])
		if !ok || (skipSelf && on == "" && a == selfTerm) {
			continue
		}
		if p[1] == unkCwd || under(p[1], wt) {
			b = append(b, onp(a)+" "+p[1])
		} else {
			rest = append(rest, onp(a))
		}
	}
	if len(b) > 0 {
		return b
	}
	for _, a := range rest {
		if c, ok := nowCwd(a); !ok {
			return []string{a + " ?(지금 폴더 모름: " + c + ")"}
		} else if c != "" && under(c, wt) {
			return []string{a + " " + c + "(지금 폴더)"}
		}
	}
	return nil
}

// nowSeen: 이번 termPairs 목록에서 본 터미널마다 nowCwd 결과(앞 글자 「?」면 모름) — gc가 작업 트리마다 다시 묻지 않게
var nowSeen = map[string]string{}

// nowCwd: 터미널 a(onp 꼴)의 지금 폴더 — hc get 한 번(상한 10초). (폴더, true), 끝남·없음은 ("", true), 모르면(get 실패·꼴 틀림·
// foreground.cwd 없는 판) (까닭, false)
func nowCwd(a string) (string, bool) {
	if v, ok := nowSeen[a]; ok {
		return strings.TrimPrefix(v, "?"), !strings.HasPrefix(v, "?")
	}
	v := ""
	switch t, found, err := hcGet(10, a); {
	case err != nil:
		v = "?hc get 실패"
	case !found || t.State != "live":
	case t.NowCwd == "":
		v = "?hc get에 foreground.cwd 없음"
	default:
		v = t.NowCwd
	}
	nowSeen[a] = v
	return strings.TrimPrefix(v, "?"), !strings.HasPrefix(v, "?")
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
