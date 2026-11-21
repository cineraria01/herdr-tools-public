//go:build !windows

package clitest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// B29: 상한을 넘긴 명령은 그 무리(손자 포함)째 꺼지고 RC 124·Timeout으로 돌아온다 — 시험 마감까지 매달리지 않는다
func TestRunTimeoutKillsGroup(t *testing.T) {
	pidf := filepath.Join(t.TempDir(), "pid")
	t0 := time.Now()
	r := Cmd{Max: 300 * time.Millisecond}.Run("sh", "-c", `sleep 30 & echo $! >"$1"; wait`, "sh", pidf)
	if !r.Timeout || r.RC != 124 || !strings.Contains(r.Err, "시간 넘김") {
		t.Fatalf("시간 넘김이어야: %+v", r)
	}
	if d := time.Since(t0); d > 10*time.Second {
		t.Fatalf("상한 뒤 곧 돌아와야: %v", d)
	}
	b, err := os.ReadFile(pidf)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 0 || proc.Alive(pid) {
		t.Fatalf("손자(%d)도 꺼져야", pid)
	}
}

func TestRunNormal(t *testing.T) {
	r := Cmd{}.Run("sh", "-c", `echo out; echo err >&2; exit 3`)
	if r.Out != "out" || r.Err != "err\n" || r.RC != 3 || r.Timeout {
		t.Fatalf("%+v", r)
	}
	if r := (Cmd{}).Run("/없는/명령"); r.RC != 127 {
		t.Fatalf("못 띄우면 127: %+v", r)
	}
}
