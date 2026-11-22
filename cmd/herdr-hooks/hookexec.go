package main

// 훅 안의 바깥 명령(git·ps·hc) — 훅 제한(settings.json timeout: cotenant 5초·loop-no-question 10초) 안에서만 기다린다
// (2026-11-22 gofix-hooks: 아스트라 H01·H02·H03·E04·E05, Fable M2). proc.Run·Output은 로컬 git 60초·hc 120초를 기다리고
// 고아를 다음 실행에서야 거둬, 훅이 먼저 꺼지면 상한이 일하지 않고 git이 그 사이 일을 끝냈다.
// 자식은 감독 무리 안에서 돈다(supervise — hookexec_unix.go): 무리 우두머리는 훅과 관으로 이어진 작은 감독 셸이고,
// 훅이 어떻게 끝나든(정상 반환·기한·TERM·무리째 KILL) 그 무리를 통째로 KILL한다 — 손자까지(재감사 R-M01: CommandContext는
// 직접 자식만 끄고 WaitDelay는 관만 닫아, 기한 뒤에도·자식이 먼저 끝난 뒤에도 손자가 완료 표식을 썼다).
// 상한은 min(proc.Limit(같은 명령), 훅 예산의 남은 시간). 넘기거나 출력 관이 안 닫히면 종료 코드 timeoutRC —
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
// 못 띄우면 127, 상한 넘김·출력 관 안 닫힘이면 timeoutRC, 신호로 끝나면 137. 돌아올 때 그 자식의 무리(손자 포함)는 없다
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
	c.WaitDelay = 200 * time.Millisecond // 자식이 끝난 뒤 관을 쥔 손자를 기다리는 시간 — 넘기면 ErrWaitDelay(그 손자는 stop이 끈다)
	stop, err := supervise(c)
	if err != nil {
		return "", "", 127
	}
	err = c.Run()
	stop()
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
