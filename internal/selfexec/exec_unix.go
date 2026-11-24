//go:build !windows

package selfexec

import "syscall"

// execve: 이 프로세스를 그 바이너리로 바꾼다(돌아오면 실패)
func execve(bin string, argv, env []string) error { return syscall.Exec(bin, argv, env) }
