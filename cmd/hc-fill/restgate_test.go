package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ht-rest-one-gate: hc-fill이 터미널을 닫거나 레인이 손을 뗀 것으로 치는 길(주차·반송 닫기 endClose, 감사 터미널 닫기 aCloseTerm,
// 운영 에이전트 끝·닫기 oaFinish·oaClose, 대기열 제출 aAutoSubmit)은 상태 done·idle에 더해 그 기계의 쉼 판정(hc-sweep --rest — 가짜는
// restFake)을 거친다. 활동·모름이면 조치하지 않고, 쉼이 되면 다음 바퀴에 한 번. 고치기 전 판은 상태만 보고 닫고 제출했다

const (
	restBusyLine = "REST\tbusy\tsid-1\t배경 작업 1개 도는 중\tend" // 끝난 하위 에이전트가 남긴 배경 셸(사고 꼴)
	restOldLine  = "flag provided but not defined: -rest"  // 옛 판 hc-sweep(답 줄 없음 = 모름)
)

// restSay: 가짜 hc-sweep(restFake, dir)이 그 터미널에 낼 답(""이면 쉼으로 되돌림)
func restSay(t *testing.T, dir, term, line string) {
	t.Helper()
	p := dir + "/rest-" + term
	if line == "" {
		os.Remove(p)
		return
	}
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func restAsked(dir, term string) int {
	b, _ := os.ReadFile(dir + "/rest.log")
	return grepN(lines(string(b)), term)
}

// 주차 터미널 닫기: 활동·모름(옛 판 워커)이면 안 닫고 까닭, 쉼이 되면 다음 바퀴에 닫음. 다시 돌려도 또 닫거나 묻지 않음(이미 없음),
// hc-sweep이 먼저 닫았으면 묻지도 않고 「이미 없음」
func TestRestGateParkClose(t *testing.T) {
	for _, c := range []struct{ line, why string }{{restBusyLine, "안에서 일이 도는 중(배경 작업 1개 도는 중)"}, {restOldLine, "쉼 모름(묻기 답 꼴이 틀림"}} {
		k := pcSetup(t, "done", "주차", "wait", "parked")
		restSay(t, DATA, "t1", c.line)
		if pc, why := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 2 || !strings.Contains(why, c.why) || k.pcKills() != 0 {
			t.Fatalf("%s: 안 닫고 기다림(2)·까닭이어야: %d %q kill %d", c.line, pc, why, k.pcKills())
		}
		restSay(t, DATA, "t1", "")
		if pc, _ := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 1 || k.pcKills() != 1 {
			t.Fatalf("%s → 쉼: 다음 바퀴에 닫아야: %d kill %d", c.line, pc, k.pcKills())
		}
		asked := restAsked(DATA, "t1")
		if pc, _ := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 1 || k.pcKills() != 1 || restAsked(DATA, "t1") != asked {
			t.Fatalf("두 번째 실행: 또 닫거나 물음 %d kill %d", pc, k.pcKills())
		}
	}
	k := pcSetup(t, "done", "주차", "wait", "parked")
	k.wr("terms.tsv", "") // hc-sweep이 먼저 닫음
	if pc, why := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 1 || k.pcKills() != 0 || restAsked(DATA, "t1") != 0 {
		t.Fatalf("이미 없음: %d %q kill %d", pc, why, k.pcKills())
	}
}

// 감사 터미널 닫기(판정을 넘긴 바퀴): 안에서 일이 돌면 그 바퀴는 안 닫고, 쉼이 된 다음 바퀴에 닫음
func TestRestGateAuditClose(t *testing.T) {
	for _, line := range []string{restBusyLine, restOldLine} {
		k := clsSetup(t, "done")
		restSay(t, DATA, "audit-a1", line)
		k.round()
		if k.kills() != 0 || restAsked(DATA, "audit-a1") == 0 {
			t.Fatalf("%s: 묻고 안 닫아야: kill %d\n%s", line, k.kills(), k.read("hc.log"))
		}
		restSay(t, DATA, "audit-a1", "")
		k.round()
		if k.kills() != 1 {
			t.Fatalf("%s → 쉼: 다음 바퀴에 닫기 %d번, 바람 1", line, k.kills())
		}
	}
}

// 운영 에이전트: 턴이 끝났어도(done) 안에서 일이 돌면 끝으로 치지 않고(다음 실행도 안 띄움) 안 닫음, 쉼이 되면 끝·닫기 한 번.
// 결과 파일로 끝났어도 닫기는 쉼일 때만(남기면 다음 띄우기 직전에 다시)
func TestRestGateOpsAgent(t *testing.T) {
	orFx(t)
	now := time.Now()
	opsAgentTick(now)
	orStatus(t, "done")
	restSay(t, DATA, "t1", restBusyLine)
	opsAgentTick(now.Add(3 * time.Minute))
	if st, _ := oaLoad(); st.Run == nil || len(orHc(t, "kill")) != 0 {
		t.Fatalf("활동: 끝으로 치지도 닫지도 않아야: %+v kill %q", st.Run, orHc(t, "kill"))
	}
	restSay(t, DATA, "t1", "")
	opsAgentTick(now.Add(4 * time.Minute))
	if st, _ := oaLoad(); st.Run != nil || len(orHc(t, "kill")) != 1 {
		t.Fatalf("쉼: 끝·닫기 한 번: %+v kill %q", st.Run, orHc(t, "kill"))
	}
	st, _ := oaLoad()
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t5\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restSay(t, DATA, "t5", restOldLine)
	oaClose(&st, "t5", "x")
	if st.Left != "t5" || len(orHc(t, "kill")) != 1 {
		t.Fatalf("모름: 안 닫고 다음 띄우기 직전에 다시(Left): %q kill %q", st.Left, orHc(t, "kill"))
	}
}

// 대기열 제출: 활동·모름이면 제출하지 않고 지금처럼 깨운다(거짓 — 레인이 스스로 받는다, 깨우기 간격 그대로, 글에 까닭 — ht-autosubmit-busy-wake).
// 쉼이 된 바퀴에 제출 한 번 — 다시 돌려도 또 제출하지 않음. 원격 레인은 그 기계에서 묻는다(hc exec --on)
func TestRestGateAutoSubmit(t *testing.T) {
	for _, m := range []string{"m", "r"} {
		k := asTSetup(t, m, "idle", "app")
		for _, c := range []struct{ line, why string }{{restBusyLine, "안에서 일이 도는 중(배경 작업 1개 도는 중)"}, {restOldLine, "쉼 모름("}} {
			restSay(t, k.dir, "w1", c.line)
			if aAutoSubmit("main") || strings.Contains(k.logged(), "land ") || inlist("asub", auAfl) || asNote != "" || !strings.Contains(asRest, c.why) {
				t.Fatalf("%s %s: 제출 없이 지금처럼 깨움(거짓, 간격 그대로, 까닭 글): %s %s %q %q", m, c.line, k.logged(), auAfl, asNote, asRest)
			}
		}
		restSay(t, k.dir, "w1", "")
		if !aAutoSubmit("main") || strings.Count(k.logged(), "land land main --verdict a1 ") != 1 {
			t.Fatalf("%s 쉼: 제출 한 번: %s", m, k.logged())
		}
		aAutoSubmit("main")
		if strings.Count(k.logged(), "land land main --verdict a1 ") != 1 || restAsked(k.dir, "w1") != 3 {
			t.Fatalf("%s 두 번째 실행: 또 제출하거나 물음(물음 %d번): %s", m, restAsked(k.dir, "w1"), k.logged())
		}
	}
}
