//go:build !windows

package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const groupsSupported = true

var (
	sigTerm     = syscall.SIGTERM
	sigKill     = syscall.SIGKILL
	stopSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP}
)

func setGroup(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Setpgid = true
}

func isGrouped(c *exec.Cmd) bool { return c.SysProcAttr != nil && c.SysProcAttr.Setpgid }

func killGroup(pg int, s syscall.Signal) error { return syscall.Kill(-pg, s) }

func groupGone(pg int) bool { return syscall.Kill(-pg, 0) == syscall.ESRCH }

func isNoProcess(err error) bool { return errors.Is(err, syscall.ESRCH) }

// CloseInherited: 물려받은 fd 3~1023(바깥 병합기의 잠금 fd 18·19 등)을 자식에게 넘기지 않는다. Go가 여는 fd는 기본 O_CLOEXEC
func CloseInherited() {
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
}

// Alive: 그 pid가 있는가(남의 것이라 신호를 못 보내도 있음)
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func signaled(ee *exec.ExitError) (int, bool) {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return int(ws.Signal()), true
	}
	return 0, false
}

// reraise: 받은 신호의 기본 동작으로 끝난다(부모가 보는 종료 상태가 신호를 받지 않았을 때와 같게)
func reraise(s os.Signal) {
	if sig, ok := s.(syscall.Signal); ok {
		signal.Reset(s)
		syscall.Kill(os.Getpid(), sig)
		time.Sleep(time.Second)
		os.Exit(128 + int(sig))
	}
	os.Exit(1)
}

// gate: c를 /bin/sh로 감싸, 부모가 내려 주는 fd에서 「go」 한 줄을 받은 뒤에야 실제 명령으로 exec 한다(pid·시작 시각은 그대로).
// 부모가 「go」 전에 죽으면 관이 닫혀 명령 없이 125로 끝난다. 돌려준 파일에 「go\n」을 쓰고 닫는다
func gate(c *exec.Cmd) (*os.File, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("관 만들기: %w", err)
	}
	fd := 3 + len(c.ExtraFiles)
	c.ExtraFiles = append(c.ExtraFiles, r)
	script := fmt.Sprintf(`IFS= read -r g <&%d && [ "$g" = go ] || exit 125; exec %d<&-; exec "$@"`, fd, fd)
	c.Args = append([]string{"sh", "-c", script, "sh", c.Path}, c.Args[1:]...)
	c.Path = "/bin/sh"
	return w, nil
}
