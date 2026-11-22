//go:build !windows

package main

// 재감사 ops R-09: 상위 sync가 게시기의 수명도 쥔다 — 게시 중 sync가 TERM을 받으면 게시기와 그 배경 자식(push 흉내)까지 끝난 뒤에
// sync가 끝난다(옛 판: 게시기가 상한 없는 bash라 무리 밖 — sync가 -15로 끝난 뒤에도 게시기 push가 돌아 미러에 main이 생겼다)

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	ct "herdr-tools/internal/clitest"
)

func (s *sx) pubTerm() {
	hang := s.T + "/pubhang"
	os.Remove(hang)
	c := exec.Command(s.sync)
	c.Env = append(os.Environ(), "PUB_FAKE_HANG="+hang)
	if err := c.Start(); err != nil {
		s.t.Fatal(err)
	}
	for i := 0; i < 1200 && !ct.Exists(hang); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	var pub, bg int
	fmt.Sscanf(ct.Read(hang), "%d", &pub)
	fmt.Sscanf(ct.Read(hang+".bg"), "%d", &bg)
	if pub <= 1 || bg <= 1 {
		c.Process.Kill()
		c.Wait()
		s.t.Fatal("가짜 게시기가 뜨지 않았다")
	}
	defer syscall.Kill(pub, syscall.SIGKILL)
	defer syscall.Kill(bg, syscall.SIGKILL)
	c.Process.Signal(syscall.SIGTERM)
	c.Wait()
	gone := func(p int) bool {
		for i := 0; i < 20; i++ { // 끝난 자식이 init에 거둬질 짧은 틈만
			if syscall.Kill(p, 0) == syscall.ESRCH {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}
	s.ok("게시 중 TERM: sync가 끝날 때 게시기와 그 배경 자식도 끝났다", gone(pub) && gone(bg))
}
