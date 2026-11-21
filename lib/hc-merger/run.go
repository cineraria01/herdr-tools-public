package main

// 외부 명령 부르기: git·hc·hc-lane·mv·sleep·ps. bash 판과 같은 명령·같은 인자 꼴로 부른다 — 계약 시험이 PATH의 가짜 git·mv·sleep로
// 끊김을 흉내 내고, 인자 꼴로 어느 호출인지 가린다(tests/hc-lane-merger*.test.sh).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// code: 명령 결과를 셸 종료 코드로 — 신호로 끝나면 128+신호, 못 띄우면 127
func code(err error) int {
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
	fmt.Fprintf(os.Stderr, "hc-lane merger: 명령을 띄우지 못했다: %v\n", err)
	return 127
}

// gitc: git을 dir(빈 값이면 지금 폴더)에서. stdout·stderr가 nil이면 버린다. 표준 입력은 /dev/null
func gitc(dir string, stdout, stderr io.Writer, args ...string) int {
	c := exec.Command("git", args...)
	c.Dir, c.Stdout, c.Stderr = dir, stdout, stderr
	return code(c.Run())
}

// run: 출력은 화면 그대로 · runQ: 출력 모두 버림(>/dev/null 2>&1) · runO: 표준 출력만 버림
func run(args ...string) int  { return gitc("", os.Stdout, os.Stderr, args...) }
func runQ(args ...string) int { return gitc("", nil, nil, args...) }
func runO(args ...string) int { return gitc("", nil, os.Stderr, args...) }

// out: $(git …) — 표준 출력(끝 줄바꿈 뺌)과 종료 코드, 오류 출력은 화면 · outQ: 오류 출력도 버림(2>/dev/null)
func out(args ...string) (string, int)  { return capture(os.Stderr, args...) }
func outQ(args ...string) (string, int) { return capture(nil, args...) }

func capture(stderr io.Writer, args ...string) (string, int) {
	var b bytes.Buffer
	rc := gitc("", &b, stderr, args...)
	return strings.TrimRight(b.String(), "\n"), rc
}

// inLog: git을 dir에서, 출력은 로그 파일 뒤에(>>"$lgf_" 2>&1)
func inLog(dir, log string, args ...string) int {
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 로그를 열지 못했다: %v\n", err)
		return 1
	}
	defer f.Close()
	return gitc(dir, f, f, args...)
}

var lockContention = regexp.MustCompile(`cannot lock ref|unable to update local ref|is at [0-9a-f]+ but expected|로컬 레퍼런스를 업데이트할 수 없습니다|\.lock'을 만들 수 없습니다`)

// fetchr: git fetch를 돌리고, 같은 클론의 다른 fetch·착지와 참조 잠금이 겹쳐 난 실패면 HC_LANE_FETCH_WAITS(기본 1 2 4초) 쉬고 다시
// (bash fetchr와 같음). quiet = 2>/dev/null
func fetchr(quiet bool, args ...string) int {
	var errw io.Writer = os.Stderr
	if quiet {
		errw = io.Discard
	}
	waits := strings.Fields(os.Getenv("HC_LANE_FETCH_WAITS"))
	if _, set := os.LookupEnv("HC_LANE_FETCH_WAITS"); !set {
		waits = []string{"1", "2", "4"}
	}
	waits = append(waits, "-")
	rc := 0
	for _, w := range waits {
		var e bytes.Buffer
		rc = gitc("", os.Stdout, &e, append([]string{"fetch"}, args...)...)
		errw.Write(e.Bytes())
		if rc == 0 || w == "-" || !lockContention.Match(e.Bytes()) {
			break
		}
		fmt.Fprintf(errw, "hc-lane: git fetch 참조 잠금 경합(같은 클론의 다른 fetch·착지) — %s초 뒤 다시\n", w)
		if s, err := strconv.ParseFloat(w, 64); err == nil {
			time.Sleep(time.Duration(s * float64(time.Second)))
		}
	}
	return rc
}

// place: tmp를 dst로 바꿔 넣는다 — bash 판처럼 mv -f 명령으로(계약 시험이 PATH의 mv로 이 자리의 끊김을 흉내 낸다)
func place(tmp, dst string) error {
	c := exec.Command("mv", "-f", tmp, dst)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("mv %s: %w", dst, err)
	}
	return nil
}

// writeVia: dst.tmp에 다 쓴 뒤 바꿔 넣는다(덮어쓰다 끊겨 반쪽 파일이 남지 않게)
func writeVia(dst, content string) error {
	if err := os.WriteFile(dst+".tmp", []byte(content), 0o666); err != nil {
		return fmt.Errorf("쓰기 %s.tmp: %w", dst, err)
	}
	return place(dst+".tmp", dst)
}

// ---- 자식 프로세스 무리(요구 1: 자기가 띄운 프로세스만 소유) ----
// 착지·준비 훅·바퀴 일꾼·빈 바퀴 sleep은 새 프로세스 무리(Setpgid)로 띄운다. 멈출 때(TERM·INT·HUP) 그 무리째 TERM → 유예 → KILL.
// 착지가 정상으로 끝난 뒤 무리에 남은 배경 프로세스(검증이 남긴 고아)는 건드리지 않는다(계약 시험 30a — 그 정리는 저장소 검증 몫).

var (
	curMu sync.Mutex
	curPG int // 지금 기다리는 자식 무리(0 = 없음)
)

// waitGroup: c를 새 무리로 띄워 끝날 때까지 기다린다. record가 있으면 그 파일에 「pid<탭>시작 시각」을 적어 두고 끝나면 지운다 —
// 이 실행이 죽어도 다음 실행이 그 착지가 아직 도는지(pid + 시작 시각) 본다(childBusy)
func waitGroup(c *exec.Cmd, record string) int {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	curMu.Lock()
	if err := c.Start(); err != nil {
		curMu.Unlock()
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 를 띄우지 못했다: %v\n", c.Path, err)
		return 127
	}
	pid := c.Process.Pid
	curPG = pid
	curMu.Unlock()
	if record != "" {
		if err := os.WriteFile(record, []byte(fmt.Sprintf("%d\t%s\n", pid, startTime(pid))), 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 경고 — 자식 기록을 쓰지 못했다: %v\n", err)
		}
	}
	rc := code(c.Wait())
	curMu.Lock()
	curPG = 0
	curMu.Unlock()
	if record != "" {
		if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 경고 — 자식 기록을 지우지 못했다: %v\n", err)
		}
	}
	return rc
}

// stopGroup: 무리 pg에 TERM, grace 안에 다 끝나지 않으면 KILL
func stopGroup(pg int, grace time.Duration) {
	if err := syscall.Kill(-pg, syscall.SIGTERM); err != nil {
		return
	}
	for end := time.Now().Add(grace); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if syscall.Kill(-pg, 0) == syscall.ESRCH {
			return
		}
	}
	if err := syscall.Kill(-pg, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 자식 무리 %d 를 끄지 못했다: %v\n", pg, err)
	}
}

// startTime: 그 pid의 시작 시각(ps lstart) — pid 재사용 검사용. 모르면 빈 값
func startTime(pid int) string {
	c := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	c.Env = append(os.Environ(), "LC_ALL=C")
	b, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// childBusy: 앞 실행이 적어 둔 착지 자식(record)이 아직 도는가. 그 pid가 살아 있고 시작 시각이 같으면 참(그 착지가 후보 작업 트리를
// 쓰는 중 — 지우지 않고 기다린다). 끝났으면 그 무리에 남은 것(죽은 병합기의 착지가 남긴 배경 프로세스)을 거두고 기록을 지운다.
// pid가 다른 프로세스에 다시 쓰였으면 기록만 지운다
func childBusy(record string, grace time.Duration) bool {
	b, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 자식 기록을 읽지 못했다(도는 것으로 친다): %v\n", err)
		return true
	}
	f := strings.SplitN(strings.TrimRight(string(b), "\n"), "\t", 2)
	pid, err := strconv.Atoi(f[0])
	if err == nil && pid > 1 {
		start := ""
		if len(f) == 2 {
			start = f[1]
		}
		if alive(pid) {
			// 시작 시각을 못 적었으면(띄운 직후 ps 실패) 도는 것으로 친다 — 후보를 지우는 쪽보다 기다리는 쪽이 안전하다
			if start == "" || startTime(pid) == start {
				return true
			}
		} else if syscall.Kill(-pid, 0) == nil {
			// ponytail: 무리 번호는 그 무리가 살아 있는 동안 다시 쓰이지 않는다(POSIX). 우두머리가 끝난 뒤 그 번호로 새 무리가 생겨
			// 우두머리마저 끝난 드문 경우는 가리지 못한다 — 생기면 무리 안 프로세스의 시작 시각도 견준다
			fmt.Fprintf(os.Stderr, "hc-lane merger: 앞 실행의 착지 무리 %d 에 남은 프로세스를 거둔다\n", pid)
			stopGroup(pid, grace)
		}
	}
	if err := os.Remove(record); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 자식 기록을 지우지 못했다(도는 것으로 친다): %v\n", err)
		return true
	}
	return false
}
