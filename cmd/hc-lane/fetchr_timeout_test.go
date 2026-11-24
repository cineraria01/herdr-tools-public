package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/proc"
)

// dedupe-helpers ①: fetch가 상한에 걸리면(모름) 반쪽 오류 글로 잠금 경합을 판정해 다시 돌지 않는다 — 병합기 fetchr(재감사 R-C01)와 같게.
// 옛 hc-lane 판은 시간 넘김 뒤에도 「cannot lock ref」 반쪽 글을 보고 세 번 더 돌았다
func TestFetchrTimeoutNoRetry(t *testing.T) {
	bin := t.TempDir()
	cnt := filepath.Join(bin, "n")
	fake := "#!/bin/sh\necho x >>" + cnt + "\necho \"error: cannot lock ref 'refs/x'\" >&2\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_REMOTE", "1")
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	t.Setenv("HC_LANE_FETCH_WAITS", "0 0 0")
	if rc := fetchr(nil, nil, "fetch", "-q", "origin"); rc != proc.TimeoutCode {
		t.Fatalf("시간 넘김이면 %d: %d", proc.TimeoutCode, rc)
	}
	b, _ := os.ReadFile(cnt)
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Fatalf("시간 넘김 뒤 다시 돌면 안 된다: %d번", n)
	}
}
