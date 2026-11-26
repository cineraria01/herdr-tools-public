package main

import (
	"fmt"
	"strconv"
	"strings"
)

// 감사 자리 없음(fill-audit-seat, 2026-11-24): 자동 감사가 기계 자리가 차서 못 뜬 바퀴를 세어 오래되면 지휘에 한 번 알린다.
// 2026-11-27 ht-seat-working-only부터 쉬는 레인은 자리를 안 쓰므로 감사 몫 예약(RSV·rsvOn)은 걷었고, 감사 줄은 바퀴 차례 맨 앞(orderRows)이다

// aseatSweep: 바퀴마다(orderRows 앞) 이제 wait가 아닌 감사 줄의 자리 없음 기록을 지운다
func aseatSweep() {
	if AUD == "" || !isFile(Q) {
		return
	}
	wait := map[string]bool{}
	for _, r := range qRows() {
		if strings.HasPrefix(r.F(qAudit), "audit=") && r.F(qState) == "wait" {
			wait[r.F(qName)] = true
		}
	}
	aseatPrune(wait)
}

// aseat.tsv: 「감사 줄 이름 · 자리 없음 바퀴 수 · 처음 자리 없음 초 · 알림(0|1)」
func aseatPath() string { return DATA + "/aseat.tsv" }

// aseatPrune: 이제 wait가 아닌 감사 줄(떴음·지움·넘김)의 기록을 지운다
func aseatPrune(wait map[string]bool) {
	if dry || !isFile(aseatPath()) {
		return
	}
	var keep []Row
	rows := readRows(aseatPath())
	for _, r := range rows {
		if wait[r.F(asName)] {
			keep = append(keep, r)
		}
	}
	if len(keep) != len(rows) {
		if err := writeAtomic(aseatPath(), rowsText(keep)); err != nil {
			say("경고: %v", err)
		}
	}
}

// aseatWait <감사 줄> <기계>: 기계 자리가 차서 못 뜬 바퀴를 센다. audit_seat_rounds(기본 5)바퀴를 넘으면 그 레인 주인 지휘에 사건 한 번.
// 도구는 상한을 넘겨 띄우지 않는다 — 자리를 올리거나 레인을 옮기는 것은 지휘
func aseatWait(an, m string) {
	if dry {
		return
	}
	lim := 5
	if v := conf("audit_seat_rounds"); v != "" {
		if !isDigits(v) {
			say("알림: 설정 audit_seat_rounds는 바퀴 수(숫자)다, 5로: %s", v)
		} else {
			lim = atoi10(v)
		}
	}
	now := nowUnix()
	rows := readRows(aseatPath())
	i := -1
	for j, r := range rows {
		if r.F(asName) == an {
			i = j
		}
	}
	if i < 0 {
		rows = append(rows, Row{an, "0", strconv.FormatInt(now, 10), "0"})
		i = len(rows) - 1
	}
	r := rows[i]
	c := atoi10(r.F(asTurns)) + 1
	r = r.Set(asTurns, strconv.Itoa(c))
	if c > lim && r.F(asTold) != "1" {
		lane := ""
		if q := qrow(an); q != nil {
			lane = strings.TrimPrefix(q.F(qAudit), "audit=")
		}
		first, _ := strconv.ParseInt(r.F(asFirst), 10, 64)
		msg := fmt.Sprintf("감사 %s(%s)가 %s 자리 없어 %d분 대기 — 자리를 올리거나 레인을 옮긴다(hc-fill은 상한을 넘겨 띄우지 않음)",
			strings.TrimPrefix(an, "audit-"), lane, m, (now-first)/60)
		if lane != "" && aesc(lane, qown(lane), msg, "hf:"+strings.TrimPrefix(an, "audit-")+":seat") == 0 {
			r = r.Set(asTold, "1")
		}
	}
	rows[i] = r
	if err := writeAtomic(aseatPath(), rowsText(rows)); err != nil {
		say("경고: %v", err)
	}
}

// aseatMin <감사 줄>: 자리 없어 기다린 분(기록 없으면 -1)
func aseatMin(an string) int64 {
	for _, r := range readRows(aseatPath()) {
		if r.F(asName) == an {
			first, err := strconv.ParseInt(r.F(asFirst), 10, 64)
			if err != nil {
				return -1
			}
			return (nowUnix() - first) / 60
		}
	}
	return -1
}
