package main

// 이어 붙인 단계 착지 확인(ht-revive-cmd, 2026-11-28 지휘 실수 — docs/hc-fill/then.md): --then으로 이어 붙인 레인이 1단계만 착지하고
// 끝나 뒤 단계가 안 돈 채 사라졌다. 바퀴마다(stallTick) 착지한 up 줄(그 실행의 마지막 보고 「착지」)의 지시문 사본에서 「## 이어서: <이름>」
// 절마다 기본 가지의 접수 뒤 커밋에 제목 머리 「<이름>:」이 있는지(onlandHit — 착지 뒤 명령과 같은 판정) 보고, 없는 것이 있으면 주인
// 지휘에 막힘 한 번. 본 줄은 DATA/then.tsv에 「레인 줄ID 결과」로 적어 다시 보지 않는다. 파일이 없는 첫 바퀴는 이미 착지한 줄을 「옛」으로만
// 적는다(이 판 전에 착지한 줄은 알리지 않는다). 판정을 모르면(fetch·조회 실패) 적지 않고 다음 바퀴에

import (
	"fmt"
	"strings"

	"herdr-tools/internal/tri"
)

func thenFile() string { return DATA + "/then.tsv" }

func thenCheckTick() {
	if EVBAD || !nonEmpty(Q) {
		return
	}
	first := !exists(thenFile())
	rows := qRows()
	inq, seen := map[string]bool{}, map[string]bool{}
	for _, q := range rows {
		inq[q.F(qName)+"\t"+q.F(qRID)] = true
	}
	var out []Row
	for _, r := range readRows(thenFile()) {
		if k := r.F(thLane) + "\t" + r.F(thRID); inq[k] && !seen[k] { // 대기열에서 빠진 줄은 걷는다
			seen[k] = true
			out = append(out, r)
		}
	}
	changed := first
	for _, q := range rows {
		n, rid, m := q.F(qName), q.F(qRID), q.F(qMach)
		if q.F(qState) != "up" || seen[n+"\t"+rid] {
			continue
		}
		names := thenNames(q.F(qBrief))
		if len(names) == 0 || (m != LOCAL && (!connHas(m) || has(evUnk(), m))) || evF(evLast(n, q.F(qAddr)), evState) != "착지" {
			continue
		}
		res := "옛"
		if !first {
			miss, ok := thenMissing(q, names)
			if !ok {
				continue
			}
			res = "없음"
			if len(miss) > 0 {
				if aesc(n, dash(q.F(qOwner)), thenMsg(n, miss, rows), "hf:"+n+":then-miss:"+rid) != 0 {
					continue
				}
				res = "알림"
			}
		}
		out, changed = append(out, Row{n, rid, res}), true
	}
	if changed && !dry {
		if err := writeAtomic(thenFile(), rowsText(out)); err != nil {
			say("이어 붙인 단계 확인 기록을 못 씀: %v", err)
		}
	}
}

// thenMissing <착지한 줄> <이어 붙인 이름들>: 기본 가지의 접수 뒤에 제목 머리가 없는 이름들 — (빠진 것, 판정함). fetch·조회를 모르면 판정 못 함
func thenMissing(q Row, names []string) ([]string, bool) {
	st, ok := sinceOf(q)
	if !ok || fetchProj(q.F(qProj)) != 0 {
		say("이어 붙인 단계 확인 미룸 %s: 접수 시각·기본 가지를 모름 — 다음 바퀴에", q.F(qName))
		return nil, false
	}
	var miss []string
	for _, tn := range names {
		lms, lmf := LMS, LMFROM // lmark(onlandHit 안)는 선행 판정용 전역을 쓴다 — 이 바퀴의 다른 판정을 흐리지 않게 되돌린다(stallMerger와 같다)
		_, hit := onlandHit(tn, int64(num(st)))
		LMS, LMFROM = lms, lmf
		switch hit {
		case tri.Unknown:
			say("이어 붙인 단계 확인 미룸 %s: %s 착지 조회 실패 — 다음 바퀴에", q.F(qName), tn)
			return nil, false
		case tri.No:
			miss = append(miss, tn)
		case tri.Yes:
		}
	}
	return miss, true
}

// thenMsg: 막힘 글 — 빠진 이름을 --after로 기다리는 줄이 있으면 그 이름도
func thenMsg(n string, miss []string, rows []Row) string {
	var waits []string
	for _, r := range rows {
		for _, a := range words(r.F(qAfter)) {
			if has(miss, a) {
				waits = append(waits, r.F(qName))
				break
			}
		}
	}
	s := fmt.Sprintf("%s 이어 붙인 단계 %s이 착지에 없다 — 그 단계가 안 돈 채 레인이 끝났다(새 줄로 다시 넣어라)", n, strings.Join(miss, ","))
	if len(waits) > 0 {
		s += " — 그 이름을 --after로 기다리는 줄 " + strings.Join(uniq(waits), ",")
	}
	return s
}
