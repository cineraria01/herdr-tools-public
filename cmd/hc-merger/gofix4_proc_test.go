//go:build !windows

package main

// gofix4-proc(Go 전환 재감사 바퀴 3): 남은 착지 무리·고아 무리로 쉬는 바퀴도 세어 막힘 사건(R-3M01) · 띄우지 못한 착지는 반송이 아니라
// 쉼(R-3M02) · 착지 상한(R-3M07)

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// leftGroup: 우두머리는 끝났고 배경 sleep만 남은 무리(번호·우두머리 시작 시각·배경 pid)
func leftGroup(t *testing.T) (int, string, int) {
	t.Helper()
	d := t.TempDir()
	c := exec.Command("sh", "-c", "sleep 60 & echo $! >\"$0/bg.tmp\"; mv \"$0/bg.tmp\" \"$0/bg\"; exit 0", d)
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
	t.Cleanup(func() { syscall.Kill(-pg, syscall.SIGKILL) })
	return pg, st, bg
}

// rounds: 일꾼(--round)을 하위 프로세스로 n번 — 종료 코드들
func rounds(t *testing.T, f *fx, m *M, n int) []int {
	t.Helper()
	args := fmt.Sprintf("%d %s %s %s %s %s %s %d", os.Getpid(), m.top, m.base, m.mj, m.mwr, m.ml, m.mme, m.batch)
	var rcs []int
	for i := 0; i < n; i++ {
		rc, out := sub(t, "HC_MERGER_TEST_HELPER=round", "HC_MERGER_TEST_ARGS="+args, "HC_LANE_SELF="+gSelf)
		if rc != 75 {
			t.Fatalf("%d번째 바퀴: 남은 무리가 있으면 75: %d\n%s", i+1, rc, out)
		}
		rcs = append(rcs, rc)
	}
	return rcs
}

func blocked(f *fx) []string {
	b, _ := os.ReadFile(f.events)
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "report|막힘|") {
			o = append(o, l)
		}
	}
	return o
}

// R-3M01(가): 죽은 일꾼의 착지는 끝났는데 검증이 남긴 배경 프로세스만 그 무리에 남음(child 기록) — 바퀴는 쉬되 세어 deferAlarm 바퀴째
// 막힘 사건 하나(무리 번호·기록 경로). 옛 판: ChildBusy 참으로 round 앞에서 75만, 사건 0
func TestGofix4LeftoverChildAlarms(t *testing.T) {
	f := newFx(t)
	t.Setenv("HERDR_PROC_DIR", f.dir+"/rec")
	m := f.merger()
	pg, st, bg := leftGroup(t)
	if err := os.WriteFile(m.mj+"/child", []byte(fmt.Sprintf("%d\t%s\n", pg, st)), 0o666); err != nil {
		t.Fatal(err)
	}
	rounds(t, f, m, deferAlarm-1)
	if b := blocked(f); len(b) != 0 {
		t.Fatalf("임계 전에는 사건 없음: %v", b)
	}
	rounds(t, f, m, 2)
	b := blocked(f)
	if len(b) != 1 || !strings.Contains(b[0], strconv.Itoa(pg)) || !strings.Contains(b[0], m.mj+"/child") {
		t.Fatalf("남은 착지 무리로 %d바퀴 쉬면 막힘 사건 하나(무리 번호·기록 경로): %v", deferAlarm, b)
	}
	if syscall.Kill(bg, 0) != nil {
		t.Fatal("소유를 입증할 수 없는 무리에 신호를 보냈다")
	}
}

// R-3M01(나): 고아 기록(proc)의 무리가 남음 — 같은 식으로 세어 막힘 사건. 옛 판: orphansDefer로 75만, 사건 0
func TestGofix4LeftoverOrphanAlarms(t *testing.T) {
	f := newFx(t)
	rec := f.dir + "/rec"
	t.Setenv("HERDR_PROC_DIR", rec)
	m := f.merger()
	pg, st, _ := leftGroup(t)
	os.MkdirAll(rec, 0o700)
	p := fmt.Sprintf("%s/%d", rec, pg)
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d\t%s\t1999999\tx\n", pg, st)), 0o600); err != nil {
		t.Fatal(err)
	}
	rounds(t, f, m, deferAlarm)
	b := blocked(f)
	if len(b) != 1 || !strings.Contains(b[0], strconv.Itoa(pg)) || !strings.Contains(b[0], p) {
		t.Fatalf("남은 고아 무리로 %d바퀴 쉬면 막힘 사건 하나: %v", deferAlarm, b)
	}
}

// R-3M02: 착지 자식을 띄우지 못함(자식 기록을 못 씀 — 명령은 안 돎)은 착지 실패가 아니다 — 반송 없이 큐에 두고 쉰다.
// 옛 판: 127 = 착지 실패로 refs/merge-failed/…에 반송
func TestGofix4LandRefusedNotBounced(t *testing.T) {
	f, landlog := batchFx(t)
	t.Setenv("HERDR_PROC_DIR", f.dir+"/rec")
	q, _, _, _ := f.submit(1000000001, "lane", "보통")
	m := f.merger()
	if err := os.MkdirAll(m.mj+"/child", 0o777); err != nil { // 자식 기록 자리가 폴더라 못 쓴다
		t.Fatal(err)
	}
	m.round()
	if b, _ := os.ReadFile(landlog); len(b) != 0 {
		t.Fatalf("착지 코드가 돌았다: %s", b)
	}
	if !f.has(q) || f.has("refs/merge-failed/1000000001-lane") || f.onMain("lane.txt") {
		t.Fatal("띄우지 못한 착지는 큐에 그대로(반송 없음)여야")
	}
	if !m.mer || !strings.Contains(m.deferred, "띄우지 못했다") {
		t.Fatalf("이 바퀴는 쉼(같은 까닭으로 세게): mer=%v deferred=%q", m.mer, m.deferred)
	}
	if _, err := os.Stat(m.mj + "/cur"); err == nil {
		t.Fatal("명령이 안 돈 후보의 일지가 남았다")
	}
}

// R-3M02(추측 일꾼): 검증 명령을 띄우지 못함은 spec-fail(resFail)이 아니라 모름(resLost). 옛 판: 127로 resFail → 원격 실패 표식
func TestGofix4SpecNotRunLost(t *testing.T) {
	f := newFx(t)
	t.Setenv("HERDR_PROC_DIR", f.dir+"/rec")
	os.MkdirAll(f.clone+"/.claude", 0o777)
	if err := os.WriteFile(f.clone+"/"+".claude/herdr-verify.sh", []byte("touch \"$0.ran\"\n"), 0o777); err != nil {
		t.Fatal(err)
	}
	tree := f.git(f.clone, "rev-parse", "HEAD^{tree}")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir() // git만 있는 PATH — bash를 못 찾아 검증을 띄우지 못한다
	if err := os.Symlink(real, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if r := specRunVerify(f.clone, f.dir+"/spec.log", f.clone, "main", strings.Repeat("a", 40), "", tree, &specEvidence{}); r != resLost {
		lg, _ := os.ReadFile(f.dir + "/spec.log")
		t.Fatalf("띄우지 못한 검증은 resLost여야: %v\n%s", r, lg)
	}
	if _, err := os.Stat(f.clone + "/.claude/herdr-verify.sh.ran"); err == nil {
		t.Fatal("검증이 돌았다(시험 전제가 틀림)")
	}
}

// R-3M07: 착지가 끝나지 않으면 HERDR_TIMEOUT_LAND에서 무리째 거두고 124(모름 — 일시 실패, 큐에 그대로). 옛 판: 상한 없이 끝까지 기다림
func TestGofix4LandBounded(t *testing.T) {
	f, _ := batchFx(t)
	t.Setenv("HERDR_PROC_DIR", f.dir+"/rec")
	old, err := os.ReadFile(gSelf)
	if err != nil {
		t.Fatal(err)
	}
	hang := "#!/bin/sh\nif [ \"$1\" = land ]; then echo $$ >\"$LANDLOG.pid\"; exec sleep 40; fi\n"
	if err := os.WriteFile(gSelf, []byte(strings.Replace(string(old), "#!/bin/sh\n", hang, 1)), 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TIMEOUT_LAND", "1")
	q, _, _, _ := f.submit(1000000001, "lane", "보통")
	m := f.merger()
	t0 := time.Now()
	m.round()
	if el := time.Since(t0); el > 25*time.Second {
		t.Fatalf("착지 상한이 안 먹었다: %v", el)
	}
	if m.lr != strconv.Itoa(proc.TimeoutCode) || !f.has(q) || f.has("refs/merge-failed/1000000001-lane") {
		t.Fatalf("상한 넘긴 착지는 124·큐에 그대로: lr=%q", m.lr)
	}
	if pid := waitPid(t, os.Getenv("LANDLOG")+".pid"); !goneBy(pid, 10*time.Second) {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("상한 넘긴 착지 무리를 거두지 않았다")
	}
}
