package main

import (
	"os"
	"strings"
	"testing"
)

// ht-park-close-term(2026-11-26): 주차한 레인의 터미널이 정리까지 중앙 33분 남아, 선행이 착지해도 hc-lane --resume이 「살아 있는 hc
// 터미널이 이미 이름 … 을 쓴다」로 거절했다(13바퀴, 바퀴 기록엔 「자리 없음」). 대기열은 주차 보고를 받으면 그 터미널을 바로 닫고,
// 이어 띄우기 전에 남은 같은 이름 주차 터미널을 닫는다 — 못 닫으면 띄우지 않고 까닭을 적는다. 가짜 hc는 auditclose_test의 것(terms.tsv).

// pcSetup: 레인 pk(주소 m/t1, 실행 t1@1700000000000)의 터미널 상태와 마지막 보고, 대기열 줄 상태(up·early 또는 wait·parked)
func pcSetup(t *testing.T, status, report, state, early string) *clsT {
	k := clsSetup(t, "")
	k.wr("terms.tsv", "m/t1\tpk\t1700000000000\t"+status+"\t"+k.wt+"\n")
	TERMS = []Term{{"m", "m/t1", "claude-code", "pk", status, k.wt}}
	addr := "m/t1"
	if state == "wait" {
		addr = "-"
	}
	k.wr("queue.tsv", strings.Join([]string{"pk", "app", "b", "after-x", "-", state, "m", addr, "-", "t", "-", "-", "normal", early, "-", "-", "r1"}, "\t")+"\n")
	k.wr("try.tsv", "pk\tr1\tm\tearly\t-\tm/t1\tt1@1700000000000\n")
	EV = []string{auTEv("2026-11-26T01:00:00Z", "m/t1", "pk", report, auTSha[:8], "준비", "t1@1700000000000"),
		auTEv("2026-11-26T01:01:00Z", "m/t1", "pk", "정리됨", "-", "hc-lane clean", "t1@1700000000000")}
	os.Remove(k.d + "/hc.log")
	return k
}

func (k *clsT) pcKills() int { return strings.Count(k.read("hc.log"), "kill m/t1\n") }

// ① 주차 보고 + 터미널 done: 줄을 대기·주차로 되돌리고 그 터미널을 바로 닫는다(고치기 전: 닫기 0번 — hc-sweep 30분을 기다렸다)
func TestParkTickClosesTerm(t *testing.T) {
	for _, st := range []string{"done", "idle"} {
		k := pcSetup(t, st, "주차", "up", "early")
		parkTick()
		if r := qrow("pk"); r == nil || r.F(qState) != "wait" || r.F(qEarly) != "parked" {
			t.Fatalf("%s: 줄이 대기·주차여야: %v", st, r)
		}
		if k.pcKills() != 1 || strings.Contains(k.read("terms.tsv"), "m/t1") {
			t.Fatalf("%s: 주차한 터미널을 닫아야(kill 1번)\n%s", st, k.read("hc.log"))
		}
	}
}

// ② 주차 터미널이 남은 채 선행 착지 → 이어 띄우기 전 판정이 닫고 「건너뜀(닫혀 없음)」 — 띄우기로 간다(고치기 전: 닫지 않고 띄워 거절)
func TestParkCandidateClosesThenResumes(t *testing.T) {
	k := pcSetup(t, "done", "주차", "wait", "parked")
	if pc, why := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 1 || why != "" {
		t.Fatalf("닫고 띄우기로 가야(1): %d %q", pc, why)
	}
	if k.pcKills() != 1 {
		t.Fatalf("같은 이름 주차 터미널을 닫아야\n%s", k.read("hc.log"))
	}
}

// ③ 같은 이름인데 일하는 터미널·마지막 보고가 주차 아님: 닫지 않고, 띄우지 않으며 까닭 한 줄(「같은 이름 주차 터미널 살아 있음 <주소>」)
func TestParkCandidateKeepsBusy(t *testing.T) {
	for _, c := range []struct{ status, report, why string }{{"working", "주차", "상태 working"}, {"done", "중간", "마지막 보고가 주차 아님"}} {
		k := pcSetup(t, c.status, c.report, "wait", "parked")
		pc, why := parkCandidate("pk", "r1", "m", "m/t1", c.status)
		if pc != 2 || !strings.Contains(why, "같은 이름 주차 터미널 살아 있음 m/t1") || !strings.Contains(why, c.why) {
			t.Fatalf("%s·%s: 기다림(2)과 까닭이어야: %d %q", c.status, c.report, pc, why)
		}
		if k.pcKills() != 0 {
			t.Fatalf("%s·%s: 닫으면 안 된다\n%s", c.status, c.report, k.read("hc.log"))
		}
	}
}

// hc keep이 셸만 남긴 주차 터미널: 가짜 get은 실측 꼴(hc 0.1.22 — status null·keep·맨 위 pid·argv·foreground{name,pid,argv}, agent·
// self 칸 없음). 전경이 맨 위 셸 자신이면 닫고, 셸에서 무엇이 돌거나(전경 pid 다름) keep이 아니면 안 닫는다(감사 atp41h9-ie3gcc·
// atp42ip-3tgv31 중간 1 — hc get에 없는 agent·self 칸으로 봐서 이 경우를 늘 「상태 -」로 남겼다)
func TestParkCloseKeepShell(t *testing.T) {
	sh := `{"argv":["/bin/zsh","-l"],"cwd":"/w","name":"zsh","pid":40170}`
	for _, c := range []struct {
		keep, fg string
		want     int
	}{{"true", sh, 1}, {"true", `{"argv":["sleep","9"],"cwd":"/w","name":"sleep","pid":40171}`, 2}, {"false", sh, 2}} {
		k := pcSetup(t, "", "주차", "wait", "parked")
		k.wr("get.json", `{"address":"m/t1","name":"pk","created_ms":1700000000000,"state":"live","cwd":"`+k.wt+`","status":null,"keep":`+c.keep+
			`,"argv":["/bin/zsh","-l"],"pid":40170,"foreground":`+c.fg+`}`)
		k.wr("hc", "#!/bin/sh\nd='"+k.d+"'\nprintf '%s\\n' \"$*\" >>\"$d/hc.log\"\ncase \"$1\" in\nget) cat \"$d/get.json\";;\nkill) ;;\n*) exit 2;;\nesac\n")
		if pc, why := parkCandidate("pk", "r1", "m", "m/t1", ""); pc != c.want || k.pcKills() != c.want%2 {
			t.Fatalf("keep %s fg %s: %d(바람 %d) kill %d %q", c.keep, c.fg, pc, c.want, k.pcKills(), why)
		}
	}
}

// ④ 닫기 실패: 주차 되돌리기는 그대로, 터미널은 남김 한 줄 — 다음 바퀴(이어 띄우기 전 판정)에 다시 닫는다
func TestParkCloseRetryNextTick(t *testing.T) {
	k := pcSetup(t, "done", "주차", "up", "early")
	k.wr("killfail", "")
	parkTick()
	if r := qrow("pk"); r == nil || r.F(qEarly) != "parked" || !strings.Contains(k.read("terms.tsv"), "m/t1") {
		t.Fatalf("닫기 실패여도 줄은 주차, 터미널은 남아야: %v", r)
	}
	if pc, why := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 2 || !strings.Contains(why, "hc kill") {
		t.Fatalf("닫기 실패 동안은 기다림과 까닭: %d %q", pc, why)
	}
	os.Remove(k.d + "/killfail")
	if pc, _ := parkCandidate("pk", "r1", "m", "m/t1", "done"); pc != 1 || strings.Contains(k.read("terms.tsv"), "m/t1") {
		t.Fatalf("다음 바퀴에 다시 닫아야: %d\n%s", pc, k.read("hc.log"))
	}
}
