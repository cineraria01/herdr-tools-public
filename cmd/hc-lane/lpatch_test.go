package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 승인 착지의 patch-id 계산이 git 상한에 걸리면 lastTimeout — 뒤따르는 die가 land에서 75(다시)로 끝난다(gofix M01).
// merger-batch-audited가 lpatch를 gitx.PatchID로 옮기며 이 표시를 잃었다(감사 atovp2i-1gnz78 중간)
func TestLpatchTimeoutMarksLastTimeout(t *testing.T) {
	bin := t.TempDir()
	fake := "#!/bin/sh\nfor a in \"$@\"; do [ \"$a\" = diff ] && exec sleep 5; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	lastTimeout = false
	t.Cleanup(func() { lastTimeout = false })
	if _, ok := lpatch("HEAD", "HEAD"); ok || !lastTimeout {
		t.Fatalf("상한에 걸린 lpatch는 ok=false·lastTimeout=true여야: ok=%v lastTimeout=%v", ok, lastTimeout)
	}
}
