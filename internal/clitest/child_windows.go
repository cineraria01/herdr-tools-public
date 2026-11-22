//go:build windows

package clitest

import (
	"os"
	"syscall"
)

// windows: 무리가 없다 — 우두머리만 끈다
func killGroup(pg int, s syscall.Signal) error {
	p, err := os.FindProcess(pg)
	if err != nil {
		return err
	}
	return p.Kill()
}

func (ch *Child) begin() {
	ch.win = make(chan int, 1)
	go func() {
		st, err := ch.cmd.Process.Wait()
		if err != nil {
			ch.win <- -1
			return
		}
		ch.win <- st.ExitCode()
	}()
}

func (ch *Child) reapNoHang() (int, bool) {
	select {
	case rc := <-ch.win:
		return rc, true
	default:
		return 0, false
	}
}
