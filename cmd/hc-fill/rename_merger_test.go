package main

// machine-rename 감사 지적(atozsn0-m2qozu 중간): 병합기 사건의 기계 칸은 띄울 때 받은 HC_LANE_MACHINE이라, 이름을 바꾼 순간 병합 중이던
// 병합기의 결과(착지·막힘)는 옛 이름으로 남는다. 새 이름 병합기만 병합기로 치면 받은편지 회신(mgres)·반송 다시 맞춤·쓰기 소유가 그 결과를
// 못 본다. machine-alias.tsv(하루)의 옛 이름도 병합기로 친다(mgSet). 고치기 전(옛 이름 기록 없음) 결과는 빈 값

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMergerOldNameResult(t *testing.T) {
	setData(t)
	MGM = " box-new"
	ev := strings.Join([]string{"2026-11-24T10:00:00Z", "box-old", "box-old/t7", "lane-l", "착지", "abc", "착지 abc 제출 ID s1"}, "\t")
	EV = []string{ev}
	if got := mgres("lane-l", "s1"); got != "" {
		t.Fatalf("옛 이름 기록 없음: 옛 이름 사건은 병합기 것으로 안 침(대조) — %q", got)
	}
	put := func(s string) {
		if err := os.WriteFile(DATA+"/machine-alias.tsv", []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := nowUnix()
	put(fmt.Sprintf("box-old\tbox-new\t%d\n", now))
	if got := mgres("lane-l", "s1"); got != ev {
		t.Fatalf("하루 안에 바꾼 옛 이름의 결과 사건은 새 이름 병합기 것으로: %q", got)
	}
	put(fmt.Sprintf("box-old\tbox-new\t%d\n", now-rnAliasKeep-1))
	if got := mgres("lane-l", "s1"); got != "" {
		t.Fatalf("기한 지난 옛 이름은 안 침: %q", got)
	}
	// 이어 바꿈 a → box-old → box-new: 새것부터 거슬러 둘 다
	put(fmt.Sprintf("box-a\tbox-old\t%d\nbox-old\tbox-new\t%d\n", now, now))
	if mg := mgSet(); !mg["box-a"] || !mg["box-old"] || !mg["box-new"] || len(mg) != 3 {
		t.Fatalf("이어 바꾼 옛 이름: %v", mg)
	}
	// 병합기가 아닌 기계의 옛 이름은 안 침
	MGM = " box-x"
	if mg := mgSet(); len(mg) != 1 || !mg["box-x"] {
		t.Fatalf("다른 병합기 기계에 옛 이름이 붙었다: %v", mg)
	}
}
