//go:build !windows

package main

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// [B19] run의 자식은 쓸기와 같은 프로세스 무리다 — launchd·사람이 쓸기 무리를 SIGKILL해도 자식(hc 등)이 남지 않는다(셸 판 subprocess와 같음)
func TestRunSameGroup(t *testing.T) {
	r := run(10*time.Second, nil, "sh", "-c", "ps -o pgid= -p $$")
	got, err := strconv.Atoi(strings.TrimSpace(r.out))
	if r.err != nil || err != nil {
		t.Fatalf("ps: %+v", r)
	}
	if got != syscall.Getpgrp() {
		t.Fatalf("자식 무리 %d ≠ 쓸기 무리 %d", got, syscall.Getpgrp())
	}
}
