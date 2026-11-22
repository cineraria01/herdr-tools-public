package clitest

// 시험이 소유하는 자식(②-F, gofix3-testkit): 시험은 자기가 띄운 자식만 회수한다 — 파일에서 읽은 숫자 PID에 신호를 보내지 않는다.
// 자식은 새 무리로 띄우고, 우두머리를 거두기(wait4)와 무리에 신호 보내기를 한 잠금 아래 둔다: 우두머리를 거두기 전에는 그 번호가
// 다른 프로세스에 다시 쓰일 수 없으니(좀비도 번호를 쥔다) 신호는 늘 이 실행의 무리에 간다. 거둔 뒤에는 신호를 보내지 않는다
// (끄는 중에 거둔 때만 그 자리에서 남은 무리에 KILL 한 번). 시험 마감(t.Deadline·-test.timeout)이 오기 전에 끄고 거둔다.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

// minRemain: 시험 마감까지 이보다 덜 남았으면 띄우지 않는다(끄고 거둘 여유가 없다)
const minRemain = 200 * time.Millisecond

// sigSent: 보낸 무리 신호 수(계약 시험이 「먼저 끝난 자식에는 신호 0」을 본다)
var sigSent atomic.Int64

// binStart: 시험 바이너리 시작(이 꾸러미 init) — testing이 -test.timeout 시계를 거는 m.Run보다 먼저라 마감을 조금 이르게 본다(안전한 쪽)
var binStart = time.Now()

// testDeadline: 시험 바이너리의 바깥 마감(-test.timeout — t.Deadline()과 같은 시계). 없으면(0·시험 밖) false
func testDeadline() (time.Time, bool) {
	f := flag.Lookup("test.timeout")
	if f == nil {
		return time.Time{}, false
	}
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return time.Time{}, false
	}
	to, ok := g.Get().(time.Duration)
	if !ok || to <= 0 {
		return time.Time{}, false
	}
	return binStart.Add(to), true
}

// window: 상한 max인 명령 하나를 언제까지 돌릴지(end)와 끄는 여유(TERM 유예 grace, KILL 뒤 기다림 kw).
// 시험 마감이 있으면 end + grace + 2·kw가 마감 안에 들게 — 여유는 남은 시간에 맞춰 줄인다. 남은 시간이 minRemain보다 짧으면 ok=false(띄우지 않음)
func window(max time.Duration) (end time.Time, grace, kw time.Duration, ok bool) {
	now := time.Now()
	grace, kw = 2*time.Second, time.Second
	end = now.Add(max)
	dl, has := testDeadline()
	if !has {
		return end, grace, kw, true
	}
	rem := dl.Sub(now)
	if rem < minRemain {
		return end, grace, kw, false
	}
	grace, kw = min(grace, rem/5), min(kw, rem/10)
	if e := dl.Add(-(grace + 2*kw)); e.Before(end) {
		end = e
	}
	return end, grace, kw, true
}

// Child: 시험이 소유하는 자식 무리(Spawn)
type Child struct {
	Pid int

	cmd      *exec.Cmd
	mu       sync.Mutex // 거두기와 무리 신호를 한 줄로
	reaped   bool
	stopping bool
	rc       int
	done     chan struct{} // 우두머리를 거뒀다
	ioDone   chan struct{} // 출력 관을 다 읽었다(또는 닫았다)
	pipes    []*os.File    // 이 쪽이 읽는 관 끝(마감에 닫는다)
	win      chan int      // windows: Wait 고루틴의 종료 코드
}

// ExitError: Wait의 0 아닌 종료(신호면 128+신호)
type ExitError struct{ RC int }

func (e *ExitError) Error() string { return fmt.Sprintf("종료 %d", e.RC) }

// start: c를 새 무리로 띄운다. 표준 입출력이 *os.File이 아니면 이 쪽 관으로 잇는다(exec의 복사 고루틴·Wait 없이 거두려고)
func start(c *exec.Cmd) (*Child, error) {
	proc.Group(c)
	ch := &Child{cmd: c, done: make(chan struct{}), ioDone: make(chan struct{})}
	var kids []*os.File // 자식에게 준 관 끝(띄운 뒤 이 쪽은 닫는다)
	var copies sync.WaitGroup
	fail := func(err error) (*Child, error) {
		for _, f := range append(kids, ch.pipes...) {
			f.Close()
		}
		return nil, err
	}
	outTo := func(w io.Writer, same *os.File) (*os.File, error) {
		if f, ok := w.(*os.File); ok || w == nil {
			return f, nil
		}
		if same != nil {
			return same, nil
		}
		r, wf, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		kids, ch.pipes = append(kids, wf), append(ch.pipes, r)
		copies.Add(1)
		go func() { defer copies.Done(); io.Copy(w, r) }() //nolint:errcheck // 관이 닫히면(EOF·마감에 닫음) 끝
		return wf, nil
	}
	o, err := outTo(c.Stdout, nil)
	if err != nil {
		return fail(err)
	}
	var e *os.File
	if c.Stderr != nil && c.Stderr == c.Stdout {
		e = o
	} else if e, err = outTo(c.Stderr, nil); err != nil {
		return fail(err)
	}
	c.Stdout, c.Stderr = o, e
	if in := c.Stdin; in != nil {
		if _, ok := in.(*os.File); !ok {
			r, w, err := os.Pipe()
			if err != nil {
				return fail(err)
			}
			kids = append(kids, r)
			c.Stdin = r
			go func() { io.Copy(w, in); w.Close() }() //nolint:errcheck // 자식이 안 읽고 끝나면 EPIPE로 끝
		}
	}
	if err := c.Start(); err != nil {
		return fail(err)
	}
	for _, f := range kids {
		f.Close()
	}
	ch.Pid = c.Process.Pid
	go func() { copies.Wait(); close(ch.ioDone) }()
	ch.begin()
	go ch.poll()
	return ch, nil
}

// poll: 우두머리가 끝나면 거둔다(잠금 안에서). 끄는 중에 거뒀으면 그 자리에서 남은 무리에 KILL 한 번 —
// 남은 것이 있으면 그것들이 무리 번호를 쥐고 있어 다른 프로세스에 가지 않는다
// ponytail: 10ms 폴링 — 시험 자식 몇 개라 충분하다. 많아지면 SIGCHLD로
func (ch *Child) poll() {
	for {
		ch.mu.Lock()
		if !ch.reaped {
			ch.tryReap()
		}
		r := ch.reaped
		ch.mu.Unlock()
		if r {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// tryReap: (잠금 안) 우두머리가 끝났으면 거두고 done을 닫는다
func (ch *Child) tryReap() {
	rc, ok := ch.reapNoHang()
	if !ok {
		return
	}
	ch.reaped, ch.rc = true, rc
	if ch.stopping {
		ch.signal(syscall.SIGKILL)
	}
	ch.cmd.Process.Release() //nolint:errcheck // 거둔 뒤 손잡이만 푼다
	close(ch.done)
}

// signal: (잠금 안) 무리에 신호
func (ch *Child) signal(s syscall.Signal) {
	sigSent.Add(1)
	killGroup(ch.Pid, s) //nolint:errcheck // 무리가 이미 비었으면 ESRCH — 끌 것이 없다
}

func (ch *Child) waitDone(d time.Duration) bool {
	select {
	case <-ch.done:
		return true
	case <-time.After(d):
		return false
	}
}

// stop: 아직 거두지 않았으면 무리에 TERM → grace 안에 안 끝나면 KILL → kw까지 기다림. 이미 끝났으면 신호 0
func (ch *Child) stop(grace, kw time.Duration) {
	ch.mu.Lock()
	if !ch.reaped {
		ch.tryReap()
	}
	if ch.reaped {
		ch.mu.Unlock()
		return
	}
	ch.stopping = true
	ch.signal(syscall.SIGTERM)
	ch.mu.Unlock()
	if ch.waitDone(grace) {
		return
	}
	ch.mu.Lock()
	if !ch.reaped {
		ch.signal(syscall.SIGKILL)
	}
	ch.mu.Unlock()
	ch.waitDone(kw)
}

// closeIO: 이 쪽 관 끝을 닫아 복사를 끝낸다(무리 밖 손자가 관을 쥐고 있어도 돌아오게)
func (ch *Child) closeIO() {
	for _, f := range ch.pipes {
		f.Close()
	}
	<-ch.ioDone
}

// Stop: 무리째 TERM → 2초 → KILL → 거둠, 출력 관도 닫는다. 이미 끝났으면 신호를 보내지 않는다
func (ch *Child) Stop() {
	ch.stop(2*time.Second, time.Second)
	ch.closeIO()
}

// Kill: 아직 거두지 않았으면 무리에 바로 KILL(정리 없는 죽음을 흉내 낼 때) 하고 거둘 때까지 기다린다(10초)
func (ch *Child) Kill() {
	ch.mu.Lock()
	if !ch.reaped {
		ch.tryReap()
	}
	if !ch.reaped {
		ch.signal(syscall.SIGKILL)
	}
	ch.mu.Unlock()
	ch.waitDone(10 * time.Second)
}

// Done: 우두머리를 거두면 닫히는 채널
func (ch *Child) Done() <-chan struct{} { return ch.done }

// Wait: 우두머리가 끝나 거둘 때까지, 그리고 출력 관을 다 읽을 때까지. 0 아니면 *ExitError
func (ch *Child) Wait() error {
	<-ch.done
	<-ch.ioDone
	if ch.rc != 0 {
		return &ExitError{RC: ch.rc}
	}
	return nil
}

// Spawn: 시험이 소유하는 자식 — 새 무리로 띄우고, t.Cleanup에서 무리째 TERM → KILL → 거둠(이미 끝났으면 신호 0).
// 시험 마감이 있으면 그 전에(끄는 여유를 남기고) 저절로 끈다 — 마감 panic에는 Cleanup이 돌지 않는다
func Spawn(t *testing.T, c *exec.Cmd) *Child {
	t.Helper()
	end, grace, kw, ok := window(24 * time.Hour)
	if dl, has := t.Deadline(); has && time.Until(dl) < minRemain {
		ok = false
	}
	if !ok {
		t.Fatalf("시험 마감까지 남은 시간이 회수 여유보다 짧아 띄우지 않음: %v", c.Args)
	}
	ch, err := start(c)
	if err != nil {
		t.Fatalf("못 띄움 %v: %v", c.Args, err)
	}
	tm := time.AfterFunc(time.Until(end), func() { ch.stop(grace, kw); ch.closeIO() })
	t.Cleanup(func() { tm.Stop(); ch.Stop() })
	return ch
}

// WaitFile: path가 생길 때까지(d 안). 안 생기거나 ENOENT 아닌 오류(모름)면 t.Fatal — 시작 표식 확인용
func WaitFile(t *testing.T, path string, d time.Duration) {
	t.Helper()
	for end := time.Now().Add(d); ; time.Sleep(20 * time.Millisecond) {
		if !Absent(t, path) {
			return
		}
		if time.Now().After(end) {
			t.Fatalf("%s 안에 %s 가 생기지 않았다", d, path)
		}
	}
}

// Absent: path가 없을 때(ENOENT)만 참. 다른 오류(EACCES 등)는 모름이라 t.Fatal
func Absent(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	t.Fatalf("있는지 모름(부재로 읽지 않는다): %v", err)
	return false
}
