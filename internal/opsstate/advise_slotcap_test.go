package opsstate

import (
	"strings"
	"testing"
)

// ht-advise-slot-cap(2026-11-26): 빈 자리는 바퀴의 띄우기 판정(Free)만 읽는다 — 레인·자리 칸으로 따로 세면
// 레인 1 + 검증 1, 상한 2인 mini-m4가 「빈 자리 1」로 보여 거짓 free-wait가 났다(바퀴는 「레인상한 참」으로 아무것도 못 띄움)
func TestAdviseFreeFromLaunchVerdict(t *testing.T) {
	s := aBase()
	s.Machines[0] = Machine{Name: "mini-m4", Reach: "ok", Cores: ip(10), LoadPerCore: fp(0.3), Live: ip(2), Seats: ip(5),
		Lanes: ip(1), Verifies: ip(1), LaneCap: ip(2), Free: ip(0), SkipWhy: "레인상한 참"}
	f := Facts{Waits: []Wait{{Lane: "doc-a", Project: "p", AgeS: ip(30 * 60), Front: "lane-front", Keys: []string{"docs/x.md"}}}}
	for _, k := range []string{"free-wait", "chain", "pinned", "skew"} {
		if it := kinds(Advise(s, f, ath), k); len(it) != 0 {
			t.Errorf("바퀴 판정 빈 자리 0인데 %s 항목 %+v", k, it)
		}
	}
	// 바퀴가 빈 자리 1로 판정했으면 낸다(근거의 수도 그 값)
	s.Machines[0].Free = ip(1)
	it := kinds(Advise(s, f, ath), "free-wait")
	if len(it) != 1 || !strings.Contains(it[0].Evidence, "빈 자리 1(mini-m4 1)") {
		t.Errorf("빈 자리 1: %+v", it)
	}
	// 모름(nil — 옛 load.tsv)이면 세지 않는다
	s.Machines[0].Free = nil
	if it := kinds(Advise(s, f, ath), "free-wait"); len(it) != 0 {
		t.Errorf("빈 자리 모름인데 %+v", it)
	}
}
