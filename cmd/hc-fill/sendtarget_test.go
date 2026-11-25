package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/journal"
)

// ht-send-target-live(2026-11-25): hc-fill이 직접 부르는 hc send(auSend — 감사 깨움·다시 맞춤·nudge·멈춤 끊기)도 받는 쪽에 에이전트가
// 있을 때만. 셸만 남은 터미널·서비스 터미널은 보내지 않고(기존 「못 보냄」 길) 일지에 까닭을 남긴다. hc get 실패는 지금처럼 보낸다
// 막힌 받는 쪽에 바퀴마다 다시 깨워도 send-skip 일지는 같은 주소에 한 시간에 한 줄(감사 atp22qf 낮음 1 — 하루 1440줄까지 쌓였다)
func TestAuSendSkipJotOnce(t *testing.T) {
	setData(t)
	sc := "#!/bin/sh\ncase $1 in\nget) printf '{\"address\":\"m/w\",\"argv\":[\"zsh\",\"-l\"],\"created_ms\":1,\"cwd\":\"/r\",\"foreground\":{\"argv\":[\"zsh\",\"-l\"],\"pid\":5},\"name\":\"w\",\"pid\":5,\"state\":\"live\",\"status\":null}\\n';;\nesac\nexit 0\n"
	if err := os.WriteFile(DATA+"/hc", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	t.Setenv(journal.Env, DATA+"/journal.tsv")
	for range 3 {
		if auSend("m/w", "hc-fill: 감사 판정이 왔다") {
			t.Fatal("셸만 남음에 보냄")
		}
	}
	if n, err := journal.Repeats("send-skip", "m/w", time.Now().Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("send-skip 일지 %d줄(%v) — 한 줄이어야", n, err)
	}
}

func TestAuSendTarget(t *testing.T) {
	for _, c := range []struct {
		name, get string // get: 가짜 hc get 본문(셸)
		sent      bool
		jot       string
	}{
		{"에이전트 working", `printf '{"address":"m/w","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["node","claude"],"pid":7},"name":"w","pid":5,"state":"live","status":{"app":"claude-code","state":"working"}}\n'`, true, ""},
		{"에이전트 idle(맨 위)", `printf '{"address":"m/w","argv":["claude"],"created_ms":1,"cwd":"/r","foreground":{"argv":["claude"],"pid":5},"name":"w","pid":5,"state":"live","status":{"app":"claude-code","state":"idle"}}\n'`, true, ""},
		{"셸만 남음", `printf '{"address":"m/w","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-l"],"pid":5},"name":"w","pid":5,"state":"live","status":null}\n'`, false, "셸만 남음"},
		{"서비스 터미널", `printf '{"address":"m/w","argv":["hc-lane","watch"],"created_ms":1,"cwd":"/r","labels":{"hc-lane-watch":"1"},"name":"w","pid":5,"state":"live","status":null}\n'`, false, "서비스 터미널"},
		{"hc get 실패 — 지금처럼 보냄", `echo boom >&2; exit 1`, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			setData(t)
			log := DATA + "/calls.log"
			sc := "#!/bin/sh\nprintf 'hc %s\\n' \"$*\" >>'" + log + "'\ncase $1 in\nget) " + c.get + ";;\nesac\nexit 0\n"
			if err := os.WriteFile(DATA+"/hc", []byte(sc), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
			t.Setenv(journal.Env, DATA+"/journal.tsv")
			got := auSend("m/w", "hc-fill: 감사 판정이 왔다")
			b, _ := os.ReadFile(log)
			sent := strings.Contains(string(b), "hc send m/w ")
			if got != c.sent || sent != c.sent {
				t.Fatalf("auSend %v, 보냄 %v — 바람 %v: %q", got, sent, c.sent, b)
			}
			j, _ := os.ReadFile(DATA + "/journal.tsv")
			if c.jot != "" && (!strings.Contains(string(j), c.jot) || !strings.Contains(string(j), "m/w")) {
				t.Fatalf("일지에 까닭(%s)이 없다: %q", c.jot, j)
			}
			if c.jot == "" && strings.Contains(string(j), "보내지 않음") {
				t.Fatalf("보낸 경우에 안 보냄 일지: %q", j)
			}
		})
	}
}
