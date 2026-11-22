package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/lockx"
)

// R-20: queue.lock을 남이 쥔 동안(바퀴가 잠금을 기다림) TERM — -race 바이너리에 경합 경고가 없고, 끝내기 훅(held.tsv 비움·사본 지움)은
// 한 번 돌고 128+15로 끝난다. 바퀴 진행은 사건(sweepTmp가 옛 임시 파일을 지움 = 훅을 단 뒤 잠금 기다림 직전)으로 본다
func TestRunSignalNoRace(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("short·windows")
	}
	d := t.TempDir()
	bin := filepath.Join(d, "hf")
	if out, err := exec.Command("go", "build", "-race", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("-race 빌드: %v\n%s", err, out)
	}
	cfg, data := filepath.Join(d, "cfg"), filepath.Join(d, "data")
	for _, x := range []string{cfg, data} {
		if err := os.MkdirAll(x, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cfg+"/hc-fill.tsv", []byte("local here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, busy, err := lockx.TryLock(data + "/queue.lock")
	if err != nil || busy {
		t.Fatalf("queue.lock: %v %v", busy, err)
	}
	defer f.Close()
	probe, sb := data+"/.probe.tmp", data+"/.run-brief.tmp"
	for _, p := range []string{probe, sb} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(probe, old, old); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bin, "run")
	c.Env = append(os.Environ(), "HOME="+d, "HERDR_TOOLS_CONFIG="+cfg, "HC_FILL_DATA="+data, "HC_FILL_SELF="+bin)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	for end := time.Now().Add(60 * time.Second); exists(probe); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			c.Process.Kill()
			<-done
			t.Fatalf("바퀴가 sweepTmp에 이르지 않았다\n%s\n%s", out.String(), errb.String())
		}
	}
	if err := c.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	var werr error
	select {
	case werr = <-done:
	case <-time.After(60 * time.Second):
		c.Process.Kill()
		<-done
		t.Fatal("TERM 뒤 끝나지 않았다")
	}
	var ee *exec.ExitError
	code := 0
	if errors.As(werr, &ee) {
		code = ee.ExitCode()
	}
	if strings.Contains(errb.String(), "DATA RACE") {
		t.Fatalf("경합 경고:\n%s", errb.String())
	}
	if code != 143 {
		t.Fatalf("종료 %d, 143이어야\n%s\n%s", code, out.String(), errb.String())
	}
	if b, err := os.ReadFile(data + "/held.tsv"); err != nil || len(b) != 0 {
		t.Fatalf("held.tsv를 비우지 않았다: %q %v", b, err)
	}
	if exists(sb) {
		t.Fatal("지시문 사본이 남았다")
	}
	if n := strings.Count(out.String(), "바퀴를 끝낸다"); n != 1 {
		t.Fatalf("끝내기 %d번:\n%s", n, out.String())
	}
}
