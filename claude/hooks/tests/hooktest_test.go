// Package tests: Claude Code PreToolUse 훅(herdr-hooks <훅>)의 계약 시험(옛 셸 시험 claude/hooks/tests/*.test.sh를 옮긴 것).
// 훅은 settings.json에 넣는 명령 줄 그대로(hookCmds — /bin/sh -c) 밖에서 부르고, PreToolUse JSON을 stdin으로 넣어 종료 코드·stderr를 본다.
// 명령 줄은 $HOME/.local/bin/herdr-hooks를 부르므로, 시험은 HOME마다 그 자리에 시험 빌드(-tags hookstest — 시험 손잡이
// HERDR_HOOKS_TEST_*가 먹는 빌드)를 걸어 둔다. 셸 판·두 판 대조는 2026-11-22 걷었다(go-shell-remove) — 판은 Go 하나다.
// 실행: go test ./claude/hooks/tests/
package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// hookCmds: settings.json PreToolUse 훅 항목의 command — 개인 settings에 이 글자 그대로 넣는다(herdr-sync가 판별 글자 = 훅 이름으로 짝짓는다).
// 바이너리 ~/.local/bin/herdr-hooks(herdr-sync가 원자 교체로 까는 Go 바이너리 그 자체)를 바로 exec한다 — 버전 확인·짓기·셸 판 없음(R-M02).
// 바이너리가 없거나 실행할 수 없으면: 안전 장치 cotenant-guard는 막고(2, 까닭 한 줄), 편의 막·알림 넷은 통과(0) — 옛 포장과 같은 쪽(R-H07).
// 실행 파일은 이 고정 자리 하나 — 환경 변수로 바꿔 끼우지 못한다(R-L01·R-41).
var hookCmds = map[string]string{
	"cotenant-guard":       `[ -x "$HOME/.local/bin/herdr-hooks" ] || { echo 'cotenant-guard: ~/.local/bin/herdr-hooks가 없어 판정하지 못해 막았다 — herdr-sync로 깔아라' >&2; exit 2; }; exec "$HOME/.local/bin/herdr-hooks" cotenant-guard`,
	"loop-no-question":     `[ -x "$HOME/.local/bin/herdr-hooks" ] || exit 0; exec "$HOME/.local/bin/herdr-hooks" loop-no-question`,
	"hc-lane-wait-guard":   `[ -x "$HOME/.local/bin/herdr-hooks" ] || exit 0; exec "$HOME/.local/bin/herdr-hooks" hc-lane-wait-guard`,
	"kill-by-lookup-guard": `[ -x "$HOME/.local/bin/herdr-hooks" ] || exit 0; exec "$HOME/.local/bin/herdr-hooks" kill-by-lookup-guard`,
	"lane-no-shell-edit":   `[ -x "$HOME/.local/bin/herdr-hooks" ] || exit 0; exec "$HOME/.local/bin/herdr-hooks" lane-no-shell-edit`,
}

var (
	hooksBinOnce sync.Once
	hooksBin     string
	hooksBinErr  error
	hooksBinDir  string
	homeOnce     sync.Once
	homeDefault  string
)

// buildHooks: 이 시험만의 herdr-hooks를 -tags hookstest로 한 번 짓는다(시험 손잡이 HERDR_HOOKS_TEST_*가 먹는 빌드).
// clitest.Bin은 태그 없이 짓는다 — 운영 빌드는 그쪽(settings_cmd_test.go)
func buildHooks(t *testing.T) string {
	t.Helper()
	hooksBinOnce.Do(func() {
		goBin, err := exec.LookPath("go")
		if err != nil {
			hooksBinErr = fmt.Errorf("go가 없다(brew install go): %w", err)
			return
		}
		root, err := filepath.Abs(filepath.Join("..", "..", ".."))
		if err != nil {
			hooksBinErr = fmt.Errorf("모듈 자리: %w", err)
			return
		}
		if hooksBinDir, err = os.MkdirTemp("", "herdr-hooks-bin."); err != nil {
			hooksBinErr = fmt.Errorf("임시 폴더: %w", err)
			return
		}
		hooksBin = filepath.Join(hooksBinDir, "herdr-hooks")
		cmd := exec.Command(goBin, "build", "-tags", "hookstest", "-trimpath", "-buildvcs=false", "-o", hooksBin, "./cmd/herdr-hooks")
		cmd.Dir = root
		cmd.Env = envWith(nil, "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOFLAGS=", "GOOS=", "GOARCH=")
		if out, err := cmd.CombinedOutput(); err != nil {
			hooksBinErr = fmt.Errorf("herdr-hooks 짓기 실패(%s): %w\n%s", root, err, out)
		}
	})
	if hooksBinErr != nil {
		t.Fatal(hooksBinErr)
	}
	return hooksBin
}

func TestMain(m *testing.M) {
	rc := m.Run()
	for _, d := range []string{hooksBinDir, homeDefault} {
		if d != "" {
			os.RemoveAll(d) //nolint:errcheck // 임시 폴더
		}
	}
	os.Exit(rc)
}

// putBin: home/.local/bin/herdr-hooks에 bin을 건다(이미 있으면 그대로)
func putBin(t *testing.T, home, bin string) {
	t.Helper()
	p := filepath.Join(home, ".local", "bin", "herdr-hooks")
	if _, err := os.Lstat(p); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin, p); err != nil {
		t.Fatal(err)
	}
}

// withHome: env의 HOME에 시험 빌드를 건다. HOME이 없거나 이 시험을 돌리는 사람의 진짜 HOME이면 시험 전용 HOME으로 바꾼다
// (진짜 ~/.local/bin/herdr-hooks를 부르지 않게)
func withHome(t *testing.T, env []string) []string {
	t.Helper()
	home, at := "", -1
	for i, kv := range env {
		if strings.HasPrefix(kv, "HOME=") {
			home, at = kv[5:], i
		}
	}
	if home == "" || home == os.Getenv("HOME") {
		homeOnce.Do(func() {
			d, err := os.MkdirTemp("", "herdr-hooks-home.")
			if err == nil {
				homeDefault = d
			}
		})
		if homeDefault == "" {
			t.Fatal("시험 HOME을 만들지 못했다")
		}
		home = homeDefault
		if at >= 0 {
			env = append(append([]string{}, env[:at]...), env[at+1:]...)
		}
		env = append(env, "HOME="+home)
	}
	putBin(t, home, buildHooks(t))
	return env
}

// hookExec: settings.json의 명령 줄(hookCmds[name])을 /bin/sh -c로 — Claude Code가 훅을 부르는 꼴
func hookExec(name string, stdin []byte, env []string, dir string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", hookCmds[name])
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	cmd.Dir = dir
	return cmd
}

// toolInput: jq -cn으로 만들던 PreToolUse 입력. HTML 이스케이프 없이(jq와 같은 바이트).
func toolInput(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}

// bashInput: {tool_name: "Bash", tool_input: {command: c}}
func bashInput(c string) []byte {
	return toolInput(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": c}})
}

// envWith: 바깥 환경에서 unset 이름을 빼고 set(K=V)을 덮는다. 옛 판 스위치(HERDR_IMPL·HERDR_HOOKS_IMPL·HERDR_HOOKS_BIN)와
// 시험 손잡이 HERDR_HOOKS_TEST_*는 늘 뺀다 — 바깥에 남은 값이 판정에 섞이지 않게.
func envWith(unset []string, set ...string) []string {
	drop := map[string]bool{"HERDR_IMPL": true, "HERDR_HOOKS_IMPL": true, "HERDR_HOOKS_BIN": true,
		"HERDR_HOOKS_TEST_GIT": true, "HERDR_HOOKS_TEST_NOW_OFFSET": true, "HERDR_HOOKS_TEST_SECONDS": true}
	for _, k := range unset {
		drop[k] = true
	}
	for _, kv := range set {
		drop[strings.SplitN(kv, "=", 2)[0]] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		if !drop[strings.SplitN(kv, "=", 2)[0]] {
			env = append(env, kv)
		}
	}
	return append(env, set...)
}

// runOut: 끝난 명령의 종료 코드(못 띄우면 t.Fatal)
func runOut(t *testing.T, cmd *exec.Cmd) (rc int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		rc = ee.ExitCode()
	default:
		t.Fatalf("훅 실행 실패: %v", err)
	}
	return rc, o.String(), e.String()
}

// runHook: 훅 name을 settings.json 명령 줄 그대로 부른다(HOME에 시험 빌드를 걸고). 종료 코드·stdout·stderr를 돌려준다.
func runHook(t *testing.T, name string, stdin []byte, env []string, dir string) (rc int, stdout, stderr string) {
	t.Helper()
	return runOut(t, hookExec(name, stdin, withHome(t, env), dir))
}

// blockCase: 이름·기대 종료 코드·명령. 막는 판(rc≠0)이면 stderr에 msg가 들어야 한다.
type blockCase struct {
	name string
	want int
	cmd  string
}

func checkRC(t *testing.T, rc, want int, stderr, msg string) {
	t.Helper()
	if rc != want || (want != 0 && !strings.Contains(stderr, msg)) {
		t.Errorf("rc=%d, want %d (stderr에 %q) err=[%s]", rc, want, msg, stderr)
	}
}
