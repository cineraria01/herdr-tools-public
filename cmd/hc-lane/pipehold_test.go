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
)

// 재감사 3바퀴 merger/R-3M07: 상한 없는 명령(검증·착지 훅 — bash)이 끝났는데 남긴 배경 프로세스가 출력 관을 쥐면 유예만큼만 기다린다
// (옛 판: 그 배경 프로세스가 끝날 때까지 hc-lane land가 끝나지 않았다). 4바퀴 merger/R-4M01 · lane/R-4M01: 그때 결과는 우두머리의
// 종료 코드 그대로다 — 0이면 통과(경고 한 줄), 0 아니면 그 코드. 옛 판은 124(모름)라 통과한 검증이 land 1·반송이 됐다
func TestRunCmdHeldPipeBounded(t *testing.T) {
	lastTimeout = false
	t.Cleanup(func() { lastTimeout = false })
	for _, tc := range []struct {
		tail string
		want int
	}{{"echo done", 0}, {"exit 3", 3}} {
		d := t.TempDir()
		var out bytes.Buffer
		t0 := time.Now()
		rc := runCmd(cmdOpt{stdout: &out, stderr: &out}, "sh", "-c", "sleep 40 & echo $! >\"$0/bg\"; "+tc.tail, d)
		el := time.Since(t0)
		if b, err := os.ReadFile(d + "/bg"); err == nil {
			if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 1 {
				syscall.Kill(p, syscall.SIGKILL)
			}
		}
		if el > 30*time.Second || rc != tc.want || lastTimeout {
			t.Fatalf("출력 관을 쥔 배경 프로세스(%s): rc=%d lastTimeout=%v 걸림 %v, 기대 %d·거짓·유예 안", tc.tail, rc, lastTimeout, el, tc.want)
		}
	}
}
