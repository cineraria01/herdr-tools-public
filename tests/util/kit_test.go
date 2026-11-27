// Package util_test: util-go 레인이 옮긴 작은 도구(hc-sweep·herdr-balance·ovis-split·worker-mac-check·worker-mac-setup)의 계약 시험.
// 옛 tests/<도구>.test.sh를 그대로 옮겼다(2026-11-21 go-tests-util) — 임시 HOME·가짜 hc·가짜 원격을 만들고 그 도구의 Go 바이너리
// (clitest.Bin)를 밖에서 exec해 출력·종료 코드·상태 파일을 본다. 단언 이름은 셸 시험의 것 그대로다. 판은 Go 하나다(2026-11-22 go-shell-remove).
// 실행: go test ./tests/util/ (저장소 루트에서)
package util_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

var repo string // 저장소 루트

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repo = filepath.Clean(filepath.Join(wd, "..", ".."))
	clitest.Main(m) // 끝나면 Bin 폴더를 지운다(재감사 base R-225)
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

// Env: 자식 환경. isolated가 허용 목록(clitest.Env — 임시 HOME T)에서 시작한다.
type Env map[string]string

func isolated(T string) Env {
	e := Env(clitest.Env(T))
	e["HC_SWEEP_TAILSCALE"] = "off" // 이 기계의 진짜 Tailscale을 부르지 않게
	e["HC_SWEEP_HEALTH"] = "off"    // 이 기계의 진짜 잠자기·키체인 점검과 그 사건을 내지 않게
	e["HC_SWEEP_EGO"] = "off"       // 이 기계의 진짜 ego 앱(작업 공간 목록)을 부르지 않게
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
	// 셸이 start를 쓴 뒤 상한에 걸려야 이 꼴이다 — 부하(조각 여럿 -race)에서 500ms가 셸 시작에 다 쓰이면 준비가 안 된 것이라 다시(5번까지,
	// 2026-11-23 shard-load-flaky)
	var out string
	var rc int
	for try := 0; try < 5 && (try == 0 || !clitest.Lexists(T+"/start")); try++ {
		out, rc = run(T, isolated(T), "/bin/sh", "-c", `(sleep 1; : >"$2") & : >"$1"; wait`, "sh", T+"/start", T+"/late")
	}
	if rc != 124 {
		t.Fatalf("rc %d out %q", rc, out)
	}
	clitest.WaitFile(t, T+"/start", time.Second)
	time.Sleep(1500 * time.Millisecond)
	if !clitest.Absent(t, T+"/late") {
		t.Fatal("배경 자식이 남아 늦은 표식을 썼다")
	}
}

// 재감사 base R-223: 바깥 마감(-test.timeout)이 cmdMax보다 먼저 오면 TERM을 무시하는 자식도 마감 전에 KILL·회수하고 124로 돌아온다.
// 시험 실행기(run)는 clitest.Cmd 위에 있다 — 마감 계약 전체는 internal/clitest/contract_test.go
func TestRunWithinTestDeadline(t *testing.T) {
	if d := os.Getenv("UTIL_DEADLINE_DIR"); d != "" {
		if _, rc := run(d, isolated(d), "/bin/sh", "-c", `trap '' TERM; : >"$1"; sleep 2; : >"$2"`, "sh", d+"/start", d+"/late"); rc != 124 {
			t.Fatalf("바깥 마감 전에 시간 넘김(124)이어야: %d", rc)
		}
		return
	}
	T := tmpdir(t)
	// 안쪽 셸이 start를 쓴 뒤 바깥 마감(1초)에 걸려야 이 꼴이다 — 부하(조각 여럿 -race)에서 그 1초가 안쪽 시험 바이너리·셸 시작에 다 쓰이면
	// 준비가 안 된 것이라 다시(5번까지, 2026-11-23 shard-load-flaky)
	var out string
	var rc int
	for try := 0; try < 5 && (try == 0 || !clitest.Lexists(T+"/start")); try++ {
		out, rc = run(T, isolated(T).With("UTIL_DEADLINE_DIR="+T), os.Args[0], "-test.run=^TestRunWithinTestDeadline$", "-test.timeout=1s", "-test.count=1")
	}
	if rc != 0 {
		t.Fatalf("안쪽 시험이 마감 전에 끝나야(종료 0): rc %d\n%s", rc, out)
	}
	clitest.WaitFile(t, T+"/start", time.Second)
	time.Sleep(2500 * time.Millisecond)
	if !clitest.Absent(t, T+"/late") {
		t.Error("바깥 마감 뒤 TERM 무시 자식이 남아 늦은 표식을 썼다")
	}
}

// runCmd: dir에서 env e(전부 — 바깥 환경을 덧씌우지 않는다)로 cmdMax 상한. 새 무리·시험 마감 안 회수는 clitest.Cmd.Run(재감사 base R-223)
func runCmd(dir string, e Env, combined bool, name string, args ...string) clitest.Res {
	return clitest.Cmd{Dir: dir, Env: e.list(), Exact: true, Combined: combined, Max: cmdMax}.Run(name, args...)
}

// run: dir에서 env로 name을 돌려 합친 출력과 종료 코드. 띄우지 못하면 rc 127, 상한은 cmdMax(rc 124) — 까닭은 출력 끝에
func run(dir string, e Env, name string, args ...string) (string, int) {
	r := runCmd(dir, e, true, name, args...)
	if r.Out != "" {
		r.Out += "\n" // clitest는 끝 줄바꿈을 뗀다 — 합친 출력 그대로처럼
	}
	return r.Out + r.Err, r.RC
}

// run2: run과 같되 stdout·stderr를 따로. 셸 시험의 "$(…)"처럼 stdout 끝 줄바꿈은 뗀다.
func run2(dir string, e Env, name string, args ...string) (stdout, stderr string, rc int) {
	r := runCmd(dir, e, false, name, args...)
	return r.Out, r.Err, r.RC
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
