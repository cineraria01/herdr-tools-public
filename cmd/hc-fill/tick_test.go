package main

// fill-tick-cost(2026-11-24): 바퀴 간격 중앙 93초 — watch가 바퀴를 다 돈 뒤 주기만큼 더 쉬었고, 끝난 떠 있음 줄마다 사건·멈춤 단계가
// hc get을 한 번씩 불러 바퀴가 줄 수에 비례해 늘었다

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// ① 바퀴가 20초 걸리면 주기(60초)에 맞춰 40초만 쉰다
func TestRestForOnPeriod(t *testing.T) {
	if got := restFor(60*time.Second, 20*time.Second, 5*time.Second); got != 40*time.Second {
		t.Fatalf("쉼 %v, 바람 40s", got)
	}
	if got := restFor(60*time.Second, 0, 5*time.Second); got != 60*time.Second {
		t.Fatalf("바로 끝난 바퀴: 쉼 %v, 바람 60s", got)
	}
}

// ② 바퀴가 주기를 넘기거나 주기 끝에 닿으면 하한만 쉰다
func TestRestForFloor(t *testing.T) {
	for _, took := range []time.Duration{90 * time.Second, 60 * time.Second, 57 * time.Second} {
		if got := restFor(60*time.Second, took, 5*time.Second); got != 5*time.Second {
			t.Fatalf("바퀴 %v: 쉼 %v, 바람 하한 5s", took, got)
		}
	}
}

// fakeHC: PATH 앞에 hc get을 받는 가짜 hc(살아 있는 터미널 하나 box-1/live)
func fakeHC(t *testing.T) {
	d := t.TempDir()
	sc := "#!/bin/sh\n[ \"$1\" = get ] || exit 2\n" +
		"echo \"{\\\"address\\\":\\\"$2\\\",\\\"created_ms\\\":1700000000000,\\\"cwd\\\":\\\"/\\\",\\\"name\\\":\\\"${2##*/}\\\",\\\"state\\\":\\\"live\\\",\\\"status\\\":{\\\"app\\\":\\\"claude-code\\\",\\\"state\\\":\\\"working\\\"}}\"\n"
	if err := os.WriteFile(d+"/hc", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}

func hcGets() int {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	return tk.calls["hc get"]
}

// ③ 끝난 떠 있음 줄(터미널 표에 없는 주소, 연결된 기계)은 줄 수가 늘어도 hc get을 부르지 않는다. 표에 있는 주소·끊긴 기계·띄운 뒤(LSSNAP 꺼짐)는
// 지금처럼 hc get으로 본다
func TestJoblastGoneRowsNoHcGet(t *testing.T) {
	setData(t)
	fakeHC(t)
	oe, ol, oc, ot, os_ := exitHooks, LOCAL, CONN, TERMS, LSSNAP
	defer func() { exitHooks, LOCAL, CONN, TERMS, LSSNAP = oe, ol, oc, ot, os_; tk.on = false }()
	LOCAL, CONN, EV = "here", []string{"box-1"}, nil
	TERMS = []Term{{"box-1", "box-1/live", "claude-code", "live", "working", "/"}}
	LSSNAP = true
	for _, n := range []int{10, 300} {
		tickStart()
		for i := 0; i < n; i++ {
			joblast(fmt.Sprintf("old-%d", i), fmt.Sprintf("box-1/a%d", i), "")
		}
		if g := hcGets(); g != 0 {
			t.Fatalf("끝난 줄 %d개: hc get %d번, 바람 0(줄 수에 비례하면 안 됨)", n, g)
		}
	}
	tickStart()
	joblast("live", "box-1/live", "")
	if PNAME != "live" || hcGets() != 1 {
		t.Fatalf("표에 있는 주소는 hc get으로: 이름 %q, hc get %d번", PNAME, hcGets())
	}
	tickStart()
	joblast("x", "box-2/x", "") // 끊긴 기계 — 표의 「없음」을 믿지 않는다
	if g := hcGets(); g != 1 {
		t.Fatalf("끊긴 기계: hc get %d번, 바람 1", g)
	}
	tickStart()
	if pget("box-1/a1"); hcGets() != 1 { // 종료를 확정하는 pget(받은편지 회신)은 표와 상관없이 hc get
		t.Fatalf("pget 직접: hc get %d번, 바람 1", hcGets())
	}
	LSSNAP = false // 띄운 뒤
	tickStart()
	joblast("old-1", "box-1/a1", "")
	if g := hcGets(); g != 1 {
		t.Fatalf("띄운 뒤: hc get %d번, 바람 1", g)
	}
}

// ④ 계측 줄 꼴 — tick.tsv 한 줄 「끝 초 · 바퀴 ms · 단계=ms/명령수 · 명령=횟수」, 로그 줄, tick.tsv는 최근 tickKeep 줄만
func TestTickLineShape(t *testing.T) {
	row := tickRow(1795200000, 31234, []tkStage{{"tables", 210, 4}, {"order", 0, 0}, {"events", 29000, 430}},
		map[string]int{"hc get": 430, "hc ls": 2, "git -C": 0})
	want := "1795200000\t31234\ttables=210/4,events=29000/430\thc get=430,hc ls=2,git -C=0"
	if row != want {
		t.Fatalf("줄\n%q\n바람\n%q", row, want)
	}
	ms, long, lms, n, ok := tickParse(row)
	if !ok || ms != 31234 || long != "events" || lms != 29000 || n != 432 {
		t.Fatalf("읽기: %d %s %d %d %v", ms, long, lms, n, ok)
	}
	l := tickLine(row)
	if !strings.HasPrefix(l, "바퀴 계측: 31.2초 · 가장 긴 단계 events 29.0초 · 바깥 명령 432번 · 단계 tables=210/4,events=29000/430 · 명령 hc get=430") {
		t.Fatalf("로그 줄: %s", l)
	}
	if empty := tickRow(1, 0, nil, nil); empty != "1\t0\t-\t-" {
		t.Fatalf("빈 바퀴: %q", empty)
	}
	if callKey("git", []string{"-C", "/r", "fetch", "-q"}) != "git fetch" || callKey("sh", []string{"-c", "x"}) != "sh" ||
		callKey("hc", []string{"exec", "--on", "m"}) != "hc exec" {
		t.Fatal("명령 열쇠")
	}

	setData(t)
	od, oe := dry, exitHooks
	defer func() { dry, exitHooks = od, oe }()
	dry = false
	for i := 0; i < tickKeep+3; i++ {
		tickStart()
		tickMark("seats")
		tickCall("hc", []string{"get"})
		tickEnd()
	}
	ls := readLines(DATA + "/tick.tsv")
	if len(ls) != tickKeep {
		t.Fatalf("tick.tsv %d줄, 바람 %d", len(ls), tickKeep)
	}
	if f := strings.Split(ls[len(ls)-1], "\t"); len(f) != 4 || !strings.Contains(f[2], "seats=") || f[3] != "hc get=1" {
		t.Fatalf("끝 줄: %q", ls[len(ls)-1])
	}
	if s := tickStatus(); !strings.HasPrefix(s, "바퀴 계측: ") || !strings.Contains(s, "최근 50바퀴 중앙") {
		t.Fatalf("status 줄: %s", s)
	}
}
