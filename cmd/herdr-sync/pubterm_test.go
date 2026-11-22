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
	s.pubKill()
}

// 재감사 2바퀴 ops R-204: 게시 중 상위 sync가 SIGKILL로 죽으면 게시기는 고아 기록에 올라 있다 — 다음 --check는 그 게시기가 끝날 때까지
// 75로 미루거나(소유 모름), 소유를 확인하고 거둔 뒤에야 돈다(옛 판: 기록 없이 띄워 --check가 앞 push와 겹쳐 돌고 게시기가 남았다)
func (s *sx) pubKill() {
	hang := s.T + "/pubhang2"
	before := map[string]bool{}
	if es, err := os.ReadDir(s.T + "/tmp"); err == nil {
		for _, e := range es {
			before[e.Name()] = true
		}
	}
	defer func() { // SIGKILL한 실행이 남긴 임시 폴더만 치운다(뒤 「임시 파일이 남지 않는다」 확인은 정상 실행 몫)
		es, _ := os.ReadDir(s.T + "/tmp")
		for _, e := range es {
			if !before[e.Name()] {
				os.RemoveAll(s.T + "/tmp/" + e.Name())
			}
		}
	}()
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
	c.Process.Kill()
	c.Wait()
	if pub <= 1 || bg <= 1 {
		s.t.Fatal("가짜 게시기가 뜨지 않았다")
	}
	defer syscall.Kill(pub, syscall.SIGKILL)
	defer syscall.Kill(bg, syscall.SIGKILL)
	alive := syscall.Kill(pub, 0) == nil
	s.run("--check")
	ended := syscall.Kill(pub, 0) == syscall.ESRCH && syscall.Kill(bg, 0) == syscall.ESRCH
	for i := 0; i < 20 && !ended; i++ { // 거둔 자식이 init에 거둬질 짧은 틈만
		time.Sleep(50 * time.Millisecond)
		ended = syscall.Kill(pub, 0) == syscall.ESRCH && syscall.Kill(bg, 0) == syscall.ESRCH
	}
	s.ok("게시 중 SIGKILL: 다음 --check는 앞 게시기를 미루거나(75) 거둔 뒤에만 돈다", alive && (s.r.RC == 75 || (s.r.RC == 0 && ended)))
}
