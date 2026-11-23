//go:build !windows

package main

// gofix3-proc(재감사 2바퀴 merger R-209): 추측 준비 훅·검증 명령은 고아 기록에 오른다 — 일꾼이 SIGKILL로 죽어도 다음 일꾼이 그 무리를
// 거두거나(소유 확인), 모르면 그 작업 트리를 쓰지 않고 75. 옛 판: WaitMax(c, "", …)가 기록 없이 띄워 훅이 남아 새 요청에 응답했다

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// TestGofix3SpecHelper: 하위 프로세스 사례(G3_SPEC_DIR가 있을 때만) — 추측 준비 훅이 뜨면 이 일꾼이 SIGKILL로 죽는다
func TestGofix3SpecHelper(t *testing.T) {
	d := os.Getenv("G3_SPEC_DIR")
	if d == "" {
		t.Skip("하위 프로세스 사례")
	}
	go func() {
		for {
			if b, err := os.ReadFile(d + "/hook"); err == nil && strings.HasSuffix(string(b), "\n") {
				syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	specRunVerify(d+"/wt", d+"/log", d, "main", strings.Repeat("a", 40), strings.Repeat("b", 40), &specEvidence{})
	time.Sleep(time.Minute)
}

func TestGofix3SpecSetupKill(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	if err := os.MkdirAll(d+"/wt/.claude", 0o755); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/bash\necho $$ >\"$G3_SPEC_DIR/hook.tmp\"; mv \"$G3_SPEC_DIR/hook.tmp\" \"$G3_SPEC_DIR/hook\"\nexec sleep 60\n"
	if err := os.WriteFile(d+"/wt/.claude/herdr-setup.sh", []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(os.Args[0], "-test.run=^TestGofix3SpecHelper$")
	c.Env = append(os.Environ(), "G3_SPEC_DIR="+d)
	out, err := c.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("일꾼 흉내가 SIGKILL로 끝나야: %v\n%s", err, out)
	}
	h := waitPid(t, d+"/hook")
	defer syscall.Kill(-h, syscall.SIGKILL)
	if syscall.Kill(h, 0) != nil {
		t.Fatal("일꾼이 죽은 뒤 훅이 살아 있어야 시험이 된다")
	}
	if orphansDefer("시험") || !goneBy(h, 10*time.Second) {
		t.Fatal("다음 일꾼이 죽은 일꾼의 준비 훅을 거두지 않았다(그 작업 트리를 함께 쓴다)")
	}
}

// 소유를 모르는 무리(우두머리는 끝났는데 무리가 남음)가 있으면 추측 검증은 작업 트리를 건드리기 전에 75
func TestGofix3SpecVerifyDefers(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	t.Chdir(d) // git 클론 밖 — 옛 판은 여기서 1(클론 아님)로 끝났다
	c := exec.Command("sh", "-c", "sleep 30 & echo $! >\"$0/bg\"; exit 0", d)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	pg := c.Process.Pid
	st, err := proc.StartTime(pg)
	if err != nil {
		t.Fatal(err)
	}
	bg := waitPid(t, d+"/bg")
	c.Wait()
	defer syscall.Kill(-pg, syscall.SIGKILL)
	os.MkdirAll(d+"/rec", 0o700)
	os.WriteFile(fmt.Sprintf("%s/rec/%d", d, pg), []byte(fmt.Sprintf("%d\t%s\t1999999\tx\n", pg, st)), 0o600)
	if rc := specVerifyMain([]string{strings.Repeat("a", 40), "main"}); rc != 75 {
		t.Fatalf("남은 무리의 소유를 모르면 75: %d", rc)
	}
	if syscall.Kill(bg, 0) != nil {
		t.Fatal("소유를 입증할 수 없는 무리에 신호를 보냈다")
	}
}
