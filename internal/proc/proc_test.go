package proc

// 프로세스 무리 시험(병합기 요구 1): 자기가 띄운 무리만 끄고, pid 재사용은 시작 시각으로 가린다.

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func TestCode(t *testing.T) {
	if rc := Code(exec.Command("sh", "-c", "exit 3").Run()); rc != 3 {
		t.Fatalf("종료 3: %d", rc)
	}
	if rc := Code(exec.Command("sh", "-c", "kill -TERM $$").Run()); rc != 128+int(syscall.SIGTERM) {
		t.Fatalf("TERM은 143: %d", rc)
	}
	if rc := Code(exec.Command("/없는/명령").Run()); rc != 127 {
		t.Fatalf("못 띄우면 127: %d", rc)
	}
}

func TestStopGroupKillsWholeGroup(t *testing.T) {
	c, bg := startGroup(t, "trap '' TERM; wait") // 우두머리는 TERM을 무시 — 유예 뒤 KILL
	done := make(chan struct{})
	go func() { c.Wait(); close(done) }()
	StopGroup(c.Process.Pid, 300*time.Millisecond)
	<-done
	if !gone(bg) {
		t.Fatal("무리 안 배경 sleep이 남았다")
	}
}

func TestWaitRecordAndStopCurrent(t *testing.T) {
	curMu.Lock()
	stopping = false // -count로 다시 돌 때 앞 회차의 StopCurrent를 지운다
	curMu.Unlock()
	rec := t.TempDir() + "/child"
	// 기록은 띄운 직후에 쓴다 — 자식은 잠시 기다렸다 본다
	if rc := Wait(exec.Command("sh", "-c", "for i in 1 2 3 4 5 6 7 8 9 10; do test -s "+rec+" && exit 4; sleep 0.2; done"), rec); rc != 4 {
		t.Fatalf("도는 동안 기록이 있어야 한다: %d", rc)
	}
	if _, err := os.Stat(rec); err == nil {
		t.Fatal("끝나면 기록을 지워야 한다")
	}
	// StopCurrent는 지금 기다리는 무리를 끄고, 그 뒤 Wait는 돌아오지 않는다(이 시험의 마지막에 — 다른 Wait 시험을 막지 않게)
	d := t.TempDir()
	done := make(chan int, 2)
	go func() {
		done <- Wait(exec.Command("sh", "-c", "echo $$ >"+d+"/lead; sleep 30 & echo $! >"+d+"/bg; wait"), "")
	}()
	pid := func(n string) int {
		for i := 0; i < 100; i++ {
			if b, err := os.ReadFile(d + "/" + n); err == nil && strings.TrimSpace(string(b)) != "" {
				p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
				return p
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal(n + "가 뜨지 않았다")
		return 0
	}
	lead, bg := pid("lead"), pid("bg")
	StopCurrent(time.Second)
	if !gone(bg) || !gone(lead) {
		t.Fatal("무리 전체가 끝나야 한다")
	}
	go func() { done <- Wait(exec.Command("touch", d+"/new"), "") }()
	select {
	case rc := <-done:
		t.Fatalf("StopCurrent 뒤 Wait는 돌아오지 않아야: %d", rc)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(d + "/new"); err == nil {
		t.Fatal("StopCurrent 뒤에는 새 자식을 띄우지 않는다")
	}
}

func TestChildBusy(t *testing.T) {
	rec := t.TempDir() + "/child"
	// 살아 있고 시작 시각이 같으면 도는 중
	c, bg := startGroup(t, "wait")
	pid := c.Process.Pid
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\t"+StartTime(pid)+"\n"), 0o666)
	if !ChildBusy(rec, time.Second) {
		t.Fatal("살아 있는 착지를 도는 중으로 봐야 한다")
	}
	// 시작 시각이 다르면 pid 재사용 — 기록만 지우고 건드리지 않는다
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\tMon Jan  1 00:00:00 2001\n"), 0o666)
	if ChildBusy(rec, time.Second) || !Alive(pid) || !Alive(bg) {
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
	st := StartTime(pid2)
	c2.Wait()
	os.WriteFile(rec, []byte(strconv.Itoa(pid2)+"\t"+st+"\n"), 0o666)
	if ChildBusy(rec, time.Second) {
		t.Fatal("끝난 착지는 도는 중이 아니다")
	}
	if !gone(bg2) {
		t.Fatal("끝난 착지 무리에 남은 프로세스를 거둬야 한다")
	}
}
