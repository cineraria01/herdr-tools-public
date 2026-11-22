//go:build !windows

package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// 재감사 3바퀴 merger/R-3M07: 상한 없는 명령(검증·착지 훅 — bash)이 끝났는데 남긴 배경 프로세스가 출력 관을 쥐면 유예만큼만 기다리고
// 모름(124, lastTimeout — land는 75로 다시)으로 끝난다. 옛 판: 그 배경 프로세스가 끝날 때까지 hc-land land가 끝나지 않았다
func TestRunCmdHeldPipeBounded(t *testing.T) {
	d := t.TempDir()
	var out bytes.Buffer
	lastTimeout = false
	t.Cleanup(func() { lastTimeout = false })
	t0 := time.Now()
	rc := runCmd(cmdOpt{stdout: &out, stderr: &out}, "sh", "-c", "sleep 40 & echo $! >\"$0/bg\"; echo done", d)
	el := time.Since(t0)
	if b, err := os.ReadFile(d + "/bg"); err == nil {
		if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 1 {
			syscall.Kill(p, syscall.SIGKILL)
		}
	}
	if el > 30*time.Second || rc != proc.TimeoutCode || !lastTimeout {
		t.Fatalf("출력 관을 쥔 배경 프로세스: rc=%d lastTimeout=%v 걸림 %v, 기대 %d·참·유예 안", rc, lastTimeout, el, proc.TimeoutCode)
	}
	// 대조: 관을 놓으면 그대로
	if rc := runCmd(cmdOpt{stdout: &out}, "sh", "-c", "exit 3"); rc != 3 || lastTimeout {
		t.Fatalf("보통 명령: rc=%d lastTimeout=%v", rc, lastTimeout)
	}
}
