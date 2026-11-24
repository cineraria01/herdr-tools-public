//go:build windows

package proc

// Windows: 레인을 돌리지 않는다 — 빌드만 된다(교차 빌드·패키징). 무리가 없어 그 프로세스 하나만 끄고, 고아 기록·신호 전달·자식 게이트는 없다.

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

const groupsSupported = false

var (
	sigTerm     = syscall.SIGTERM
	sigKill     = syscall.SIGKILL
	stopSignals = []os.Signal{os.Interrupt}
)

func setGroup(c *exec.Cmd) {}

func isGrouped(c *exec.Cmd) bool { return false }

func killGroup(pg int, s syscall.Signal) error {
	p, err := os.FindProcess(pg)
	if err != nil {
		return err
	}
	return p.Kill()
}

func groupGone(pg int) bool { return !Alive(pg) }

func isNoProcess(err error) bool { return errors.Is(err, os.ErrProcessDone) }

// CloseInherited: Windows는 핸들을 기본으로 물려주지 않는다 — 할 일 없음(빌드만 깨지지 않게)
func CloseInherited() {}

// Alive: 그 pid가 있는가
func Alive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

func signaled(ee *exec.ExitError) (int, bool) { return 0, false }

func reraise(s os.Signal) { os.Exit(1) }

func gate(c *exec.Cmd) (*os.File, error) {
	return nil, errors.New("자식 기록 게이트는 유닉스만")
}
