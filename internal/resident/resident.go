// Package resident: 상주 도구(오래 도는 고리)가 지금 깔린 실행 파일과 같은 판으로 도는지 잰다(2026-11-24 stale-resident-check —
// 지휘의 사건 감시 하나가 스스로 넘어가기가 없는 옛 판으로 하루 가까이 돌아 그 사이 아홉 번 넘는 배포가 닿지 않았다. 실행 파일은
// rename으로 갈려 옛 프로세스는 지워진 옛 파일을 쥔 채 멀쩡히 돌고, hc ls의 이름·상태로는 구분이 안 된다).
//
//   - 표시(Mark): selfexec.New가 뜰 때(스스로 넘어간 새 판도 다시 New) 「도구·pid·경로·판(커밋·크기·수정 시각)·hc 터미널·launchd 라벨·
//     인자」를 resident/pids/<pid>.json 한 장에 남긴다. 바깥에서 실행 파일을 들여다보지 않는다(열린 파일 조회는 느린 기계에서 몇 분).
//   - 판정(Scan): 상주 표(Kinds)에 맞는 프로세스마다 — 표시가 있고(pid 같고 프로세스가 뜬 뒤 쓰임) 깔린 판과 같으면 지나감, 다르면 old,
//     표시가 없으면 unknown(같음으로 읽지 않는다). 프로세스가 뜬 지·깔린 판이 놓인 지 둘 다 유예를 넘긴 것만(다음 바퀴에 스스로 넘어갈 수 있다).
//   - 결과(Report): 점검하는 쪽(hc-sweep 건강 점검)이 바퀴마다 쓰는 한 장 resident/report.json — hc-fill state·herdr-sync --status가 읽는다.
//
// 읽기만 한다 — 프로세스를 끄거나 다시 띄우지 않는다(다시 띄우는 명령은 글로 줄 뿐, 지휘가 한다).
package resident

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/version"
)

// Mark: 상주 프로세스가 스스로 남기는 판 표시
type Mark struct {
	Who    string    `json:"who"`
	Pid    int       `json:"pid"`
	Path   string    `json:"path"`
	Commit string    `json:"commit"`
	Size   int64     `json:"size"`
	Mtime  time.Time `json:"mtime"`
	At     time.Time `json:"at"`
	Term   string    `json:"term"`
	Label  string    `json:"label"`
	Args   []string  `json:"args"`
}

// Proc: ps로 본 프로세스 하나. Term·Label은 표시가 없는 것만 부르는 쪽이 그 프로세스 환경에서 채운다
type Proc struct {
	Pid   int
	Start time.Time
	Args  []string
	Term  string
	Label string
}

// Bin: 지금 깔린 실행 파일
type Bin struct {
	Size  int64
	Mtime time.Time
}

// Finding: 옛 판(old)이거나 판 모름(unknown)인 상주 하나
type Finding struct {
	Who       string    `json:"who"`
	Pid       int       `json:"pid"`
	Start     time.Time `json:"start"`
	State     string    `json:"state"` // old | unknown
	Have      string    `json:"have"`
	Installed string    `json:"installed"`
	InstAt    time.Time `json:"installed_at"` // 영 = 깔린 판 모름
	Term      string    `json:"term"`
	Restart   string    `json:"restart"`
}

// Report: 마지막 점검 한 장. Unknown = 마지막 바퀴가 재지 못한 까닭(앞 결과는 그대로), Sent = 보낸 사건 키 → 보낸 때
type Report struct {
	Checked  time.Time         `json:"checked"`
	Unknown  string            `json:"unknown"`
	Findings []Finding         `json:"findings"`
	Sent     map[string]string `json:"sent"`
}

// Kind: 상주 표 한 줄 — Who는 selfexec.New에 준 이름(표시의 who), Bin은 실행 파일 이름, Match는 그 바이너리의 인자 가운데 상주인 꼴.
// hc-sweep·hc-core-relaunch는 단발이라 표에 없다(docs/go/selfexec.md)
type Kind struct {
	Who, Bin string
	Match    func(args []string) bool
}

func sub(name string) func([]string) bool {
	return func(a []string) bool { return len(a) > 1 && a[1] == name }
}

// Kinds: 상주 표. selfexec.New를 부르는 곳이 늘면 여기에도 한 줄(시험이 맞춘다)
var Kinds = []Kind{
	{"hc-lane watch", "hc-lane", sub("watch")},
	{"hc-fill watch", "hc-fill", sub("watch")},
	{"herdr-balance", "herdr-balance", func(a []string) bool {
		for _, x := range a[1:] {
			if x == "--watch" {
				return true
			}
		}
		return false
	}},
	{"hc-job-server", "hc-job-server", func([]string) bool { return true }},
	// 병합기 바깥 고리(hc-lane merger → hc-merger --repo …). 바퀴 일꾼(--round)·추측 일꾼·한 번(--once)은 상주가 아니다
	{"hc-lane merger", "hc-merger", func(a []string) bool {
		if len(a) < 2 || a[1] != "--repo" && a[1] != "--tick" && strings.HasPrefix(a[1], "-") {
			return false // --round·--spec-verify·--version … 하위 명령
		}
		return slices.Contains(a, "--repo") && !slices.Contains(a, "--once")
	}},
}

// KindOf: 인자(argv[0]의 이름 + 하위 명령)로 본 상주 종류. 시험 경로(TestArgs)의 프로세스는 상주가 아니다 — 시험 고아는 hc-sweep
// 시험 고아 점검(hc-test-orphan)이 따로 알린다
func KindOf(args []string) (Kind, bool) {
	if len(args) == 0 || TestArgs(args) {
		return Kind{}, false
	}
	b := filepath.Base(args[0])
	for _, k := range Kinds {
		if k.Bin == b && k.Match(args) {
			return k, true
		}
	}
	return Kind{}, false
}

// TestArgs: 인자 가운데 시험이 지은·만든 임시 경로(TestPath)가 있다
func TestArgs(args []string) bool {
	return slices.ContainsFunc(args, TestPath)
}

// TestPath: 시험 임시 경로 — 길 조각이 herdr-test-bin.*(clitest가 지은 도구)·herdr-verify-ram-*(검증 메모리 디스크)이거나,
// 임시 폴더(macOS …/T, /tmp) 바로 아래 Test*(go test의 t.TempDir) (2026-11-26 ht-test-orphan-leak)
func TestPath(p string) bool {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "herdr-test-bin.") || strings.HasPrefix(s, "herdr-verify-ram-") ||
			i > 0 && strings.HasPrefix(s, "Test") && (segs[i-1] == "T" || segs[i-1] == "tmp") {
			return true
		}
	}
	return false
}

// DirFor: <state>/herdr-tools/resident — state가 비면 <home>/.local/state
func DirFor(state, home string) string {
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "herdr-tools", "resident")
}

// Dir: ${XDG_STATE_HOME:-~/.local/state}/herdr-tools/resident
func Dir() string {
	h, _ := os.UserHomeDir()
	return DirFor(os.Getenv("XDG_STATE_HOME"), h)
}

func pidsDir(dir string) string { return filepath.Join(dir, "pids") }

// NewMark: 이 프로세스의 표시
func NewMark(who, path string, st os.FileInfo, now time.Time) Mark {
	return Mark{Who: who, Pid: os.Getpid(), Path: path, Commit: version.Commit, Size: st.Size(), Mtime: st.ModTime(), At: now,
		Term: os.Getenv("HC_TERMINAL_ID"), Label: os.Getenv("XPC_SERVICE_NAME"), Args: os.Args}
}

// Save: 이 프로세스의 표시를 Dir()에 — selfexec.New가 부른다
func Save(who, path string, st os.FileInfo) error {
	return Write(Dir(), NewMark(who, path, st, time.Now()))
}

// Write: dir/pids/<pid>.json(같은 pid면 덮어씀 — 스스로 넘어간 새 판)
func Write(dir string, m Mark) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pidsDir(dir), 0o755); err != nil {
		return fmt.Errorf("판 표시 폴더: %w", err)
	}
	return atomic.WriteFile(filepath.Join(pidsDir(dir), strconv.Itoa(m.Pid)+".json"), append(b, '\n'), 0o644)
}

// Load: 표시 모두(pid → 표시). 폴더가 없으면 빈 것. 깨진 한 장은 없는 것으로(그 프로세스는 판 모름 — 같음으로 읽지 않는다)
func Load(dir string) (map[int]Mark, error) {
	ms := map[int]Mark{}
	es, err := os.ReadDir(pidsDir(dir))
	if errors.Is(err, os.ErrNotExist) {
		return ms, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		pid, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(pidsDir(dir), e.Name()))
		if err != nil {
			continue
		}
		var m Mark
		if json.Unmarshal(b, &m) == nil && m.Pid == pid {
			ms[pid] = m
		}
	}
	return ms, nil
}

// Prune: alive에 없는 pid의 표시를 지운다(끝난 프로세스). 지운 수
func Prune(dir string, marks map[int]Mark, alive map[int]bool) int {
	n := 0
	for pid := range marks {
		if !alive[pid] && os.Remove(filepath.Join(pidsDir(dir), strconv.Itoa(pid)+".json")) == nil {
			n++
		}
	}
	return n
}

// psLayout: LC_ALL=C ps -o lstart 꼴
const psLayout = "Mon Jan _2 15:04:05 2006"

// ParsePS: LC_ALL=C ps -axww -o pid=,lstart=,command= 출력 → Proc(풀 수 없는 줄은 건너뜀). 인자는 빈칸으로 나눈 것(빈칸 든 인자는 갈라진다 —
// 상주 종류·다시 띄우기 글에만 쓴다)
func ParsePS(out string, loc *time.Location) []Proc {
	var ps []Proc
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 7 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		st, err := time.ParseInLocation(psLayout, strings.Join(f[1:6], " "), loc)
		if err != nil {
			continue
		}
		ps = append(ps, Proc{Pid: pid, Start: st, Args: f[6:]})
	}
	return ps
}

// EnvOf: ps -wwwE -p <pid> -o command= 출력에서 HC_TERMINAL_ID·XPC_SERVICE_NAME(값에 빈칸 없음)
func EnvOf(out string) (term, label string) {
	for _, w := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(w, "HC_TERMINAL_ID="); ok {
			term = v
		}
		if v, ok := strings.CutPrefix(w, "XPC_SERVICE_NAME="); ok {
			label = v
		}
	}
	return term, label
}

// Marked: 그 프로세스의 표시 — pid가 같고 프로세스가 뜬 뒤 쓰인 것만(ps 시작 시각은 초 단위라 1초 여유). 뜨기 전 표시는 pid 재사용
func Marked(p Proc, marks map[int]Mark) (Mark, bool) {
	m, ok := marks[p.Pid]
	if !ok || m.At.Before(p.Start.Add(-time.Second)) {
		return Mark{}, false
	}
	return m, true
}

func stamp(t time.Time) string { return t.Local().Format("01-02 15:04") }

func verText(commit string, mt time.Time) string {
	if commit != "" {
		return commit + " " + stamp(mt)
	}
	return stamp(mt)
}

// Scan: 옛 판·판 모름인 상주(유예를 넘긴 것만). stat은 실행 파일 경로 → 지금 깔린 판(못 읽으면 false)
func Scan(procs []Proc, marks map[int]Mark, stat func(string) (Bin, bool), now time.Time, grace time.Duration) []Finding {
	var o []Finding
	for _, p := range procs {
		k, ok := KindOf(p.Args)
		if !ok || now.Sub(p.Start) < grace {
			continue
		}
		m, marked := Marked(p, marks)
		path := p.Args[0]
		if marked {
			path = m.Path
		}
		var b Bin
		have := false
		if filepath.IsAbs(path) {
			b, have = stat(path)
		}
		if have && now.Sub(b.Mtime) < grace {
			continue // 방금 깔렸다 — 다음 바퀴에 스스로 넘어갈 수 있다
		}
		f := Finding{Who: k.Who, Pid: p.Pid, Start: p.Start, Term: p.Term, Installed: "모름"}
		if have {
			f.Installed, f.InstAt = stamp(b.Mtime), b.Mtime
		}
		args, label := p.Args, p.Label
		if marked {
			if have && m.Size == b.Size && m.Mtime.Equal(b.Mtime) {
				continue
			}
			f.State, f.Have, f.Term, args, label = "old", verText(m.Commit, m.Mtime), m.Term, m.Args, m.Label
		} else {
			f.State, f.Have = "unknown", "모름"
		}
		f.Restart = restart(k.Who, p.Pid, f.Term, label, args)
		o = append(o, f)
	}
	sort.Slice(o, func(i, j int) bool { return o[i].Pid < o[j].Pid })
	return o
}

// restart: 다시 띄우는 명령. hc 터미널이면 그 터미널을(hc 터미널 안의 프로세스는 코어의 launchd 라벨을 물려받으니 라벨보다 먼저),
// launchd면 kickstart, 둘 다 모르면 pid와 같은 명령
func restart(who string, pid int, term, label string, args []string) string {
	owner := ""
	for i, a := range args {
		if a == "--to" && i+1 < len(args) {
			owner = args[i+1]
		} else if v, ok := strings.CutPrefix(a, "--to="); ok {
			owner = v
		}
	}
	cmd := strings.Join(args, " ")
	switch {
	case who == "hc-lane watch" && owner != "" && term != "":
		return "hc kill " + term + " && hc-lane coord --watch " + owner
	case who == "hc-lane watch" && owner != "":
		return "kill " + strconv.Itoa(pid) + " && hc-lane coord --watch " + owner
	case term != "":
		return "hc kill " + term + " (띄운 지킴이·감독이 다시 띄운다 — 안 뜨면 같은 명령 " + cmd + ")"
	case label != "" && label != "0" && !strings.HasPrefix(label, "application."):
		return "launchctl kickstart -k gui/$(id -u)/" + label
	}
	return "kill " + strconv.Itoa(pid) + " 뒤 같은 명령 " + cmd
}

// Text: 사건·상태 한 줄
func (f Finding) Text() string {
	where := "pid " + strconv.Itoa(f.Pid)
	if f.Term != "" {
		where = f.Term + " " + where
	}
	ver := "판 " + f.Have
	if f.State == "unknown" {
		ver = "판 모름 — 표시 없음(시작 " + stamp(f.Start) + ")"
	}
	return fmt.Sprintf("옛 판으로 도는 상주: %s(%s, 시작 %s) — %s · 깔린 판 %s — 다시 띄우기: %s", f.Who, where, stamp(f.Start), ver, f.Installed, f.Restart)
}

// Key: 사건 키 — 같은 프로세스(pid·시작)·같은 깔린 판에는 한 번(다시 배포했는데도 옛 판이면 그 판으로 한 번 더)
func (f Finding) Key(machine string) string {
	inst := "-"
	if !f.InstAt.IsZero() {
		inst = strconv.FormatInt(f.InstAt.Unix(), 10)
	}
	return fmt.Sprintf("hc-stale-resident:%s:%d:%d:%s", machine, f.Pid, f.Start.Unix(), inst)
}

func ReportPath(dir string) string { return filepath.Join(dir, "report.json") }

// LoadReport: 없으면 빈 한 장(오류 없음)
func LoadReport(path string) (Report, error) {
	var r Report
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Report{}, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

func SaveReport(path string, r Report) error {
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomic.WriteFile(path, append(b, '\n'), 0o644)
}

// Lines: 상태·배포 상태에 보일 줄(있을 때만). 마지막 바퀴가 재지 못했으면 그 까닭 한 줄(앞 결과는 그대로 보인다)
func Lines(r Report, now time.Time) []string {
	var o []string
	if r.Unknown != "" {
		o = append(o, "상주 판 점검 모름: "+r.Unknown)
	}
	for _, f := range r.Findings {
		o = append(o, f.Text()+" · 확인 "+now.Sub(r.Checked).Round(time.Minute).String()+" 전")
	}
	return o
}

// Show: 이 기계 결과 한 장의 줄 — 못 읽으면 그 까닭 한 줄(모름은 모름)
func Show(dir string, now time.Time) []string {
	r, err := LoadReport(ReportPath(dir))
	if err != nil {
		return []string{"상주 판 점검 결과를 못 읽음: " + err.Error()}
	}
	return Lines(r, now)
}
