// Package events: hc-lane 사건 줄(…/hc-lane/events.tsv) 읽고 쓰기 — 지금 꼴 그대로(lib/hc-lane/report.sh·common.sh 「report」).
// 한 줄 12칸(탭): 시각(UTC)·기계·터미널·레인·상태·sha·한 줄·turn·--to·발행 실행·수신 지휘·키. 옛 7~11칸 줄도 읽는다.
// 쓰기는 한 번의 O_APPEND, 줄은 1000바이트 안(여러 쓰는 쪽이 함께 더해도 줄이 섞이지 않게).
package events

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxLine: 줄 하나의 바이트 상한(줄바꿈 빼고)
const MaxLine = 1000

// Event: 사건 한 줄. 빈 칸은 쓸 때 「-」
type Event struct {
	Time, Machine, Term, Lane, State, SHA, Msg string
	Turn, To, Run, Owner, Key                  string
}

// Flat: 탭·CR·줄바꿈을 빈칸으로(칸이 늘지 않게)
func Flat(s string) string { return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s) }

// Bcut: 바이트로 자르고 끝에 남은 반쪽 UTF-8 글자(최대 3바이트)만 버린다. n이 0 이하면 빈 글.
// 셸 판 bcut(iconv -c)과의 의도된 차이: 셸은 가운데 깨진 바이트도 빼고, 다 빠져 비면 자른 바이트를 그대로 둔다 — 여기는 받은 바이트를 그대로 둔다
func Bcut(n int, s string) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	s = s[:n]
	for i := len(s) - 1; i >= 0 && i >= len(s)-3; i-- {
		if utf8.RuneStart(s[i]) {
			if !utf8.FullRuneInString(s[i:]) {
				s = s[:i]
			}
			break
		}
	}
	return s
}

// UTC: 사건 시각 꼴(2006-01-02T15:04:05Z)
func UTC(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// cols: 한 줄 칸(7칸)의 앞(탭으로 끝남)과 뒤(탭으로 시작). 기계·터미널·레인·지휘는 64바이트, 모든 칸은 탭·줄바꿈을 빈칸으로
func (e Event) cols() (head, tail string) {
	t := e.Time
	if t == "" {
		t = UTC(time.Now())
	}
	tail = "\t" + Flat(dash(e.Turn)) + "\t" + Flat(dash(e.To)) + "\t" + Flat(dash(e.Run)) + "\t" + Bcut(64, Flat(dash(e.Owner))) + "\t" + Flat(dash(e.Key))
	head = Flat(t) + "\t" + Bcut(64, Flat(dash(e.Machine))) + "\t" + Bcut(64, Flat(dash(e.Term))) + "\t" + Bcut(64, Flat(dash(e.Lane))) +
		"\t" + Flat(e.State) + "\t" + Flat(dash(e.SHA)) + "\t"
	return head, tail
}

// Line: report와 같은 꼴의 한 줄(줄바꿈 없음). 늘 12칸이고 패닉하지 않는다 — 1000바이트에 맞춰 줄이는 것은 한 줄 칸(7칸)뿐.
// 다른 칸만으로 넘치면 한 줄 칸이 비고 줄이 1000바이트를 넘는다(쓰기 전에 Check가 막는다). 시각이 비면 지금
func (e Event) Line() string {
	head, tail := e.cols()
	return head + Bcut(MaxLine-len(head)-len(tail), Flat(dash(e.Msg))) + tail
}

// Check: 쓰기 전 입력 검사 — 줄이 그대로 지켜야 할 칸(시각·상태·sha·turn·--to·실행·지휘·키)에 탭·줄바꿈이 있거나,
// 한 줄 칸 1바이트도 남지 않게 길면(1000바이트) 오류. 이때 한 줄 칸을 줄여 맞추지 않는다(받는 쪽·키가 깨진 사건을 쓰지 않게)
func (e Event) Check() error {
	for _, c := range [][2]string{{"시각", e.Time}, {"상태", e.State}, {"sha", e.SHA}, {"turn", e.Turn}, {"--to", e.To},
		{"실행", e.Run}, {"지휘", e.Owner}, {"키", e.Key}} {
		if strings.ContainsAny(c[1], "\t\r\n") {
			return fmt.Errorf("사건 %s 칸에 탭·줄바꿈이 있다: %q", c[0], c[1])
		}
	}
	head, tail := e.cols()
	if n := len(head) + len(tail); n >= MaxLine {
		return fmt.Errorf("사건 줄이 한 줄 칸 없이 %d바이트(상한 %d) — --to·--key·실행 칸이 너무 길다", n, MaxLine)
	}
	return nil
}

// Parse: 한 줄을 읽는다. 7칸이 안 되면 false(옛 줄·깨진 줄)
func Parse(line string) (Event, bool) {
	c := strings.Split(strings.TrimRight(line, "\r\n"), "\t")
	if len(c) < 7 {
		return Event{}, false
	}
	for len(c) < 12 {
		c = append(c, "")
	}
	return Event{c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[8], c[9], c[10], c[11]}, true
}

// submitIDRe: hc-lane land 제출 ID 꼴 <기계>-<UTC yyyymmddThhmmss>-<pid>(cmd/hc-lane/submit.go)
var submitIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+-[0-9]{8}T[0-9]{6}-[0-9]+$`)

// IsSubmitID: s가 제출 ID 꼴인가
func IsSubmitID(s string) bool { return submitIDRe.MatchString(s) }

// SubmitID: 「제출」 사건의 제출 ID — 12칸 submit:<ID>(hc-lane land만 적는다 — report가 손 제출을 거절)를 먼저, 키가 없는
// 옛 줄은 한 줄 칸 첫 낱말이 ID 꼴일 때만. 손으로 적은 report 제출·다른 상태는 "" — 제출로 치지 않는다(2026-11-23 fix-inbox-submit-id)
func (e Event) SubmitID() string {
	if e.State != "제출" {
		return ""
	}
	if id, ok := strings.CutPrefix(e.Key, "submit:"); ok && id != "" {
		return id
	}
	if f := strings.Fields(e.Msg); len(f) > 0 && IsSubmitID(f[0]) {
		return f[0]
	}
	return ""
}

// Scan: r의 줄을 차례로 fn에. fn이 false면 멈춘다. 7칸이 안 되는 줄은 건너뛴다
func Scan(r io.Reader, fn func(Event) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if e, ok := Parse(sc.Text()); ok && !fn(e) {
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("사건 읽기: %w", err)
	}
	return nil
}

// ScanFile: path를 Scan. 파일이 없으면 사건 없음(nil)
func ScanFile(path string, fn func(Event) bool) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("사건 파일 열기: %w", err)
	}
	defer f.Close()
	return Scan(f, fn)
}

// Append: path 뒤에 한 줄을 한 번의 쓰기로 더한다(O_APPEND). Check가 실패하면 파일을 건드리지 않고 그 오류
func Append(path string, e Event) error {
	if err := e.Check(); err != nil {
		return err
	}
	return AppendLine(path, e.Line(), 0o666)
}

// AppendLine: path 뒤에 line+줄바꿈을 한 번의 쓰기로(O_APPEND, 없으면 perm으로 만든다) — 운영 기록(internal/oplog)도 쓴다
func AppendLine(path, line string, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return fmt.Errorf("사건 파일 열기: %w", err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("사건 쓰기: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("사건 파일 닫기: %w", err)
	}
	return nil
}
