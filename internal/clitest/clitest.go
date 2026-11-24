// Package clitest: 명령을 밖에서 부르는 계약 시험 도우미 — tests/*.test.sh를 Go 시험으로 옮긴 것(2026-11-21 go-tests).
// 시험은 임시 HOME·가짜 원격·가짜 hc를 만들고 그 도구의 Go 바이너리(Bin — 시험 프로세스마다 cmd/<도구>를 한 번 지음)를 exec해
// 출력·종료 코드·상태를 본다. 셸 시험의 단언 하나가 Ok 한 번이고, 이름은 셸 시험 그대로다(go test -v에 「ok N - 이름」, 실패는 「FAIL N - 이름」).
// 셸 판은 2026-11-22 걷었다(go-shell-remove) — 판은 Go 하나다.
package clitest

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/proc"
)

// C: 셸 시험의 t() — 단언마다 번호를 매기고 실패면 Detail을 함께 찍는다
type C struct {
	T      *testing.T
	N      int
	Fails  int
	Detail func() string
}

// Ok: 단언 하나
func (c *C) Ok(name string, ok bool) {
	c.T.Helper()
	c.N++
	if ok {
		c.T.Logf("ok %d - %s", c.N, name)
		return
	}
	c.Fails++
	d := ""
	if c.Detail != nil {
		d = "\n" + c.Detail()
	}
	c.T.Errorf("FAIL %d - %s%s", c.N, name, d)
}

// Done: 셸 시험의 끝 줄
func (c *C) Done() { c.T.Logf("%d/%d 통과", c.N-c.Fails, c.N) }

// Root: go.mod가 있는 모듈 루트(go test의 작업 폴더 = 꾸러미 폴더에서 위로)
func Root(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			t.Fatal("go.mod를 찾지 못했다")
		}
		d = p
	}
}

// Repo: 범용 저장소 루트. 계약 시험은 저장소 꼴(cmd/·config.example/)에서만 돈다 — 깔린 곳(~/.local/lib/herdr-go)은 건너뛴다
func Repo(t *testing.T) string {
	t.Helper()
	r := Root(t)
	for _, p := range []string{"cmd", "config.example"} {
		if st, err := os.Stat(filepath.Join(r, p)); err != nil || !st.IsDir() {
			t.Skipf("저장소 밖(%s 없음 — 깔린 곳): 계약 시험은 범용 저장소에서 돈다", p)
		}
	}
	return r
}

// Tool: 부를 도구 — 시험 전용 환경 override(예 HERDR_SYNC)가 있으면 그것, 없으면 Bin(name)
func Tool(t *testing.T, name, override string) string {
	t.Helper()
	if v := os.Getenv(override); override != "" && v != "" {
		return v
	}
	return Bin(t, name)
}

var (
	binMu   sync.Mutex
	binDir  string
	binErr  = map[string]error{}
	binDone = map[string]bool{}
)

// Bin: cmd/<name>을 이 시험 프로세스에서 한 번 지은 바이너리 경로. 모든 도구를 한 폴더에 지어, 곁 도구(hc-lane → hc-merger·
// herdr-tools-config 등)를 바이너리 곁에서 찾는 길이 운영(~/.local/bin)과 같다. go build도 Cmd.Run(시험 마감 안·무리 회수)으로 부른다.
// 폴더 herdr-test-bin.<pid>.*는 Main이 시험 프로세스 끝에 지우고, 마감 panic·KILL로 남은 것은 다음 Bin이 주인 pid가 없을 때 지운다(재감사 base R-224·R-225)
func Bin(t *testing.T, name string) string {
	t.Helper()
	binMu.Lock()
	defer binMu.Unlock()
	if binDir == "" {
		sweepBinDirs()
		d, err := os.MkdirTemp("", fmt.Sprintf("herdr-test-bin.%d.", os.Getpid()))
		if err != nil {
			t.Fatal(err)
		}
		binDir = d
	}
	p := filepath.Join(binDir, name)
	if !binDone[name] {
		binDone[name] = true
		binErr[name] = build(Root(t), p, name, TestTag)
	}
	if err := binErr[name]; err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTag: 시험 손잡이(시험 전용 환경 변수)를 넣는 빌드 태그 — Bin은 늘 이것으로 짓고, 배포 빌드(herdr-sync)는 태그 없이 짓는다.
// 손잡이 코드는 cmd/<도구>/*_herdrtest.go(//go:build herdrtest)에만 둔다(docs/repo/tests.md 「시험 손잡이」)
const TestTag = "herdrtest"

// BinPlain: 배포와 같은 꼴(태그 없이)로 dir에 지은 cmd/<name> — 시험 손잡이가 빠졌는지 보는 시험만 쓴다
func BinPlain(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := build(Root(t), p, name, ""); err != nil {
		t.Fatal(err)
	}
	return p
}

// BuildOverlayEnv: 변이 확인용 손잡이(2026-11-24 contract-subtests) — 값이 go build -overlay JSON의 절대 경로면 Bin·BinPlain이
// 짓는 도구에 그 overlay를 준다. 저장소 사본 없이 「이 변이에 그 절이 실패한다」를 본다(docs/repo/tests.md 「변이 확인」).
// 시험 프로세스가 뜰 때 한 번 읽는다 — Isolate가 환경을 허용 목록으로 줄인 뒤에 짓는 시험도 받게
const BuildOverlayEnv = "HERDR_BUILD_OVERLAY"

var buildOverlay = os.Getenv(BuildOverlayEnv)

// build: root에서 cmd/<name>을 out으로(tags가 비면 태그 없이, buildOverlay가 있으면 -overlay)
func build(root, out, name, tags string) error {
	args := []string{"build"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	if buildOverlay != "" {
		if !filepath.IsAbs(buildOverlay) { // go build는 작업 폴더(root) 기준으로 읽어 시험의 폴더와 어긋난다
			return fmt.Errorf("%s=%s: 절대 경로여야 한다", BuildOverlayEnv, buildOverlay)
		}
		args = append(args, "-overlay", buildOverlay)
	}
	r := Cmd{Dir: root, Env: []string{"CGO_ENABLED=0", "GOFLAGS=", "GOOS=", "GOARCH="}, Combined: true, Max: 10 * time.Minute}.
		Run("go", append(args, "-o", out, "./cmd/"+name)...)
	if r.RC != 0 {
		return fmt.Errorf("go build ./cmd/%s: 종료 %d\n%s%s", name, r.RC, r.Out, r.Err)
	}
	return nil
}

// sweepBinDirs: (binMu 안) 주인 시험 프로세스가 없는(ESRCH) herdr-test-bin.<pid>.* 폴더를 지운다. 이름에 pid가 없으면(옛 꼴) 모름 — 둔다
func sweepBinDirs() {
	ms, _ := filepath.Glob(filepath.Join(os.TempDir(), "herdr-test-bin.*.*"))
	for _, m := range ms {
		f := strings.Split(filepath.Base(m), ".")
		pid, err := strconv.Atoi(f[1])
		if len(f) == 3 && err == nil && pid > 1 && pid != os.Getpid() && !proc.Alive(pid) {
			os.RemoveAll(m) //nolint:errcheck // 남은 시험 빌드 — 못 지우면 다음에
		}
	}
}

// Main: 패키지 TestMain에서 m.Run 대신 부른다 — 시험이 끝나면 Bin 폴더를 지우고 그 종료 코드로 끝난다
func Main(m *testing.M) {
	rc := m.Run()
	binMu.Lock()
	if binDir != "" {
		os.RemoveAll(binDir) //nolint:errcheck // 시험 빌드 폴더
	}
	binMu.Unlock()
	os.Exit(rc)
}

// BinDir: 그 도구들을 모두 지은 폴더(PATH 앞에 둔다)
func BinDir(t *testing.T, names ...string) string {
	t.Helper()
	if len(names) == 0 {
		t.Fatal("BinDir: 도구가 없다")
	}
	var p string
	for _, n := range names {
		p = Bin(t, n)
	}
	return filepath.Dir(p)
}

// EnvAllow: 시험이 바깥에서 받는 값 — 허용 목록(2026-11-24 test-env-allowlist). 실행 경로·임시 폴더(검증 실행기의 메모리 디스크
// TMPDIR과 부팅 볼륨 HERDR_VERIFY_BOOT_TMPDIR)·사용자·셸·터미널·언어·시간대만 받고 나머지는 버린다. 옛 지우기 목록 여섯 벌(18~39개)은
// 레인 터미널·병합기의 HC_*·HERDR_*·LANE_*·XDG_*·도구 손잡이를 저마다 다르게 빠뜨려 바깥 값이 새어 들었다.
// 꾸러미가 더 받아야 하는 값은 Env·Isolate의 keep으로 드러나게 넘긴다(docs/repo/tests.md 「시험의 환경」)
var EnvAllow = []string{"PATH", "TMPDIR", "HERDR_VERIFY_BOOT_TMPDIR", "USER", "LOGNAME", "SHELL", "TERM", "LANG", "LC_ALL", "LC_CTYPE", "TZ",
	// 시험 바이너리가 스스로 넣는 값: internal/journal·internal/oplog의 init이 지정이 없으면 /dev/null로 넣어, 시험이 띄운 도구가 실제
	// 일지·운영 기록에 쌓지 않게 자식에 물려준다(이름은 journal.Env·oplog.Env — 그 꾸러미를 가져오면 모든 시험 바이너리에서 init이 돈다)
	"HERDR_TOOLS_JOURNAL", "HERDR_TOOLS_OPLOG",
	// 격리한 시험 프로세스의 표식(internal/clitest/sandbox Marker — 가져오면 그 init이 돌아 tests/repo까지 격리되니 이름만): 틀이 다시 부른
	// 시험 바이너리가 부모가 넣은 HOME 그대로 돈다
	"HERDR_SANDBOX_HOME"}

// Env: 시험 자식 환경의 바탕 — 허용 목록과 keep의 바깥 값, 임시 HOME(T), GIT_CEILING_DIRECTORIES(git이 T 위로 저장소를 찾아
// 올라가지 못하게). 시험 틀이 이 위에 자기 값(가짜 명령 PATH·git 이름 등)을 덧씌운다
func Env(T string, keep ...string) map[string]string {
	e := map[string]string{}
	for _, k := range append(append([]string(nil), EnvAllow...), keep...) {
		if v, ok := os.LookupEnv(k); ok {
			e[k] = v
		}
	}
	e["HOME"] = T
	e["GIT_CEILING_DIRECTORIES"] = T + ":" + filepath.Dir(T)
	return e
}

// Isolate: 시험 프로세스 자신의 환경을 허용 목록으로 — os.Environ()을 물려주는 Cmd·Git의 자식도 같아진다. 허용 목록·keep·HOME·
// PWD(go test가 꾸러미 폴더로 넣는다)와 go 도구 값(GO*·CGO_* — 이 프로세스가 Bin으로 짓고 go test를 부른다) 말고는 지우고, 바깥 병합기·후보 잠금(fd 19·18)이 자식에
// 물리지 않게 하고, git이 T 위로 저장소를 찾아 올라가지 못하게 한다. 환경은 t.Setenv라 시험이 끝나면 되돌아간다
func Isolate(t *testing.T, T string, keep ...string) {
	t.Helper()
	syscall.CloseOnExec(18)
	syscall.CloseOnExec(19)
	allow := map[string]bool{"HOME": true, "PWD": true}
	for _, k := range append(append([]string(nil), EnvAllow...), keep...) {
		allow[k] = true
	}
	for _, kv := range os.Environ() {
		k := key(kv)
		if k != "" && !allow[k] && !strings.HasPrefix(k, "GO") && !strings.HasPrefix(k, "CGO_") {
			Unsetenv(t, k)
		}
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", T+":"+filepath.Dir(T))
}

// Unsetenv: 시험 동안만 지운다(끝나면 되돌림)
func Unsetenv(t *testing.T, k string) {
	t.Helper()
	t.Setenv(k, "")
	os.Unsetenv(k)
}

// TempDir: 실제 경로(pwd -P)인 임시 폴더 — macOS /var → /private/var. 셸 시험의 mktemp -d + cd -P
func TempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// DefaultMax: Cmd.Max가 0일 때 명령 하나의 상한 — go test 기본 마감(10분)보다 짧게
var DefaultMax = 60 * time.Second

// Cmd: 한 번 부르기. Env는 os.Environ() 위에 덧씌운다("K=" 빈 값, "K" 혼자는 지움) — Exact면 Env가 환경 전부
type Cmd struct {
	Dir      string
	Env      []string
	Exact    bool // Env만(바깥 환경을 물려받지 않는다)
	Stdin    string
	Combined bool          // 2>&1
	Max      time.Duration // 명령 하나의 상한(0이면 DefaultMax). 시험 마감(-test.timeout)이 더 이르면 그 안으로 줄인다
}

// Res: 셸 $(…)처럼 Out은 끝 줄바꿈을 뺀 표준 출력(Combined면 둘 다), Err는 표준 오류 그대로
type Res struct {
	Out     string
	Err     string
	RC      int
	Timeout bool // 상한을 넘겨(또는 끝난 뒤 출력 관이 상한만큼 안 닫혀) 무리째 껐다 — RC 124
}

// Run: 부르고 결과. 못 띄우면 RC 127. 명령은 새 프로세스 무리로 띄우고(Child), 상한을 넘기면 그 무리만 TERM → 유예 → KILL하고
// 거둔 뒤 RC 124·Timeout. 상한·유예·KILL 기다림은 모두 시험 마감 안에 든다 — 남은 시간이 회수 여유(minRemain)보다 짧으면
// 띄우지 않고 RC 127(재감사 base R-223: TERM을 무시하는 자식도 마감 전에 KILL·회수)
func (c Cmd) Run(name string, args ...string) Res {
	max := c.Max
	if max <= 0 {
		max = DefaultMax
	}
	end, grace, kw, ok := window(max)
	if !ok {
		return Res{RC: 127, Err: fmt.Sprintf("clitest: 시험 마감까지 남은 시간이 회수 여유(%s)보다 짧아 띄우지 않음: %s\n", minRemain, name)}
	}
	lim := time.Until(end)
	cmd := exec.Command(name, args...)
	cmd.Dir = c.Dir
	if c.Exact {
		cmd.Env = MergeEnv(nil, c.Env)
	} else {
		cmd.Env = MergeEnv(os.Environ(), c.Env)
	}
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	var o, e bytes.Buffer
	cmd.Stdout = &o
	if c.Combined {
		cmd.Stderr = &o
	} else {
		cmd.Stderr = &e
	}
	ch, err := start(cmd)
	if err != nil {
		return Res{RC: 127, Err: err.Error() + "\n"}
	}
	timeout := !ch.waitDone(time.Until(end))
	ioWait := time.Until(end)
	if timeout {
		ch.stop(grace, kw)
		ioWait = kw
	}
	select {
	case <-ch.ioDone:
	case <-time.After(ioWait):
		if !timeout {
			// 우두머리는 끝났는데 출력 관을 쥔 손자가 남았다 — 관을 쥔 것이 무리에 있으면 그것이 무리 번호를 쥐고 있다
			timeout = true
			sigSent.Add(2)
			killGroup(ch.Pid, syscall.SIGTERM) //nolint:errcheck // 비었으면 ESRCH
			select {
			case <-ch.ioDone:
			case <-time.After(grace):
			}
			killGroup(ch.Pid, syscall.SIGKILL) //nolint:errcheck // 비었으면 ESRCH
		}
		ch.closeIO()
	}
	if timeout {
		e.WriteString(fmt.Sprintf("clitest: 시간 넘김(%s) — 무리를 껐다: %s\n", lim.Round(time.Millisecond), name))
		return Res{Out: strings.TrimRight(o.String(), "\n"), Err: e.String(), RC: 124, Timeout: true}
	}
	return Res{Out: strings.TrimRight(o.String(), "\n"), Err: e.String(), RC: ch.rc}
}

// MergeEnv: base 위에 over를 덧씌운다. "K"(= 없음)는 지운다
func MergeEnv(base, over []string) []string {
	m := map[string]int{}
	out := append([]string(nil), base...)
	for i, kv := range out {
		m[key(kv)] = i
	}
	for _, kv := range over {
		k := key(kv)
		i, ok := m[k]
		switch {
		case !strings.Contains(kv, "=") && ok:
			out[i] = ""
		case !strings.Contains(kv, "="):
		case ok:
			out[i] = kv
		default:
			m[k] = len(out)
			out = append(out, kv)
		}
	}
	r := out[:0]
	for _, kv := range out {
		if kv != "" {
			r = append(r, kv)
		}
	}
	return r
}

func key(kv string) string { k, _, _ := strings.Cut(kv, "="); return k }

// Lines: 줄 목록(빈 글은 없음)
func Lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// HasLine: grep -Fxq
func HasLine(s, line string) bool {
	for _, l := range Lines(s) {
		if l == line {
			return true
		}
	}
	return false
}

// HasPrefixLine: grep -q "^<p>"
func HasPrefixLine(s, p string) bool {
	for _, l := range Lines(s) {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// LineNo: 처음 그 앞글자로 시작하는 줄 번호(1부터, 없으면 0) — grep -n "^p" | cut -d: -f1
func LineNo(s, p string) int {
	for i, l := range Lines(s) {
		if strings.HasPrefix(l, p) {
			return i + 1
		}
	}
	return 0
}

// CountNonEmpty: grep -c .
func CountNonEmpty(s string) int {
	n := 0
	for _, l := range Lines(s) {
		if l != "" {
			n++
		}
	}
	return n
}

// NewlineCount: wc -l
func NewlineCount(s string) int { return strings.Count(s, "\n") }

// Put: mkdir -p 뒤 쓴다(셸 put)
func Put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Append: >> 덧붙임
func Append(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// Chmod: 실패면 시험 중단
func Chmod(t *testing.T, path string, m fs.FileMode) {
	t.Helper()
	if err := os.Chmod(path, m); err != nil {
		t.Fatal(err)
	}
}

// Read: cat (없으면 빈 글)
func Read(path string) string { b, _ := os.ReadFile(path); return string(b) }

// Is: [ "$(cat f)" = "$(printf %b want)" ] — 양쪽 끝 줄바꿈을 뺀 비교
func Is(path, want string) bool {
	return strings.TrimRight(Read(path), "\n") == strings.TrimRight(want, "\n")
}

// Exists: [ -e ] (링크는 대상을 따른다)
func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Lexists: [ -e ] || [ -L ]
func Lexists(p string) bool { _, err := os.Lstat(p); return err == nil }

// IsLink: [ -L ]
func IsLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&fs.ModeSymlink != 0
}

// IsDir: [ -d ]
func IsDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

// IsExec: [ -x ] (일반 파일)
func IsExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

// Readlink: readlink (없으면 빈 글)
func Readlink(p string) string { s, _ := os.Readlink(p); return s }

// Empty: [ -z "$(ls -A d)" ] — 없는 폴더도 빈 것
func Empty(d string) bool { es, _ := os.ReadDir(d); return len(es) == 0 }

// Glob1: 셸의 "$(echo d/*/x)" — 하나면 그 경로, 없으면 패턴 그대로
func Glob1(pat string) string {
	m, _ := filepath.Glob(pat)
	if len(m) == 1 {
		return m[0]
	}
	if len(m) == 0 {
		return pat
	}
	return strings.Join(m, " ")
}

// FindName: find root -name <이름> 이 있는지(이름 글롭)
func FindName(root, glob string) bool {
	found := false
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ok, _ := filepath.Match(glob, d.Name()); ok && p != root {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// GrepRx: grep -rqx <줄> root — 아래 파일 가운데 그 줄이 통째로 있는 것
func GrepRx(root, line string) bool {
	found := false
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found || !d.Type().IsRegular() {
			return nil
		}
		if HasLine(Read(p), line) {
			found = true
		}
		return nil
	})
	return found
}

// Snap: (cd base && find <폴더…> -type f ! -name <빼기> -exec cksum {} + | sort) 와 같은 판정 — 경로·내용 CRC·크기 목록
func Snap(base string, skipName string, dirs ...string) string {
	var rows []string
	for _, d := range dirs {
		filepath.WalkDir(filepath.Join(base, d), func(p string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() || e.Name() == skipName {
				return nil
			}
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(base, p)
			rows = append(rows, fmt.Sprintf("%d %d %s", crc32.ChecksumIEEE(b), len(b), rel))
			return nil
		})
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// Git: git -C dir … 의 표준 출력(끝 줄바꿈 뺌). 실패해도 출력만(셸 $(git …)처럼)
func Git(dir string, args ...string) string {
	return Cmd{}.Run("git", append([]string{"-C", dir}, args...)...).Out
}

// GitOK: git -C dir … 이 0으로 끝났는지
func GitOK(dir string, args ...string) bool {
	return Cmd{}.Run("git", append([]string{"-C", dir}, args...)...).RC == 0
}

// MustGit: 실패면 시험 중단(준비 단계)
func MustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	r := Cmd{Combined: true}.Run("git", append([]string{"-C", dir}, args...)...)
	if r.RC != 0 {
		t.Fatalf("git -C %s %s: %d %s", dir, strings.Join(args, " "), r.RC, r.Out)
	}
	return r.Out
}

// Ck: herdr-settings-merge base 값 — printf '%s\n' "$(jq -cS . <<<json)" | cksum | awk '{print $1, $2}'
func Ck(canon string) string { return cksum.Space([]byte(canon + "\n")) }
