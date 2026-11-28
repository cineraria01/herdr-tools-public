package main

// ht-alert-sources(2026-11-28): 「감사 터미널 … 정리 필요」는 할 일이 있을 때만 — 이 바퀴 목록(TERMS)에는 쉬는 감사 터미널로 보였지만
// 내기 직전에 다시 보니 이미 끝났으면(finished·없음) 내지 않고 감사 줄을 뺀다(받은 지휘가 가 보니 닫을 것이 없었다). 살아 있는데 못 닫으면 지금처럼 한 번

import (
	"strings"
	"testing"
)

func cleanupSetup(t *testing.T) *clsT {
	k := clsSetup(t, "done")
	k.setRound("taken", "pass")
	k.wr("hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/report.log'\n")
	return k
}

func TestAuditCleanupGoneTerm(t *testing.T) {
	k := cleanupSetup(t)
	k.wr("terms.tsv", "m/w1\tw1\t1700000000000\tidle\t"+k.wt+"\n") // hc get: 감사 터미널은 이미 없음(목록 TERMS는 바퀴 앞에 읽은 것)
	k.round()
	if r := k.read("report.log"); strings.Contains(r, "정리 필요") {
		t.Fatalf("이미 끝난 감사 터미널에 정리 필요: %q", r)
	}
	if qrow("audit-a1") != nil {
		t.Fatalf("끝난 터미널의 감사 줄은 뺀다")
	}
}

// 반대 장면: 살아 있는데 못 닫음 → 정리 필요 한 번(지금처럼)
func TestAuditCleanupLiveKillFails(t *testing.T) {
	k := cleanupSetup(t)
	k.wr("killfail", "")
	k.round()
	if r := k.read("report.log"); strings.Count(r, "정리 필요") != 1 {
		t.Fatalf("살아 있는데 못 닫음 → 정리 필요 한 번: %q", r)
	}
}
