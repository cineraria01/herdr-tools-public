package hcjson

import (
	"encoding/json"

	"herdr-tools/internal/tri"
)

// 받는 쪽 판정 한 곳(ht-send-target-live, 2026-11-25): hc send·keys를 보내기 전에 받는 터미널에 에이전트가 있나. herdr-hooks(지휘의
// Bash hc send 막기)와 Go 도구가 직접 부르는 hc send·keys(hc-fill 감사 깨움·nudge·멈춤 끊기, 병합기 알림·신뢰 질문, hc-lane 감시·send·
// 띄우기 질문 넘기기)가 함께 쓴다. 셸만 남은 터미널에 보낸 글은 셸이 명령으로 실행하고, 서비스 터미널에 보낸 키는 서버를 끊는다.

// SendTarget: 받는 쪽 t(hc get 한 번)에 보내도 되나 — 있음(보냄), 없음(까닭 — 서비스 터미널·에이전트가 끝나 셸만 남음), 모름(까닭).
// live가 아닌 터미널은 있음 — hc send가 스스로 실패한다
func SendTarget(t Terminal) (tri.State, string) {
	if t.State != "live" {
		return tri.Yes, ""
	}
	if why := ServiceTerm(t); why != "" {
		return tri.No, why
	}
	return AgentState(t)
}

// SendBlocked: hc get 결과(t, found, err)로 보내지 말아야 하면 까닭, 보내도 되면 "". 조회 실패(err)·없는 터미널·판정 모름은 ""
// — 도구의 지금 동작(보냄)을 그대로 둔다(셸만 남음은 확인된 경우만 막는다. 모름까지 막으면 막 뜬 레인·옛 hc에서 깨움이 끊긴다)
func SendBlocked(t Terminal, found bool, err error) string {
	if err != nil || !found {
		return ""
	}
	if st, why := SendTarget(t); st == tri.No {
		return why
	}
	return ""
}

// ServiceTerm: 에이전트가 아닌 서비스 터미널이면 까닭 — 이 저장소 도구의 서비스 라벨(hc-lane-watch·hc-merger-spec), 또는 상태 보고가
// 한 번도 없고(status·status_ms 없음) 전경 argv가 `… server`(계정 풀 프록시 꼴, 2026-11-25 실측). 막 뜬 에이전트는 상태 보고 전이라도
// 전경이 `… server`가 아니라 여기 걸리지 않는다
// ponytail: 서버 꼴은 argv 끝 낱말 하나로 가른다 — 다른 꼴의 서비스가 생기면 그 표식을 여기에 더한다
func ServiceTerm(t Terminal) string {
	for _, l := range []string{"hc-lane-watch", "hc-merger-spec"} {
		if t.Labels[l] == "1" {
			return "에이전트가 아닌 서비스 터미널이다(라벨 " + l + ")"
		}
	}
	if r, ok := t.Raw["status_ms"]; t.Status != "" || ok && string(r) != "null" {
		return ""
	}
	var fg struct {
		Argv []string `json:"argv"`
	}
	if r, ok := t.Raw["foreground"]; !ok || json.Unmarshal(r, &fg) != nil || len(fg.Argv) < 2 || fg.Argv[len(fg.Argv)-1] != "server" {
		return ""
	}
	return "에이전트가 아닌 서버 터미널이다(상태 보고 없음, 전경이 … server)"
}
