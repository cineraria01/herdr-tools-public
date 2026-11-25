package main

// ht-advise-land-remote(2026-11-26): 병합 줄 칸의 재료 — 병합기 일지 마지막 줄이 워커 착지 실행 결정이면 그 기계의 살아 있는
// land-<차례 8> 터미널과 그 착지 로그·작업 트리(<클론>.land-wt/<차례 12>)가 손잡이 분 안에 갱신됐나(추측과 같은 verified.FreshSh).
// 갱신 중이면 병합기가 착지 중(Landing) — advise가 안 움직임으로 세지 않는다. 고치기 전 코드에는 stateLandOn이 없어 컴파일부터 실패한다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/opsstate"
)

func TestStateLandOn(t *testing.T) {
	old := LOCAL
	defer func() { LOCAL = old }()
	LOCAL = "here"
	d := t.TempDir()
	clone := filepath.Join(d, "app")
	wt := clone + ".land-wt/ab12cd34ef56"
	for _, p := range []string{clone, wt + "/node_modules"} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	write(wt+".log", now.Add(-time.Hour))
	write(wt+"/node_modules/x.js", now) // 빼고 본다
	f := opsstate.JournalFacts{LandOn: "here", LandOnLane: "ln-a", LandOnAt: now.Add(-18 * time.Minute)}
	terms := []hcjson.Terminal{{Address: "t1", State: "live", Name: "spec-ab12cd34", Cwd: clone},
		{Address: "t5", State: "live", Name: "land-ab12cd34", Cwd: clone}}
	look := func(f opsstate.JournalFacts, terms []hcjson.Terminal, min int) opsstate.MergeLine {
		var g opsstate.MergeLine
		stateLandOn(&g, f, terms, min, now)
		return g
	}
	// 워커 착지 멈춤(로그 한 시간 전) — 착지 중 아님, 기계·터미널·나이
	if g := look(f, terms, 10); g.LandOn != "here" || g.LandOnTerm != "t5" || g.LandOnFresh || g.Landing != "" || g.LandOnAgeS == nil || *g.LandOnAgeS != 18*60 {
		t.Fatalf("멈춤: %+v", g)
	}
	// 갱신 중(착지 로그가 방금 늘어남) — 병합기가 그 레인을 착지 중, 나이는 착지 실행 결정부터
	write(wt+".log", now)
	if g := look(f, terms, 10); !g.LandOnFresh || g.Landing != "ln-a" || g.LandingAgeS == nil || *g.LandingAgeS != 18*60 {
		t.Errorf("갱신 중: %+v", g)
	}
	// 터미널 없음(끝났거나 죽음 — 끝난 터미널·다른 기계 터미널은 안 셈)
	gone := []hcjson.Terminal{{Address: "t5", State: "finished", Name: "land-ab12cd34", Cwd: clone},
		{Address: "box-2/t9", State: "live", Name: "land-ab12cd34", Cwd: clone}}
	if g := look(f, gone, 10); g.LandOn != "here" || g.LandOnTerm != "" || g.LandOnFresh || g.Landing != "" {
		t.Errorf("터미널 없음: %+v", g)
	}
	// 워커 착지 아님 · 손잡이 0 — 빈 값(지금 그대로)
	for _, g := range []opsstate.MergeLine{look(opsstate.JournalFacts{}, terms, 10), look(f, terms, 0)} {
		if g.LandOn != "" || g.Landing != "" {
			t.Errorf("빈 값이어야: %+v", g)
		}
	}
}

// status 병합기 줄: 일지 마지막 줄이 워커 착지 실행 결정이면 「착지 실행: <기계> <나이>째(<까닭>)」(나이는 같은 줄의 다른 칸처럼
// stDur), 그 뒤 결과 줄이 있으면 지금처럼
func TestMergerLineLandOnRunning(t *testing.T) {
	maFx(t, maQ("w1", 20*time.Minute), "")
	st := t.TempDir()
	t.Setenv("XDG_STATE_HOME", st)
	os.MkdirAll(st+"/hc-lane", 0o777)
	at := time.Now().Add(-18 * time.Minute).UTC().Format(time.RFC3339)
	row := func(lane, result, what string) string {
		return strings.Join([]string{at, "1-" + lane, lane, "-", "-", "-", "1", at, "-", result, "-", what + " — 구성원 " + lane}, "\t") + "\n"
	}
	j := st + "/hc-lane/merger-journal.tsv"
	os.WriteFile(j, []byte(row("b", "착지 실행", "box-2(검증 자리 남음 · 코어당 부하 0.43)")), 0o666)
	confLines = []string{"repo app m " + DATA, "merger app m"}
	TERMS = []Term{{M: "m", Addr: "m/t1", Agent: "zsh", Name: "merger-app", Status: "-", Cwd: "/c/app"}}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: box-2 18m째(검증 자리 남음 · 코어당 부하 0.43)") {
		t.Fatalf("워커 착지 중: %q", l)
	}
	os.WriteFile(j, []byte(row("b", "착지 실행", "box-2(검증 자리 남음 · 코어당 부하 0.43)")+row("b", "착지", "b 넣음")), 0o666)
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: box-2(검증 자리 남음 · 코어당 부하 0.43)") {
		t.Fatalf("착지 끝남: %q", l)
	}
}
