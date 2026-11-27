package main

// 먼저 띄움과 선행 사슬(early-own-chain, 2026-11-24): 먼저 띄움을 판정할 때 후보의 선행 사슬(--after로 이어진 것, --then으로 받은 줄 포함)이
// 쥔 예약은 겹침으로 세지 않는다 — 선행이 고치는 파일을 뒤 레인도 고치는 것은 --after를 건 까닭 그 자체라, 준비를 막지 않고 착지 뒤 rebase로 푼다.
// 사슬 밖 레인과의 겹침·단독·일반 배치 판정은 그대로

import (
	"fmt"
	"os"
	"strings"
)

// OVEARLY: 지금 ovlCheck가 먼저 띄움 판정 중(earlyRows만 켠다). OVCHAIN: 그 판정에서 세지 않은 선행 사슬 겹침 「앞레인 실제|예약 키」
var (
	OVEARLY bool
	OVCHAIN []Row
)

// qGraph: 기다림 사슬 — 레인 → 「,선행,」을 이어 붙인 글(대기열 줄 QEDGES·쓰기 소유 실행 기록 OREC), 이어 붙인 이름 → 받은 줄(THENMAP)
func qGraph() (A, T map[string]string) {
	A, T = map[string]string{}, map[string]string{}
	for _, r := range QEDGES {
		if r.NF() >= qeAfter {
			A[r.F(qeLane)] += r.F(qeAfter)
		}
	}
	for _, r := range THENMAP {
		if r.NF() >= tmInto {
			T[r.F(tmName)] = r.F(tmInto)
		}
	}
	for _, r := range OREC {
		if r.NF() >= owAfter {
			A[r.F(owLane)] += r.F(owAfter)
		}
	}
	return A, T
}

// reach: from이 (여러 단계 --after, --then 이름은 받은 줄로 바꿔서) 기다리는 레인 전부
func reach(A, T map[string]string, from string) map[string]bool {
	q, seen := []string{from}, map[string]bool{}
	for h := 0; h < len(q); h++ {
		for _, k := range strings.Split(A[q[h]], ",") {
			if k == "" || k == "-" {
				continue
			}
			if t, ok := T[k]; ok {
				k = t
			}
			if !seen[k] {
				seen[k] = true
				q = append(q, k)
			}
		}
	}
	return seen
}

// chainCut <후보> <겹침(ovcmp)>: 먼저 띄움 판정이면 후보 선행 사슬의 실제·예약 겹침을 빼서 OVCHAIN에 두고 나머지를 돌려준다(언급은 그대로 경고)
func chainCut(c string, ov []Row) []Row {
	if !OVEARLY {
		return ov
	}
	A, T := qGraph()
	up := reach(A, T, c)
	var rest []Row
	for _, r := range ov {
		if up[r.F(ovLane)] && r.F(ovLane) != c && (r.F(ovGrade) == "예약" || r.F(ovGrade) == "실제") {
			say("먼저 띄움 선행 겹침 %s: %s (%s) — 선행 사슬이라 막지 않음, 선행 착지 뒤 rebase", c, r.F(ovLane), r.F(ovKeys))
			OVCHAIN = append(OVCHAIN, r)
			continue
		}
		rest = append(rest, r)
	}
	return rest
}

// earlyChainSave <줄 ID 있는 대기열 줄들> <이번 바퀴에 띄운 「레인 줄ID 앞레인 키」>: DATA/early-chain.tsv(ls 비고)를 지금 대기열에 먼저 띄움(up·early)으로
// 남은 줄만 남기고 새 줄을 더해 다시 쓴다
func earlyChainSave(rows []Row, add []Row) {
	f := DATA + "/early-chain.tsv"
	live := map[string]bool{}
	for _, r := range rows {
		if r.F(qState) == "up" && rowMode(r) == mEarly {
			live[r.F(qName)+"\t"+r.F(qRID)] = true
		}
	}
	old := readRows(f)
	var keep []Row
	for _, r := range old {
		if live[r.F(ecLane)+"\t"+r.F(ecRID)] {
			keep = append(keep, r)
		}
	}
	if len(add) == 0 && len(keep) == len(old) || dry {
		return
	}
	if err := writeAtomic(f, rowsText(append(keep, add...))); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// earlyChainNote <레인> <줄 ID>: ls 비고 — 선행 예약과 겹친 채 먼저 띄운 줄이면 「 선행 예약과 겹침(앞레인: 키 — 착지 뒤 rebase)」
func earlyChainNote(n, rid string) string {
	var s []string
	for _, r := range readRows(DATA + "/early-chain.tsv") {
		if r.F(ecLane) == n && r.F(ecRID) == rid {
			s = append(s, r.F(ecFront)+": "+r.F(ecKeys))
		}
	}
	if len(s) == 0 {
		return ""
	}
	return " 선행 예약과 겹침(" + strings.Join(s, ", ") + " — 착지 뒤 rebase)"
}

// shFail <출력> <종료>: 원격 조각이 실패한 까닭 한 토막 — 종료 코드와 끝 줄(조각 끝 표식이 없으면 잘림)
func shFail(out string, rc int) string {
	w := "종료 " + fmt.Sprint(rc)
	if rc == 0 {
		w = "끝 표식 없음(출력 잘림)"
	}
	ls := lines(strings.TrimRight(out, "\n"))
	if len(ls) == 0 || ls[len(ls)-1] == "" {
		return w + ", 출력 없음"
	}
	l := ls[len(ls)-1]
	if r := []rune(l); len(r) > 160 {
		l = string(r[:160]) + "…"
	}
	return w + ", 끝 줄 「" + l + "」"
}
