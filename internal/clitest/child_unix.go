//go:build !windows

package clitest

import "syscall"

func killGroup(pg int, s syscall.Signal) error { return syscall.Kill(-pg, s) }

func (ch *Child) begin() {}

// reapNoHang: (잠금 안) 우두머리가 끝났으면 거두고 종료 코드(신호면 128+신호)
func (ch *Child) reapNoHang() (int, bool) {
	var ws syscall.WaitStatus
	pid, err := syscall.Wait4(ch.Pid, &ws, syscall.WNOHANG, nil)
	if err == syscall.EINTR || (err == nil && pid == 0) {
		return 0, false
	}
	if err != nil { // ECHILD — 거둘 것이 없다(이 실행이 띄운 것이라 이미 거둔 것). 끝난 것으로
		return -1, true
	}
	if ws.Signaled() {
		return 128 + int(ws.Signal()), true
	}
	return ws.ExitStatus(), true
}
