package main

// ht-stall-spec-alive(2026-11-25): 병합 줄 칸의 재료 — 병합기가 맨 앞 추측 검증을 기다리는 중이면 그 검증 기계의 터미널과 그 검증이
// 진행 중인가. ht-merger-state-record(2026-11-26): 재료가 specwait 풀기 + spec-<아이디8> 이름으로 터미널 찾기 + hc-fill이 직접 잰 로그
// 갱신에서 병합기 기록(단계 spec-wait·기다리는 대상 주소·마지막 진행)으로 바뀌었다. 갱신을 재는 일(링크 폴더·원격 기계·못 읽음)은
// 병합기 쪽 시험(hc-merger specoverrun_test.go·staterec_test.go)이 본다. 장면과 기대는 그대로.

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/mergerstate"
	"herdr-tools/internal/opsstate"
)

func TestStateSpecWait(t *testing.T) {
	old := LOCAL
	defer func() { LOCAL = old }()
	DATA, LOCAL = t.TempDir(), "here"
	now := time.Now()
	wait := func(addr string, prog time.Duration) mergerstate.Rec {
		m := "here"
		if a, _, ok := strings.Cut(addr, "/"); ok {
			m = a
		}
		return mergerstate.Rec{At: now.Unix(), Progress: now.Add(-prog).Unix(), Machine: "here", Phase: mergerstate.SpecWait,
			Since: now.Add(-20 * time.Minute).Unix(), Members: []string{"a"},
			Waits: []mergerstate.Wait{{Machine: m, Addr: addr, Role: "추측", At: now.Add(-20 * time.Minute).Unix()}}}
	}
	term := func(addr string) []hcjson.Terminal {
		return []hcjson.Terminal{{Address: "t1", State: "live"}, {Address: addr, State: "live"}}
	}
	look := func(x maRow, terms []hcjson.Terminal, ok bool) opsstate.MergeLine {
		var g opsstate.MergeLine
		stateRec(&g, x, terms, ok, now)
		return g
	}
	// 기다림 아님(병합기가 쉼) — 기다리는 대상 없음
	idle := srRow(t, mergerstate.Rec{At: now.Unix(), Progress: now.Unix(), Phase: mergerstate.Idle, Since: now.Unix()})
	if g := look(idle, term("t2"), true); len(g.Waits) != 0 || g.Phase != mergerstate.Idle {
		t.Fatalf("기다림 아님: %+v", g)
	}
	// ② 살아 있으나 오래 멈춤(로컬 터미널) — 터미널은 살아 있고 마지막 진행 오래 전
	if g := look(srRow(t, wait("t2", time.Hour)), term("t2"), true); len(g.Waits) != 1 || g.Waits[0].Addr != "t2" || g.Waits[0].Live != "live" ||
		g.LastRoundAgeS == nil || *g.LastRoundAgeS != 3600 {
		t.Errorf("② 멈춤: %+v", g)
	}
	// ① 살아 있고 진행 중 — 마지막 진행 방금
	if g := look(srRow(t, wait("t2", 0)), term("t2"), true); g.Waits[0].Live != "live" || *g.LastRoundAgeS != 0 {
		t.Errorf("① 진행 중: %+v", g)
	}
	// ③ 터미널 없음(끝났거나 죽음 — 끝난 터미널·다른 기계의 같은 번호는 안 셈)
	gone := []hcjson.Terminal{{Address: "t2", State: "finished"}, {Address: "box-2/t2", State: "live"}}
	if g := look(srRow(t, wait("t2", 0)), gone, true); g.Waits[0].Live != "gone" {
		t.Errorf("③ 터미널 없음: %+v", g)
	}
	// ④ 다른 기계: 병합기가 그 기계 주소를 적으면 이 기계 hc ls의 같은 주소로. 이 기계를 가리키는 주소는 기계를 뗀다
	if g := look(srRow(t, wait("box-2/t2h2", 0)), term("box-2/t2h2"), true); g.Waits[0].Addr != "box-2/t2h2" || g.Waits[0].Live != "live" || g.Waits[0].Machine != "box-2" {
		t.Errorf("④ 원격: %+v", g)
	}
	if g := look(srRow(t, wait("here/t2", 0)), term("t2"), true); g.Waits[0].Addr != "t2" || g.Waits[0].Live != "live" {
		t.Errorf("④ 이 기계 주소: %+v", g)
	}
	// hc ls를 못 읽음 — 모름(없음과 가른다)
	if g := look(srRow(t, wait("t2", 0)), nil, false); g.Waits[0].Live != "?" {
		t.Errorf("hc ls 못 읽음: %+v", g)
	}
}
