package main

import (
	"strings"
	"testing"
)

// 시험용: 부모 트리 + 무리 → 후보 트리는 「부모/열쇠」 글자로 흉내 낸다
func row(parent, key, machine string, at int64) specRow {
	return specRow{id: specID(parent, key), tree: parent + "/" + key, parent: parent, group: key, machine: machine, at: at}
}

func groups(keys ...string) []specGroup {
	g := make([]specGroup, len(keys))
	for i, k := range keys {
		g[i] = specGroup{key: k, shas: []string{k}, lanes: k}
	}
	return g
}

func trees(st []specStep) string {
	var s []string
	for _, x := range st {
		t := "nil"
		if x.row != nil {
			t = x.row.state.String()
		}
		s = append(s, x.g.key+"@"+x.parent+":"+t)
	}
	return strings.Join(s, " ")
}

func TestSpecMsgRoundTrip(t *testing.T) {
	m := specMsg("t0", "a+b", "box-1", 42, "run")
	r, ok := parseSpec(m)
	if !ok || r.parent != "t0" || r.group != "a+b" || r.machine != "box-1" || r.at != 42 || r.conflict || r.dispatchX {
		t.Fatalf("parseSpec = %+v %v", r, ok)
	}
	if r, ok := parseSpec(specMsg("t0", "a", "-", 1, "conflict")); !ok || !r.conflict {
		t.Fatalf("conflict = %+v %v", r, ok)
	}
	for _, bad := range []string{"", "hc-merger-spec parent=t", strings.Replace(m, "state=run", "state=x", 1), strings.Replace(m, "at=42", "at=z", 1)} {
		if _, ok := parseSpec(bad); ok {
			t.Errorf("parseSpec(%q) ok", bad)
		}
	}
}

// A·B·C 모두 도는 중 → 길은 A·B·C 차례, 맨 앞은 기다림. 셋 다 통과면 맨 앞 빠른 착지
func TestSpecWalkChain(t *testing.T) {
	rs := []specRow{row("T", "a", "m1", 0), row("T/a", "b", "m2", 0), row("T/a/b", "c", "m3", 0)}
	specResolve(rs, nil, nil, 10, 3600)
	st := specWalk("T", groups("a", "b", "c"), rs)
	if got := trees(st); got != "a@T:도는 중 b@T/a:도는 중 c@T/a/b:도는 중" {
		t.Fatalf("길 = %s", got)
	}
	if specFront(st) != specWaitFront {
		t.Fatal("맨 앞이 도는 중인데 기다리지 않음")
	}
	if b := specBusy(rs); !b["m1"] || !b["m2"] || !b["m3"] {
		t.Fatalf("바쁜 기계 = %v", b)
	}
	specResolve(rs, map[string]bool{"T/a": true, "T/a/b": true, "T/a/b/c": true}, nil, 10, 3600)
	st = specWalk("T", groups("a", "b", "c"), rs)
	if specFront(st) != specLandFast {
		t.Fatal("맨 앞 통과인데 빠른 착지 아님")
	}
	// A가 착지(기준 = T/a) → 길은 b·c, A 줄은 지울 것
	st = specWalk("T/a", groups("b", "c"), rs)
	if got := trees(st); got != "b@T/a:통과 c@T/a/b:통과" {
		t.Fatalf("A 착지 뒤 길 = %s", got)
	}
	if s := specStale(st, rs); len(s) != 1 || s[0].group != "a" {
		t.Fatalf("지울 것 = %+v", s)
	}
}

// B 실패 → B는 건너뛰고 C는 T/a 위에서 다시(줄 없음 → 지을 자리). 옛 C(T/a/b 위)는 도는 중이 아니면 지울 것
func TestSpecWalkFailSkips(t *testing.T) {
	rs := []specRow{row("T", "a", "m1", 0), row("T/a", "b", "m2", 0), row("T/a/b", "c", "m3", 0)}
	specResolve(rs, map[string]bool{"T/a": true, "T/a/b/c": true}, map[string]bool{specID("T/a", "b"): true}, 10, 3600)
	st := specWalk("T", groups("a", "b", "c"), rs)
	if got := trees(st); got != "a@T:통과 b@T/a:실패 c@T/a:nil" {
		t.Fatalf("길 = %s", got)
	}
	last := st[len(st)-1]
	if last.row != nil || last.parent != "T/a" || last.g.key != "c" {
		t.Fatalf("지을 자리 = %+v", last)
	}
	if s := specStale(st, rs); len(s) != 1 || s[0].group != "c" || s[0].parent != "T/a/b" {
		t.Fatalf("지울 것 = %+v", s)
	}
	// 맨 앞이 실패면 지금처럼 착지(병합기 기계에서 검증 → 지금 규칙대로 반송)
	if specFront(specWalk("T/a", groups("b", "c"), rs)) != specLand {
		t.Fatal("맨 앞 실패인데 착지 아님")
	}
}

// 시간 넘김·띄우기 실패 = 모름: 길은 그 트리로 이어 가고(더 깊은 추측은 쓸모 있음), 맨 앞이면 지금처럼 착지, 기계는 놓인다
func TestSpecLost(t *testing.T) {
	rs := []specRow{row("T", "a", "m1", 0), row("T/a", "b", "m2", 0)}
	rs[1].dispatchX = true
	specResolve(rs, nil, nil, 5000, 3600)
	st := specWalk("T", groups("a", "b", "c"), rs)
	if got := trees(st); got != "a@T:모름 b@T/a:모름 c@T/a/b:nil" {
		t.Fatalf("길 = %s", got)
	}
	if specFront(st) != specLand {
		t.Fatal("맨 앞 모름인데 지금처럼 착지 아님")
	}
	if b := specBusy(rs); len(b) != 0 {
		t.Fatalf("모름인 기계가 바쁨: %v", b)
	}
}

// main이 움직임(기준 트리가 다름) → 옛 추측은 하나도 길에 없다: 도는 중이 아니면 모두 지울 것, 맨 앞은 줄 없음 → 지을 자리
func TestSpecMainMoved(t *testing.T) {
	rs := []specRow{row("T", "a", "m1", 0), row("T/a", "b", "m2", 0)}
	specResolve(rs, map[string]bool{"T/a": true}, nil, 10, 3600)
	st := specWalk("X", groups("a", "b"), rs)
	if got := trees(st); got != "a@X:nil" {
		t.Fatalf("길 = %s", got)
	}
	if specFront(st) != specLand {
		t.Fatal("줄 없음인데 지금처럼 착지 아님")
	}
	if s := specStale(st, rs); len(s) != 1 || s[0].group != "a" {
		t.Fatalf("지울 것 = %+v(도는 중인 b는 남겨야 함)", s)
	}
	if b := specBusy(rs); !b["m2"] || b["m1"] {
		t.Fatalf("바쁜 기계 = %v", b)
	}
}

// 충돌 줄은 건너뛰고 다시 짓지 않는다
func TestSpecConflict(t *testing.T) {
	r := row("T", "a", "-", 0)
	r.conflict, r.tree = true, "T"
	rs := []specRow{r}
	specResolve(rs, nil, nil, 10, 3600)
	st := specWalk("T", groups("a", "b"), rs)
	if got := trees(st); got != "a@T:충돌 b@T:nil" {
		t.Fatalf("길 = %s", got)
	}
}
