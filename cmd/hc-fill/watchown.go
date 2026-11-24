package main

import "strings"

// watchOwners: 열린 줄(떠 있음·주차함)이 있는 주인 지휘마다 hc-lane coord --watch — 그 지휘의 watch가 한가로 끝났으면 다시 띄운다
// (watch-idle-open, 2026-11-24: 마지막 제출에서 결과까지 60분 넘은 착지가 30% — 그사이 watch가 끝나 알림이 늦거나 다른 지휘에게 갔다).
// 떠 있음 줄은 hc-fill rm 전까지 남으니 마지막 보고(last.tsv, evTick)가 착지·판정인 줄은 닫힌 것으로 본다 — 보고 없음·모름은 열림
// (놓치지 않기). 감사 줄은 그 레인 줄이 센다. 지휘 확인·에이전트 확인·같은 지휘로 하나만은 hc-lane이 한다. 결과는 띄웠을 때만 한 줄
func watchOwners() {
	done := map[[2]string]bool{}
	for _, l := range readRows(DATA + "/last.tsv") {
		if s := l.F(2); s == "착지" || s == "판정" {
			done[[2]string{l.F(1), l.F(8)}] = true
		}
	}
	seen := map[string]bool{}
	for _, r := range qRows() {
		o, st := r.F(12), r.F(6)
		if o == "" || o == "-" || seen[o] || strings.HasPrefix(r.F(15), "audit=") {
			continue
		}
		if !(st == "up" && !done[[2]string{r.F(1), r.F(17)}] || st == "wait" && r.F(14) == "parked") {
			continue
		}
		seen[o] = true
		if out, _ := (Cmd{Name: "hc-lane", Args: []string{"coord", "--watch", o}, Err: errDrop}).Run(); strings.Contains(out, "띄움") {
			say("감시: 주인 지휘 %s 의 watch를 다시 띄움(열린 레인 있음)", o)
		}
	}
}
