// Package sandbox: 시험 바이너리의 집·환경 격리(2026-11-24 knob-test-leak). 시험 파일에서 빈 가져오기(`import _ "herdr-tools/internal/clitest/sandbox"`)로
// 들이면, 그 시험 바이너리는 시작할 때(가져온 꾸러미의 init — 시험하는 꾸러미의 꾸러미 변수보다 먼저) 한 번:
//
//   - 운영 값을 지운다: 도구가 설정으로 읽는 이름 공간 HC_·HERDR_·LANE_·XDG_·CLAUDE_·GIT_(Drop). 검증 실행기 값 HERDR_VERIFY_*와
//     시험 전용 이름(…_TEST…·…_EXAMPLE·…_CRASH_AT, 도구 덮어쓰기 Keep)은 남긴다. 병합기가 일꾼에게 넘긴 손잡이(HC_LANE_SPEC_IDLE_LOAD 등)가
//     저장소 검증의 go test까지 이어져 「한가함 문턱」을 기본값으로 보는 시험이 깨졌다.
//   - HOME을 새 임시 폴더로 돌린다(그 안에 git 이름만 둔 .gitconfig) — 실제 손잡이(~/.local/state/herdr-ops)·대기열(~/.local/share/hc-fill)·
//     일지·기계 쉬게 하기·부하·배포 기록을 읽지도 쓰지도 않는다. go의 캐시·모듈·설정(GOCACHE·GOPATH·GOMODCACHE·GOENV)은 HOME을 바꾸기
//     전 자리로 못박는다(Bin의 go build가 빈 캐시로 다시 짓지 않게).
//   - 일지·운영 기록은 /dev/null(internal/journal·oplog의 init과 같은 값 — 바깥에 실제 경로가 있어도).
//
// 다시 부른 자식 시험 바이너리(os.Args[0] — 환경을 물려받음)는 표식 Marker가 있어 아무것도 하지 않는다(부모 시험이 넣은 값 그대로).
// 시험 프로세스가 아니면(testing.Testing() 거짓) 아무것도 하지 않는다. 일부러 이 기계 값을 읽는 꾸러미(tests/repo — 개인 값 검사·
// 레인 표식)는 가져오지 않고, 그 목록은 tests/repo의 TestTestsSandboxed가 지킨다(docs/repo/tests.md 「시험의 집」).
//
// 이 꾸러미는 저장소의 다른 꾸러미를 가져오지 않는다(그 꾸러미의 시험이 이것을 가져오면 가져오기 고리가 된다).
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Marker: 격리한 시험 프로세스가 넣는 표식(값 = 임시 HOME). clitest.EnvAllow에 있어 시험 틀의 자식에도 간다
const Marker = "HERDR_SANDBOX_HOME" // 시험 손잡이 꼴(「_TEST」)이 아니다 — 그 꼴은 태그 파일에만(clitest TestTestKnobsListed)

const homePrefix = "herdr-test-home."

// dropPrefix: 도구가 설정·운영 값으로 읽는 이름 공간
var dropPrefix = []string{"HC_", "HERDR_", "LANE_", "XDG_", "CLAUDE_", "GIT_"}

// Keep: dropPrefix에 걸려도 남기는 시험 전용 이름(사람이 시험에 주는 덮어쓰기 — 도구는 읽지 않는다)
var Keep = map[string]bool{
	"HERDR_SYNC": true, "HERDR_SETTINGS_MERGE": true, "HERDR_TOOLS_PUBLISH": true, "HERDR_TOOLS_CONFIG_BIN": true, // clitest.Tool 덮어쓰기
	"HERDR_HOOKS_CORPUS": true, "HERDR_SCAN_ROOT": true,
	Marker: true, childEnv: true,
}

// Drop: 시험 바이너리가 바깥에서 받지 않는 이름인가
func Drop(k string) bool {
	if Keep[k] || strings.HasPrefix(k, "HERDR_VERIFY_") || strings.Contains(k, "_TEST") ||
		strings.HasSuffix(k, "_EXAMPLE") || strings.HasSuffix(k, "_CRASH_AT") {
		return false
	}
	for _, p := range dropPrefix {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// Home: 이 시험 프로세스의 임시 HOME(격리 안 했으면 빈 값)
func Home() string { return home }

var home string

func init() {
	if !testing.Testing() {
		return
	}
	if err := Enter(); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: 시험 격리 실패 — %v\n", err)
		os.Exit(2) // 격리 없이 돌면 실제 상태를 건드릴 수 있다 — 돌지 않는다
	}
}

// Enter: 이 프로세스를 격리한다(init이 부른다 — 시험이 직접 부를 일은 없다). 표식이 이미 맞으면 그대로
func Enter() error {
	if m := os.Getenv(Marker); m != "" && strings.HasPrefix(filepath.Base(m), homePrefix) {
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			home = m
			return nil
		}
	}
	pinGo()
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != "" && Drop(k) {
			os.Unsetenv(k)
		}
	}
	sweep()
	d, err := os.MkdirTemp("", fmt.Sprintf("%s%d.", homePrefix, os.Getpid()))
	if err != nil {
		return err
	}
	if d, err = filepath.EvalSymlinks(d); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(d, ".gitconfig"), []byte("[user]\n\tname = herdr-test\n\temail = herdr-test@example.invalid\n"), 0o644); err != nil {
		return err
	}
	home = d
	for k, v := range map[string]string{"HOME": d, Marker: d, "HERDR_TOOLS_JOURNAL": os.DevNull, "HERDR_TOOLS_OPLOG": os.DevNull} {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// childEnv: Check가 독을 넣어 다시 부른 자식의 표식(값 = 독 HOME). Keep에 있어 Drop되지 않는다(시험 손잡이 꼴 이름은
// 태그 파일에만 두는 규칙 — clitest TestTestKnobsListed — 이라 「_TEST」를 쓰지 않는다)
const childEnv = "HERDR_SANDBOX_POISON"

// Check: 꾸러미마다 TestSandboxed가 부른다 — ① 이 프로세스가 격리됐다 ② 독(실제 꼴의 손잡이·대기열·일지 파일을 둔 HOME,
// 운영 손잡이 환경 변수)을 넣고 표식 없이 이 시험 바이너리를 다시 불러도 자식이 그 HOME·값을 보지 않는다
func Check(t *testing.T) {
	t.Helper()
	if p := os.Getenv(childEnv); p != "" {
		if os.Getenv("HOME") == p || home == "" || os.Getenv("HOME") != home {
			t.Fatalf("독 HOME %s 을 그대로 본다(HOME=%s)", p, os.Getenv("HOME"))
		}
		checkEnv(t)
		return
	}
	if home == "" || os.Getenv("HOME") != home {
		t.Fatalf("격리되지 않았다: HOME=%s sandbox=%s", os.Getenv("HOME"), home)
	}
	checkEnv(t)
	p := t.TempDir()
	for f, s := range map[string]string{
		".local/state/herdr-ops/knobs.tsv": "# 독\nmerger_idle_load\t100\tpoison\tpoison\t1\t-\t1\n",
		".local/share/hc-fill/queue.tsv":   "poison\n",
		".config/herdr-tools/hc-fill.tsv":  "poison\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(p, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, f), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != Marker && k != "HOME" {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+p, childEnv+"="+p, "XDG_STATE_HOME="+p+"/.local/state", "XDG_CONFIG_HOME="+p+"/.config",
		"HC_LANE_SPEC_IDLE_LOAD=100", "HC_LANE_FRONT_LOCAL=0", "HC_FILL_DATA="+p+"/.local/share/hc-fill",
		"HERDR_TOOLS_CONFIG="+p+"/.config/herdr-tools", "HERDR_TOOLS_JOURNAL="+p+"/journal.jsonl", "HERDR_TOOLS_OPLOG="+p+"/ops.tsv",
		"HERDR_FUTURE_KNOB=poison", "LANE_REPO="+p, "GIT_DIR="+p)
	c := exec.Command(os.Args[0], "-test.run=^TestSandboxed$", "-test.count=1")
	c.Env = env
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("독을 넣고 다시 부른 시험 바이너리가 격리되지 않았다(%v):\n%s", err, out)
	}
}

func checkEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); Drop(k) && k != "HERDR_TOOLS_JOURNAL" && k != "HERDR_TOOLS_OPLOG" {
			t.Errorf("운영 값이 남았다: %s", kv)
		}
	}
	if os.Getenv("HERDR_TOOLS_JOURNAL") != os.DevNull || os.Getenv("HERDR_TOOLS_OPLOG") != os.DevNull {
		t.Errorf("일지·운영 기록이 /dev/null이 아니다: %q %q", os.Getenv("HERDR_TOOLS_JOURNAL"), os.Getenv("HERDR_TOOLS_OPLOG"))
	}
}

// pinGo: HOME에서 나오는 go 자리를 지금 값으로 못박는다(이미 환경에 있거나 GOENV 파일이 정했으면 그대로)
func pinGo() {
	cache, _ := os.UserCacheDir()
	conf, _ := os.UserConfigDir()
	h, _ := os.UserHomeDir()
	goenv := os.Getenv("GOENV")
	if goenv == "" && conf != "" {
		goenv = filepath.Join(conf, "go", "env")
		os.Setenv("GOENV", goenv)
	}
	b, _ := os.ReadFile(goenv)
	inFile := func(k string) bool { return strings.Contains("\n"+string(b), "\n"+k+"=") }
	gopath := os.Getenv("GOPATH")
	if gopath == "" && h != "" && !inFile("GOPATH") {
		gopath = filepath.Join(h, "go")
		os.Setenv("GOPATH", gopath)
	}
	if os.Getenv("GOMODCACHE") == "" && gopath != "" && !inFile("GOMODCACHE") {
		os.Setenv("GOMODCACHE", filepath.Join(strings.Split(gopath, string(os.PathListSeparator))[0], "pkg", "mod"))
	}
	if os.Getenv("GOCACHE") == "" && cache != "" && !inFile("GOCACHE") {
		os.Setenv("GOCACHE", filepath.Join(cache, "go-build"))
	}
}

// sweep: 주인 시험 프로세스가 없는 옛 임시 HOME(herdr-test-home.<pid>.*)을 지운다 — 시험 바이너리는 끝에 지울 곳이 없다
// ponytail: 다음 시험 바이너리가 치운다(남는 것은 빈 폴더와 작은 .gitconfig 정도). windows는 살아 있음을 못 봐 안 지운다
func sweep() {
	if runtime.GOOS == "windows" {
		return
	}
	ms, _ := filepath.Glob(filepath.Join(os.TempDir(), homePrefix+"*.*"))
	for _, m := range ms {
		f := strings.Split(strings.TrimPrefix(filepath.Base(m), homePrefix), ".")
		pid, err := strconv.Atoi(f[0])
		if len(f) != 2 || err != nil || pid <= 1 || pid == os.Getpid() || alive(pid) {
			continue
		}
		os.RemoveAll(m) //nolint:errcheck // 못 지우면 다음에
	}
}
