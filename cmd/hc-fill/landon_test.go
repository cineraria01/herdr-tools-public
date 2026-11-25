package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// 병합기가 이 기계면 status·ls 병합기 줄 끝에 이 기계 병합기 일지의 마지막 「착지 실행」(ht-merger-land-offload). 다른 기계 병합기는 없음
func TestMergerLineLandOn(t *testing.T) {
	maFx(t, maQ("w1", 20*time.Minute), "")
	st := t.TempDir()
	t.Setenv("XDG_STATE_HOME", st)
	os.MkdirAll(st+"/hc-lane", 0o777)
	row := func(lane, what string) string {
		return strings.Join([]string{"t", "1-" + lane, lane, "-", "-", "-", "1", "t0", "-", "착지 실행", "-", what + " — 구성원 " + lane}, "\t") + "\n"
	}
	os.WriteFile(st+"/hc-lane/merger-journal.tsv", []byte(row("a", "w1(검증 자리 남음 · 코어당 부하 0.10)")+row("b", "w2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.90)")), 0o666)
	confLines = []string{"repo app m " + DATA, "merger app m"} // 병합기가 이 기계
	TERMS = []Term{{M: "m", Addr: "m/t1", Agent: "zsh", Name: "merger-app", Status: "-", Cwd: "/c/app"}}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || !strings.HasSuffix(l[0], " · 착지 실행: w2(모두 검증 중 — 가장 덜 바쁨 · 코어당 부하 0.90)") {
		t.Fatalf("이 기계 병합기: %q", l)
	}
	confLines = []string{"repo app m " + DATA, "merger app m2"}
	TERMS = []Term{maMerger()}
	MQP, MQS, MQBAD, maSnap = "", nil, "", mqSnap{}
	mergerTick()
	mergerAliveTick()
	if l := maLines(); len(l) != 1 || strings.Contains(l[0], "착지 실행") {
		t.Fatalf("다른 기계 병합기는 안 보임: %q", l)
	}
}
