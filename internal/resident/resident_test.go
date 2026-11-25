package resident

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// 상주 판 점검(stale-resident-check) — 표시·ps 풀기·판정. 시각은 고정 값(실제 기다림 없음)

var t0 = time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)

const grace = 5 * time.Minute

func watchProc(pid int, start time.Time) Proc {
	return Proc{Pid: pid, Start: start, Args: []string{"/u/.local/bin/hc-lane", "watch", "--to", "t3yx"}}
}

func markFor(pid int, at time.Time, b Bin) Mark {
	return Mark{Who: "hc-lane watch", Pid: pid, Path: "/u/.local/bin/hc-lane", Size: b.Size, Mtime: b.Mtime, At: at,
		Term: "t9zz", Args: []string{"/u/.local/bin/hc-lane", "watch", "--to", "t3yx"}}
}

func statOf(b Bin) func(string) (Bin, bool) { return func(string) (Bin, bool) { return b, true } }

var (
	oldBin = Bin{Size: 100, Mtime: t0.Add(-26 * time.Hour)}
	newBin = Bin{Size: 120, Mtime: t0.Add(-10 * time.Minute)}
)

// ① 깔린 판과 다른 판 표시 + 유예 지남 → old 하나, 다시 띄우는 명령 포함
func TestScanOldAfterGrace(t *testing.T) {
	start := t0.Add(-20 * time.Hour)
	f := Scan([]Proc{watchProc(41, start)}, map[int]Mark{41: markFor(41, start.Add(time.Second), oldBin)}, statOf(newBin), t0, grace)
	if len(f) != 1 || f[0].State != "old" || f[0].Pid != 41 || f[0].Who != "hc-lane watch" {
		t.Fatalf("옛 판 하나여야: %+v", f)
	}
	if !strings.Contains(f[0].Restart, "hc kill t9zz") || !strings.Contains(f[0].Restart, "hc-lane coord --watch t3yx") {
		t.Fatalf("다시 띄우는 명령이 없다: %q", f[0].Restart)
	}
	txt := f[0].Text()
	for _, w := range []string{"옛 판으로 도는 상주: hc-lane watch", "t9zz", "시작 ", "다시 띄우기: " + f[0].Restart} {
		if !strings.Contains(txt, w) {
			t.Fatalf("글에 %q 없음: %s", w, txt)
		}
	}
}

// ② 깔린 지 유예 안 → 알림 없음(다음 바퀴에 스스로 넘어갈 수 있다)
func TestScanWithinGrace(t *testing.T) {
	start := t0.Add(-20 * time.Hour)
	just := Bin{Size: 130, Mtime: t0.Add(-2 * time.Minute)}
	if f := Scan([]Proc{watchProc(41, start)}, map[int]Mark{41: markFor(41, start, oldBin)}, statOf(just), t0, grace); len(f) != 0 {
		t.Fatalf("유예 안에 알림: %+v", f)
	}
	// 방금 뜬 프로세스(아직 표시를 못 남겼을 수 있다)도 유예 안이면 모름으로 읽지 않는다
	if f := Scan([]Proc{watchProc(42, t0.Add(-time.Minute))}, nil, statOf(oldBin), t0, grace); len(f) != 0 {
		t.Fatalf("방금 뜬 프로세스에 알림: %+v", f)
	}
}

// ③ 판 표시가 없는 옛 프로세스 → 「판 모름」(같음으로 읽지 않는다)
func TestScanNoMarkIsUnknown(t *testing.T) {
	p := watchProc(43, t0.Add(-20*time.Hour))
	p.Term = "t1ab"
	f := Scan([]Proc{p}, nil, statOf(newBin), t0, grace)
	if len(f) != 1 || f[0].State != "unknown" {
		t.Fatalf("판 모름 하나여야: %+v", f)
	}
	if txt := f[0].Text(); !strings.Contains(txt, "판 모름 — 표시 없음(시작 ") || !strings.Contains(txt, "t1ab") {
		t.Fatalf("판 모름 글: %s", txt)
	}
	// pid가 같아도 프로세스가 뜨기 전에 쓰인 표시(pid 재사용)는 표시가 아니다
	old := markFor(43, t0.Add(-30*time.Hour), newBin)
	if f := Scan([]Proc{p}, map[int]Mark{43: old}, statOf(newBin), t0, grace); len(f) != 1 || f[0].State != "unknown" {
		t.Fatalf("pid 재사용 표시를 믿었다: %+v", f)
	}
}

// ④ 스스로 넘어간 프로세스: 새 판이 같은 pid로 표시를 다시 쓰면 알림 없음
func TestSelfReexecRewritesMark(t *testing.T) {
	dir := t.TempDir()
	start := t0.Add(-20 * time.Hour)
	if err := Write(dir, markFor(44, start, oldBin)); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, markFor(44, t0.Add(-8*time.Minute), newBin)); err != nil { // 넘어간 새 판이 New에서 다시
		t.Fatal(err)
	}
	ms, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ms[44].Size != newBin.Size {
		t.Fatalf("표시가 새 판으로 안 바뀜: %+v", ms[44])
	}
	if f := Scan([]Proc{watchProc(44, start)}, ms, statOf(newBin), t0, grace); len(f) != 0 {
		t.Fatalf("넘어간 프로세스에 알림: %+v", f)
	}
}

// 상주 표에 없는 프로세스(같은 바이너리의 단발 명령)는 보지 않는다
func TestScanIgnoresOneShot(t *testing.T) {
	p := Proc{Pid: 45, Start: t0.Add(-20 * time.Hour), Args: []string{"/u/.local/bin/hc-lane", "report", "중간"}}
	if f := Scan([]Proc{p}, nil, statOf(newBin), t0, grace); len(f) != 0 {
		t.Fatalf("단발 명령을 상주로 봄: %+v", f)
	}
}

func TestNewMarkFromEnv(t *testing.T) {
	t.Setenv("HC_TERMINAL_ID", "t7qq")
	t.Setenv("XPC_SERVICE_NAME", "com.example.watch")
	p := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(p, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMark("hc-fill watch", p, st, t0)
	if m.Pid != os.Getpid() || m.Term != "t7qq" || m.Label != "com.example.watch" || m.Size != 2 || !m.Mtime.Equal(st.ModTime()) || len(m.Args) == 0 {
		t.Fatalf("표시 칸: %+v", m)
	}
}

// launchd가 띄운 상주는 kickstart로
func TestRestartLaunchd(t *testing.T) {
	p := Proc{Pid: 46, Start: t0.Add(-20 * time.Hour), Args: []string{"/u/.local/bin/hc-job-server"}, Label: "com.example.jobs"}
	f := Scan([]Proc{p}, nil, statOf(newBin), t0, grace)
	if len(f) != 1 || !strings.Contains(f[0].Restart, "launchctl kickstart -k gui/") || !strings.Contains(f[0].Restart, "/com.example.jobs") {
		t.Fatalf("launchd 다시 띄우기: %+v", f)
	}
}

// selfexec.New를 부르는 상주(cmd/…)는 모두 상주 표에 있다 — 새 상주를 더하고 표를 잊으면 그 상주의 판 표시가 버려진다
func TestKindsCoverSelfexecCallers(t *testing.T) {
	re := regexp.MustCompile(`selfexec\.New\((\w+|"[^"]+")`)
	var found []string
	err := filepath.WalkDir("../../cmd", func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			who := strings.Trim(m[1], `"`)
			if who == "me" { // hc-job-server: const me = "hc-job-server"
				who = "hc-job-server"
			}
			found = append(found, who)
			ok := false
			for _, k := range Kinds {
				ok = ok || k.Who == who
			}
			if !ok {
				t.Errorf("%s: selfexec.New(%q)가 상주 표(Kinds)에 없다", p, who)
			}
		}
		return nil
	})
	if err != nil || len(found) < len(Kinds) {
		t.Fatalf("selfexec.New 부르는 곳 %v (%v) — 표 %d줄보다 적다", found, err, len(Kinds))
	}
}

// 병합기 바깥 고리만 상주(ht-resident-reexec) — 바퀴 일꾼·추측 일꾼·한 번은 아니다
func TestKindMerger(t *testing.T) {
	for _, c := range []struct {
		args string
		want bool
	}{
		{"/u/.local/bin/hc-merger --repo /u/m main", true},
		{"/u/.local/bin/hc-merger main --repo /u/m --tick 5", true},
		{"/u/.local/bin/hc-merger --tick 5 --repo /u/m main", true},
		{"/u/.local/bin/hc-merger --repo /u/m --once main", false},
		{"/u/.local/bin/hc-merger --round 123 /u/m main", false},
		{"/u/.local/bin/hc-merger --spec-verify --repo /u/m", false},
		{"/u/.local/bin/hc-merger --version", false},
	} {
		k, ok := KindOf(strings.Fields(c.args))
		if ok != c.want || ok && k.Who != "hc-lane merger" {
			t.Errorf("%s: %v %q, 바람 %v", c.args, ok, k.Who, c.want)
		}
	}
}

func TestEnvOf(t *testing.T) {
	term, label := EnvOf("/u/bin/hc-job-server PATH=/bin HC_TERMINAL_ID=t4kk XPC_SERVICE_NAME=com.example.jobs HOME=/u")
	if term != "t4kk" || label != "com.example.jobs" {
		t.Fatalf("환경: %q %q", term, label)
	}
}

func TestParsePS(t *testing.T) {
	out := "  41 Mon Nov 23 16:02:11 2026     /u/.local/bin/hc-lane watch --to t3yx\n" +
		"  77 Tue Nov 24 09:00:00 2026 /usr/bin/true\n이상한 줄\n"
	ps := ParsePS(out, time.Local)
	if len(ps) != 2 || ps[0].Pid != 41 || ps[0].Start.Hour() != 16 || strings.Join(ps[0].Args, " ") != "/u/.local/bin/hc-lane watch --to t3yx" {
		t.Fatalf("ps 풀기: %+v", ps)
	}
}

// ⑥ 결과 한 장 → 보이는 줄(있을 때만)
func TestReportLines(t *testing.T) {
	dir := t.TempDir()
	if l := Lines(Report{}, t0); len(l) != 0 {
		t.Fatalf("없을 때 줄: %v", l)
	}
	f := Scan([]Proc{watchProc(41, t0.Add(-20*time.Hour))}, nil, statOf(newBin), t0, grace)
	if err := SaveReport(ReportPath(dir), Report{Checked: t0, Findings: f}); err != nil {
		t.Fatal(err)
	}
	r, err := LoadReport(ReportPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	l := Lines(r, t0.Add(time.Minute))
	if len(l) != 1 || !strings.Contains(l[0], "hc-lane watch") || !strings.Contains(l[0], "판 모름") {
		t.Fatalf("보이는 줄: %v", l)
	}
}
