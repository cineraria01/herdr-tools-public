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
	stopping bool                  // StopCurrent 뒤 — 새 자식을 띄우지 않는다
	bounded  = map[int]string{}    // 이 실행이 따로 무리로 띄워 기다리는 자식(Run·Start·Wait) → 그 시작 시각. 부른 쪽 무리를 꺼도 안 꺼지니 StopCurrent가 함께 끈다
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
// record와 상관없이 Run·Start처럼 고아 기록에 올린다(재감사 2바퀴 P3 — 옛 판은 record가 빈 자식을 기록 없이 띄워 이 실행이 SIGKILL로
// 죽으면 게시기·착지 뒤 명령·준비 훅이 아무도 모르게 남았다). 자식은 기록(고아 기록·record)이 다 쓰인 뒤에야 실제 명령을 시작하고(gate),
// 기록을 못 쓰면 띄우지 않는다(127). 상한이 있으면 준비(시작 시각·고아 확인)도 그 안에서 — 넘기면 TimeoutCode.
// 상한이 없으면(max 0 — 병합기 바퀴 일꾼·착지·게시기) 기록에 keep 표식: 이 실행이 죽어도 그 일은 끝까지 가고, 다음 실행은 거두지 않고
// 끝날 때까지 센다(Orphans — 75로 미룸). 상한이 있으면 Run처럼 다음 실행이 소유를 확인하고 거둔다.
// 우두머리가 정상으로 끝난 뒤 무리에 남은 배경 프로세스는 신호 없이 놓아 준다(옛 판과 같음 — 검증이 남긴 배경 프로세스가 다음 실행을
// 막지 않게, 그 정리는 hc-sweep 고아 몫)
func WaitMax(c *exec.Cmd, record string, max time.Duration) int {
	setGroup(c)
	grouped := isGrouped(c)
	var deadline time.Time
	if max > 0 {
		deadline = time.Now().Add(max)
	}
	var extra func(pid int, st string) error
	if record != "" && grouped {
		extra = func(pid int, st string) error {
			return atomic.WriteFile(record, []byte(fmt.Sprintf("%d\t%s\n", pid, st)), 0o666)
		}
	}
	path := c.Path
	if err := launch(c, grouped, deadline, head(c.Args), max, extra, max <= 0); err != nil {
		if past(deadline) {
			return TimeoutCode // 「모름 … 띄우지 않음」 줄은 launch가 적었다
		}
		fmt.Fprintf(os.Stderr, "%s: %s 를 띄우지 못했다: %v\n", Who, path, err)
		return 127
	}
	pid := c.Process.Pid
	var fired bool
	var t *time.Timer
	stopped := make(chan struct{})
	if max > 0 {
		t = time.AfterFunc(time.Until(deadline), func() {
			defer close(stopped)
			curMu.Lock()
			fired = true
			curMu.Unlock()
			fmt.Fprintf(os.Stderr, "%s: %s\n", Who, &TimeoutError{Head: head(c.Args), D: max})
			if grouped {
				Stop(pid, 5*time.Second)
			} else {
				c.Process.Kill() // 무리가 없는 곳(Windows) — 그 프로세스 하나만
			}
		})
	}
	rc := Code(c.Wait())
	leaderDone(pid, grouped)
	if t != nil && !t.Stop() {
		<-stopped // 돈 타이머는 무리를 다 거둔 뒤에
	}
	if grouped {
		drop(pid, true) // 우두머리를 거뒀다 — 기록·bounded에서 내린다(남은 배경 프로세스에는 신호하지 않는다)
	}
	if record != "" {
		if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 자식 기록을 지우지 못했다: %v\n", Who, err)
		}
	}
	finish(pid) // 멈추는 중이면 돌아오지 않는다 — 신호로 끈 자식의 종료 코드를 부른 쪽이 결과(착지 실패 등)로 읽지 않게
	curMu.Lock()
	if fired {
		rc = TimeoutCode // 상한이 먼저 — 자식이 TERM을 받고 0으로 끝냈어도 결과는 모름
	}
	curMu.Unlock()
	return rc
}

// StopCurrent: 이 실행이 띄워 기다리는 무리(Run·Start·Wait)를 모두 끈다(StopGroup) — 신호를 받아 끝내기 직전에 부르고,
// 부른 쪽은 곧 끝낸다. 그 뒤로 Wait·Run은 돌아오지 않는다(기다리던 것도, 새로 부른 것도 — 새 자식은 띄우지 않는다)
func StopCurrent(grace time.Duration) {
	curMu.Lock()
	stopping = true
	bs := map[int]string{}
	for pg, st := range bounded {
		bs[pg] = st
	}
	curMu.Unlock()
	var wg sync.WaitGroup
	for pg, st := range bs {
		wg.Add(1)
		go func(pg int, st string) {
			defer wg.Done()
			stopOwned(pg, st, grace, true)
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
// live = 이 실행이 띄운 무리(bounded). 무리가 사라졌으면 고아 기록을 지우고 live면 bounded에서도 내린다(그 뒤로는 그 번호를 그것이라
// 하지 않는다 — 재감사 2바퀴 P5·merger R-207)
func stopOwned(pg int, start string, grace time.Duration, live bool) {
	switch ownGroup(pg, start, live) {
	case groupMine:
		StopGroup(pg, grace)
		if groupGone(pg) {
			drop(pg, live)
		}
	case groupNone:
		drop(pg, live)
	}
}

// drop: 사라진 무리 pg의 고아 기록을 지우고, live면 bounded에서 내린다
func drop(pg int, live bool) {
	unrecord(pg)
	if live {
		curMu.Lock()
		delete(bounded, pg)
		curMu.Unlock()
	}
}

type groupState int

const (
	groupUnknown groupState = iota // ps를 못 읽음·소유를 입증 못 함 — 건드리지 않는다
	groupMine                      // 그 무리가 기록한 그것이다
	groupNone                      // 그 무리가 없거나 다른 것이 번호를 쓴다
)

// ownGroup: 무리 pg가 기록한 그것인가(마감 없이 — ownGroupBy)
func ownGroup(pg int, start string, live bool) groupState {
	return ownGroupBy(pg, start, live, time.Time{})
}

// ownGroupBy: 무리 pg가 기록한 그것인가. live(이 실행이 띄워 bounded에 올린 무리)와 디스크 기록(앞 실행이 남김)을 가른다(재감사 R-04·R-C02·R-07).
//   - live: 우두머리는 이 실행의 자식이다 — 거두기 전에는 그 번호를 다른 프로세스가 못 쓰고, 거둔 뒤에도 무리가 남은 동안 번호는 그 무리의
//     것이다(POSIX). 그래서 bounded에 있고 무리가 있으면 ps 없이 그것이다(시간 넘김 때 ps가 실패해도 거둔다 — R-C01). 거둔 뒤 무리가
//     사라지면 곧바로 bounded에서 내려(leaderDone·drop) 그 뒤 같은 번호를 숫자만으로 그것이라 하지 않는다(2바퀴 P5)
//   - 디스크 기록: 기록한 실행이 죽은 뒤에는 무리 소멸·번호 재사용·새 우두머리 퇴장을 가르지 못한다 — 우두머리가 살아 있고 기록한 시작
//     시각과 같을 때만 그것이다. 우두머리가 끝났거나 시작 시각이 비었으면 모름(신호를 보류하고 기록을 둔다). 시작 시각 조회는 deadline 안
func ownGroupBy(pg int, start string, live bool, deadline time.Time) groupState {
	if pg <= 1 || groupGone(pg) {
		return groupNone
	}
	if live {
		curMu.Lock()
		_, ok := bounded[pg]
		curMu.Unlock()
		if ok {
			return groupMine
		}
		return groupUnknown // 이미 내린 번호 — 숫자만으로는 그것이 아니다
	}
	if !Alive(pg) || start == "" {
		return groupUnknown
	}
	st, err := startTimeBy(pg, deadline)
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
func StartTime(pid int) (string, error) { return startTimeBy(pid, time.Time{}) }

// startTimeBy: StartTime을 deadline 안에서(빈 값이면 20초) — 상한 걸린 Run의 준비도 같은 남은 시간을 쓴다(재감사 R-C05).
// ps는 따로 무리로 띄워 마감이면 무리째 KILL(관을 쥔 손자까지)
func startTimeBy(pid int, deadline time.Time) (string, error) {
	d := 20 * time.Second
	if !deadline.IsZero() {
		if r := time.Until(deadline); r < d {
			d = r
		}
	}
	if d <= 0 {
		return "", fmt.Errorf("ps %d: %w", pid, ErrTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	c := exec.CommandContext(ctx, psPath, "-o", "lstart=", "-p", strconv.Itoa(pid))
	c.Env = append(os.Environ(), "LC_ALL=C")
	setGroup(c)
	c.Cancel = func() error { return killGroup(c.Process.Pid, sigKill) }
	c.WaitDelay = 200 * time.Millisecond
	b, err := c.Output()
	s := strings.TrimSpace(string(b))
	if err != nil || s == "" {
		if ctx.Err() != nil {
			err = ErrTimeout
		} else if err == nil {
			err = errors.New("빈 출력")
		}
		return "", fmt.Errorf("ps %d: %w", pid, err)
	}
	return s, nil
}

// ChildBusy: 앞 실행이 적어 둔 자식(record)이 아직 도는가. 그 pid가 기록한 그 프로세스면 참(지우지 않고 기다린다).
// 우두머리가 끝났는데 그 무리가 남았으면 소유를 입증할 수 없다 — 신호 없이 참(기록 그대로, 무리가 사라지면 다음 부름이 지운다 —
// 재감사 2바퀴 P5·merger R-207, 옛 판은 시작 시각 없이 그 번호의 무리를 거뒀다). 무리까지 사라졌거나 pid가 다른 프로세스에 다시
// 쓰였으면 기록만 지운다. 모르면(기록·ps를 못 읽음·꼴이 틀림) 참 — 도는 착지 밑에서 치우는 것보다 기다리는 쪽이 안전하다
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
	if err != nil || pid <= 1 {
		fmt.Fprintf(os.Stderr, "%s: 자식 기록 %s 의 꼴이 틀렸다(도는 것으로 친다)\n", Who, record)
		return true
	}
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
		fmt.Fprintf(os.Stderr, "%s: 앞 실행의 자식 %d 는 끝났는데 그 무리가 남았다 — 소유를 입증할 수 없어 신호 없이 기다린다\n", Who, pid)
		return true
	}
	if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "%s: 자식 기록을 지우지 못했다(도는 것으로 친다): %v\n", Who, err)
		return true
	}
	return false
}
