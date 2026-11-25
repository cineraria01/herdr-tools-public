//go:build !windows

package main

// ht-slow-machine-light(2026-11-26): 느린 기계(light_only@<기계> 1)는 가벼운 레인만, 무거운 레인은 빠른 기계로
// ① light_only 기계 + 보통 레인 → 거기 안 띄우고 다른 기계(그 기계뿐이면 기다림) ② 가벼운 레인 → 거기 먼저(부하가 더 높아도),
// 꽉 찼으면 다른 기계 ③ 파일 줄 문서뿐 → 가벼움 추정 ④ add·edit --light → 가벼움(18칸) ⑤ 손잡이 0 → 지금과 같음

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

func wtBrief(t *testing.T, body string) string {
	t.Helper()
	p := t.TempDir() + "/b.md"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ③ 추정: 「파일:」 줄의 낱말이 모두 문서일 때만 가벼움 — 코드 경로·도구 이름·설명 글이 섞이거나 파일 줄이 없으면 보통
func TestLaneWeightEstimate(t *testing.T) {
	for _, c := range []struct {
		body  string
		light bool
	}{
		{"# x\n파일: docs/hc-fill/a.md·README.md\n", true},
		{"- 파일: `CLAUDE.md`, `AGENTS.md`\n", true},
		{"**파일:** docs/\n", true},
		{"파일: docs/a.md\n파일: cmd/hc-fill/a.go\n", false},
		{"파일: docs/a.md (+ 새 시험 파일)\n", false},
		{"파일: hc-fill의 add·edit\n", false},
		{"# 문서만 고친다\n", false},
		{"파일:\n", false},
	} {
		r := pad16("l", "p", wtBrief(t, c.body))
		if got, why := laneWeight(r); got != c.light || why == "" {
			t.Errorf("%q → %v(%s), 원하는 값 %v", c.body, got, why, c.light)
		}
	}
	doc := wtBrief(t, "파일: docs/a.md\n")
	if l, why := laneWeight(pad16("l", "p", doc)); !l || why != "가벼움(파일 줄 문서뿐)" {
		t.Errorf("문서뿐 근거: %v %q", l, why)
	}
	code := wtBrief(t, "파일: cmd/a.go\n")
	if l, why := laneWeight(pad16("l", "p", code).Set(qWeight, "light")); !l || why != "가벼움(지정)" {
		t.Errorf("지정 가벼움이 추정보다 앞선다: %v %q", l, why)
	}
	if l, why := laneWeight(pad16("l", "p", doc).Set(qWeight, "normal")); l || why != "보통(지정)" {
		t.Errorf("지정 보통이 추정보다 앞선다: %v %q", l, why)
	}
}

func wtLight(t *testing.T, reround func(), m, v string) {
	t.Helper()
	if err := knobSet([]string{"light_only@" + m, v, "--why", "느린 기계"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reround()
}

// ① ② ⑤: a(이 기계)·b, b가 light_only
func TestLightOnlyPlace(t *testing.T) {
	reround := lpFix(t)
	t.Cleanup(func() { LIGHT, LIGHTWHY = false, "" })
	ld := map[string]mLoad{"a": {L1: 20, Cores: 12, OK: true}, "b": {L1: 1.2, Cores: 8, OK: true}} // b가 훨씬 한가함
	// ⑤ 손잡이 없음·0 → 지금과 같음(부하 낮은 b)
	if got := lpPlace(t, ld); got != "b" {
		t.Fatalf("손잡이 없음: 부하 낮은 b: %q", got)
	}
	wtLight(t, reround, "b", "0")
	if got := lpPlace(t, ld); got != "b" {
		t.Fatalf("light_only@b 0: 지금과 같음 b: %q", got)
	}
	wtLight(t, reround, "b", "1")
	// ① 보통 레인 → b가 한가해도 a
	LIGHT = false
	if got := lpPlace(t, ld); got != "a" || MWHY["b"] != "가벼운 일만" {
		t.Fatalf("보통 레인은 light_only b에 안 뜬다: %q 까닭 %q", got, MWHY["b"])
	}
	// 그 기계뿐(--only b) → 기다림
	LOADS, NOW, dry = ld, nil, true
	place("l2", "p", DATA+"/brief.md", "b", "-", "-", "")
	dry = false
	if len(NOW) != 0 {
		t.Fatalf("보통 레인 --only b: 띄우지 않아야: %q", NOW)
	}
	// ② 가벼운 레인 → b가 a보다 바빠도 b 먼저
	LIGHT, LIGHTWHY = true, "가벼움(지정)"
	busyB := map[string]mLoad{"a": {L1: 1, Cores: 12, OK: true}, "b": {L1: 16, Cores: 8, OK: true}}
	if got := lpPlace(t, busyB); got != "b" {
		t.Fatalf("가벼운 레인은 light_only b 먼저: %q", got)
	}
	// b 꽉 참 → 다른 기계
	SEATS = []string{"b", "b", "b"}
	if got := lpPlace(t, busyB); got != "a" {
		t.Fatalf("b 자리 참 → a: %q", got)
	}
	// 기계 줄 조정에 「가벼운 일만」
	if !strings.Contains(machAdj("b"), "가벼운 일만") {
		t.Fatalf("기계 줄 조정: %q", machAdj("b"))
	}
}

// advise: 보통 대기 줄의 띄울 곳이 light_only 기계뿐이면 「가벼운 기계만 남음」
func TestLightOnlyAdviseWait(t *testing.T) {
	reround := lpFix(t)
	wtLight(t, reround, "b", "1")
	code := wtBrief(t, "파일: cmd/a.go\n")
	w := adviseWait(pad16("l9", "p", code, "-", "b", "wait"), time.Now())
	if len(w.Can) != 0 || strings.Join(w.LightOnly, ",") != "b" {
		t.Fatalf("보통 줄 --only b: Can %v LightOnly %v", w.Can, w.LightOnly)
	}
	doc := wtBrief(t, "파일: docs/a.md\n")
	if w := adviseWait(pad16("l8", "p", doc, "-", "b", "wait"), time.Now()); strings.Join(w.Can, ",") != "b" || len(w.LightOnly) != 0 {
		t.Fatalf("가벼운 줄은 b에 뜰 수 있다: %v %v", w.Can, w.LightOnly)
	}
	seats, live := 3, 0
	st := opsstate.State{Machines: []opsstate.Machine{{Name: "b", Reach: "ok", Seats: &seats, Live: &live}}}
	it := opsstate.Advise(st, opsstate.Facts{Waits: []opsstate.Wait{w}}, opsstate.Thresholds{IdleMin: 1})
	found := false
	for _, x := range it {
		if x.Kind == "light-only" && strings.Contains(x.What, "가벼운 기계만 남음") && strings.Contains(strings.Join(x.Next, " "), "--light") {
			found = true
		}
	}
	if !found {
		t.Fatalf("advise 항목 light-only가 없다: %+v", it)
	}
}

// ④ add·edit --light·--weight: 18칸, 줄 ID 그대로(없던 옛 줄은 새로), --weight -는 추정으로
func TestWeightAddEdit(t *testing.T) {
	k := ehKit(t)
	out, se, rc := k.run(t, "edit", "lane-a", "--light", "--why", "문서만")
	if rc != 0 || !strings.Contains(out, "무게 가벼움") {
		t.Fatalf("edit --light: %d %q %q", rc, out, se)
	}
	r := ehRow(t, k, "lane-a")
	if r.F(qWeight) != "light" || r.F(qRID) == "" || r.F(qPrio) != "high" {
		t.Fatalf("edit --light 줄: %q", r.Line())
	}
	rid := r.F(qRID)
	if _, se, rc := k.run(t, "edit", "lane-a", "--weight", "normal"); rc != 0 || ehRow(t, k, "lane-a").F(qWeight) != "normal" || ehRow(t, k, "lane-a").F(qRID) != rid {
		t.Fatalf("edit --weight normal: %d %q %q", rc, se, ehRow(t, k, "lane-a").Line())
	}
	if _, _, rc := k.run(t, "edit", "lane-a", "--weight", "heavy"); rc != 2 {
		t.Fatalf("잘못된 무게는 종료 2: %d", rc)
	}
	b := k.home + "/brief.md"
	ehPut(t, b, "# x\n", 0o644)
	if out, se, rc := k.run(t, "add", "--name", "lane-l", "--project", "app", "--brief", b, "--light"); rc != 0 || !strings.Contains(out, "가벼움") {
		t.Fatalf("add --light: %d %q %q", rc, out, se)
	}
	if r := ehRow(t, k, "lane-l"); r.F(qWeight) != "light" || r.F(qRID) == "" || r.NF() != qNF {
		t.Fatalf("add --light 줄: %q", r.Line())
	}
	if _, _, rc := k.run(t, "add", "--name", "lane-n", "--project", "app", "--brief", b); rc != 0 || ehRow(t, k, "lane-n").NF() != qRID {
		t.Fatalf("무게 없이 넣은 줄은 지금 꼴(17칸) 그대로: %q", ehRow(t, k, "lane-n").Line())
	}
}
