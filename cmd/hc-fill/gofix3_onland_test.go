//go:build !windows

package main

// gofix3-proc(재감사 2바퀴 fill R-213): 실행기(_onland)가 죽어 잠금이 풀려도 그 명령은 살아 있다 — 다음 실행기는 앞 명령(자식 기록
// <이름>.child)이 끝난 뒤에만 같은 이름을 돌린다. 옛 판: 기록이 없어 두 명령이 동시에 돌았다

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

func TestGofix3OnlandPrev(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	ran, seen := d+"/ran", d+"/seen"
	// 명령: 돌면서 자기 자식 기록이 있는지 본다(seen) — 명령 무리가 기록에 올랐는가
	onlandEnv(t, d, "test -s \"$HC_DATA_CHILD\" && touch "+seen+"; touch "+ran)
	t.Setenv("HC_DATA_CHILD", onlandDir()+"/dev.child")
	old := onlandPoll
	onlandPoll = 50 * time.Millisecond
	t.Cleanup(func() { onlandPoll = old })
	if err := os.MkdirAll(onlandDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// 죽은 실행기가 남긴 명령 흉내: 살아 있는 무리와 그 「pid 시작 시각」 기록
	prev := exec.Command("sleep", "30")
	prev.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := prev.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { syscall.Kill(-prev.Process.Pid, syscall.SIGKILL); prev.Wait() }()
	st, err := proc.StartTime(prev.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onlandDir()+"/dev.child", []byte(strconv.Itoa(prev.Process.Pid)+"\t"+st+"\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := onlandPend("dev", "a", "t1", "s1"); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() { done <- onlandRun("dev") }()
	time.Sleep(time.Second)
	if isFile(ran) {
		t.Fatal("앞 명령이 도는데 같은 이름 명령을 겹쳐 돌렸다")
	}
	syscall.Kill(-prev.Process.Pid, syscall.SIGKILL)
	prev.Wait()
	select {
	case rc := <-done:
		if rc != 0 {
			t.Fatalf("rc %d", rc)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("앞 명령이 끝났는데 실행기가 안 끝남")
	}
	if !isFile(ran) {
		t.Fatal("앞 명령이 끝난 뒤 명령을 돌리지 않았다")
	}
	if !isFile(seen) {
		t.Fatal("명령이 도는 동안 자식 기록(<이름>.child)이 없었다")
	}
	if _, err := os.Stat(onlandDir() + "/dev.child"); err == nil {
		t.Fatal("끝난 명령의 자식 기록이 남았다")
	}
}
