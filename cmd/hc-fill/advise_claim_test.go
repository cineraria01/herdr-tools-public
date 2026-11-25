package main

// ht-advise-claim(2026-11-26): 같은 알림을 지휘 둘이 따로 손댔다 — advise 항목에 「조치함」, 사건에도 그 줄, 손잡이 덮기 알림,
// 레인 항목은 주인 지휘에게만 손댈 것(다른 지휘에게는 묶음 참고 한 줄)

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/journal"
)

// 레인 항목: 주인 있으면 그 주인에게만(--to, 조치함 줄 실음), 다른 지휘에게는 참고(묶음 키) · 주인 없는 레인은 지금처럼 모두에게
func TestAdviseClaimOwnerTick(t *testing.T) {
	advFx(t)
	now := time.Now()
	add := now.Add(-3 * time.Hour).Format("2006-01-02T15:04")
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	akWrite(t, "queue.tsv", string(q)+strings.Join([]string{"lane-d", "app", "b", "-", "-", "wait", "-", "-", "-", add, "-", "-", "normal", "-", "-", "-", "r4"}, "\t")+"\n")
	akWrite(t, "overlap.tsv", "lane-c\tlane-a\tdocs/x.md\t-\tapp\nlane-d\tlane-a\tdocs/y.md\t-\tapp\n")
	if err := journal.Append(journal.Entry{Time: now.Add(-5 * time.Minute), By: "w9/t3", Tool: "hc-fill", Action: "edit", Target: "lane-b", Why: "선행 풀기", Result: "됨"}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(journal.Entry{Time: now.Add(-50 * time.Minute), By: "w9/t3", Tool: "hc-fill", Action: "edit", Target: "lane-c", Why: "옛 조치", Result: "됨"}); err != nil {
		t.Fatal(err)
	}
	a := adviseNow(now)
	for _, it := range a.Items {
		switch it.Key {
		case "free-wait:lane-b":
			if !strings.HasPrefix(it.Claim, "w9/t3 5분 전 — edit lane-b(선행 풀기)") {
				t.Errorf("조치함 안 붙음: %+v", it)
			}
		default:
			if it.Claim != "" {
				t.Errorf("%s: 오래된·없는 조치인데 조치함 %q", it.Key, it.Claim)
			}
		}
	}
	adviseTick(now)
	var own, all []string
	ref := refReports(t)
	for _, r := range reports(t) {
		if strings.Contains(r, "--to m/t1") {
			own = append(own, r)
		} else {
			all = append(all, r)
		}
	}
	if len(own) != 1 || !strings.Contains(own[0], "손댈 것: 풀 수 있는 대기: lane-b") || !strings.Contains(own[0], "조치함: w9/t3 5분 전 — edit lane-b(선행 풀기) 외 ") {
		t.Errorf("주인 지휘에게: %q\n전체 %q", own, reports(t))
	}
	if len(ref) != 1 || !strings.Contains(ref[0], "손댈 것 참고") || !strings.Contains(ref[0], "--key "+events.AdviseRefKey) || !strings.Contains(ref[0], "free-wait:lane-b → m/t1") || strings.Contains(ref[0], "lane-d") {
		t.Errorf("다른 지휘에게 참고: %q", ref)
	}
	if len(all) != 1 || !strings.Contains(all[0], "lane-d") || strings.Contains(all[0], "lane-b") || strings.Contains(all[0], "lane-c") {
		t.Errorf("주인 없는 레인은 모두에게: %q", all)
	}
}

// 감사 atp2vpf 중간 1: 주인 지휘 터미널이 확인된 없음(hc get not_found)이면 그 레인 항목은 모두에게(--to 없이) — 사라진 주인의 watch로만
// 가면 아무 지휘도 손댈 것으로 받지 못하고 열쇠는 알린 것으로 적혔다. 주인 조회 모름은 지금처럼 주인에게
// 감사 atp2wjf 중간 1: 확인된 끝남(hc get 종료 11·state finished)도 사라짐 — hc-lane getState와 같은 기준
func TestAdviseOwnerGoneToAll(t *testing.T) {
	for name, get := range map[string]string{
		"없음": "echo '{\"error\":{\"kind\":\"not_found\",\"message\":\"no\"}}' >&2; exit 3",
		"끝남": "echo '{\"address\":\"m/t1\",\"created_ms\":1,\"name\":\"w\",\"pid\":5,\"state\":\"finished\",\"status\":null}'; exit 11",
	} {
		t.Run(name, func(t *testing.T) {
			advFx(t)
			akWrite(t, "hc", "#!/bin/sh\ncase \"$1\" in ls) cat '"+DATA+"/lsjson';; machine) cat '"+DATA+"/mls' || exit 1;;\nget) "+get+";; esac\n")
			adviseTick(time.Now())
			r := reports(t)
			if len(r) != 1 || strings.Contains(r[0], "--to ") || !strings.Contains(r[0], "손댈 것: 풀 수 있는 대기: lane-b") || len(refReports(t)) != 0 {
				t.Fatalf("사라진 주인의 레인 항목은 모두에게 한 사건: %q 참고 %q", r, refReports(t))
			}
		})
	}
}

// knob set이 다른 지휘가 손잡이 advise_claim분 안에 정한 같은 손잡이를 덮으면 표준 오류 한 줄 — 내가 정한 것·오래된 것은 없음
func TestKnobSetOverwriteNotice(t *testing.T) {
	reround := knobFix(t)
	confLines = []string{"local here"}
	now := time.Now()
	set := func(by string, at time.Time, val, why string) string {
		t.Helper()
		t.Setenv("HC_TERMINAL_ID", by)
		reround()
		return errCapture(t, func() {
			if err := knobSet([]string{"idle", val, "--why", why}, at); err != nil {
				t.Fatal(err)
			}
		})
	}
	set("w1/t1", now.Add(-60*time.Minute), "40", "옛 값")
	if got := set("w1/t2", now.Add(-20*time.Minute), "30", "밤사이"); strings.Contains(got, "덮음") {
		t.Errorf("오래된 값(그때 40분 전)에 덮음 알림: %q", got)
	}
	if got := set("w1/t2", now.Add(-15*time.Minute), "35", "다시"); strings.Contains(got, "덮음") {
		t.Errorf("내가 정한 값에 덮음 알림: %q", got)
	}
	if got := set("w1/t3", now, "20", "줄임"); !strings.Contains(got, "hc-fill: w1/t2가 15분 전 35로 정함(다시) — 덮음") {
		t.Errorf("덮음 알림 없음: %q", got)
	}
}
