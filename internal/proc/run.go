package proc

// 상한 걸린 실행(Run·RunMax·RunCtx·Start)과 띄우기(launch). 마감은 진입에서 한 번 정하고 준비(신호 처리기·시작 시각·고아 확인)까지
// 덮는다. 결과(종료 코드·출력)는 자식 종료와 출력 끝이 둘 다 그 마감 안일 때만 채택한다 — 마감 뒤 유예는 관·무리를 거두는 데만
// 쓴다(재감사 2바퀴 P2·P4).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Run: c.Run() 자리 — Limit(c.Args) 상한 안에서 기다린다. 상한이 있으면 새 무리로 띄우고(표준 입력이 이 프로세스의 것이면 무리를
// 나누지 않는다 — 단말에서 SIGTTIN), 넘으면 무리째 TERM → Grace → KILL 하고 *TimeoutError(ErrTimeout). 자식이 끝났는데 출력 관이
// 마감 안에 안 닫히면(남은 손자가 쥠) 그 무리를 거두고 *TimeoutError{Pipe}. 두 경우 모두 c.Stdout·c.Stderr에 받던 출력은 믿지 않는다.
// 따로 무리로 띄운 자식은 고아 기록(orphan.go)에 남기고, 이 프로세스가 TERM·INT·HUP을 받으면 그 무리를 거둔 뒤 같은 신호로 끝난다
func Run(c *exec.Cmd) error { return RunMax(c, Limit(c.Args)) }

// RunMax: Run과 같되 상한을 d로(proc.Limit이 모르는 명령 — 알림·화면 붙이기·로컬 sh 등). d가 0 이하면 상한·무리 없이 c.Run().
// 출력(c.Stdout·c.Stderr가 파일이 아닌 것)은 이 함수가 관으로 받아 옮긴다(output.go): 쓰개 오류는 결과 오류, 돌아온 뒤에는 쓰개에
// 새로 쓰지 않는다, 쓰개가 막혀도 마감 + 회수 유예 안에 돌아온다
func RunMax(c *exec.Cmd, d time.Duration) error {
	if d <= 0 {
		return c.Run()
	}
	return run(nil, c, d)
}

// RunCtx: RunMax에 ctx — ctx가 끝나면 마감과 같게(무리째 TERM → Grace → KILL, 결과는 모름: errors.Is로 ErrTimeout과 ctx 오류 둘 다 참).
// d가 0 이하면 ctx의 마감만(ctx도 끝나지 않는 것이면 c.Run())
func RunCtx(ctx context.Context, c *exec.Cmd, d time.Duration) error {
	if d <= 0 && ctx.Done() == nil {
		return c.Run()
	}
	return run(ctx, c, d)
}

func run(ctx context.Context, c *exec.Cmd, d time.Duration) error {
	var deadline time.Time
	if d > 0 {
		deadline = time.Now().Add(d)
	}
	var cancel <-chan struct{}
	if ctx != nil {
		if dl, ok := ctx.Deadline(); ok && (deadline.IsZero() || dl.Before(deadline)) {
			deadline, d = dl, time.Until(dl)
		}
		cancel = ctx.Done()
	}
	h := head(c.Args) // gate가 c.Args를 바꾸기 전에
	ctxErr := func(e error) error {
		if ctx != nil && ctx.Err() != nil {
			return fmt.Errorf("%w: %w", ctx.Err(), e)
		}
		return e
	}
	if ctx != nil && ctx.Err() != nil {
		return ctxErr(refuse(h, d, ctx.Err()))
	}
	if c.SysProcAttr == nil && c.Stdin != os.Stdin {
		setGroup(c)
	}
	grouped := isGrouped(c)
	if c.WaitDelay == 0 {
		c.WaitDelay = Grace // 표준 입력을 옮기는 exec 고루틴 몫
	}
	o, err := takeOutput(c)
	if err != nil {
		return fmt.Errorf("출력 관: %w", err)
	}
	if err := launch(c, grouped, deadline, h, d, nil, false); err != nil {
		o.abandon(Grace)
		return ctxErr(err)
	}
	o.started()
	pid := c.Process.Pid
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	var tc <-chan time.Time
	if !deadline.IsZero() {
		t := time.NewTimer(time.Until(deadline))
		defer t.Stop()
		tc = t.C
	}
	select {
	case err := <-done:
		leaderDone(pid, grouped)
		hook("leaderdone")
		// 출력 끝은 마감 안, 그리고 종료 뒤 Grace 안(무리 밖으로 나간 손자가 관을 오래 쥐면 마감까지 다 기다리지 않는다 — R-H09)
		by := time.Now().Add(Grace)
		if !deadline.IsZero() && deadline.Before(by) {
			by = deadline
		}
		if errors.Is(err, exec.ErrWaitDelay) || !o.drainedBy(by, cancel) {
			// 자식은 끝났는데 출력 관이 마감 안에 안 닫혔다 — 종료 코드와 반쪽 출력으로 결과를 정하지 않는다(모름)
			e := &TimeoutError{Head: h, D: d, Pipe: true}
			fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
			if grouped {
				Stop(pid, Grace) // 관을 쥔 손자가 그 무리에 있으면 함께(무리가 이미 사라졌으면 bounded에 없어 신호 0)
			}
			o.abandon(Grace)
			finish(pid)
			return ctxErr(e)
		}
		if grouped {
			settle(pid)
		}
		o.close()
		finish(pid)
		if err == nil {
			if werr := o.err(); werr != nil {
				return fmt.Errorf("출력을 옮기지 못했다: %w", werr)
			}
		}
		return err
	case <-tc:
	case <-cancel:
	}
	e := &TimeoutError{Head: h, D: d}
	fmt.Fprintf(os.Stderr, "%s: %s\n", Who, e)
	if grouped {
		Stop(pid, Grace)
	} else if err := c.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		fmt.Fprintf(os.Stderr, "%s: %d 를 끄지 못했다: %v\n", Who, pid, err)
	}
	// ponytail: 신호도 안 먹는 커널 대기(D 상태)면 거두지 못한다 — 몇 초만 더 기다리고 놓아 둔다(c.Wait 고루틴은 그 프로세스와 함께 남는다.
	// 출력은 이 함수의 관이라 abandon 뒤에는 부른 쪽 쓰개에 새로 쓰지 않는다)
	select {
	case <-done:
		leaderDone(pid, grouped)
	case <-time.After(2*Grace + 3*time.Second):
	}
	o.abandon(Grace)
	finish(pid)
	return ctxErr(e)
}

// Start: 부른 쪽이 직접 기다릴 자식(파이프로 이은 명령처럼 Run이 못 기다리는 것)을 띄우고 Run의 무리처럼 올린다 — Group을 먼저 부르면
// 이 프로세스가 TERM·INT·HUP을 받을 때 그 무리를 거두고(StopCurrent) 부모가 SIGKILL로 죽으면 다음 실행이 거둔다(고아 기록 — 자식은
// 기록이 쓰인 뒤에야 명령을 시작한다). 부른 쪽은 c.Wait 뒤 release를 꼭 부른다(무리에 남은 배경 프로세스를 거두고 기록을 지움).
// 오류면 자식은 명령 없이 이미 거둬졌다(c.Wait를 부르지 않는다). 멈추는 중이면 Start·release는 돌아오지 않는다(Run과 같음).
// 상한은 부른 쪽이 건다 — 마감을 먼저 정했으면 StartBy로 준비도 그 안에서
func Start(c *exec.Cmd) (release func(), err error) { return StartBy(c, time.Time{}) }

// StartBy: Start를 deadline 안에서(빈 값이면 마감 없음) — 준비·시작 시각 조회·기록이 마감을 넘기면 명령 없이 *TimeoutError
func StartBy(c *exec.Cmd, deadline time.Time) (release func(), err error) {
	grouped := isGrouped(c)
	var d time.Duration
	if !deadline.IsZero() {
		d = time.Until(deadline)
	}
	if err := launch(c, grouped, deadline, head(c.Args), d, nil, false); err != nil {
		return nil, err
	}
	pid := c.Process.Pid
	return func() {
		leaderDone(pid, grouped)
		if grouped {
			settle(pid)
		}
		finish(pid)
	}, nil
}

// Stop: Run·Start·Wait가 올린 무리 pid를 TERM → grace → KILL로 거둔다 — 이 실행이 올린 무리만(내린 번호·남의 번호는 건드리지 않는다, 재감사 ops R-05)
func Stop(pid int, grace time.Duration) {
	curMu.Lock()
	st, ok := bounded[pid]
	curMu.Unlock()
	if ok {
		stopOwned(pid, st, grace, true)
	}
}

// testHook: 시험이 끼어드는 자리 — 「started」(자식은 떴고 기록 전), 「recorded」(기록 뒤 명령 시작 전), 「leaderdone」(우두머리를 거둔
// 뒤 출력 끝 전 — 소유 판정 시험)
var testHook func(stage string)

func hook(stage string) {
	if testHook != nil {
		testHook(stage)
	}
}

// launch: c를 띄우고 무리면 bounded·고아 기록에 올린다(멈추는 중이면 띄우지 않고 돌아오지 않는다). 기록할 무리는 gate로 감싸
// pid·시작 시각·부모 소유 기록(그리고 extra — WaitMax의 자식 기록)을 디스크에 다 쓴 뒤에야 실제 명령을 시작시킨다(재감사 R-02·R-H05·
// fill R-06·ops R-04) — 그 전에 이 실행이 죽으면 자식은 명령 없이 끝난다. 준비(prepare)·시작 시각·기록 가운데 하나라도 못 얻거나
// 마감을 넘기면 명령 없이 자식을 거두고(또는 띄우지 않고) *TimeoutError{Why}(모름 — 띄우지 않음). keep이면 고아 기록에 keep 표식
// (부모가 죽어도 거두지 않고 끝날 때까지 센다 — 상한 없는 Wait)
func launch(c *exec.Cmd, grouped bool, deadline time.Time, h string, d time.Duration, extra func(pid int, st string) error, keep bool) error {
	var goAhead *os.File
	selfSt := ""
	if grouped {
		// 첫 무리 — 신호를 받으면 무리째 거두게, 앞 실행의 고아 무리를 거둔다. 마감 안에서만(못 끝내면 띄우지 않는다 — 2바퀴 P2)
		if err := prepare(deadline); err != nil {
			return refuse(h, d, err)
		}
		if RecordDir() != "" || extra != nil {
			if RecordDir() != "" {
				s, err := self(deadline)
				if err != nil {
					return refuse(h, d, fmt.Errorf("이 프로세스의 시작 시각: %w", err))
				}
				selfSt = s
			}
			var err error
			if goAhead, err = gate(c); err != nil {
				return err
			}
		}
	}
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		park() // 멈추는 중 — 새 자식을 띄우지 않는다
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
	if err == nil && selfSt != "" {
		err = record(pid, st, selfSt, keep)
	}
	if err == nil && extra != nil {
		err = extra(pid, st)
	}
	if err == nil && past(deadline) {
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

// leaderDone: 우두머리를 거뒀다 — 무리가 이미 사라졌으면 곧바로 bounded·고아 기록에서 내린다. 그 뒤로 그 번호는 다른 무리에 다시
// 쓰일 수 있어 숫자만으로 그것이라 하지 않는다(재감사 2바퀴 P5·merger R-207·R-221). 무리가 남았으면 번호는 아직 그 무리의 것이다
func leaderDone(pid int, grouped bool) {
	if grouped && groupGone(pid) {
		drop(pid, true)
	}
}

// settle: 우두머리가 끝난 무리 — 다 끝났으면 기록만 지우고, 남은 배경 프로세스가 있으면 거둔다(이 실행이 올린 그 무리일 때만)
func settle(pid int) {
	if groupGone(pid) {
		drop(pid, true)
	} else {
		Stop(pid, Grace)
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

// Combined: c.CombinedOutput() 자리 — 상한(Limit)에 걸리면 출력 없이 *TimeoutError
func Combined(c *exec.Cmd) ([]byte, error) { return CombinedMax(c, Limit(c.Args)) }

// Output: c.Output() 자리(표준 출력만 받는다, c.Stdout은 비워 둔다) — 상한(Limit)에 걸리면 출력 없이 *TimeoutError
func Output(c *exec.Cmd) ([]byte, error) { return OutputMax(c, Limit(c.Args)) }

// CombinedMax: Combined를 상한 d로(RunMax 규칙)
func CombinedMax(c *exec.Cmd, d time.Duration) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	return result(&b, RunMax(c, d))
}

// OutputMax: Output을 상한 d로(RunMax 규칙)
func OutputMax(c *exec.Cmd, d time.Duration) ([]byte, error) {
	var b bytes.Buffer
	c.Stdout = &b
	return result(&b, RunMax(c, d))
}

// result: 시간 넘김(모름)이면 출력 없이
func result(b *bytes.Buffer, err error) ([]byte, error) {
	if errors.Is(err, ErrTimeout) {
		return nil, err
	}
	return b.Bytes(), err
}
