package main

// 착지를 시작한 쪽(2026-11-24 fill-auto-submit, docs/hc-fill/audit.md 「대기열이 제출」): hc-fill이 감사 통과 판정을 레인 대신
// 제출할 때는 레인의 실행 ID(HC_LANE_JOB)로 같은 land --verdict 줄을 돌리고 HC_LANE_LAND_BY=fill을 준다. 시작 표시
// landing<TAB>시각<TAB>실행<TAB>patch-id 뒤 다섯째 칸에 fill이 붙는다. 표시는 먼저 만든 쪽만 이기고(ln), 실행 ID가 같아도
// 시작한 쪽이 다르면 잇지 않는다 — 에이전트와 대기열이 둘 다 제출하는 일이 없다

import (
	"fmt"
	"os"
	"strings"
)

// lvByFill: hc-fill이 레인 대신 돌리는 착지인가
func lvByFill() bool { return os.Getenv("HC_LANE_LAND_BY") == "fill" }

// lvByTag: 시작 표시 끝에 붙일 칸(대기열이면 「<TAB>fill」)
func lvByTag() string {
	if lvByFill() {
		return "\tfill"
	}
	return ""
}

// lvOwner <시작 표시 다섯째 칸> <대기열이 부름>: 0 이 쪽이 시작한 착지 — 잇는다, 1 대기열이 맡음 — 에이전트는 아무것도 안 한다,
// 2 에이전트가 시작함 — 대기열은 손대지 않는다
func lvOwner(fifth string, fill bool) int {
	switch {
	case (fifth == "fill") == fill:
		return 0
	case fifth == "fill":
		return 1
	}
	return 2
}

// ackFifth: 표시 첫 줄의 다섯째 칸(없거나 못 읽으면 빈 값)
func ackFifth(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	f := strings.Split(strings.SplitN(string(b), "\n", 2)[0], "\t")
	if len(f) < 5 {
		return ""
	}
	return f[4]
}

// lvOwnerGate: land --verdict가 「landing」 표시를 이을 때 — 시작한 쪽이 다르면 끝낸다
func lvOwnerGate() {
	switch lvOwner(ackFifth(lvx), lvByFill()) {
	case 1:
		fmt.Printf("hc-lane land: 판정 %s 는 대기열(hc-fill)이 레인 대신 제출하고 있다 — 아무것도 안 했다(착지 보고도 하지 않는다). 다음 지시를 기다린다\n", lvd)
		exit(0)
	case 2:
		eprintf("hc-lane land: 판정 %s 착지를 레인 에이전트가 시작했다 — 대기열은 손대지 않았다\n", lvd)
		exit(2)
	}
}
