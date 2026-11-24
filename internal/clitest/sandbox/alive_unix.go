//go:build !windows

package sandbox

import (
	"errors"
	"syscall"
)

// alive: 그 pid가 있는가(EPERM — 남의 프로세스 — 도 있음). 모르면 있음(지우지 않는 쪽)
func alive(pid int) bool { return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }
