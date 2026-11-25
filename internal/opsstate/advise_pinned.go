package opsstate

import (
	"fmt"
	"strings"
)

// pinnedItems: --only 기계가 모두 상한·멈춤이라 advise_pinned분 못 뜬 대기 줄, 제한 밖(저장소 있음)에 빈 자리가 있으면 손댈 것
// (ht-advise-only-stuck, 2026-11-25 — 묶인 기계 레인상한으로 「선행 기다림」에 남은 줄 옆에서 다른 기계가 놀았다). free·freeFor는 Advise의 빈 자리
func pinnedItems(s State, f Facts, th Thresholds, free map[string]int, freeFor func([]string) (int, int, string)) []Item {
	var o []Item
	for _, w := range f.Waits {
		if w.Pin == "" || th.PinMin <= 0 || w.PinS == nil || *w.PinS < th.PinMin*60 || len(w.Wider) == 0 {
			continue
		}
		n, _, ft := freeFor(w.Wider)
		if n == 0 {
			continue
		}
		var wide []string
		for _, m := range s.Machines {
			if free[m.Name] > 0 && has(w.Wider, m.Name) {
				wide = append(wide, m.Name)
			}
		}
		only := strings.Join(append(append([]string(nil), w.Can...), wide...), ",")
		o = append(o, Item{Key: "pinned:" + w.Lane, Kind: "pinned",
			What:     fmt.Sprintf("묶인 기계 때문에 못 뜸: %s — 다른 기계에 자료를 두고 hc-fill edit %s --only %s 로 넓히기", w.Lane, w.Lane, only),
			Evidence: fmt.Sprintf("묶인 기계 %s · %s째 · 다른 기계 빈 자리 %d(%s)", w.Pin, age(w.PinS), n, ft),
			Cost:     "안 넓히면: 묶인 기계 자리가 날 때까지 이 줄이 안 뜨고 다른 기계는 논다 · 넓히면: 그 기계에만 둔 자료를 먼저 옮겨야 한다(지시문을 본다)",
			Next:     []string{"hc-fill edit " + w.Lane + " --only " + only + " --why '묶인 기계 넓히기(hc-fill advise)'"}, Sure: SureJudge})
	}
	return o
}
