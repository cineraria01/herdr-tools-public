package main

// ht-advise-land-remote(2026-11-26): 병합기가 맨 앞 착지를 워커에서 돌리는 동안 — 그 워커·나이·터미널, 진행 중이면 병합기가 착지 중
// (Landing — advise가 안 움직임으로 세지 않는다). ht-merger-state-record: 재료가 일지 마지막 「착지 실행」 줄 + land-<차례 8> 터미널
// 이름 + 워커 로그 갱신에서 병합기 기록(단계 land-on·기다리는 대상 주소·마지막 진행)으로 바뀌었다 — 장면과 기대는 그대로.

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/mergerstate"
	"herdr-tools/internal/opsstate"
)

// srRow: 기록이 든 병합기 살아 있음 줄(이 기계 병합기)
func srRow(t *testing.T, r mergerstate.Rec) maRow {
	t.Helper()
	srMA(t, &r, "")
	return maLoad()["app"]
}

func TestStateLandOn(t *testing.T) {
	old := LOCAL
	defer func() { LOCAL = old }()
	DATA, LOCAL = t.TempDir(), "here"
	now := time.Now()
	r := mergerstate.Rec{At: now.Unix(), Progress: now.Unix() - 30, Machine: "here", Phase: mergerstate.LandOn,
		Since: now.Add(-18 * time.Minute).Unix(), Why: "워커 착지 실행 — ln-a(box-2)", Members: []string{"ln-a"},
		Waits: []mergerstate.Wait{{Machine: "box-2", Addr: "box-2/t5", Role: "착지 실행", At: now.Add(-18 * time.Minute).Unix()}}}
	terms := []hcjson.Terminal{{Address: "t1", State: "live"}, {Address: "box-2/t5", State: "live"}}
	look := func(x maRow, terms []hcjson.Terminal) opsstate.MergeLine {
		var g opsstate.MergeLine
		stateRec(&g, x, terms, true, now)
		return g
	}
	// 워커 착지 중 — 기계·터미널·나이, 맨 앞 레인이 착지 중
	g := look(srRow(t, r), terms)
	if g.LandOn != "box-2" || len(g.Waits) != 1 || g.Waits[0].Addr != "box-2/t5" || g.Waits[0].Live != "live" ||
		g.Landing != "ln-a" || g.LandingAgeS == nil || *g.LandingAgeS != 18*60 || g.LastRoundAgeS == nil || *g.LastRoundAgeS != 30 {
		t.Fatalf("워커 착지 중: %+v", g)
	}
	// 터미널 없음(끝났거나 죽음 — 끝난 터미널은 안 셈)
	gone := []hcjson.Terminal{{Address: "box-2/t5", State: "finished"}}
	if g := look(srRow(t, r), gone); g.LandOn != "box-2" || g.Waits[0].Live != "gone" {
		t.Errorf("터미널 없음: %+v", g)
	}
	// 워커 착지 아님 — 빈 값
	r.Phase = mergerstate.Round
	if g := look(srRow(t, r), terms); g.LandOn != "" || g.Landing != "" {
		t.Errorf("빈 값이어야: %+v", g)
	}
}

// status 병합기 줄: 병합기가 워커 착지 실행 중이면 「착지 실행: <기계> <나이>째(<까닭>)」(나이는 같은 줄의 다른 칸처럼 stDur), 끝나면
// 마지막 결정 그대로
func TestMergerLineLandOnRunning(t *testing.T) {
	maFx(t, maQ("w1", 20*time.Minute), "")
	now := time.Now()
	why := "box-2(검증 자리 남음 · 코어당 부하 0.43)"
	r := mergerstate.Rec{At: now.Unix(), Progress: now.Unix(), Phase: mergerstate.LandOn, Since: now.Add(-18 * time.Minute).Unix(),
		Members: []string{"b"}, LandOn: why, Waits: []mergerstate.Wait{{Machine: "box-2", Addr: "box-2/t5", Role: "착지 실행"}}}
	maRecFile(t, r)
	confLines = []string{"repo app m " + DATA, "merger app m"}
	TERMS = []Term{{M: "m", Addr: "m/t1", Agent: "zsh", Name: "merger-app", Status: "-", Cwd: "/c/app"}}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: box-2 18m째(검증 자리 남음 · 코어당 부하 0.43)") {
		t.Fatalf("워커 착지 중: %q", l)
	}
	r.Phase, r.Waits = mergerstate.Idle, nil
	maRecFile(t, r)
	MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: "+why) {
		t.Fatalf("착지 끝남: %q", l)
	}
}
