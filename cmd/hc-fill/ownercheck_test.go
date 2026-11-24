package main

import (
	"os"
	"testing"

	"herdr-tools/internal/tri"
)

// watch-agent-guard: hc-lane coord --check가 「에이전트가 끝나 셸만 남음」(종료 1)이면 주인 없음이 확인된 것 — 모름(미룸)이 아니다.
// 터미널은 살아 있으니 hc get(pget)으로는 끝남을 확인 못 해 이 글 없이는 그 주인의 레인을 매 바퀴 미뤘다
func TestOwnerCheckShellOnly(t *testing.T) {
	dir := t.TempDir()
	fake := "#!/bin/sh\necho \"hc-lane coord --check: t1 의 에이전트가 끝나 셸만 남음(전경 zsh -l)\" >&2\nexit 1\n"
	if err := os.WriteFile(dir+"/hc-lane", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	if st, why := ownerCheck("t1"); st != tri.No {
		t.Fatalf("셸만 남은 주인 = %v(%s), 바람 없음", st, why)
	}
}
