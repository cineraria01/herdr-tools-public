package workermac

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

// 셸 안전 인용: 어떤 값이든 sh가 다시 읽으면 그대로
func TestQuoteRoundTrip(t *testing.T) {
	for _, v := range []string{"", "a b", "it's", `"$HOME" $(id) ; rm -rf x`, "x*", "'", "a\nb", `\'`} {
		out, err := exec.Command("/bin/sh", "-c", "printf %s "+Quote(v)).Output()
		if err != nil || string(out) != v {
			t.Errorf("Quote(%q) → sh가 %q (%v)", v, out, err)
		}
	}
}

// 상한 시간: 시간이 지나면 무리째(손자까지) 거두고 142, 그때까지 받은 출력은 남는다
func TestOutputTimeoutKillsGroup(t *testing.T) {
	pidf := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	out, rc := Output(context.Background(), 1, nil, "/bin/sh", "-c", "echo 앞; sleep 30 & echo $! >"+pidf+"; wait")
	if rc != TimeoutCode || out != "앞\n" {
		t.Fatalf("rc %d out %q", rc, out)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("너무 오래 걸림 %v", d)
	}
	b, err := os.ReadFile(pidf)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("손자 %d 가 남았다", pid)
	}
}

func TestOutputCodes(t *testing.T) {
	if _, rc := Output(context.Background(), 5, nil, "/bin/sh", "-c", "exit 3"); rc != 3 {
		t.Errorf("exit 3 → %d", rc)
	}
	if _, rc := Output(context.Background(), 5, nil, "/bin/sh", "-c", "kill -TERM $$"); rc != 143 {
		t.Errorf("TERM → %d", rc)
	}
	if _, rc := Output(context.Background(), 5, nil, "no-such-command-xyz"); rc != 127 {
		t.Errorf("없는 명령 → %d", rc)
	}
}

// 자식이 끝났는데 배경 손자가 출력 관을 쥐어도 오래 기다리지 않는다
func TestOutputHeldPipe(t *testing.T) {
	pidf := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	out, rc := Output(context.Background(), 0, nil, "/bin/sh", "-c", "echo 끝; sleep 30 & echo $! >"+pidf+"; exit 0")
	if b, err := os.ReadFile(pidf); err == nil { // 시험이 배경 프로세스를 남기지 않게
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	if rc != 0 || out != "끝\n" {
		t.Fatalf("rc %d out %q", rc, out)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("너무 오래 걸림 %v", d)
	}
}

func TestFieldTrimLookIn(t *testing.T) {
	if got := Field("user=a\nts4=1.2.3.4\nuser=b\n", "user="); got != "a\nb" {
		t.Errorf("Field %q", got)
	}
	if Field("x=1", "y=") != "" || Trim("a\n\n") != "a" {
		t.Error("Field·Trim")
	}
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "tool"), []byte("#!/bin/sh\n"), 0o755)
	os.WriteFile(filepath.Join(d, "plain"), []byte(""), 0o644)
	if LookIn("/nonexistent:"+d, "tool") != d+"/tool" || LookIn(d, "plain") != "" || LookIn(d, "/abs/x") != "/abs/x" {
		t.Error("LookIn")
	}
}

// fakeConfig: herdr-tools-config 가짜 — get은 키마다 값·종료 코드, file은 이름이 있으면 경로
func fakeConfig(t *testing.T, body string) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "herdr-tools-config"), []byte("#!/bin/bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}

func TestConfig(t *testing.T) {
	fakeConfig(t, `case "$1 $2" in
  "get a.x") echo "값 하나"; echo ;;
  "get a.y") echo "설정 없음" >&2; exit 3 ;;
  "get a.bad") echo "같은 키 두 번 a.bad" >&2; exit 2 ;;
  "file have") echo /cfg/have ;;
  *) exit 3 ;;
esac`)
	c := &Config{Ctx: context.Background()}
	if v := c.Get("a.x"); v != "값 하나" {
		t.Errorf("값 %q", v)
	}
	if v := c.Get("a.y", "기본"); v != "기본" || len(c.Miss) != 0 {
		t.Errorf("기본값 %q %v", v, c.Miss)
	}
	if v := c.Get("a.y"); v != "" || len(c.Miss) != 1 || c.Miss[0] != "a.y" {
		t.Errorf("없음 %q %v", v, c.Miss)
	}
	if p, ok := c.File("have"); !ok || p != "/cfg/have" {
		t.Errorf("file %q %v", p, ok)
	}
	if _, ok := c.File("none"); ok {
		t.Error("없는 file")
	}
	if c.Get("a.bad"); c.Err == nil || !strings.Contains(c.Err.Error(), "같은 키 두 번") {
		t.Fatalf("오류 %v", c.Err)
	}
	if v := c.Get("a.x"); v != "" {
		t.Errorf("오류 뒤 Get은 빈 값이어야: %q", v)
	}
}

func TestConfigDir(t *testing.T) {
	t.Setenv("HERDR_TOOLS_CONFIG", "")
	t.Setenv("HOME", "/h")
	if ConfigDir() != "/h/.config/herdr-tools" {
		t.Error(ConfigDir())
	}
	t.Setenv("HERDR_TOOLS_CONFIG", "/c")
	if ConfigDir() != "/c" {
		t.Error(ConfigDir())
	}
}
