//go:build !windows

package main

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — 병합기의 프로세스 지적(R-H04·R-T01·R-C04·R-T02) 고치기 전 실패 시험

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

func reauditProcHelper(h string) {
	d := os.Getenv("HC_MERGER_TEST_DIR")
	switch h {
	case "bounded": // R-C04: 알림 명령을 기다리는 중 일꾼이 SIGKILL로 죽는다
		go func() {
			for {
				if b, err := os.ReadFile(d + "/hcpid"); err == nil && strings.HasSuffix(string(b), "\n") {
					syscall.Kill(os.Getpid(), syscall.SIGKILL)
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
		boundedRun(30*time.Second, d+"/hc", nil, "send", "x", "y")
		time.Sleep(time.Minute)
	}
}

func waitPid(t *testing.T, f string) int {
	t.Helper()
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(f); err == nil && strings.HasSuffix(string(b), "\n") {
			if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 1 {
				return p
			}
		}
	}
	t.Fatal(f + " 가 쓰이지 않았다")
	return 0
}

func goneBy(pid int, d time.Duration) bool {
	for end := time.Now().Add(d); ; time.Sleep(20 * time.Millisecond) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

// R-H04·R-T01: 실제 상주를 띄워 일꾼의 git(ls-remote)이 TERM을 무시하는 손자를 띄운 뒤 상주에 TERM — 상주가 끝날 때는 그 손자도
// 끝나 있다(일꾼이 제 유예로 거둔 뒤 상주가 끝난다). 옛 판: proc 처리기(유예 2초)가 상주·일꾼 모두에서 함께 받아, 상주가 2초에
// 일꾼 무리를 KILL하고 끝나 일꾼이 거두던 손자가 남았다
func TestGofix2ResidentTermReaps(t *testing.T) {
	f := newFx(t)
	d := t.TempDir()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	sc := "#!/bin/sh\nif [ \"$1\" = ls-remote ]; then (trap '' TERM; exec sleep 60) & echo $! >" + d + "/gc.tmp; mv " + d + "/gc.tmp " + d + "/gc; wait; exit 1; fi\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	self, _ := recorder(t)
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HC_MERGER_TEST_HELPER=resident", "HC_MERGER_TEST_ARGS=--repo "+f.clone+" --tick 1",
		"HC_LANE_SELF="+self, "HERDR_PROC_DIR="+d+"/rec", "PATH="+d+":"+os.Getenv("PATH"))
	var out strings.Builder
	c.Stdout, c.Stderr = &out, &out
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	gc := waitPid(t, d+"/gc")
	defer syscall.Kill(gc, syscall.SIGKILL)
	c.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err = <-done:
	case <-time.After(40 * time.Second):
		c.Process.Kill()
		t.Fatalf("TERM 뒤 상주가 끝나지 않았다\n%s", out.String())
	}
	if rc := proc.Code(err); rc != 143 {
		t.Fatalf("상주는 143: %d\n%s", rc, out.String())
	}
	if !goneBy(gc, time.Second) { // 상주가 끝난 그때(좀비가 거둬질 짧은 틈만)
		t.Fatalf("상주가 끝났는데 일꾼 git의 손자 %d 가 남았다\n%s", gc, out.String())
	}
}

// R-C04: 알림 명령(boundedRun)도 고아 기록에 오른다 — 기다리던 일꾼이 SIGKILL로 죽으면 다음 실행(Reap)이 그 무리를 거둔다
// (옛 판: 따로 무리만 만들고 기록하지 않아 상한이 지나도 hc가 남았다)
func TestGofix2BoundedRunRecorded(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	if err := os.WriteFile(d+"/hc", []byte("#!/bin/sh\necho $$ >"+d+"/hcpid.tmp; mv "+d+"/hcpid.tmp "+d+"/hcpid\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc, out := sub(t, "HC_MERGER_TEST_HELPER=bounded", "HC_MERGER_TEST_DIR="+d)
	if rc != 128+int(syscall.SIGKILL) {
		t.Fatalf("일꾼 흉내가 SIGKILL로 끝나야: %d\n%s", rc, out)
	}
	hc := waitPid(t, d+"/hcpid")
	defer syscall.Kill(hc, syscall.SIGKILL)
	if n := proc.Reap(); n != 1 {
		t.Fatalf("죽은 일꾼의 알림 무리 하나를 거둬야: %d", n)
	}
	if !goneBy(hc, 10*time.Second) {
		t.Fatal("알림 명령이 남았다")
	}
}

// 작업 중 발견(R-H03과 같은 꼴): 추측 검증 결과를 남길 때 ls-remote가 종료 0에 잘린 SHA를 주면 「병합기가 버렸다」(0)가 아니라 모름(1)
func TestGofix2SpecWriteTrunc(t *testing.T) {
	f := newFx(t)
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf 'abcdef012345\trefs/x\n'; exit 0; }`)
	r := specRow{id: "s1", commit: strings.Repeat("a", 40), tree: strings.Repeat("b", 40)}
	if rc := specWrite(f.clone, r, resPass, specEvidence{}, t.TempDir()+"/log"); rc != 1 {
		t.Fatalf("잘린 ls-remote는 모름(1): %d", rc)
	}
}

// R-T02·R-C01: fetch가 시간 넘김(모름)이면 그때까지의 반쪽 오류 글로 잠금 경합을 판정해 다시 돌지 않는다 — 한 번만 부르고 124
func TestGofix2FetchrTimeoutNoRetry(t *testing.T) {
	newFx(t)
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	sc := "#!/bin/sh\necho x >>" + d + "/calls\necho \"error: cannot lock ref 'refs/x'\" >&2\nexec sleep 60\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_REMOTE", "1")
	if rc := fetchr(true, "-q", "origin"); rc != proc.TimeoutCode {
		t.Fatalf("시간 넘김은 124: %d", rc)
	}
	b, _ := os.ReadFile(d + "/calls")
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Fatalf("시간 넘김 뒤 다시 불렀다: %d번", n)
	}
}
