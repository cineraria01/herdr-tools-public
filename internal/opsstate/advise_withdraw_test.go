package opsstate

// ht-merger-withdraw ③: 같은 제출이 든 묶음이 두 번 넘게 실패하면(병합기 기록 fails ≥ 3) 「거둘 수 있음」 — 레인 항목(주인 지휘에게),
// 다음 명령은 미리 보기 withdraw, 확신은 판단 필요(스스로 거두지 않는다)

import "testing"

func TestAdviseWithdraw(t *testing.T) {
	m := mline(ip(5*60), ip(60))
	m.Fails = map[string]int{"ln-a": 3, "ln-b": 2}
	s := State{Schema: 1, At: t0, Queue: Queue{Counts: map[string]int{}}, Merge: []MergeLine{m}}
	var got []Item
	for _, x := range Advise(s, Facts{}, msth) {
		if x.Kind == "withdraw" {
			got = append(got, x)
		}
	}
	if len(got) != 1 {
		t.Fatalf("두 번 넘게 실패한 ln-a 하나만: %+v", got)
	}
	it := got[0]
	if it.Key != "withdraw:ln-a" || it.Lane() != "ln-a" || it.Sure != SureJudge || len(it.Next) != 1 ||
		it.Next[0] != "hc-fill withdraw ln-a --why '<까닭>' --dry-run" || it.What != "거둘 수 있음: ln-a(p 병합 줄)" || it.Evidence != "그 제출이 든 묶음이 3번 실패(병합기 기록)" {
		t.Fatalf("항목: %+v", it)
	}
	m.Merger = "gone" // 병합기가 없으면 병합기 항목만(거둘 것을 따로 내지 않는다)
	s.Merge = []MergeLine{m}
	for _, x := range Advise(s, Facts{}, msth) {
		if x.Kind == "withdraw" {
			t.Fatalf("병합기 없음에 거둘 수 있음: %+v", x)
		}
	}
}
