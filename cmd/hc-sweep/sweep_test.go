package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSelfTest(t *testing.T) {
	if err := selfTest(); err != nil {
		t.Fatal(err)
	}
}

// landedClean: git 결과를 차례로 돌려주는 가짜로 — 착지·더러움·증거 없음·모름
func TestLandedClean(t *testing.T) {
	dir := t.TempDir()
	ev := filepath.Join(dir, "events.tsv")
	os.WriteFile(ev, []byte("2026\tm\tt1\tland-x\t제출\tabc\t-\t3\n"), 0o644)
	seq := func(outs ...result) runner {
		i := 0
		return func(_ time.Duration, _ []string, _ string, args ...string) result {
			if i >= len(outs) {
				t.Fatalf("더 부름: %v", args)
			}
			i++
			return outs[i-1]
		}
	}
	ok := func(s string) result { return result{out: s} }
	term := map[string]any{"cwd": "/r.wt/land-x", "name": "land-x"}
	repo, br, why := landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok("- abc\n")), ev)
	if repo != "/r" || br != "land-x" || why != "" {
		t.Fatalf("착지: %q %q %q", repo, br, why)
	}
	if _, _, why = landedClean(term, seq(ok("?? x\n")), ev); why != "커밋 안 된 변경" {
		t.Fatal(why)
	}
	if _, _, why = landedClean(term, seq(ok(""), ok("land-x\n"), ok("/r/.git\n"), ok("+ abc\n")), ev); !strings.HasPrefix(why, "착지 전") {
		t.Fatal(why)
	}
	other := map[string]any{"cwd": "/r.wt/land-n", "name": "land-n"}
	if _, _, why = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok("")), ev); !strings.HasPrefix(why, "착지 증거 없음") {
		t.Fatal(why)
	}
	if repo, _, _ = landedClean(other, seq(ok(""), ok("land-n\n"), ok("/r/.git\n"), ok(""), ok("abc\trefs/hc-landed/land-n/1\n")), ev); repo != "/r" {
		t.Fatal("원격 표식이면 착지")
	}
	if _, _, why = landedClean(term, seq(ok(""), result{rc: 1}), ev); !strings.HasPrefix(why, "모름") {
		t.Fatal(why)
	}
}

// run: 상한 시간이면 무리째 거둔다(배경 자식도)
func TestRunTimeoutKillsGroup(t *testing.T) {
	pidf := filepath.Join(t.TempDir(), "pid")
	r := run(300*time.Millisecond, nil, "sh", "-c", "sleep 30 & echo $! >"+pidf+"; wait")
	if r.err == nil {
		t.Fatal("시간 초과가 아님")
	}
	b, _ := os.ReadFile(pidf)
	pid := strings.TrimSpace(string(b))
	time.Sleep(200 * time.Millisecond)
	if pid == "" {
		t.Fatal("자식 pid 없음")
	}
	var n int
	for _, c := range pid {
		n = n*10 + int(c-'0')
	}
	if alive(n) {
		t.Fatalf("배경 자식 %d가 남음", n)
	}
}
