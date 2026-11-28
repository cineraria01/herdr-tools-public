package main

import (
	"os"
	"strings"
	"testing"
)

// 실물 점검 띄우기(ht-upstream-contract ⑦⑧·운영 장면): 한 번에 한 대(보류 먼저), 같은 판에 두 번 띄우지 않음(결과를 기다림), 결과가 나오면
// 다음 기계, 못 띄움은 상한까지 다음 바퀴에 다시 → 상한에 막힘 한 번, 연결 안 됨·풀 빔인 기계는 점검 없음(다른 기계에 영향 없음)
func TestUpstreamTickOneAtATime(t *testing.T) {
	DATA = t.TempDir()
	LOCAL, dry, CONN = "m", false, []string{"w1", "w2", "w3"}
	LAUNCH = `sh -c 'exec example-agent --model cheap "$1"' _ {task}`
	confLines = nil
	log := DATA + "/calls.log"
	rcf := DATA + "/rc"
	wr := func(f, s string) {
		if err := os.WriteFile(DATA+"/"+f, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wr("rc", "0")
	wr("hc", "#!/bin/sh\nprintf 'hc %s\\n' \"$*\" >>"+log+"\nexit $(cat "+rcf+")\n")
	wr("hc-lane", "#!/bin/sh\nprintf 'hc-lane %s\\n' \"$*\" >>"+log+"\n")
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	calls := func() string { b, _ := os.ReadFile(log); os.Remove(log); return string(b) }
	tick := func(p map[string]string) {
		PROBE = map[string]map[string]string{}
		for m, v := range p {
			PROBE[m] = map[string]string{"upstream": v}
		}
		upstreamTick()
	}
	// 처음 깔림: 모든 기계가 기준 점검 차례(보류 없음), 한 기계는 보류 — 보류 기계 하나만 띄운다
	tick(map[string]string{"w1": "ok claude-1_hc-1", "w2": "hold claude-2_hc-1", "w3": "ok claude-1_hc-1", "w4": "ok claude-1_hc-1"})
	c := calls()
	if strings.Count(c, "hc run") != 1 || !strings.Contains(c, "--on w2") || !strings.Contains(c, "-- zsh -lc exec hc-sweep --upstream-probe -- \"$@\" _ sh -c exec example-agent --model cheap \"$1\" _ {probe}") {
		t.Fatalf("보류 기계 하나만: %q", c)
	}
	if !upHeld("w2") || upHeld("w1") || upHeld("w9") {
		t.Fatal("보류 기록")
	}
	// 두 번째 바퀴: 결과를 기다리는 동안 아무것도 안 띄움(같은 판에 두 번 띄우지 않음)
	tick(map[string]string{"w1": "ok claude-1_hc-1", "w2": "hold claude-2_hc-1", "w3": "ok claude-1_hc-1"})
	if c := calls(); strings.Contains(c, "hc run") {
		t.Fatalf("기다리는 중인데 또 띄움: %q", c)
	}
	// 바퀴 순서 반대(이번 바퀴에 조각이 안 온 기계) — 앞 기록 그대로, 보류는 이어진다
	tick(nil)
	if c := calls(); strings.Contains(c, "hc run") || !upHeld("w2") {
		t.Fatalf("조각 없는 바퀴: %q", c)
	}
	// w2 결과(통과): 다음 기계(이름순 w1)
	tick(map[string]string{"w1": "ok claude-1_hc-1", "w2": "ok -", "w3": "ok claude-1_hc-1"})
	if c := calls(); strings.Count(c, "hc run") != 1 || !strings.Contains(c, "--on w1") || upHeld("w2") {
		t.Fatalf("다음 기계: %q", c)
	}
	// w1 결과 → w3 차례인데 연결 안 됨이면 w4(조각 없음)도 없으니 아무것도 안 띄움, w3은 기록 그대로
	CONN = []string{"w1", "w2"}
	tick(map[string]string{"w1": "ok -", "w3": "ok claude-1_hc-1"})
	if c := calls(); strings.Contains(c, "hc run") {
		t.Fatalf("연결 안 된 기계: %q", c)
	}
	// 못 띄움(일시): 상한까지 바퀴마다 다시, 상한에 막힘 한 번, 그 뒤 조용
	CONN = []string{"w1", "w2", "w3"}
	wr("rc", "1")
	tick(map[string]string{"w3": "hold claude-3_hc-1"})
	tick(map[string]string{"w3": "hold claude-3_hc-1"})
	tick(map[string]string{"w3": "hold claude-3_hc-1"})
	if c := calls(); strings.Count(c, "hc run") != 3 || strings.Contains(c, "hc-lane report") {
		t.Fatalf("상한까지 다시: %q", c)
	}
	tick(map[string]string{"w3": "hold claude-3_hc-1"})
	tick(map[string]string{"w3": "hold claude-3_hc-1"})
	if c := calls(); strings.Contains(c, "hc run") || strings.Count(c, "hc-lane report 막힘 - 실물 점검을 3번 띄웠는데 결과가 없다(w3 · claude-3_hc-1)") != 1 ||
		!strings.Contains(c, "--key hc-upstream:w3:claude-3_hc-1:cap") {
		t.Fatalf("상한 막힘 한 번: %q", c)
	}
	// 새 판이 오면 상한을 0부터
	wr("rc", "0")
	tick(map[string]string{"w3": "hold claude-4_hc-1"})
	if c := calls(); strings.Count(c, "hc run") != 1 {
		t.Fatalf("새 판은 다시: %q", c)
	}
	// 보류가 일을 막지 않게(지휘 정정 3): 보류 기계의 자리 셈은 그대로 — 새 레인·감사가 평소대로 뜬다(보류는 닫기 판정에만 걸린다)
	TERMS, ROWS, AGUNK = nil, nil, nil
	for _, isa := range []bool{false, true} {
		if n, why := machRoom(Machine{Name: "w3", Cap: "3", LCap: "-"}, isa); !upHeld("w3") || n != 3 || why != "" {
			t.Fatalf("보류 기계 자리(감사 %v): %d %q", isa, n, why)
		}
	}
	// 꼴이 틀린 줄(옛 판·unknown)은 앞 기록 그대로
	tick(map[string]string{"w3": "unknown 깨짐"})
	if r := upRows()["w3"]; r.F(usOwed) != "claude-4_hc-1" || r.F(usTries) != "1" {
		t.Fatalf("모름은 앞 기록: %v", r)
	}
}
