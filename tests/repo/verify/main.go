//go:build !windows

// Command verify: herdr-tools 저장소 검증 본문 — .claude/herdr-verify.sh가 저장소 뿌리에서 짓고 exec 한다(2026-11-22 go-shell-remove:
// 옛 셸 검증의 병렬 실행기·단계·셸 시험 목록을 걷고 go 명령 몇 개로). 표준 라이브러리만 쓴다(레인이 internal을 깨도 검증은 지어지게).
//
//	verify [--cheap-only]
//
// 싼 단계: go vet ./... · go test ./tests/repo/(gofmt·셸 문법·줄 수 예산·개인 값·비밀 꼴·싼 검사 빠짐). --cheap-only면 여기까지
// (.claude/herdr-precheck.sh — 병합기 제출 직전). 싼 단계가 실패하면 무거운 단계를 건너뛰고 1(HERDR_VERIFY_ALL=1이면 끝까지).
// 무거운 단계: go test -race ./...(Go 계약 시험 전부). TestCotenantGuard는 1분 부하를 재는 시험이라 먼저 홀로 돈다
// (2026-11-22 verify-go-timeout — 다른 꾸러미와 함께 돌면 10분을 넘겼다). HERDR_VERIFY_JOBS=<수>면 go test -p <수>.
//
// 고아 검사: 단계마다 go 명령을 새 프로세스 무리로 띄운다. 끝난 뒤 그 무리에 남은 프로세스, 또는 그 단계 중 새로 생겨 부모가 1이 된
// 프로세스 가운데 명령 줄이나 cwd에 이 폴더 경로가 든 것(과 그 자손)은 고아다: 목록(pid·cwd·명령)을 찍고 끄고(TERM, 3초 뒤 KILL)
// 실패. cwd로만 걸리고 명령 줄에 /.claude/가 든 것(이 폴더에서 도는 Claude 세션의 훅·플러그인이 떼어 띄운 것)은 뺀다.
// INT·TERM·HUP을 받으면 도는 무리에 TERM(10초 뒤 KILL)을 보내고 128+신호로 끝난다.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const heavyTimeout = "40m" // 꾸러미 시험 바이너리 하나의 상한(go test 기본 10분은 tests/hclane·hcfill에 짧다)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify:", err)
		os.Exit(1)
	}
	os.Exit(verify(root, os.Args[1:], stop, os.Stdout))
}

// plan: 싼 단계·무거운 단계의 go 인자
func plan(jobs string) (cheap, heavy [][]string) {
	cheap = [][]string{{"go", "vet", "./..."}, {"go", "test", "-count=1", "./tests/repo/"}}
	h := []string{"go", "test", "-race", "-count=1", "-timeout", heavyTimeout}
	if jobs != "" {
		h = append(h, "-p", jobs)
	}
	heavy = [][]string{
		append(append([]string{}, h...), "-run", "^TestCotenantGuard$", "./claude/hooks/tests/"),
		append(append([]string{}, h...), "-skip", "^TestCotenantGuard$", "./..."),
	}
	return cheap, heavy
}

// verify: 종료 코드. 0 통과 · 1 실패 · 2 틀린 인자 · 128+신호 끊김
func verify(root string, args []string, stop <-chan os.Signal, w *os.File) int {
	cheapOnly := len(args) == 1 && args[0] == "--cheap-only"
	if len(args) > 0 && !cheapOnly {
		fmt.Fprintf(w, "verify: 모르는 인자 %q (쓰는 법: verify [--cheap-only])\n", strings.Join(args, " "))
		return 2
	}
	jobs := os.Getenv("HERDR_VERIFY_JOBS")
	if n, err := strconv.Atoi(jobs); jobs != "" && (err != nil || n < 1) {
		fmt.Fprintf(w, "verify: HERDR_VERIFY_JOBS=%q 는 1 이상 정수가 아니라 무시한다\n", jobs)
		jobs = ""
	}
	cheap, heavy := plan(jobs)
	t0, fail := time.Now(), false
	for _, a := range cheap {
		rc := runStep(root, a, stop, w)
		if rc >= 128 {
			return rc
		}
		fail = fail || rc != 0
	}
	if cheapOnly {
		return b2i(fail)
	}
	if fail && os.Getenv("HERDR_VERIFY_ALL") != "1" {
		fmt.Fprintln(w, "실패 싼 검사 — 무거운 시험을 건너뛰고 끝 (끝까지 돌리려면 HERDR_VERIFY_ALL=1)")
		return 1
	}
	for _, a := range heavy {
		rc := runStep(root, a, stop, w)
		if rc >= 128 {
			return rc
		}
		fail = fail || rc != 0
	}
	fmt.Fprintf(w, "검증 시간 %d초 — %s\n", int(time.Since(t0).Seconds()), map[bool]string{false: "통과", true: "실패"}[fail])
	return b2i(fail)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// runStep: 한 명령을 새 무리로 돌리고 고아 검사. 종료 코드(고아가 있으면 0이라도 1, 신호로 끊기면 128+신호)
func runStep(root string, argv []string, stop <-chan os.Signal, w *os.File) int {
	fmt.Fprintf(w, "== %s\n", strings.Join(argv, " "))
	before, err := procs()
	if err != nil {
		fmt.Fprintf(w, "실패 고아 검사 준비: %v\n", err)
		return 1
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir, c.Stdout, c.Stderr = root, w, w // *os.File이라 그대로 물려준다(복사 고루틴이 출력을 붙든 자손을 기다리지 않게)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		fmt.Fprintf(w, "실패 %s: %v\n", argv[0], err)
		return 127
	}
	pg := c.Process.Pid
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err = <-done:
	case s := <-stop:
		_ = syscall.Kill(-pg, syscall.SIGTERM) // 이미 끝난 무리면 ESRCH — 볼 것 없다
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-pg, syscall.SIGKILL)
			<-done
		}
		fmt.Fprintf(w, "끊김(%v) — 도는 시험 무리를 껐다\n", s)
		if n, ok := s.(syscall.Signal); ok {
			return 128 + int(n)
		}
		return 130
	}
	rc := 0
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		if st, ok := ee.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			rc = 128 + int(st.Signal())
		} else {
			rc = ee.ExitCode()
		}
	default:
		fmt.Fprintf(w, "실패 %s: %v\n", argv[0], err)
		rc = 1
	}
	lines, err := orphans(root, pg, before)
	if err != nil {
		fmt.Fprintf(w, "실패 고아 검사 못 함: %v\n", err)
		return max(rc, 1)
	}
	if len(lines) > 0 {
		fmt.Fprintln(w, strings.Join(lines, "\n"))
		fmt.Fprintf(w, "실패 고아를 남김(위 목록, 껐음) — %s\n", strings.Join(argv, " "))
		if rc == 0 {
			rc = 1
		}
	}
	return rc
}

type pinfo struct {
	pgid, ppid int
	cmd        string
}

// procs: 이 사용자의 프로세스(ps -x). ps가 실패하면 오류 — 모르면 「고아 없음」으로 읽지 않는다
func procs() (map[int]pinfo, error) {
	out, err := exec.Command("ps", "-x", "-o", "pid=,pgid=,ppid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	m := map[int]pinfo{}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		pg, e2 := strconv.Atoi(f[1])
		pp, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		cmd := ""
		if len(f) > 3 {
			cmd = strings.Join(f[3:], " ")
		}
		m[pid] = pinfo{pg, pp, cmd}
	}
	return m, nil
}

// cwds: pid → cwd(lsof). 못 읽은 pid는 빠진다(그 프로세스는 명령 줄로만 판정)
func cwds(pids []int) map[int]string {
	m := map[int]string{}
	if len(pids) == 0 {
		return m
	}
	ps := make([]string, len(pids))
	for i, p := range pids {
		ps[i] = strconv.Itoa(p)
	}
	out, _ := exec.Command("lsof", "-a", "-d", "cwd", "-p", strings.Join(ps, ","), "-Fpn").Output() // 사라진 pid가 있으면 1 — 읽은 만큼 쓴다
	pid := 0
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "p"):
			pid, _ = strconv.Atoi(l[1:])
		case strings.HasPrefix(l, "n") && pid > 0:
			m[pid] = l[1:]
		}
	}
	return m
}

// orphans: pg 무리에 남은 것과, before에 없고 부모가 1이며 이 폴더(root) 것인 프로세스와 그 자손을 찍고 끈다. 찍은 줄
func orphans(root string, pg int, before map[int]pinfo) ([]string, error) {
	ps, err := procs()
	if err != nil {
		return nil, err
	}
	here := []string{root}
	if r, err := filepath.EvalSymlinks(root); err == nil && r != root {
		here = append(here, r)
	}
	me := os.Getpid()
	var fresh []int
	for p, i := range ps {
		if _, old := before[p]; !old && i.ppid == 1 {
			fresh = append(fresh, p)
		}
	}
	cwd := cwds(fresh)
	stray := func(p int) bool {
		c, cmd := cwd[p], ps[p].cmd
		for _, h := range here {
			if strings.Contains(cmd, h+"/") || (c != "" && strings.HasPrefix(c+"/", h+"/") && !strings.Contains(cmd, "/.claude/")) {
				return true
			}
		}
		return false
	}
	bad := map[int]bool{}
	for p, i := range ps {
		if p != me && i.pgid == pg {
			bad[p] = true
		}
	}
	for _, p := range fresh {
		if p != me && stray(p) {
			bad[p] = true
		}
	}
	for grew := true; grew; { // 자손도 함께
		grew = false
		for p, i := range ps {
			if !bad[p] && p != me && bad[i.ppid] {
				bad[p], grew = true, true
			}
		}
	}
	if len(bad) == 0 {
		return nil, nil
	}
	var pids []int
	for p := range bad {
		pids = append(pids, p)
	}
	sort.Ints(pids)
	var need []int
	for _, p := range pids {
		if _, ok := cwd[p]; !ok {
			need = append(need, p)
		}
	}
	for p, c := range cwds(need) {
		cwd[p] = c
	}
	lines := make([]string, 0, len(pids))
	for _, p := range pids {
		c := cwd[p]
		if c == "" {
			c = "?"
		}
		cmd := []rune(ps[p].cmd)
		if len(cmd) > 120 {
			cmd = cmd[:120]
		}
		lines = append(lines, fmt.Sprintf("  고아 pid %d (cwd %s): %s", p, c, string(cmd)))
	}
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGTERM) // 이미 끝났으면 ESRCH
	}
	for i := 0; i < 30 && anyAlive(pids); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil {
			_ = syscall.Kill(p, syscall.SIGKILL)
		}
	}
	return lines, nil
}

func anyAlive(pids []int) bool {
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil {
			return true
		}
	}
	return false
}
