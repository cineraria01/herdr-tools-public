package opsstate

// 2026-11-24 지휘: 「같은 단계에 오래」는 터미널이 살아 있는(또는 모르는) 줄만. 실제 대기열에서 끝남(터미널 없음)·마지막 사건 완료인
// 줄(스스로 착지·정리하고 완료로 보고한 레인, 착지 없는 검토 레인)이 거짓 양성이었다 — 자리를 안 잡고 hc read할 터미널도 없다.
// 고치기 전 코드로는 끝난 두 줄에도 lane-long이 나와 실패한다

import "testing"

func TestAdviseLongSkipsClosedTerminal(t *testing.T) {
	s := aBase()
	s.Lanes = append(s.Lanes,
		Lane{Name: "self-landed", Machine: "a", Stage: "완료 — 감사 기다림", StageAgeS: ip(200 * 60), LastEvent: "완료 abc1234 스스로 착지·정리함"},
		Lane{Name: "review", Machine: "a", Stage: "완료", StageAgeS: ip(300 * 60), LastEvent: "완료 검토 보고"},
		Lane{Name: "live-long", Machine: "a", Stage: "작업 중(working)", StageAgeS: ip(120 * 60), Open: true})
	f := Facts{Addr: map[string]string{"self-landed": "a/t1", "review": "a/t2", "live-long": "a/t3"}}
	it := kinds(Advise(s, f, ath), "lane-long")
	if len(it) != 1 || it[0].Key != "lane-long:live-long" {
		t.Fatalf("같은 단계에 오래는 살아 있는 live-long 하나여야: %+v", it)
	}
}
