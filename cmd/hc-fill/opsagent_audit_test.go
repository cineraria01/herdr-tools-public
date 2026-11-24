package main

// ops-agent 감사 atozu10-ln5gn1 고침의 시험.
// [중간] --dry-run을 모르는 명령(nudge·retry)에 그 글자를 붙이면 관문을 「읽기」로 지나 본 동작을 했다 — 판단만이면 「하려던 것」으로 끝나야.
// [낮음] 다른 기계 주소(기계/tN)로 띄운 실행이 도는 동안 이 기계의 같은 ID 사람 터미널을 에이전트로 봤다.
// [낮음] 대화형 CLI가 결과 파일 없이 턴을 끝내면(상태 idle) 상한까지 「도는 중」이었다.
// 고치기 전 코드로는 셋 다 실패한다

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpsGateDryRunOnlyForDryCommands(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	for _, args := range [][]string{{"nudge", "lane-a", "--dry-run"}, {"retry", "lane-a", "--why", "--dry-run"}} {
		out, se, rc := k.run(t, args...)
		if rc != 0 || !strings.Contains(out, "판단만") {
			t.Fatalf("%v: 판단만으로 끝나야 — %d %q %q", args, rc, out, se)
		}
	}
	if js := ehJournal(t, k, "하려던 것"); len(js) != 2 || js[0]["target"] != "nudge" || js[1]["target"] != "retry" {
		t.Fatalf("일지: %+v", js)
	}
	// 실제 미리 보기는 그대로 읽기(관문을 안 거침 — 판단만 글 없음)
	if out, se, rc := k.run(t, "prio", "lane-a", "낮음", "--dry-run"); rc != 0 || strings.Contains(out, "판단만") {
		t.Fatalf("prio --dry-run: %d %q %q", rc, out, se)
	}
}

func TestOpsGateRemoteAddrNotLocalTerminal(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	ehPut(t, k.data+"/ops-agent/state.json", `{"run":{"id":"oa-1","addr":"box/t9","mode":"판단만","deadline":9999999999},"keys":{}}`, 0o644)
	k.env = append(k.env, oaEnvRun+"=", "HC_TERMINAL_ID=t9")
	if _, se, rc := k.run(t, "knob", "set", "ops_agent", "끔", "--why", "사람이 끔"); rc != 0 {
		t.Fatalf("이 기계의 같은 ID 사람 터미널: %d %q", rc, se)
	}
}

func TestOpsAgentTurnEndedNoResult(t *testing.T) {
	oaFx(t, "ops_agent 조치")
	if err := os.WriteFile(DATA+"/hc", []byte("#!/bin/sh\ncase \"$1\" in ls) cat '"+DATA+"/lsjson';; machine) cat '"+DATA+"/mls';; get) cat '"+DATA+"/getjson';; esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DATA+"/getjson", []byte(`{"address":"t9","state":"live","status":"idle","cwd":"/w","name":"hc-fill-agent","created_ms":1700000000000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	opsAgentTick(now)
	opsAgentTick(now.Add(time.Minute)) // 띄운 직후의 idle은 끝이 아니다
	if st, _ := oaLoad(); st.Run == nil {
		t.Fatal("띄운 직후 idle을 끝으로 봄")
	}
	opsAgentTick(now.Add(3 * time.Minute))
	st, _ := oaLoad()
	if st.Run != nil || len(st.Recent) != 1 || st.Recent[0].Result != "턴 끝남(결과 없음)" {
		t.Fatalf("턴 끝남: %+v", st)
	}
}
