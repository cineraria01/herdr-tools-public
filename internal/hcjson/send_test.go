package hcjson

import (
	"errors"
	"strings"
	"testing"

	"herdr-tools/internal/tri"
)

// 받는 쪽 판정 한 곳(ht-send-target-live): 견본은 cmd/hc-lane/testdata/agent(2026-11-23 실측)와 같은 꼴
const (
	sendWorking = `{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["node","/x/claude"],"pid":61000},"name":"w","pid":50980,"state":"live","status":{"app":"claude-code","state":"working"}}`
	sendIdleTop = `{"address":"t1","argv":["claude"],"created_ms":1,"cwd":"/r","foreground":{"argv":["claude"],"pid":50980},"name":"w","pid":50980,"state":"live","status":{"app":"claude-code","state":"idle"}}`
	sendShell   = `{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-l"],"pid":50980},"name":"w","pid":50980,"state":"live","status":null,"status_ms":1}`
	sendZshC    = `{"address":"t1","argv":["zsh","-c","claude x"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-c","claude x"],"pid":50980},"name":"w","pid":50980,"state":"live","status":null}`
	sendServer  = `{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["/x/teamx","server"],"pid":61000},"name":"p","pid":50980,"state":"live","status":null}`
	sendWatch   = `{"address":"t1","argv":["hc-lane","watch"],"created_ms":1,"cwd":"/r","labels":{"hc-lane-watch":"1"},"name":"watch","pid":50980,"state":"live","status":null}`
	sendOdd     = `{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-l"],"pid":50980},"name":"w","pid":50980,"state":"live","status":{"app":"claude-code","state":"idle"}}`
	sendDone    = `{"address":"t1","created_ms":1,"name":"w","pid":50980,"state":"finished","status":null}`
)

func TestSendTarget(t *testing.T) {
	for _, c := range []struct {
		name, js string
		want     tri.State
		why      string
	}{
		{"에이전트 도는 중", sendWorking, tri.Yes, ""},
		{"에이전트가 맨 위(idle)", sendIdleTop, tri.Yes, ""},
		{"셸만 남음", sendShell, tri.No, "셸만 남음"},
		{"zsh -c 꼴(자식이 돌아도 전경 pid = 셸)", sendZshC, tri.Yes, ""},
		{"프록시 서버", sendServer, tri.No, "서버 터미널"},
		{"감시 라벨", sendWatch, tri.No, "hc-lane-watch"},
		{"셸이 전경인데 status 남음", sendOdd, tri.Unknown, "status"},
		{"끝난 터미널(hc send가 스스로 실패)", sendDone, tri.Yes, ""},
	} {
		tm, found, err := ParseGet([]byte(c.js), 0, "")
		if err != nil || !found {
			t.Fatalf("%s: 견본을 못 읽음 %v %v", c.name, found, err)
		}
		st, why := SendTarget(tm)
		if st != c.want || !strings.Contains(why, c.why) {
			t.Errorf("%s: %v %q — 바람 %v %q", c.name, st, why, c.want, c.why)
		}
		// SendBlocked는 없음만 막는다 — 모름·있음은 ""(도구의 지금 동작 그대로)
		if b := SendBlocked(tm, true, nil); (b != "") != (c.want == tri.No) {
			t.Errorf("%s: SendBlocked %q", c.name, b)
		}
	}
	tm, _, _ := ParseGet([]byte(sendShell), 0, "")
	if SendBlocked(tm, true, errors.New("hc get 시간 넘김")) != "" || SendBlocked(tm, false, nil) != "" {
		t.Error("조회 실패·없는 터미널은 막지 않는다(지금 동작 유지)")
	}
}
