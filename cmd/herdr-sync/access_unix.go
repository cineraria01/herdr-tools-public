//go:build !windows

package main

import "syscall"

// canExec: [ -x 파일 ] — 이 사용자가 실행할 수 있는가
func canExec(f string) bool { return syscall.Access(f, 1) == nil }
