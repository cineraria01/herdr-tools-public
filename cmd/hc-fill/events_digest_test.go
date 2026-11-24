package main

// coord-digest(2026-11-24) ⑧: 운영 에이전트가 조치 방식이고 새 손댈 것을 모두 맡으면 hc-fill-advise 사건에 키 hf:advise:agent:<초>를 달아
// 지휘 watch가 묶음으로 보낸다(cmd/hc-lane digest.go). 판단만·범위 밖이면 키 없이 — 지금처럼 즉시. 고치기 전 코드로는 키가 없어 실패한다

import (
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
)

func TestAdviseAgentKey(t *testing.T) {
	for _, c := range []struct {
		why   string
		extra []string
		key   bool
	}{
		{"조치 — 운영 에이전트가 맡음", []string{"ops_agent 조치"}, true},
		{"판단만 — 지금처럼 즉시", []string{"ops_agent 판단만"}, false},
		{"끔", nil, false},
		{"조치지만 범위 밖 프로젝트", []string{"ops_agent 조치", "ops_agent_scope 다른것"}, false},
	} {
		t.Run(c.why, func(t *testing.T) {
			oaFx(t, append([]string{"advise_every 5"}, c.extra...)...) // 설정은 앞 줄이 이긴다(oaFx 뒤의 advise_every 0보다 앞)
			adviseTick(time.Now())
			r := reports(t)
			if len(r) != 1 || !strings.Contains(r[0], "--name hc-fill-advise") {
				t.Fatalf("사건 한 번: %q", r)
			}
			if got := strings.Contains(r[0], "--key "+events.AdviseAgentKey); got != c.key {
				t.Fatalf("키 %v, 바람 %v: %s", got, c.key, r[0])
			}
		})
	}
}
