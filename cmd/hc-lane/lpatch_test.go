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

// R-H02(gofix2): 시간 넘김이 아닌 git 오류(patch-id 128)도 모름 — lastTimeout이라 land가 75(다시)로 끝나고 병합기가 반송하지 않는다.
// 실제 불일치만 종료 2
func TestLpatchGitErrorMarksUnknown(t *testing.T) {
	bin := t.TempDir()
	fake := "#!/bin/sh\ncase \" $* \" in *\" patch-id \"*) cat >/dev/null; echo 'fatal: x' >&2; exit 128 ;; *\" diff \"*) echo 'diff --git a/f b/f' ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	lastTimeout = false
	t.Cleanup(func() { lastTimeout = false })
	if _, ok := lpatch("HEAD", "HEAD"); ok || !lastTimeout {
		t.Fatalf("git 오류로 못 구한 lpatch는 ok=false·lastTimeout=true여야: ok=%v lastTimeout=%v", ok, lastTimeout)
	}
}
