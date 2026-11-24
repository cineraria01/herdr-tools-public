package main

// knob 명령(2026-11-24 ops-knobs, docs/ops/knobs.md): 실행 중에 바로 먹는 손잡이 — ls·set·unset. 저장은 internal/knob(git 밖 상태 파일).
// hc-fill 손잡이는 conf()가 설정 줄보다 먼저 읽고(바퀴 = 새 프로세스라 다음 바퀴에 먹는다), 표에서 Remote인 손잡이는 바퀴마다
// 원격 참조 refs/herdr/knobs로 올린다(knobsRemoteTick — 바뀐 때만, 다른 기계의 도구가 knob.ReadRemote로 읽는다)

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/knob"
)

func init() { register("knob", knobCmd) }

var (
	knobOnce sync.Once
	knobES   map[string]knob.Entry
)

// knobEntries: 저장 파일을 한 번 읽는다. 깨졌거나 못 읽으면 손잡이 없이(설정 값으로) 돌고 한 줄 알린다
func knobEntries() map[string]knob.Entry {
	knobOnce.Do(func() {
		es, err := knob.Load(knob.Path(HOME))
		if err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: 알림 — 손잡이를 못 읽어 설정 값으로 돈다: %v\n", err)
			es = map[string]knob.Entry{}
		}
		knobES = es
	})
	return knobES
}

// knobConf: hc-fill 손잡이가 살아 있으면 그 값(설정 줄보다 앞선다). 표에 없는 키는 손잡이를 보지도 않는다
func knobConf(k string) (string, bool) {
	d, ok := knob.Find(k)
	if !ok || d.Tool != "hc-fill" {
		return "", false
	}
	if e, ok := knobEntries()[k]; ok && e.Live(time.Now()) {
		return e.Value, true
	}
	return "", false
}

func knobCmd(args []string) {
	if len(args) == 0 {
		usage()
	}
	var err error
	switch args[0] {
	case "ls":
		err = knobLs(args[1:])
	case "set":
		err = knobSet(args[1:], time.Now())
	case "unset":
		err = knobUnset(args[1:], time.Now())
	default:
		usage()
	}
	if errors.Is(err, errKnobUsage) {
		usage()
	}
	if err != nil {
		die("%v", err)
	}
}

var errKnobUsage = errors.New("잘못 부름")

type knobRow struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
	By     string `json:"by,omitempty"`
	Why    string `json:"why,omitempty"`
	At     string `json:"at,omitempty"`
	Until  string `json:"until,omitempty"`
	Pin    bool   `json:"pin"`
	Tool   string `json:"tool"`
	Unit   string `json:"unit,omitempty"`
	Range  string `json:"range"`
	Remote bool   `json:"remote"`
	Desc   string `json:"desc"`
}

func knobRows(now time.Time) []knobRow {
	es := knobEntries()
	var rs []knobRow
	for _, d := range knob.Defs {
		if d.Per != "" { // 묶음 손잡이(seats@<기계> 등)는 둔 값만 한 줄씩
			var ns []string
			for n, e := range es {
				if strings.HasPrefix(n, d.Name) && e.Live(now) {
					ns = append(ns, n)
				}
			}
			sort.Strings(ns)
			for _, n := range ns {
				e := es[n]
				fd, _ := knob.Find(n)
				r := knobRow{Name: n, Value: e.Value, Source: string(knob.FromKnob), Tool: d.Tool, Unit: d.Unit, Range: fd.Range(), Remote: d.Remote, Desc: d.Desc,
					By: e.By, Why: e.Why, Pin: e.Pin, At: stamp(e.At)}
				if e.Until != 0 {
					r.Until = stamp(e.Until)
				}
				rs = append(rs, r)
			}
			continue
		}
		cv := ""
		if d.Tool == "hc-fill" {
			if cv = confRaw(d.Name); cv == "" {
				cv = maLegacy(d.Name) // 옛 표식 파일(병합기 살아 있음)도 「설정」으로 보인다
			}
		}
		v, src, e := knob.Effective(d, es, cv, now)
		r := knobRow{Name: d.Name, Value: v, Source: string(src), Tool: d.Tool, Unit: d.Unit, Range: d.Range(), Remote: d.Remote, Desc: d.Desc}
		if e != nil {
			r.By, r.Why, r.Pin, r.At = e.By, e.Why, e.Pin, stamp(e.At)
			if e.Until != 0 {
				r.Until = stamp(e.Until)
			}
		}
		rs = append(rs, r)
		rs = append(rs, knobProjectRows(d, es, now)...)
	}
	return rs
}

// knobProjectRows: 병합기 손잡이의 프로젝트별 값(이름@프로젝트 — merger-knob-per-project)을 공통 줄 바로 밑에 둔 것만 한 줄씩
func knobProjectRows(d knob.Def, es map[string]knob.Entry, now time.Time) []knobRow {
	var ns []string
	for n, e := range es {
		if fd, ok := knob.Find(n); ok && fd.Per == knob.PerProject && fd.Base() == d.Name && e.Live(now) {
			ns = append(ns, n)
		}
	}
	sort.Strings(ns)
	var rs []knobRow
	for _, n := range ns {
		e := es[n]
		r := knobRow{Name: n, Value: e.Value, Source: string(knob.FromKnob), Tool: d.Tool, Unit: d.Unit, Range: d.Range(), Remote: d.Remote,
			Desc: d.Desc + " — 프로젝트 " + n[len(d.Name)+1:] + " 몫", By: e.By, Why: e.Why, Pin: e.Pin, At: stamp(e.At)}
		if e.Until != 0 {
			r.Until = stamp(e.Until)
		}
		rs = append(rs, r)
	}
	return rs
}

func stamp(s int64) string { return time.Unix(s, 0).Format("2006-01-02 15:04") }

func knobLs(args []string) error {
	js := false
	for _, a := range args {
		if a != "--json" {
			return errKnobUsage
		}
		js = true
	}
	rs := knobRows(time.Now())
	if js {
		b, err := json.MarshalIndent(rs, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	for _, r := range rs {
		line := fmt.Sprintf("%-12s %6s %s  %-6s 범위 %s  %s(%s)", r.Name, r.Value, r.Unit, r.Source, r.Range, r.Desc, r.Tool)
		if r.Source == string(knob.FromKnob) {
			u := "계속"
			if r.Until != "" {
				u = r.Until + "까지"
			}
			pin := ""
			if r.Pin {
				pin = " · 고정"
			}
			line += fmt.Sprintf("\n             누가 %s · 왜 %s · %s부터 %s%s", r.By, r.Why, r.At, u, pin)
		}
		fmt.Println(line)
	}
	return nil
}

// knobSet: set <이름> <값> --why <글> [--for <분> | --until <시각>] [--pin] [--force]
func knobSet(args []string, now time.Time) error {
	args, dryK := takeDry(args)
	var pos []string
	why, forM, until, pin, force := "", "", "", false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case (a == "--why" || a == "--for" || a == "--until") && i+1 < len(args):
			i++
			switch a {
			case "--why":
				why = args[i]
			case "--for":
				forM = args[i]
			default:
				until = args[i]
			}
		case a == "--pin":
			pin = true
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "--"):
			return errKnobUsage
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 2 || (forM != "" && until != "") {
		return errKnobUsage
	}
	if strings.TrimSpace(why) == "" {
		return errors.New("--why <까닭>이 있어야 한다(누가 왜 바꿨는지 남긴다)")
	}
	name, val := pos[0], pos[1]
	d, ok := knob.Find(name)
	if !ok {
		return unknownKnob(name)
	}
	if err := d.Check(val); err != nil {
		return err
	}
	if d.Per == "기계" && !machineKnown(d.Of()) {
		return fmt.Errorf("설정에 없는 기계: %s — machine 줄의 이름만", d.Of())
	}
	if d.Per == knob.PerProject && !projectKnown(d.Of()) {
		return fmt.Errorf("설정에 없는 프로젝트: %s — repo 줄의 이름만", d.Of())
	}
	if d.Per == knob.PerProject && repoOf(d.Of(), LOCAL) == "" { // 원격에 올리는 knobsRemoteTick은 이 기계 repo 줄만 돈다 — 안 올라갈 값을 받지 않는다
		return fmt.Errorf("이 기계(%s)에 repo 줄이 없는 프로젝트: %s — 원격에 올릴 클론이 없다", LOCAL, d.Of())
	}
	e := knob.Entry{Name: name, Value: val, By: knobBy(), Why: why, At: now.Unix(), Pin: pin}
	switch {
	case forM != "":
		n, err := strconv.Atoi(forM)
		if err != nil || n < 1 {
			return fmt.Errorf("--for는 1 이상 분이다: %q", forM)
		}
		e.Until = now.Add(time.Duration(n) * time.Minute).Unix()
	case until != "":
		t, err := parseUntil(until)
		if err != nil {
			return err
		}
		if !t.After(now) {
			return fmt.Errorf("--until이 지났다: %s", until)
		}
		e.Until = t.Unix()
	}
	old, src, oe := knob.Effective(d, knobEntries(), confRaw(name), now)
	if dryK {
		return knobPreview(name, "= "+val, force, now)
	}
	if err := knob.Set(knob.Path(HOME), e, force, now); err != nil {
		return err
	}
	u := "기한 없음"
	if e.Until != 0 {
		u = stamp(e.Until) + "까지(지나면 설정 값으로)"
	}
	if old == "" { // 묶음 손잡이(seats@<기계> 등)는 표 기본이 없다 — 설정 줄 값
		old, src = "설정 값", knob.FromConf
	}
	say("손잡이 %s = %s (전: %s, %s) — 다음 바퀴부터, %s", name, val, old, src, u)
	undo := "hc-fill knob unset " + name
	if src == knob.FromKnob && oe != nil && oe.Name == name { // 프로젝트별 첫 값의 oe는 공통 줄 — 되돌리기는 unset 이름@프로젝트
		undo = knobSetLine(*oe)
	}
	if pin {
		undo += " --force"
	}
	jot("knob set", name, why, "됨", undo, fmt.Sprintf("전 %s(%s) → %s, %s", old, src, val, u))
	return nil
}

func knobUnset(args []string, now time.Time) error {
	args, dryK := takeDry(args)
	args, why := takeWhy(args)
	var pos []string
	force := false
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "--"):
			return errKnobUsage
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 1 {
		return errKnobUsage
	}
	d, ok := knob.Find(pos[0])
	if !ok {
		return unknownKnob(pos[0])
	}
	ov, osrc, oe := knob.Effective(d, knobEntries(), confRaw(d.Name), now)
	if dryK {
		return knobPreview(pos[0], "지움", force, now)
	}
	had, err := knob.Unset(knob.Path(HOME), d.Name, force, now)
	if err != nil {
		return err
	}
	rest := map[string]knob.Entry{} // 지운 뒤 먹는 값 — 프로젝트별(이름@프로젝트)이면 공통 손잡이까지 본다
	for k, e := range knobEntries() {
		if k != d.Name {
			rest[k] = e
		}
	}
	v, src, _ := knob.Effective(d, rest, confRaw(d.Name), now)
	if v == "" { // 묶음 손잡이(seats@<기계> 등) — 설정 줄 값으로
		v, src = "설정 값", knob.FromConf
	}
	if !had {
		say("손잡이 %s: 둔 값이 없다 — 지금 %s(%s)", d.Name, v, src)
		return nil
	}
	say("손잡이 %s를 지웠다 — 다음 바퀴부터 %s(%s)", d.Name, v, src)
	// 기한이 지나 이미 안 먹던 값이면 되돌릴 것이 없다(프로젝트별 값이 안 먹던 때의 oe는 공통 줄이라 마찬가지)
	undo := "-"
	if oe != nil && oe.Name == d.Name {
		undo = knobSetLine(*oe)
	}
	jot("knob unset", d.Name, why, "됨", undo, fmt.Sprintf("전 %s(%s) → %s(%s)", ov, osrc, v, src))
	return nil
}

// knobSetLine: 그 손잡이 값을 다시 두는 명령 한 줄(되돌리기 — 기한은 옮기지 않는다)
func knobSetLine(e knob.Entry) string {
	s := fmt.Sprintf("hc-fill knob set %s %s --why 되돌림", e.Name, e.Value)
	if e.Pin {
		s += " --pin"
	}
	return s
}

func unknownKnob(name string) error {
	msg := fmt.Sprintf("모르는 손잡이: %s — 손잡이는 hc-fill knob ls의 이름뿐이다(안전 관문은 손잡이가 아니다)", name)
	if s := knob.Suggest(name); len(s) > 0 {
		msg += " · 이것인가: " + strings.Join(s, ", ")
	}
	return errors.New(msg)
}

// knobBy: 부른 터미널 주소, 없으면 사람
func knobBy() string {
	if v := os.Getenv("HC_TERMINAL_ID"); v != "" {
		return v
	}
	return "사람"
}

func parseUntil(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, f := range []string{"2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("--until 꼴은 RFC3339 또는 「2006-01-02 15:04」(이 기계 시간대)다: %q", s)
}

// knobsRemoteTick: Remote 손잡이를 이 기계 repo 줄의 프로젝트마다 원격 참조 knob.Ref로(날짜 0 빈 트리 커밋 — 같은 내용이면 같은 SHA).
// 올린 내용의 지문을 <DATA>/knobs-remote.tsv에 두어 바뀌지 않은 바퀴는 git을 부르지 않는다. 못 올리면 다음 바퀴에 다시.
// 내용은 프로젝트마다 그 몫으로 푼 값(knob.ForProject — 「이름@프로젝트」가 공통을 덮고 다른 프로젝트 값은 안 실림)
func knobsRemoteTick() {
	es, now := knobEntries(), time.Now()
	rf := DATA + "/knobs-remote.tsv"
	done := map[string]string{}
	for _, r := range readRows(rf) {
		done[r.F(kvKey)] = r.F(kvVal)
	}
	var keep []string
	for _, f := range confRows("repo") {
		if len(f) < 4 || f[2] != LOCAL {
			continue
		}
		p := f[1]
		msg := knob.FormatRemote(knob.ForProject(es, p, now), now)
		sum := sha256hex([]byte(msg))
		if done[p] == sum || (msg == "" && done[p] == "") {
			if done[p] != "" {
				keep = append(keep, p+"\t"+sum)
			}
			continue
		}
		if knobsRemoteOne(p, repoOf(p, LOCAL), msg) {
			if msg != "" {
				keep = append(keep, p+"\t"+sum)
			}
		} else if done[p] != "" {
			keep = append(keep, p+"\t"+done[p]) // 다음 바퀴에 다시
		}
	}
	if dry {
		return
	}
	body := strings.Join(keep, "\n")
	if body != "" {
		body += "\n"
	}
	if old, _ := readFile(rf); old != body {
		if err := writeAtomic(rf, body); err != nil {
			say("손잡이 원격: 기록을 쓰지 못했다: %v", err)
		}
	}
}

func knobsRemoteOne(p, r, msg string) bool {
	cur, rc := run("git", "-C", r, "ls-remote", "origin", knob.Ref)
	if rc != 0 {
		say("손잡이 원격 %s: 원격을 못 읽음 — 다음 바퀴에", p)
		return false
	}
	cur = firstField(cur)
	want := ""
	if msg != "" {
		et, rc := run("git", "-C", r, "hash-object", "-t", "tree", "-w", "/dev/null")
		if rc != 0 {
			return false
		}
		env := []string{"GIT_AUTHOR_NAME=hc-fill", "GIT_AUTHOR_EMAIL=hc-fill@localhost", "GIT_AUTHOR_DATE=@0 +0000",
			"GIT_COMMITTER_NAME=hc-fill", "GIT_COMMITTER_EMAIL=hc-fill@localhost", "GIT_COMMITTER_DATE=@0 +0000"}
		o, rc := Cmd{Name: "git", Args: []string{"-C", r, "commit-tree", sub(et), "-m", msg}, Env: env}.Run()
		if rc != 0 {
			return false
		}
		want = sub(o)
	}
	if cur == want {
		return true
	}
	if dry {
		say("%s손잡이 원격 %s: git push origin %s:%s", P, p, want, knob.Ref)
		return true
	}
	if _, rc := run("git", "-C", r, "push", "-q", "--force-with-lease="+knob.Ref+":"+cur, "origin", want+":"+knob.Ref); rc != 0 {
		say("손잡이 원격 %s: push 실패 — 다음 바퀴에", p)
		return false
	}
	say("손잡이 원격 %s: 맞춤", p)
	return true
}
