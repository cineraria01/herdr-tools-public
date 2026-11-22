//go:build !windows

package main

// 검증 본문의 계약(옛 tests/herdr-verify.test.sh를 옮김, 2026-11-22 go-shell-remove): 싼 검사 먼저·--cheap-only·HERDR_VERIFY_ALL·
// HERDR_VERIFY_JOBS, 고아 검사(같은 무리·떼어 띄움·cwd만·/.claude/ 플러그인 뺌·검증 전부터 돌던 것 그대로), 끊기면 무리째 끔.
// 옛 병렬 실행기·단계(--stages)·시간 넘김·셸 판 건너뜀 시험은 그 기능과 함께 걷었다(go test가 꾸러미를 동시에 돌리고 -timeout이 상한).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 도우미 프로세스: VERIFY_HELPER=detach면 자기를 새 세션(무리 밖)·VERIFY_DIR cwd·VERIFY_ARG 인자로 띄워 pid를 VERIFY_PID에 적고 끝
// (떼어 띄운 것은 부모가 1이 된다). VERIFY_HELPER=sleep이면 5분 잔다
func TestMain(m *testing.M) {
	switch os.Getenv("VERIFY_HELPER") {
	case "sleep":
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "detach":
		c := exec.Command(os.Args[0], strings.Fields(os.Getenv("VERIFY_ARG"))...)
		c.Dir, c.Env = os.Getenv("VERIFY_DIR"), append(os.Environ(), "VERIFY_HELPER=sleep")
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			os.Exit(1)
		}
		if os.WriteFile(os.Getenv("VERIFY_PID"), []byte(strconv.Itoa(c.Process.Pid)), 0o600) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func tmp(t *testing.T) string {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func alive(p int) bool { return syscall.Kill(p, 0) == nil }

func gone(p int) bool {
	for i := 0; i < 20; i++ {
		if !alive(p) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// readPid: 시험이 남긴 pid. 시험 끝에 끈다
func readPid(t *testing.T, f string) int {
	t.Helper()
	var p int
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
			p, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if p <= 0 {
		t.Fatalf("pid 파일 %s 없음", f)
	}
	t.Cleanup(func() { _ = syscall.Kill(p, syscall.SIGKILL) })
	return p
}

// step: runStep을 돌리고 (종료 코드, 출력)
func step(t *testing.T, root string, argv ...string) (int, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rc := runStep(root, argv, make(chan os.Signal), f)
	b, _ := os.ReadFile(f.Name())
	return rc, string(b)
}

// detach: 떼어 띄운 도우미의 pid(cwd dir, 인자 arg)
func detach(t *testing.T, dir, arg string) []string {
	pf := filepath.Join(t.TempDir(), "pid")
	return []string{"env", "VERIFY_HELPER=detach", "VERIFY_DIR=" + dir, "VERIFY_PID=" + pf, "VERIFY_ARG=" + arg, os.Args[0], pf}
}

func TestOrphans(t *testing.T) {
	R := tmp(t)
	os.MkdirAll(R+"/sub", 0o755)
	P := t.TempDir()

	if rc, out := step(t, R, "sh", "-c", "exit 0"); rc != 0 || strings.Contains(out, "고아") {
		t.Errorf("깨끗한 명령: 0이어야 rc=%d\n%s", rc, out)
	}
	if rc, _ := step(t, R, "sh", "-c", "exit 3"); rc != 3 {
		t.Errorf("종료 코드는 그대로: %d", rc)
	}
	// 같은 무리에 배경 sleep
	rc, out := step(t, R, "sh", "-c", "sleep 300 </dev/null >/dev/null 2>&1 & echo $! >'"+P+"/grp'")
	p := readPid(t, P+"/grp")
	if rc != 1 || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)) || !strings.Contains(out, "실패 고아를 남김") || !gone(p) {
		t.Errorf("배경 sleep 남김: 실패·목록·끔이어야 rc=%d alive=%v\n%s", rc, alive(p), out)
	}
	// 무리를 벗어나 부모가 1이 된, cwd로만 이 폴더인 것: 잡음, 줄에 cwd
	a := detach(t, R+"/sub", "")
	rc, out = step(t, R, a[:len(a)-1]...)
	p = readPid(t, a[len(a)-1])
	if rc != 1 || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)+" (cwd "+R+"/sub)") || !gone(p) {
		t.Errorf("cwd로만 이 폴더인 떼어 띄운 것: 실패·끔·cwd 줄이어야 rc=%d\n%s", rc, out)
	}
	// 명령 줄에 이 폴더 경로가 든 떼어 띄운 것(cwd는 밖): 잡음
	a = detach(t, P, R+"/x")
	rc, out = step(t, R, a[:len(a)-1]...)
	p = readPid(t, a[len(a)-1])
	if rc != 1 || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)) || !gone(p) {
		t.Errorf("명령 줄에 이 폴더 경로: 실패·끔이어야 rc=%d\n%s", rc, out)
	}
	// cwd가 이 폴더이고 명령 줄에 /.claude/(Claude 세션의 플러그인·훅): 고아가 아니다
	a = detach(t, R, P+"/home/.claude/plugins/x")
	rc, out = step(t, R, a[:len(a)-1]...)
	p = readPid(t, a[len(a)-1])
	if rc != 0 || strings.Contains(out, "고아 pid") || !alive(p) {
		t.Errorf("cwd가 이 폴더인 /.claude/ 플러그인: 통과·그대로 삶이어야 rc=%d\n%s", rc, out)
	}
	// /.claude/가 들어도 이 폴더 경로가 같이 들면(저장소 .claude/ 스크립트): 잡음
	a = detach(t, R, R+"/.claude/x.sh")
	rc, out = step(t, R, a[:len(a)-1]...)
	p = readPid(t, a[len(a)-1])
	if rc != 1 || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)) || !gone(p) {
		t.Errorf("저장소 .claude/ 아래: 실패·끔이어야 rc=%d\n%s", rc, out)
	}
	// 검증 전부터 돌던 이 폴더 프로세스(부모 1)는 건드리지 않는다
	a = detach(t, R+"/sub", "")
	if out, err := exec.Command(a[0], a[1:len(a)-1]...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	pre := readPid(t, a[len(a)-1])
	if rc, out := step(t, R, "sh", "-c", "exit 0"); rc != 0 || !alive(pre) {
		t.Errorf("검증 전부터 돌던 프로세스: 통과·그대로 삶이어야 rc=%d\n%s", rc, out)
	}
}

// 끊기면(TERM) 도는 무리를 배경 자식까지 끄고 143
func TestStop(t *testing.T) {
	R, P := tmp(t), t.TempDir()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stop := make(chan os.Signal, 1)
	rcc := make(chan int, 1)
	go func() {
		rcc <- runStep(R, []string{"sh", "-c", "sleep 300 </dev/null >/dev/null 2>&1 & echo $! >'" + P + "/bg'; wait"}, stop, f)
	}()
	p := readPid(t, P+"/bg")
	stop <- syscall.SIGTERM
	select {
	case rc := <-rcc:
		if rc != 143 || !gone(p) {
			t.Errorf("끊김: 143·배경 sleep 끔이어야 rc=%d alive=%v", rc, alive(p))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("끊김 뒤 돌아오지 않음")
	}
}

// 싼 검사 먼저: 가짜 go로 부른 차례를 본다
func TestPlan(t *testing.T) {
	R, B := tmp(t), t.TempDir()
	log := B + "/log"
	fake := "#!/bin/sh\necho \"$*\" >>'" + log + "'\ncase \"$*\" in *' ./tests/repo/') exit \"${FAKE_CHEAP_RC:-0}\" ;; esac\nexit 0\n"
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "")
	run := func(args ...string) (int, string, string) {
		t.Helper()
		os.Remove(log)
		f, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rc := verify(R, args, make(chan os.Signal), f)
		o, _ := os.ReadFile(f.Name())
		l, _ := os.ReadFile(log)
		return rc, string(o), string(l)
	}
	rc, out, calls := run()
	want := "vet ./...\ntest -count=1 ./tests/repo/\n" +
		"test -race -count=1 -timeout 40m -run ^TestCotenantGuard$ ./claude/hooks/tests/\n" +
		"test -race -count=1 -timeout 40m -skip ^TestCotenantGuard$ ./...\n"
	if rc != 0 || calls != want || !strings.Contains(out, "검증 시간 ") {
		t.Errorf("통과: 싼 검사 → cotenant 홀로 → 나머지 차례여야 rc=%d\n%s\n%s", rc, calls, out)
	}
	t.Setenv("FAKE_CHEAP_RC", "1")
	rc, out, calls = run()
	if rc != 1 || strings.Contains(calls, "-race") || !strings.Contains(out, "실패 싼 검사 — 무거운 시험을 건너뛰고 끝") {
		t.Errorf("싼 검사 실패: 무거운 시험 없이 1이어야 rc=%d\n%s\n%s", rc, calls, out)
	}
	t.Setenv("HERDR_VERIFY_ALL", "1")
	rc, _, calls = run()
	if rc != 1 || strings.Count(calls, "-race") != 2 || strings.Count(calls, "./tests/repo/") != 1 {
		t.Errorf("HERDR_VERIFY_ALL=1: 무거운 시험까지 돌고 1, 싼 검사는 한 번이어야 rc=%d\n%s", rc, calls)
	}
	rc, _, calls = run("--cheap-only")
	if rc != 1 || strings.Contains(calls, "-race") {
		t.Errorf("--cheap-only: 싼 검사 실패면 1, 무거운 시험 안 부름 rc=%d\n%s", rc, calls)
	}
	t.Setenv("FAKE_CHEAP_RC", "0")
	t.Setenv("HERDR_VERIFY_ALL", "")
	rc, _, calls = run("--cheap-only")
	if rc != 0 || calls != "vet ./...\ntest -count=1 ./tests/repo/\n" {
		t.Errorf("--cheap-only: 싼 검사만 0이어야 rc=%d\n%s", rc, calls)
	}
	t.Setenv("HERDR_VERIFY_JOBS", "3")
	if _, _, calls = run(); strings.Count(calls, "-timeout 40m -p 3 ") != 2 {
		t.Errorf("HERDR_VERIFY_JOBS=3: 무거운 시험에 -p 3이어야\n%s", calls)
	}
	t.Setenv("HERDR_VERIFY_JOBS", "x")
	if rc, out, calls = run(); rc != 0 || strings.Contains(calls, "-p ") || !strings.Contains(out, "무시한다") {
		t.Errorf("틀린 HERDR_VERIFY_JOBS는 알리고 무시 rc=%d\n%s\n%s", rc, calls, out)
	}
	if rc, _, _ = run("--bogus"); rc != 2 {
		t.Errorf("모르는 인자는 2: %d", rc)
	}
}
