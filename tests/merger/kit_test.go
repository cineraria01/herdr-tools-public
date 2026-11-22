// Package merger_test: 병합기(hc-lane merger·hc-lane land 제출)의 계약 시험 — 옛 tests/hc-lane-merger.test.sh·
// hc-lane-merger-recovery.test.sh를 옮긴 것(2026-11-22 go-shell-remove). 임시 HOME·bare origin·레인 클론·병합기 전용 클론·가짜 hc를
// 만들고 시험 프로세스에서 지은 Go 바이너리(clitest.BinDir — hc-lane·hc-merger·herdr-tools-config)를 밖에서 exec해 종료 코드·출력·
// 원격 참조·사건 파일을 본다. 단언 이름은 셸 시험 그대로다(go test -v에 「ok N - 이름」).
//
// 시험 준비(커밋·push 등 셸 시험의 (cd … && git …) 묶음)는 bash 한 줄(k.sh)로 두고, 판정은 Go로 한다. 가짜 git·hc·mv는 셸 시험처럼
// 실행 중에 쓰는 시험 대역이다.
// 실행: go test ./tests/merger/ (저장소 루트에서)
package merger_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
	"herdr-tools/internal/proc"
)

// R: 한 번 부른 결과. Out은 셸 $(…)처럼 끝 줄바꿈을 뺀 표준 출력
type R struct {
	Out, Err string
	RC       int
}

// K: 시험 하나의 상태 — 셸 시험의 전역 환경(export)과 단언 번호
type K struct {
	t             *testing.T
	T             string            // 시험 폴더(실제 경로)
	env           map[string]string // 바깥 export
	n, fails      int
	LANE, MERGER  string // 지은 hc-lane·hc-merger
	realGit       string
	detail        func() string // 실패 때 함께 찍을 것
	extraFiles19  *os.File      // 다음 merger 부름에 fd 19로 물려줄 파일(격리 시험)
	defaultMaxDur time.Duration
}

// dropEnv: tests/isolate.sh(lib/hc-lane/test-isolate.sh)와 셸 시험 머리가 지우던 바깥 값
var dropEnv = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_PREFIX", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
	"XDG_DATA_HOME", "HC_LANE", "HC_LANE_OWNER", "HC_LANE_OWNER_RUN", "HC_LANE_LEASE", "HERDR_LAND_PRIORITY", "HC_LANE_PRIORITY",
	"HC_LANE_MERGER_BIN", "HC_TERMINAL_ID", "HC_TERMINAL_SOCKET", "HC_SCOPE", "HC_LANE_MACHINE", "HC_LANE_JOB", "HERDR_MERGER",
	"HERDR_TOOLS_CONFIG", "HERDR_LAND_PUSH_EXTRA", "HC_FILL_VERDICTS", "HC_LANE_MERGER_BATCH", "HERDR_LAND_PATCHID",
	"HERDR_LAND_SHA", "HERDR_LAND_VERDICT", "HERDR_LAND_AFTER", "HC_LANE_COORD", "HC_LANE_FILL", "HC_LANE_BIN", "HC_LANE_SELF",
	"GIT_CONFIG_COUNT", "GOFLAGS"}

// newK: 시험 폴더 T(실제 경로)와 바깥 환경. PATH 앞은 T/bin(가짜 hc) → 지은 바이너리 폴더
func newK(t *testing.T) *K {
	t.Helper()
	clitest.Repo(t)
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	k := &K{t: t, T: T, env: map[string]string{}, defaultMaxDur: 5 * time.Minute}
	bd := clitest.BinDir(t, "hc-lane", "hc-merger", "herdr-tools-config")
	k.LANE, k.MERGER = filepath.Join(bd, "hc-lane"), filepath.Join(bd, "hc-merger")
	g, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	k.realGit = g
	for _, kv := range os.Environ() {
		key, v, _ := strings.Cut(kv, "=")
		k.env[key] = v
	}
	for _, key := range dropEnv {
		delete(k.env, key)
	}
	mkdirs(t, filepath.Join(T, "bin"))
	k.set("PATH="+filepath.Join(T, "bin")+":"+bd+":"+k.env["PATH"], "HOME="+T, "T="+T, "LANE="+k.LANE, "real_git="+g,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CEILING_DIRECTORIES="+T+":"+filepath.Dir(T), "HC_LANE_MACHINE=box", "TERM=xterm-256color")
	return k
}

func (k *K) set(kv ...string) {
	for _, s := range kv {
		key, v, _ := strings.Cut(s, "=")
		k.env[key] = v
	}
}

func (k *K) unset(keys ...string) {
	for _, key := range keys {
		delete(k.env, key)
	}
}

// environ: 바깥 export 위에 이 명령에만 먹는 VAR=값(셸 `VAR=값 명령`). "K"(= 없음)는 지운다
func (k *K) environ(extra []string) []string {
	m := map[string]string{}
	for key, v := range k.env {
		m[key] = v
	}
	for _, s := range extra {
		key, v, ok := strings.Cut(s, "=")
		if !ok {
			delete(m, key)
			continue
		}
		m[key] = v
	}
	l := make([]string, 0, len(m))
	for key, v := range m {
		l = append(l, key+"="+v)
	}
	return l
}

// exe: dir에서 부른다. 새 프로세스 무리로 띄우고 max(0이면 기본 5분)를 넘기면 그 무리를 끄고 RC 124(셸 perl alarm 꼴)
func (k *K) exe(dir string, extra []string, max time.Duration, name string, args ...string) R {
	if max <= 0 {
		max = k.defaultMaxDur
	}
	ctx, cancel := context.WithTimeout(context.Background(), max)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	proc.Group(cmd)
	cmd.Cancel = func() error { proc.StopGroup(cmd.Process.Pid, 2*time.Second); return nil }
	cmd.WaitDelay = 30 * time.Second // 출력을 물고 남은 손자(배경 sleep 등)가 있어도 끝나게
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	if k.extraFiles19 != nil {
		cmd.ExtraFiles = make([]*os.File, 17)
		cmd.ExtraFiles[16] = k.extraFiles19 // fd 3+16 = 19
	}
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	r := R{Out: strings.TrimRight(o.String(), "\n"), Err: e.String()}
	var ee *exec.ExitError
	switch {
	case ctx.Err() != nil:
		r.RC = 124
	case errors.As(err, &ee):
		r.RC = ee.ExitCode()
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			r.RC = 128 + int(ws.Signal())
		}
	case errors.Is(err, exec.ErrWaitDelay):
	case err != nil:
		r.RC, r.Err = 127, r.Err+err.Error()
	}
	return r
}

// sh: 시험 준비 셸 한 줄(bash -c, $1… = args). 실패면 시험을 멈춘다
func (k *K) sh(dir, script string, args ...string) string {
	k.t.Helper()
	r := k.shr(dir, nil, script, args...)
	if r.RC != 0 {
		k.t.Fatalf("준비 실패(rc %d): %s\n%s\n%s", r.RC, script, r.Out, r.Err)
	}
	return r.Out
}

// shr: 셸 한 줄의 결과(실패해도 멈추지 않음)
func (k *K) shr(dir string, extra []string, script string, args ...string) R {
	return k.exe(dir, extra, 0, "bash", append([]string{"-c", script, "sh"}, args...)...)
}

// lane: hc-lane …
func (k *K) lane(dir string, extra []string, args ...string) R {
	return k.exe(dir, extra, 0, k.LANE, args...)
}

// merger: 병합기 부르기 — 이 한 곳에서. 운영처럼 `hc-lane merger …`(곁의 hc-merger를 같은 pid로 exec, HC_LANE_SELF=자기 경로)
func (k *K) merger(dir string, extra []string, max time.Duration, args ...string) R {
	return k.exe(dir, extra, max, k.LANE, append([]string{"merger"}, args...)...)
}

// mergerSh: 가짜 git 같은 셸 대역 안에서 병합기를 부를 명령 앞부분(merger와 같은 길)
func (k *K) mergerSh() string { return "'" + k.LANE + "' merger" }

// mergerCmd: 배경으로 띄울 병합기(SIGKILL 시험). merger와 같은 길 — exec라 이 pid가 곧 병합기다
func (k *K) mergerCmd(dir string, extra []string, out string, args ...string) *exec.Cmd {
	cmd := exec.Command(k.LANE, append([]string{"merger"}, args...)...)
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	f, err := os.Create(out)
	if err != nil {
		k.t.Fatal(err)
	}
	k.t.Cleanup(func() { f.Close() })
	cmd.Stdout, cmd.Stderr = f, f
	return cmd
}

// git: git -C dir … 의 출력(실패해도 출력만 — 셸 $(git …))
func (k *K) git(dir string, args ...string) string {
	return k.exe(k.T, nil, 0, "git", append([]string{"-C", dir}, args...)...).Out
}

// gitOK: git -C dir … 이 0으로 끝났는지
func (k *K) gitOK(dir string, args ...string) bool {
	return k.exe(k.T, nil, 0, "git", append([]string{"-C", dir}, args...)...).RC == 0
}

// check: 셸 시험의 t — 번호·이름, 실패면 detail을 함께
func (k *K) check(name string, ok bool, detail ...string) {
	k.t.Helper()
	k.n++
	if ok {
		k.t.Logf("ok %d - %s", k.n, name)
		return
	}
	k.fails++
	d := strings.Join(detail, "\n")
	if k.detail != nil {
		d += "\n" + k.detail()
	}
	k.t.Errorf("FAIL %d - %s\n%s", k.n, name, d)
}

// stub: 실행 파일 쓰기(가짜 git·hc·검증 끼움 등)
func (k *K) stub(path, content string) {
	k.t.Helper()
	mkdirs(k.t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		k.t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		k.t.Fatal(err)
	}
}

// rows: TSV 파일의 줄을 칸으로(awk -F'\t')
func rows(path string) [][]string {
	var r [][]string
	for _, l := range lines(read(path)) {
		r = append(r, strings.Split(l, "\t"))
	}
	return r
}

// col: 1부터 센 칸(awk $n), 없으면 ""
func col(row []string, n int) string {
	if n-1 < len(row) {
		return row[n-1]
	}
	return ""
}

// evs: 사건 파일에서 레인($4)·상태($5)가 맞는 줄(원문)
func evs(path, lane, state string) []string {
	var r []string
	for _, row := range rows(path) {
		if col(row, 4) == lane && col(row, 5) == state {
			r = append(r, strings.Join(row, "\t"))
		}
	}
	return r
}

// anyHas: 줄 가운데 s를 담은 것이 있는지(grep -Fq)
func anyHas(ls []string, s string) bool {
	for _, l := range ls {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// countHas: s를 담은 줄 수(grep -c -F)
func countHas(ls []string, s string) int {
	n := 0
	for _, l := range ls {
		if strings.Contains(l, s) {
			n++
		}
	}
	return n
}

func lines(s string) []string { return clitest.Lines(s) }

func hasLine(s, l string) bool { return clitest.HasLine(s, l) }

// grepE: 확장 정규식이 걸린 줄이 있는지
func grepE(s, re string) bool { return regexp.MustCompile("(?m)" + re).MatchString(s) }

func read(p string) string { b, _ := os.ReadFile(p); return string(b) }

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

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

func touch(t *testing.T, p string) { t.Helper(); write(t, p, "") }

// countNonEmpty: grep -c .
func countNonEmpty(s string) int { return clitest.CountNonEmpty(s) }

// head: 앞 n줄을 쉼표로(head -n N | paste -sd , -)
func head(s string, n int) string {
	l := lines(s)
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, ",")
}

// tryLock: 그 잠금 파일을 지금 잡을 수 있는지(flock LOCK_EX|LOCK_NB — 잡았으면 바로 놓는다)
func tryLock(p string) bool {
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil
}

// holdLock: 그 잠금 파일을 쥔다 — 돌려준 함수로 놓는다(셸 perl flock … sleep 60 &)
func holdLock(t *testing.T, p string) (*os.File, func()) {
	t.Helper()
	mkdirs(t, filepath.Dir(p))
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	return f, func() { f.Close() }
}

// waitFor: 조건이 참이 될 때까지 step마다 n번(셸 for i in $(seq n); do … && break; sleep …)
func waitFor(n int, step time.Duration, ok func() bool) bool {
	for i := 0; i < n; i++ {
		if ok() {
			return true
		}
		time.Sleep(step)
	}
	return ok()
}
