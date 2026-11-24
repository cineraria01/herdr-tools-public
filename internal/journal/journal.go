// Package journal: 운영 조치 일지(2026-11-24 ops-journal). 사람·에이전트가 한 조치(손잡이·자리·병합기·대기 줄·배포)를
// 한 줄씩 남긴다 — 누가 · 무엇을(명령과 대상) · 왜 · 결과 · 되돌리는 법 · 그때의 상태. 아침에 사람이 읽고 뒤집거나,
// 다른 지휘가 같은 조치를 되풀이·뒤집지 않게 보려는 것이다. 칸 꼴은 docs/ops/journal.md
//
// 자리 ${XDG_STATE_HOME:-~/.local/state}/herdr-tools/journal.jsonl(시험·다른 자리는 HERDR_TOOLS_JOURNAL=<파일>).
// 한 줄 JSON 하나, 쓰기는 한 번의 O_APPEND. 지우지 않는다 — 8MB를 넘으면 쓰기 전에 journal.jsonl.<UTC 시각>으로 넘기고
// 읽을 때는 넘긴 파일까지 함께 읽는다. 일지는 곁에 더할 뿐이다 — 못 써도 조치는 그대로 된다(Record가 한 줄 알림).
package journal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/events"
)

const (
	Env      = "HERDR_TOOLS_JOURNAL" // 일지 파일 경로(시험용 — XDG_STATE_HOME을 바꾸지 않고)
	fieldMax = 400                   // 글 칸 하나의 바이트 상한(줄 하나가 한 번의 쓰기로 들어가게)
)

// maxSize: 넘으면 쓰기 전에 옛 파일로 넘긴다(지우지 않는다). 시험이 줄인다
var maxSize int64 = 8 << 20

// RepeatWindow: 되풀이 알림이 세는 기간(기본). 막지 않고 알리기만 하니 안전 하한만 — 부르는 쪽이 Repeats로 다르게 셀 수 있다
const RepeatWindow = 2 * time.Hour

// 시험 바이너리는 지정이 없으면 /dev/null로(oplog와 같은 까닭 — 시험이 띄운 도구가 실제 일지에 쌓지 않게)
func init() {
	if os.Getenv(Env) == "" && (strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe")) {
		os.Setenv(Env, os.DevNull)
	}
}

// Entry: 일지 한 줄. Undo는 되돌리는 명령 한 줄(없으면 「-」 — 되돌릴 것이 없는 조치)
type Entry struct {
	Time   time.Time `json:"time"`
	By     string    `json:"by"`     // 부른 터미널 주소(HC_TERMINAL_ID) 또는 「사람」
	Tool   string    `json:"tool"`   // 도구(hc-fill·herdr-sync …)
	Action string    `json:"action"` // 하위 명령(knob set·retry·deploy …)
	Target string    `json:"target"` // 대상(손잡이 이름·레인·기계·경로)
	Why    string    `json:"why"`    // --why 글(없으면 빈 값)
	Result string    `json:"result"` // 결과(됨·실패: <까닭>)
	Undo   string    `json:"undo"`   // 되돌리는 명령
	State  string    `json:"state"`  // 그때의 상태 요약 한 줄
}

// Path: 일지 파일 경로
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
	return filepath.Join(base, "herdr-tools", "journal.jsonl"), nil
}

// Who: 부른 쪽 — 에이전트 터미널이면 그 주소, 아니면 「사람」(계정 이름은 적지 않는다)
func Who() string {
	if id := strings.TrimSpace(os.Getenv("HC_TERMINAL_ID")); id != "" {
		return events.Bcut(128, events.Flat(id))
	}
	return "사람"
}

func clean(s string) string { return events.Bcut(fieldMax, events.Flat(strings.TrimSpace(s))) }

// Append: 한 줄을 더한다. Time·By가 비면 지금·Who()로 채운다
func Append(e Entry) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	e.Time = e.Time.UTC().Truncate(time.Second)
	if e.By == "" {
		e.By = Who()
	}
	for _, f := range []*string{&e.By, &e.Tool, &e.Action, &e.Target, &e.Why, &e.Result, &e.Undo, &e.State} {
		*f = clean(*f)
	}
	if e.Undo == "" {
		e.Undo = "-"
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if p == os.DevNull {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	// 같은 초 이름이 이미 있으면(남이 방금 넘김) 넘기지 않고 이어 쓴다 — 덮어 지우지 않는다
	if st, err := os.Stat(p); err == nil && st.Size() > maxSize {
		old := p + "." + time.Now().UTC().Format("20060102T150405Z")
		if _, err := os.Stat(old); errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(p, old); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return events.AppendLine(p, string(b), 0o600)
}

// Record: 조치 명령이 부르는 한 줄 — 일지에 남기고, 같은 대상에 같은 조치가 RepeatWindow 안에 되풀이됐으면 w에 한 줄 알린다.
// 일지를 못 써도 조치는 그대로다(w에 한 줄 알림). 알림 머리는 e.Tool
func Record(w io.Writer, e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if n, err := Repeats(e.Action, e.Target, e.Time.Add(-RepeatWindow)); err == nil && n >= 1 {
		fmt.Fprintf(w, "%s: 최근 %s에 같은 조치(%s %s) %d번째 — 일지는 hc-fill journal\n",
			e.Tool, hours(RepeatWindow), e.Action, e.Target, n+1)
	}
	if err := Append(e); err != nil {
		fmt.Fprintf(w, "%s: 일지를 남기지 못했다(조치는 그대로): %v\n", e.Tool, err)
	}
}

func hours(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%d시간", int(d/time.Hour))
	}
	return d.String()
}

// Filter: Read의 거름. 빈 값은 거르지 않는다
type Filter struct {
	Since  time.Time
	By     string
	Action string
	Target string
}

func (f Filter) ok(e Entry) bool {
	return (f.Since.IsZero() || !e.Time.Before(f.Since)) &&
		(f.By == "" || e.By == f.By) &&
		(f.Action == "" || e.Action == f.Action) &&
		(f.Target == "" || e.Target == f.Target)
}

// files: 읽을 파일 — 넘긴 옛 파일(이름 순 = 시각 순) 뒤에 지금 파일. 없으면 빈 목록
func files() ([]string, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	if p == os.DevNull {
		return nil, nil
	}
	old, err := filepath.Glob(globEscape(p) + ".*")
	if err != nil {
		return nil, err
	}
	sort.Strings(old)
	return append(old, p), nil
}

func globEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`).Replace(s)
}

// Read: 거름에 맞는 줄을 시각 순으로. 읽지 못하는 줄(깨진 JSON)은 건너뛰고 그 수를 돌려준다.
// 파일이 없으면 빈 목록(아직 조치가 없음) — 읽기 실패(권한 등)는 오류로(모름을 「없음」으로 읽지 않는다)
func Read(f Filter) (out []Entry, bad int, err error) {
	fs, err := files()
	if err != nil {
		return nil, 0, err
	}
	for _, p := range fs {
		fh, err := os.Open(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			if len(strings.TrimSpace(sc.Text())) == 0 {
				continue
			}
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				bad++
				continue
			}
			if f.ok(e) {
				out = append(out, e)
			}
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return nil, 0, fmt.Errorf("%s 읽기: %w", p, err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, bad, nil
}

// Repeats: since 뒤로 같은 action·target 줄 수
func Repeats(action, target string, since time.Time) (int, error) {
	es, _, err := Read(Filter{Since: since, Action: action, Target: target})
	return len(es), err
}
