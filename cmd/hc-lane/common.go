package main

// 공통 도우미(셸 lib/hc-lane/common.sh·land.sh의 같은 이름 함수). 명령·인자·출력 꼴은 셸 판 그대로 — 계약 시험이 PATH의 가짜
// git·hc로 끊김을 흉내 내고 인자 꼴로 어느 호출인지 가린다. git 결과는 셋(있음·없음·모름): 종료 코드를 늘 본다.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/oplog"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/shx"
)

var (
	on   string // --on <기계>(빈 값이면 이 기계)
	core string // 1이면 로컬도 hc 코어 문맥(hc exec --) — clean이 켠다
	repo string // --repo
	// 정리 일(임시 파일 지우기 등): die·끝날 때 부른다(셸 trap EXIT)
	cleanups []func()
)

// exit: 정리 일을 하고 끝낸다
func exit(code int) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
	oplog.Exited("hc-lane", code)
	os.Exit(code)
}

// dieHook: die가 끝내기 전에 부른다(clean은 결과 JSON 「stopped」 — 셸 판 die와 같게)
var dieHook func(msg string)

// die: 「hc-lane: <글>」 한 줄 뒤 종료 1
func die(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	rc := 1
	if timeoutExit != 0 && lastTimeout {
		rc = timeoutExit
		msg += " (바로 앞 바깥 명령이 시간 상한에 걸림 — 결과 모름, 같은 줄을 다시)"
	}
	fmt.Fprintf(os.Stderr, "hc-lane: %s\n", msg)
	oplog.Fatal("hc-lane", "", oplog.Die, rc, msg)
	if dieHook != nil {
		dieHook(msg)
	}
	exit(rc)
}

// lastTimeout: 바로 앞 runCmd가 상한에 걸렸다(124 — 모름). timeoutExit가 0이 아니면 그 뒤 die는 1(실패) 대신 이 값(land: 75 — 다시,
// gofix M01: 병합기·레인이 「시간 넘김」을 반송·실패로 읽지 않게)
var (
	lastTimeout bool
	timeoutExit int
)

func eprintf(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }

// code: 명령의 셸 종료 코드(못 띄우면 127, 신호면 128+신호)
func code(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) || errors.Is(err, proc.ErrTimeout) || errors.Is(err, exec.ErrWaitDelay) {
		return proc.Code(err) // 신호면 128+그 신호(gofix L09), 상한에 걸림(proc.Limit)이면 124 — 모름
	}
	return 127
}

// cmdOpt: 명령 하나 부르기의 입출력
type cmdOpt struct {
	dir    string
	stdout io.Writer // nil이면 버림
	stderr io.Writer // nil이면 버림
	stdin  io.Reader // nil이면 /dev/null
	env    []string  // 더할 환경(K=V)
	// noLimit: 상한 없이 이 프로세스의 무리에서(작업 트리를 바꾸는 rebase·merge — 중간에 끊으면 반쯤 바뀐 작업 트리와 멈춘 rebase가
	// 남는다, gofix M2. 셸 판도 상한이 없다. 무리가 같아 hc-lane을 끄는 신호는 함께 받는다)
	noLimit bool
}

func runCmd(o cmdOpt, name string, args ...string) int {
	c := exec.Command(name, args...)
	c.Dir, c.Stdout, c.Stderr, c.Stdin = o.dir, o.stdout, o.stderr, o.stdin
	if len(o.env) > 0 {
		c.Env = append(os.Environ(), o.env...)
	}
	var err error
	if o.noLimit {
		err = c.Run()
	} else if proc.Limit(c.Args) == 0 {
		// 상한 없는 명령(검증·착지 훅 — bash 등)도 끝난 뒤 출력 관은 유예만큼만 기다린다(재감사 3바퀴 merger/R-3M07 — 옛 판은 남긴
		// 배경 프로세스가 끝날 때까지 hc-lane land가 끝나지 않았다). 관을 쥔 것이 남아도 우두머리는 끝났고 그 출력은 다 받았다 — 결과는
		// 우두머리의 종료 코드 그대로(ErrWaitDelay는 그것이 0일 때만 온다, 0 아니면 ExitError). 잘린 것은 남은 프로세스의 뒤 출력뿐
		// (4바퀴 merger/R-4M01 · lane/R-4M01 — 124로 두면 통과한 검증이 land 1·반송이 됐다)
		err = proc.RunPipeGrace(c, pipeGrace)
		if errors.Is(err, exec.ErrWaitDelay) && c.ProcessState != nil && c.ProcessState.Success() {
			eprintf("hc-lane: 경고 — %s 은 0으로 끝났는데 남은 배경 프로세스가 출력 관을 %v 넘게 쥐어 관을 닫았다(그 뒤 출력은 버림)\n", name, pipeGrace)
			err = nil
		}
	} else {
		err = proc.Run(c) // git·hc·ssh 등은 상한 안에서만(proc.Limit)
	}
	lastTimeout = errors.Is(err, exec.ErrWaitDelay) || errors.Is(err, proc.ErrTimeout)
	return code(err)
}

// pipeGrace: 상한 없는 명령이 끝난 뒤 출력 관을 기다리는 유예
const pipeGrace = 10 * time.Second

// trim: $(…)처럼 끝 줄바꿈을 모두 뺀다
func trim(s string) string { return strings.TrimRight(s, "\n") }

// git 부르기 — 이름 끝 글자: (없음) 출력 화면 · Q 출력 모두 버림 · O 표준 출력만 버림
func gitRun(args ...string) int {
	return runCmd(cmdOpt{stdout: os.Stdout, stderr: os.Stderr}, "git", args...)
}
func gitRunQ(args ...string) int { return runCmd(cmdOpt{}, "git", args...) }
func gitRunO(args ...string) int { return runCmd(cmdOpt{stderr: os.Stderr}, "git", args...) }

// gitOut: $(git …) — 출력(끝 줄바꿈 뺌)과 종료 코드. 오류 출력은 화면. 실패해도 출력은 돌려준다(셸 $(…)와 같음 — 부르는 쪽이
// 종료 코드를 먼저 본다)
func gitOut(args ...string) (string, int) {
	var b bytes.Buffer
	rc := runCmd(cmdOpt{stdout: &b, stderr: os.Stderr}, "git", args...)
	return trim(b.String()), rc
}

// gitOutQ: $(git … 2>/dev/null)
func gitOutQ(args ...string) (string, int) {
	var b bytes.Buffer
	rc := runCmd(cmdOpt{stdout: &b}, "git", args...)
	return trim(b.String()), rc
}

// gitOut2: $(git … 2>&1) — 표준 출력과 오류 출력을 한데
func gitOut2(env []string, args ...string) (string, int) {
	var b bytes.Buffer
	rc := runCmd(cmdOpt{stdout: &b, stderr: &b, env: env}, "git", args...)
	return trim(b.String()), rc
}

// gitWrite2: gitOut2와 같되 상한 없이(cmdOpt.noLimit — 작업 트리를 바꾸는 rebase)
func gitWrite2(env []string, args ...string) (string, int) {
	var b bytes.Buffer
	rc := runCmd(cmdOpt{stdout: &b, stderr: &b, env: env, noLimit: true}, "git", args...)
	return trim(b.String()), rc
}

// lines: 줄로 나눔(빈 글은 빈 목록)
func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// field2: ls-remote 꼴 「값 이름」 줄에서 이름이 ref인 값들(awk '$2 == r { print $1 }' — 여럿이면 줄바꿈으로)
func field2(list, ref string) string {
	var o []string
	for _, l := range lines(list) {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == ref {
			o = append(o, f[0])
		}
	}
	return strings.Join(o, "\n")
}

// firstWord: ${v%%[[:space:]]*}
func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n\r\v\f"); i >= 0 {
		return s[:i]
	}
	return s
}

// cut: ${s:0:n}
func cut(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// utc: date -u +%Y-%m-%dT%H:%M:%SZ
func utc() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

func unixNow() string { return strconv.FormatInt(time.Now().Unix(), 10) }

// sleepS: sleep <초>(소수 됨). 못 읽으면 쉬지 않는다(sleep 명령이 실패한 것과 같게)
func sleepS(w string) {
	if s, err := strconv.ParseFloat(w, 64); err == nil && s > 0 {
		time.Sleep(time.Duration(s * float64(time.Second)))
	}
}

// waitsEnv: ${<키>:-1 2 4} 낱말들 + 끝 표지 「-」
func waitsEnv(k string) []string {
	v := os.Getenv(k)
	if v == "" {
		v = "1 2 4"
	}
	return append(strings.Fields(v), "-")
}

// plain: 출력하는 명령에 따옴표 없이 넣으므로 브랜치 이름은 이 글자만 받는다
func plain(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("/_.-", r)) {
			return false
		}
	}
	return true
}

// q: 셸 따옴표('…', 안의 '는 '\”)
func q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// xArgs: 대상 기계에서(셸 x) — --on이면 hc exec --on, core=1이면 로컬도 hc exec
func xArgs(args ...string) []string {
	if on != "" {
		return append([]string{"hc", "exec", "--on", on, "--timeout", "600", "--"}, args...)
	}
	if core != "" {
		return append([]string{"hc", "exec", "--timeout", "600", "--"}, args...)
	}
	return args
}

func xRun(o cmdOpt, args ...string) int {
	a := xArgs(args...)
	return runCmd(o, a[0], a[1:]...)
}

func xOut(args ...string) (string, int) {
	var b bytes.Buffer
	rc := xRun(cmdOpt{stdout: &b, stderr: os.Stderr}, args...)
	return trim(b.String()), rc
}

// zxArgs: zx <폴더> <명령…>의 명령 줄 — 대상 기계의 그 폴더에서. --on(hc 코어 환경)이면 로그인 zsh로 ~/.zshrc까지 읽고 돈다(shx.ZX)
func zxArgs(dir string, args ...string) []string {
	return xArgs(append([]string{"sh", "-c", shx.ZX, "hc-lane", dir, on + core}, args...)...)
}

var lockContention = regexp.MustCompile(`cannot lock ref|unable to update local ref|is at [0-9a-f]+ but expected|로컬 레퍼런스를 업데이트할 수 없습니다|\.lock'을 만들 수 없습니다`)

// fetchrCmd: 명령(git fetch를 그대로 또는 x·zx로)을 돌리고, 같은 클론의 다른 fetch·착지와 참조 잠금이 겹친 실패면
// HC_LANE_FETCH_WAITS(기본 1 2 4초) 쉬고 다시(셸 fetchr). 오류 출력은 시도마다 모았다가 그대로 내보낸다
func fetchrCmd(stdout, stderr io.Writer, argv ...string) int {
	rc := 0
	for _, w := range waitsEnv("HC_LANE_FETCH_WAITS") {
		var e bytes.Buffer
		rc = runCmd(cmdOpt{stdout: stdout, stderr: &e}, argv[0], argv[1:]...)
		if stderr != nil {
			stderr.Write(e.Bytes())
		}
		if rc == 0 || w == "-" || !lockContention.Match(e.Bytes()) {
			break
		}
		if stderr != nil {
			fmt.Fprintf(stderr, "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — %s초 뒤 다시\n", w)
		}
		sleepS(w)
	}
	return rc
}

// fetchr: fetchr git <args…>
func fetchr(stdout, stderr io.Writer, args ...string) int {
	return fetchrCmd(stdout, stderr, append([]string{"git"}, args...)...)
}

// repoDefaults: 원격과 기본 가지를 같은 조회 결과로 정한다. 조회 오류(ok=false)는 기본값 선택과 구분한다
func repoDefaults(dir string) (remote bool, base string, ok bool) {
	switch xRun(cmdOpt{}, "git", "-C", dir, "remote", "get-url", "origin") {
	case 0:
		refs, rc := xOut("git", "-C", dir, "for-each-ref", "--format=%(symref)", "refs/remotes/origin/HEAD")
		if rc != 0 {
			return false, "", false
		}
		base = strings.TrimPrefix(refs, "refs/remotes/origin/")
		if base == "" {
			base = "main"
		}
		return true, base, true
	case 2:
		refs, rc := xOut("git", "-C", dir, "for-each-ref", "--format=%(refname)", "refs/heads/main", "refs/heads/master")
		if rc != 0 {
			return false, "", false
		}
		base = "main"
		has := map[string]bool{}
		for _, l := range lines(refs) {
			has[l] = true
		}
		if !has["refs/heads/main"] && has["refs/heads/master"] {
			base = "master"
		}
		return false, base, true
	}
	return false, "", false
}

func originBase(dir string) (string, bool) {
	_, b, ok := repoDefaults(dir)
	return b, ok
}

// evdir: 상태 폴더 ${XDG_STATE_HOME:-$HOME/.local/state}/hc-lane
func evdir() string {
	s := os.Getenv("XDG_STATE_HOME")
	if s == "" {
		s = filepath.Join(os.Getenv("HOME"), ".local/state")
	}
	return s + "/hc-lane"
}

// reportCmd: (report_cmd …) — 이 바이너리의 report를 부른다. 표준 출력은 out(nil이면 버림), 오류 출력은 errw
func reportCmd(out, errw io.Writer, args ...string) int {
	return runCmd(cmdOpt{stdout: out, stderr: errw}, selfExe(), append([]string{"report"}, args...)...)
}

// trTo: tr -c '<글자들>' -  (바이트마다)
func trTo(s, keep string) string {
	b := []byte(s)
	for i, c := range b {
		if !strings.ContainsRune(keep, rune(c)) || c >= 0x80 {
			b[i] = '-'
		}
	}
	return string(b)
}

const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// laneChars: a-z 0-9 _ - 만
func laneChars(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// lnameOK: 레인 이름은 hc-fill과 같은 알파벳과 길이(맨 앞 - 은 git이 가지 이름으로 거절)
func lnameOK(s string) bool {
	return s != "" && s[0] != '-' && laneChars(s) && len(s) <= 32
}

// markLaneName <낸 이름>: 지금 가지 이름과 다르면 가지 설정 branch.<가지>.hcLane에 남긴다 — 다시 맞춤·고침 레인(<원래>-reland 등)이
// HC_LANE=<원래>로 내면 큐·착지·주차 표식은 <원래> 이름이라, clean이 이 이름으로 찾는다. 가지를 지우면 git이 그 설정도 지운다
func markLaneName(n string) {
	b, rc := gitOutQ("symbolic-ref", "-q", "--short", "HEAD")
	if rc != 0 || b == "" || b == n {
		return
	}
	if gitRunQ("config", "branch."+b+".hcLane", n) != 0 {
		eprintf("hc-lane: 경고 — 가지 설정 branch.%s.hcLane 에 낸 이름 %s 를 남기지 못했다(clean이 이 표식을 못 알아볼 수 있다)\n", b, n)
	}
}

// lmname: 이 레인 이름(HC_LANE, 없으면 가지 이름) — 표식·임대 ref에 쓸 수 있을 때만
func lmname() (string, bool) {
	n := os.Getenv("HC_LANE")
	if n == "" {
		n, _ = gitOutQ("branch", "--show-current")
	}
	if n == "" || !laneChars(n) {
		return "", false
	}
	return n, true
}
