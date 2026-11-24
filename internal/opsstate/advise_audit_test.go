package opsstate

// 감사 atozr8z(ops-advise 1번째) 지적 시험 — 고치기 전 코드로 둘 다 실패했다

import (
	"testing"
)

// 중간 1: 옛 판 상주의 다시 띄우는 명령(kill <pid>·hc kill)은 다음 명령으로 내지 않는다 — 사람이 정한다, 근거에만
func TestAdviseResidentNoKill(t *testing.T) {
	for _, c := range []string{"kill 4242 && hc-lane coord --watch t3yx", "hc kill t9zz && hc-lane coord --watch t3yx", "launchctl kickstart -k gui/501/x"} {
		line := "옛 판으로 도는 상주: hc-lane watch(pid 4242, 시작 x) — 판 a · 깔린 판 b — 다시 띄우기: " + c + " · 확인 3h0m0s 전"
		it := kinds(Advise(aBase(), Facts{Resident: []string{line}}, ath), "resident")
		if len(it) != 1 || len(it[0].Next) != 0 || it[0].Sure != SureHuman || it[0].Evidence != line {
			t.Errorf("%s: %+v", c, it)
		}
	}
}

// 낮음 1: 상태가 그대로면 나이가 59초 → 1분, 59분 → 1시간을 넘어도 지문이 같다
func TestFingerprintAgeUnits(t *testing.T) {
	fpOf := func(stage int, wait int) string {
		s := aBase()
		s.Lanes[0].StageAgeS = ip(stage)
		// idle 0은 끔(advise-fixes) — 겹침 줄은 오래 기다린 것으로 앞 레인 단계 나이를, 기다린 나이는 나이와 무관한 거절 줄이 근거에 낸다
		f := Facts{Waits: []Wait{{Lane: "doc-a", Project: "p", AgeS: ip(3600), Front: "lane-front", Keys: []string{"docs/x.md"}},
			{Lane: "rj", Project: "p", AgeS: ip(wait), Reject: "가지가 이미 있음"}}}
		s.Notable = Summaries(Advise(s, f, Thresholds{IdleMin: 1}))
		if len(s.Notable) == 0 {
			t.Fatal("항목 없음")
		}
		return Fingerprint(s)
	}
	if a, b := fpOf(59, 59), fpOf(61, 61); a != b {
		t.Errorf("초 → 분에서 지문이 바뀜")
	}
	if a, b := fpOf(59*60, 59*60), fpOf(61*60, 61*60); a != b {
		t.Errorf("분 → 시간에서 지문이 바뀜")
	}
}
