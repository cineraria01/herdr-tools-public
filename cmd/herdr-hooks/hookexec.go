package main

// 훅 안의 바깥 명령(git·ps·hc) — 훅 제한(settings.json timeout: cotenant 5초·loop-no-question 10초) 안에서만 기다린다
// (2026-11-22 gofix-hooks: 아스트라 H01·H02·H03·E04·E05, Fable M2). proc.Run·Output은 자식을 새 무리로 띄우고 로컬 git 60초·
// hc 120초를 기다려, 훅이 먼저 꺼지면 상한이 일하지 않고 git이 고아로 남았다.
// 여기서는 훅 무리 안에 띄운다(무리를 나누지 않는다 — Claude Code가 훅을 무리째 끄면 자식도 함께 끝난다, 셸 판과 같음).
// 상한은 min(proc.Limit(같은 명령), 훅 예산의 남은 시간). 넘기거나 출력 관이 안 닫히면 그 자식을 KILL하고 종료 코드 timeoutRC —
// 부른 쪽은 「모름」으로 읽는다(실패·없음·깨끗함이 아니다 — 안전 판정은 막는 쪽).

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"herdr-tools/internal/proc"
)

const timeoutRC = proc.TimeoutCode

// hookDeadline: 이 훅 판정의 기한 — 훅 들어올 때 hookBudget으로 정한다(시험처럼 한 프로세스에서 여러 번 불러도 그때마다)
var hookDeadline = time.Now().Add(4 * time.Second)

func hookBudget(d time.Duration) { hookDeadline = time.Now().Add(d) }

// hookRun: path args…를 돌려 표준 출력·표준 오류·종료 코드. limitAs는 proc.Limit에 줄 명령 꼴(이름이 git·hc인 것으로 —
// 시험의 가짜 git도 git 상한을 받게), max는 그 명령만의 상한(0 = 없음). env가 nil이면 이 프로세스의 환경.
// 못 띄우면 127, 상한 넘김·출력 관 안 닫힘이면 timeoutRC, 신호로 끝나면 137
func hookRun(limitAs []string, max time.Duration, env []string, stdin string, path string, args ...string) (stdout, stderr string, rc int) {
	d := time.Until(hookDeadline)
	if l := proc.Limit(limitAs); l > 0 && l < d {
		d = l
	}
	if max > 0 && max < d {
		d = max
	}
	if d <= 0 {
		return "", "", timeoutRC
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	c := exec.CommandContext(ctx, path, args...)
	c.Env = env
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	c.WaitDelay = 200 * time.Millisecond // 기한(또는 자식 끝남) 뒤 관을 쥔 손자를 기다리는 시간 — 넘기면 ErrWaitDelay
	err := c.Run()
	if ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay) {
		return "", "", timeoutRC
	}
	if err == nil {
		return o.String(), e.String(), 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return "", "", 127
	}
	if rc = ee.ExitCode(); rc < 0 {
		rc = 128 + 9 // 신호로 끝남(우리가 끈 것은 위에서 걸렀다) — 결과는 실패
	}
	return o.String(), e.String(), rc
}
