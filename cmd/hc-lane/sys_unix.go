//go:build !windows

package main

// 유닉스 전용 호출(gofix L11 — Windows 교차 빌드가 깨지지 않게 sys_windows.go와 짝)

import "syscall"

// umask: 지금 생성 마스크(바꾸지 않고 읽는다)
func umask() int {
	um := syscall.Umask(0)
	syscall.Umask(um)
	return um
}

// closeInherited: 물려받은 fd(바깥 병합기 잠금 fd 18·19 등)를 자식에게 넘기지 않는다
func closeInherited() {
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
}

// execve: 이 프로세스를 그 바이너리로 바꾼다(돌아오면 실패)
func execve(bin string, argv, env []string) error { return syscall.Exec(bin, argv, env) }
