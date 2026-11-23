package main

// watch-window-stale-stop(2026-11-23): 바퀴 창(끝 500줄) 밖으로 착지 줄이 빠지고 정리됨 줄만 창 안에 남으면
// 「멈춤 뜻밖의 종료(정리됨)」로 되살아났다(mini-m4 yt-cal-chip, 4일 뒤). 판정 문맥은 창보다 넓게 읽는다.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvWindowStaleCleaned(t *testing.T) {
	d := t.TempDir()
	row := func(ts, term, lane, st, sha, msg, turn string) string {
		return strings.Join([]string{ts, "m", term, lane, st, sha, msg, turn}, tab)
	}
	ls := []string{
		row("2026-11-19T02:43:00Z", "t7qp", "yt-cal-chip", "착지", "abc1234", "착지", "3"),
		row("2026-11-19T02:44:00Z", "t8aa", "dead-mid", "중간", "-", "작업 중", "3"),
		row("2026-11-19T03:17:19Z", "t7qp", "yt-cal-chip", "정리됨", "-", "hc-sweep R2", "1"),
		row("2026-11-19T03:18:00Z", "t8aa", "dead-mid", "정리됨", "-", "hc-sweep R2", "4"),
		row("2026-11-19T03:19:00Z", "t9bb", "no-report", "정리됨", "-", "hc-sweep R2", "4"),
	}
	for i := 0; len(ls) < evWin+2; i++ { // 첫 두 줄(착지·중간)만 창 밖, 정리됨 셋은 창 안
		ls = append(ls, row(fmt.Sprintf("2026-11-20T00:%02d:%02dZ", i/60%60, i%60), "t1zz", fmt.Sprintf("fill-%d", i), "중간", "-", "x", "1"))
	}
	if err := os.WriteFile(filepath.Join(d, "events.tsv"), []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &ev{ed: d, cur: filepath.Join(d, "cur")}
	ctx, win := e.evreadCtx(evWin, evCtx)
	if len(win) != evWin || len(ctx) != 2 || !strings.Contains(win[0], "yt-cal-chip"+tab+"정리됨") {
		t.Fatalf("창 %d줄·문맥 %d줄(기대 %d·2), 창 첫 줄 %q", len(win), len(ctx), evWin, win[0])
	}
	stop := map[string]bool{}
	for _, l := range e.newEvents(ctx, win, "") {
		if f := strings.Split(l, tab); f[4] == "멈춤" {
			stop[f[3]] = true
		}
	}
	if stop["yt-cal-chip"] {
		t.Errorf("착지가 창 밖이라도 착지 뒤 정리됨은 알리지 않아야 한다")
	}
	if !stop["dead-mid"] {
		t.Errorf("작업 중(중간) 뒤 정리됨은 앞 보고가 창 밖이어도 뜻밖의 종료로 알려야 한다")
	}
	if !stop["no-report"] {
		t.Errorf("보고 없이 정리된 레인은 알려야 한다")
	}
}
