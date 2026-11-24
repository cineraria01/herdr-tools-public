package main

import (
	"os"
	"strings"
	"testing"
)

// sweep-landed-keep: hc keep이 켜진 레인 터미널은 에이전트가 끝나면 셸로 바뀌어 status 없이 live로 남는다(2026-11-21 mini-m4
// land-claim-net — 착지·정리 뒤 13시간 자리 하나). 상태 없는 hc-lane 레인이라도 그 레인이 끝났으면(작업 트리 없음·이 실행이 제출함)
// 자리(SEATS)로도, 그 기계 레인 수(lanesOn)로도 세지 않는다. 띄운 직후(작업 트리 있고 사건 없음)는 지금처럼 센다
func TestCountSeatsEndedKeepShell(t *testing.T) {
	setData(t)
	HOME = t.TempDir()
	defer func(o bool) { dry = o }(dry) // 뒤 시험에 dry를 남기지 않는다
	dry, AGENT, LOCAL = true, "claude-code", "here"
	ALLM, AUDN, AWT, NOWL, EVBAD = nil, nil, nil, nil, false
	r := t.TempDir()
	for _, d := range []string{"/r.wt/sub", "/r.wt/fresh"} {
		if err := os.MkdirAll(r+d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	key := func(a string) string { return a + "@1795200000000" }
	try := ""
	for _, x := range [][2]string{{"gone", "x1"}, {"sub", "x2"}, {"fresh", "x3"}} {
		try += strings.Join([]string{x[0], "rl1", LOCAL, "-", "-", x[1], key(x[1])}, "\t") + "\n"
	}
	if err := os.WriteFile(DATA+"/try.tsv", []byte(try), 0o644); err != nil {
		t.Fatal(err)
	}
	EV = []string{strings.Join([]string{"2026-11-21T13:47:00Z", LOCAL, "x2", "sub", "제출", "abc", "-", "3", "-", key("x2")}, "\t")}
	LANEADDR = []string{"x1", "x2", "x3"}
	TERMS = []Term{
		{LOCAL, "x1", "zsh", "gone", "-", r + "/r.wt/gone"},   // 정리 끝(작업 트리 없음) — keep이 셸로 바꿈
		{LOCAL, "x2", "zsh", "sub", "-", r + "/r.wt/sub"},     // 병합기에 제출하고 끝남
		{LOCAL, "x3", "env", "fresh", "-", r + "/r.wt/fresh"}, // 띄운 직후 — 상태 전
	}
	countSeats()
	if got := strings.Join(SEATS, ","); got != LOCAL {
		t.Fatalf("SEATS %q, 바람 %q(띄운 직후 하나만)", got, LOCAL)
	}
	ROWS = nil
	for _, x := range [][2]string{{"gone", "x1"}, {"sub", "x2"}, {"fresh", "x3"}} {
		ROWS = append(ROWS, pad16(x[0], "p", "/b.md", "-", "-", "up", LOCAL, x[1]))
	}
	if n := lanesOn(LOCAL); n != 1 {
		t.Fatalf("lanesOn %d, 바람 1(끝난 keep 셸 레인은 안 셈)", n)
	}
}
