//go:build !windows

package main

import "syscall"

// closeInherited: 물려받은 fd 3~1023을 자식에게 넘기지 않는다
func closeInherited() {
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
}
