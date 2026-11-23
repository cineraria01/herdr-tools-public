// Package oplog: 도구가 겪은 이상(die·경고·시간 넘김·모름·0이 아닌 종료·훅 패닉)을 운영 기록 한 줄로 남긴다(2026-11-23 ht-oplog).
// 자리 ${XDG_STATE_HOME:-~/.local/state}/herdr-tools/ops.tsv(시험·다른 자리는 HERDR_TOOLS_OPLOG=<파일>). 한 줄 8칸(탭):
// 시각(UTC) · 도구 · 하위 명령 · pid · 종류 · 종료 코드(없으면 -) · cwd(뒤 64바이트) · 글. 줄은 1000바이트 안, 쓰기는 한 번의 O_APPEND
// (여러 프로세스가 함께 써도 줄이 섞이지 않는다). 4MB를 넘으면 쓰기 전에 ops.tsv.1로 돌리고 한 세대만 둔다.
// 기록은 곁에 더할 뿐이다 — 못 써도 도구의 출력·종료 코드는 그대로이고, 실패는 표준 오류에 프로세스마다 한 번만 알린다.
// 글은 도구가 이미 표준 오류에 쓴 글 그대로(환경 변수 값·전체 argv는 적지 않는다). 칸 꼴은 docs/go/oplog.md
package oplog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"herdr-tools/internal/events"
)

// 종류
const (
	Die     = "die"     // 멈춤(도구의 die)
	Warn    = "warn"    // 경고·계속
	Timeout = "timeout" // 바깥 명령 시간 넘김(모름)
	Refuse  = "refuse"  // 거절·띄우지 않음(모름)
	Exit    = "exit"    // 0이 아닌 종료(die·refuse 줄이 없을 때)
	Panic   = "panic"   // 잡은 패닉
)

const (
	Env     = "HERDR_TOOLS_OPLOG" // 기록 파일 경로(시험용 — XDG_STATE_HOME을 바꾸지 않고)
	MaxLine = events.MaxLine      // 줄 하나의 바이트 상한(줄바꿈 빼고)
	MaxSize = 4 << 20             // 넘으면 쓰기 전에 .1로 돌린다
)

var (
	failOnce sync.Once
	ended    atomic.Bool // 도구가 끝내는 줄(Fatal)을 남겼거나 사용법으로 끝난다(Usage) — 뒤따르는 Exited는 남기지 않는다
)

// 시험 바이너리(go test가 지은 <꾸러미>.test)는 지정이 없으면 기록을 /dev/null로 — 환경 값이라 그 시험이 띄운 자식 바이너리도
// 물려받는다(감사 atoy7cz-120azh M1: 시험이 지어 띄운 hc-lane이 실제 ~/.local/state에 쌓았다). 기록을 보려는 시험은 경로를 준다
func init() {
	if os.Getenv(Env) == "" && testBinary(os.Args[0]) {
		os.Setenv(Env, os.DevNull)
	}
}

func testBinary(name string) bool {
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe")
}

// Path: 기록 파일 경로
func Path() (string, error) {
	if p := os.Getenv(Env); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("집 폴더를 모름: %w", err)
		}
		base = filepath.Join(h, ".local", "state")
	}
	return filepath.Join(base, "herdr-tools", "ops.tsv"), nil
}

// sub: 하위 명령 칸 — argv[1](32바이트) 하나뿐. 없거나 「=」가 든 옵션(값이 붙음)이면 -
func sub(args []string) string {
	if len(args) < 2 || args[1] == "" || strings.Contains(args[1], "=") {
		return "-"
	}
	return events.Bcut(32, events.Flat(args[1]))
}

// Log: 한 줄을 남긴다. sub가 비면 argv[1], code가 음수면 -. 계속 도는 알림(proc의 경고·시간 넘김·모름)은 이것
func Log(tool, subcmd, kind string, code int, msg string) {
	if err := write(tool, subcmd, kind, code, msg); err != nil {
		failOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "%s: 운영 기록을 남기지 못했다(동작은 그대로): %v\n", tool, err)
		})
	}
}

// Fatal: 도구가 끝내는 줄(die·refuse) — Log 뒤 이 프로세스의 Exited를 끈다(같은 종료를 두 줄로 남기지 않게)
func Fatal(tool, subcmd, kind string, code int, msg string) {
	ended.Store(true)
	Log(tool, subcmd, kind, code, msg)
}

// Usage: 사용법(-h·--help·인자 오류의 사용법 글)으로 끝난다 — 이상이 아니라 Exited가 남기지 않는다(감사 L1)
func Usage() { ended.Store(true) }

// Exited: 0이 아닌 종료 한 줄 — 이 프로세스가 이미 Fatal 줄을 남겼거나 Usage면 건너뛴다.
// ponytail: 프로세스 하나에 한 번 — 늘 도는 watch가 잠금 안 die 뒤 같은 프로세스로 계속 돌면 그 뒤 종료 줄은 빠진다(launchd --log가 남긴다)
func Exited(tool string, code int) {
	if code != 0 && !ended.Load() {
		Log(tool, "", Exit, code, "종료 "+strconv.Itoa(code))
	}
}

// tail64: 뒤 64바이트(경로는 끝 — 레인 폴더 이름 — 이 알려 준다). 앞에 남은 반쪽 글자는 버린다
func tail64(s string) string {
	if len(s) <= 64 {
		return s
	}
	s = s[len(s)-64:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

func write(tool, subcmd, kind string, code int, msg string) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if subcmd == "" {
		subcmd = sub(os.Args)
	}
	c := "-"
	if code >= 0 {
		c = strconv.Itoa(code)
	}
	wd, _ := os.Getwd() // 모르면 빈 칸(-)
	if wd == "" {
		wd = "-"
	}
	head := strings.Join([]string{events.UTC(time.Now()), events.Bcut(64, events.Flat(tool)), events.Flat(subcmd),
		strconv.Itoa(os.Getpid()), kind, c, tail64(events.Flat(wd))}, "\t") + "\t"
	line := head + events.Bcut(MaxLine-len(head), events.Flat(msg))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	// ponytail: 두 프로세스가 함께 돌리면 앞 세대 하나가 덮일 수 있다 — 잃는 것은 4MB 넘은 옛 기록뿐.
	// 남이 먼저 돌려 파일이 없으면(ErrNotExist) 그대로 새 파일에 덧붙인다(감사 L3)
	if st, err := os.Stat(p); err == nil && st.Size() > MaxSize {
		if err := os.Rename(p, p+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return events.AppendLine(p, line, 0o600)
}
