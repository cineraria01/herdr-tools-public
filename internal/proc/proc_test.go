//go:build !windows

package proc

// 프로세스 무리 시험(병합기 요구 1): 자기가 띄운 무리만 끄고, pid 재사용은 시작 시각으로 가린다.
// 꾸러미 전역(stopping)을 바꾸는 시험은 이 시험 바이너리를 하위 프로세스로 다시 띄워 돌린다(T01 — -count·-shuffle에서도 다른 시험을 막지 않게).

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain: HERDR_PROC_TEST_HELPER=<사례>면 이 바이너리가 그 사례의 도구 흉내를 하고 끝난다
func TestMain(m *testing.M) {
	if h := os.Getenv("HERDR_PROC_TEST_HELPER"); h != "" {
		os.Exit(helper(h, os.Getenv("HERDR_PROC_TEST_DIR")))
	}
	os.Exit(m.Run())
}

// until: ok가 참이 될 때까지 20ms마다(상한 30초 — 부하 때 프로세스 띄우기·거두기가 몇 초씩 늦어도 넘기지 않게, 2026-11-22 proc-race-flaky).
// 참이면 바로 돌아오니 평소 시험 시간은 그대로다
func until(ok func() bool) bool {
	for end := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if ok() {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

// waitFile: 파일에 적힌 pid(다른 프로세스가 쓴다)
func waitFile(t *testing.T, f string) int {
	t.Helper()
	p := 0
	if !until(func() bool {
		b, err := os.ReadFile(f)
		if err == nil && strings.HasSuffix(string(b), "\n") {
			p, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		return p > 1
	}) {
		t.Fatal(f + " 가 쓰이지 않았다")
	}
	return p
}

// startGroup: sh 스크립트를 새 무리로 띄우고, 그 안의 배경 sleep pid를 돌려준다
func startGroup(t *testing.T, script string) (*exec.Cmd, int) {
	t.Helper()
	f := t.TempDir() + "/bg"
	c := exec.Command("sh", "-c", "sleep 30 & echo $! >"+f+"; "+script)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	pid := waitFile(t, f)
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	return c, pid
}

// gone: 좀비는 우리 자식이 아니면 init이 거둔다 — kill 0이 ESRCH가 될 때까지(옛 상한 2초는 TestRunHeldPipeNoRace의 4초 손자도 못 기다렸다)
func gone(pid int) bool { return until(func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH }) }

// helperCmd: 사례 h를 하위 프로세스로(기록 폴더·작업 폴더는 d)
func helperCmd(h, d string) *exec.Cmd {
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HERDR_PROC_TEST_HELPER="+h, "HERDR_PROC_TEST_DIR="+d, "HERDR_PROC_DIR="+d+"/rec")
	return c
}

func TestCode(t *testing.T) {
	if rc := Code(exec.Command("sh", "-c", "exit 3").Run()); rc != 3 {
		t.Fatalf("종료 3: %d", rc)
	}
	if rc := Code(exec.Command("sh", "-c", "kill -TERM $$").Run()); rc != 128+int(syscall.SIGTERM) {
		t.Fatalf("TERM은 143: %d", rc)
	}
	if rc := Code(exec.Command("/없는/명령").Run()); rc != 127 {
		t.Fatalf("못 띄우면 127: %d", rc)
	}
	if rc := Code(fmt.Errorf("감쌈: %w", &TimeoutError{Head: "git", D: time.Second})); rc != TimeoutCode {
		t.Fatalf("시간 넘김은 감싸도 TimeoutCode: %d", rc)
	}
}

func TestStopGroupKillsWholeGroup(t *testing.T) {
	c, bg := startGroup(t, "trap '' TERM; wait") // 우두머리는 TERM을 무시 — 유예 뒤 KILL
	done := make(chan struct{})
	go func() { c.Wait(); close(done) }()
	StopGroup(c.Process.Pid, 300*time.Millisecond)
	<-done
	if !gone(bg) {
		t.Fatal("무리 안 배경 sleep이 남았다")
	}
}

// E01: 자식은 기록이 다 쓰인 뒤에야 시작한다 — 첫 줄에서 기록을 봐도 늘 있다(옛 판은 띄운 뒤에 써 자식이 먼저 돌 수 있었다)
func TestWaitRecordBeforeChildRuns(t *testing.T) {
	for i := 0; i < 20; i++ {
		rec := t.TempDir() + "/child"
		if rc := Wait(exec.Command("sh", "-c", "test -s "+rec+" && exit 4; exit 5"), rec); rc != 4 {
			t.Fatalf("%d번째: 자식이 기록보다 먼저 돌았다(종료 %d)", i, rc)
		}
		if _, err := os.Stat(rec); err == nil {
			t.Fatal("끝나면 기록을 지워야 한다")
		}
	}
}

// E01: 부모가 「go」 전에 죽으면(관이 닫힘) 자식은 명령 없이 끝난다
func TestGateWithoutGoRunsNothing(t *testing.T) {
	d := t.TempDir()
	c := exec.Command("touch", d+"/ran")
	w, err := gate(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	c.ExtraFiles[len(c.ExtraFiles)-1].Close()
	w.Close() // 부모가 죽은 것과 같다
	if rc := Code(c.Wait()); rc != 125 {
		t.Fatalf("명령 없이 125로 끝나야: %d", rc)
	}
	if _, err := os.Stat(d + "/ran"); err == nil {
		t.Fatal("「go」 없이 명령이 돌았다")
	}
}

// M11: 상한에 걸리면 자식이 TERM을 받고 0으로 끝내도 TimeoutCode(모름)
func TestWaitMaxTimeoutIsUnknown(t *testing.T) {
	t0 := time.Now()
	rc := WaitMax(exec.Command("sh", "-c", "trap 'exit 0' TERM; sleep 60 & wait"), "", 500*time.Millisecond)
	if el := time.Since(t0); el > 30*time.Second || rc != TimeoutCode { // 상한이 안 먹으면 60초 — 30초는 부하 여유
		t.Fatalf("상한에 걸리면 %d 이어야: rc=%d %v", TimeoutCode, rc, el)
	}
}

func TestChildBusy(t *testing.T) {
	rec := t.TempDir() + "/child"
	// 살아 있고 시작 시각이 같으면 도는 중
	c, bg := startGroup(t, "wait")
	pid := c.Process.Pid
	st, err := StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\t"+st+"\n"), 0o666)
	if !ChildBusy(rec, time.Second) {
		t.Fatal("살아 있는 착지를 도는 중으로 봐야 한다")
	}
	// 시작 시각이 다르면 pid 재사용 — 기록만 지우고 건드리지 않는다
	os.WriteFile(rec, []byte(strconv.Itoa(pid)+"\tMon Jan  1 00:00:00 2001\n"), 0o666)
	if ChildBusy(rec, time.Second) || !Alive(pid) || !Alive(bg) {
		t.Fatal("다시 쓰인 pid의 프로세스를 건드리면 안 된다")
	}
	if _, err := os.Stat(rec); err == nil {
		t.Fatal("기록을 지워야 한다")
	}
	syscall.Kill(-pid, syscall.SIGKILL)
	c.Wait()

	// 우두머리는 끝났는데 무리에 배경 프로세스가 남았으면(죽은 병합기의 착지가 남긴 것) 거둔다
	c2, bg2 := startGroup(t, "exit 0")
	pid2 := c2.Process.Pid
	st2, _ := StartTime(pid2)
	c2.Wait()
	os.WriteFile(rec, []byte(strconv.Itoa(pid2)+"\t"+st2+"\n"), 0o666)
	if ChildBusy(rec, 5*time.Second) { // 유예는 넉넉히(부하 때 TERM 받은 sleep이 거둬지기까지) — 거둬지면 바로 돌아온다
		t.Fatal("끝난 착지는 도는 중이 아니다")
	}
	if !gone(bg2) {
		t.Fatal("끝난 착지 무리에 남은 프로세스를 거둬야 한다")
	}
}

// E02·F2: ps가 실패하면(시작 시각 모름) 「pid 재사용」으로 읽지 않는다 — 도는 것으로 치고 기록을 둔다
func TestChildBusyPsUnknown(t *testing.T) {
	d := t.TempDir()
	rec := d + "/child"
	c, _ := startGroup(t, "wait")
	defer func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL); c.Wait() }()
	st, err := StartTime(c.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(rec, []byte(strconv.Itoa(c.Process.Pid)+"\t"+st+"\n"), 0o666)
	os.WriteFile(d+"/ps", []byte("#!/bin/sh\nexit 1\n"), 0o755)
	old := psPath
	psPath = d + "/ps"
	defer func() { psPath = old }()
	if _, err := StartTime(c.Process.Pid); err == nil {
		t.Fatal("ps가 실패하면 StartTime은 오류여야")
	}
	if !ChildBusy(rec, time.Second) {
		t.Fatal("ps를 못 읽으면 도는 것으로 쳐야 한다")
	}
	if _, err := os.Stat(rec); err != nil {
		t.Fatal("모르면 기록을 지우면 안 된다")
	}
}

// T01: StopCurrent는 지금 기다리는 무리를 끄고, 그 뒤 Wait는 돌아오지 않는다 — 하위 프로세스에서
func TestWaitRecordAndStopCurrent(t *testing.T) {
	d := t.TempDir()
	c := helperCmd("stopcurrent", d)
	var out strings.Builder
	c.Stdout, c.Stderr = &out, &out
	if err := c.Run(); err != nil {
		t.Fatalf("흉내 도구 실패: %v\n%s", err, out.String())
	}
	lead, bg := waitFile(t, d+"/lead"), waitFile(t, d+"/bg")
	if !gone(bg) || !gone(lead) {
		t.Fatal("무리 전체가 끝나야 한다")
	}
	if _, err := os.Stat(d + "/new"); err == nil {
		t.Fatal("StopCurrent 뒤에는 새 자식을 띄우지 않는다")
	}
}

// M03: 신호로 멈출 때 상한 걸린 Run의 결과가 부른 쪽으로 돌아가지 않는다 — TERM을 무시하는 무리 안 자식까지 거둔 뒤 끝난다
// M02·바탕 ①: 따로 신호 처리가 없는 도구가 TERM을 받으면, 상한 걸린 git 무리(따로 무리)를 거둔 뒤 143으로 끝난다
func TestSignalStopsBoundedGroup(t *testing.T) {
	// started: Run을 거치지 않고 Start로 올린 무리(publish pack|unpack)도 같다
	for _, h := range []string{"ownsignal", "nosignal", "started"} {
		t.Run(h, func(t *testing.T) {
			d := t.TempDir()
			fakeStuckGit(t, d)
			c := helperCmd(h, d)
			c.Env = append(c.Env, "PATH="+d+":"+os.Getenv("PATH"))
			if err := c.Start(); err != nil {
				t.Fatal(err)
			}
			gc := waitFile(t, d+"/grandchild")
			c.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- c.Wait() }()
			var err error
			select {
			case err = <-done:
			case <-time.After(20 * time.Second):
				c.Process.Kill()
				t.Fatal("TERM 뒤 끝나지 않았다")
			}
			if rc := Code(err); rc != 143 {
				t.Fatalf("TERM이면 143: %d", rc)
			}
			if _, e := os.Stat(d + "/returned"); e == nil {
				t.Fatal("멈추는 중 Run의 결과가 부른 쪽으로 돌아갔다")
			}
			if !gone(gc) {
				syscall.Kill(gc, syscall.SIGKILL)
				t.Fatal("TERM을 무시하는 무리 안 손자가 남았다")
			}
		})
	}
}

// 바탕 ①: 부모가 SIGKILL로 죽어 남은 무리는 다음 실행(Reap)이 소유를 확인하고 거둔다. 부모가 살아 있는 기록은 건드리지 않는다
func TestReapOrphanAfterParentKill(t *testing.T) {
	d := t.TempDir()
	fakeStuckGit(t, d)
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	for _, h := range []string{"killed", "startkilled"} { // startkilled: Start로 올린 무리도 고아 기록이 남는다
		os.Remove(d + "/grandchild")
		c := helperCmd(h, d)
		c.Env = append(c.Env, "PATH="+d+":"+os.Getenv("PATH"))
		err := c.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
			t.Fatalf("%s: 흉내 도구가 SIGKILL로 끝나지 않았다: %v", h, err)
		}
		gc := waitFile(t, d+"/grandchild")
		defer syscall.Kill(gc, syscall.SIGKILL)
		if !Alive(gc) {
			t.Fatalf("%s: 부모가 죽은 뒤 무리가 남아 있어야 시험이 된다", h)
		}
		if n := Reap(); n != 1 {
			t.Fatalf("%s: 고아 무리 하나를 거둬야: %d", h, n)
		}
		if !gone(gc) {
			t.Fatalf("%s: 고아 무리의 손자가 남았다", h)
		}
		if es, _ := os.ReadDir(d + "/rec"); len(es) != 0 {
			t.Fatalf("%s: 거둔 기록은 지워야: %v", h, es)
		}
	}
	// 살아 있는 남의 부모 기록·다시 쓰인 번호는 건드리지 않는다
	keep, bg := startGroup(t, "wait")
	defer func() { syscall.Kill(-keep.Process.Pid, syscall.SIGKILL); keep.Wait() }()
	pst, _ := StartTime(os.Getppid())
	kst, _ := StartTime(keep.Process.Pid)
	os.WriteFile(d+"/rec/"+strconv.Itoa(keep.Process.Pid), []byte(fmt.Sprintf("%d\t%s\t%d\t%s\n", keep.Process.Pid, kst, os.Getppid(), pst)), 0o600)
	if Reap() != 0 || !Alive(bg) {
		t.Fatal("부모가 살아 있는 무리를 거뒀다")
	}
	os.WriteFile(d+"/rec/"+strconv.Itoa(keep.Process.Pid), []byte(fmt.Sprintf("%d\tMon Jan  1 00:00:00 2001\t1999999\tx\n", keep.Process.Pid)), 0o600)
	if Reap() != 0 || !Alive(bg) {
		t.Fatal("시작 시각이 다른(다시 쓰인) 무리를 거뒀다")
	}
}

// fakeStuckGit: d에 멈추는 git — TERM을 무시하는 손자를 남기고 그 pid를 d/grandchild에
func fakeStuckGit(t *testing.T, d string) {
	t.Helper()
	sc := "#!/bin/sh\n(trap '' TERM; sleep 60) & echo $! >" + d + "/grandchild.tmp; mv " + d + "/grandchild.tmp " + d + "/grandchild\nwait\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
}

// helper: 하위 프로세스 사례
func helper(h, d string) int {
	switch h {
	case "stopcurrent":
		done := make(chan int, 2)
		go func() {
			done <- Wait(exec.Command("sh", "-c", "echo $$ >"+d+"/lead; sleep 30 & echo $! >"+d+"/bg; wait"), "")
		}()
		until(func() bool { b, _ := os.ReadFile(d + "/bg"); return strings.HasSuffix(string(b), "\n") })
		StopCurrent(time.Second)
		go func() { done <- Wait(exec.Command("touch", d+"/new"), "") }()
		select {
		case rc := <-done:
			fmt.Println("StopCurrent 뒤 Wait가 돌아왔다", rc)
			return 1
		case <-time.After(300 * time.Millisecond):
		}
		return 0
	case "ownsignal", "nosignal", "killed", "started", "startkilled":
		Grace = 300 * time.Millisecond
		os.Setenv("HERDR_TIMEOUT_GIT", "60")
		if h == "ownsignal" { // 병합기·hc-fill 꼴 — 도구가 신호를 받아 StopCurrent 뒤 128+신호
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, syscall.SIGTERM)
			go func() { <-ch; StopCurrent(Grace); os.Exit(143) }()
		}
		if h == "killed" || h == "startkilled" {
			// 부모는 고아 기록이 쓰인 뒤에 죽는다 — Run은 손자가 뜬 뒤에도 ps(시작 시각)를 기다려 기록하니, 손자 파일만 보고 죽으면
			// 부하 때 기록 전에 죽어 Reap이 0이었다(2026-11-22 proc-race-flaky). 느린 ps로 그 순서를 늘 벌려 둔다
			os.WriteFile(d+"/ps", []byte("#!/bin/sh\nsleep 0.5\nexec /bin/ps \"$@\"\n"), 0o755)
			psPath = d + "/ps"
			go func() {
				until(func() bool {
					b, _ := os.ReadFile(d + "/grandchild")
					es, _ := os.ReadDir(d + "/rec")
					rec := false
					for _, e := range es {
						_, err := strconv.Atoi(e.Name())
						rec = rec || err == nil
					}
					return strings.HasSuffix(string(b), "\n") && rec
				})
				syscall.Kill(os.Getpid(), syscall.SIGKILL)
			}()
		}
		var err error
		if h == "started" || h == "startkilled" { // publish의 pack|unpack 꼴 — 바깥에서 띄우고 기다리는 무리
			c := exec.Command("git", "status")
			Group(c)
			var release func()
			if release, err = Start(c); err == nil {
				err = c.Wait()
				release()
			}
		} else {
			err = Run(exec.Command("git", "status"))
		}
		os.WriteFile(d+"/returned", []byte(fmt.Sprint(err)), 0o644)
		return 0
	}
	return 2
}
