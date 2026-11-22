//go:build !windows

package main

// gofix2 재감사(아스트라 1바퀴) 지적의 실패 시험 — 이름 앞 R-ID는 보고서 지적 ID.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// R-12: 직접 자식(herdr-app)은 0으로 끝났는데 자손이 출력 관을 쥐고 남으면 c.Run이 WaitDelay(3초) 뒤 ErrWaitDelay로 돌아온다.
// 그때도 runLim은 「모름」(오류)이고, 돌아올 때 그 무리(관을 쥔 자손)는 거둬져 있어야 한다 — 남으면 뒤에 완료 파일을 쓴다.
// 시험 규칙(②-F): 자손의 PID를 파일로 읽어 신호·생존 조회를 하지 않는다(재감사 base R-213) — 자손은 시작 표식을 쓰고, 살아남으면
// 6초 뒤 늦은 표식을 쓴다(수명이 유한하다). 경로는 환경으로 따로 넘긴다
func TestR12RunLimReapsPipeHolder(t *testing.T) {
	d := clitest.TempDir(t)
	start, done := filepath.Join(d, "start"), filepath.Join(d, "done")
	// 자손(서브셸)은 표준 출력을 물려받아 관을 쥔 채 sleep을 기다렸다가 늦은 표식을 쓴다. 우두머리는 바로 0
	clitest.Put(t, filepath.Join(d, "herdr-app"), "#!/bin/sh\n( : >\"$R12_START\"; sleep 6; : >\"$R12_DONE\" ) &\nexit 0\n")
	clitest.Chmod(t, filepath.Join(d, "herdr-app"), 0o755)
	t.Setenv("PATH", d+":/usr/bin:/bin")
	t.Setenv("R12_START", start)
	t.Setenv("R12_DONE", done)
	t0 := time.Now()
	_, _, _, err := runLim(context.Background(), 15*time.Second, "herdr-app", "ls")
	clitest.WaitFile(t, start, time.Second) // 원인(관을 쥔 자손)이 실제로 있었다
	if err == nil {
		t.Fatal("출력 관을 쥔 자손이 남아 ErrWaitDelay로 돌아왔으면 결과 모름(오류)이어야")
	}
	time.Sleep(time.Until(t0.Add(7 * time.Second)))
	if !clitest.Absent(t, done) {
		t.Errorf("ErrWaitDelay 뒤 관을 쥔 자손이 남아 늦은 표식을 썼다 — runLim이 그 무리를 거두지 않음 (err=%v)", err)
	}
}
