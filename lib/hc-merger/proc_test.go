package main

// 프로세스 무리·잠금 시험(요구 1·2): 자기가 띄운 무리만 끄고, 잠금은 자식에 물리지 않는다.

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain: HC_MERGER_TEST_HOLD=<잠금 파일>이면 이 시험 바이너리가 병합기 흉내 — 잠금을 쥐고 오래 사는 자식(sleep)을 남긴 채 SIGKILL로 죽는다
func TestMain(m *testing.M) {
	if lk := os.Getenv("HC_MERGER_TEST_HOLD"); lk != "" {
		if busy, err := flockNB(lk); busy || err != nil {
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
	}
	os.Exit(m.Run())
}

func TestLockNotInheritedByChildren(t *testing.T) {
	lk := t.TempDir() + "/merger.lock"
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HC_MERGER_TEST_HOLD="+lk)
	if err := c.Run(); code(err) != 128+int(syscall.SIGKILL) {
		t.Fatalf("흉내 병합기가 SIGKILL로 끝나지 않았다: %v", err)
	}
	b, err := os.ReadFile(lk + ".child")
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	defer syscall.Kill(pid, syscall.SIGKILL)
	if !alive(pid) {
		t.Fatalf("자식 sleep(%d)이 살아 있어야 시험이 된다", pid)
	}
	busy, err := flockNB(lk)
	if err != nil || busy {
		t.Fatalf("병합기가 죽으면 자식이 살아 있어도 잠금을 바로 얻어야 한다: busy=%v err=%v", busy, err)
	}
}

// startGroup: sh 스크립트를 새 무리로 띄우고, 그 안의 배경 sleep pid를 돌려준다
func startGroup(t *testing.T, script string) (*exec.Cmd, int) {
	t.Helper()
	f := t.TempDir() + "/bg"
	c := exec.Command("sh", "-c", "sleep 30 & echo $! >"+f+"; "+script)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(f); err == nil && strings.TrimSpace(string(b)) != "" {
			pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
			return c, pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("배경 sleep이 뜨지 않았다")
	return nil, 0
}

func gone(pid int) bool {
	for i := 0; i < 50; i++ {
		// 좀비는 우리 자식이 아니면 init이 거둔다 — kill 0이 ESRCH가 될 때까지
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestStopGroupKillsWholeGroup(t *testing.T) {
	c, bg := startGroup(t, "trap '' TERM; wait") // 우두머리는 TERM을 무시 — 유예 뒤 KILL
	done := make(chan struct{})
	go func() { c.Wait(); close(done) }()
	stopGroup(c.Process.Pid, 300*time.Millisecond)
	<-done
	if !gone(bg) {
		t.Fatal("무리 안 배경 sleep이 남았다")
	}
}

func TestChildBusy(t *testing.T) {
	rec := t.TempDir() + "/child"
	// 살아 있고 시작 시각이 같으면 도는 중
	c, bg := startGroup(t, "wait")
	pid := c.Process.Pid
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\t"+startTime(pid)+"\n"), 0o666)
	if !childBusy(rec, time.Second) {
		t.Fatal("살아 있는 착지를 도는 중으로 봐야 한다")
	}
	// 시작 시각이 다르면 pid 재사용 — 기록만 지우고 건드리지 않는다
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\tMon Jan  1 00:00:00 2001\n"), 0o666)
	if childBusy(rec, time.Second) || !alive(pid) || !alive(bg) {
		t.Fatal("다시 쓰인 pid의 프로세스를 건드리면 안 된다")
	}
	if _, err := os.Stat(rec); err == nil {
		t.Fatal("기록을 지워야 한다")
	}
	syscall.Kill(-pid, syscall.SIGKILL)
	c.Wait()

	// 우두머리는 끝났는데 무리에 배경 프로세스가 남았으면(죽은 병합기의 착지가 남긴 것) 거둔다
	c2, bg2 := startGroup(t, "exit 0")
	pid2 := c2.Process.Pid
	st := startTime(pid2)
	c2.Wait()
	os.WriteFile(rec, []byte(strconv.Itoa(pid2)+"\t"+st+"\n"), 0o666)
	if childBusy(rec, time.Second) {
		t.Fatal("끝난 착지는 도는 중이 아니다")
	}
	if !gone(bg2) {
		t.Fatal("끝난 착지 무리에 남은 프로세스를 거둬야 한다")
	}
}

func TestIntSetting(t *testing.T) {
	for _, c := range []struct {
		v    string
		want int
	}{{"", 30}, {"17", 17}, {"0", 0}, {"abc", 30}, {"-5", 30}, {"1.5", 30}, {"99999999999999999999", 30}} {
		if got := intSetting("--tick", c.v, 30, 0); got != c.want {
			t.Errorf("intSetting(%q) = %d, 바람 %d", c.v, got, c.want)
		}
	}
	if got := intSetting("HC_LANE_MERGER_BATCH", "0", 3, 1); got != 3 {
		t.Errorf("묶음 0은 기본 3이어야: %d", got)
	}
}
