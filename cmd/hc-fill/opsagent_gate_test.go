package main

// ops-agent(2026-11-24): 운영 에이전트 실행이 부른 hc-fill 명령의 관문 — 지은 바이너리로(main의 나누기를 지나게).
// ④ 「판단만」 → 조치 명령이 안 먹고 일지에 「하려던 것」 ⑥ 에이전트 실행이 ops_agent* 손잡이를 바꾸려 하면 거절
// ⑦ 넘길 것 종류(레인 지우기)는 거절·「사람이 정한다」 사건 · 끝난 실행·읽는 명령·사람이 부른 명령.
// 고치기 전 코드로는 관문이 없어 prio·knob·rm이 그대로 먹어 실패한다

import (
	"os"
	"strings"
	"testing"
)

// gateKit: ehKit + 도는 실행 oa-1(주소 t9) 상태 파일 + 사건을 적는 가짜 hc-lane + 설정 ops_agent <mode>
func gateKit(t *testing.T, mode string) (fillKit, func() []string) {
	t.Helper()
	k := ehKit(t)
	bin := k.bin[:strings.LastIndexByte(k.bin, '/')]
	ehPut(t, bin+"/hc-lane", "#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+k.home+"/reports'\n", 0o755)
	if err := os.MkdirAll(k.data+"/ops-agent", 0o755); err != nil {
		t.Fatal(err)
	}
	ehPut(t, k.data+"/ops-agent/state.json", `{"run":{"id":"oa-1","addr":"t9","mode":"`+mode+`","deadline":9999999999},"keys":{}}`, 0o644)
	c, _ := os.ReadFile(k.home + "/conf/hc-fill.tsv")
	ehPut(t, k.home+"/conf/hc-fill.tsv", string(c)+"ops_agent "+mode+"\n", 0o644)
	k.env = append(k.env, oaEnvRun+"=oa-1")
	return k, func() []string {
		b, _ := os.ReadFile(k.home + "/reports")
		return lines(string(b))
	}
}

func queueOf(t *testing.T, k fillKit) string {
	b, err := os.ReadFile(k.data + "/queue.tsv")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ④ 판단만: prio가 안 먹고(대기열 그대로) 일지에 「하려던 것」, 종료 0. 읽는 명령은 그대로
func TestOpsGateDryJudge(t *testing.T) {
	k, _ := gateKit(t, "판단만")
	q := queueOf(t, k)
	out, se, rc := k.run(t, "prio", "lane-a", "낮음", "--why", "사슬 풀기")
	if rc != 0 || !strings.Contains(out, "판단만") || queueOf(t, k) != q {
		t.Fatalf("판단만 prio: %d %q %q\n%s", rc, out, se, queueOf(t, k))
	}
	js := ehJournal(t, k, "하려던 것")
	if len(js) != 1 || js[0]["target"] != "prio" || js[0]["why"] != "사슬 풀기" || !strings.Contains(js[0]["state"], "hc-fill prio lane-a 낮음") {
		t.Fatalf("일지: %+v", js)
	}
	if _, se, rc := k.run(t, "ls", "--json"); rc != 0 {
		t.Fatalf("읽는 명령: %d %q", rc, se)
	}
}

// 조치: 같은 명령이 먹는다(사람과 같은 명령)
func TestOpsGateAct(t *testing.T) {
	k, _ := gateKit(t, "조치")
	if _, se, rc := k.run(t, "prio", "lane-a", "낮음", "--why", "사슬 풀기"); rc != 0 || !strings.Contains(queueOf(t, k), "\tlow\t") {
		t.Fatalf("조치 prio: %d %q\n%s", rc, se, queueOf(t, k))
	}
}

// ⑥ 에이전트 실행(표식 또는 그 터미널)이 ops_agent* 손잡이를 바꾸려 하면 거절·사건 — 사람은 된다
func TestOpsGateOwnKnob(t *testing.T) {
	k, reps := gateKit(t, "판단만")
	_, se, rc := k.run(t, "knob", "set", "ops_agent", "조치", "--why", "넓힘")
	if rc != 1 || !strings.Contains(se, "사람에게 넘겼다") || fileHas(k.home+"/.local/state/herdr-ops/knobs.tsv", "ops_agent") {
		t.Fatalf("표식: %d %q", rc, se)
	}
	if r := reps(); len(r) != 1 || !strings.Contains(r[0], "막힘 - 사람이 정한다: 운영 에이전트(oa-1)가 「hc-fill knob set ops_agent 조치") || !strings.Contains(r[0], "--name hc-fill-agent") {
		t.Fatalf("사건: %q", r)
	}
	// 표식을 지워도 도는 실행의 터미널이면 막힌다
	k.env = append(k.env, oaEnvRun+"=", "HC_TERMINAL_ID=t9")
	if _, se, rc := k.run(t, "knob", "unset", "ops_agent_max_hour"); rc != 1 || !strings.Contains(se, "자기 손잡이") {
		t.Fatalf("터미널: %d %q", rc, se)
	}
	k.env = append(k.env, "HC_TERMINAL_ID=")
	if _, se, rc := k.run(t, "knob", "set", "ops_agent", "조치", "--why", "사람"); rc != 0 {
		t.Fatalf("사람: %d %q", rc, se)
	}
	// 다른 손잡이는 된다 — 까닭 글에 ops_agent가 있어도
	k.env = append(k.env, oaEnvRun+"=oa-1")
	if _, se, rc := k.run(t, "knob", "set", "idle", "20", "--why", "ops_agent 판단: 알림이 잦음"); rc != 0 {
		t.Fatalf("다른 손잡이: %d %q", rc, se)
	}
}

// ⑦ 레인 지우기는 어느 방식이든 거절·사건, 대기열 그대로. 끝난 실행(다른 ID)의 조치도 거절
func TestOpsGateHandoffAndStale(t *testing.T) {
	k, reps := gateKit(t, "조치")
	q := queueOf(t, k)
	if _, se, rc := k.run(t, "rm", "lane-a", "--why", "치움"); rc != 1 || queueOf(t, k) != q || !strings.Contains(se, "레인 지우기") {
		t.Fatalf("rm: %d %q", rc, se)
	}
	if r := reps(); len(r) != 1 || !strings.Contains(r[0], "사람이 정한다") {
		t.Fatalf("사건: %q", r)
	}
	if js := ehJournal(t, k, "거절"); len(js) != 1 || js[0]["target"] != "rm" {
		t.Fatalf("일지: %+v", js)
	}
	k.env = append(k.env, oaEnvRun+"=oa-old")
	if _, se, rc := k.run(t, "prio", "lane-a", "낮음"); rc != 1 || queueOf(t, k) != q || !strings.Contains(se, "끝났다") {
		t.Fatalf("끝난 실행: %d %q", rc, se)
	}
}

func fileHas(p, s string) bool {
	b, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(b), s)
}
