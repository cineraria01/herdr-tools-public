package main

// cli-usage-version(2026-11-24): run의 종료 75(다른 run이 잠금을 쥠 — 정상 건너뜀)는 운영 기록에 이상(exit) 줄을 남기지 않는다.
// 종료 코드·표준 오류 글은 그대로

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/lockx"
)

func TestRunBusyNotInOpLog(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "hf")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	data, ops := d+"/data", d+"/ops.tsv"
	ex, err := os.ReadFile("../../config.example/hc-fill.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(d+"/conf", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d+"/conf/hc-fill.tsv", ex, 0o644); err != nil {
		t.Fatal(err)
	}
	f, busy, err := lockx.TryLock(data + "/run.lock")
	if err != nil || busy {
		t.Fatalf("잠금: %v %v", busy, err)
	}
	defer f.Close()
	c := exec.Command(bin, "run")
	c.Env = append(os.Environ(), "HOME="+d, "HERDR_TOOLS_CONFIG="+d+"/conf", "HC_FILL_DATA="+data, "HERDR_TOOLS_OPLOG="+ops)
	var se strings.Builder
	c.Stderr = &se
	err = c.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 75 || se.String() != "hc-fill: 다른 run이 돌고 있다, 이번 바퀴는 건너뜀\n" {
		t.Fatalf("종료 75·글 그대로여야: %v %q", err, se.String())
	}
	if b, err := os.ReadFile(ops); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("75는 이상이 아니다 — 기록이 없어야: %q %v", b, err)
	}
}
