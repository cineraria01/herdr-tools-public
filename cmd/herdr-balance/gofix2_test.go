//go:build !windows

package main

// gofix2 재감사(아스트라 1바퀴) 지적의 실패 시험 — 이름 앞 R-ID는 보고서 지적 ID.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// R-12: 직접 자식(herdr-app)은 0으로 끝났는데 자손이 출력 관을 쥐고 남으면 c.Run이 WaitDelay(3초) 뒤 ErrWaitDelay로 돌아온다.
// 그때도 runLim은 「모름」(오류)이고, 돌아올 때 그 무리(관을 쥔 자손)는 거둬져 있어야 한다 — 남으면 뒤에 완료 파일을 쓴다.
func TestR12RunLimReapsPipeHolder(t *testing.T) {
	d := t.TempDir()
	sub, slp, done := filepath.Join(d, "sub.pid"), filepath.Join(d, "sleep.pid"), filepath.Join(d, "done")
	// 자손(서브셸)은 표준 출력을 물려받아 관을 쥔 채 sleep을 기다렸다가 완료 파일을 쓴다. 우두머리는 바로 0
	app := "#!/bin/sh\n( sleep 8 & echo $! >" + slp + "; wait; : >" + done + " ) &\necho $! >" + sub + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(d, "herdr-app"), []byte(app), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":/usr/bin:/bin")
	pidOf := func(p string) int {
		b, _ := os.ReadFile(p)
		n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		return n
	}
	t.Cleanup(func() {
		for _, p := range []string{sub, slp} {
			if n := pidOf(p); n > 0 {
				syscall.Kill(n, syscall.SIGKILL)
			}
		}
	})
	_, _, _, err := runLim(context.Background(), 15*time.Second, "herdr-app", "ls")
	if err == nil {
		t.Fatal("출력 관을 쥔 자손이 남아 ErrWaitDelay로 돌아왔으면 결과 모름(오류)이어야")
	}
	pids := []int{pidOf(sub), pidOf(slp)}
	for _, n := range pids {
		if n <= 0 {
			t.Fatalf("자손 pid를 못 읽음 — 시험 준비 실패 %v", pids)
		}
	}
	// 거둔다면 runLim이 돌아올 때 이미 끝났다 — 좀비 거두기 몫으로 3초만 더 본다
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) && (syscall.Kill(pids[0], 0) == nil || syscall.Kill(pids[1], 0) == nil) {
		time.Sleep(100 * time.Millisecond)
	}
	for _, n := range pids {
		if syscall.Kill(n, 0) == nil {
			t.Errorf("ErrWaitDelay 뒤 관을 쥔 자손 %d가 남았다 — runLim이 그 무리를 거두지 않음 (err=%v)", n, err)
		}
	}
}
