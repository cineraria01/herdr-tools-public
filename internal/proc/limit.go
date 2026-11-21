package proc

// 바깥 명령 시간 상한(2026-11-21 go-followups-2): 맥 hc-fill 바퀴 하나가 자식 `git remote get-url origin`이 커널 안에서 잠들어
// 37분 멈췄다 — 상한 없이 기다려 그동안 대기열이 하나도 뜨지 않았다. git·hc·hc-lane·ssh·gh·curl은 여기 Run·Output을 거쳐
// 상한 안에서만 기다린다. 넘으면 그 무리를 거두고 「시간 넘김 <명령 앞부분> <초>」 한 줄 — 결과는 모름(TimeoutCode)이다.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// TimeoutCode: 상한에 걸린 명령의 종료 코드(coreutils timeout과 같은 124) — 부른 쪽 셋 결과에서는 모름이다
const TimeoutCode = 124

// Grace: 상한에 걸린 무리를 TERM한 뒤 KILL까지
var Grace = 2 * time.Second

// TimeoutError: 상한에 걸림
type TimeoutError struct {
	Head string
	D    time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("시간 넘김 %s %d", e.Head, int(e.D/time.Second))
}

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
// 로컬 git HERDR_TIMEOUT_GIT(60), 원격 git·hc·ssh·gh·curl HERDR_TIMEOUT_REMOTE(120), hc-lane HERDR_TIMEOUT_LANE(600)
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
		s := firstWord(rest)
		if s == "wait" || (s == "run" && !has(rest, "--bg")) || has(rest, "--timeout") {
			return 0 // 기다림이 일이거나 스스로 상한을 받은 명령
		}
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "ssh", "gh", "curl":
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "hc-lane":
		switch firstWord(rest) {
		case "land", "wait", "watch":
			return 0 // 검증·착지·기다림이 일이다
		}
		return envSec("HERDR_TIMEOUT_LANE", 600) // 레인 띄우기는 그 저장소 herdr-setup.sh(의존성 설치·빌드)를 기다린다
	}
	return 0
}

func has(a []string, x string) bool {
	for _, s := range a {
		if s == x {
			return true
		}
	}
	return false
}

func firstWord(a []string) string {
	for _, s := range a {
		if !strings.HasPrefix(s, "-") {
			return s
		}
	}
	return ""
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

// head: 사건·로그 줄에 적는 명령 앞부분(이름 + 인자 넷까지)
func head(args []string) string {
	a := append([]string{filepath.Base(args[0])}, args[1:]...)
	if len(a) > 5 {
		a = a[:5]
	}
	return strings.Join(a, " ")
}

// Run: c.Run() 자리 — Limit(c.Args) 상한 안에서 기다린다. 상한이 있으면 새 무리로 띄우고(표준 입력이 이 프로세스의 것이면 무리를
// 나누지 않는다 — 단말에서 SIGTTIN), 넘으면 무리째 TERM → Grace → KILL 하고 *TimeoutError. 그때 c.Stdout에 받던 출력은 믿지 않는다
func Run(c *exec.Cmd) error {
	d := Limit(c.Args)
	if d <= 0 {
		return c.Run()
	}
	if c.SysProcAttr == nil && c.Stdin != os.Stdin {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	grouped := c.SysProcAttr != nil && c.SysProcAttr.Setpgid
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		select {} // 멈추는 중 — 새 자식을 띄우지 않는다(Wait와 같음)
	}
	if err := c.Start(); err != nil {
		curMu.Unlock()
		return err
	}
	pid := c.Process.Pid
	if grouped {
		bounded[pid] = true
	}
	curMu.Unlock()
	defer func() { curMu.Lock(); delete(bounded, pid); curMu.Unlock() }()
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		return err
	case <-t.C:
	}
	e := &TimeoutError{Head: head(c.Args), D: d}
	fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
	if grouped {
		StopGroup(pid, Grace)
	} else if err := c.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "%s: %d 를 끄지 못했다: %v\n", Who, pid, err)
	}
	// ponytail: 신호도 안 먹는 커널 대기(D 상태)면 거두지 못한다 — 몇 초만 더 기다리고 놓아 둔다(기다리는 고루틴은 그 프로세스와 함께 남음)
	select {
	case <-done:
	case <-time.After(Grace + 3*time.Second):
	}
	return e
}

// Combined: c.CombinedOutput() 자리 — 상한에 걸리면 출력 없이 *TimeoutError
func Combined(c *exec.Cmd) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	if err := Run(c); err != nil {
		var te *TimeoutError
		if errors.As(err, &te) {
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
		var te *TimeoutError
		if errors.As(err, &te) {
			return nil, err
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}
