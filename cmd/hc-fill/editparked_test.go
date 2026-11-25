//go:build !windows

package main

// ht-edit-parked(2026-11-26): 먼저 띄운 레인이 hc-lane park로 「주차」를 보고한 뒤 run 터미널이 done으로 남아 있으면, run 바퀴의
// parkTick이 줄을 대기·parked로 되돌리기 전까지 edit --brief가 「이미 떴다(up)」로 거절됐다(지휘가 터미널을 손으로 닫아야 했다).
// edit은 그 줄에 parkTick과 같은 판정을 바로 한다 — 주차 + 터미널 done → 줄을 주차로 되돌리고 받음, 일하는 중(working) → 지금처럼 거절

import (
	"os"
	"strings"
	"testing"
)

func epKit(t *testing.T, status, report string) (fillKit, string) {
	k, f := nudgeKit(t)
	ehPut(t, k.data+"/queue.tsv", "lane-a\tapp\t/b.md\tp0\t-\tup\there\tt1\t-\t1\t-\t-\tnormal\tearly\t-\t-\tr1\n", 0o644)
	nudgeGet(t, f, "lane-a", status)
	ehPut(t, f+"/events", "2026-11-25T22:51:34Z\there\tt1\tlane-a\t"+report+"\tabcdef1\tmsg\t0\t-\tt1@1700000000000\n", 0o644)
	ehPut(t, k.home+"/new.md", "# 새 지시문\n이어 가기에 한 줄\n", 0o644)
	return k, k.home + "/new.md"
}

// 주차 보고 + 터미널 done: edit --brief를 받고, 줄은 대기·parked(줄 ID 그대로)
func TestEditParkedDoneTerm(t *testing.T) {
	k, nb := epKit(t, "done", "주차")
	out, se, rc := k.run(t, "edit", "lane-a", "--brief", nb)
	if rc != 0 {
		t.Fatalf("주차한 레인의 edit --brief는 받아야 — %d %q %q", rc, out, se)
	}
	r := ehRow(t, k, "lane-a")
	if r.F(qState) != "wait" || r.F(qEarly) != "parked" || r.F(qRID) != "r1" || !strings.HasSuffix(r.F(qBrief), "/briefs/added/lane-a.md") {
		t.Fatalf("줄이 주차(대기·parked·같은 줄 ID·새 지시문)여야: %q", r.Line())
	}
	if !strings.Contains(out, "주차") {
		t.Errorf("주차로 되돌렸다는 말이 있어야: %q", out)
	}
}

// 일하는 중(working)·주차 보고 없음(중간): 지금처럼 「이미 떴다」로 거절, 대기열 그대로
func TestEditParkedRefused(t *testing.T) {
	for _, c := range []struct{ status, report string }{{"working", "주차"}, {"done", "중간"}} {
		k, nb := epKit(t, c.status, c.report)
		before, _ := os.ReadFile(k.data + "/queue.tsv")
		if _, se, rc := k.run(t, "edit", "lane-a", "--brief", nb); rc != 2 || !strings.Contains(se, "이미 떴다") {
			t.Errorf("%s·%s: 종료 2·「이미 떴다」여야 — %d %q", c.status, c.report, rc, se)
		}
		if after, _ := os.ReadFile(k.data + "/queue.tsv"); string(after) != string(before) {
			t.Errorf("%s·%s: 거절했는데 대기열이 바뀌었다:\n%s", c.status, c.report, after)
		}
	}
}
