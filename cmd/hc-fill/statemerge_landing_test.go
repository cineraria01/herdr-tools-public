package main

// advise-merge-stall-age(2026-11-24): 병합 줄 칸의 재료 — 가장 오래된 제출의 레인(병합기 살아 있음 기록 11칸). 고치기 전 코드에는 maQueue가
// 레인을 돌려주지 않아 컴파일부터 실패한다. 이 기계 병합기가 착지 중인지(옛 재료: 클론 child 기록 + 착지 로그 이름 — TestStateLanding)는
// ht-merger-state-record에서 병합기 기록의 단계(land)로 옮겼다 — staterec_test.go ⑤·hc-merger staterec_test.go

import (
	"strings"
	"testing"
)

func TestMaQueueOldestLane(t *testing.T) {
	old := maSnap
	defer func() { maSnap = old }()
	a := "abc" + strings.Repeat("0", 37) // 가짜 SHA는 실행 중에(추적 파일 비밀 꼴 검사)
	maSnap = mqSnap{p: ",app", s: []Row{{"app", "refs/merge-queue/1795500200-ln-b", a}, {"app", "refs/merge-queue/1795500100-ln-a", a},
		{"app", "refs/herdr/merger/box", a}}}
	lanes, oldest, ln, ok := maQueue("app")
	if !ok || len(lanes) != 2 || oldest != 1795500100 || ln != "ln-a" {
		t.Fatalf("maQueue: %v %d %q %v", lanes, oldest, ln, ok)
	}
	// 기록 한 줄에 11칸으로 남고 다시 읽힌다(옛 10칸 기록은 빈 값)
	x := maRow{P: "app", M: "box", St: "alive", Addr: "t7", K: "2", Oldest: oldest, OldLane: ln}
	r := Row(lsSplit(x.line(), maRecWhy)) // 14칸(ht-merger-state-record — 기록 칸)
	if r.F(maOldLane) != "ln-a" || dash0(Row(lsSplit("app\tbox\talive\t1\tt7\t1\t5\t6\t-\t-", maOldLane)).F(maOldLane)) != "" {
		t.Errorf("11칸: %q", x.line())
	}
	// 기록 칸: 기록·까닭이 한 줄에 남고 다시 읽힌다(옛 12칸 기록은 빈 값)
	x.Rec, x.RecWhy = `{"v":1,"phase":"idle"}`, ""
	if r := Row(lsSplit(x.line(), maRecWhy)); r.F(maRec) != x.Rec || r.F(maRecWhy) != "-" {
		t.Errorf("13칸: %q", x.line())
	}
}
