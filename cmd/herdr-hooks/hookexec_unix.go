//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// reaperSh: 감독 셸 — 표준 입력(훅이 쥔 관의 읽는 끝)이 닫히면(훅이 끝남·죽음) 자기 무리를 통째로 KILL한다.
// 훅이 관에 쓰는 일은 없다 — read는 EOF에서만 돌아온다
const reaperSh = "read -r _; kill -KILL 0"

// supervise: c를 새 무리 G에서 돌게 한다. G의 우두머리는 훅과 관으로 이어진 감독 셸이라, 훅이 SIGKILL로 죽어도
// 관이 닫혀 감독이 G를 끈다. stop(c가 끝난 뒤 또는 기한에 부른다)은 G를 바로 KILL하고 감독을 거둔다 — 손자도 남지 않는다.
// 기한에 걸리면(CommandContext) 직접 자식만이 아니라 G를 끈다
func supervise(c *exec.Cmd) (stop func(), err error) {
	r, w, err := os.Pipe() // 둘 다 CLOEXEC — 자식(git 등)은 w를 물려받지 않는다
	if err != nil {
		return nil, err
	}
	sup := exec.Command("/bin/sh", "-c", reaperSh)
	sup.Stdin = r
	sup.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	err = sup.Start()
	r.Close() //nolint:errcheck // 읽는 끝은 감독만 쥔다
	if err != nil {
		w.Close() //nolint:errcheck
		return nil, err
	}
	pg := sup.Process.Pid
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pg}
	c.Cancel = func() error { return syscall.Kill(-pg, syscall.SIGKILL) }
	return func() {
		syscall.Kill(-pg, syscall.SIGKILL) //nolint:errcheck // 이미 다 끝났을 수 있다
		w.Close()                          //nolint:errcheck
		sup.Wait()                         //nolint:errcheck // KILL로 끝난 감독
	}, nil
}
