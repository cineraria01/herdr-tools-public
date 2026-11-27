package main

// ht-alert-route(2026-11-27): 운영 에이전트의 「사람이 정한다」(레인 이름이 든 것)가 --to 없이 나가 지휘 둘이 각자 받아 겹치는 레인을 넣었다.
// alert_route one이면 글에 든 레인의 대기열 주인에게만(주인이 하나이고 살아 있을 때) — docs/hc-fill/alert-route.md

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

func alertKnob(t *testing.T, v string) {
	t.Helper()
	oh := HOME
	HOME = t.TempDir()
	knobOnce, knobES = sync.Once{}, nil
	t.Cleanup(func() { HOME = oh; knobOnce, knobES = sync.Once{}, nil })
	if err := knob.Set(knob.Path(HOME), knob.Entry{Name: "alert_route", Value: v, By: "시험", Why: "-", At: time.Now().Unix()}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// agentAsk: 운영 에이전트 끝 바퀴 — 결과 파일의 「사람이 정한다」 줄이 막힘 사건으로
func agentAsk(t *testing.T, res string) []string {
	t.Helper()
	now := time.Now()
	opsAgentTick(now)
	r := oaRunNow(t)
	if err := os.WriteFile(r.Dir+"/result", []byte(res), 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(time.Minute))
	var o []string
	for _, l := range reports(t) {
		if strings.Contains(l, "--name hc-fill-agent") {
			o = append(o, l)
		}
	}
	return o
}

// ③ 「사람이 정한다」에 레인 이름 → 그 레인 주인만(--to). ⑨ all이면 오늘과 같은 인자
func TestAlertOwnerAgentAsk(t *testing.T) {
	res := "요약 한 줄\n사람이 정한다: lane-b 선행 — 산출물이 필요한지 모름\n사람이 정한다: 병합기 표시 — 레인 이름 없음\n"
	oaFx(t, "ops_agent 조치")
	alertKnob(t, "all")
	all := agentAsk(t, res)
	oaFx(t, "ops_agent 조치")
	alertKnob(t, "one")
	one := agentAsk(t, res)
	if len(all) != 3 || len(one) != 3 {
		t.Fatalf("사건 수 all %d one %d: %q %q", len(all), len(one), all, one)
	}
	for _, l := range all {
		if strings.Contains(l, "--to") {
			t.Errorf("⑨ all: --to가 붙었다: %q", l)
		}
	}
	got := map[string]bool{}
	for _, l := range one {
		got[l] = strings.Contains(l, "--to m/t1")
	}
	for l, to := range got {
		want := strings.Contains(l, "lane-b")
		if to != want {
			t.Errorf("③ one: 레인 이름 든 것만 주인 m/t1에게 — %v: %q", to, l)
		}
	}
}

// ③ 거절 경로(에이전트가 넘길 것 종류를 침 — 레인 지우기)의 「사람이 정한다」도 그 레인 주인에게(감사 atp52u3 중간 2). all이면 --to 없음
func TestAlertOwnerGateHandoff(t *testing.T) {
	for _, mode := range []string{"all", "one"} {
		k, reps := gateKit(t, "조치")
		const own = "t5"
		var b strings.Builder
		for _, l := range lines(queueOf(t, k)) {
			if f := strings.Split(l, "\t"); len(f) > qOwner && f[0] == "lane-a" {
				f[qOwner-1] = own
				l = strings.Join(f, "\t")
			}
			b.WriteString(l + "\n")
		}
		if err := os.WriteFile(k.data+"/queue.tsv", []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := ehRow(t, k, "lane-a").F(qOwner); got != own {
			t.Fatalf("주인 칸을 못 넣음: %q", got)
		}
		if err := knob.Set(knob.Path(k.home), knob.Entry{Name: "alert_route", Value: mode, By: "시험", Why: "-", At: time.Now().Unix()}, true, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, se, rc := k.run(t, "rm", "lane-a", "--why", "치움"); rc != 1 {
			t.Fatalf("%s rm: %d %q", mode, rc, se)
		}
		var ask string
		for _, r := range reps() {
			if strings.Contains(r, "사람이 정한다") {
				ask = r
			}
		}
		if to := strings.Contains(ask, "--to "+own); to != (mode == "one") || mode == "all" && strings.Contains(ask, "--to") {
			t.Errorf("%s: 거절 「사람이 정한다」 받는 쪽 — %q (주인 %s)", mode, ask, own)
		}
	}
}

// 주인이 둘이거나 확인된 없음이면 빈 값(watch가 대표를 고른다). 이름은 낱말 단위(lane-a가 lane-ab에 걸리지 않음)
func TestAlertOwnerPick(t *testing.T) {
	advFx(t)
	alertKnob(t, "one")
	if o := alertOwner("lane-b 선행"); o != "m/t1" {
		t.Errorf("주인 %q", o)
	}
	if o := alertOwner("lane-bc 선행"); o != "" {
		t.Errorf("낱말 아님인데 %q", o)
	}
	q, _ := os.ReadFile(Q)
	if err := os.WriteFile(Q, []byte(string(q)+
		strings.Join([]string{"lane-d", "app", "b", "-", "-", "wait", "-", "-", "-", "2026-11-27T00:00", "-", "m/t2", "normal", "-", "-", "-", "r9"}, "\t")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if o := alertOwner("lane-b와 lane-d"); o != "" {
		t.Errorf("주인 둘인데 %q", o)
	}
}

// ⑪ advise 목록은 받는 쪽 손잡이와 상관없이 같다(모든 지휘가 그대로 본다)
func TestAlertAdviseSame(t *testing.T) {
	advFx(t)
	now := time.Now()
	alertKnob(t, "all")
	a := adviseNow(now).Items
	alertKnob(t, "one")
	if b := adviseNow(now).Items; !reflect.DeepEqual(a, b) {
		t.Errorf("⑪ advise가 손잡이로 바뀜:\n%+v\n%+v", a, b)
	}
}
