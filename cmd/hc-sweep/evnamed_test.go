package main

// events-named-cols(2026-11-24): hc-sweep의 사건 판독·「정리됨」 쓰기가 internal/events를 거친다 — 판독은 옮기기 전(old*)과 같고,
// 쓰기는 report와 같은 상한(64바이트 칸·1000바이트 줄·탭 지우기)을 지킨다

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

func j2Ev(r []string) events.Event { return events.Cols(strings.Join(r, "\t")) }

// laneEventLine: 파일에 적히는 「정리됨」 줄(laneEvent의 done(true)가 쓰는 꼴)
func laneEventLine(t map[string]any, why string) (string, error) {
	e, err := laneEventOf(t, why)
	return cleanLine(e), err
}

func oldThisRun(t map[string]any, row []string) bool {
	created, ok := num(t, "created_ms")
	if !ok || created <= 0 {
		return false
	}
	if len(row) >= 10 && row[9] != "-" {
		return row[9] == str(t, "address")+"@"+pyStr(t["created_ms"])
	}
	at, err := time.Parse("2006-01-02T15:04:05Z", row[0])
	return err == nil && len(row) > 2 && row[2] == str(t, "address") && afterCreated(at.Unix(), created)
}

// ① 칸 수별 견본(8·10·11·12·13·14칸, 빈 실행 칸, 짧은 줄)을 읽은 판정이 옮기기 전과 같다
func TestSweepEventsSameAsBefore(t *testing.T) {
	rows := []string{
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t제출\tabc1234\tsub\t3",
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t착지\tabc1234\tsub\t3\t-\tt1@1795000000000",
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t착지\tabc1234\tsub\t3\t-\t\tt9",
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t완료\tabc1234\tsub\t3\t-\t-\tt9@1\t-",
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t판정\tabc1234\tsub\t3\t-\tt1@1795000000000\tt9@1\t-\tgithub.com/org/repo",
		"2026-11-21T10:00:00Z\tm\tt1\tlane-a\t정리됨\t-\thc-sweep x\t3\t-\t-\t-\t-\t-\tlane",
		"2026-11-21T10:00:00Z\tm\tt1", "x", "",
	}
	terms := []map[string]any{j2Term("t1", "lane-a", 1795000000000), j2Term("t1", "lane-a", 1600000000000), j2Term("t2", "lane-a", 1600000000000), j2Term("t1", "lane-a", 0)}
	for _, l := range rows {
		for _, tm := range terms {
			if a, b := thisRun(tm, events.Cols(l)), oldThisRun(tm, strings.Split(l, "\t")); a != b {
				t.Errorf("thisRun(%v, %q) = %v, 전 %v", tm, l, a, b)
			}
		}
	}
	ev := filepath.Join(t.TempDir(), "events.tsv")
	for n := 1; n <= len(rows); n++ {
		if err := os.WriteFile(ev, []byte(strings.Join(rows[:n], "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := eventRows(ev)
		if err != nil || len(got) != n {
			t.Fatalf("eventRows %d줄: %d %v", n, len(got), err)
		}
		for i, r := range got {
			if r.Raw() != rows[i] {
				t.Errorf("eventRows가 줄을 바꿨다: %q → %q", rows[i], r.Raw())
			}
		}
	}
}

// ② 보통 입력의 「정리됨」 줄은 옮기기 전과 바이트 그대로(시각 칸 빼고 — 지금 시각)
func TestSweepLaneEventLineSame(t *testing.T) {
	t.Setenv("HC_LANE_MACHINE", "box")
	old := func(tm map[string]any, why string) string {
		turn := "-"
		if tm["turn"] != nil {
			turn = pyStr(tm["turn"])
		}
		f := []string{"T", "box", str(tm, "address"), dash(str(tm, "name")), "정리됨", "-", "hc-sweep " + why, turn}
		for i := range f {
			f[i] = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(f[i])
		}
		return strings.Join(f, "\t")
	}
	for _, tm := range []map[string]any{
		{"address": "t1", "name": "lane-a", "turn": float64(3)},
		{"address": "t1", "name": "", "turn": nil},
		{"address": "t1", "name": "a\tb", "turn": float64(12)},
	} {
		for _, why := range []string{"R2", "쉰 지 30분\n둘째 줄"} {
			got, err := laneEventLine(tm, why)
			if err != nil {
				t.Fatal(err)
			}
			_, gt, _ := strings.Cut(got, "\t")
			_, wt, _ := strings.Cut(old(tm, why), "\t")
			if gt != wt {
				t.Errorf("%v %q:\n%q\n%q", tm, why, gt, wt)
			}
		}
	}
}

// ④ 너무 긴 칸은 report와 같은 상한으로 — 레인·터미널 64바이트, 줄 1000바이트, 늘 8칸(옛 판은 그대로 이어 1000바이트를 넘겼다)
func TestSweepLaneEventLineLimits(t *testing.T) {
	t.Setenv("HC_LANE_MACHINE", strings.Repeat("m", 100))
	tm := map[string]any{"address": strings.Repeat("a", 100), "name": strings.Repeat("레", 50), "turn": float64(3)}
	l, err := laneEventLine(tm, strings.Repeat("가", 1000))
	if err != nil {
		t.Fatal(err)
	}
	e := events.Cols(l)
	if e.N != 8 || len(l) > events.MaxLine || len(e.Machine) != 64 || len(e.Term) != 64 || len(e.Lane) > 64 || e.State != "정리됨" || e.Turn != "3" {
		t.Fatalf("8칸·1000바이트·64바이트 칸: N %d 줄 %d 기계 %d 터미널 %d 레인 %d %q %q", e.N, len(l), len(e.Machine), len(e.Term), len(e.Lane), e.State, e.Turn)
	}
	// 지킬 수 없는 줄(턴 칸이 한 줄 칸 자리까지 먹음)은 쓰지 않는다 — kill 전(laneEvent)에 못 씀(ok=false)으로 닫지 않고 파일도 안 만든다
	big := map[string]any{"address": "t1", "name": "lane-a", "turn": strings.Repeat("9", 1200), "labels": map[string]any{"hc-lane": "1"}}
	if _, err := laneEventLine(big, "x"); err == nil {
		t.Fatal("상한을 못 지키면 오류여야")
	}
	saved := laneDir
	laneDir = t.TempDir()
	defer func() { laneDir = saved }()
	if _, ok := laneEvent(big, "x"); ok {
		t.Fatal("kill 전에 못 씀이어야")
	}
	if _, err := os.Stat(filepath.Join(laneDir, "events.tsv")); !os.IsNotExist(err) {
		t.Fatalf("파일을 건드리지 않아야: %v", err)
	}
}
