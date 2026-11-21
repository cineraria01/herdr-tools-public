// Package proc: 자식 프로세스를 새 프로세스 무리(Setpgid)로 띄우고, 소유를 pid + 시작 시각으로 확인하고, 무리째 거둔다.
// 병합기 Go 판(cmd/hc-merger)에서 끌어올렸다 — 자기가 띄운 프로세스만 소유한다(2026-11-21 병합기 결함: 잠금·자식이 남아 다음 실행을 막음).
package proc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Who: 경고 줄 앞에 붙는 이름(도구가 main에서 정한다)
var Who = "herdr-tools"

// Code: 명령 결과를 셸 종료 코드로 — 신호로 끝나면 128+신호, 못 띄우면 127
func Code(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "%s: 명령을 띄우지 못했다: %v\n", Who, err)
	return 127
}

var (
	curMu    sync.Mutex
	curPG    int  // 지금 기다리는 자식 무리(0 = 없음)
	stopping bool // StopCurrent 뒤 — 새 자식을 띄우지 않는다
)

// Wait: c를 새 무리로 띄워 끝날 때까지 기다리고 종료 코드를 돌려준다. record가 있으면 그 파일에 「pid<탭>시작 시각」을 적어 두고
// 끝나면 지운다 — 이 실행이 죽어도 다음 실행이 그 자식이 아직 도는지 본다(ChildBusy). 기다리는 동안의 무리는 StopCurrent가 끈다
func Wait(c *exec.Cmd, record string) int {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	curMu.Lock()
	if stopping {
		curMu.Unlock()
		select {} // 멈추는 중 — 새 자식을 띄우지 않고 돌아오지도 않는다(부른 쪽이 곧 끝낸다)
	}
	if err := c.Start(); err != nil {
		curMu.Unlock()
		fmt.Fprintf(os.Stderr, "%s: %s 를 띄우지 못했다: %v\n", Who, c.Path, err)
		return 127
	}
	pid := c.Process.Pid
	curPG = pid
	curMu.Unlock()
	if record != "" {
		if err := os.WriteFile(record, []byte(fmt.Sprintf("%d\t%s\n", pid, StartTime(pid))), 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 자식 기록을 쓰지 못했다: %v\n", Who, err)
		}
	}
	rc := Code(c.Wait())
	curMu.Lock()
	curPG = 0
	if stopping {
		curMu.Unlock()
		select {} // 신호로 끈 자식의 종료 코드를 부른 쪽이 결과(착지 실패 등)로 읽지 않게 돌아오지 않는다
	}
	curMu.Unlock()
	if record != "" {
		if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "%s: 경고 — 자식 기록을 지우지 못했다: %v\n", Who, err)
		}
	}
	return rc
}

// StopCurrent: 지금 기다리는 무리를 끈다(StopGroup) — 신호를 받아 끝내기 직전에 부르고, 부른 쪽은 곧 os.Exit 한다.
// 그 뒤로 Wait는 돌아오지 않는다(기다리던 Wait도, 새로 부른 Wait도 — 새 자식은 띄우지 않는다)
func StopCurrent(grace time.Duration) {
	curMu.Lock()
	stopping = true
	pg := curPG
	curMu.Unlock()
	if pg != 0 {
		StopGroup(pg, grace)
	}
}

// StopGroup: 무리 pg에 TERM, grace 안에 다 끝나지 않으면 KILL
func StopGroup(pg int, grace time.Duration) {
	if err := syscall.Kill(-pg, syscall.SIGTERM); err != nil {
		return
	}
	for end := time.Now().Add(grace); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if syscall.Kill(-pg, 0) == syscall.ESRCH {
			return
		}
	}
	if err := syscall.Kill(-pg, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		fmt.Fprintf(os.Stderr, "%s: 자식 무리 %d 를 끄지 못했다: %v\n", Who, pg, err)
	}
}

// StartTime: 그 pid의 시작 시각(ps lstart) — pid 재사용 검사용. 모르면 빈 값
func StartTime(pid int) string {
	c := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	c.Env = append(os.Environ(), "LC_ALL=C")
	b, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Alive: 그 pid가 있는가(남의 것이라 신호를 못 보내도 있음)
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Owned: pid가 살아 있고 시작 시각이 start와 같은가(기록한 그 프로세스가 맞는가). start가 비면 확인할 수 없어 참으로 친다 —
// 남의 것을 거두는 쪽보다 기다리는 쪽이 안전하다
func Owned(pid int, start string) bool {
	return Alive(pid) && (start == "" || StartTime(pid) == start)
}

// ChildBusy: 앞 실행이 적어 둔 자식(record)이 아직 도는가. 그 pid가 기록한 그 프로세스면 참(지우지 않고 기다린다).
// 끝났으면 그 무리에 남은 것(죽은 실행의 자식이 남긴 배경 프로세스)을 거두고 기록을 지운다.
// pid가 다른 프로세스에 다시 쓰였으면 기록만 지운다
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
			if Owned(pid, start) {
				return true
			}
		} else if syscall.Kill(-pid, 0) == nil {
			// ponytail: 무리 번호는 그 무리가 살아 있는 동안 다시 쓰이지 않는다(POSIX). 우두머리가 끝난 뒤 그 번호로 새 무리가 생겨
			// 우두머리마저 끝난 드문 경우는 가리지 못한다 — 생기면 무리 안 프로세스의 시작 시각도 견준다
			fmt.Fprintf(os.Stderr, "%s: 앞 실행의 착지 무리 %d 에 남은 프로세스를 거둔다\n", Who, pid)
			StopGroup(pid, grace)
		}
	}
	if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "%s: 자식 기록을 지우지 못했다(도는 것으로 친다): %v\n", Who, err)
		return true
	}
	return false
}
