package hcfill_test

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// R-30: leftwOK — 살아 있는 후보의 소유(lsof)를 못 읽으면 「확인됨」이 아니다(남은 것 0으로 통과시키지 않는다)
func TestLeftwOKLsofFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows")
	}
	T := t.TempDir()
	k := &K{t: t, T: T, DATA: T + "/data"}
	// 이 시험 폴더 밖 경로로 「…/hc-fill watch」 꼴 명령 줄(명령 줄만으로는 이 시험 것인지 모르는 후보)
	other := t.TempDir()
	c := exec.Command("bash", "-c", `exec -a "$0" sleep 60`, other+"/hc-fill watch")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { c.Process.Kill(); c.Wait() }()
	for i := 0; ; i++ {
		if l, ok := pgrepOK("-f", other+"/hc-fill watch"); ok && len(l) > 0 {
			break
		}
		if i == 500 {
			t.Fatal("후보 프로세스가 목록에 안 보인다")
		}
		exec.Command("sleep", "0.02").Run()
	}
	fake := t.TempDir()
	if err := os.WriteFile(fake+"/lsof", []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("PATH")
	t.Setenv("PATH", fake+":"+path)
	if l, ok := k.leftwOK(); ok {
		t.Fatalf("lsof 실패인데 확인됨: %v", l)
	}
	t.Setenv("PATH", path)
	if l, ok := k.leftwOK(); !ok || len(l) != 0 {
		t.Fatalf("진짜 lsof: 남은 %v 확인 %v — 이 시험 watch.lock을 연 것은 없어야", l, ok)
	}
}
