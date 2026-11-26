package main

// 끝난 레인(2026-11-26 ht-fill-status-done — status가 848줄이었다: 244시간 전 「되돌려줌」까지 모두 나오고, 하루 넘게 전에 착지한
// 레인 80개가 아직 「감사 통과 — 대기열이 제출함」이었으며 「넣은 뒤」가 끝난 뒤에도 늘었다).
// 끝 = 착지(이 줄 접수 뒤의 착지 표식 refs/hc-landed/<레인>/<초> — 병합기 착지와 레인 직접 착지가 둘 다 둔다. status가 이미 읽는
// ls-remote 한 번에 들어 있다)·되돌려줌(장부 sent 줄의 바꾼 시각). 끝난 줄의 「넣은 뒤」는 끝난 시각에서 멈추고, 끝난 지
// state_done분(상태 한 장이 끝난 레인을 숨기는 손잡이 — 같은 값·같은 비교)이 넘은 줄은 기본 표에서 빠진다(--all은 모두).

import (
	"strings"
	"time"
)

// stMark: 레인의 가장 늦은 착지 표식 — 초와 그 표식이 가리키는 커밋
type stMark struct {
	Sec int64
	SHA string
}

// stMarks: ls-remote 출력에서 레인마다 가장 늦은 착지 표식(제출 ID 표식 refs/hc-landed/<ID>는 레인 칸이 없어 빠진다)
func stMarks(out string) map[string]stMark {
	m := map[string]stMark{}
	for _, l := range lines(out) {
		f := fields(l)
		if len(f) != 2 || !landedRe.MatchString(f[1]) {
			continue
		}
		rest := strings.TrimPrefix(f[1], "refs/hc-landed/")
		i := strings.LastIndexByte(rest, '/')
		lane, sec := rest[:i], int64(num(rest[i+1:]))
		if sec >= m[lane].Sec {
			m[lane] = stMark{sec, f[0]}
		}
	}
	return m
}

// stEnd: 떠 있는(up) 줄의 착지 — 병합기 줄에 없고(다시 낸 제출이 줄에 있으면 아직 안 끝남) 이 줄 접수 뒤의 표식이 있으면
// 「<sha 8> <월-일 시:분>」과 그 초. 아니면 빈 값·0
// ponytail: 표식이 기본 브랜치의 조상인지는 안 본다(status는 fetch하지 않는다) — 표식은 기본 브랜치 push와 한 원자 push다
func stEnd(r Row, pos int, mk map[string]stMark) (string, int64) {
	m, ok := mk[r.F(qName)]
	s, sok := sinceOf(r)
	if pos > 0 || !ok || !sok || m.Sec < int64(num(s)) {
		return "", 0
	}
	return head(m.SHA, 8) + " " + time.Unix(m.Sec, 0).Format("01-02 15:04"), m.Sec
}

// stSentAt <대기열 줄> <장부 요청 ID → 바꾼 시각>: 되돌려준 시각(초). 손으로 넣은 줄·장부에 없음은 0(모름)
func stSentAt(r Row, led map[string]string) int64 {
	_, rid, ok := strings.Cut(r.F(qReply), "#")
	if !ok {
		return 0
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", led[rid], time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// stLedTimes: 장부의 sent 줄 요청 ID → 바꾼 시각
func stLedTimes() map[string]string {
	m := map[string]string{}
	for _, r := range readRows(LED) {
		if r.F(ldState) == "sent" {
			m[r.F(ldRID)] = r.F(ldTime)
		}
	}
	return m
}

// stOld: 끝난 줄이 기본 표에서 빠질 만큼 오래됐나 — 끝난 지 doneMin분 넘음(opsstate.Trim과 같은 비교, 0이면 끝나자마자)
// ponytail: 끝난 시각을 모르면(0 — 장부에 없는 「되돌려줌」) 넣은 때로 본다. 막 되돌려준 옛 줄이 일찍 빠질 수 있다
func stOld(end int64, added string, doneMin int, now time.Time) bool {
	if end == 0 {
		t, err := time.ParseInLocation("2006-01-02T15:04", added, time.Local)
		if err != nil {
			return false
		}
		end = t.Unix()
	}
	return now.Unix()-end > int64(doneMin)*60
}
