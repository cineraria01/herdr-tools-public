// hc-sweep: 끝났는데 안 닫힌 hc 에이전트 터미널을 닫고, 레인이 남긴 고아 프로세스·지운 작업 트리의 색인·임시 찌꺼기를 쓴다
// (Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」). 규칙·출력·기록 꼴은 셸 판 lib/hc-sweep/sweep.py·disk.py 머리글과 같다 —
// 두 판이 같은 closed.log·events.tsv·disk-low 표식을 번갈아 써도 된다. 되돌리기: HC_SWEEP_IMPL=bash(bin/hc-sweep 포장).
// 바뀐 것: 외부 명령은 새 프로세스 무리로 띄우고 상한 시간에 무리째 거둔다(run), 판정은 모두 「모름이면 남김」 그대로.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const minute = 60 * 1000.0 // ms

var (
	home     = os.Getenv("HOME")
	tasksDir = filepath.Join(home, ".claude-tasks")
	sessDir  = filepath.Join(home, ".claude", "sessions")
	laneDir  = filepath.Join(stateHome(), "hc-lane")
	binDir   = findBinDir()
	busyRe   = regexp.MustCompile(`still running|·\s*\d+\s+(?:shell|monitor)s?\b`)
	shells   = map[string]bool{"zsh": true, "bash": true, "sh": true, "fish": true, "dash": true, "ksh": true}
)

func stateHome() string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return s
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state")
}

// findBinDir: herdr-tools-config·hc-lane이 있는 bin 폴더. 포장(bin/hc-sweep)이 HERDR_BIN_DIR로 준다. 없으면 바이너리 자리에서
// 짐작(저장소 <루트>/.bin → <루트>/bin, 깔린 곳 ~/.local/lib/herdr-go/.bin → ~/.local/bin)
func findBinDir() string {
	if d := os.Getenv("HERDR_BIN_DIR"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	root := filepath.Dir(filepath.Dir(exe))
	if filepath.Base(root) == "herdr-go" {
		return filepath.Join(filepath.Dir(filepath.Dir(root)), "bin")
	}
	return filepath.Join(root, "bin")
}

// result: 외부 명령 결과. err는 띄우지 못했거나 상한 시간을 넘김(파이썬 판의 예외)
type result struct {
	out, errs string
	rc        int
	err       error
}

type runner func(timeout time.Duration, env []string, name string, args ...string) result

// run: 새 프로세스 무리로 띄우고(표준 입력 /dev/null) 상한 시간이면 무리째 KILL. env가 nil이면 물려받은 환경
func run(timeout time.Duration, env []string, name string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	var o, e bytes.Buffer
	c.Stdout, c.Stderr, c.Env = &o, &e, env
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 3 * time.Second
	err := c.Run()
	r := result{out: o.String(), errs: e.String()}
	var ee *exec.ExitError
	switch {
	case ctx.Err() != nil:
		r.rc, r.err = -1, fmt.Errorf("%s: 시간 초과", name)
	case errors.As(err, &ee):
		r.rc = ee.ExitCode()
		if r.rc < 0 {
			r.rc, r.err = -1, err
		}
	case err != nil:
		r.rc, r.err = -1, err
	}
	return r
}

// hc <인자…> --json: 결과 JSON(대상이 객체면 argv는 셸 칸 판정에만 쓰고 버린다 — 자격증명이 섞여 있다)
func hc(args ...string) (map[string]any, error) {
	r := run(2*time.Minute, nil, "hc", append(args, "--json")...)
	if r.err != nil || r.rc != 0 {
		first := "hc 실패"
		if l := strings.Split(strings.TrimSpace(r.errs), "\n"); l[0] != "" {
			first = l[0]
		} else if r.err != nil {
			first = r.err.Error()
		}
		return nil, errors.New(cutRunes(first, 200))
	}
	if strings.TrimSpace(r.out) == "" {
		return map[string]any{"shell": false}, nil
	}
	var v any
	if err := json.Unmarshal([]byte(r.out), &v); err != nil {
		return nil, fmt.Errorf("hc %s: JSON 아님: %v", args[0], err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("hc %s: 객체 아님", args[0])
	}
	argv, _ := m["argv"].([]any)
	var sv []string
	for _, a := range argv {
		s, _ := a.(string)
		sv = append(sv, s)
	}
	if len(sv) == 0 {
		sv = []string{""}
	}
	m["shell"] = isShellPane(sv)
	delete(m, "argv")
	delete(m, "env")
	delete(m, "foreground")
	return m, nil
}

// isShellPane: 사람이 연 셸 칸인가. -c(-lc 등)나 스크립트 인자를 받은 셸은 명령 실행기라 아니다
func isShellPane(argv []string) bool {
	if !shells[strings.TrimLeft(filepath.Base(argv[0]), "-")] {
		return false
	}
	for _, a := range argv[1:] {
		if !strings.HasPrefix(a, "-") || (!strings.HasPrefix(a, "--") && strings.Contains(a, "c")) {
			return false
		}
	}
	return true
}

// 파이썬 판과 같은 값 읽기(JSON은 map[string]any)
func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func obj(m map[string]any, k string) map[string]any {
	o, _ := m[k].(map[string]any)
	return o
}
func num(m map[string]any, k string) (float64, bool) { f, ok := m[k].(float64); return f, ok }
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case float64:
		return x != 0
	case map[string]any:
		return len(x) > 0
	case []any:
		return len(x) > 0
	}
	return true
}

// pyStr: 파이썬 str()과 같은 꼴(None·정수 float·True)
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// pyFloat: 파이썬 str(float) 꼴(3.0, 0.5, 1e-06)
func pyFloat(f float64) string {
	a := f
	if a < 0 {
		a = -a
	}
	if a != 0 && (a < 1e-4 || a >= 1e16) {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		if i := strings.Index(s, "e"); i > 0 && len(s) == i+3 { // e-6 → e-06
			s = s[:i+2] + "0" + s[i+2:]
		}
		return s
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func flat(s string) string { return strings.NewReplacer("\t", " ", "\n", " ").Replace(s) }

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// record: ~/.claude-tasks/closed.log 한 줄(시각·주소·이름·까닭·cwd·sessionId·제목)
func record(address, name, why, cwd, sid, title string) error {
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return err
	}
	f := []string{time.Now().Format("2006-01-02T15:04:05-0700"), address, dash(name), why, dash(cwd), sid, dash(title)}
	for i := range f {
		f[i] = flat(f[i])
	}
	return appendLine(filepath.Join(tasksDir, "closed.log"), strings.Join(f, "\t"))
}

func appendLine(path, line string) error {
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err = fh.WriteString(line + "\n"); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// conf: 설정 값(herdr-tools-config get <키> <기본값>). 설정 도구가 곁에 없으면 기본값, 있는데 실패하면 ok=false(끄지 않음)
func conf(key, def string) (string, bool) {
	return confWith(filepath.Join(binDir, "herdr-tools-config"), key, def)
}

func confWith(tool, key, def string) (string, bool) {
	if syscall.Access(tool, 1) != nil { // X_OK
		return def, true
	}
	r := run(20*time.Second, nil, tool, "get", key, def)
	if r.err != nil || r.rc != 0 {
		return "", false
	}
	return strings.TrimSpace(r.out), true
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("hc-sweep 1")
		return
	}
	fs := flag.NewFlagSet("hc-sweep", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "닫을 것과 이유만 출력")
	self := fs.Bool("self-test", false, "가짜 기록으로 규칙 판정 확인")
	minAge := fs.Float64("min-age", 10, "고아·색인 최소 나이(분, 기본 10)")
	only := fs.String("only-under", "", "이 폴더 아래 작업 트리만(시험용)")
	fs.Parse(os.Args[1:])
	if *self {
		if err := selfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "hc-sweep self-test 실패:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
		return
	}
	// 색인·디스크 쓸기는 따로: hc ls 실패와 상관없이 돌고, 실패해도 서로·터미널·고아 쓸기를 막지 않는다
	sweepIndex(*dry, *minAge*60)
	if err := diskRun(*dry, run); err != nil {
		fmt.Println("디스크 쓸기 실패:", err)
	}
	ls, err := hc("ls")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hc-sweep: hc ls 실패:", err)
		os.Exit(1)
	}
	terms := ls["terminals"]
	sweepTerminals(terms, *dry)
	sweepOrphans(terms, *dry, *minAge*60, *only)
}
