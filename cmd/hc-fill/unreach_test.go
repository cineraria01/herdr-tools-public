package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// worker-health-sweep(2026-11-24): 레인이 있는 기계가 unreach분 넘게 연결 안 됨 → 주인 지휘에 막힘 한 번, 되풀이 없음, 붙으면 한 줄.
// 레인이 없는 기계는 끊겨도 사건 없음. 재기는 이 바퀴에 읽은 CONN·TERMS·대기열뿐(가짜 hc-lane만 PATH에)

func urFx(t *testing.T) (events func() string) {
	t.Helper()
	DATA = t.TempDir()
	Q, AT, LOCAL, dry = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", false
	confLines = []string{"unreach 10"}
	LANEADDR = nil
	if err := os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/ev.log'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	return func() string { b, _ := os.ReadFile(DATA + "/ev.log"); return string(b) }
}

// urTick: 바퀴 하나 — 앞 바퀴가 적은 「그 상태 시작」을 back초 앞으로 당긴다(문턱을 기다리지 않게)
func urTick(back int64) {
	var rs []Row
	for _, r := range readRows(reachFile()) {
		rs = append(rs, r.Set(3, fmt.Sprint(int64(atoi10(r.F(3)))-back)))
	}
	if len(rs) > 0 {
		writeAtomic(reachFile(), rowsText(rs))
	}
	unreachTick()
}

func urQ(name, m, owner string) string {
	return strings.Join([]string{name, "app", "b", "-", "-", "up", m, m + "/t5", "-", "t", "-", owner, "normal", "-", "auto", "-", "r1"}, "\t") + "\n"
}

// ⑤ 레인이 있는 기계가 닿지 않음이 상한을 넘김 → 사건 1번(되풀이 없음), 붙으면 회복 한 줄
func TestUnreachLaneMachineOnce(t *testing.T) {
	ev := urFx(t)
	if err := os.WriteFile(Q, []byte(urQ("w1", "m2", "t0")), 0o644); err != nil {
		t.Fatal(err)
	}
	CONN = []string{"m2"}
	TERMS = []Term{{M: "m2", Addr: "m2/t5", Agent: "claude-code", Name: "w1", Status: "working", Cwd: "/w"}}
	urTick(0) // 연결된 동안: 레인 w1을 기억
	CONN, TERMS = nil, nil
	urTick(0) // 끊김 시작 — 문턱 전에는 없음
	if e := ev(); e != "" {
		t.Fatalf("문턱 전 사건: %q", e)
	}
	urTick(11 * 60)
	urTick(11 * 60)
	e := ev()
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "--name hc-fill-unreach") || !strings.Contains(e, "기계 m2") ||
		!strings.Contains(e, "w1") || !strings.Contains(e, "--to t0") || !strings.Contains(e, "--key hf:unreach:m2:") {
		t.Fatalf("끊김 사건 1번이어야: %q", e)
	}
	if !strings.Contains(strings.Join(machineLinesReach(), "\n"), "끊김") {
		t.Fatalf("기계 줄에 끊김이 없다: %v", machineLinesReach())
	}
	CONN = []string{"m2"}
	TERMS = []Term{{M: "m2", Addr: "m2/t5", Agent: "claude-code", Name: "w1", Status: "working", Cwd: "/w"}}
	urTick(0)
	urTick(0)
	e = ev()
	if strings.Count(e, "report 중간") != 1 || !strings.Contains(e, "다시 닿음") || strings.Count(e, "report 막힘") != 1 {
		t.Fatalf("회복 한 줄이어야: %q", e)
	}
}

// ⑥ 레인이 없는 기계(대기열 줄도, 라벨 터미널도 없음)는 끊겨도 사건 없음
func TestUnreachNoLaneSilent(t *testing.T) {
	ev := urFx(t)
	CONN = []string{"m3"}
	TERMS = []Term{{M: "m3", Addr: "m3/t1", Agent: "zsh", Name: "proxy", Status: "-", Cwd: "/"}}
	urTick(0)
	CONN, TERMS = nil, nil
	for i := 0; i < 3; i++ {
		urTick(30 * 60)
	}
	if e := ev(); e != "" {
		t.Fatalf("레인 없는 기계에 사건: %q", e)
	}
}

// 연결된 동안 레인이 없던 기계는 끊겨도 끝난 대기열 up 줄(터미널 없음)로 알리지 않는다 — 처음 본 기계로 읽지 않게
func TestUnreachStaleQueueRowSilent(t *testing.T) {
	ev := urFx(t)
	if err := os.WriteFile(Q, []byte(urQ("w3", "m5", "t0")), 0o644); err != nil {
		t.Fatal(err)
	}
	CONN = []string{"m5"}
	TERMS = []Term{{M: "m5", Addr: "m5/t1", Agent: "zsh", Name: "proxy", Status: "-", Cwd: "/"}} // w3 터미널은 이미 끝남
	urTick(0)
	CONN, TERMS = nil, nil
	urTick(0)
	urTick(30 * 60)
	if e := ev(); e != "" {
		t.Fatalf("끝난 줄로 알림: %q", e)
	}
}

// 끊긴 채 처음 본 기계: 대기열 up 줄로 레인을 안다 · 손잡이 0이면 끔
func TestUnreachFirstSeenDownAndOff(t *testing.T) {
	ev := urFx(t)
	if err := os.WriteFile(Q, []byte(urQ("w2", "m4", "-")), 0o644); err != nil {
		t.Fatal(err)
	}
	CONN, TERMS = nil, nil
	confLines = []string{"unreach 0"}
	urTick(0)
	urTick(60 * 60)
	if e := ev(); e != "" {
		t.Fatalf("unreach 0인데 사건: %q", e)
	}
	confLines = []string{"unreach 10"}
	urTick(11 * 60)
	if e := ev(); strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "w2") || strings.Contains(e, "--to") {
		t.Fatalf("주인 없는 레인은 모든 지휘에 한 번: %q", e)
	}
}

// machineLinesReach: 기계 줄 끝 붙임(설정 machine 줄이 없는 시험이라 기록에서 바로)
func machineLinesReach() []string {
	var o []string
	for m, s := range reachNotes() {
		o = append(o, m+s)
	}
	return o
}
