// Package util_test: util-go 레인이 옮긴 작은 도구(hc-sweep·herdr-balance·ovis-split·worker-mac-check·worker-mac-setup)의 계약 시험.
// 옛 tests/<도구>.test.sh를 그대로 옮겼다(2026-11-21 go-tests-util) — 임시 HOME·가짜 hc·가짜 원격을 만들고 그 도구의 Go 바이너리
// (clitest.Bin)를 밖에서 exec해 출력·종료 코드·상태 파일을 본다. 단언 이름은 셸 시험의 것 그대로다. 판은 Go 하나다(2026-11-22 go-shell-remove).
// 실행: go test ./tests/util/ (저장소 루트에서)
package util_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

var repo string // 저장소 루트

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repo = filepath.Clean(filepath.Join(wd, "..", ".."))
	os.Exit(m.Run())
}

// run1: 새 임시 폴더·격리 환경으로 f를 돌린다.
func run1(t *testing.T, f func(t *testing.T, T string, e Env)) {
	T := tmpdir(t)
	f(t, T, isolated(T))
}

// tool: 시험할 그 도구의 Go 바이너리(이 시험 프로세스에서 cmd/<name>을 지은 것). 곁에서 찾는 도구(hc-lane·herdr-tools-config 등)도
// 같은 폴더에 지어 운영(~/.local/bin)과 같은 꼴로 둔다
func tool(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(clitest.BinDir(t, "hc-lane", "hc-merger", "herdr-tools-config", name), name)
}

// Env: 자식 환경. isolated가 바깥 환경을 복사해 시험 격리(tests/isolate.sh와 같은 것)를 한다.
type Env map[string]string

var isolateDrop = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_PREFIX",
	"XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
	"HC_LANE", "HC_LANE_OWNER", "HC_LANE_OWNER_RUN", "HC_LANE_LEASE", "HERDR_LAND_PRIORITY", "HC_LANE_PRIORITY", "HC_LANE_MERGER_BIN",
	"HC_TERMINAL_ID"}

func isolated(T string) Env {
	e := Env{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		e[k] = v
	}
	for _, k := range isolateDrop {
		delete(e, k)
	}
	e["GIT_CEILING_DIRECTORIES"] = T + ":" + filepath.Dir(T)
	e["HC_SWEEP_TAILSCALE"] = "off" // 이 기계의 진짜 Tailscale을 부르지 않게
	return e
}

func (e Env) With(kv ...string) Env {
	n := Env{}
	for k, v := range e {
		n[k] = v
	}
	for _, s := range kv {
		k, v, _ := strings.Cut(s, "=")
		n[k] = v
	}
	return n
}

func (e Env) Without(keys ...string) Env {
	n := e.With()
	for _, k := range keys {
		delete(n, k)
	}
	return n
}

func (e Env) list() []string {
	l := make([]string, 0, len(e))
	for k, v := range e {
		l = append(l, k+"="+v)
	}
	return l
}

// cmdMax: 시험이 부르는 명령 하나의 상한 — go test 기본 마감(10분)보다 짧게(B29). 넘기면 그 명령의 무리만 거두고 rc 124
var cmdMax = 4 * time.Minute

// [B29] 상한을 넘긴 명령은 그 무리(배경 자식까지)를 거두고 124로 곧 돌아온다 — 시험 마감에 걸려 자식이 남지 않게
func TestRunBounded(t *testing.T) {
	old := cmdMax
	cmdMax = 500 * time.Millisecond
	defer func() { cmdMax = old }()
	T := tmpdir(t)
	out, rc := run(T, isolated(T), "/bin/sh", "-c", "sleep 30 & echo $! >"+T+"/pid; wait")
	b, _ := os.ReadFile(T + "/pid")
	kid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if rc != 124 || kid <= 0 {
		t.Fatalf("rc %d kid %d out %q", rc, kid, out)
	}
	for i := 0; i < 20 && syscall.Kill(kid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if syscall.Kill(kid, 0) == nil {
		syscall.Kill(kid, syscall.SIGKILL)
		t.Fatalf("배경 자식 %d가 남음", kid)
	}
}

// start: name을 새 프로세스 무리로, cmdMax 상한과 함께. 끝나면 무리에 남은 것(출력을 문 손자 등)도 거둔다 — 시험이 띄운 무리만
func start(dir string, e Env, name string, args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdMax)
	cmd := exec.CommandContext(ctx, name, args...)
	proc.Group(cmd)
	cmd.Cancel = func() error { proc.StopGroup(cmd.Process.Pid, 2*time.Second); return nil }
	cmd.Dir, cmd.Env = dir, e.list()
	cmd.WaitDelay = 10 * time.Second // 출력을 물고 남은 손자가 있어도 끝나게
	return cmd, ctx, cancel
}

// finish: start한 cmd의 결과 — 상한·출력 관 기다림 넘김이면 그 무리를 끄고 (124, 까닭)
func finish(cmd *exec.Cmd, err error, ctxErr error) (int, string) {
	if cmd.Process != nil && (ctxErr != nil || errors.Is(err, exec.ErrWaitDelay)) {
		proc.StopGroup(cmd.Process.Pid, 2*time.Second)
		return 124, fmt.Sprintf("시험 실행기: 시간 넘김(%s 또는 출력 관) — 무리를 껐다: %s\n", cmdMax, cmd.Path)
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, ""
	case errors.As(err, &ee) && ee.ExitCode() >= 0:
		return ee.ExitCode(), ""
	}
	return -1, err.Error()
}

// run: dir에서 env로 name을 돌려 합친 출력과 종료 코드. 띄우지 못하면 rc -1과 그 오류를 출력에. 상한은 cmdMax(rc 124)
func run(dir string, e Env, name string, args ...string) (string, int) {
	cmd, ctx, cancel := start(dir, e, name, args...)
	defer cancel()
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err := cmd.Run()
	rc, why := finish(cmd, err, ctx.Err())
	return b.String() + why, rc
}

// run2: run과 같되 stdout·stderr를 따로. 셸 시험의 "$(…)"처럼 stdout 끝 줄바꿈은 뗀다.
func run2(dir string, e Env, name string, args ...string) (stdout, stderr string, rc int) {
	cmd, ctx, cancel := start(dir, e, name, args...)
	defer cancel()
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	rc, why := finish(cmd, err, ctx.Err())
	se.WriteString(why)
	return strings.TrimRight(so.String(), "\n"), se.String(), rc
}

// tmpdir: 실제 경로(pwd -P)로 푼 임시 폴더. 끝나면 쓰기 막힌 폴더도 지울 수 있게 풀어 둔다.
func tmpdir(t *testing.T) string {
	T, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		filepath.Walk(T, func(p string, fi os.FileInfo, err error) error {
			if err == nil && fi.IsDir() {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	return T
}

func mkdirs(t *testing.T, ds ...string) {
	t.Helper()
	for _, d := range ds {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	mkdirs(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// script: 실행 파일. @T@는 시험 폴더로 바꾼다(셸 시험의 here-doc 안 $T).
func script(t *testing.T, p, T, s string) {
	t.Helper()
	write(t, p, strings.ReplaceAll(s, "@T@", T))
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func read(p string) string { b, _ := os.ReadFile(p); return string(b) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// lines: 빈 줄을 뺀 줄 수(grep -c .).
func lines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			n++
		}
	}
	return n
}

// grepc: 정규식에 걸린 줄 수(grep -c, 확장 정규식 꼴 — BRE의 글자 괄호는 \(로 적는다).
func grepc(s, re string) int {
	r := regexp.MustCompile(re)
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if r.MatchString(l) {
			n++
		}
	}
	return n
}

func grep(s, re string) bool { return grepc(s, re) > 0 }

// q: 정규식 안에 넣을 글자 그대로.
func q(s string) string { return regexp.QuoteMeta(s) }

// ordered: s 안에 parts가 그 차례로 있는가(case $s in *a*b*).
func ordered(s string, parts ...string) bool {
	for _, p := range parts {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return true
}

func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

// gone: 2초 안에 끝나는가.
func gone(pid int) bool {
	for i := 0; i < 10; i++ {
		if !alive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// checker: 셸 시험의 t 꼴 — 실패해도 이어 가고, 실패면 이름과 마지막 출력을 찍는다.
type checker struct {
	t   *testing.T
	out *string
	rc  *int
}

func (c checker) ok(name string, cond bool) {
	c.t.Helper()
	if cond {
		return
	}
	msg := "FAIL - " + name
	if c.rc != nil {
		msg += "\n  rc=" + strconv.Itoa(*c.rc)
	}
	if c.out != nil && *c.out != "" {
		msg += "\n  | " + strings.ReplaceAll(strings.TrimRight(*c.out, "\n"), "\n", "\n  | ")
	}
	c.t.Error(msg)
}
