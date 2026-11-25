package main

import (
	"os"
	"strings"
	"testing"
)

// ht-send-target-live(2026-11-25): 띄울 때 질문 넘기기 키(hcKeys)도 받는 쪽에 에이전트가 있을 때만 — 셸만 남은 터미널·서비스 터미널에는
// 보내지 않는다. hc get 실패는 지금처럼 보낸다
func TestHcKeysTarget(t *testing.T) {
	for _, c := range []struct {
		name, get string
		sent      bool
	}{
		{"에이전트가 맨 위", `printf '{"address":"t1","argv":["claude"],"created_ms":1,"cwd":"/r","foreground":{"argv":["claude"],"pid":5},"name":"w","pid":5,"state":"live","status":null}\n'`, true},
		{"셸만 남음", `printf '{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-l"],"pid":5},"name":"w","pid":5,"state":"live","status":null}\n'`, false},
		{"프록시 서버", `printf '{"address":"t1","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["/x/teamx","server"],"pid":7},"name":"p","pid":5,"state":"live","status":null}\n'`, false},
		{"hc get 실패", `echo boom >&2; exit 1`, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			log := d + "/calls.log"
			if err := os.WriteFile(d+"/hc", []byte("#!/bin/sh\nprintf 'hc %s\\n' \"$*\" >>'"+log+"'\ncase $1 in get) "+c.get+";; esac\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", d+":"+os.Getenv("PATH"))
			got := hcKeys("t1", "down", "enter")
			b, _ := os.ReadFile(log)
			sent := strings.Contains(string(b), "hc keys t1 down enter")
			if got != c.sent || sent != c.sent {
				t.Fatalf("hcKeys %v, 보냄 %v — 바람 %v: %q", got, sent, c.sent, b)
			}
		})
	}
}
