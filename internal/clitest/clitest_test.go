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

// 재감사 base R-15: 바깥 마감(-test.timeout)이 명령 상한보다 먼저 오면 명령 상한을 남은 시간 안(끄기·기다림 여유 포함)으로 줄여,
// 시험 바이너리가 마감 panic으로 죽기 전에 그 무리를 거두고 124로 돌아온다. 안쪽 실행은 기본 상한(60초)을 그대로 쓴다
func TestRunWithinTestDeadline(t *testing.T) {
	if pidf := os.Getenv("CLITEST_DEADLINE_PIDF"); pidf != "" {
		r := Cmd{}.Run("sh", "-c", `sleep 60 & echo $! >"$1"; wait`, "sh", pidf)
		if r.RC != 124 || !r.Timeout {
			t.Fatalf("바깥 마감 전에 시간 넘김이어야: %+v", r)
		}
		return
	}
	pidf := filepath.Join(t.TempDir(), "pid")
	t0 := time.Now()
	r := Cmd{Env: []string{"CLITEST_DEADLINE_PIDF=" + pidf}, Combined: true, Max: 2 * time.Minute}.
		Run(os.Args[0], "-test.run=^TestRunWithinTestDeadline$", "-test.timeout=8s", "-test.count=1")
	b, _ := os.ReadFile(pidf)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 && proc.Alive(pid) {
		proc.StopGroup(pid, time.Second)
		t.Errorf("바깥 마감 뒤 손자(%d)가 남았다", pid)
	}
	if r.RC != 0 || pid <= 0 {
		t.Fatalf("안쪽 시험이 마감 전에 끝나야(종료 0): rc %d pid %d %v\n%s", r.RC, pid, time.Since(t0), r.Out)
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
