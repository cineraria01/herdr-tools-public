package proc

// 바깥 명령 시간 상한(2026-11-21 go-followups-2): 맥 hc-fill 바퀴 하나가 자식 `git remote get-url origin`이 커널 안에서 잠들어
// 37분 멈췄다 — 상한 없이 기다려 그동안 대기열이 하나도 뜨지 않았다. git·hc·hc-lane·ssh·gh·curl은 여기 Run·Output을 거쳐
// 상한 안에서만 기다린다. 넘으면 그 무리를 거두고 「시간 넘김 <명령·하위 명령> <초>」 한 줄 — 결과는 모름(ErrTimeout, TimeoutCode)이다.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TimeoutCode: 상한에 걸린 명령의 종료 코드(coreutils timeout과 같은 124) — 부른 쪽 셋 결과에서는 모름이다
const TimeoutCode = 124

// ErrTimeout: 상한에 걸렸거나(자식이 끝나지 않음) 자식이 끝났는데 출력 관이 닫히지 않았다 — 결과는 모름.
// errors.Is(err, proc.ErrTimeout)로 가린다. 「실패」·「없음」·「성공」으로 읽지 않는다(상태를 바꾸지 않고 다음 바퀴에)
var ErrTimeout = errors.New("시간 넘김")

// Grace: 상한에 걸린 무리를 TERM한 뒤 KILL까지. 자식이 끝난 뒤 출력 관을 기다리는 시간(WaitDelay)도 이것
var Grace = 2 * time.Second

// TimeoutError: 상한에 걸림(Pipe면 자식은 끝났는데 출력 관을 쥔 프로세스가 남음). Why가 있으면 띄우기 전에 멈춤 — 고아 기록·시작 시각을
// 확정하지 못해 명령을 시작시키지 않았다(재감사 R-H05, 결과는 같은 모름)
type TimeoutError struct {
	Head string
	D    time.Duration
	Pipe bool
	Why  string
}

func (e *TimeoutError) Error() string {
	if e.Why != "" {
		return fmt.Sprintf("모름 %s 띄우지 않음: %s", e.Head, e.Why)
	}
	if e.Pipe {
		return fmt.Sprintf("시간 넘김 %s 출력 관이 닫히지 않음", e.Head)
	}
	return fmt.Sprintf("시간 넘김 %s %d", e.Head, int(e.D/time.Second))
}

// Is: errors.Is(err, ErrTimeout)
func (e *TimeoutError) Is(t error) bool { return t == ErrTimeout }

// 기본 상한(초) — 환경 변수로 바꾼다(0이면 상한 없음)
func envSec(name string, def int) time.Duration {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		fmt.Fprintf(os.Stderr, "%s: 경고 — %s=%q 를 읽지 못해 기본 %d초\n", Who, name, v, def)
	}
	return time.Duration(def) * time.Second
}

// Limit: 명령(이름·인자)의 상한. 0 = 상한 없음(정해 둔 명령이 아니거나, 기다리는 것이 일인 명령 — hc wait·hc-lane land 등).
// 로컬 git HERDR_TIMEOUT_GIT(60), 원격 git·hc·ssh·gh·curl HERDR_TIMEOUT_REMOTE(120), hc-lane HERDR_TIMEOUT_LANE(600).
// 하위 명령은 맨 앞 낱말만, 옵션은 하위 명령의 옵션 자리(`--`·실행할 명령 앞)만 본다 — 옵션 값·넘기는 명령의 인자는 판정에 쓰지 않는다
func Limit(args []string) time.Duration {
	if len(args) == 0 {
		return 0
	}
	rest := args[1:]
	switch filepath.Base(args[0]) {
	case "git":
		switch s, next := gitSub(rest); s {
		case "fetch", "push", "pull", "ls-remote", "clone", "submodule", "worktree": // worktree: 큰 저장소 checkout
			return envSec("HERDR_TIMEOUT_REMOTE", 120)
		case "remote":
			if next == "update" || next == "prune" || next == "show" {
				return envSec("HERDR_TIMEOUT_REMOTE", 120)
			}
		}
		return envSec("HERDR_TIMEOUT_GIT", 60)
	case "hc":
		s := sub(rest)
		opts := hcOpts(s, rest)
		if _, bg := opts["--bg"]; s == "wait" || (s == "run" && !bg) {
			return 0 // 기다림이 일
		}
		r := envSec("HERDR_TIMEOUT_REMOTE", 120)
		if v, ok := opts["--timeout"]; ok && r > 0 {
			// 원격 작업이 받은 상한과 로컬 hc 프로세스의 마감은 따로 — 원격 상한 + 전송 여유(재감사 R-19: 원격 상한이 있다고 로컬 hc를
			// 상한 없이 기다렸다). 0·못 읽는 값은 원격도 상한 없음이라 로컬도 없음
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return time.Duration(n)*time.Second + r
			}
			return 0
		}
		return r
	case "ssh", "gh", "curl":
		return envSec("HERDR_TIMEOUT_REMOTE", 120)
	case "hc-lane":
		switch sub(rest) {
		case "land", "wait", "watch":
			return 0 // 검증·착지·기다림이 일이다
		}
		return envSec("HERDR_TIMEOUT_LANE", 600) // 레인 띄우기(옵션으로 시작하는 꼴)는 그 저장소 herdr-setup.sh(의존성 설치·빌드)를 기다린다
	}
	return 0
}

// sub: 하위 명령 — 맨 앞 낱말이 옵션이 아닐 때만(`hc-lane --name x …`처럼 옵션으로 시작하면 하위 명령 없음)
func sub(a []string) string {
	if len(a) > 0 && !strings.HasPrefix(a[0], "-") {
		return a[0]
	}
	return ""
}

// hcValue: 값을 받는 hc 옵션(그다음 낱말은 값)
var hcValue = map[string]bool{"--on": true, "--cwd": true, "--env": true, "--timeout": true, "--name": true, "--label": true,
	"--parent": true, "--if": true, "--output": true, "--version": true, "--from": true, "--via": true}

// hcOpts: hc 하위 명령 뒤 옵션 자리의 옵션들과 값(값 없는 옵션은 빈 값) — `--`에서, run·exec는 실행할 명령(첫 위치 인자)에서 멈춘다
func hcOpts(s string, rest []string) map[string]string {
	m := map[string]string{}
	for i := 1; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			k, v, hasEq := strings.Cut(a, "=")
			if hcValue[k] && !hasEq && i+1 < len(rest) {
				i++
				v = rest[i]
			}
			m[k] = v
			continue
		}
		if s == "run" || s == "exec" {
			break
		}
	}
	return m
}

// gitSub: git의 하위 명령과 그다음 낱말(-C·-c 등 값 받는 전역 옵션은 건너뜀)
func gitSub(a []string) (string, string) {
	for i := 0; i < len(a); i++ {
		switch s := a[i]; {
		case s == "-C" || s == "-c" || s == "--git-dir" || s == "--work-tree" || s == "--namespace":
			i++
		case strings.HasPrefix(s, "-"):
		default:
			if i+1 < len(a) {
				return s, a[i+1]
			}
			return s, ""
		}
	}
	return "", ""
}

var (
	plainWord = regexp.MustCompile(`^[a-z][a-z-]*$`)
	gitNested = map[string]bool{"remote": true, "worktree": true, "stash": true, "submodule": true, "notes": true, "config": true}
)

// head: 사건·로그 줄에 적는 명령 앞부분 — 이름과 하위 명령만(인자 값·사건 본문·주소·URL은 적지 않는다).
// git은 -C 경로까지(어느 저장소인지), -c 값은 「…」로, 하위 명령 뒤 낱말은 소문자 낱말(remote get-url 등)일 때만
func head(args []string) string {
	if len(args) == 0 {
		return ""
	}
	name := filepath.Base(args[0])
	rest := args[1:]
	switch name {
	case "git":
		out := []string{name}
		for i := 0; i < len(rest); i++ {
			s := rest[i]
			switch {
			case (s == "-C" || s == "--git-dir" || s == "--work-tree") && i+1 < len(rest):
				out = append(out, s, rest[i+1])
				i++
			case (s == "-c" || s == "--namespace") && i+1 < len(rest):
				out = append(out, s, "…")
				i++
			case strings.HasPrefix(s, "-"):
				out = append(out, s)
			default:
				out = append(out, s)
				if i+1 < len(rest) && gitNested[s] && plainWord.MatchString(rest[i+1]) {
					out = append(out, rest[i+1])
				}
				return strings.Join(out, " ")
			}
		}
		return strings.Join(out, " ")
	case "hc", "hc-lane", "gh":
		if s := sub(rest); s != "" && plainWord.MatchString(s) {
			return name + " " + s
		}
	}
	return name
}

// Run: c.Run() 자리 — Limit(c.Args) 상한 안에서 기다린다. 상한이 있으면 새 무리로 띄우고(표준 입력이 이 프로세스의 것이면 무리를
// 나누지 않는다 — 단말에서 SIGTTIN), 넘으면 무리째 TERM → Grace → KILL 하고 *TimeoutError(ErrTimeout). 자식이 끝났는데 출력 관이
// Grace 안에 안 닫히면(남은 손자가 쥠) 그 무리를 거두고 *TimeoutError{Pipe}. 두 경우 모두 c.Stdout·c.Stderr에 받던 출력은 믿지 않는다.
// 따로 무리로 띄운 자식은 고아 기록(orphan.go)에 남기고, 이 프로세스가 TERM·INT·HUP을 받으면 그 무리를 거둔 뒤 같은 신호로 끝난다
func Run(c *exec.Cmd) error { return RunMax(c, Limit(c.Args)) }

// RunMax: Run과 같되 상한을 d로(proc.Limit이 모르는 명령 — 알림·화면 붙이기·로컬 sh 등). d가 0 이하면 상한·무리 없이 c.Run().
// 마감은 띄우기 전에 정한다 — 준비(시작 시각·고아 기록)도 같은 남은 시간을 쓴다(재감사 R-C05). 출력(c.Stdout·c.Stderr가 파일이 아닌 것)은
// 이 함수가 따로 관으로 받아 옮긴다: 자식 종료와 출력 끝을 따로 확인하고(R-H09 — exec.Cmd.Wait는 비정상 종료면 관 기다림 넘김을 감춘다),
// 돌아오기 전에 옮기는 고루틴을 끝내 부른 쪽 버퍼에 늦게 쓰지 않는다(R-C01)
func RunMax(c *exec.Cmd, d time.Duration) error {
	if d <= 0 {
		return c.Run()
	}
	deadline := time.Now().Add(d)
	h := head(c.Args) // gate가 c.Args를 바꾸기 전에
	if c.SysProcAttr == nil && c.Stdin != os.Stdin {
		setGroup(c)
	}
	grouped := isGrouped(c)
	if c.WaitDelay == 0 {
		c.WaitDelay = Grace // 표준 입력을 옮기는 exec 고루틴 몫
	}
	o, err := takeOutput(c)
	if err != nil {
		return err
	}
	if err := launch(c, grouped, deadline, h, d); err != nil {
		o.abandon()
		return err
	}
	o.started()
	pid := c.Process.Pid
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	t := time.NewTimer(time.Until(deadline))
	defer t.Stop()
	select {
	case err := <-done:
		if errors.Is(err, exec.ErrWaitDelay) || !o.drained(Grace) {
			// 자식은 끝났는데 출력 관을 쥔 프로세스가 남았다 — 종료 코드와 반쪽 출력으로 결과를 정하지 않는다(모름)
			e := &TimeoutError{Head: h, D: d, Pipe: true}
			fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
			if grouped {
				stopOwned(pid, startOf(pid), Grace, true) // 관을 쥔 손자가 그 무리에 있으면 함께
			}
			o.abandon()
			finish(pid)
			return e
		}
		if grouped {
			settle(pid)
		}
		finish(pid)
		return err
	case <-t.C:
	}
	e := &TimeoutError{Head: h, D: d}
	fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
	if grouped {
		stopOwned(pid, startOf(pid), Grace, true)
	} else if err := c.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "%s: %d 를 끄지 못했다: %v\n", Who, pid, err)
	}
	// ponytail: 신호도 안 먹는 커널 대기(D 상태)면 거두지 못한다 — 몇 초만 더 기다리고 놓아 둔다(c.Wait 고루틴은 그 프로세스와 함께 남는다.
	// 출력은 이 함수의 관이라 abandon 뒤에는 부른 쪽 버퍼에 아무도 쓰지 않는다)
	select {
	case <-done:
	case <-time.After(2*Grace + 3*time.Second):
	}
	o.abandon()
	finish(pid)
	return e
}

// Start: 부른 쪽이 직접 기다릴 자식(파이프로 이은 명령처럼 Run이 못 기다리는 것)을 띄우고 Run의 무리처럼 올린다 — Group을 먼저 부르면
// 이 프로세스가 TERM·INT·HUP을 받을 때 그 무리를 거두고(StopCurrent) 부모가 SIGKILL로 죽으면 다음 실행이 거둔다(고아 기록 — 자식은
// 기록이 쓰인 뒤에야 명령을 시작한다). 부른 쪽은 c.Wait 뒤 release를 꼭 부른다(무리에 남은 배경 프로세스를 거두고 기록을 지움).
// 오류면 자식은 명령 없이 이미 거둬졌다(c.Wait를 부르지 않는다). 멈추는 중이면 Start·release는 돌아오지 않는다(Run과 같음).
// 상한은 부른 쪽이 건다 — 마감을 먼저 정했으면 StartBy로 준비도 그 안에서
func Start(c *exec.Cmd) (release func(), err error) { return StartBy(c, time.Time{}) }

// StartBy: Start를 deadline 안에서(빈 값이면 마감 없음) — 시작 시각 조회·기록이 마감을 넘기면 명령 없이 *TimeoutError
func StartBy(c *exec.Cmd, deadline time.Time) (release func(), err error) {
	grouped := isGrouped(c)
	var d time.Duration
	if !deadline.IsZero() {
		d = time.Until(deadline)
	}
	if err := launch(c, grouped, deadline, head(c.Args), d); err != nil {
		return nil, err
	}
	pid := c.Process.Pid
	return func() {
		if grouped {
			settle(pid)
		}
		finish(pid)
	}, nil
}

// Stop: Run·Start가 올린 무리 pid를 TERM → grace → KILL로 거둔다 — 이 실행이 올린 무리만(내린 번호·남의 번호는 건드리지 않는다, 재감사 ops R-05)
func Stop(pid int, grace time.Duration) {
	curMu.Lock()
	st, ok := bounded[pid]
	curMu.Unlock()
	if ok {
		stopOwned(pid, st, grace, true)
	}
}

// testHook: 시험이 띄우기 단계 사이(「started」 — 자식은 떴고 기록 전, 「recorded」 — 기록 뒤 명령 시작 전)에 끼어드는 자리
var testHook func(stage string)

func hook(stage string) {
	if testHook != nil {
		testHook(stage)
	}
}

// launch: c를 띄우고 무리면 bounded·고아 기록에 올린다(멈추는 중이면 띄우지 않고 돌아오지 않는다). 기록할 무리는 gate로 감싸
// pid·시작 시각·부모 소유 기록을 디스크에 다 쓴 뒤에야 실제 명령을 시작시킨다(재감사 R-02·R-H05·fill R-06·ops R-04) — 그 전에 이 실행이
// 죽으면 자식은 명령 없이 끝난다. 시작 시각·이 프로세스의 시작 시각·기록 가운데 하나라도 못 얻거나 마감을 넘기면 명령 없이 자식을
// 거두고 *TimeoutError{Why}(모름 — 띄우지 않음)
func launch(c *exec.Cmd, grouped bool, deadline time.Time, h string, d time.Duration) error {
	var goAhead *os.File
	selfSt := ""
	if grouped {
		prepare() // 첫 무리 — 앞 실행의 고아 무리를 거두고, 신호를 받으면 무리째 거두게
		if RecordDir() != "" {
			s, err := self(deadline)
			if err != nil {
				return refuse(h, d, fmt.Errorf("이 프로세스의 시작 시각: %w", err))
			}
			selfSt = s
			if goAhead, err = gate(c); err != nil {
				return err
			}
		}
	}
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		park() // 멈추는 중 — 새 자식을 띄우지 않는다(Wait와 같음)
	}
	err := c.Start()
	if goAhead != nil {
		c.ExtraFiles[len(c.ExtraFiles)-1].Close() // 자식 쪽 끝(부모는 쓰지 않는다)
	}
	if err != nil {
		curMu.Unlock()
		if goAhead != nil {
			goAhead.Close()
		}
		return err
	}
	pid := c.Process.Pid
	if grouped {
		bounded[pid] = "" // 시작 시각 전에도 StopCurrent가 끈다(이 실행이 띄운 자식이라 그것이다)
	}
	curMu.Unlock()
	if goAhead == nil {
		return nil
	}
	hook("started")
	st, err := startTimeBy(pid, deadline)
	if err == nil {
		err = record(pid, st, selfSt)
	}
	if err == nil && !deadline.IsZero() && !time.Now().Before(deadline) {
		err = errors.New("마감을 넘김")
	}
	if err != nil {
		goAhead.Close() // gate가 명령 없이 끝난다
		c.Wait()
		unrecord(pid)
		finish(pid)
		return refuse(h, d, err)
	}
	curMu.Lock()
	if _, ok := bounded[pid]; ok {
		bounded[pid] = st
	}
	stop := stopping
	curMu.Unlock()
	if stop {
		goAhead.Close() // 신호 처리 쪽이 이 무리(명령 없이 끝나는 gate)를 거둔다
		park()
	}
	hook("recorded")
	goAhead.WriteString("go\n")
	goAhead.Close()
	return nil
}

// refuse: 띄우지 않음(모름) 한 줄을 적고 그 오류
func refuse(h string, d time.Duration, why error) error {
	e := &TimeoutError{Head: h, D: d, Why: why.Error()}
	fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
	return e
}

// outPipe: RunMax가 자식 출력을 받는 관 — 파일이 아닌 c.Stdout·c.Stderr마다 os.Pipe 하나와 옮기는 고루틴 하나
type outPipe struct {
	rs, ws []*os.File
	wg     sync.WaitGroup
	done   chan struct{} // 옮기기가 다 끝나면(모든 쓰는 쪽이 닫혀 EOF, 또는 abandon) 닫힌다
}

// takeOutput: c의 파일 아닌 출력을 관으로 바꾼다(같은 Writer면 관 하나 — exec와 같음)
func takeOutput(c *exec.Cmd) (*outPipe, error) {
	o := &outPipe{done: make(chan struct{})}
	same := sameWriter(c.Stdout, c.Stderr)
	var err error
	c.Stdout, err = o.wrap(c.Stdout)
	if err == nil {
		if same {
			c.Stderr = c.Stdout
		} else {
			c.Stderr, err = o.wrap(c.Stderr)
		}
	}
	go func() { o.wg.Wait(); close(o.done) }()
	if err != nil {
		o.abandon()
		return nil, fmt.Errorf("출력 관: %w", err)
	}
	return o, nil
}

func sameWriter(a, b io.Writer) (same bool) {
	defer func() {
		if recover() != nil {
			same = false // 견줄 수 없는 꼴
		}
	}()
	return a != nil && a == b
}

func (o *outPipe) wrap(w io.Writer) (io.Writer, error) {
	if w == nil {
		return nil, nil
	}
	if f, ok := w.(*os.File); ok {
		return f, nil
	}
	r, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	o.rs, o.ws = append(o.rs, r), append(o.ws, pw)
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		io.Copy(w, r)
	}()
	return pw, nil
}

// started: 자식이 떴다 — 부모 쪽 쓰는 끝을 닫는다(남은 쓰는 끝은 자식·손자의 것)
func (o *outPipe) started() {
	for _, w := range o.ws {
		w.Close()
	}
}

// drained: 출력이 다 옮겨졌는가(max 안에)
func (o *outPipe) drained(max time.Duration) bool {
	select {
	case <-o.done:
		return true
	case <-time.After(max):
		return false
	}
}

// abandon: 관을 닫고 옮기는 고루틴이 끝나기를 기다린다 — 돌아온 뒤에는 부른 쪽 버퍼에 아무도 쓰지 않는다
func (o *outPipe) abandon() {
	for _, f := range append(append([]*os.File{}, o.ws...), o.rs...) {
		f.Close()
	}
	<-o.done
}

// settle: 우두머리가 끝난 무리 — 다 끝났으면 기록만 지우고, 남은 배경 프로세스가 있으면 거둔다
func settle(pid int) {
	if groupGone(pid) {
		unrecord(pid)
	} else {
		stopOwned(pid, startOf(pid), Grace, true)
	}
}

// finish: bounded에서 내린다. 멈추는 중이면 돌아오지 않는다(신호 처리 쪽이 무리를 다 거두고 끝낸다)
func finish(pid int) {
	curMu.Lock()
	delete(bounded, pid)
	stop := stopping
	curMu.Unlock()
	if stop {
		park()
	}
}

func startOf(pid int) string {
	curMu.Lock()
	defer curMu.Unlock()
	return bounded[pid]
}

// Combined: c.CombinedOutput() 자리 — 상한에 걸리면 출력 없이 *TimeoutError
func Combined(c *exec.Cmd) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	if err := Run(c); err != nil {
		if errors.Is(err, ErrTimeout) {
			return nil, err
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

// Output: c.Output() 자리(표준 출력만 받는다, c.Stdout은 비워 둔다) — 상한에 걸리면 출력 없이 *TimeoutError
func Output(c *exec.Cmd) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout = &b
	if err := Run(c); err != nil {
		if errors.Is(err, ErrTimeout) {
			return nil, err
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}
