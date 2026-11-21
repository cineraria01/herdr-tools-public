//go:build !windows

package main

// K2·B11: herdr-app 한 번의 상한과 무리째 거두기, B25: --version에 릴리스 커밋

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 멈춘 herdr-app(손자 sleep까지 띄움)은 상한에 끊기고 「시간 넘김」이며, 그 무리(손자 포함)가 남지 않는다
func TestAppOutTimeoutReapsGroup(t *testing.T) {
	d := t.TempDir()
	pidf := filepath.Join(d, "pid")
	app := "#!/bin/sh\nsleep 30 &\necho $! >" + pidf + "\nwait\n"
	if err := os.WriteFile(filepath.Join(d, "herdr-app"), []byte(app), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":/usr/bin:/bin")
	defer func(l time.Duration) { appLimit = l }(appLimit)
	appLimit = time.Second
	start := time.Now()
	_, err := appOut(context.Background(), "ls")
	if err == nil || !strings.Contains(err.Error(), "시간 넘김") {
		t.Fatalf("시간 넘김 오류여야: %v", err)
	}
	if el := time.Since(start); el > 8*time.Second {
		t.Fatalf("너무 늦게 돌아옴: %v", el)
	}
	b, _ := os.ReadFile(pidf)
	pid, perr := strconv.Atoi(strings.TrimSpace(string(b)))
	if perr != nil {
		t.Fatalf("손자 pid 못 읽음: %q", b)
	}
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	for i := 0; i < 30 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("손자 sleep이 남았다 — 무리째 거두지 않음")
	}
}

// 신호(ctx 끝남)면 곧 「멈춤」으로 돌아온다
func TestAppOutCanceled(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "herdr-app"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":/usr/bin:/bin")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	if _, err := appOut(ctx, "ls"); err == nil || !strings.Contains(err.Error(), "멈춤") {
		t.Fatalf("멈춤 오류여야: %v", err)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("너무 늦게 돌아옴: %v", el)
	}
}

func TestVersionShowsCommit(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "herdr-balance")
	if b, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abc123def456", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, b)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "herdr-balance (Go abc123def456)" {
		t.Fatalf("--version: %q %v", out, err)
	}
}
