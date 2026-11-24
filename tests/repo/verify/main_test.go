//go:build !windows

package main

// 검증 본문의 계약(옛 tests/herdr-verify.test.sh를 옮김, 2026-11-22 go-shell-remove): 싼 검사 먼저·--cheap-only·HERDR_VERIFY_ALL·
// HERDR_VERIFY_JOBS·검증 잠금(HERDR_VERIFY_NO_LOCK), 고아 검사(같은 무리·떼어 띄움·cwd만·/.claude/ 플러그인 뺌·검증 전부터 돌던 것 그대로), 끊기면 무리째 끔.
// 옛 병렬 실행기·단계(--stages)·시간 넘김·셸 판 건너뜀 시험은 그 기능과 함께 걷었다(go test가 꾸러미를 동시에 돌리고 -timeout이 상한).

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	// 시험의 verify()는 진짜 hdiutil·diskutil을 부르지 않는다 — 메모리 디스크 시험(ramtmp_test.go)은 가짜 명령을 PATH 앞에 두고 켠다
	os.Setenv("HERDR_VERIFY_RAMTMP", "0")
	// 범위 지문은 끄고, 통과 기록은 진짜 사용자 캐시가 아니라 임시 폴더에 — 범위 시험(scope_run_test.go)이 켠다
	os.Setenv("HERDR_VERIFY_SCOPE", "off")
	cache, err := os.MkdirTemp("", "verify-scope-cache.")
	if err != nil {
		os.Exit(1)
	}
	scopeCache = func() string { return cache }
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
	fake := "#!/bin/sh\necho \"$*\" >>'" + log + "'\ncase \"$*\" in *' ./tests/repo/') exit \"${FAKE_CHEAP_RC:-0}\" ;;\n" +
		"  'list ./...') [ \"${FAKE_LIST_RC:-0}\" = 0 ] || exit 1; printf 'herdr-tools/a\\nherdr-tools/tests/merger\\nherdr-tools/b\\n' ;; esac\nexit 0\n"
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "")
	// 검증 잠금은 이 시험의 폴더에(이 시험은 검증 안에서도 돈다 — 바깥 검증이 쥔 진짜 잠금을 기다리지 않게)
	t.Setenv("HOME", B)
	t.Setenv("XDG_CACHE_HOME", B+"/.cache")
	t.Setenv("HERDR_VERIFY_NO_LOCK", "")
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
	want := "vet ./...\ntest -count=1 ./tests/repo/\nlist ./...\n" + // go list는 범위 판정(scope.go)이 cotenant 단계 앞에서 쓴다
		"test -race -count=1 -timeout 40m -run ^TestCotenantGuard$ ./claude/hooks/tests/\n"
	p := strconv.Itoa(max(1, runtime.NumCPU()/3))
	shardsWant := []string{ // 조각은 함께 돌아 차례가 없다
		"test -race -count=1 -timeout 40m -p " + p + " -skip ^TestCotenantGuard$ herdr-tools/a herdr-tools/b",
	}
	partRuns := func(pkg string) (runs []string) { // 그 꾸러미의 part(표 splits 차례)
		for _, sp := range splits {
			if sp.pkg == mod+pkg {
				for _, pt := range sp.parts {
					runs = append(runs, pt.run)
				}
			}
		}
		return runs
	}
	// 계약 시험 묶음은 표에 글자로 있어야 한다 — 묶음이 빠지거나 이름이 바뀌어 통째 한 조각으로 되돌면 실패(lows-polish 2026-11-24)
	for pkg, want := range map[string][]string{
		"tests/merger":   {"^TestMergerContractA$", "^TestMergerContractB$", "^TestMergerContractC$", "^TestMergerContractD$", "^TestMergerContractE$"},
		"cmd/herdr-sync": {"^TestContractA$", "^TestContractB$", "^TestContractC$", "^TestContractD$", "^TestContractE$"},
	} {
		got := partRuns(pkg)
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("splits의 %s에 part %s가 없다: %q", pkg, w, got)
			}
		}
	}
	var prev []string // tests/merger의 part: 앞 part는 -skip
	runs := partRuns("tests/merger")
	if len(runs) == 0 {
		t.Fatal("splits에 tests/merger가 없다")
	}
	for _, r := range runs {
		s := "test -race -count=1 -timeout 40m -run " + r
		if len(prev) > 0 {
			s += " -skip " + strings.Join(prev, "|")
		}
		shardsWant, prev = append(shardsWant, s+" herdr-tools/tests/merger"), append(prev, r)
	}
	shardsWant = append(shardsWant, "test -race -count=1 -timeout 40m -skip "+strings.Join(prev, "|")+" herdr-tools/tests/merger")
	nsh := strconv.Itoa(len(shardsWant))
	if rc != 0 || !strings.HasPrefix(calls, want) || !sameSet(strings.TrimPrefix(calls, want), shardsWant) || !strings.Contains(out, "검증 시간 ") {
		t.Errorf("통과: 싼 검사 → cotenant 홀로 → go list → 조각이어야 rc=%d\n%s\n%s", rc, calls, out)
	}
	if !strings.Contains(out, "== [1/"+nsh+"] ") || !strings.Contains(out, "-- ["+nsh+"/"+nsh+"] ") || !strings.Contains(out, " 끝 ") {
		t.Errorf("조각마다 띄움·끝 줄이어야\n%s", out)
	}
	t.Setenv("FAKE_LIST_RC", "1")
	rc, out, calls = run()
	if rc != 0 || !strings.HasSuffix(calls, "list ./...\n"+strings.Split(want, "\n")[3]+"\ntest -race -count=1 -timeout 40m -skip ^TestCotenantGuard$ ./...\n") || !strings.Contains(out, "알림 go list 실패") {
		t.Errorf("go list 실패: 옛 꼴 한 번이어야 rc=%d\n%s\n%s", rc, calls, out)
	}
	t.Setenv("FAKE_LIST_RC", "0")
	t.Setenv("FAKE_CHEAP_RC", "1")
	rc, out, calls = run()
	if rc != 1 || strings.Contains(calls, "-race") || !strings.Contains(out, "실패 싼 검사 — 무거운 시험을 건너뛰고 끝") {
		t.Errorf("싼 검사 실패: 무거운 시험 없이 1이어야 rc=%d\n%s\n%s", rc, calls, out)
	}
	t.Setenv("HERDR_VERIFY_ALL", "1")
	rc, _, calls = run()
	if rc != 1 || strings.Count(calls, "-race") != 1+len(shardsWant) || strings.Count(calls, "./tests/repo/") != 1 {
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
	if _, _, calls = run(); strings.Count(calls, "-timeout 40m -p 3 -run ^TestCotenantGuard$") != 1 || strings.Count(calls, "-p ") != 2 ||
		!strings.Contains(calls, "-timeout 40m -p 1 -skip ^TestCotenantGuard$ herdr-tools/a") {
		t.Errorf("HERDR_VERIFY_JOBS=3: cotenant -p 3, 나머지 조각 -p 1(3의 1/3)이어야\n%s", calls)
	}
	t.Setenv("HERDR_VERIFY_JOBS", "x")
	if rc, out, calls = run(); rc != 0 || strings.Count(calls, "-p ") != 1 || !strings.Contains(calls, "-p "+p+" -skip") || !strings.Contains(out, "무시한다") {
		t.Errorf("틀린 HERDR_VERIFY_JOBS는 알리고 무시(코어 수 — 나머지 조각만 -p %s) rc=%d\n%s\n%s", p, rc, calls, out)
	}
	if rc, _, _ = run("--bogus"); rc != 2 {
		t.Errorf("모르는 인자는 2: %d", rc)
	}
	// 검증 잠금: 다른 검증이 쥔 동안 무거운 단계 앞에서 기다렸다가 돈다(싼 검사는 먼저) · HERDR_VERIFY_NO_LOCK=1이면 안 기다린다
	t.Setenv("HERDR_VERIFY_JOBS", "")
	if !strings.HasPrefix(lockPath(), B+"/") {
		t.Fatalf("잠금 파일이 시험 폴더 밖: %s", lockPath())
	}
	held := hold(t, lockPath())
	releaseOnBlock(t, held)
	if rc, out, calls = run(); rc != 0 || !waitedRe.MatchString(out) ||
		!strings.HasPrefix(calls, want) || strings.Index(out, "기다렸다") > strings.Index(out, "TestCotenantGuard") {
		t.Errorf("잠금을 쥔 동안: 기다린 초 한 줄 뒤 무거운 단계여야 rc=%d\n%s\n%s", rc, calls, out)
	}
	held = hold(t, lockPath())
	defer held.Close()
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	t0 := time.Now()
	if rc, out, _ = run(); rc != 0 || strings.Contains(out, "잠금") || time.Since(t0) > 5*time.Second {
		t.Errorf("HERDR_VERIFY_NO_LOCK=1: 잠금을 안 보고 바로 돈다 rc=%d %v\n%s", rc, time.Since(t0), out)
	}
}

// waitedRe: 기다린 줄 — 초 값은 0보다 크기만 본다(부하에서 타이머가 밀려도)
var waitedRe = regexp.MustCompile(`다른 검증을 [1-9][0-9]*초 기다렸다`)

// releaseOnBlock: 쥔 쪽(held)을 시간이 아니라 순서로 놓는다 — 둘째가 그 잠금에 막힌 것(lockBlocked)을 본 뒤에만 닫는다.
// 막히지 않으면(잠금을 안 보면) 놓지 않아 기다린 줄이 없다. 돌려주는 함수(시험 끝에도 불림)가 고리를 걷고 닫는다
func releaseOnBlock(t *testing.T, held *os.File) func() {
	t.Helper()
	var once sync.Once
	lockBlocked = func() { once.Do(func() { held.Close() }) }
	undo := func() { lockBlocked = func() {}; once.Do(func() { held.Close() }) }
	t.Cleanup(undo)
	return undo
}

// hold: 다른 검증처럼 path에 배타 flock을 쥔다(닫으면 풀린다)
func hold(t *testing.T, path string) *os.File {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	return f
}

// 검증 잠금 자체: 풀리면 얻고 · 상한을 넘기면 경고 뒤 잠금 없이 진행 · 기다리다 끊기면 128+신호 · 얻은 잠금은 풀 때까지 남을 막는다
func TestVerifyLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "verify.lock")
	held := hold(t, p)
	unhook := releaseOnBlock(t, held)
	f, read := outFile(t)
	release, rc := verifyLock(p, time.Minute, make(chan os.Signal), f)
	unhook()
	if rc != 0 || !waitedRe.MatchString(read()) {
		t.Errorf("풀리면 얻음: 기다린 초 한 줄이어야 rc=%d\n%s", rc, read())
	}
	if f2, err := os.OpenFile(p, os.O_RDWR, 0); err != nil || syscall.Flock(int(f2.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
		t.Errorf("얻은 잠금은 남을 막아야 err=%v", err)
	} else {
		f2.Close()
	}
	release()

	held = hold(t, p)
	defer held.Close()
	f, read = outFile(t)
	t0 := time.Now()
	release, rc = verifyLock(p, 1500*time.Millisecond, make(chan os.Signal), f)
	release()
	if d := time.Since(t0); rc != 0 || d < 1500*time.Millisecond || d > 10*time.Second || !strings.Contains(read(), "상한을 넘겼다 — 잠금 없이 진행한다") {
		t.Errorf("상한 뒤 진행: 경고 한 줄·0이어야 rc=%d %v\n%s", rc, d, read())
	}

	f, read = outFile(t)
	stop := make(chan os.Signal, 1)
	stop <- syscall.SIGTERM
	if _, rc = verifyLock(p, time.Minute, stop, f); rc != 143 || !strings.Contains(read(), "검증 잠금을 기다리다") {
		t.Errorf("기다리다 끊김: 143이어야 rc=%d\n%s", rc, read())
	}
}

// sameSet: got(줄마다 하나)과 want가 차례 없이 같다
func sameSet(got string, want []string) bool {
	var g []string
	if got = strings.TrimSuffix(got, "\n"); got != "" {
		g = strings.Split(got, "\n")
	}
	w := append([]string{}, want...)
	sort.Strings(g)
	sort.Strings(w)
	return strings.Join(g, "\n") == strings.Join(w, "\n")
}

func outFile(t *testing.T) (*os.File, func() string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, func() string { b, _ := os.ReadFile(f.Name()); return string(b) }
}

// 조각 실행: 실패 조각의 출력이 「끝」 줄 뒤에 그대로, 남긴 배경 프로세스는 다 끝난 뒤 고아, 함께 도는 수는 n까지(들어가는 조각 먼저)
func TestRunShards(t *testing.T) {
	R, P := tmp(t), t.TempDir()
	f, read := outFile(t)
	sh := []shard{
		{argv: []string{"sh", "-c", `echo "--- FAIL: TestX ($S)"; exit 3`}, env: []string{"S=s1"}},
		{argv: []string{"sh", "-c", "sleep 300 </dev/null >/dev/null 2>&1 & echo $! >'" + P + "/bg'"}},
		{argv: []string{"sh", "-c", "echo ok-c"}},
		{argv: []string{"sh", "-c", "sleep 2; echo slow-ok"}}, // 다른 조각이 끝나며 고아 검사를 해도 함께 도는 이 조각은 그대로
	}
	rc := runShards(R, sh, nil, 4, make(chan os.Signal), f)
	out := read()
	p := readPid(t, P+"/bg")
	if rc != 1 || !strings.Contains(out, "--- FAIL: TestX (s1)") || !regexp.MustCompile(`끝 [0-9]+초 종료 3\n--- FAIL`).MatchString(out) ||
		!strings.Contains(out, "ok-c") || !strings.Contains(out, "고아 pid "+strconv.Itoa(p)) || !strings.Contains(out, "실패 고아를 남김") || !gone(p) ||
		!regexp.MustCompile(`sleep 2; echo slow-ok 끝 [0-9]+초 종료 0\nslow-ok`).MatchString(out) {
		t.Errorf("실패 조각·고아: 1·출력 그대로·고아 끔이어야 rc=%d alive=%v\n%s", rc, alive(p), out)
	}
	two := []shard{{argv: []string{"sleep", "2"}}, {argv: []string{"sleep", "2"}}}
	heavy := []shard{{argv: []string{"sleep", "2"}, w: 2}, {argv: []string{"sleep", "2"}}}
	// 들어가는 조각을 먼저: 둘째(자리 2)가 못 들어가도 셋째(자리 1)는 첫째와 함께 — 2+2초(앞에 막히면 2+2+2초)
	ahead := []shard{{argv: []string{"sleep", "2"}}, {argv: []string{"sleep", "2"}, w: 2}, {argv: []string{"sleep", "2"}}}
	for _, c := range []struct {
		sh        []shard
		n, lo, hi int
	}{{two, 2, 0, 3500}, {two, 1, 4000, 60000}, {heavy, 2, 4000, 60000}, {heavy, 3, 0, 3500}, {ahead, 2, 4000, 5800}} {
		f, read := outFile(t)
		t0 := time.Now()
		rc := runShards(R, c.sh, nil, c.n, make(chan os.Signal), f)
		if ms := int(time.Since(t0).Milliseconds()); rc != 0 || ms < c.lo || ms > c.hi {
			t.Errorf("n=%d: 통과·%d~%dms여야 rc=%d %dms\n%s", c.n, c.lo, c.hi, rc, ms, read())
		}
	}
}

// 끊기면(TERM) 도는 조각 무리를 모두 끄고 143
func TestRunShardsStop(t *testing.T) {
	R, P := tmp(t), t.TempDir()
	f, read := outFile(t)
	stop := make(chan os.Signal, 1)
	rcc := make(chan int, 1)
	sh := []shard{
		{argv: []string{"sh", "-c", "sleep 300 </dev/null >/dev/null 2>&1 & echo $! >'" + P + "/a'; wait"}},
		{argv: []string{"sh", "-c", "sleep 300 </dev/null >/dev/null 2>&1 & echo $! >'" + P + "/b'; wait"}},
	}
	go func() { rcc <- runShards(R, sh, nil, 2, stop, f) }()
	a, b := readPid(t, P+"/a"), readPid(t, P+"/b")
	stop <- syscall.SIGTERM
	select {
	case rc := <-rcc:
		if rc != 143 || !gone(a) || !gone(b) {
			t.Errorf("끊김: 143·두 조각 다 끔이어야 rc=%d\n%s", rc, read())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("끊김 뒤 돌아오지 않음")
	}
}

// 진짜 go로: 임시 모듈의 시험(하위 시험·HCFILL_SEC 절 포함)이 조각들에서 정확히 한 번씩 돌고, 표에 없는 새 시험도 들고,
// TestCotenantGuard는 안 돈다
func TestShardsRealGo(t *testing.T) {
	M := tmp(t)
	mark := "func mark(s string) {\n\tf, _ := os.OpenFile(os.Getenv(\"ATOMS\"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)\n\tf.WriteString(s + \"\\n\")\n\tf.Close()\n}\n"
	files := map[string]string{
		"go.mod": "module herdr-tools\n\ngo 1.24\n",
		"p/p_test.go": "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" + mark +
			"func TestA(t *testing.T) {\n\tfor _, s := range []string{\"x\", \"y\", \"z\"} {\n\t\tt.Run(s, func(t *testing.T) { mark(\"TestA/\" + s) })\n\t}\n}\n" +
			"func TestB(t *testing.T)   { mark(\"TestB\") }\nfunc TestC(t *testing.T)   { mark(\"TestC\") }\nfunc TestNew(t *testing.T) { mark(\"TestNew\") }\n",
		"s/s_test.go": "package s\n\nimport (\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n)\n\n" + mark +
			"var secs []string\n\nfunc reg(order int, name string) { secs = append(secs, name) }\n\nfunc init() { reg(1, \"s1\"); reg(2, \"s2\"); reg(3, \"s3\") }\n\n" +
			"func TestS(t *testing.T) {\n\tonly := os.Getenv(\"HCFILL_SEC\")\n\tfor _, n := range secs {\n\t\tif only == \"\" || strings.Contains(\",\"+only+\",\", \",\"+n+\",\") {\n\t\t\tmark(\"TestS/\" + n)\n\t\t}\n\t}\n}\n" +
			"func TestT(t *testing.T) { mark(\"TestT\") }\n",
		"q/q_test.go": "package q\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" + mark +
			"func TestQ(t *testing.T)              { mark(\"TestQ\") }\nfunc TestCotenantGuard(t *testing.T) { mark(\"TestCotenantGuard\") }\n",
	}
	for n, s := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(M, n)), 0o755)
		if err := os.WriteFile(filepath.Join(M, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	atoms := filepath.Join(t.TempDir(), "atoms")
	t.Setenv("ATOMS", atoms)
	t.Setenv("HCFILL_SEC", "")
	t.Setenv("GOFLAGS", "")
	sp := []split{
		{mod + "p", []part{{run: "^TestA$/^x$"}, {run: "^TestA$/^(y)$"}, {run: "^TestA$"}, {run: "^TestB$"}}, 0},
		{mod + "s", []part{{run: "^TestS$", secs: [][]string{{"s2"}, {"gone"}}}}, 0},
	}
	pkgs, err := goList(M)
	if err != nil {
		t.Fatal(err)
	}
	sh := shards(M, pkgs, []string{"go", "test", "-count=1"}, sp, 4)
	f, read := outFile(t)
	if rc := runShards(M, sh, nil, 4, make(chan os.Signal), f); rc != 0 || len(sh) != 9 {
		t.Fatalf("조각 9개 통과여야 rc=%d 조각 %d\n%s", rc, len(sh), read())
	}
	b, _ := os.ReadFile(atoms)
	want := []string{"TestA/x", "TestA/y", "TestA/z", "TestB", "TestC", "TestNew", "TestS/s1", "TestS/s2", "TestS/s3", "TestT", "TestQ"}
	if !sameSet(string(b), want) {
		t.Errorf("시험마다 정확히 한 번이어야\n%s\n조각 출력:\n%s", b, read())
	}
}

// selects: go test -run·-skip(꼴: 맨 위 |로 나뉜 대안, 대안은 /로 나뉜 단계)이 시험 경로 path를 돌리는지 — testing의 맞춤 규칙
func selects(run, skip string, path []string) bool {
	alts := func(p string, sep byte) []string { // 괄호·대괄호 밖의 sep로 나눈다
		var out []string
		depth, last := 0, 0
		for i := 0; i < len(p); i++ {
			switch p[i] {
			case '(', '[':
				depth++
			case ')', ']':
				depth--
			case sep:
				if depth == 0 {
					out, last = append(out, p[last:i]), i+1
				}
			}
		}
		return append(out, p[last:])
	}
	match := func(p string, full bool) bool {
		for _, a := range alts(p, '|') {
			lv := alts(a, '/')
			if full && len(lv) > len(path) {
				continue
			}
			ok := true
			for i := 0; i < len(lv) && i < len(path); i++ {
				ok = ok && regexp.MustCompile(lv[i]).MatchString(path[i])
			}
			if ok {
				return true
			}
		}
		return false
	}
	return (run == "" || match(run, false)) && (skip == "" || !match(skip, true))
}

// 저장소의 실제 splits: 표의 꾸러미·part가 다 있고, 그 꾸러미의 최상위 시험(나눈 시험은 절마다)이 조각 하나에서만 돌고,
// 나머지 꾸러미는 나머지 조각에 한 번씩
func TestSplitsCoverRepo(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := goList(root)
	if err != nil {
		t.Fatal(err)
	}
	sh := shards(root, pkgs, []string{"go", "test"}, splits, 12)
	type sel struct{ run, skip, sec string }
	byPkg, inRest := map[string][]sel{}, map[string]int{}
	for _, s := range sh {
		var x sel
		var ps []string
		for i := 2; i < len(s.argv); i++ {
			switch a := s.argv[i]; a {
			case "-run", "-skip":
				if a == "-run" {
					x.run = s.argv[i+1]
				} else {
					x.skip = s.argv[i+1]
				}
				i++
			case "-p":
				i++
			default:
				ps = append(ps, a)
			}
		}
		for _, e := range s.env {
			x.sec = strings.TrimPrefix(e, "HCFILL_SEC=")
		}
		if len(ps) > 1 || x.skip == cotenant {
			for _, p := range ps {
				inRest[p]++
			}
			continue
		}
		byPkg[ps[0]] = append(byPkg[ps[0]], x)
	}
	have := map[string]bool{}
	for _, p := range pkgs {
		have[p] = true
	}
	split := map[string]bool{}
	for _, s := range splits {
		split[s.pkg] = true
		if !have[s.pkg] {
			t.Errorf("splits의 %s가 go list에 없다 — 표를 고친다", s.pkg)
		}
	}
	for _, p := range pkgs {
		if want := map[bool]int{true: 0, false: 1}[split[p]]; inRest[p] != want {
			t.Errorf("%s: 나머지 조각에 %d번이어야 %d번", p, want, inRest[p])
		}
	}
	for _, s := range splits {
		if !have[s.pkg] {
			continue
		}
		c := exec.Command("go", "test", "-list", "^(Test|Example|Fuzz)", "./"+strings.TrimPrefix(s.pkg, mod))
		c.Dir = root
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("go test -list %s: %v\n%s", s.pkg, err, out)
		}
		secs := sections(filepath.Join(root, strings.TrimPrefix(s.pkg, mod)))
		var paths [][]string
		for _, n := range strings.Fields(string(out)) {
			if !regexp.MustCompile(`^(Test|Example|Fuzz)`).MatchString(n) {
				continue
			}
			sub := false
			for _, pt := range s.parts {
				sub = sub || ((pt.secs != nil || strings.Contains(pt.run, "/")) && selects(pt.run, "", []string{n}))
			}
			if !sub {
				paths = append(paths, []string{n})
				continue
			}
			if secs == nil {
				t.Fatalf("%s %s: 절(reg 호출)을 못 읽었다", s.pkg, n)
			}
			for _, sc := range secs {
				paths = append(paths, []string{n, sc})
			}
		}
		for _, pt := range s.parts {
			if pt.secs != nil && (s.pkg != mod+"tests/hcfill" || pt.run != "^TestHcFill$") {
				t.Errorf("%s: part %q의 secs — HCFILL_SEC는 tests/hcfill TestHcFill만 읽는다", s.pkg, pt.run)
			}
			hit := false
			for _, p := range paths {
				hit = hit || selects(pt.run, "", p) && (len(p) == 2 || !strings.Contains(pt.run, "/")) // 하위 시험 part는 절까지 맞아야
			}
			if !hit {
				t.Errorf("%s: part %q가 고르는 시험이 없다 — 표를 고친다", s.pkg, pt.run)
			}
		}
		for _, p := range paths {
			n := 0
			for _, x := range byPkg[s.pkg] {
				if selects(x.run, x.skip, p) && (x.sec == "" || len(p) < 2 || strings.Contains(","+x.sec+",", ","+p[1]+",")) {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%s %s: 조각 %d개에서 돈다 — 정확히 하나여야", s.pkg, strings.Join(p, "/"), n)
			}
		}
	}
}
