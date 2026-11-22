// Package tests: claude/hooks/*.sh 훅의 계약 시험(셸 시험 claude/hooks/tests/*.test.sh를 옮긴 것).
// 훅을 settings.json과 같은 꼴(훅 파일이 없으면 0, 있으면 /bin/bash <훅>)로 밖에서 부르고, PreToolUse JSON을 stdin으로 넣어
// 종료 코드·stderr를 본다. 훅 자리는 HOOK(한 훅만 도는 시험에서 그 훅 대신, 예 HOOK=/nonexistent = 훅 없을 때), 없으면 이 폴더의 ../<훅>.sh.
// 두 판 대조: HERDR_HOOKS_IMPL(없으면 HERDR_IMPL)이 밖에서 정해졌으면 그 판 하나만, 아니면 go·bash 두 판을 모두 돈다.
// 실행: go test ./claude/hooks/tests/   (한 판만: HERDR_HOOKS_IMPL=bash go test ./claude/hooks/tests/ -run CotenantGuard)
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

// hookPath: HOOK이 있으면 그것, 없으면 이 시험 폴더 곁의 훅.
func hookPath(t *testing.T, name string) string {
	t.Helper()
	if h := os.Getenv("HOOK"); h != "" {
		return h
	}
	p, err := filepath.Abs(filepath.Join("..", name+".sh"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// impls: 판 목록. 훅 다섯은 한 바이너리 herdr-hooks라 스위치도 하나 — HERDR_HOOKS_IMPL(없으면 HERDR_IMPL, lib/go-run.sh와 같은 차례).
// 밖에서 정했으면 그 판 하나만, 아니면 go 하나 — 셸 판 갈래는 HERDR_TEST_BASH=1일 때만 두 판을(2026-11-22 verify-parallel:
// 셸 판은 곧 걷는다, 지휘 t1cj). 셸 판을 지우는 날 이 스위치도 지운다.
func impls() []string {
	for _, k := range []string{"HERDR_HOOKS_IMPL", "HERDR_IMPL"} {
		if v := os.Getenv(k); v != "" {
			return []string{v}
		}
	}
	if os.Getenv("HERDR_TEST_BASH") == "1" {
		return []string{"go", "bash"}
	}
	return []string{"go"}
}

var (
	hooksBinOnce sync.Once
	hooksBin     string
	hooksBinErr  error
	hooksBinDir  string
)

// buildHooks: 이 시험만의 herdr-hooks를 -tags hookstest로 한 번 짓는다(시험 손잡이 HERDR_HOOKS_TEST_*가 먹는 빌드 —
// 옛 hooks-impl.sh와 같다). 밖에서 HERDR_HOOKS_BIN을 주면 그것을 쓴다.
func buildHooks(t *testing.T) string {
	t.Helper()
	hooksBinOnce.Do(func() {
		if b := os.Getenv("HERDR_HOOKS_BIN"); b != "" {
			hooksBin = b
			return
		}
		goBin, err := exec.LookPath("go")
		if err != nil {
			hooksBinErr = fmt.Errorf("go가 없다(brew install go) — HERDR_HOOKS_IMPL=bash로 셸 판만 볼 수 있다: %w", err)
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
		cmd.Env = envWith(nil, "GOTOOLCHAIN=local", "CGO_ENABLED=0")
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
	if hooksBinDir != "" {
		os.RemoveAll(hooksBinDir) //nolint:errcheck // 임시 폴더
	}
	os.Exit(rc)
}

// forImpls: 판마다 f를 부분 시험으로 돈다. implEnv = HERDR_HOOKS_IMPL=<판>, Go 판이면 HERDR_HOOKS_BIN=<시험 빌드>도 —
// IMPL=go인데 바이너리를 못 쓰면 훅이 종료 1이라 셸 판으로 슬쩍 넘어가지 않는다. name은 읽는 사람용(어느 훅인지).
func forImpls(t *testing.T, name string, f func(t *testing.T, implEnv []string)) {
	for _, impl := range impls() {
		t.Run(impl, func(t *testing.T) {
			env := []string{"HERDR_HOOKS_IMPL=" + impl}
			if impl == "go" || impl == "auto" {
				env = append(env, "HERDR_HOOKS_BIN="+buildHooks(t))
			}
			f(t, env)
		})
	}
}

// isGo: 그 판이 Go 판인가 — 셸 판을 sed로 고치던 시험은 Go 판에서 시험 손잡이(HERDR_HOOKS_TEST_*)로.
func isGo(implEnv []string) bool {
	for _, kv := range implEnv {
		if strings.HasPrefix(kv, "HERDR_HOOKS_BIN=") {
			return true
		}
	}
	return false
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

// envWith: 바깥 환경에서 unset 이름을 빼고 set(K=V)을 덮는다. 판 스위치 HERDR_IMPL과 시험 손잡이 HERDR_HOOKS_TEST_*는
// 늘 뺀다(옛 hooks-impl.sh의 unset) — 판은 forImpls가 HERDR_HOOKS_IMPL로 정한다.
func envWith(unset []string, set ...string) []string {
	drop := map[string]bool{"HERDR_IMPL": true, "HERDR_HOOKS_TEST_GIT": true, "HERDR_HOOKS_TEST_NOW_OFFSET": true, "HERDR_HOOKS_TEST_SECONDS": true}
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

// runHook: settings.json의 포장 그대로 — 훅 파일을 못 읽으면 0, 아니면 /bin/bash <훅>. 종료 코드·stdout·stderr를 돌려준다.
func runHook(t *testing.T, hook string, stdin []byte, env []string, dir string) (rc int, stdout, stderr string) {
	t.Helper()
	if f, err := os.Open(hook); err != nil {
		return 0, "", ""
	} else {
		f.Close()
	}
	cmd := exec.Command("/bin/bash", hook)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	cmd.Dir = dir
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
