package opsstate

// ht-fill-submitted-line(2026-11-26): 병합 줄에서 정상으로 기다리는 제출(병합기 살아 있음 + 큐에 있음·도는 묶음의 구성원 — 모으는 쪽이
// Queued로 정한다)에는 「같은 단계 오래」를 올리지 않는다. 줄이 안 움직이는 것은 병합 줄 항목이 맡는다. 큐에도 없고 결과도 없는
// 제출(잃어버림)과 큐를 못 읽은 제출은 지금처럼 올린다. 고치기 전 코드에는 Lane.Queued가 없어 컴파일부터 실패한다

import "testing"

func TestAdviseLongSkipsQueued(t *testing.T) {
	s := aBase()
	s.Lanes = append(s.Lanes,
		Lane{Name: "in-batch", Machine: "a", Stage: "제출됨(병합기 줄 2번째 · 묶음 검증 중 b 12분째)", StageAgeS: ip(94 * 60), Open: true, Queued: true},
		Lane{Name: "lost", Machine: "a", Stage: "제출됨(줄에서 빠짐 — 결과 기다림)", StageAgeS: ip(94 * 60), Open: true},
		Lane{Name: "unread", Machine: "a", Stage: "제출됨(병합기 줄 모름)", StageAgeS: ip(94 * 60), Open: true})
	f := Facts{Addr: map[string]string{"in-batch": "a/t1", "lost": "a/t2", "unread": "a/t3"}}
	got := map[string]bool{}
	for _, it := range kinds(Advise(s, f, ath), "lane-long") {
		got[it.Key] = true
	}
	if got["lane-long:in-batch"] || !got["lane-long:lost"] || !got["lane-long:unread"] || len(got) != 2 {
		t.Fatalf("lane-long은 잃어버린 제출·줄 모름 둘만: %v", got)
	}
}
