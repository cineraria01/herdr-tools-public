package opsstate

// 병합 줄의 「안 움직임」·「느림」(advise.go에서 나눔 — 줄 수 예산, docs/ops/advise.md merge-stall·merge-slow)

import "fmt"

// mergeStall: 줄이 찬 병합 줄의 「안 움직임」·「느림」(2026-11-24 advise-merge-stall-age — 40분 비었던 줄에 4분 전 제출 1건이 「병합기
// 마지막 바퀴 40분 전」으로 울렸다: 일지는 일이 있을 때만 줄이 생긴다). 시계는 가장 오래된 제출의 나이, 일지 마지막 줄은 그 제출 뒤에
// 병합기가 한 일이 있나의 보조 — 조용한 시간 = min(가장 오래 나이, 일지 마지막 줄 나이). 일지를 못 읽었으면(다른 기계 병합기 — 마지막
// 바퀴·착지 수 모두 nil) 정하지 않는다. 이 기계 병합기가 지금 착지 중이면 안 움직임이 아니라 그 착지 나이를 SlowMin과 견준다(merge-slow)
func mergeStall(m MergeLine, th Thresholds) (Item, bool) {
	ev := fmt.Sprintf("줄 %d · 가장 오래 %s", *m.Length, age(m.OldestAgeS))
	if m.OldestLane != "" {
		ev += "(" + m.OldestLane + ")"
	}
	next := []string{"hc-lane logs", "hc-fill status"}
	if m.Landing != "" && m.LandingAgeS != nil {
		if th.SlowMin <= 0 || *m.LandingAgeS < th.SlowMin*60 {
			return Item{}, false
		}
		on := ""
		if m.LandOn != "" {
			on = "(" + m.LandOn + ")"
		}
		return Item{Key: "merge-slow:" + m.Project, Kind: "merge-slow",
			What:     fmt.Sprintf("병합 줄이 느림: %s — 맨 앞 %s 착지 검증 %d분째%s", m.Project, m.Landing, *m.LandingAgeS/60, on),
			Evidence: ev + " · 검증 맡김 " + ival(m.Delegated) + "번",
			Cost:     "착지 검증이 끝날 때까지 뒤 제출이 모두 기다린다", Next: next, Sure: SureJudge}, true
	}
	q := m.OldestAgeS
	if m.LastRoundAgeS != nil && (q == nil || *m.LastRoundAgeS < *q) {
		q = m.LastRoundAgeS
	}
	if th.WaitMin <= 0 || q == nil || m.LastRoundAgeS == nil && m.Lands == nil || *q < th.WaitMin*60 {
		return Item{}, false
	}
	// 맨 앞 추측 검증을 기다리는 중이고 그 검증이 살아서 진행 중이면 안 움직임이 아니다(ht-stall-spec-alive — 추측 기다림 바퀴는 일지에 줄을
	// 남기지 않아, 진행 중인 검증을 20분 기다리는 동안 merge-stall이 올라갔다). 터미널이 없거나 갱신이 멈췄으면 그대로 안 움직임
	switch {
	case m.SpecMachine != "" && m.SpecTerm != "" && m.SpecFresh:
		return Item{}, false
	case m.SpecMachine != "" && m.SpecTerm == "":
		ev += " · 추측 검증 기다림(" + m.SpecMachine + ") — 살아 있는 spec 터미널 없음"
	case m.SpecMachine != "" && m.SpecWhy != "":
		ev += " · 추측 검증 " + m.SpecTerm + ": " + m.SpecWhy
	case m.SpecMachine != "":
		ev += " · 추측 검증 " + m.SpecTerm + " 로그·작업 트리 갱신 없음(advise_spec_fresh분 넘게)"
	}
	// 워커 착지(ht-advise-land-remote): 살아서 갱신 중이면 hc-fill이 Landing을 채워 위에서 끝났다 — 여기 오면 멈췄거나 모름
	if m.LandOn != "" {
		ev += " · 워커 착지 멈춤 " + m.LandOn + " " + age(m.LandOnAgeS)
		switch {
		case m.LandOnTerm == "":
			ev += "(살아 있는 land 터미널 없음)"
		case m.LandOnWhy != "":
			ev += "(" + m.LandOnTerm + ": " + m.LandOnWhy + ")"
		default:
			ev += "(" + m.LandOnTerm + " 로그·작업 트리 갱신 없음, advise_spec_fresh분 넘게)"
		}
	}
	switch {
	case m.LastRoundAgeS == nil:
		ev += " · 병합기 일지에 이 프로젝트 줄 없음"
	case m.OldestAgeS != nil && *m.LastRoundAgeS > *m.OldestAgeS:
		ev += " · 그 뒤 병합기 일지 줄 없음(마지막 바퀴 " + age(m.LastRoundAgeS) + " 전)"
	default:
		ev += " · 병합기 마지막 바퀴 " + age(m.LastRoundAgeS) + " 전"
	}
	return Item{Key: "merge-stall:" + m.Project, Kind: "merge-stall", What: "병합 줄이 안 움직임: " + m.Project,
		Evidence: ev + " · 검증 맡김 " + ival(m.Delegated) + "번",
		Cost:     "맡긴 검증 기계가 바쁘거나 맨 앞이 막히면 줄 전체가 기다린다", Next: next, Sure: SureJudge}, true
}
