//go:build !windows

package main

import "syscall"

// closeInherited: 물려받은 fd(바깥 병합기의 잠금 fd 19 등)를 자식에게 넘기지 않는다(요구 2). Go가 여는 fd는 기본 O_CLOEXEC
func closeInherited() {
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
}
