//go:build unix

package main

// 안전 시험(2026-11-22 gofix-ops — Go 전환 감사 A10·C06·C08·B06·B07·B19): 계약 시험(TestContract, 두 판)과 따로 둔다.
// 두 판이 같아야 하는 것은 ct.Impl 판으로, Go 판만 고친 것(셸 판은 「알려진 차이」 — 곧 걷음)은 Go 판으로 못박아 돈다.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/lockx"
)

type mergeEnv struct {
	t           *testing.T
	merge, keys string
	H           string
	impl        string
}

// newMerge: 새 임시 HOME. impl ""이면 ct.Impl 판
func newMerge(t *testing.T, impl string) *mergeEnv {
	t.Helper()
	if impl == "" {
		impl = ct.Impl("HERDR_SETTINGS_MERGE_IMPL")
	}
	return &mergeEnv{t: t, merge: ct.Tool(t, "herdr-settings-merge", "HERDR_SETTINGS_MERGE"),
		keys: filepath.Join(ct.Repo(t), "config.example/settings/keys.json"), H: ct.TempDir(t) + "/home", impl: impl}
}

func (e *mergeEnv) env() []string {
	return []string{"HOME=" + e.H, "HERDR_MACHINE=w1", "HERDR_KEYS=" + e.keys, "HERDR_SETTINGS_MERGE_IMPL=" + e.impl}
}

func (e *mergeEnv) S() string    { return e.H + "/.claude/settings.json" }
func (e *mergeEnv) ST() string   { return e.H + "/.local/state/herdr-sync" }
func (e *mergeEnv) Base() string { return e.ST() + "/settings-base.json" }

// run: sh <포장> 인자… (umask가 있으면 그 umask로)
func (e *mergeEnv) run(in, umask string, args ...string) ct.Res {
	if umask == "" {
		return ct.Cmd{Stdin: in, Env: e.env()}.Run("sh", append([]string{e.merge}, args...)...)
	}
	return ct.Cmd{Stdin: in, Env: e.env()}.Run("sh", append([]string{"-c", `umask ` + umask + `; exec sh "$0" "$@"`, e.merge}, args...)...)
}

func (e *mergeEnv) baseOf(kv ...string) string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i+1 < len(kv); i += 2 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(js(kv[i]) + ":" + js(ckj(kv[i+1])))
	}
	return b.String() + "}\n"
}

func mode(p string) os.FileMode {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Mode().Perm()
}

// A10: 표준 입력을 기다리는 동안 공개 진입점(포장) PID만 SIGKILL하고 그 뒤에 입력을 보내도 아무것도 쓰지 않는다 —
// 판을 고른 뒤 그 PID가 병합기 자체여야 한다(예전: bash -c 자식이 Go 판을 띄워 래퍼만 죽고 자식이 나중에 설정을 썼다)
func TestWrapperKilledBeforeInput(t *testing.T) {
	e := newMerge(t, "")
	ct.Put(t, e.S(), `{"model":"old"}`+"\n")
	if r := e.run(`{"values":{}}`, ""); r.RC != 0 { // 미리 한 번: 바이너리 짓기를 끝내 둔다
		t.Fatalf("준비 실행 실패 %d: %s", r.RC, r.Err)
	}
	cmd := exec.Command("sh", e.merge, "--take", ".model")
	cmd.Env = ct.MergeEnv(os.Environ(), e.env())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	w, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pg := cmd.Process.Pid
	defer syscall.Kill(-pg, syscall.SIGKILL) // 시험이 띄운 무리만
	// 판을 고를 때까지: 그 무리에 Go 판 바이너리(이름 herdr-settings-merge)나 셸 판의 cat(표준 입력 받기)이 보이면
	ready := false
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end) && !ready; time.Sleep(50 * time.Millisecond) {
		out, _ := exec.Command("/bin/ps", "-axo", "pgid=,comm=").Output()
		for _, l := range strings.Split(string(out), "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && f[0] == strconv.Itoa(pg) {
				b := filepath.Base(strings.Join(f[1:], " "))
				ready = ready || b == "herdr-settings-merge" || b == "cat"
			}
		}
	}
	if !ready {
		t.Fatal("30초 안에 병합기가 입력을 기다리는 데까지 가지 않았다")
	}
	syscall.Kill(pg, syscall.SIGKILL) // 포장 PID만
	w.Write([]byte(`{"values":{".model":"new"}}`))
	w.Close()
	cmd.Wait()
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end) && syscall.Kill(-pg, 0) == nil; time.Sleep(50 * time.Millisecond) {
	}
	if got := ct.Read(e.S()); got != `{"model":"old"}`+"\n" {
		t.Errorf("죽인 병합기가 나중에 설정을 썼다: %q", got)
	}
	if ct.Exists(e.Base()) && strings.Contains(ct.Read(e.Base()), ".model") {
		t.Errorf("죽인 병합기가 base를 썼다: %s", ct.Read(e.Base()))
	}
}

// C06·O-09: 판별 글자에 맥 HOME이 들어 HOME 바꾸기 뒤 사라지면 panic 없이 종료 2, 아무것도 쓰지 않는다(Go 판 — 셸 판은 hook:…:null을 쓴다)
func TestHookMarkerLostAfterHome(t *testing.T) {
	e := newMerge(t, "go")
	e.keys = ct.TempDir(t) + "/keys.json"
	ct.Put(t, e.keys, `{"values":[".model"],"hooks":{"PreToolUse":["/Users/mac/.claude/hooks/x.sh"]}}`+"\n")
	ct.Put(t, e.S(), `{"model":"a"}`+"\n")
	r := e.run(`{"home":"/Users/mac","values":{},"hooks":[{"event":"PreToolUse","matcher":"A","command":"/Users/mac/.claude/hooks/x.sh"}]}`, "")
	if r.RC != 2 || r.Out != "" || ct.NewlineCount(r.Err) != 1 || strings.Contains(r.Err, "panic") {
		t.Errorf("종료 2·출력 없음·오류 한 줄이어야: rc=%d out=%q err=%q", r.RC, r.Out, r.Err)
	}
	if ct.Read(e.S()) != `{"model":"a"}`+"\n" || ct.Exists(e.Base()) {
		t.Errorf("아무것도 쓰지 않아야: settings=%q base=%v", ct.Read(e.S()), ct.Exists(e.Base()))
	}
}

// C08: umask 077에서 새 base는 0600(셸과 같음), 이미 있는 base의 좁은 권한은 umask 022에서도 그대로(Go 판)
func TestBaseMode(t *testing.T) {
	e := newMerge(t, "")
	ct.Put(t, e.S(), `{"language":"한국어"}`+"\n")
	r := e.run(`{"values":{".language":"한국어"}}`, "077")
	if r.RC != 0 || mode(e.Base()) != 0o600 {
		t.Errorf("umask 077의 새 base는 0600이어야: rc=%d mode=%o err=%s", r.RC, mode(e.Base()), r.Err)
	}

	e = newMerge(t, "go")
	ct.Put(t, e.S(), `{"model":"old"}`+"\n")
	ct.Put(t, e.Base(), e.baseOf(".model", `"old"`))
	ct.Chmod(t, e.Base(), 0o600)
	r = e.run(`{"values":{".model":"new"}}`, "022")
	if r.RC != 0 || r.Out != "씀 .model" || mode(e.Base()) != 0o600 {
		t.Errorf("이미 있는 base의 0600은 그대로여야: rc=%d out=%q mode=%o", r.RC, r.Out, mode(e.Base()))
	}
}

// B06: 같은 HOME의 병합은 잠금 안에서 설정·base를 읽고 쓴다 — 남이 잠금을 쥔 동안 쓰지 않고, 풀린 뒤 남이 쓴 키를 살린다(Go 판)
func TestConcurrentMergeLock(t *testing.T) {
	e := newMerge(t, "go")
	ct.Put(t, e.S(), `{"language":"old","model":"old"}`+"\n")
	ct.Put(t, e.Base(), e.baseOf(".language", `"old"`, ".model", `"old"`))
	if r := e.run(`{"values":{}}`, ""); r.RC != 0 { // 미리 한 번(바이너리 짓기)
		t.Fatalf("준비 실행 실패 %d: %s", r.RC, r.Err)
	}
	lk, err := lockx.Lock(e.ST() + "/" + lockName)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan ct.Res, 1)
	go func() { done <- e.run(`{"values":{".model":"new"}}`, "") }()
	time.Sleep(1500 * time.Millisecond)
	// 잠금을 쥔 다른 병합기: 잠금 안에서 읽은 판에 .language만 바꿔 쓴다
	ct.Put(t, e.S(), `{"language":"new","model":"old"}`+"\n")
	ct.Put(t, e.Base(), e.baseOf(".language", `"new"`, ".model", `"old"`))
	lk.Close()
	var r ct.Res
	select {
	case r = <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("잠금이 풀린 뒤에도 병합기가 끝나지 않았다")
	}
	st := canon(ct.Read(e.S()))
	if r.RC != 0 || r.Out != "씀 .model" || st != canon(`{"language":"new","model":"new"}`) {
		t.Errorf("두 키가 다 남아야: rc=%d out=%q settings=%s err=%s", r.RC, r.Out, st, r.Err)
	}
	if b := canon(ct.Read(e.Base())); b != canon(e.baseOf(".language", `"new"`, ".model", `"new"`)) {
		t.Errorf("base에 두 키가 새 값으로 남아야: %s", b)
	}
}

// B07: settings.json이 (상대) 심볼릭 링크면 링크는 두고 최종 대상 곁에 새 파일을 써 rename — 대상을 제자리에서 자르지 않는다(Go 판)
func TestSymlinkReplacedByRename(t *testing.T) {
	e := newMerge(t, "go")
	real := e.H + "/target/real.json"
	ct.Put(t, real, `{"model":"old"}`+"\n")
	os.MkdirAll(e.H+"/.claude", 0o755)
	if err := os.Symlink("../target/real.json", e.S()); err != nil {
		t.Fatal(err)
	}
	var before syscall.Stat_t
	syscall.Stat(real, &before)
	r := e.run(`{"values":{".model":"new"}}`, "", "--take", ".model")
	var after syscall.Stat_t
	syscall.Stat(real, &after)
	if r.RC != 0 || !ct.IsLink(e.S()) || ct.Readlink(e.S()) != "../target/real.json" || canon(ct.Read(real)) != canon(`{"model":"new"}`) {
		t.Errorf("링크를 두고 대상에 새 값: rc=%d link=%q real=%q err=%s", r.RC, ct.Readlink(e.S()), ct.Read(real), r.Err)
	}
	if after.Ino == before.Ino {
		t.Errorf("대상 파일을 제자리에서 잘라 썼다(같은 inode %d) — 새 파일을 rename해야", after.Ino)
	}
	if n := dirNames(e.H + "/target"); n != "real.json" {
		t.Errorf("대상 폴더에 임시 파일이 남았다: %s", n)
	}
}

// B19: base를 읽지 못하거나(권한) 깨졌으면 쓰기 전에 종료 1 — 빈 장부로 시작해 다른 키 이력을 지우지 않는다(Go 판)
func TestBaseUnreadable(t *testing.T) {
	for _, c := range []struct{ name, base string }{{"권한", ""}, {"깨진 JSON", "{\".model\": \n"}} {
		e := newMerge(t, "go")
		ct.Put(t, e.S(), `{"model":"x"}`+"\n")
		want := c.base
		if want == "" {
			want = e.baseOf(".language", `"한국어"`, ".model", `"x"`)
		}
		ct.Put(t, e.Base(), want)
		if c.base == "" {
			ct.Chmod(t, e.Base(), 0)
		}
		r := e.run(`{"values":{".model":"x"}}`, "")
		os.Chmod(e.Base(), 0o600)
		if r.RC != 1 || r.Out != "" || ct.NewlineCount(r.Err) != 1 {
			t.Errorf("%s: 종료 1·출력 없음·오류 한 줄이어야: rc=%d out=%q err=%q", c.name, r.RC, r.Out, r.Err)
		}
		if got := ct.Read(e.Base()); got != want {
			t.Errorf("%s: base가 바뀌었다: %q", c.name, got)
		}
	}
}
