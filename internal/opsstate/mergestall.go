package opsstate

// 병합 줄의 「안 움직임」·「느림」(advise.go에서 나눔 — 줄 수 예산, docs/ops/advise.md merge-stall·merge-slow)

import "fmt"

// mergeStall: 줄이 찬 병합 줄의 「안 움직임」·「느림」. 재료는 병합기가 쓴 상태 한 장(hc-fill이 MergeLine에 옮김 — docs/hc-lane/merger-state.md)
// 뿐이다(ht-merger-state-record — 밖에서 이름·일지로 짐작하던 것을 다섯 번 고쳤다). 시계는 가장 오래된 제출의 나이, 병합기의 마지막 진행
// 시각은 그 제출 뒤에 병합기가 나아간 것이 있나의 보조 — 조용한 시간 = min(가장 오래 나이, 마지막 진행 나이)(2026-11-24 advise-merge-stall-age
// — 40분 비었던 줄에 4분 전 제출 1건이 울렸다). 기록을 모르면(옛 판 병합기 등 — Phase 빈 값) 정하지 않는다. 착지 단계이고 진행이 문턱
// 안이면 안 움직임이 아니라 그 착지 나이를 SlowMin과 견준다(merge-slow — 안쪽 단계가 있으면 그 단계로)
func mergeStall(m MergeLine, th Thresholds) (Item, bool) {
	ev := fmt.Sprintf("줄 %d · 가장 오래 %s", *m.Length, age(m.OldestAgeS))
	if m.OldestLane != "" {
		ev += "(" + m.OldestLane + ")"
	}
	next := []string{"hc-lane logs", "hc-fill status"}
	stale := m.LastRoundAgeS != nil && th.WaitMin > 0 && *m.LastRoundAgeS >= th.WaitMin*60
	if m.Landing != "" && m.LandingAgeS != nil && !stale {
		if th.SlowMin <= 0 || *m.LandingAgeS < th.SlowMin*60 {
			return Item{}, false
		}
		on := ""
		if m.LandOn != "" {
			on = "(" + m.LandOn + ")"
		}
		what := fmt.Sprintf("병합 줄이 느림: %s — 맨 앞 %s 착지 검증 %d분째%s", m.Project, m.Landing, *m.LandingAgeS/60, on)
		cost := "착지 검증이 끝날 때까지 뒤 제출이 모두 기다린다"
		if m.Inner != "" { // 착지 안쪽 단계(착지 스크립트가 알린 것 — 예: 공개 선점 기다림)
			what = fmt.Sprintf("병합 줄이 느림: %s — 맨 앞 %s %s %s째(착지 %d분째%s)", m.Project, m.Landing, m.Inner, age(m.InnerAgeS), *m.LandingAgeS/60, on)
			cost = "착지의 그 단계가 끝날 때까지 뒤 제출이 모두 기다린다"
		}
		return Item{Key: "merge-slow:" + m.Project, Kind: "merge-slow", What: what,
			Evidence: ev + " · 검증 맡김 " + ival(m.Delegated) + "번", Cost: cost, Next: next, Sure: SureJudge}, true
	}
	q := m.OldestAgeS
	if m.LastRoundAgeS != nil && (q == nil || *m.LastRoundAgeS < *q) {
		q = m.LastRoundAgeS
	}
	if th.WaitMin <= 0 || q == nil || m.Phase == "" || *q < th.WaitMin*60 {
		return Item{}, false
	}
	ev += " · 단계 " + or(m.PhaseWhy, m.Phase) + " " + age(m.PhaseAgeS) + "째"
	if m.Inner != "" {
		ev += " · 안쪽 단계 " + m.Inner + " " + age(m.InnerAgeS) + "째"
	}
	if m.Landing != "" { // 착지 단계인데 진행이 멎음
		if m.LandOn != "" {
			ev += " · 워커 착지 멈춤 " + m.LandOn + " " + age(m.LandingAgeS)
		} else {
			ev += " · 착지 멈춤 " + m.Landing + " " + age(m.LandingAgeS)
		}
	}
	for _, w := range m.Waits { // 기다리는 대상 — 병합기가 적은 주소를 이번 hc ls에서
		who := " · 기다리는 " + w.Role + " " + or(w.Addr, w.Machine)
		switch {
		case w.Addr == "":
			ev += who + " 터미널 모름(병합기가 못 찾음)"
		case w.Live == "gone":
			ev += who + " 터미널 없음"
		case w.Live == "live":
			ev += who + " 진행 없음"
		default:
			ev += who + " 살아 있는지 모름(hc ls 못 읽음)"
		}
	}
	switch {
	case m.LastRoundAgeS == nil:
		ev += " · 병합기 기록에 진행 시각 없음"
	case m.OldestAgeS != nil && *m.LastRoundAgeS > *m.OldestAgeS:
		ev += " · 그 뒤 병합기 진행 없음(마지막 진행 " + age(m.LastRoundAgeS) + " 전)"
	default:
		ev += " · 병합기 마지막 진행 " + age(m.LastRoundAgeS) + " 전"
	}
	if m.RecAgeS != nil && *m.RecAgeS >= 120 {
		ev += " · 병합기 기록 " + age(m.RecAgeS) + " 전"
	}
	return Item{Key: "merge-stall:" + m.Project, Kind: "merge-stall", What: "병합 줄이 안 움직임: " + m.Project,
		Evidence: ev + " · 검증 맡김 " + ival(m.Delegated) + "번",
		Cost:     "맡긴 검증 기계가 바쁘거나 맨 앞이 막히면 줄 전체가 기다린다", Next: next, Sure: SureJudge}, true
}
