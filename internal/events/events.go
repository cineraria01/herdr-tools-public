// Package events: hc-lane 사건 줄(…/hc-lane/events.tsv) 읽고 쓰기 — 지금 꼴 그대로(lib/hc-lane/report.sh·common.sh 「report」).
// 한 줄 12칸(탭): 시각(UTC)·기계·터미널·레인·상태·sha·한 줄·turn·--to·발행 실행·수신 지휘·키. 옛 7~11칸 줄도 읽는다.
// 쓰기는 한 번의 O_APPEND, 줄은 1000바이트 안(여러 쓰는 쪽이 함께 더해도 줄이 섞이지 않게).
package events

import (
	"bufio"
	"fmt"
	"io"
	"os"
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

// Bcut: 바이트로 자르고 끝에 남은 반쪽 UTF-8 글자는 버린다
func Bcut(n int, s string) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
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

// Line: report와 같은 꼴의 한 줄(줄바꿈 없음). 앞 칸은 64바이트씩, 남는 자리를 한 줄 칸이 쓴다. 시각이 비면 지금
func (e Event) Line() string {
	t := e.Time
	if t == "" {
		t = UTC(time.Now())
	}
	tail := "\t" + dash(e.Turn) + "\t" + dash(e.To) + "\t" + dash(e.Run) + "\t" + Bcut(64, dash(e.Owner)) + "\t" + dash(e.Key)
	head := t + "\t" + Bcut(64, Flat(dash(e.Machine))) + "\t" + Bcut(64, Flat(dash(e.Term))) + "\t" + Bcut(64, Flat(dash(e.Lane))) +
		"\t" + e.State + "\t" + Flat(dash(e.SHA)) + "\t"
	return Bcut(MaxLine-len(tail), head+Flat(dash(e.Msg))) + tail
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

// Append: path 뒤에 한 줄을 한 번의 쓰기로 더한다(O_APPEND)
func Append(path string, e Event) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return fmt.Errorf("사건 파일 열기: %w", err)
	}
	if _, err := f.WriteString(e.Line() + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("사건 쓰기: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("사건 파일 닫기: %w", err)
	}
	return nil
}
