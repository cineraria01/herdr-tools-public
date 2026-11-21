//go:build !windows

package main

// Go 전환 감사(2026-11-21) 지적 시험 — 고치기 전 코드에서 실패한다. 프로세스를 끝내는 길(die·os.Exit·신호)은 이 시험 바이너리를
// 하위 프로세스로 다시 띄워 본다(TestMain).

import (
	"errors"
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

// TestMain: 상주 병합기가 띄우는 일꾼(이 바이너리 --round …)과 하위 프로세스 사례(HC_MERGER_TEST_HELPER)
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--round" {
		if os.Getenv("HC_MERGER_TEST_ROUND") == "kill" {
			syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		gSelf, gEvdir = selfPath(), evdir()
		roundMain(os.Args[2:])
		return
	}
	if h := os.Getenv("HC_MERGER_TEST_HELPER"); h != "" {
		gSelf, gEvdir = selfPath(), evdir()
		helper(h)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helper(h string) {
	switch h {
	case "resident":
		residentMain(strings.Fields(os.Getenv("HC_MERGER_TEST_ARGS")))
	case "round":
		roundMain(strings.Fields(os.Getenv("HC_MERGER_TEST_ARGS")))
	case "term": // F3: 상주·일꾼이 TERM을 받으면 기다리던 자식 무리(TERM을 무시하는 배경 포함)를 끄고 143
		d := os.Getenv("HC_MERGER_TEST_DIR")
		onSignals(time.Second)
		proc.Wait(exec.Command("sh", "-c", "trap '' TERM; sleep 30 & echo $! >"+d+"/bg.tmp; mv "+d+"/bg.tmp "+d+"/bg; wait"), "")
	}
}

// sub: 하위 프로세스 사례 h를 env와 함께 돌려 종료 코드·출력
func sub(t *testing.T, env ...string) (int, string) {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), env...)
	b, err := c.CombinedOutput()
	return proc.Code(err), string(b)
}

// recorder: 사건만 적는 가짜 hc-lane(HC_LANE_SELF)
func recorder(t *testing.T) (self, log string) {
	d := t.TempDir()
	self, log = d+"/hc-lane", d+"/events"
	os.WriteFile(self, []byte("#!/bin/sh\nprintf '%s|' \"$@\" >>"+log+"\necho >>"+log+"\n"), 0o777)
	return
}

// F1: 지우기가 끊겨 .git이 먼저 지워진 후보·등록 전에 끊긴 반쯤 찬 후보도 거둔다(영원히 75가 아니게)
func TestWtcleanHalfRemoved(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	wt := m.mwr + "/c-ABCDEF"
	f.git(f.clone, "worktree", "add", "-q", "-B", "x", wt, "origin/main")
	os.Remove(wt + "/.git")
	half := m.mwr + "/c-GHIJKL"
	os.MkdirAll(half+"/sub", 0o777)
	os.WriteFile(half+"/sub/f", []byte("x"), 0o666)
	if !m.wtclean() {
		t.Fatal("반쯤 지워진 후보도 거둬야 한다")
	}
	for _, p := range []string{wt, half} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("남았다: %s", p)
		}
	}
	if l := f.git(f.clone, "worktree", "list", "--porcelain"); strings.Contains(l, "c-ABCDEF") {
		t.Fatalf("등록이 남았다: %s", l)
	}
	if !m.wtclean() {
		t.Fatal("다시 불러도 참")
	}
}

// E07: 돌려보낼 기록을 읽지 못하면(권한) 깨진 기록으로 지우지 않는다
func TestFretryUnreadableKept(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	p := m.mj + "/fail/1000000001-lane"
	os.WriteFile(p, []byte("refs/merge-queue/1000000001-lane\n"+strings.Repeat("a", 40)+"\nlane\n-\n-\n까닭\n"), 0o666)
	os.Chmod(p, 0)
	defer os.Chmod(p, 0o666)
	m.fretry()
	if _, err := os.Lstat(p); err != nil || !m.mer {
		t.Fatalf("읽기 실패면 기록을 두고 쉬어야: err=%v mer=%v", err, m.mer)
	}
}

// E12: 사건 파일을 읽지 못하면(같은 사건이 있는지 모름) 사건을 또 쓰지 않고 보낼 함을 둔다
func TestSendKeepsOutboxWhenEventsUnreadable(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	ev := gEvdir + "/events.tsv"
	os.WriteFile(ev, []byte("t\tbox\t-\tx\t착지\t-\t보낼 함 시험\n"), 0o666)
	os.Chmod(ev, 0o200)
	defer os.Chmod(ev, 0o666)
	ob := m.mj + "/out/1000000002-x"
	os.WriteFile(ob, []byte("착지\n-\n보낼 함 시험\nx\n\n\n\n\n\n\n\n"), 0o666)
	m.flush()
	if _, err := os.Stat(ob); err != nil {
		t.Fatal("보낼 함을 지웠다")
	}
	if b, _ := os.ReadFile(f.events); len(b) != 0 {
		t.Fatalf("모르는 채 사건을 또 썼다: %s", b)
	}
}

// E06: hc get이 0인데 응답을 해석하지 못하면, 또 그 기계를 모르면(잠깐 끊김) 다른 실행으로 보지 않고 다시 한다(.try)
func TestSendRetriesUnknownGet(t *testing.T) {
	for name, body := range map[string]string{"잘린 JSON": "echo '{\"state\": \"li'; exit 0", "machine_unknown": "echo '{\"error\":\"machine_unknown\"}'; exit 1"} {
		t.Run(name, func(t *testing.T) {
			f := newFx(t)
			m := f.merger()
			bin := t.TempDir()
			os.WriteFile(bin+"/hc", []byte("#!/bin/sh\ncase $1 in get) "+body+" ;; esac\nexit 0\n"), 0o777)
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			ob := m.mj + "/out/1000000003-y"
			os.WriteFile(ob, []byte("착지\n-\n알림 시험\ny\n-\nbox\nt9\n\n\n123\n-\n"), 0o666)
			m.send("1000000003-y")
			b, err := os.ReadFile(ob + ".try")
			if err != nil || strings.TrimSpace(string(b)) != "1" {
				t.Fatalf("다시 해야(.try=1): %q %v", b, err)
			}
			if _, err := os.Stat(ob); err != nil {
				t.Fatal("보낼 함을 지웠다")
			}
		})
	}
}

// F7: 제출 ID 표식을 못 읽으면(ls-remote 실패 — 모름) 일지를 지우지 않는다(「없음」으로 읽으면 일지를 치운다)
func TestRecoverUnknownKeepsJournal(t *testing.T) {
	f := newFx(t)
	q, sha, meta, _ := f.submit(1000000001, "lane", "보통")
	m := f.merger()
	B := f.git(f.clone, "rev-parse", "origin/main")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue-meta/*:refs/hc-merger/meta/*")
	os.WriteFile(m.mj+"/cur", []byte("lane "+B+" "+sha+"\n"+q+" "+sha+" lane "+meta+"\n"), 0o666)
	fakeGit(t, " ls-remote ")
	if m.recover() || !m.mer {
		t.Fatal("표식을 못 읽으면 쉬어야")
	}
	if _, err := os.Stat(m.mj + "/cur"); err != nil {
		t.Fatal("모르면 일지를 지우면 안 된다")
	}
}

// X1: 제출 기록의 기계·터미널 값이 옵션으로 읽히지 않게
func TestAddrOK(t *testing.T) {
	for s, want := range map[string]bool{"t9": true, "mini-m4": true, "-x": false, "": false, "a b": false, "a/b": false, "--json": false} {
		if addrOK(s) != want {
			t.Errorf("addrOK(%q) != %v", s, want)
		}
	}
}

// E04·F5: 잠금 이름 두 꼴 — bash 판 tr의 UTF-8 로캘(글자마다)과 C 로캘(바이트마다)
func TestCkeyBothLocales(t *testing.T) {
	p := "/tmp/공백 있는/m"
	for name, fn := range map[string]func(string) string{"C": ckeyBytes, "en_US.UTF-8": ckey} {
		c := exec.Command("tr", "-c", "A-Za-z0-9_-", "_")
		c.Env = append(os.Environ(), "LC_ALL="+name)
		c.Stdin = strings.NewReader(p)
		b, err := c.Output()
		if err != nil {
			t.Fatal(err)
		}
		if got := fn(p); got != string(b) {
			t.Errorf("%s 로캘: Go %q, bash %q", name, got, b)
		}
	}
}

// E05·F9: 알림·사건 명령은 상한 안에 — 자식이 끝나도 손자가 출력 관을 쥐면 기다리지 않고 그 무리를 거둔다
func TestBoundedRunLimits(t *testing.T) {
	d := t.TempDir()
	t0 := time.Now()
	_, rc, _ := boundedRun(time.Second, "sh", nil, "-c", "sleep 6 & echo $! >"+d+"/bg; exit 0")
	if el := time.Since(t0); el > 4*time.Second || rc != 142 {
		t.Fatalf("관을 쥔 손자: rc %d %v", rc, el)
	}
	b, _ := os.ReadFile(d + "/bg")
	if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 1 {
		time.Sleep(300 * time.Millisecond)
		if syscall.Kill(pid, 0) == nil {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("손자가 남았다")
		}
	}
	t0 = time.Now()
	if _, rc, _ := boundedRun(500*time.Millisecond, "sleep", nil, "5"); rc != 142 || time.Since(t0) > 4*time.Second {
		t.Fatalf("상한: rc %d", rc)
	}
}

// E11·G1: 준비 훅에 상한 — 넘으면 무리째 거두고 75(큐를 둔다)
func TestSetupHookLimit(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	m.mwt = t.TempDir()
	m.lgf = m.mwt + "/log"
	os.MkdirAll(m.mwt+"/.claude", 0o777)
	os.WriteFile(m.mwt+"/.claude/herdr-setup.sh", []byte("sleep 30 & echo $! >"+m.mwt+"/bg\nwait\n"), 0o666)
	t.Setenv("HERDR_TIMEOUT_SETUP", "1")
	t0 := time.Now()
	if rc := m.runCandidate(); rc != 75 || time.Since(t0) > 15*time.Second {
		t.Fatalf("준비 훅 상한: rc %d %v", rc, time.Since(t0))
	}
	b, _ := os.ReadFile(m.mwt + "/bg")
	if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 1 && syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("준비 훅 무리가 남았다")
	}
}

// M01(병합기 몫): 착지가 시간 넘김(124 — 모름)으로 끝나면 영구 실패가 아니라 일시 실패
func TestTrans124(t *testing.T) {
	m := &M{lr: "124", lgf: filepath.Join(t.TempDir(), "없음")}
	if !m.trans() {
		t.Fatal("124는 일시 실패여야")
	}
}

// F11: 빈 HC_LANE_FETCH_WAITS도 bash처럼 기본(1 2 4)
func TestFetchWaitsEmptyIsDefault(t *testing.T) {
	f := newFx(t)
	_ = f
	bin := t.TempDir()
	cnt := bin + "/n"
	os.WriteFile(bin+"/git", []byte("#!/bin/sh\necho x >>"+cnt+"\necho \"error: cannot lock ref 'refs/x'\" >&2\nexit 1\n"), 0o777)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HC_LANE_FETCH_WAITS", "")
	start := time.Now()
	fetchr(true, "-q", "origin")
	b, _ := os.ReadFile(cnt)
	if n := strings.Count(string(b), "x"); n != 4 || time.Since(start) < 6*time.Second {
		t.Fatalf("기본 1 2 4초 쉬며 네 번: %d번 %v", n, time.Since(start))
	}
}

// E10: 명시한 --tick이 정수가 아니면 bash 판처럼 멈춘다(종료 1)
func TestTickInvalidExits1(t *testing.T) {
	f := newFx(t)
	rc, out := sub(t, "HC_MERGER_TEST_HELPER=resident", "HC_MERGER_TEST_ARGS=--tick abc --once --repo "+f.clone)
	if rc != 1 || !strings.Contains(out, "--tick은 초(정수)") {
		t.Fatalf("rc %d: %s", rc, out)
	}
}

// F6: 로컬 git 시간 넘김(124)은 모름 — 병합기를 멈추지(die) 않고 그 바퀴만 쉰다(75)
func TestLocalGitTimeoutRests(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	self, log := recorder(t)
	real, _ := exec.LookPath("git")
	bin := t.TempDir()
	os.WriteFile(bin+"/git", []byte("#!/bin/sh\ncase \" $* \" in *\" status \"*) sleep 30 ;; esac\nexec "+real+" \"$@\"\n"), 0o777)
	args := strings.Join([]string{strconv.Itoa(os.Getpid()), f.clone, "main", m.mj, m.mwr, f.dir, "box", "3"}, " ")
	rc, out := sub(t, "HC_MERGER_TEST_HELPER=round", "HC_MERGER_TEST_ARGS="+args, "HC_LANE_SELF="+self,
		"PATH="+bin+":"+os.Getenv("PATH"), "HERDR_TIMEOUT_GIT=1", "HERDR_PROC_DIR="+t.TempDir())
	if rc != 75 {
		t.Fatalf("시간 넘김이면 75: rc %d\n%s", rc, out)
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "막힘") {
		t.Fatalf("멈춤 사건을 내면 안 된다: %s", b)
	}
}

// B25: --version에 릴리스 커밋(-ldflags -X …version.Commit)이 보인다
func TestVersionShowsCommit(t *testing.T) {
	bin := t.TempDir() + "/hc-merger"
	b, err := exec.Command("go", "build", "-ldflags", "-X herdr-tools/internal/version.Commit=abc123def456", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("빌드: %v %s", err, b)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || !strings.Contains(string(out), "abc123def456") {
		t.Fatalf("--version에 커밋: %q %v", out, err)
	}
}

// F14: 일꾼 인자가 틀려도(상주와 판이 갈림) 막힘 사건을 남긴다
func TestRoundBadArgsReports(t *testing.T) {
	self, log := recorder(t)
	rc, _ := sub(t, "HC_MERGER_TEST_HELPER=round", "HC_MERGER_TEST_ARGS=x", "HC_LANE_SELF="+self)
	b, _ := os.ReadFile(log)
	if rc != 1 || !strings.Contains(string(b), "report|막힘|") {
		t.Fatalf("rc %d 사건 %q", rc, b)
	}
}

// G2: 일꾼이 신호로 죽으면 상주가 막힘 사건을 내고 멈춘다
func TestResidentReportsKilledWorker(t *testing.T) {
	f := newFx(t)
	self, log := recorder(t)
	rc, out := sub(t, "HC_MERGER_TEST_HELPER=resident", "HC_MERGER_TEST_ARGS=--once --repo "+f.clone, "HC_MERGER_TEST_ROUND=kill",
		"HC_LANE_SELF="+self, "HERDR_PROC_DIR="+t.TempDir())
	b, _ := os.ReadFile(log)
	if rc != 1 || !strings.Contains(string(b), "report|막힘|") {
		t.Fatalf("rc %d 사건 %q\n%s", rc, b, out)
	}
}

// F4: 둘 다 죽은 앞 실행의 착지(자식 기록 — pid + 시작 시각)가 아직 돌면 그 후보를 건드리지 않고 쉰다
func TestRoundWaitsForLiveChildRecord(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	c := exec.Command("sleep", "30")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { c.Process.Kill(); c.Wait() }()
	st, err := proc.StartTime(c.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(m.mj+"/child", []byte(strconv.Itoa(c.Process.Pid)+"\t"+st+"\n"), 0o666)
	keep := m.mwr + "/c-KEEPME"
	os.MkdirAll(keep, 0o777)
	os.WriteFile(keep+"/f", []byte("x"), 0o666)
	self, _ := recorder(t)
	args := strings.Join([]string{strconv.Itoa(os.Getpid()), f.clone, "main", m.mj, m.mwr, f.dir, "box", "3"}, " ")
	rc, out := sub(t, "HC_MERGER_TEST_HELPER=round", "HC_MERGER_TEST_ARGS="+args, "HC_LANE_SELF="+self, "HERDR_PROC_DIR="+t.TempDir())
	if rc != 75 || !strings.Contains(out, "앞 실행이 후보 작업 트리를 쓰고 있다") {
		t.Fatalf("rc %d\n%s", rc, out)
	}
	if _, err := os.Stat(keep + "/f"); err != nil {
		t.Fatal("도는 착지의 후보를 지웠다")
	}
}

// F3: TERM을 받으면 기다리던 자식 무리(TERM을 무시하는 배경까지)를 끄고 143
func TestTermStopsWaitedGroup(t *testing.T) {
	d := t.TempDir()
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "HC_MERGER_TEST_HELPER=term", "HC_MERGER_TEST_DIR="+d)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	bg := 0
	for i := 0; i < 250 && bg == 0; i++ {
		if b, err := os.ReadFile(d + "/bg"); err == nil {
			bg, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if bg <= 1 {
		c.Process.Kill()
		t.Fatal("배경이 뜨지 않았다")
	}
	c.Process.Signal(syscall.SIGTERM)
	err := c.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || proc.Code(err) != 143 {
		t.Fatalf("143이어야: %v", err)
	}
	for i := 0; i < 100 && syscall.Kill(bg, 0) == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(bg, 0) == nil {
		syscall.Kill(bg, syscall.SIGKILL)
		t.Fatal("기다리던 무리의 배경이 남았다")
	}
}
