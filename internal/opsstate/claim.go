package opsstate

// 조치함(2026-11-26 ht-advise-claim, docs/ops/advise.md) — 같은 알림을 지휘 둘이 각자 받아 따로 손댔다(손잡이를 서로 덮음).
// 손댈 것 항목의 대상에 최근 다른 지휘의 조치가 일지에 있으면 항목에 한 줄을 붙인다. 막지 않는다 — 보고 판단은 지휘

import (
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/journal"
)

// claimActs: 손댈 것에 손댄 조치로 보는 일지 동작(지휘·사람이 친 조치 명령). 운영 에이전트의 「하려던 것」·「거절」 등은 조치가 아니다
var claimActs = map[string]bool{"knob set": true, "knob unset": true, "edit": true, "rm": true, "requeue": true, "retry": true,
	"prio": true, "nudge": true, "merger restart": true, "human-ok": true, "withdraw": true}

// itemSubjects: 항목이 겨냥하는 대상 — 열쇠 이름(레인·프로젝트·기계·지휘), 굶는 감사는 그 레인의 기계도(자리 손잡이가 겨냥)
func itemSubjects(it Item, mach map[string]string) []string {
	_, n, ok := strings.Cut(it.Key, ":")
	if it.Kind == "unknown" || !ok || n == "" {
		return nil
	}
	o := []string{n}
	if m := mach[n]; it.Kind == "audit-starve" && m != "" {
		o = append(o, m)
	}
	return o
}

// actSubjects: 일지 줄이 겨냥한 대상 — 대상 칸 그대로, 손잡이 「이름@<기계|프로젝트>」는 그 기계·프로젝트도
func actSubjects(e journal.Entry) []string {
	o := []string{e.Target}
	if _, of, ok := strings.Cut(e.Target, "@"); ok && strings.HasPrefix(e.Action, "knob ") {
		o = append(o, of)
	}
	return o
}

// claimOK: 조치함으로 셀 줄인가 — 기간 안·조치 동작·남(me 아님, me가 「사람」이면 사람 줄도 셈)·바퀴 스스로 아님·실패·거절·안 함 아님
func claimOK(e journal.Entry, me string, since time.Time) bool {
	if e.Time.Before(since) || !claimActs[e.Action] || e.By == "hc-fill 스스로" || me != "사람" && e.By == me {
		return false
	}
	for _, p := range []string{"실패", "거절", "안 "} {
		if strings.HasPrefix(e.Result, p) {
			return false
		}
	}
	return true
}

// ActedOn: 일지 줄 e가 since 뒤 subs 가운데 하나를 겨냥한 조치인가 — Claims와 같은 재료(claimOK의 동작·결과 거름, actSubjects의 대상).
// 누가 했는지는 부르는 쪽이 본다(hc-lane watch의 놓침 막기 — ht-alert-route)
func ActedOn(e journal.Entry, subs []string, since time.Time) bool {
	if !claimOK(e, "사람", since) {
		return false
	}
	for _, a := range actSubjects(e) {
		if has(subs, a) {
			return true
		}
	}
	return false
}

// Claims: items마다 대상에 win 안의 다른 지휘 조치가 있으면 Claim을 채운다(가장 최근 하나, 여럿이면 「· 조치 n건」). es는 일지(시각 순), me는 부른 쪽
// (journal.Who). win 0이면 끔
func Claims(items []Item, s State, es []journal.Entry, me string, now time.Time, win time.Duration) {
	if win <= 0 {
		return
	}
	mach := map[string]string{}
	for _, l := range s.Lanes {
		mach[l.Name] = l.Machine
	}
	since := now.Add(-win)
	for i := range items {
		sub := itemSubjects(items[i], mach)
		if sub == nil {
			continue
		}
		var hit []journal.Entry
		for j := len(es) - 1; j >= 0; j-- {
			e := es[j]
			if !claimOK(e, me, since) {
				continue
			}
			for _, a := range actSubjects(e) {
				if has(sub, a) {
					hit = append(hit, e)
					break
				}
			}
		}
		if len(hit) == 0 {
			continue
		}
		e := hit[0]
		c := fmt.Sprintf("%s %d분 전 — %s %s", e.By, int(now.Sub(e.Time)/time.Minute), e.Action, e.Target)
		if w := []rune(e.Why); len(w) > 40 {
			c += "(" + string(w[:40]) + "…)"
		} else if len(w) > 0 {
			c += "(" + e.Why + ")"
		}
		if len(hit) > 1 { // 「외 n」이 아니라 — 사건 글의 항목 수 「외 n」과 겹쳤다(감사 atp2vpf 낮음 2)
			c += fmt.Sprintf(" · 조치 %d건", len(hit))
		}
		items[i].Claim = c
	}
}
