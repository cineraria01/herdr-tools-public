package main

// ht-alert-sources(2026-11-28): hc-sweep의 「정리됨」은 그 터미널의 같은 실행 사건에서 얻을 수 있는 주인·저장소·실행 열쇠를 실어 사건
// 공용 쓰기(events.Record)로 적는다 — 한 문이 앞 사건에서 물려받아 어림하지 않게. 같은 실행 사건이 없으면 지금처럼 8칸(물려받기 어림은 남김)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/events"
)

func cleanFx(t *testing.T, rows ...string) {
	t.Helper()
	t.Setenv("HC_LANE_MACHINE", "box")
	saved := laneDir
	laneDir = t.TempDir()
	t.Cleanup(func() { laneDir = saved })
	if err := os.WriteFile(filepath.Join(laneDir, "events.tsv"), []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSweepCleanCarriesRun(t *testing.T) {
	cleanFx(t,
		"2026-11-21T10:00:00Z\tbox\tt1\tlane-a\t완료\tabc1234\t끝\t3\t-\tt1@1795000000000\tc9@1700\t-\texample.com/team/a",
		"2026-11-21T09:00:00Z\tbox\tt1\tlane-a\t완료\tabc1234\t옛 실행\t3\t-\tt1@1600000000000\tc7@1600\t-\texample.com/team/old")
	tm := map[string]any{"address": "t1", "name": "lane-a", "turn": float64(4), "created_ms": float64(1795000000000)}
	l, err := laneEventLine(tm, "R2")
	if err != nil {
		t.Fatal(err)
	}
	e := events.Cols(l)
	if e.State != "정리됨" || e.Msg != "hc-sweep R2" || e.Turn != "4" || e.Run != "t1@1795000000000" || e.Owner != "c9@1700" ||
		e.Repo != "example.com/team/a" || e.N != 13 {
		t.Fatalf("같은 실행의 주인·저장소·실행 열쇠를 실은 13칸: %q", l)
	}
}

// 같은 실행 사건이 없음(옛 실행만·사건 파일 없음·주인 칸 없는 옛 줄) → 지금처럼 8칸
func TestSweepCleanNoRunKeeps8(t *testing.T) {
	for name, rows := range map[string][]string{
		"옛 실행만":   {"2026-11-21T09:00:00Z\tbox\tt1\tlane-a\t완료\tabc1234\t옛 실행\t3\t-\tt1@1600000000000\tc7@1600\t-\texample.com/team/old"},
		"주인 칸 없음": {"2026-11-21T10:00:00Z\tbox\tt1\tlane-a\t완료\tabc1234\t끝\t3\t-\tt1@1795000000000"},
		"빈 파일":    {""},
	} {
		t.Run(name, func(t *testing.T) {
			cleanFx(t, rows...)
			tm := map[string]any{"address": "t1", "name": "lane-a", "turn": float64(4), "created_ms": float64(1795000000000)}
			l, err := laneEventLine(tm, "R2")
			if err != nil {
				t.Fatal(err)
			}
			if e := events.Cols(l); e.N != 8 || e.State != "정리됨" {
				t.Fatalf("8칸 그대로: %q", l)
			}
		})
	}
}
