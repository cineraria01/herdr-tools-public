package proc

// 바깥 명령 시간 상한(2026-11-21 go-followups-2): 맥 hc-fill 바퀴 하나가 자식 `git remote get-url origin`이 커널 안에서 잠들어
// 37분 멈췄다 — 상한 없이 기다려 그동안 대기열이 하나도 뜨지 않았다. git·hc·hc-lane·ssh·gh·curl은 여기 Run·Output을 거쳐
// 상한 안에서만 기다린다. 넘으면 그 무리를 거두고 「시간 넘김 <명령·하위 명령> <초>」 한 줄 — 결과는 모름(ErrTimeout, TimeoutCode)이다.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeoutCode: 상한에 걸린 명령의 종료 코드(coreutils timeout과 같은 124) — 부른 쪽 셋 결과에서는 모름이다
const TimeoutCode = 124

// ErrTimeout: 상한에 걸렸거나(자식이 끝나지 않음) 자식이 끝났는데 출력 관이 닫히지 않았다 — 결과는 모름.
// errors.Is(err, proc.ErrTimeout)로 가린다. 「실패」·「없음」·「성공」으로 읽지 않는다(상태를 바꾸지 않고 다음 바퀴에)
var ErrTimeout = errors.New("시간 넘김")

// Grace: 상한에 걸린 무리를 TERM한 뒤 KILL까지. 자식이 끝난 뒤 출력 관을 기다리는 시간(WaitDelay)도 이것
var Grace = 2 * time.Second

// TimeoutError: 상한에 걸림(Pipe면 자식은 끝났는데 출력 관을 쥔 프로세스가 남음)
type TimeoutError struct {
	Head string
	D    time.Duration
	Pipe bool
}

func (e *TimeoutError) Error() string {
	if e.Pipe {
		return fmt.Sprintf("시간 넘김 %s 출력 관이 닫히지 않음", e.Head)
	}
	return fmt.Sprintf("시간 넘김 %s %d", e.Head, int(e.D/time.Second))
}

// Is: errors.Is(err, ErrTimeout)
func (e *TimeoutError) Is(t error) bool { return t == ErrTimeout }

// 기본 상한(초) — 환경 변수로 바꾼다(0이면 상한 없음)
func envSec(name string, def int) time.Duration {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		fmt.Fprintf(os.Stderr, "%s: 경고 — %s=%q 를 읽지 못해 기본 %d초\n", Who, name, v, def)
	}
	return time.Duration(def) * time.Second
}

// Limit: 명령(이름·인자)의 상한. 0 = 상한 없음(정해 둔 명령이 아니거나, 기다리는 것이 일인 명령 — hc wait·hc-lane land 등).
// 로컬 git HERDR_TIMEOUT_GIT(60), 원격 git·hc·ssh·gh·curl HERDR_TIMEOUT_REMOTE(120), hc-lane HERDR_TIMEOUT_LANE(600).
// 하위 명령은 맨 앞 낱말만, 옵션은 하위 명령의 옵션 자리(`--`·실행할 명령 앞)만 본다 — 옵션 값·넘기는 명령의 인자는 판정에 쓰지 않는다
func Limit(args []string) time.Duration {
	if len(args) == 0 {
		return 0
	}
	rest := args[1:]
	switch filepath.Base(args[0]) {
	case "git":
		switch s, next := gitSub(rest); s {
		case "fetch", "push", "pull", "ls-remote", "clone", "submodule", "worktree": // worktree: 큰 저장소 checkout
			return envSec("HERDR_TIMEOUT_REMOTE", 120)
		case "remote":
			if next == "update" || next == "prune" || next == "show" {
				return envSec("HERDR_TIMEOUT_REMOTE", 120)
			}
		}
		return envSec("HERDR_TIMEOUT_GIT", 60)
	case "hc":
		s := sub(rest)
		opts := hcOpts(s, rest)
		if s == "wait" || (s == "run" && !opts["--bg"]) || opts["--timeout"] {
			return 0 // 기다림이 일이거나 스스로 상한을 받은 명령
		}
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "ssh", "gh", "curl":
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "hc-lane":
		switch sub(rest) {
		case "land", "wait", "watch":
			return 0 // 검증·착지·기다림이 일이다
		}
		return envSec("HERDR_TIMEOUT_LANE", 600) // 레인 띄우기(옵션으로 시작하는 꼴)는 그 저장소 herdr-setup.sh(의존성 설치·빌드)를 기다린다
	}
	return 0
}

// sub: 하위 명령 — 맨 앞 낱말이 옵션이 아닐 때만(`hc-lane --name x …`처럼 옵션으로 시작하면 하위 명령 없음)
func sub(a []string) string {
	if len(a) > 0 && !strings.HasPrefix(a[0], "-") {
		return a[0]
	}
	return ""
}

// hcValue: 값을 받는 hc 옵션(그다음 낱말은 값)
var hcValue = map[string]bool{"--on": true, "--cwd": true, "--env": true, "--timeout": true, "--name": true, "--label": true,
	"--parent": true, "--if": true, "--output": true, "--version": true, "--from": true, "--via": true}

// hcOpts: hc 하위 명령 뒤 옵션 자리의 옵션들 — `--`에서, run·exec는 실행할 명령(첫 위치 인자)에서 멈춘다
func hcOpts(s string, rest []string) map[string]bool {
	m := map[string]bool{}
	for i := 1; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			k, _, hasEq := strings.Cut(a, "=")
			m[k] = true
			if hcValue[k] && !hasEq {
				i++
			}
			continue
		}
		if s == "run" || s == "exec" {
			break
		}
	}
	return m
}

// gitSub: git의 하위 명령과 그다음 낱말(-C·-c 등 값 받는 전역 옵션은 건너뜀)
func gitSub(a []string) (string, string) {
	for i := 0; i < len(a); i++ {
		switch s := a[i]; {
		case s == "-C" || s == "-c" || s == "--git-dir" || s == "--work-tree" || s == "--namespace":
			i++
		case strings.HasPrefix(s, "-"):
		default:
			if i+1 < len(a) {
				return s, a[i+1]
			}
			return s, ""
		}
	}
	return "", ""
}

var (
	plainWord = regexp.MustCompile(`^[a-z][a-z-]*$`)
	gitNested = map[string]bool{"remote": true, "worktree": true, "stash": true, "submodule": true, "notes": true, "config": true}
)

// head: 사건·로그 줄에 적는 명령 앞부분 — 이름과 하위 명령만(인자 값·사건 본문·주소·URL은 적지 않는다).
// git은 -C 경로까지(어느 저장소인지), -c 값은 「…」로, 하위 명령 뒤 낱말은 소문자 낱말(remote get-url 등)일 때만
func head(args []string) string {
	if len(args) == 0 {
		return ""
	}
	name := filepath.Base(args[0])
	rest := args[1:]
	switch name {
	case "git":
		out := []string{name}
		for i := 0; i < len(rest); i++ {
			s := rest[i]
			switch {
			case (s == "-C" || s == "--git-dir" || s == "--work-tree") && i+1 < len(rest):
				out = append(out, s, rest[i+1])
				i++
			case (s == "-c" || s == "--namespace") && i+1 < len(rest):
				out = append(out, s, "…")
				i++
			case strings.HasPrefix(s, "-"):
				out = append(out, s)
			default:
				out = append(out, s)
				if i+1 < len(rest) && gitNested[s] && plainWord.MatchString(rest[i+1]) {
					out = append(out, rest[i+1])
				}
				return strings.Join(out, " ")
			}
		}
		return strings.Join(out, " ")
	case "hc", "hc-lane", "gh":
		if s := sub(rest); s != "" && plainWord.MatchString(s) {
			return name + " " + s
		}
	}
	return name
}

// Run: c.Run() 자리 — Limit(c.Args) 상한 안에서 기다린다. 상한이 있으면 새 무리로 띄우고(표준 입력이 이 프로세스의 것이면 무리를
// 나누지 않는다 — 단말에서 SIGTTIN), 넘으면 무리째 TERM → Grace → KILL 하고 *TimeoutError(ErrTimeout). 자식이 끝났는데 출력 관이
// Grace 안에 안 닫히면(남은 손자가 쥠) 그 무리를 거두고 *TimeoutError{Pipe}. 두 경우 모두 c.Stdout·c.Stderr에 받던 출력은 믿지 않는다.
// 따로 무리로 띄운 자식은 고아 기록(orphan.go)에 남기고, 이 프로세스가 TERM·INT·HUP을 받으면 그 무리를 거둔 뒤 같은 신호로 끝난다
func Run(c *exec.Cmd) error {
	d := Limit(c.Args)
	if d <= 0 {
		return c.Run()
	}
	if c.SysProcAttr == nil && c.Stdin != os.Stdin {
		setGroup(c)
	}
	grouped := isGrouped(c)
	if c.WaitDelay == 0 {
		c.WaitDelay = Grace
	}
	if grouped {
		prepare() // 첫 무리 — 앞 실행의 고아 무리를 거두고, 신호를 받으면 무리째 거두게
	}
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		park() // 멈추는 중 — 새 자식을 띄우지 않는다(Wait와 같음)
	}
	if err := c.Start(); err != nil {
		curMu.Unlock()
		return err
	}
	pid := c.Process.Pid
	if grouped {
		bounded[pid] = ""
	}
	curMu.Unlock()
	if grouped {
		st, _ := StartTime(pid) // 모르면 빈 값 — 우리가 막 띄운 무리로 친다(ownGroup)
		curMu.Lock()
		if _, ok := bounded[pid]; ok {
			bounded[pid] = st
		}
		curMu.Unlock()
		record(pid, st)
	}
	finish := func() {
		curMu.Lock()
		delete(bounded, pid)
		stop := stopping
		curMu.Unlock()
		if stop {
			park() // 신호로 끈 자식의 결과를 부른 쪽이 읽지 않게 — 신호 처리 쪽이 무리를 다 거두고 끝낸다
		}
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		if errors.Is(err, exec.ErrWaitDelay) {
			e := &TimeoutError{Head: head(c.Args), D: d, Pipe: true}
			fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
			if grouped {
				stopOwned(pid, startOf(pid), Grace) // 관을 쥔 손자가 그 무리에 있으면 함께
			}
			finish()
			return e
		}
		if grouped && groupGone(pid) {
			unrecord(pid)
		} else if grouped {
			stopOwned(pid, startOf(pid), Grace) // 우두머리는 끝났는데 무리에 남은 배경 프로세스
		}
		finish()
		return err
	case <-t.C:
	}
	e := &TimeoutError{Head: head(c.Args), D: d}
	fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
	if grouped {
		stopOwned(pid, startOf(pid), Grace)
	} else if err := c.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "%s: %d 를 끄지 못했다: %v\n", Who, pid, err)
	}
	// ponytail: 신호도 안 먹는 커널 대기(D 상태)면 거두지 못한다 — WaitDelay·몇 초만 더 기다리고 놓아 둔다(그 고루틴은 그 프로세스와 함께 남고,
	// 출력 버퍼는 c.Wait가 돌아오기 전까지 그 고루틴이 쓸 수 있으니 부른 쪽은 시간 넘김 때 버퍼를 읽지 않는다 — Output·Combined)
	select {
	case <-done:
	case <-time.After(2*Grace + 3*time.Second):
	}
	finish()
	return e
}

func startOf(pid int) string {
	curMu.Lock()
	defer curMu.Unlock()
	return bounded[pid]
}

// Combined: c.CombinedOutput() 자리 — 상한에 걸리면 출력 없이 *TimeoutError
func Combined(c *exec.Cmd) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	if err := Run(c); err != nil {
		if errors.Is(err, ErrTimeout) {
			return nil, err
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

// Output: c.Output() 자리(표준 출력만 받는다, c.Stdout은 비워 둔다) — 상한에 걸리면 출력 없이 *TimeoutError
func Output(c *exec.Cmd) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout = &b
	if err := Run(c); err != nil {
		if errors.Is(err, ErrTimeout) {
			return nil, err
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}
