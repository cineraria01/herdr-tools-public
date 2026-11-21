// Package proc: 자식 프로세스를 새 프로세스 무리(Setpgid)로 띄우고, 소유를 pid + 시작 시각으로 확인하고, 무리째 거둔다.
// 병합기 Go 판(cmd/hc-merger)에서 끌어올렸다 — 자기가 띄운 프로세스만 소유한다(2026-11-21 병합기 결함: 잠금·자식이 남아 다음 실행을 막음).
// 무리·신호는 유닉스만 — Windows는 빌드만 된다(무리 없이 그 프로세스 하나만 끈다, sys_windows.go).
package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/atomic"
)

// Who: 경고 줄 앞에 붙는 이름(도구가 main에서 정한다)
var Who = "herdr-tools"

// Code: 명령 결과를 셸 종료 코드로 — 신호로 끝나면 128+신호, 못 띄우면 127, 상한·출력 관 기다림 넘김(모름)이면 TimeoutCode
func Code(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, exec.ErrWaitDelay) {
		return TimeoutCode // 「시간 넘김」 줄은 Run이 이미 적었다
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if sig, ok := signaled(ee); ok {
			return 128 + sig
		}
		return ee.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "%s: 명령을 띄우지 못했다: %v\n", Who, err)
	return 127
}

var (
	curMu    sync.Mutex
	curPG    int                   // 지금 기다리는 자식 무리(0 = 없음)
	stopping bool                  // StopCurrent 뒤 — 새 자식을 띄우지 않는다
	bounded  = map[int]string{}    // Run(상한)이 따로 무리로 띄워 기다리는 자식 → 그 시작 시각. 부른 쪽 무리를 꺼도 안 꺼지니 StopCurrent가 함께 끈다
	parked   = make(chan struct{}) // 멈추는 중 돌아오지 않는 자리(닫히지 않는다)
)

// Group: c를 새 프로세스 무리로 띄우게 한다(Windows는 그대로)
func Group(c *exec.Cmd) { setGroup(c) }

// park: 멈추는 중 — 부른 쪽이 신호로 끈 자식의 결과를 「실패」로 읽지 않게 돌아오지 않는다(끝내는 쪽이 곧 프로세스를 끝낸다)
func park() { <-parked }

// Wait: c를 새 무리로 띄워 끝날 때까지 기다리고 종료 코드를 돌려준다. record가 있으면 그 파일에 「pid<탭>시작 시각」을 적어 두고
// 끝나면 지운다 — 이 실행이 죽어도 다음 실행이 그 자식이 아직 도는지 본다(ChildBusy). 기다리는 동안의 무리는 StopCurrent가 끈다
func Wait(c *exec.Cmd, record string) int { return WaitMax(c, record, 0) }

// WaitMax: Wait에 상한 max(0이면 없음) — 넘으면 「시간 넘김」 한 줄을 적고 그 무리째 TERM → 5초 → KILL, 종료 코드는 TimeoutCode(모름).
// record가 있으면 자식은 기록이 다 쓰인 뒤에야 실제 명령을 시작한다(gate) — 기록 전에 이 실행이 죽으면 자식은 명령 없이 끝난다
func WaitMax(c *exec.Cmd, record string, max time.Duration) int {
	setGroup(c)
	var goAhead *os.File
	if record != "" && groupsSupported {
		w, err := gate(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s 를 띄우지 못했다: %v\n", Who, c.Path, err)
			return 127
		}
		goAhead = w
	}
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		park() // 멈추는 중 — 새 자식을 띄우지 않고 돌아오지도 않는다(부른 쪽이 곧 끝낸다)
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
		fmt.Fprintf(os.Stderr, "%s: %s 를 띄우지 못했다: %v\n", Who, c.Path, err)
		return 127
	}
	pid := c.Process.Pid
	curPG = pid
	curMu.Unlock()
	if goAhead != nil {
		st, serr := StartTime(pid)
		if serr != nil {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 자식 %d 의 시작 시각을 모른다(기록은 그대로, 다음 실행은 도는 것으로 친다): %v\n", Who, pid, serr)
		}
		if werr := atomic.WriteFile(record, []byte(fmt.Sprintf("%d\t%s\n", pid, st)), 0o666); werr != nil {
			// 기록 없이 띄우면 이 실행이 죽은 뒤 다음 실행이 도는 자식을 모른다 — 띄우지 않는다(gate가 명령 없이 끝난다)
			goAhead.Close()
			c.Wait()
			curMu.Lock()
			curPG = 0
			curMu.Unlock()
			fmt.Fprintf(os.Stderr, "%s: 자식 기록을 쓰지 못해 띄우지 않았다: %v\n", Who, werr)
			return 127
		}
		goAhead.WriteString("go\n")
		goAhead.Close()
	}
	var timedOut sync.WaitGroup
	fired := false
	var t *time.Timer
	if max > 0 {
		timedOut.Add(1)
		t = time.AfterFunc(max, func() {
			defer timedOut.Done()
			curMu.Lock()
			fired = true
			curMu.Unlock()
			fmt.Fprintf(os.Stderr, "%s: %s\n", Who, &TimeoutError{Head: head(c.Args), D: max})
			StopGroup(pid, 5*time.Second)
		})
	}
	rc := Code(c.Wait())
	if t != nil && t.Stop() {
		timedOut.Done() // 타이머가 돌지 않았다
	}
	timedOut.Wait() // 돈 타이머는 무리를 다 거둔 뒤에
	curMu.Lock()
	curPG = 0
	if stopping {
		curMu.Unlock()
		park() // 신호로 끈 자식의 종료 코드를 부른 쪽이 결과(착지 실패 등)로 읽지 않게 돌아오지 않는다
	}
	if fired {
		rc = TimeoutCode // 상한이 먼저 — 자식이 TERM을 받고 0으로 끝냈어도 결과는 모름
	}
	curMu.Unlock()
	if record != "" {
		if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 자식 기록을 지우지 못했다: %v\n", Who, err)
		}
	}
	return rc
}

// StopCurrent: 지금 기다리는 무리(Wait)와 상한으로 기다리는 무리(Run)를 모두 끈다(StopGroup) — 신호를 받아 끝내기 직전에 부르고,
// 부른 쪽은 곧 끝낸다. 그 뒤로 Wait·Run은 돌아오지 않는다(기다리던 것도, 새로 부른 것도 — 새 자식은 띄우지 않는다)
func StopCurrent(grace time.Duration) {
	curMu.Lock()
	stopping = true
	cur := curPG
	bs := map[int]string{}
	for pg, st := range bounded {
		bs[pg] = st
	}
	curMu.Unlock()
	var wg sync.WaitGroup
	if cur != 0 {
		wg.Add(1)
		go func() { defer wg.Done(); StopGroup(cur, grace) }()
	}
	for pg, st := range bs {
		wg.Add(1)
		go func(pg int, st string) {
			defer wg.Done()
			stopOwned(pg, st, grace)
		}(pg, st)
	}
	wg.Wait()
}

// StopGroup: 무리 pg에 TERM, grace 안에 다 끝나지 않으면 KILL
func StopGroup(pg int, grace time.Duration) {
	if err := killGroup(pg, sigTerm); err != nil {
		return
	}
	for end := time.Now().Add(grace); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if groupGone(pg) {
			return
		}
	}
	if err := killGroup(pg, sigKill); err != nil && !isNoProcess(err) {
		fmt.Fprintf(os.Stderr, "%s: 자식 무리 %d 를 끄지 못했다: %v\n", Who, pg, err)
		return
	}
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end) && !groupGone(pg); time.Sleep(50 * time.Millisecond) {
		// KILL 뒤 거둬질 때까지(남의 자식인 좀비는 init이 거둔다) — 부른 쪽이 「끝났음」을 바로 본다
	}
}

// stopOwned: 무리 pg가 기록한 그것(우두머리 시작 시각 start)일 때만 끈다 — 우두머리가 끝나 번호가 다른 무리에 다시 쓰였으면 건드리지 않는다.
// 끝나면 고아 기록도 지운다
func stopOwned(pg int, start string, grace time.Duration) {
	switch ownGroup(pg, start) {
	case groupMine:
		StopGroup(pg, grace)
		if groupGone(pg) {
			unrecord(pg)
		}
	case groupNone:
		unrecord(pg)
	}
}

type groupState int

const (
	groupUnknown groupState = iota // ps를 못 읽음 — 건드리지 않는다
	groupMine                      // 그 무리가 기록한 그것이다
	groupNone                      // 그 무리가 없거나 다른 것이 번호를 쓴다
)

// ownGroup: 무리 pg가 기록한 그것인가. 무리가 살아 있는 동안 그 번호는 다시 쓰이지 않으니(POSIX) 우두머리가 끝났어도 남은 무리는 그것이다.
// 우두머리가 살아 있으면 시작 시각을 견준다(start가 비면 우리가 막 띄운 것으로 친다)
func ownGroup(pg int, start string) groupState {
	if pg <= 1 || groupGone(pg) {
		return groupNone
	}
	if !Alive(pg) {
		return groupMine
	}
	if start == "" {
		return groupMine
	}
	st, err := StartTime(pg)
	if err != nil {
		return groupUnknown
	}
	if st != start {
		return groupNone
	}
	return groupMine
}

// psPath: 절대 경로(PATH에 /bin이 없어도) — 시험이 바꾼다
var psPath = "/bin/ps"

// StartTime: 그 pid의 시작 시각(ps lstart) — pid 재사용 검사용. ps를 못 부르거나 그 pid가 없으면 오류(모름 — 「다른 프로세스」로 읽지 않는다)
func StartTime(pid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, psPath, "-o", "lstart=", "-p", strconv.Itoa(pid))
	c.Env = append(os.Environ(), "LC_ALL=C")
	c.WaitDelay = time.Second
	b, err := c.Output()
	s := strings.TrimSpace(string(b))
	if err != nil || s == "" {
		if err == nil {
			err = errors.New("빈 출력")
		}
		return "", fmt.Errorf("ps %d: %w", pid, err)
	}
	return s, nil
}

// ChildBusy: 앞 실행이 적어 둔 자식(record)이 아직 도는가. 그 pid가 기록한 그 프로세스면 참(지우지 않고 기다린다).
// 끝났으면 그 무리에 남은 것(죽은 실행의 자식이 남긴 배경 프로세스)을 거두고 기록을 지운다.
// pid가 다른 프로세스에 다시 쓰였으면 기록만 지운다. 모르면(기록·ps를 못 읽음) 참 — 도는 착지 밑에서 치우는 것보다 기다리는 쪽이 안전하다
func ChildBusy(record string, grace time.Duration) bool {
	b, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: 자식 기록을 읽지 못했다(도는 것으로 친다): %v\n", Who, err)
		return true
	}
	f := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 2)
	pid, err := strconv.Atoi(f[0])
	if err == nil && pid > 1 {
		start := ""
		if len(f) == 2 {
			start = f[1]
		}
		if Alive(pid) {
			if start == "" {
				return true // 시작 시각 없이 적힌 기록 — 확인할 수 없어 도는 것으로 친다
			}
			st, serr := StartTime(pid)
			if serr != nil {
				fmt.Fprintf(os.Stderr, "%s: 자식 %d 의 시작 시각을 읽지 못했다(도는 것으로 친다): %v\n", Who, pid, serr)
				return true
			}
			if st == start {
				return true
			}
		} else if !groupGone(pid) {
			// 무리 번호는 그 무리가 살아 있는 동안 다시 쓰이지 않는다(POSIX) — 우두머리가 끝난 무리는 그 자식의 것이다
			fmt.Fprintf(os.Stderr, "%s: 앞 실행의 착지 무리 %d 에 남은 프로세스를 거둔다\n", Who, pid)
			StopGroup(pid, grace)
			if !groupGone(pid) {
				return true
			}
		}
	}
	if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "%s: 자식 기록을 지우지 못했다(도는 것으로 친다): %v\n", Who, err)
		return true
	}
	return false
}
