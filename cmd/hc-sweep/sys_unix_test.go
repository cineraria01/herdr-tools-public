//go:build !windows

package main

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// [B19·R-11] run의 자식은 따로 무리(우두머리 = 그 자식)로 뜬다 — 상한·신호 때 손자까지 무리째 거두고, 쓸기가 SIGKILL로 죽으면
// 다음 회차가 고아 기록으로 거둔다(TestO2ParentKillNextRoundReaps). 쓸기 무리와 같으면 무리째 거둘 수 없다
func TestRunOwnGroup(t *testing.T) {
	r := run(10*time.Second, nil, "sh", "-c", "echo $$ $(ps -o pgid= -p $$)")
	f := strings.Fields(r.out)
	if r.err != nil || len(f) != 2 {
		t.Fatalf("ps: %+v", r)
	}
	pid, e1 := strconv.Atoi(f[0])
	pg, e2 := strconv.Atoi(f[1])
	if e1 != nil || e2 != nil || pg != pid || pg == syscall.Getpgrp() {
		t.Fatalf("자식 %d 무리 %d (쓸기 무리 %d) — 자식이 우두머리인 따로 무리여야", pid, pg, syscall.Getpgrp())
	}
}
