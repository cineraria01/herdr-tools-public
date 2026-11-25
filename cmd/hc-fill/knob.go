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
	knobCF   knob.Conf
)

// knobEntries: 저장 파일을 한 번 읽는다. 깨졌거나 못 읽으면 손잡이 없이(설정 값으로) 돌고 한 줄 알린다
func knobEntries() map[string]knob.Entry {
	knobLoad()
	return knobES
}

// knobFile: 손잡이 설정 파일(개인 설정 폴더 knobs.tsv)을 한 번 읽는다. 거절한 줄은 한 줄씩 알리고 아래 층으로
func knobFile() knob.Conf {
	knobLoad()
	return knobCF
}

func knobLoad() {
	knobOnce.Do(func() {
		es, err := knob.Load(knob.Path(HOME))
		if err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: 알림 — 손잡이를 못 읽어 설정 값으로 돈다: %v\n", err)
			es = map[string]knob.Entry{}
		}
		knobES = es
		knobCF = knob.LoadConf(knob.ConfPath(), os.Stderr)
	})
}

// knobConf: hc-fill 손잡이가 살아 있으면 그 값, 아니면 설정 파일 값(둘 다 hc-fill.tsv 줄보다 앞선다). 표에 없는 키는 보지도 않는다
func knobConf(k string) (string, bool) {
	d, ok := knob.Find(k)
	if !ok || d.Tool != "hc-fill" {
		return "", false
	}
	if e, ok := knobEntries()[k]; ok && e.Live(time.Now()) {
		return e.Value, true
	}
	if c, ok := knobFile()[k]; ok {
		return c.Value, true
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
	case "save":
		err = knobSave(args[1:], time.Now())
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
	Where  string `json:"where,omitempty"` // 설정 파일이면 「경로:줄」
	Kind   string `json:"kind,omitempty"`  // 잠깐 바꾼 것(손잡이) · 정해 둔 것(설정 파일)
}

const (
	kindTemp  = "잠깐 바꾼 것"
	kindFixed = "정해 둔 것"
)

func knobRows(now time.Time) []knobRow {
	es, cf := knobEntries(), knobFile()
	var rs []knobRow
	for _, d := range knob.Defs {
		if d.Per == "" {
			cv := ""
			if d.Tool == "hc-fill" {
				if cv = confRaw(d.Name); cv == "" {
					cv = maLegacy(d.Name) // 옛 표식 파일(병합기 살아 있음)도 「설정」으로 보인다
				}
			}
			rs = append(rs, knobRowOf(d, d.Desc, es, cf, cv, now))
		}
		// 「이름@대상」(묶음 손잡이 seats@<기계> 등 · 병합기 프로젝트별 merger-knob-per-project)은 손잡이나 설정 파일에 둔 것만 한 줄씩
		var ns []string
		for n := range knobAtNames(es, cf, now) {
			if fd, _ := knob.Find(n); d.Per != "" && strings.HasPrefix(n, d.Name) || fd.Per == knob.PerProject && fd.Base() == d.Name {
				ns = append(ns, n)
			}
		}
		sort.Strings(ns)
		for _, n := range ns {
			fd, _ := knob.Find(n)
			desc := d.Desc
			if fd.Per == knob.PerProject {
				desc += " — 프로젝트 " + fd.Of() + " 몫"
			}
			rs = append(rs, knobRowOf(fd, desc, es, cf, "", now))
		}
	}
	return rs
}

// knobAtNames: 살아 있는 손잡이와 설정 파일 줄 가운데 「이름@대상」 이름
func knobAtNames(es map[string]knob.Entry, cf knob.Conf, now time.Time) map[string]bool {
	o := map[string]bool{}
	for n, e := range es {
		if strings.Contains(n, "@") && e.Live(now) {
			o[n] = true
		}
	}
	for n := range cf {
		if strings.Contains(n, "@") {
			o[n] = true
		}
	}
	return o
}

// knobRowOf: 손잡이 한 줄 — 지금 값과 그 출처(손잡이면 누가·왜·언제까지, 설정 파일이면 경로:줄·까닭)
func knobRowOf(d knob.Def, desc string, es map[string]knob.Entry, cf knob.Conf, legacy string, now time.Time) knobRow {
	v, src, e := knob.Resolve(d, es, cf, legacy, now)
	r := knobRow{Name: d.Name, Value: v, Source: string(src), Tool: d.Tool, Unit: d.Unit, Range: d.Range(), Remote: d.Remote, Desc: desc}
	switch {
	case e != nil:
		r.By, r.Why, r.Pin, r.At, r.Kind = e.By, e.Why, e.Pin, stamp(e.At), kindTemp
		if e.Until != 0 {
			r.Until = stamp(e.Until)
		}
	case src == knob.FromFile:
		c, _ := cf.Lookup(d)
		r.Why, r.Where, r.Kind = c.Why, c.Where(knob.ConfPath()), kindFixed
	}
	return r
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
		src := r.Source
		if r.Where != "" {
			src += " " + r.Where
		}
		line := fmt.Sprintf("%-12s %6s %s  %-6s 범위 %s  %s(%s)", r.Name, r.Value, r.Unit, src, r.Range, r.Desc, r.Tool)
		switch r.Source {
		case string(knob.FromKnob):
			u := "계속 — 정해 둘 값이면 hc-fill knob save " + r.Name
			if r.Until != "" {
				u = r.Until + "까지"
			}
			pin := ""
			if r.Pin {
				pin = " · 고정"
			}
			line += fmt.Sprintf("\n             %s · 누가 %s · 왜 %s · %s부터 %s%s", r.Kind, r.By, r.Why, r.At, u, pin)
		case string(knob.FromFile):
			why := r.Why
			if why == "" {
				why = "-"
			}
			line += fmt.Sprintf("\n             %s · 왜 %s", r.Kind, why)
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
	old, src, oe := knob.Resolve(d, knobEntries(), knobFile(), confRaw(name), now)
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
	ov, osrc, oe := knob.Resolve(d, knobEntries(), knobFile(), confRaw(d.Name), now)
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
	v, src, _ := knob.Resolve(d, rest, knobFile(), confRaw(d.Name), now)
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

// knobSave: save <이름> [--why <까닭>] — 실행 중 손잡이 값을 설정 파일(개인 설정 폴더 knobs.tsv)에 정해 두고 손잡이는 지운다.
// 먹는 값은 그대로다(고정 손잡이도 옮긴다). 까닭을 안 주면 손잡이의 까닭. 기한은 옮기지 않는다(이제 계속). git 커밋은 하지 않는다
func knobSave(args []string, now time.Time) error {
	args, dryK := takeDry(args)
	args, why := takeWhy(args)
	if len(args) != 1 || strings.HasPrefix(args[0], "--") {
		return errKnobUsage
	}
	name := args[0]
	if _, ok := knob.Find(name); !ok {
		return unknownKnob(name)
	}
	e, ok := knobEntries()[name]
	if !ok || !e.Live(now) {
		return fmt.Errorf("실행 중 손잡이가 없다: %s — 옮길 값이 없다(설정 파일 %s를 직접 고치거나 먼저 knob set)", name, knob.ConfPath())
	}
	if strings.TrimSpace(why) == "" {
		why = e.Why
	}
	cp := knob.ConfPath()
	if dryK {
		preview("손잡이 %s = %s 를 설정 파일 %s 에 정해 두고 손잡이는 지움", name, e.Value, cp)
		return nil
	}
	if err := knob.SaveConf(cp, name, e.Value, why); err != nil {
		return fmt.Errorf("설정 파일 %s에 못 썼다(손잡이는 그대로): %w", cp, err)
	}
	if _, err := knob.Unset(knob.Path(HOME), name, true, now); err != nil {
		return fmt.Errorf("설정 파일에는 썼는데 손잡이를 못 지웠다(값은 같다 — hc-fill knob unset %s --force): %w", name, err)
	}
	note := ""
	if e.Until != 0 {
		note = " · 기한(" + stamp(e.Until) + "까지)은 옮기지 않았다 — 이제 계속"
	}
	say("손잡이 %s = %s 를 설정 파일 %s 에 정해 뒀다(실행 중 손잡이는 지움 — 먹는 값은 그대로)%s", name, e.Value, cp, note)
	next("개인 설정 저장소에서 커밋한다: git -C %s add %s && git -C %s commit -m \"손잡이 %s 정해 둠\"", toolDir(cp), knob.ConfName, toolDir(cp), name)
	jot("knob save", name, why, "됨", knobSetLine(e)+" (설정 파일 줄은 직접 지움)", fmt.Sprintf("손잡이 %s → 설정 파일 %s", e.Value, cp))
	return nil
}

func toolDir(p string) string { return p[:strings.LastIndex(p, "/")] }

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
		msg := knob.FormatRemote(knobRemoteOf(es, p, now), now)
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

// knobRemoteOf: 프로젝트 p의 원격 몫 — 설정 파일 값 위에 살아 있는 손잡이(둘 다 그 프로젝트 몫으로 푼 것). 다른 기계는 이 참조만
// 읽어도 지휘 기계와 같은 값(손잡이 > 설정 파일)을 쓴다. 설정 파일이 없으면 손잡이만(전과 바이트까지 같다)
func knobRemoteOf(es map[string]knob.Entry, p string, now time.Time) map[string]knob.Entry {
	o := knob.ForProject(knobFile().Entries(), p, now)
	for n, e := range knob.ForProject(es, p, now) {
		if e.Live(now) {
			o[n] = e
		}
	}
	return o
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
