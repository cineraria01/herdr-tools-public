// hc-sweep: 끝났는데 안 닫힌 hc 에이전트 터미널을 닫고, 레인이 남긴 고아 프로세스·지운 작업 트리의 색인·임시 찌꺼기를 쓴다
// (2026-11-21 사용자 「모두 고로 옮기고 싶어」 — 옛 셸·파이썬 판은 2026-11-22 go-shell-remove로 걷었다). 규칙·출력·기록 꼴
// (closed.log·events.tsv·disk-low 표식·sweep.lock)은 옛 판 그대로다.
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
	"time"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/version"
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

// findBinDir: herdr-tools-config·hc-lane이 있는 폴더 = 이 바이너리 곁(링크를 따른 실제 폴더 — 도구는 모두 한 폴더에 깔린다).
// 실행 파일을 환경 변수로 바꿔 끼우는 길(옛 HERDR_BIN_DIR)은 없다
func findBinDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

// result: 외부 명령 결과. err는 띄우지 못했거나 상한 시간을 넘김(파이썬 판의 예외)
type result struct {
	out, errs string
	rc        int
	err       error
}

type runner func(timeout time.Duration, env []string, name string, args ...string) result

// run: 띄우고(표준 입력 /dev/null) 상한 시간이면 그 자식을 KILL. env가 nil이면 물려받은 환경.
// 자식은 이 프로세스와 같은 무리에 둔다(셸 판 subprocess와 같게, B19) — launchd·사람이 쓸기 무리를 끄면(SIGKILL 포함) 자식도 함께 끝난다.
// ponytail: 상한에 걸린 자식의 손자는 거두지 않는다 — 부르는 것(hc·ps·lsof·du·sqlite3·getconf, 포장이 exec하는 Go 도구)은 손자를 두지 않는다
func run(timeout time.Duration, env []string, name string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	var o, e bytes.Buffer
	c.Stdout, c.Stderr, c.Env = &o, &e, env
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
	case err != nil: // 못 띄움·출력 관 기다림 넘김(ErrWaitDelay) — 모름
		r.rc, r.err = -1, err
	}
	return r
}

// hcError: hc가 실패함. notFound = 그 터미널이 없다고 코어가 답함(error.kind not_found) — 그 밖(망·시간 초과·모르는 답)은 모름
type hcError struct {
	msg      string
	notFound bool
}

func (e *hcError) Error() string { return e.msg }

func isNotFound(err error) bool {
	var he *hcError
	return errors.As(err, &he) && he.notFound
}

// notFoundIn: hc가 찍은 오류 JSON(줄마다) 가운데 error.kind(또는 kind)가 not_found인 것이 있나
func notFoundIn(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(l)), &m) != nil {
			continue
		}
		if str(obj(m, "error"), "kind") == "not_found" || str(m, "kind") == "not_found" {
			return true
		}
	}
	return false
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
		return nil, &hcError{msg: cutRunes(first, 200), notFound: r.err == nil && notFoundIn(r.errs+"\n"+r.out)}
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
	if !executable(tool) {
		return def, true
	}
	r := run(20*time.Second, nil, tool, "get", key, def)
	if r.err != nil || r.rc != 0 {
		return "", false
	}
	return strings.TrimSpace(r.out), true
}

// sweepLock: 두 판(Go·셸) 공통 잠금 <상태>/hc-sweep/sweep.lock(flock 배타, 쥔 프로세스가 죽으면 커널이 푼다). 둘이 함께 돌면 같은
// 터미널을 두 번 닫고 디스크 알림을 두 번 보낸다(B07). 남이 쥐면 busy
func sweepLock() (f *os.File, busy bool, err error) {
	d := filepath.Join(stateHome(), "hc-sweep")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, false, fmt.Errorf("잠금 폴더: %w", err)
	}
	return lockx.TryLock(filepath.Join(d, "sweep.lock"))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("hc-sweep"))
		return
	}
	os.Exit(sweepMain(os.Args[1:]))
}

func sweepMain(args []string) int {
	fs := flag.NewFlagSet("hc-sweep", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "닫을 것과 이유만 출력")
	self := fs.Bool("self-test", false, "가짜 기록으로 규칙 판정 확인")
	minAge := fs.Float64("min-age", 10, "고아·색인 최소 나이(분, 기본 10)")
	only := fs.String("only-under", "", "이 폴더 아래 작업 트리만(시험용)")
	fs.Parse(args)
	if fs.NArg() > 0 { // 셸 판 argparse와 같게 쓸기 전에 사용 오류(B02 — flag는 첫 위치 인자에서 멈춰 뒤의 --dry-run을 놓친다)
		fmt.Fprintf(os.Stderr, "hc-sweep: 모르는 인자: %s\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return 2
	}
	if *self {
		if err := selfTest(); err != nil {
			fmt.Fprintln(os.Stderr, "hc-sweep self-test 실패:", err)
			return 1
		}
		fmt.Println("ok")
		return 0
	}
	if !*dry {
		lk, busy, err := sweepLock()
		if busy {
			fmt.Fprintln(os.Stderr, "hc-sweep: 다른 쓸기가 도는 중(sweep.lock) — 이번 바퀴는 건너뜀")
			return 75
		}
		if err != nil {
			// 잠금 파일을 못 만듦(상태 폴더 자리가 막힘 등): 같은 자리를 쓰는 다른 판도 쥘 수 없다 — 경고하고 돈다
			fmt.Fprintln(os.Stderr, "hc-sweep: 경고 — 잠금을 못 잡음(그대로 돈다):", err)
		} else {
			defer lk.Close()
		}
	}
	// 색인·디스크 쓸기·Tailscale 점검은 따로: hc ls 실패와 상관없이 돌고, 실패해도 서로·터미널·고아 쓸기를 막지 않는다
	sweepIndex(*dry, *minAge*60)
	if err := diskRun(*dry, run); err != nil {
		fmt.Println("디스크 쓸기 실패:", err)
	}
	tailscaleRun(*dry, run)
	ls, err := hc("ls")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hc-sweep: hc ls 실패:", err)
		return 1
	}
	terms := ls["terminals"]
	sweepTerminals(terms, *dry)
	if _, err := sweepOrphans(terms, *dry, *minAge*60, *only); err != nil {
		return 1 // 셸 판처럼 고아 판정 실패(ps 등)는 종료 코드에(B23). 줄은 sweepOrphans가 찍었다
	}
	return 0
}
