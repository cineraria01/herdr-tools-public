//go:build !windows

package proc

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — proc 지적의 고치기 전 실패 시험

import (
	"bytes"
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

// jobGit: d에 git — 실제 일을 시작하면 자기 pid를 d/ran에 적고, TERM을 무시하는 sleep을 기다린다
func jobGit(t *testing.T, d string) {
	t.Helper()
	sc := "#!/bin/sh\necho $$ >" + d + "/ran.tmp; mv " + d + "/ran.tmp " + d + "/ran\ntrap '' TERM\nsleep 60 &\nwait\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
}

func digitRecords(dir string) []string {
	es, _ := os.ReadDir(dir)
	var r []string
	for _, e := range es {
		if _, err := strconv.Atoi(e.Name()); err == nil {
			r = append(r, e.Name())
		}
	}
	return r
}

// R-02·R-H05·R-T04·R-34·ops R-04: 부모가 자식을 띄운 뒤 기록 전(started)·기록 뒤 명령 허용 전(recorded)에 SIGKILL로 죽으면
// 자식은 실제 일을 시작하지 않는다. 느린 ps로 띄우기와 기록 사이를 벌린 채 죽어도(slowps) 다음 실행의 Reap이 남은 일을 다 거둔다
// (옛 판: 자식이 기록보다 먼저 돌아 기록 0·회수 0·자식 생존). Run·Start 둘 다
func TestGofix2KillBeforeRecord(t *testing.T) {
	for _, mode := range []string{"run", "start"} {
		for _, stage := range []string{"started", "recorded", "slowps"} {
			t.Run(mode+"-"+stage, func(t *testing.T) {
				d := t.TempDir()
				jobGit(t, d)
				t.Setenv("HERDR_PROC_DIR", d+"/rec")
				c := helperCmd("prekill", d)
				c.Env = append(c.Env, "PATH="+d+":"+os.Getenv("PATH"), "HERDR_PROC_TEST_STAGE="+stage, "HERDR_PROC_TEST_MODE="+mode)
				var out strings.Builder
				c.Stdout, c.Stderr = &out, &out
				err := c.Run()
				var ee *exec.ExitError
				if !errors.As(err, &ee) || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
					t.Fatalf("흉내 도구가 SIGKILL로 끝나지 않았다: %v\n%s", err, out.String())
				}
				if stage != "slowps" {
					child := waitFile(t, d+"/child")
					if !gone(child) {
						syscall.Kill(-child, syscall.SIGKILL)
						t.Fatal("명령 허용 전에 부모가 죽었는데 자식이 남았다")
					}
					if _, err := os.Stat(d + "/ran"); err == nil {
						t.Fatal("명령 허용 전에 부모가 죽었는데 실제 일이 시작됐다")
					}
				}
				// 거둔 뒤 기록이 없어질 때까지(막 끝난 gate가 좀비인 사이에는 모름으로 둔다 — 다음 실행이 다시 본다)
				until(func() bool { Reap(); return len(digitRecords(d+"/rec")) == 0 })
				if b, err := os.ReadFile(d + "/ran"); err == nil {
					p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
					if p > 1 && !gone(p) {
						syscall.Kill(-p, syscall.SIGKILL)
						t.Fatalf("시작된 일(%d)을 다음 실행이 거두지 못했다(기록 %v)", p, digitRecords(d+"/rec"))
					}
				}
				if r := digitRecords(d + "/rec"); len(r) != 0 {
					t.Fatalf("끝난 무리의 기록이 남았다: %v", r)
				}
			})
		}
	}
}

// R-03·R-H06: Reap은 숫자 pid 이름의 게시 완료 기록만 본다 — 다른 실행이 쓰는 중인 임시 파일을 지우지 않는다
func TestGofix2ReapLeavesTempFiles(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d)
	tmp := d + "/.12345.tmp3k9x"
	if err := os.WriteFile(tmp, []byte("12345\tMon Jan  1"), 0o600); err != nil {
		t.Fatal(err)
	}
	Reap()
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal("다른 실행이 쓰는 중인 임시 기록을 지웠다")
	}
}

// R-04·R-C02·R-07·ops R-05: 앞 실행(부모가 죽음)의 디스크 기록은 우두머리가 살아 있고 기록한 시작 시각과 같을 때만 그것이다 —
// 시작 시각이 비었거나 우두머리가 이미 끝난 무리는 거두지 않고 기록을 둔다(옛 판: 둘 다 자기 것으로 쳐 거뒀다)
func TestGofix2DiskRecordOwnership(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d)
	put := func(pg int, st string) {
		if err := os.WriteFile(fmt.Sprintf("%s/%d", d, pg), []byte(fmt.Sprintf("%d\t%s\t1999999\tx\n", pg, st)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keep, bg := startGroup(t, "wait")
	defer func() { syscall.Kill(-keep.Process.Pid, syscall.SIGKILL); keep.Wait() }()
	put(keep.Process.Pid, "")
	c2, bg2 := startGroup(t, "exit 0")
	st2, _ := StartTime(c2.Process.Pid)
	c2.Wait()
	defer syscall.Kill(-c2.Process.Pid, syscall.SIGKILL)
	put(c2.Process.Pid, st2+"x") // 우두머리는 끝남 — 시각이 무엇이든 입증할 수 없다
	if n := Reap(); n != 0 {
		t.Fatalf("입증할 수 없는 무리를 거뒀다: %d", n)
	}
	if !Alive(bg) || !Alive(keep.Process.Pid) {
		t.Fatal("시작 시각이 빈 기록의 살아 있는 무리를 거뒀다")
	}
	if !Alive(bg2) {
		t.Fatal("우두머리가 끝난 무리(번호 재사용과 못 가름)를 거뒀다")
	}
	if r := digitRecords(d); len(r) != 2 {
		t.Fatalf("모르는 기록은 그대로 둬야: %v", r)
	}
	// 2바퀴 P1: 우두머리가 끝난 무리도 남아 있으면 센다(모름 = 미룸)
	if n := Orphans(); n != 2 {
		t.Fatalf("무리가 남은 모르는 기록은 둘: %d", n)
	}
	// 이 실행이 띄워 bounded에 올린 무리(live)는 시작 시각 없이도·우두머리가 끝나도 무리가 남은 동안 그것이다. 올리지 않은(내린) 번호는 아니다
	if ownGroup(c2.Process.Pid, "", true) == groupMine {
		t.Fatal("bounded에 없는 번호를 숫자만으로 그것이라 했다")
	}
	curMu.Lock()
	bounded[c2.Process.Pid], bounded[keep.Process.Pid] = "", ""
	curMu.Unlock()
	defer func() {
		curMu.Lock()
		delete(bounded, c2.Process.Pid)
		delete(bounded, keep.Process.Pid)
		curMu.Unlock()
	}()
	if ownGroup(c2.Process.Pid, "", true) != groupMine || ownGroup(keep.Process.Pid, "", true) != groupMine {
		t.Fatal("이 실행이 띄운 무리는 그것이다")
	}
}

// R-H05: 고아 기록을 못 쓰면 명령을 시작하지 않고 모름(ErrTimeout)을 돌려준다(옛 판: 경고만 하고 띄웠다)
func TestGofix2RecordFailRunsNothing(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(d+"/file", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PROC_DIR", d+"/file/rec") // 파일 아래라 폴더를 못 만든다
	jobGit(t, d)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "30")
	err := Run(exec.Command("git", "status"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("기록을 못 쓰면 모름(ErrTimeout)이어야: %v", err)
	}
	if b, err := os.ReadFile(d + "/ran"); err == nil {
		p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		syscall.Kill(-p, syscall.SIGKILL)
		t.Fatal("기록 없이 명령을 시작했다")
	}
}

// R-H09: 자식이 0이 아닌 코드로 끝났는데 출력 관을 쥔(무리 밖) 손자가 남으면 종료 코드·반쪽 출력으로 결과를 정하지 않는다(모름)
// (옛 판: exec.Cmd.Wait가 종료 오류를 앞세워 ErrWaitDelay를 감춰 rc=1·부분 출력을 돌려줬다)
func TestGofix2NonzeroHeldPipe(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	sc := "#!/bin/sh\nperl -e 'setpgrp(0,0); print STDERR \"partial-merge-output\\n\"; sleep 3' &\necho $! >" + d + "/helper\nexit 1\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "30")
	old := Grace
	Grace = 300 * time.Millisecond
	defer func() { Grace = old }()
	out, err := Combined(exec.Command("git", "merge-tree"))
	if !errors.Is(err, ErrTimeout) || out != nil {
		t.Fatalf("관이 안 닫히면 모름·빈 출력: err=%v out=%q", err, out)
	}
	if h := waitFile(t, d+"/helper"); h > 1 {
		gone(h)
	}
}

// R-C01·R-T02: 시간 넘김 때 ps가 실패해도 이 실행이 띄운 무리는 거두고, 돌아온 뒤에는 부른 쪽 버퍼에 아무도 쓰지 않는다
// (옛 판: 소유 모름으로 git을 남기고, 출력 복사 고루틴이 돌아온 뒤에도 버퍼에 썼다 — -race 경합)
func TestGofix2TimeoutNoLateWrite(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	if err := os.WriteFile(d+"/ps", []byte("#!/bin/sh\n[ -e "+d+"/psfail ] && exit 1\nexec /bin/ps \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := psPath
	psPath = d + "/ps"
	defer func() { psPath = old }()
	sc := "#!/bin/sh\necho $$ >" + d + "/lead\nperl -e 'setpgrp(0,0); for (1..40) { print STDERR \"x\" x 100, \"\\n\"; select(undef,undef,undef,0.1) }' &\necho $! >" + d + "/helper\nsleep 0.5; touch " + d + "/psfail\nsleep 60\n"
	if err := os.WriteFile(d+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "2")
	var b bytes.Buffer
	c := exec.Command("git", "status")
	c.Stderr = &b
	err := Run(c)
	n := b.Len()
	time.Sleep(600 * time.Millisecond)
	if b.Len() != n {
		t.Fatalf("돌아온 뒤에 버퍼에 썼다: %d → %d", n, b.Len())
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("시간 넘김이어야: %v", err)
	}
	if lead := waitFile(t, d+"/lead"); !gone(lead) {
		syscall.Kill(-lead, syscall.SIGKILL)
		t.Fatal("ps가 실패한 뒤 이 실행이 띄운 git을 거두지 않았다")
	}
	if h := waitFile(t, d+"/helper"); h > 1 {
		gone(h)
	}
}

// R-C05: 상한은 띄우기 전에 정한다 — 시작 시각 조회가 상한을 다 쓰면 명령을 시작하지 않고 모름(옛 판: 시계가 준비 뒤에 걸려 상한 1초에
// 1.8초 뒤 0·완료)
func TestGofix2LaunchBudget(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	self(time.Time{}) // 이 프로세스의 시작 시각은 미리(느린 ps 전에)
	if err := os.WriteFile(d+"/ps", []byte("#!/bin/sh\nsleep 3\nexec /bin/ps \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := psPath
	psPath = d + "/ps"
	defer func() { psPath = old }()
	jobGit(t, d)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_TIMEOUT_GIT", "1")
	t0 := time.Now()
	err := Run(exec.Command("git", "status"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("준비가 상한을 넘기면 모름: %v (%v)", err, time.Since(t0))
	}
	if el := time.Since(t0); el > 2900*time.Millisecond { // 느린 ps 3초를 다 기다리면 안 된다
		t.Fatalf("상한 1초인데 %v", el)
	}
	if b, err := os.ReadFile(d + "/ran"); err == nil {
		p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		syscall.Kill(-p, syscall.SIGKILL)
		t.Fatal("마감 뒤에 명령을 시작했다")
	}
}

// ops R-05: Stop은 이 실행이 올린 무리만 끈다 — 올리지 않은 번호에는 신호를 보내지 않는다
func TestGofix2StopOnlyOwned(t *testing.T) {
	keep, bg := startGroup(t, "wait")
	defer func() { syscall.Kill(-keep.Process.Pid, syscall.SIGKILL); keep.Wait() }()
	Stop(keep.Process.Pid, 300*time.Millisecond)
	if !Alive(bg) {
		t.Fatal("올리지 않은 무리를 껐다")
	}
}

// R-H04: TakeSignals를 부른 도구는 제 처리기 하나만 — proc 처리기가 짧은 유예로 먼저 끝내지 않아, 도구 처리기가 TERM을 무시하는
// 무리 안 손자까지 거두고(긴 유예) 표식을 남긴 뒤 끝난다
func TestGofix2TakeSignals(t *testing.T) {
	d := t.TempDir()
	fakeStuckGit(t, d)
	c := helperCmd("takesignals", d)
	c.Env = append(c.Env, "PATH="+d+":"+os.Getenv("PATH"))
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	gc := waitFile(t, d+"/grandchild")
	c.Process.Signal(syscall.SIGTERM)
	err := c.Wait()
	if rc := Code(err); rc != 143 {
		t.Fatalf("도구 처리기의 143이어야: %d", rc)
	}
	if _, e := os.Stat(d + "/handled"); e != nil {
		t.Fatal("도구 처리기가 끝까지 돌지 않았다(proc 처리기가 먼저 끝냄)")
	}
	if !gone(gc) {
		syscall.Kill(gc, syscall.SIGKILL)
		t.Fatal("손자가 남았다")
	}
}

// reauditHelper: 하위 프로세스 사례(재감사)
func reauditHelper(h, d string) int {
	switch h {
	case "prekill":
		Grace = 300 * time.Millisecond
		os.Setenv("HERDR_TIMEOUT_GIT", "60")
		stage := os.Getenv("HERDR_PROC_TEST_STAGE")
		die := func() { syscall.Kill(os.Getpid(), syscall.SIGKILL); time.Sleep(time.Minute) }
		if stage == "slowps" {
			os.WriteFile(d+"/ps", []byte("#!/bin/sh\nsleep 0.5\nexec /bin/ps \"$@\"\n"), 0o755)
			psPath = d + "/ps"
			go func() {
				until(func() bool {
					_, err := os.Stat(d + "/ran")
					return err == nil || len(digitRecords(d+"/rec")) > 0
				})
				die()
			}()
		} else {
			testHook = func(s string) {
				if s != stage {
					return
				}
				curMu.Lock()
				for pid := range bounded {
					os.WriteFile(d+"/child", []byte(fmt.Sprintf("%d\n", pid)), 0o644)
				}
				curMu.Unlock()
				die()
			}
		}
		c := exec.Command("git", "status")
		if os.Getenv("HERDR_PROC_TEST_MODE") == "start" {
			Group(c)
			if release, err := Start(c); err == nil {
				c.Wait()
				release()
			}
		} else {
			Run(c)
		}
		return 0
	case "takesignals":
		Grace = 300 * time.Millisecond // proc 처리기의 유예(남아 있으면 이것으로 먼저 끝낸다)
		os.Setenv("HERDR_TIMEOUT_GIT", "60")
		TakeSignals()
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		go func() {
			<-ch
			StopCurrent(3 * time.Second) // 도구의 긴 유예 — TERM을 무시하는 손자는 KILL까지
			os.WriteFile(d+"/handled", []byte("1\n"), 0o644)
			os.Exit(143)
		}()
		Run(exec.Command("git", "status"))
		time.Sleep(time.Minute)
		return 0
	}
	return gofix3Helper(h, d)
}
