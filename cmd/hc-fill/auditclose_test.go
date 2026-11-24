package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"herdr-tools/internal/hcjson"
)

// fill-audit-close(2026-11-24): 대기열이 판정을 수거해 레인에 넘긴(deliver — 판정 파일을 두고 깨움) 바퀴에, 자기가 띄운 그 감사 터미널을
// 닫는다 — 레인이 판정을 받자마자 착지·hc-lane clean으로 가도 끝난 감사 터미널이 작업 트리를 쥐고 있지 않게(하루 네 번 「clean 멈춤」).
// 가짜 hc는 terms.tsv(주소 이름 created_ms 상태 cwd) 하나로 get·kill·send·ls를 흉내 낸다.

type clsT struct {
	t      *testing.T
	d, wt  string
	status string // 감사 터미널 상태(TERMS·hc get)
}

func clsSetup(t *testing.T, status string) *clsT {
	setData(t)
	t.Setenv("HOME", DATA)
	k := &clsT{t: t, d: DATA, wt: DATA + "/repo.wt/w1", status: status}
	if err := os.MkdirAll(k.wt, 0o755); err != nil {
		t.Fatal(err)
	}
	ol, od, oc, oa, oe, os_, ot, ocf, opr := LOCAL, dry, CONN, AUNK, EVBAD, LSSNAP, TERMS, confLines, pcRef
	t.Cleanup(func() {
		LOCAL, dry, CONN, AUNK, EVBAD, LSSNAP, TERMS, confLines, pcRef = ol, od, oc, oa, oe, os_, ot, ocf, opr
	})
	LOCAL, dry, CONN, AUNK, EVBAD, LSSNAP = "m", false, nil, nil, false, false
	WAKE, ACKMAX, AMAX = 10, 30, 2
	confLines = []string{"repo app m " + DATA + "/repo"}
	pcRef = map[string]pcRes{DATA + "/repo": {"origin/main", true}}
	k.wr("hc", "#!/bin/sh\nd='"+DATA+"'\nprintf '%s\\n' \"$*\" >>\"$d/hc.log\"\ncase \"$1\" in\n"+
		"get) awk -F'\\t' -v a=\"$2\" '$1==a{printf \"{\\\"address\\\":\\\"%s\\\",\\\"name\\\":\\\"%s\\\",\\\"created_ms\\\":%s,\\\"state\\\":\\\"live\\\",\\\"cwd\\\":\\\"%s\\\",\\\"status\\\":{\\\"app\\\":\\\"claude-code\\\",\\\"state\\\":\\\"%s\\\"}}\\n\",$1,$2,$3,$5,$4; f=1} END{exit !f}' \"$d/terms.tsv\" && exit 0\n"+
		"  echo '{\"error\":{\"kind\":\"not_found\",\"message\":\"없음\"}}'; exit 3;;\n"+
		"kill) [ ! -e \"$d/killfail\" ] || exit 1; awk -F'\\t' -v a=\"$2\" '$1!=a' \"$d/terms.tsv\" >\"$d/terms.new\" && mv \"$d/terms.new\" \"$d/terms.tsv\";;\n"+
		"send) [ ! -e \"$d/sendfail\" ] || exit 1;;\n"+
		"ls) awk -F'\\t' 'BEGIN{printf \"{\\\"terminals\\\":[\"} {printf \"%s{\\\"address\\\":\\\"%s\\\",\\\"name\\\":\\\"%s\\\",\\\"state\\\":\\\"live\\\",\\\"cwd\\\":\\\"%s\\\",\\\"status\\\":\\\"%s\\\"}\",(NR>1?\",\":\"\"),$1,$2,$5,$4} END{print \"]}\"}' \"$d/terms.tsv\";;\n"+
		"*) exit 2;;\nesac\n")
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	k.wr("queue.tsv", strings.Join([]string{"w1", "app", "b", "-", "m", "up", "m", "m/w1", "-", "t", "-", "-", "normal", "-", "auto", "-", "r1"}, "\t")+"\n"+
		strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")+"\n")
	k.wr("try.tsv", "w1\tr1\tm\t-\t-\tm/w1\tw1@1700000000000\naudit-a1\tr2\tm\t-\t-\tm/audit-a1\taudit-a1@1700000000000\n")
	k.wr("audit-proj.tsv", "a1\tapp\n")
	k.setRound("deliver", "pass")
	k.setAudit(status, "audit-a1")
	EV = []string{auTEv("2026-11-24T01:00:00Z", "m/w1", "w1", "완료", auTSha[:8], "끝", "w1@1700000000000"),
		auTEv("2026-11-24T01:10:00Z", "m/audit-a1", "audit-a1", "판정", auTSha, "감사 완료 a1 높음 0·중간 0·낮음 0", "-")}
	return k
}

func (k *clsT) wr(f, s string) {
	k.t.Helper()
	if err := os.WriteFile(k.d+"/"+f, []byte(s), 0o755); err != nil {
		k.t.Fatal(err)
	}
}

func (k *clsT) read(f string) string {
	b, _ := os.ReadFile(k.d + "/" + f)
	return string(b)
}

// setRound: 차수 a1의 상태·판정(16칸 표시는 비움)
func (k *clsT) setRound(st, v string) {
	k.wr("audit.tsv", strings.Join([]string{"a1", "w1", "1", "w1@1700000000000", auTSha, "m", "m/w1", st, "0", v, "0", "0", "-", "-", "0", "-",
		"~/.local/share/hc-fill/reports/audit-a1.md"}, "\t")+"\n")
}

// setAudit: 감사 터미널(주소 m/audit-a1)의 상태와 지금 이름 — 이번 바퀴 터미널 표(TERMS)와 가짜 hc를 함께
func (k *clsT) setAudit(status, name string) {
	k.status = status
	terms := "m/w1\tw1\t1700000000000\tidle\t" + k.wt + "\n"
	TERMS = []Term{{"m", "m/w1", "claude-code", "w1", "idle", k.wt}}
	if status != "" {
		terms += "m/audit-a1\t" + name + "\t1700000000000\t" + status + "\t" + k.wt + "\n"
		TERMS = append(TERMS, Term{"m", "m/audit-a1", "claude-code", name, status, k.wt})
	}
	k.wr("terms.tsv", terms)
}

// round: 이 차수를 한 바퀴(auditTick 안의 a_round 한 번과 같다)
func (k *clsT) round() {
	r := arow("a1")
	auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(atID), r.F(atLane), r.F(atRound), r.F(atRun), r.F(atSHA), r.F(atMach), r.F(atAddr), r.F(atState)
	auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(atFixes), r.F(atVerdict), r.F(atWakes), r.F(atWaited), r.F(atChecked), r.F(atWoke), r.F(atNoVerdict), r.F(atFlags)
	aRound()
}

func (k *clsT) kills() int { return strings.Count(k.read("hc.log"), "kill m/audit-a1\n") }

func (k *clsT) sent() bool { return strings.Contains(k.read("hc.log"), "send m/w1 ") }

func (k *clsT) verdictFile() bool {
	_, err := os.Stat(k.d + "/.local/share/hc-fill/verdicts/w1/a1.md")
	return err == nil
}

// ① 판정 수거·전달 뒤 그 주소·이름의 끝난 감사 터미널을 닫는다(고치기 전: 닫기 0번 — 레인이 받아 간 뒤 다음 바퀴에야 닫았다).
// 닫은 뒤 바퀴는 다시 닫지 않는다
func TestAuditCloseAfterDeliver(t *testing.T) {
	for _, v := range []string{"pass", "fix"} {
		t.Run(v, func(t *testing.T) {
			k := clsSetup(t, "done")
			k.setRound("deliver", v)
			k.round()
			if !k.verdictFile() || !k.sent() {
				t.Fatalf("판정 파일·깨움: 파일 %v 깨움 %v\n%s", k.verdictFile(), k.sent(), k.read("hc.log"))
			}
			if k.kills() != 1 {
				t.Fatalf("판정을 넘긴 뒤 끝난 감사 터미널 닫기 %d번, 바람 1\n%s", k.kills(), k.read("hc.log"))
			}
			if got := arow("a1").F(atState); got != "deliver" {
				t.Fatalf("닫기가 차수 상태를 바꿨다: %s", got)
			}
			k.setAudit("", "")
			k.round()
			if k.kills() != 1 {
				t.Fatalf("닫은 뒤 다시 닫았다: %d번", k.kills())
			}
		})
	}
}

// ② 일하는 중(working·blocked)이면 안 닫고, 다음 바퀴에 끝남이 되면 닫는다
func TestAuditCloseWaitsWhileWorking(t *testing.T) {
	k := clsSetup(t, "working")
	k.round()
	k.setAudit("blocked", "audit-a1")
	k.round()
	if k.kills() != 0 {
		t.Fatalf("일하는 중인 감사 터미널을 닫았다\n%s", k.read("hc.log"))
	}
	k.setAudit("idle", "audit-a1")
	k.round()
	if k.kills() != 1 {
		t.Fatalf("끝남이 된 바퀴에 닫기 %d번, 바람 1\n%s", k.kills(), k.read("hc.log"))
	}
}

// ② 상한: 일하는 중으로 audit_close바퀴를 넘기면 그만둔다(정리 도구에 맡김) — 그 뒤 끝남이 돼도 닫지 않는다
func TestAuditCloseGivesUpAfterRounds(t *testing.T) {
	k := clsSetup(t, "working")
	confLines = append(confLines, "audit_close 2")
	for i := 0; i < 3; i++ {
		k.round()
	}
	k.setAudit("done", "audit-a1")
	k.round()
	if k.kills() != 0 {
		t.Fatalf("상한(2바퀴)을 넘긴 뒤에도 닫았다\n%s", k.read("hc.log"))
	}
}

// ③ 주소는 같고 지금 이름이 다르면(주소 재사용) 닫지 않는다
func TestAuditCloseNameMismatch(t *testing.T) {
	k := clsSetup(t, "done")
	k.setAudit("done", "someone-else")
	k.round()
	if k.kills() != 0 {
		t.Fatalf("이름이 다른 터미널을 닫았다\n%s", k.read("hc.log"))
	}
}

// ③ 감사 기록(try.tsv)에 주소·실행이 없으면 닫지 않는다(대기열이 띄운 것이 확인되지 않음)
func TestAuditCloseNoRecord(t *testing.T) {
	k := clsSetup(t, "done")
	k.wr("try.tsv", "w1\tr1\tm\t-\t-\tm/w1\tw1@1700000000000\n")
	k.round()
	if k.kills() != 0 {
		t.Fatalf("기록 없는 감사 터미널을 닫았다\n%s", k.read("hc.log"))
	}
}

// ④ 판정을 아직 수거하지 못한 감사(queued, 판정 사건 없음)는 끝났어도 닫지 않는다
func TestAuditCloseNotBeforeVerdict(t *testing.T) {
	k := clsSetup(t, "done")
	k.setRound("queued", "-")
	EV = EV[:1]
	k.round()
	if k.kills() != 0 {
		t.Fatalf("판정 전 감사 터미널을 닫았다\n%s", k.read("hc.log"))
	}
}

// ①' queued에서 판정을 받은 그 바퀴(받음 → deliver)에도 닫는다
func TestAuditCloseOnConsumeRound(t *testing.T) {
	k := clsSetup(t, "done")
	k.setRound("queued", "-")
	k.round()
	if got := arow("a1").F(atState); got != "deliver" {
		t.Fatalf("판정을 받지 못했다: %s\n%s", got, k.read("hc.log"))
	}
	if k.kills() != 1 {
		t.Fatalf("판정 받은 바퀴 닫기 %d번, 바람 1\n%s", k.kills(), k.read("hc.log"))
	}
}

// ⑤ 닫기가 실패해도 판정 파일·레인 깨움은 그대로, 다음 바퀴에 다시 닫는다
func TestAuditCloseKillFails(t *testing.T) {
	k := clsSetup(t, "done")
	k.wr("killfail", "")
	k.round()
	if !k.verdictFile() || !k.sent() || arow("a1").F(atState) != "deliver" {
		t.Fatalf("닫기 실패가 판정 전달을 막았다: 파일 %v 깨움 %v 상태 %s", k.verdictFile(), k.sent(), arow("a1").F(atState))
	}
	if k.kills() != 1 {
		t.Fatalf("닫기를 시도하지 않았다\n%s", k.read("hc.log"))
	}
	if err := os.Remove(k.d + "/killfail"); err != nil {
		t.Fatal(err)
	}
	k.round()
	if k.kills() != 2 {
		t.Fatalf("실패 다음 바퀴에 다시 닫지 않았다: %d번", k.kills())
	}
}

// ⑥ 손잡이 audit_close 0이면 닫지 않는다(판정 전달은 그대로)
func TestAuditCloseKnobOff(t *testing.T) {
	k := clsSetup(t, "done")
	confLines = append(confLines, "audit_close 0")
	k.round()
	if k.kills() != 0 || !k.verdictFile() || !k.sent() {
		t.Fatalf("끔인데 닫았거나 전달이 빠졌다: 닫기 %d 파일 %v 깨움 %v", k.kills(), k.verdictFile(), k.sent())
	}
}

// ⑦ 닫은 뒤 레인의 hc-lane clean 「다른 터미널」 판정(hc ls --json의 터미널 가운데 cwd가 작업 트리를 쓰는 것 — hcjson.Uses)에
// 감사 터미널이 걸리지 않는다. 고치기 전에는 판정을 넘긴 바퀴 뒤에도 감사 터미널이 남아 clean이 멈췄다
func TestAuditCloseUnblocksClean(t *testing.T) {
	k := clsSetup(t, "done")
	busy := func() []string {
		out, err := exec.Command("hc", "ls", "--json").Output()
		if err != nil {
			t.Fatal(err)
		}
		l, err := hcjson.ParseList(out)
		if err != nil {
			t.Fatalf("가짜 hc ls: %v\n%s", err, out)
		}
		var b []string
		for _, x := range l.Terminals {
			if x.Address != "m/w1" && hcjson.Uses(x.CwdText, k.wt) { // 레인 자신은 clean이 뺀다
				b = append(b, x.Address)
			}
		}
		return b
	}
	if b := busy(); len(b) != 1 {
		t.Fatalf("장면 준비: 감사 터미널이 작업 트리를 써야 한다: %v", b)
	}
	k.round()
	if b := busy(); len(b) != 0 {
		t.Fatalf("판정을 넘긴 뒤에도 작업 트리를 쓰는 다른 터미널이 남았다(clean이 멈춘다): %v", b)
	}
}
