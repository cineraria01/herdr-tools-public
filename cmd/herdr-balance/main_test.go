package main

// 단위 시험: worker 고르기·상태 파일·이분 배치 순서·watch 판정. herdr-app을 부르는 길은 tests/util/herdrbalance_test.go(가짜 herdr-app)가 본다.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkersOf(t *testing.T) {
	tb := tab{ID: "x", Terminals: []term{{Terminal: "t0", Name: "lead"}, {Terminal: "t1", Name: "a"}, {Terminal: "t2"},
		{Terminal: "t3", Name: ""}, {Terminal: "t4", Name: "b"}}}
	if g := strings.Join(workersOf(tb, "t0"), ","); g != "t1,t4" {
		t.Fatal(g)
	}
}

func TestSaveSaved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, ok := saved("w1/tab"); ok {
		t.Fatal("없으면 ok=false")
	}
	if err := save("w1/tab", []string{"t1", "탭"}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(os.Getenv("HOME"), ".local/state/herdr-balance/w1__tab.json")
	b, err := os.ReadFile(p)
	// 셸 판 json.dump 꼴(ensure_ascii, 끝 줄바꿈 없음)
	if want := `["t1", "` + `\` + `ud0ed"]`; err != nil || string(b) != want {
		t.Fatalf("%q %v", b, err)
	}
	if ws, ok := saved("w1/tab"); !ok || strings.Join(ws, ",") != "t1,탭" {
		t.Fatal(ws, ok)
	}
	if err := save("t", nil); err != nil {
		t.Fatal(err)
	}
	if ws, ok := saved("t"); !ok || len(ws) != 0 {
		t.Fatal("빈 목록도 기록", ws, ok)
	}
	if err := os.WriteFile(p, []byte("깨짐"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved("w1/tab"); ok {
		t.Fatal("깨진 파일은 없는 것으로")
	}
}

// 골든: 셸 판(lib/herdr-balance/herdr-balance.py)을 가짜 herdr-app으로 worker 7개에 돌린 keep 순서(2026-11-21)
func TestBuildOrderLikePython(t *testing.T) {
	var got []string
	n := 0
	keep := func(tm string, anchor any, place string) (any, error) {
		n++
		got = append(got, fmt.Sprintf("%s@%v/%s", tm, anchor, place))
		return fmt.Sprintf("n%d", n), nil
	}
	ws := []string{"w1", "w2", "w3", "w4", "w5", "w6", "w7"}
	first, _ := keep(ws[0], "pm", "right")
	if err := build(ws, first, keep); err != nil {
		t.Fatal(err)
	}
	want := "w1@pm/right w4@n1/down w2@n1/down w3@n3/down w6@n2/down w5@n2/down w7@n5/down"
	if g := strings.Join(got, " "); g != want {
		t.Fatalf("\n%s\n%s", g, want)
	}
}

func TestSameSetAndPrefix(t *testing.T) {
	if !sameSet([]string{"a", "b"}, []string{"b", "a", "a"}) || sameSet([]string{"a"}, []string{"a", "b"}) || sameSet(nil, []string{"a"}) {
		t.Fatal("집합 비교")
	}
	if hasPrefix("w1/t", nil) || !hasPrefix("w1/t", []string{"w2/", "w1/"}) {
		t.Fatal("앞부분")
	}
}

func TestTruthy(t *testing.T) {
	for _, v := range []any{nil, "", false, []any{}, map[string]any{}} {
		if truthy(v) {
			t.Errorf("거짓이어야 %#v", v)
		}
	}
	for _, v := range []any{"x", true, []any{1}} {
		if !truthy(v) {
			t.Errorf("참이어야 %#v", v)
		}
	}
}
