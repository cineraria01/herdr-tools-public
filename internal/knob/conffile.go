package knob

// 손잡이 설정 파일(2026-11-26 ht-config-knobs, docs/ops/knobs.md 「설정 파일」): 개인 설정 폴더($HERDR_TOOLS_CONFIG)의 ConfName 한 장이
// 「정해 둔 값」 층이다 — 등록 표의 모든 손잡이를 여기서 정하고, herdr-sync로 모든 기계에 같은 파일이 간다. 실행 중 손잡이(Path)는
// 「잠깐 바꾼 값」으로 이 파일을 덮는다. 우선순위: 손잡이 > 설정 파일 > 도구별 옛 설정(hc-fill.tsv 키·환경 변수) > 기본.
// 줄 꼴: 「이름[@대상]  값  # 까닭」(공백·탭). 표에 없는 이름·범위 밖 값·값 없음·같은 이름 두 번(뒤 줄)은 줄마다 거절하고 먹지 않는다.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/toolcfg"
)

// ConfName: 개인 설정 폴더의 손잡이 설정 파일 이름(범용 틀은 config.example/ 같은 이름)
const ConfName = "knobs.tsv"

// ConfPath: 이 기계의 손잡이 설정 파일
func ConfPath() string { return toolcfg.Dir() + "/" + ConfName }

// ConfEntry: 설정 파일 한 줄
type ConfEntry struct {
	Name, Value, Why string
	Line             int
}

// Where: 「경로:줄」 — 출처 칸
func (c ConfEntry) Where(path string) string { return fmt.Sprintf("%s:%d", path, c.Line) }

// Conf: 설정 파일에서 먹는 줄(이름 → 줄)
type Conf map[string]ConfEntry

// ParseConf: 설정 파일 글을 읽는다. 거절한 줄은 「경로:줄: 까닭」 오류로 하나씩(먹지 않는다)
func ParseConf(s, path string) (Conf, []error) {
	c := Conf{}
	var errs []error
	for i, l := range strings.Split(s, "\n") {
		body, why, _ := strings.Cut(strings.TrimSuffix(l, "\r"), "#")
		f := strings.Fields(body)
		if len(f) == 0 {
			continue
		}
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("%s:%d: %s", path, i+1, fmt.Sprintf(format, a...)))
		}
		d, ok := Find(f[0])
		switch {
		case !ok:
			msg := "모르는 손잡이 " + f[0]
			if sg := Suggest(f[0]); len(sg) > 0 {
				msg += " · 이것인가: " + strings.Join(sg, ", ")
			}
			bad("%s", msg)
		case len(f) != 2:
			bad("「이름 값 # 까닭」 꼴이 아니다(%s)", f[0])
		case c[f[0]].Line != 0:
			bad("같은 이름 두 번 %s(앞 %d줄을 쓴다)", f[0], c[f[0]].Line)
		default:
			if err := d.Check(f[1]); err != nil {
				bad("%v", err)
				continue
			}
			c[f[0]] = ConfEntry{f[0], f[1], strings.TrimSpace(why), i + 1}
		}
	}
	return c, errs
}

// LoadConf: 설정 파일을 읽는다. 없으면 빈 것. 거절한 줄은 w에 한 줄씩 알린다(w nil이면 말 없이) — 그 손잡이는 아래 층으로
func LoadConf(path string, w io.Writer) Conf {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Conf{}
	}
	if err != nil {
		if w != nil {
			fmt.Fprintf(w, "알림: 손잡이 설정 파일을 못 읽어 없는 것으로: %v\n", err)
		}
		return Conf{}
	}
	c, errs := ParseConf(string(b), path)
	for _, e := range errs {
		if w != nil {
			fmt.Fprintf(w, "알림: 손잡이 설정 파일 줄을 버림(기본으로) — %v\n", e)
		}
	}
	return c
}

// Lookup: 그 이름의 줄, 프로젝트별이면 공통 이름 줄 차례
func (c Conf) Lookup(d Def) (ConfEntry, bool) {
	if e, ok := c[d.Name]; ok {
		return e, true
	}
	if d.Per == PerProject {
		e, ok := c[d.Base()]
		return e, ok
	}
	return ConfEntry{}, false
}

// FromFile: 설정 파일에서 온 값(「설정」은 도구별 옛 설정 — hc-fill.tsv 키·환경 변수)
const FromFile Source = "설정 파일"

// Resolve: 손잡이(프로젝트별이면 「이름@프로젝트」 → 공통) > 설정 파일(같은 차례) > 옛 설정(빈 값이 아니면) > 기본. 손잡이면 그 줄도
func Resolve(d Def, es map[string]Entry, cf Conf, legacy string, now time.Time) (string, Source, *Entry) {
	for _, n := range []string{d.Name, d.Base()} {
		if e, ok := es[n]; ok && e.Live(now) && (n == d.Name || d.Per == PerProject) {
			return e.Value, FromKnob, &e
		}
	}
	if c, ok := cf.Lookup(d); ok {
		return c.Value, FromFile, nil
	}
	if legacy != "" {
		return legacy, FromConf, nil
	}
	return d.Default, FromDefault, nil
}

// Local: 이 기계의 값 — 저장 파일(home) 손잡이 > 설정 파일 > 기본. 저장 파일이 깨졌으면 err와 함께 설정 파일·기본 값
// (그때 어떻게 돌지는 부르는 쪽이 정한다). 설정 파일의 거절한 줄은 w에 한 줄씩(nil이면 말 없이)
func Local(home, name string, w io.Writer, now time.Time) (string, error) {
	d, ok := Find(name)
	if !ok {
		return "", fmt.Errorf("모르는 손잡이 %s", name)
	}
	es, err := Load(Path(home))
	v, _, _ := Resolve(d, es, LoadConf(ConfPath(), w), "", now)
	return v, err
}

// Entries: 설정 파일 줄을 손잡이 줄 꼴로(기한 없음) — 원격 참조에 실을 때 손잡이와 같은 길(ForProject·FormatRemote)로 푼다
func (c Conf) Entries() map[string]Entry {
	o := make(map[string]Entry, len(c))
	for n, e := range c {
		o[n] = Entry{Name: n, Value: e.Value, By: "설정 파일", Why: e.Why}
	}
	return o
}

// With: 원격 값에 없는 손잡이를 설정 파일 값으로 채운 사본(프로젝트별 줄은 빼고 — 그 몫은 지휘 기계가 참조에 풀어 싣는다)
func (r Remote) With(c Conf) Remote {
	o := Remote{}
	for n, e := range c {
		if d, ok := Find(n); ok && d.Per != PerProject {
			o[n] = e.Value
		}
	}
	for n, v := range r {
		o[n] = v
	}
	return o
}

// SaveConf: 설정 파일에 「이름 값 # 까닭」을 둔다 — 같은 이름 줄이 있으면 그 자리를 바꾸고, 없으면 끝에 더한다(주석·다른 줄은 그대로)
func SaveConf(path, name, value, why string) error {
	if err := os.MkdirAll(path[:strings.LastIndex(path, "/")], 0o755); err != nil {
		return err
	}
	lk, err := lockx.Lock(path + ".lock")
	if err != nil {
		return err
	}
	defer lk.Close()
	line := name + "\t" + value
	if why = strings.Join(strings.Fields(strings.ReplaceAll(why, "#", "＃")), " "); why != "" {
		line += "\t# " + why
	}
	return atomic.Update(path, 0o644, func(old []byte, _ bool) ([]byte, error) {
		ls := strings.Split(strings.TrimSuffix(string(old), "\n"), "\n")
		if len(old) == 0 {
			ls = nil
		}
		done := false
		for i, l := range ls {
			body, _, _ := strings.Cut(l, "#")
			if f := strings.Fields(body); len(f) > 0 && f[0] == name && !done {
				ls[i], done = line, true
			}
		}
		if !done {
			ls = append(ls, line)
		}
		return []byte(strings.Join(ls, "\n") + "\n"), nil
	})
}

// Template: 범용 틀 config.example/knobs.tsv — 등록 표에서 만든다(모두 주석, 기본값·범위·설명). TestConfTemplate가 틀과 바이트까지 맞춘다
func Template() string {
	var b strings.Builder
	b.WriteString(`# herdr-tools 손잡이 설정 파일(` + ConfName + `) — 「정해 둔 값」. 개인 설정 폴더($HERDR_TOOLS_CONFIG)에 두고 herdr-sync로 모든 기계에 맞춘다.
# 한 줄에 하나: 이름[@대상]  값  # 까닭   (공백·탭으로 나눔, # 뒤는 까닭)
# 우선순위: 실행 중 손잡이(hc-fill knob set — 잠깐 바꾼 값) > 이 파일 > 도구별 옛 설정(hc-fill.tsv 키·환경 변수) > 기본.
# 표에 없는 이름·범위 밖 값·같은 이름 두 번은 읽을 때 한 줄 알리고 버린다(아래 층으로). 검사: herdr-tools-config knobs-lint
# 실행 중 손잡이를 이 파일로 옮기기: hc-fill knob save <이름> --why <까닭>
# 이 틀은 등록 표(internal/knob Defs)에서 만든다 — 손으로 고치지 않는다: go run ./cmd/herdr-tools-config knobs-template > config.example/` + ConfName + `
`)
	ds := append([]Def(nil), Defs...)
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].Tool < ds[j].Tool })
	tool := ""
	for _, d := range ds {
		if d.Tool != tool {
			tool = d.Tool
			fmt.Fprintf(&b, "\n## %s\n", tool)
		}
		unit := ""
		if d.Unit != "" {
			unit = " " + d.Unit
		}
		fmt.Fprintf(&b, "# — %s (범위 %s%s)\n", d.Desc, d.Range(), unit)
		switch {
		case d.Per != "":
			fmt.Fprintf(&b, "# %s<%s>\t<값>\n", d.Name, d.Per)
		default:
			fmt.Fprintf(&b, "# %s\t%s\n", d.Name, d.Default)
			if d.Remote && d.Tool == "hc-merger" {
				fmt.Fprintf(&b, "# %s@<%s>\t%s\n", d.Name, PerProject, d.Default)
			}
		}
	}
	return b.String()
}
