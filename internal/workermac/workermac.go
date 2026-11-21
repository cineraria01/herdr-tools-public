// Package workermac: worker-mac-check·worker-mac-setup(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」)이 함께 쓰는 것 —
// 셸 안전 인용, 상한 시간을 건 실행(exec.CommandContext + 새 프로세스 무리 — 시간이 지나거나 신호를 받으면 무리째 거둔다),
// 개인 설정 읽기(herdr-tools-config). 원격에서 도는 부분은 bash 그대로 보낸다(원격 기계에는 Go가 없다) — 여기는 맥 쪽만.
package workermac

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"herdr-tools/internal/proc"
)

// Who: 경고 줄 앞에 붙는 이름(도구가 main에서 정한다)
var Who = "worker-mac"

// Grace: 끝낼 무리를 TERM한 뒤 KILL까지
var Grace = 2 * time.Second

// TimeoutCode: 상한 시간에 걸린 실행의 종료 코드(셸 판 perl alarm과 같은 128+SIGALRM)
const TimeoutCode = 142

// Quote: 셸 안전 인용 — 작은따옴표로 감싸고, 안의 작은따옴표는 닫고 \' 를 넣고 다시 연다(원격 bash 스크립트 앞에 붙이는 설정 값)
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

var inflight sync.WaitGroup // 기다리는 자식(신호를 받으면 거둘 때까지 기다렸다 끝낸다)

// Command: exec.CommandContext에 새 프로세스 무리를 단 것 — ctx가 끝나면(상한·신호) 무리째 TERM → Grace → KILL.
// 표준 입력은 c.Stdin(nil이면 /dev/null). 자식이 끝났는데 손자가 출력 관을 쥐고 있으면 몇 초 뒤 그만 기다린다
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	proc.Group(c)
	c.Cancel = func() error { proc.StopGroup(c.Process.Pid, Grace); return nil }
	c.WaitDelay = Grace + 3*time.Second
	return c
}

// Run: Command로 만든 c를 띄워 끝날 때까지 기다리고 셸 종료 코드를 돌려준다 — 못 띄우면 127, 신호로 끝나면 128+신호,
// ctx 상한에 걸렸으면 TimeoutCode
func Run(ctx context.Context, c *exec.Cmd) int {
	inflight.Add(1)
	defer inflight.Done()
	if err := c.Start(); err != nil {
		if c.Stderr != nil { // 셸 판이라면 「command not found」가 나갈 자리
			fmt.Fprintf(c.Stderr, "%s: %s 를 띄우지 못했다: %v\n", Who, c.Path, err)
		}
		return 127
	}
	err := c.Wait()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return TimeoutCode
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// 자식은 끝났는데 손자가 출력 관을 쥐고 있었다 — 받은 출력이 다인지 모른다(Go 전환 감사 B20). 그 무리를 거두고 모름으로
		proc.StopGroup(c.Process.Pid, Grace)
		fmt.Fprintf(os.Stderr, "%s: %s 는 끝났는데 출력 관이 닫히지 않았다 — 결과를 모른다\n", Who, c.Path)
		return TimeoutCode
	}
	ps := c.ProcessState
	if ps == nil {
		fmt.Fprintf(os.Stderr, "%s: %s 결과를 모른다: %v\n", Who, c.Path, err)
		return 127
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

// Output: $(name args…)에 sec초 상한(0이면 없음) — 표준 출력(끝 줄바꿈 그대로, 시간이 지나도 그때까지 받은 것)과 종료 코드.
// 표준 오류는 errw(nil이면 버림)
func Output(ctx context.Context, sec int, errw io.Writer, name string, args ...string) (string, int) {
	var b bytes.Buffer
	if sec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(sec)*time.Second)
		defer cancel()
	}
	c := Command(ctx, name, args...)
	c.Stdout, c.Stderr = &b, errw
	rc := Run(ctx, c)
	return b.String(), rc
}

// Trim: $(…)처럼 끝 줄바꿈을 뺀다
func Trim(s string) string { return strings.TrimRight(s, "\n") }

// Field: sed -n 's/^<prefix>//p' — prefix로 시작하는 줄들에서 prefix를 뺀 것(여러 줄이면 줄바꿈으로 잇는다)
func Field(out, prefix string) string {
	var r []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			r = append(r, l[len(prefix):])
		}
	}
	return strings.Join(r, "\n")
}

// Signals: 도구의 바탕 ctx. INT·TERM·HUP을 받으면 ctx를 끝내(기다리던 자식 무리를 거둔다 — 자식은 새 무리라 단말의 ^C를
// 받지 않는다) 거두기를 기다린 뒤 128+신호로 끝낸다
func Signals() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		s := <-ch
		cancel()
		done := make(chan struct{})
		go func() { inflight.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(Grace + 4*time.Second):
		}
		os.Exit(128 + int(s.(syscall.Signal)))
	}()
	return ctx
}

// ConfigDir: 개인 설정 폴더(${HERDR_TOOLS_CONFIG:-$HOME/.config/herdr-tools})
func ConfigDir() string {
	if d := os.Getenv("HERDR_TOOLS_CONFIG"); d != "" {
		return d
	}
	return os.Getenv("HOME") + "/.config/herdr-tools"
}

// Config: herdr-tools-config get 읽기. 키가 없으면(종료 3) 기본값, 기본값도 없으면 빈 값으로 두고 Miss에 키를 더한다.
// 그 밖의 실패(같은 키 두 번·못 부름 등)는 Err에 그 표준 오류를 남기고 그 뒤 Get은 빈 값 — 부른 쪽이 종료 2로 끝낸다
type Config struct {
	Ctx  context.Context
	Miss []string
	Err  error
}

// Get: 키 값(끝 줄바꿈 뺌). def는 0개나 1개
func (c *Config) Get(key string, def ...string) string {
	if c.Err != nil {
		return ""
	}
	var eb bytes.Buffer
	out, rc := Output(c.Ctx, 30, &eb, "herdr-tools-config", "get", key)
	switch rc {
	case 0:
		return Trim(out)
	case 3:
		if len(def) > 0 {
			return def[0]
		}
		c.Miss = append(c.Miss, key)
		return ""
	}
	if eb.Len() > 0 {
		c.Err = errors.New(strings.TrimRight(eb.String(), "\n"))
	} else {
		c.Err = fmt.Errorf("%s: herdr-tools-config get %s 실패(종료 %d)", Who, key, rc)
	}
	return ""
}

// File: herdr-tools-config file <이름> — 설정 폴더의 그 파일 절대 경로. 없거나 못 읽으면 ok 거짓
func (c *Config) File(name string) (string, bool) {
	out, rc := Output(c.Ctx, 30, nil, "herdr-tools-config", "file", name)
	return Trim(out), rc == 0
}

// LookIn: PATH 꼴 목록 dirs에서 실행 파일 name을 찾는다(/가 들면 그대로). 없으면 빈 값
func LookIn(dirs, name string) string {
	if strings.Contains(name, "/") {
		return name
	}
	for _, d := range strings.Split(dirs, ":") {
		if d == "" {
			d = "."
		}
		p := d + "/" + name
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}
