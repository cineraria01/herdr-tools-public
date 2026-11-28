package main

import (
	"strings"
	"testing"
)

// ht-smalls-1129: 놀고 있음 알림의 「겹침 대기」는 이 바퀴 목록(ROWS) 하나로 — 바퀴 도중 넣은 줄은 다음 바퀴가 본다. 겹침 대기 줄 가운데
// 선행을 기다리는 주차 줄(rowMode = 주차 — 표 stStage0 「대기(주차 — 선행 기다림)」와 같은 정의)은 「선행 기다림」으로 센다
func TestIdleWorthRoundRows(t *testing.T) {
	setData(t)
	confLines = []string{"machine here 2 -", "repo p here " + DATA + "/r"}
	row := func(name, state, mode string) Row {
		return Row{name, "p", "b.md", "-", "-", state, "-", "-", "-", "t", "-", "-", "normal", mode, "-", "-", "q-" + name}
	}
	file := func(rs ...Row) {
		var o []string
		for _, r := range rs {
			o = append(o, strings.Join(r, "\t"))
		}
		rejW(t, Q, strings.Join(o, "\n")+"\n")
	}
	ow := func(n string) Row { return Row{n, "front", "bin/tool", "-", "p"} }
	pk, ov, late, gone := row("pk", "wait", "parked"), row("ov", "wait", "-"), row("late", "wait", "-"), row("gone", "wait", "-")
	WAITOK = map[string]bool{}

	// 바퀴 처음: 주차 줄 pk(겹침 대기)·이번 바퀴에 뜬 gone. 바퀴 도중 late가 들어옴 → 알림 없음
	ROWS, OVWAIT = []Row{pk, gone}, []Row{ow("pk")}
	file(pk, row("gone", "up", "-"), late)
	if idleWorth("here") {
		t.Fatal("바퀴 도중 넣은 줄·이번 바퀴에 뜬 줄로 「뜰 수 있는데 안 뜸」이 되면 안 된다")
	}
	if o, p := idleWaits("here"); o != 0 || p != 1 || idleNote(o, p) != "(선행 기다림 1줄)" {
		t.Fatalf("주차 줄은 선행 기다림: 겹침 %d · 선행 %d · %q", o, p, idleNote(o, p))
	}
	if idleWhy(0, 1) != "선행 기다림 — 선행이 착지하면 뜬다(hc-fill ls)" {
		t.Fatalf("까닭: %q", idleWhy(0, 1))
	}

	// 진짜 겹침 대기 ov + 뜰 수 있는데 안 뜬 late(바퀴 처음부터 있음) → 지금 그대로 「겹침 대기 n줄」
	ROWS, OVWAIT = []Row{ov, late}, []Row{ow("ov")}
	file(ov, late)
	if !idleWorth("here") {
		t.Fatal("바퀴 처음부터 있던 뜰 수 있는 줄은 알린다")
	}
	if o, p := idleWaits("here"); o != 1 || p != 0 || idleNote(o, p) != "(겹침 대기 1줄)" || idleWhy(o, p) != "겹침 대기 — 앞 레인이 착지하면 뜬다(hc-fill ls)" {
		t.Fatalf("겹침 대기 그대로: %d %d %q %q", o, p, idleNote(o, p), idleWhy(o, p))
	}
	if n := idleNote(2, 1); n != "(겹침 대기 2줄 · 선행 기다림 1줄)" {
		t.Fatalf("섞임: %q", n)
	}
}
