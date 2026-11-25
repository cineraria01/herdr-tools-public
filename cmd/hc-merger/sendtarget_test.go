package main

import (
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/hcjson"
)

// ht-send-target-live(2026-11-25): 병합기 → 레인 알림(hc send)도 받는 쪽에 에이전트가 있을 때만. 셸만 남았으면 보내지 않고
// 기존 「못 보냄」 길(.try — 세 번 뒤 사건만)으로. 이미 받은 hc get 응답으로 가른다(hc get을 더 부르지 않는다)
func TestSendSkipsShellOnlyLane(t *testing.T) {
	for _, c := range []struct {
		name, fg string
		sent     bool
	}{
		{"에이전트 도는 중", `"foreground":{"argv":["node","claude"],"pid":7},"status":{"app":"claude-code","state":"working"}`, true},
		{"셸만 남음", `"foreground":{"argv":["zsh","-l"],"pid":5},"status":null`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFx(t)
			m := f.merger()
			bin := t.TempDir()
			log := bin + "/calls.log"
			js := `{"address":"t9","argv":["zsh","-l"],"created_ms":123,"cwd":"/r","name":"y","pid":5,"state":"live",` + c.fg + `}`
			os.WriteFile(bin+"/get.json", []byte(js+"\n"), 0o666)
			os.WriteFile(bin+"/hc", []byte("#!/bin/sh\nprintf 'hc %s\\n' \"$1\" >>'"+log+"'\ncase $1 in get) cat '"+bin+"/get.json' ;; esac\nexit 0\n"), 0o777)
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			ob := m.mj + "/out/1000000003-y"
			os.WriteFile(ob, []byte("착지\n-\n알림 시험\ny\n-\nbox\nt9\n\n\n123\n-\n"), 0o666)
			m.send("1000000003-y")
			b, _ := os.ReadFile(log)
			calls := string(b)
			if strings.Count(calls, "hc get") != 1 {
				t.Fatalf("hc get은 한 번: %q", calls)
			}
			if got := strings.Contains(calls, "hc send"); got != c.sent {
				t.Fatalf("보냄 %v — 바람 %v: %q", got, c.sent, calls)
			}
			if c.sent {
				return
			}
			tr, err := os.ReadFile(ob + ".try")
			if err != nil || strings.TrimSpace(string(tr)) != "1" {
				t.Fatalf("못 보냄 길(.try=1)로 가야: %q %v", tr, err)
			}
		})
	}
}

// 신뢰 질문 키(rtrust)도 이미 받은 hc get으로 — 셸만 남은 풀이 터미널에는 키를 보내지 않는다
func TestRtrustSkipsShellOnly(t *testing.T) {
	bin := t.TempDir()
	log := bin + "/calls.log"
	scr := "Accessing workspace:\n\n /x/y\n\n Is this a project you created or one you trust?\n\n ❯ No, exit\n   Yes, I trust this folder\n"
	os.WriteFile(bin+"/scr.txt", []byte(scr), 0o666)
	os.WriteFile(bin+"/hc", []byte("#!/bin/sh\nprintf 'hc %s\\n' \"$*\" >>'"+log+"'\ncase $1 in read) cat '"+bin+"/scr.txt';; esac\nexit 0\n"), 0o777)
	os.WriteFile(bin+"/herdr-tools-config", []byte("#!/bin/sh\necho on\n"), 0o777)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	og := gSelf
	gSelf = bin + "/hc-merger"
	defer func() { gSelf = og }()
	tm, _, err := hcjson.ParseGet([]byte(`{"address":"t9","argv":["zsh","-l"],"created_ms":1,"cwd":"/r","foreground":{"argv":["zsh","-l"],"pid":5},"name":"r","pid":5,"state":"live","status":null}`), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	m := &M{}
	m.rtrust(&rstate{addr: "t9", dir: bin}, tm)
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "hc keys") {
		t.Fatalf("셸만 남은 터미널에 키를 보냈다: %q", b)
	}
}
