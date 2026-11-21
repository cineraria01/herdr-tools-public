//go:build !windows

package lockx

// 잠금 시험(병합기 요구 2): 잠금은 자식에 물리지 않는다 — 쥔 프로세스가 죽으면 자식이 살아 있어도 바로 풀린다.

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

// TestMain: HERDR_LOCKX_TEST_HOLD=<잠금 파일>이면 이 시험 바이너리가 잠금 쥔 도구 흉내 — 잠금을 쥐고 오래 사는 자식(sleep)을 남긴 채 SIGKILL로 죽는다
func TestMain(m *testing.M) {
	if lk := os.Getenv("HERDR_LOCKX_TEST_HOLD"); lk != "" {
		f, busy, err := TryLock(lk)
		if busy || err != nil {
			os.Exit(3)
		}
		c := exec.Command("sleep", "30")
		if err := c.Start(); err != nil {
			os.Exit(4)
		}
		if err := os.WriteFile(lk+".child", []byte(strconv.Itoa(c.Process.Pid)), 0o666); err != nil {
			os.Exit(5)
		}
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
		f.Close() // 닿지 않음 — f를 끝까지 쥔다
	}
	os.Exit(m.Run())
}

func TestLockNotInheritedByChildren(t *testing.T) {
	lk := t.TempDir() + "/merger.lock"
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HERDR_LOCKX_TEST_HOLD="+lk)
	err := c.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("흉내 도구가 SIGKILL로 끝나지 않았다: %v", err)
	}
	b, err := os.ReadFile(lk + ".child")
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	defer syscall.Kill(pid, syscall.SIGKILL)
	if syscall.Kill(pid, 0) != nil {
		t.Fatalf("자식 sleep(%d)이 살아 있어야 시험이 된다", pid)
	}
	f, busy, err := TryLock(lk)
	if err != nil || busy {
		t.Fatalf("쥔 프로세스가 죽으면 자식이 살아 있어도 잠금을 바로 얻어야 한다: busy=%v err=%v", busy, err)
	}
	defer f.Close()
	// 쥔 동안은 남이 못 잡는다(같은 프로세스라도 다른 열기면 flock은 따로)
	if f2, busy, err := TryLock(lk); err != nil || !busy || f2 != nil {
		t.Fatalf("쥔 잠금은 busy여야: busy=%v err=%v", busy, err)
	}
	f.Close()
	f3, err := Lock(lk)
	if err != nil {
		t.Fatalf("놓은 뒤 Lock은 바로: %v", err)
	}
	f3.Close()
}
