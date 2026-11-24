// Package knob: 실행 중에 바로 먹는 공통 손잡이(2026-11-24 ops-knobs, docs/ops/knobs.md).
//
// 손잡이가 될 수 있는 값은 등록 표 Defs에 있는 것뿐이다 — 착지 판정·보호 규칙·훅·잠금 같은 안전 관문은 올리지 않는다.
// 값은 git 밖 상태 파일(Path)에 「이름·값·누가·왜·언제·언제까지·고정」으로 두고, 우선순위는 손잡이 > 설정 > 기본.
// 기한이 지난 값·범위 밖 값·표에 없는 이름은 읽을 때 버린다(설정 값으로 돌아감). 파일이 깨졌으면 손잡이가 없는 것으로 읽는다.
package knob

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/lockx"
)

// Def: 등록 표 한 줄. Enum이 있으면 그 낱말 가운데 하나, 없으면 Min~Max 정수
type Def struct {
	Name     string   // 손잡이 이름(Tool의 설정 키와 같으면 그 키를 덮는다)
	Tool     string   // 읽는 도구
	Default  string   // 설정에도 없을 때 도구가 쓰는 값(표시용 — 도구 코드의 기본과 같게)
	Min, Max int64    // 정수 범위(닫힌 구간)
	Enum     []string // 낱말 값
	List     bool     // 쉼표로 나눈 이름 목록(글자 a-zA-Z0-9_.-), 「-」 = 비움
	Unit     string   // 단위(분·시간 등, 표시용)
	Remote   bool     // 다른 기계의 도구가 읽음 — 원격 참조 Ref에 싣는다
	Desc     string
}

// Defs: 등록 표. 새 손잡이는 여기에 한 줄 더하고, 도구는 한 함수(hc-fill은 conf, 다른 기계는 ReadRemote → Remote.Get)로 읽는다
var Defs = []Def{
	{Name: "idle", Tool: "hc-fill", Default: "10", Min: 0, Max: 1440, Unit: "분", Desc: "놀고 있음 알림(자리가 남은 채 이만큼 이어지면, 0이면 끔)"},
	{Name: "hold", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "쉰 쓰기 레인의 자리를 안 세기까지(0이면 늘 셈)"},
	{Name: "done_stall", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "완료 뒤 멈춤 알림(0이면 끔)"},
	{Name: "wake_every", Tool: "hc-fill", Default: "10", Min: 1, Max: 240, Unit: "분", Desc: "감사 판정 수거 깨우기 간격"},
	{Name: "ack_max", Tool: "hc-fill", Default: "30", Min: 1, Max: 1440, Unit: "분", Desc: "판정 수거 기다림 상한(넘으면 주인 지휘에 한 번)"},
	{Name: "early_wait", Tool: "hc-fill", Default: "5", Min: 1, Max: 48, Unit: "시간", Desc: "먼저 띄운 레인이 선행을 기다리는 기한"},
	{Name: "auto_submit", Tool: "hc-fill", Default: "-", List: true, Unit: "프로젝트", Desc: "감사 통과 뒤 쉬는 레인 대신 대기열이 제출할 프로젝트(쉼표, - = 끔)"},
	{Name: "auto_submit_max", Tool: "hc-fill", Default: "120", Min: 10, Max: 600, Unit: "초", Desc: "대기열 제출 한 번의 시간 상한(넘으면 에이전트에 넘김)"},
}

// Find: 표에서 이름으로
func Find(name string) (Def, bool) {
	for _, d := range Defs {
		if d.Name == name {
			return d, true
		}
	}
	return Def{}, false
}

// Range: 허용 범위 글
func (d Def) Range() string {
	if len(d.Enum) > 0 {
		return strings.Join(d.Enum, "|")
	}
	if d.List {
		return "-|이름,이름…"
	}
	return fmt.Sprintf("%d~%d", d.Min, d.Max)
}

// Check: 값이 허용 범위인가
func (d Def) Check(v string) error {
	if d.List {
		if v == "-" {
			return nil
		}
		for _, n := range strings.Split(v, ",") {
			if n == "" || n == "-" || strings.Trim(n, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
				return fmt.Errorf("%s 값은 「-」 또는 쉼표로 나눈 이름(a-zA-Z0-9_.-)이다: %q", d.Name, v)
			}
		}
		return nil
	}
	if len(d.Enum) > 0 {
		for _, e := range d.Enum {
			if v == e {
				return nil
			}
		}
		return fmt.Errorf("%s 값은 %s 가운데 하나다: %q", d.Name, d.Range(), v)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < d.Min || n > d.Max || strconv.FormatInt(n, 10) != v {
		return fmt.Errorf("%s 값은 %s 정수다(%s): %q", d.Name, d.Range(), d.Unit, v)
	}
	return nil
}

// Suggest: 모르는 이름에 가까운 등록 이름(편집 거리 3 이하 또는 서로 포함), 가까운 차례로 셋까지
func Suggest(name string) []string {
	type c struct {
		n string
		d int
	}
	var cs []c
	for _, d := range Defs {
		e := dist(name, d.Name)
		if name != "" && (strings.Contains(d.Name, name) || strings.Contains(name, d.Name)) {
			e = -1 // 서로 포함이 먼저
		}
		if e <= 3 {
			cs = append(cs, c{d.Name, e})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	var o []string
	for i := 0; i < len(cs) && i < 3; i++ {
		o = append(o, cs[i].n)
	}
	return o
}

func dist(a, b string) int {
	x, y := []rune(a), []rune(b)
	prev := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		cur := make([]int, len(y)+1)
		cur[0] = i
		for j := 1; j <= len(y); j++ {
			k := 1
			if x[i-1] == y[j-1] {
				k = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+k)
		}
		prev = cur
	}
	return prev[len(y)]
}

// Entry: 저장된 손잡이 하나. Until 0 = 계속
type Entry struct {
	Name, Value, By, Why string
	At, Until            int64
	Pin                  bool
}

// Live: 기한 안이고 표에 있고 값이 범위 안인가
func (e Entry) Live(now time.Time) bool {
	d, ok := Find(e.Name)
	return ok && (e.Until == 0 || now.Unix() < e.Until) && d.Check(e.Value) == nil
}

// Path: 지휘 기계의 저장 파일(git 밖)
func Path(home string) string { return home + "/.local/state/herdr-ops/knobs.tsv" }

const head = "# herdr-tools 손잡이(hc-fill knob) — 이름\t값\t누가\t왜\t언제(초)\t언제까지(초|-)\t고정(1|0)\n"

// Load: 저장 파일을 읽는다. 없으면 빈 것. 꼴이 틀린 줄이 하나라도 있으면 오류(부르는 쪽은 손잡이 없이 돌고 한 줄 알린다)
func Load(path string) (map[string]Entry, error) {
	b, _, err := atomic.Read(path)
	if err != nil {
		return nil, err
	}
	return parse(string(b), path)
}

func parse(s, path string) (map[string]Entry, error) {
	es := map[string]Entry{}
	for i, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		bad := len(f) != 7 || f[0] == "" || (f[6] != "0" && f[6] != "1")
		var at, until int64
		if !bad {
			var e1, e2 error
			at, e1 = strconv.ParseInt(f[4], 10, 64)
			if f[5] != "-" {
				until, e2 = strconv.ParseInt(f[5], 10, 64)
			}
			bad = e1 != nil || e2 != nil
		}
		if bad {
			return nil, fmt.Errorf("손잡이 파일 %s %d줄 꼴이 틀렸다", path, i+1)
		}
		es[f[0]] = Entry{f[0], f[1], f[2], f[3], at, until, f[6] == "1"}
	}
	return es, nil
}

func format(es map[string]Entry, now time.Time) []byte {
	var ns []string
	for n, e := range es {
		if e.Live(now) {
			ns = append(ns, n)
		}
	}
	sort.Strings(ns)
	var b strings.Builder
	b.WriteString(head)
	for _, n := range ns {
		e := es[n]
		u, p := "-", "0"
		if e.Until != 0 {
			u = strconv.FormatInt(e.Until, 10)
		}
		if e.Pin {
			p = "1"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", e.Name, e.Value, clean(e.By), clean(e.Why), e.At, u, p)
	}
	return []byte(b.String())
}

func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "-"
	}
	return s
}

// ErrPinned: 고정된 값을 --force 없이 바꾸려 함
var ErrPinned = errors.New("고정된 값")

// update: 잠금 안에서 읽고 고쳐 원자 쓰기. 깨진 파일은 고치지 않는다(사람이 보고 지우게)
func update(path string, now time.Time, f func(es map[string]Entry) error) error {
	dir := path[:strings.LastIndex(path, "/")]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lk, err := lockx.Lock(path + ".lock")
	if err != nil {
		return err
	}
	defer lk.Close()
	return atomic.Update(path, 0o644, func(old []byte, _ bool) ([]byte, error) {
		es, err := parse(string(old), path)
		if err != nil {
			return nil, err
		}
		if err := f(es); err != nil {
			return nil, err
		}
		return format(es, now), nil
	})
}

// Set: 값을 둔다(이름·값은 부르는 쪽이 Find·Check로 먼저 본다). 살아 있는 고정 값은 force 없이 ErrPinned
func Set(path string, e Entry, force bool, now time.Time) error {
	return update(path, now, func(es map[string]Entry) error {
		if o, ok := es[e.Name]; ok && o.Pin && o.Live(now) && !force {
			return fmt.Errorf("%w: %s(누가 %s, 왜 %s) — 바꾸려면 --force", ErrPinned, o.Name, o.By, o.Why)
		}
		es[e.Name] = e
		return nil
	})
}

// Unset: 값을 지운다(설정 값으로 돌아감). 없었으면 had=false
func Unset(path, name string, force bool, now time.Time) (had bool, err error) {
	err = update(path, now, func(es map[string]Entry) error {
		o, ok := es[name]
		if ok && o.Pin && o.Live(now) && !force {
			return fmt.Errorf("%w: %s(누가 %s, 왜 %s) — 지우려면 --force", ErrPinned, o.Name, o.By, o.Why)
		}
		had = ok && o.Live(now)
		delete(es, name)
		return nil
	})
	return had, err
}

// Source: 지금 먹는 값의 출처
type Source string

const (
	FromDefault Source = "기본"
	FromConf    Source = "설정"
	FromKnob    Source = "손잡이"
)

// Effective: 손잡이 > 설정(빈 값이 아니면) > 기본. 손잡이면 그 줄도
func Effective(d Def, es map[string]Entry, confVal string, now time.Time) (string, Source, *Entry) {
	if e, ok := es[d.Name]; ok && e.Live(now) {
		return e.Value, FromKnob, &e
	}
	if confVal != "" {
		return confVal, FromConf, nil
	}
	return d.Default, FromDefault, nil
}
