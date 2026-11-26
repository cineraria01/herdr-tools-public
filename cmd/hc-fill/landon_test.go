package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/mergerstate"
)

// maRecFile: maFx의 가짜 hc-merger가 돌려줄 기록(클론 /c/app → DATA/state-app.json)
func maRecFile(t *testing.T, r mergerstate.Rec) {
	t.Helper()
	r.V = mergerstate.Version
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DATA+"/state-app.json", b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// 병합기 줄 끝에 마지막 「착지 실행」 결정(ht-merger-land-offload) — ht-merger-state-record부터 병합기가 기록에 적은 것(land_on)을 읽는다.
// 다른 기계 병합기도 같은 길(hc-merger --state를 hc exec --on으로)이라 보인다. 기록이 없는 병합기는 짐작하지 않고 모름
func TestMergerLineLandOn(t *testing.T) {
	maFx(t, maQ("w1", 20*time.Minute), "")
	now := time.Now().Unix()
	maRecFile(t, mergerstate.Rec{At: now, Progress: now, Phase: mergerstate.Idle, Since: now, LandOn: "w2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.90)"})
	confLines = []string{"repo app m " + DATA, "merger app m"} // 병합기가 이 기계
	TERMS = []Term{{M: "m", Addr: "m/t1", Agent: "zsh", Name: "merger-app", Status: "-", Cwd: "/c/app"}}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: w2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.90)") {
		t.Fatalf("이 기계 병합기: %q", l)
	}
	// 다른 기계 병합기 — 같은 명령으로 그 기계에서 읽는다(hc exec --on m2)
	confLines = []string{"repo app m " + DATA, "merger app m2"}
	TERMS = []Term{maMerger()}
	MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: w2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.90)") {
		t.Fatalf("다른 기계 병합기: %q", l)
	}
	if b, _ := os.ReadFile(DATA + "/hc.log"); !strings.Contains(string(b), "exec --on m2 -- sh") {
		t.Errorf("원격 읽기가 hc exec --on 길이 아님: %s", b)
	}
	// 기록 없음(옛 판 병합기) — 착지 실행을 일지에서 짐작하지 않고 모름
	os.Remove(DATA + "/state-app.json")
	MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || strings.Contains(l[0], "착지 실행") || !strings.HasSuffix(l[0], " · 기록 "+maOldMerger) {
		t.Fatalf("옛 판 병합기: %q", l)
	}
}
