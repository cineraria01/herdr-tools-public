// Package hclane_test: hc-lane의 계약 시험(tests/hc-lane.test.sh를 옮긴 것). 임시 HOME·가짜 원격(bare origin)·가짜 hc를 만들고
// hc-lane Go 바이너리(clitest.Bin — 곁에 hc-merger·herdr-tools-config도 지음)를 밖에서 exec해 출력·종료 코드·상태(작업 트리·가지·
// 원격 참조·사건 파일)를 본다. 단언 이름·차례는 옛 셸 시험 그대로다. 판은 Go 하나다(2026-11-22 go-shell-remove).
//
// 가짜 hc·ps·herdr-app은 testdata/의 셸 스크립트(셸 시험 here-doc 그대로) — 시험 대상이 아니라 시험 대역이라 바이트를 맞췄다.
// 실행: go test ./tests/hclane/ (저장소 루트에서)
package hclane_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

// laneBin: 시험할 hc-lane — 이 시험 프로세스에서 지은 Go 바이너리. 곁 도구(병합기 hc-merger·설정 herdr-tools-config)도 같은 폴더에
func laneBin(t *testing.T) string {
	t.Helper()
	return filepath.Join(clitest.BinDir(t, "hc-lane", "hc-merger", "herdr-tools-config"), "hc-lane")
}

// K: 시험 상태. 셸 시험의 전역 변수(T·W·D·R·B·out·rc·HCLOG)와 export한 환경.
type K struct {
	t     *testing.T
	T, W  string
	LANE  string
	HCLOG string
	D, R  string
	B     string
	out   string
	rc    int
	env   map[string]string
	n     int
	fails int
	wt    string // 셸 $wt: land·mklane이 정한 작업 트리
}

// withK: 새 K로 f를 돈다.
func withK(t *testing.T, f func(k *K)) {
	k := newK(t)
	f(k)
	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

func newK(t *testing.T) *K {
	T, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	k := &K{t: t, T: T, W: filepath.Join(T, "공백 있는 폴더"), HCLOG: filepath.Join(T, "hc.log"), env: clitest.Env(T)}
	k.LANE = laneBin(t)
	bin := filepath.Join(T, "bin")
	mkdirs(t, bin, k.W)
	for _, f := range []string{"hc", "ps", "herdr-app"} {
		b, err := os.ReadFile(filepath.Join("testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, f), b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(k.LANE, filepath.Join(bin, "hc-lane")); err != nil { // 착지 절차의 hc-lane clean이 이 사본을 쓰게
		t.Fatal(err)
	}
	k.set("PATH="+bin+":"+k.env["PATH"], "HCLOG="+k.HCLOG, "HC_LANE_POLL=0.1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1",
		"HOME="+T, "GIT_CEILING_DIRECTORIES="+T+":"+filepath.Dir(T))
	write(t, filepath.Join(T, ".zshrc"), "export HL_ZSHRC=read\ncd /\n") // --on 설정 단계만 읽어야 하고, 안의 cd가 setup 위치를 바꾸면 안 된다
	write(t, filepath.Join(T, ".zprofile"), "export HL_ZPROFILE=read\n") // --on은 로그인 셸(.zprofile의 brew shellenv 등)도
	return k
}

// set·unset: 셸 시험의 export·unset(뒤 명령 모두에 먹는다).
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

// environ: 바깥 export 위에 이 명령에만 먹는 VAR=값(셸의 `VAR=값 명령`).
func (k *K) environ(extra []string) []string {
	m := map[string]string{}
	for key, v := range k.env {
		m[key] = v
	}
	for _, s := range extra {
		key, v, _ := strings.Cut(s, "=")
		m[key] = v
	}
	l := make([]string, 0, len(m))
	for key, v := range m {
		l = append(l, key+"="+v)
	}
	return l
}

// check: 셸 시험의 t. 번호·이름을 「ok n - 이름」/「FAIL n - 이름」 꼴로.
func (k *K) check(name string, ok bool) {
	k.t.Helper()
	k.n++
	if ok {
		k.t.Logf("ok %d - %s", k.n, name)
		return
	}
	k.fails++
	k.t.Errorf("FAIL %d - %s\nstdout:\n%s\nstderr:\n%s", k.n, name, k.out, read(filepath.Join(k.T, "err")))
}

// exe: dir에서 이름을 돌려 stdout(끝 줄바꿈 뗌 — 셸 $(…) 꼴)·종료 코드. stderr는 errPath(빈 값이면 버림).
func (k *K) exe(dir, errPath string, extra []string, name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, k.environ(extra)
	cmd.WaitDelay = 30 * time.Second // 출력을 물고 남은 손자가 있어도 끝나게
	if errPath != "" {
		f, err := os.Create(errPath)
		if err != nil {
			k.t.Fatal(err)
		}
		defer f.Close()
		cmd.Stderr = f
	}
	b, err := cmd.Output()
	o := strings.TrimRight(string(b), "\n")
	if err == nil {
		return o, 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() >= 0 {
		return o, ee.ExitCode()
	}
	return o + err.Error(), -1
}

// run: 셸 시험의 run(out·rc, stderr는 $T/err). 작업 폴더는 T.
func (k *K) run(args ...string) { k.runIn(k.T, nil, args...) }

// runE: `VAR=값 run …`.
func (k *K) runE(extra []string, args ...string) { k.runIn(k.T, extra, args...) }

// runIn: `out=$(cd dir && VAR=값 "$LANE" …)`.
func (k *K) runIn(dir string, extra []string, args ...string) {
	k.out, k.rc = k.exe(dir, filepath.Join(k.T, "err"), extra, k.LANE, args...)
}

// git: git -C dir … 의 stdout(끝 줄바꿈 뗌). 실패해도 출력만(셸 $(git …) 꼴).
func (k *K) git(dir string, args ...string) string {
	o, _ := k.exe(k.T, "", nil, "git", append([]string{"-C", dir}, args...)...)
	return o
}

// gitOK: git -C dir … 이 0으로 끝났는지(-q show-ref 등).
func (k *K) gitOK(dir string, args ...string) bool {
	_, rc := k.exe(k.T, "", nil, "git", append([]string{"-C", dir}, args...)...)
	return rc == 0
}

// sh: 작업 폴더 dir에서 셸 한 줄(시험 준비 — 시드 커밋·push 등 셸 시험의 (cd … && …) 묶음). 실패면 시험을 멈춘다.
func (k *K) sh(dir, script string, args ...string) {
	k.t.Helper()
	if o, rc := k.exe(dir, "", nil, "bash", append([]string{"-c", script, "sh"}, args...)...); rc != 0 {
		k.t.Fatalf("준비 실패(rc %d): %s\n%s", rc, script, o)
	}
}

// mk <이름> [기본 브랜치]: origin.git + clone. clone 뒤 origin에 커밋을 하나 더 올려 fetch가 필요한 상태로 둔다.
func (k *K) mk(name string, base ...string) {
	k.t.Helper()
	k.D, k.B = filepath.Join(k.W, name), "main"
	if len(base) > 0 {
		k.B = base[0]
	}
	for _, s := range []string{"", ".exec", ".keys"} {
		os.Remove(k.HCLOG + s)
	}
	k.sh(k.T, `git init -q --bare -b "$1" "$2/origin.git" && { git clone -q "$2/origin.git" "$2/seed" 2>/dev/null; true; }`, k.B, k.D)
	k.seed("a", "a")
	k.sh(k.T, `git clone -q "$1/origin.git" "$1/저장소 한글"`, k.D)
	k.seed("b", "b")
	k.R = filepath.Join(k.D, "저장소 한글")
}

// seed <파일> <내용>: seed 클론에서 커밋해 origin/<B>로 push.
func (k *K) seed(file, content string) {
	k.t.Helper()
	k.sh(filepath.Join(k.D, "seed"), `mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add "$1" && git commit -qm "$1" && git push -q origin "HEAD:$3"`, file, content, k.B)
}

// hclog: 가짜 hc가 적은 run 인자 줄들($HCLOG). 파일이 없으면 nil.
func (k *K) hclog(suffix string) []string {
	b, err := os.ReadFile(k.HCLOG + suffix)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// has: hc run이 받은 인자 한 줄(grep -Fx).
func (k *K) has(s string) bool { return k.line(s) > 0 }

// line: 그 줄의 첫 번호(1부터, 없으면 0 — grep -n -Fx | head -1).
func (k *K) line(s string) int {
	for i, l := range k.hclog("") {
		if l == s {
			return i + 1
		}
	}
	return 0
}

// pair: 옵션 다음 값(grep -Fx -A1 opt | grep -Fx val).
func (k *K) pair(opt, val string) bool {
	l := k.hclog("")
	for i, s := range l {
		if s == opt && i+1 < len(l) && l[i+1] == val {
			return true
		}
	}
	return false
}

// errf: $T/err 내용.
func (k *K) errf() string { return read(filepath.Join(k.T, "err")) }

// lastLine: 셸 printf "%s\n" "$out" | tail -n 1.
func lastLine(s string) string {
	i := strings.LastIndex(s, "\n")
	return s[i+1:]
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

func read(p string) string { b, _ := os.ReadFile(p); return string(b) }

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

// grep: 확장 정규식이 걸린 줄이 있는지(grep -E/-q).
func grep(s, re string) bool { return regexp.MustCompile("(?m)" + re).MatchString(s) }

// sq: 셸 작은따옴표 감싸기(출력에 '<경로>' 꼴로 나오는 것과 견줄 때).
func sq(s string) string { return "'" + s + "'" }
