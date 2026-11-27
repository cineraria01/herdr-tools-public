package opsstate

// 선행·겹침 앞 레인의 단계 글(ht-advise-stage-lookup, 2026-11-26) — Advise의 한 단계. 재료는 Facts.Pred(hc-fill이 대기열 전체와
// 기본 가지 착지에서 채움, 열쇠 「이름@프로젝트」). 사슬 머리가 첫 선행 단계만 붙이고, 추린 떠 있는 레인에서만 찾아 착지한 선행·대기 줄이
// 「단계 모름(떠 있는 레인에 없음)」, 나이 모름이 「모름째」로 보였다

import "strings"

// PredKey: Pred 열쇠 — 「이름@프로젝트」 꼴은 그대로, 아니면 기다리는 줄 프로젝트를 붙인다(같은 이름의 다른 프로젝트 줄과 섞지 않는다)
func PredKey(a, proj string) string {
	if strings.Contains(a, "@") {
		return a
	}
	return a + "@" + proj
}

// predStage: a(「이름」 또는 「이름@프로젝트」, proj = 기다리는 줄 프로젝트)의 단계 — 착지면 「착지 <sha8> <나이> 전」, 나이를 모르면 단계만,
// 기계를 알면 「(<기계>)」. 대기열·착지에 없으면 「대기열에 없음」. 재료 없음(Pred nil — hc-fill 밖에서 부름)은 상태 한 장의 레인에서
// 이름으로, 거기도 없으면 「단계 모름」
func predStage(s State, f Facts, a, proj string) string {
	if r := thenWait(f, a, proj); r != "" {
		return "앞 레인 " + r + "의 이어서 단계 " + afterName(a) + " 대기(" + stageText(f.Pred[PredKey(r, proj)]) + ")"
	}
	if f.Pred == nil {
		for _, l := range s.Lanes {
			if l.Name == afterName(a) {
				return stageText(l)
			}
		}
		return "단계 모름"
	}
	l, ok := f.Pred[PredKey(a, proj)]
	if !ok {
		return "대기열에 없음"
	}
	return stageText(l)
}

// thenWait: 선행 a가 앞 레인 지시문에 이어 붙인 단계(--then, Facts.Then)이고 받은 줄이 대기열에서 아직 안 끝났으면(Ended 아님) 그 받은 줄,
// 아니면 빈 값. 이어서 단계가 이미 착지했거나(Pred에 착지), 받은 줄이 끝났거나 대기열에 없으면 빈 값 — 그때는 보통 선행으로 판정한다
func thenWait(f Facts, a, proj string) string {
	r, ok := f.Then[afterName(a)]
	if !ok {
		return ""
	}
	if p, ok := f.Pred[PredKey(a, proj)]; ok && strings.HasPrefix(p.Stage, "착지") {
		return ""
	}
	if l, ok := f.Pred[PredKey(r, proj)]; !ok || Ended(l) {
		return ""
	}
	return r
}

func stageText(l Lane) string {
	st := l.Stage
	switch {
	case l.StageAgeS == nil:
	case strings.HasPrefix(l.Stage, "착지"):
		st += " " + age(l.StageAgeS) + " 전"
	default:
		st += " " + age(l.StageAgeS) + "째"
	}
	if l.Machine != "" {
		st += "(" + l.Machine + ")"
	}
	return st
}

// predStages: 선행마다 제 단계
func predStages(s State, f Facts, w Wait) string {
	var st []string
	for _, a := range w.After {
		st = append(st, a+" "+predStage(s, f, a, w.Project))
	}
	return strings.Join(st, ", ")
}
