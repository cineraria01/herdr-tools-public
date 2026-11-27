package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ht-revive-cmd(2026-11-28): 워커 기계 전원이 끊겨 18시간 일한 레인의 터미널만 죽었다 — 대기열 줄은 옛 주소에 묶여 「끝남」, 지휘가 손으로
// 띄운 새 세션의 완료는 그 줄 것으로 읽히지 않아 감사가 열리지 않았고, 멈춤 알림에서도 빠져 아무 알림 없이 묻혔다.
// 가짜 hc는 auditclose_test의 것(terms.tsv), 가짜 hc-lane은 --print-steps·coord·report와 --revive 띄우기(새 터미널 m/w1b)

type rvT struct{ *clsT }

func rvFx(t *testing.T) *rvT {
	k := &rvT{clsSetup(t, "")}
	om, ob, oq, ola, ota, ow, op, ops, olw, oh := MQP, MQBAD, MQS, LAUNCH, TASK, WAIT, POLL, P, LANEADDR, HOME
	t.Cleanup(func() {
		MQP, MQBAD, MQS, LAUNCH, TASK, WAIT, POLL, P, LANEADDR, HOME = om, ob, oq, ola, ota, ow, op, ops, olw, oh
	})
	MQP, MQBAD, MQS, LAUNCH, TASK, WAIT, POLL, P, LANEADDR, HOME = "", "", nil, "claude {task}", "{run} 지시문을 읽고 진행하라.", 3, 0, "", nil, DATA
	AUD, AK, RSRC = "", "", srcNew
	if err := os.MkdirAll(DATA+"/repo", 0o755); err != nil {
		t.Fatal(err)
	}
	k.git(DATA+"/repo", "init", "-q", "-b", "main") // 원격 없는 저장소 — 병합기 줄 없음, 기본 가지 refs/heads/main
	pcRef = map[string]pcRes{DATA + "/repo": {"refs/heads/main", true}}
	FETCHED, FETCHBAD = "", ""
	k.git(k.wt, "init", "-q", "-b", "w1")
	k.wr("repo.wt/w1/a", "커밋한 줄\n")
	k.git(k.wt, "add", "a")
	k.git(k.wt, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "-m", "w1: 반쯤")
	k.wr("repo.wt/w1/a", "커밋한 줄\n반쯤 고친 줄\n") // 커밋 안 된 변경
	k.wr("b.md", "# 본 일\n")
	k.wr("queue.tsv", strings.Join([]string{"w1", "app", DATA + "/b.md", "-", "-", "up", "m", "m/w1", "-", "t", "-", "t0", "normal", "-", "auto", "-", "r1"}, "\t")+"\n")
	k.wr("audit.tsv", "")
	k.wr("terms.tsv", "") // 레인 터미널이 죽었다(hc get not_found)
	TERMS, LSSNAP = nil, true
	EV = []string{auTEv("2026-11-28T01:00:00Z", "m/w1", "w1", "중간", "-", "일하는 중", "w1@1700000000000")}
	k.wr("hc-lane", "#!/bin/sh\nd='"+DATA+"'\nprintf '%s\\n' \"$*\" >>\"$d/lane.log\"\n"+
		"case \" $* \" in *\" --print-steps \"*) printf '### 착지·정리\\n1. land\\n'; case \" $* \" in *\" --revive \"*) printf '### 재시동 뒤 이어 가기\\n';; esac; exit 0;; esac\n"+
		"case \"$1\" in coord) echo '지휘 없음'; exit 1;; report) exit 0;; esac\n"+
		"echo \"STACK=${HC_LANE_STACK-}\" >>\"$d/lane.env\"\n"+
		"[ ! -e \"$d/lanerc\" ] || { cat \"$d/lanerc.out\" 2>/dev/null; exit \"$(cat \"$d/lanerc\")\"; }\n"+
		"n=; p=; for a in \"$@\"; do [ \"$p\" != --name ] || n=$a; p=$a; done; s=$(cat \"$d/seq\" 2>/dev/null); echo \"${s}b\" >\"$d/seq\"\n"+
		"printf 'm/%s%s\\t%s\\t170000000900%s\\tworking\\t%s\\n' \"$n\" \"${s}b\" \"$n\" \"${#s}\" '"+DATA+"/repo.wt/'\"$n\" >>\"$d/terms.tsv\"\n"+
		"echo \"{\\\"name\\\":\\\"$n\\\",\\\"lane\\\":\\\"m/$n${s}b\\\"}\"\n")
	os.Remove(DATA + "/hc.log")
	return k
}

func (k *rvT) git(dir string, a ...string) string {
	k.t.Helper()
	o, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput()
	if err != nil {
		k.t.Fatalf("git %v: %v %s", a, err, o)
	}
	return string(o)
}

// snap: 작업 트리의 커밋 안 된 변경 전부(합격 기준 — 앞뒤 바이트까지 같다)
func (k *rvT) snap() string {
	return k.git(k.wt, "status", "--porcelain=v2") + k.git(k.wt, "diff") + k.read("repo.wt/w1/a")
}

func (k *rvT) launches() int { return strings.Count(k.read("lane.log"), "--revive --continue-from") }

// ① not_found 확정 + 작업 트리 있음 → hc-lane --revive로 같은 작업 트리에 띄우고 줄을 새 실행에 묶는다(14·15칸 그대로)
func TestReviveLaunchBinds(t *testing.T) {
	k := rvFx(t)
	before := k.snap()
	rc, line := reviveRow("w1", "명령")
	if rc != rvDone || !strings.HasPrefix(line, "이어 띄움 w1 @m: 작업 트리 "+k.wt+" (HEAD ") || !strings.Contains(line, "커밋 안 된 파일 1) — 새 터미널 m/w1b") {
		t.Fatalf("이어 띄움이어야: %d %q", rc, line)
	}
	q := qrow("w1")
	if q.F(qState) != "up" || q.F(qAddr) != "m/w1b" || q.F(qAudit) != "auto" || q.F(qRID) != "r1" || !strings.Contains(k.read("try.tsv"), "m/w1b\tw1b@1700000009000") {
		t.Fatalf("줄이 새 실행에 묶여야: %v\n%s", q, k.read("try.tsv"))
	}
	if k.launches() != 1 || !strings.Contains(k.read("lane.log"), "--continue-from "+DATA+"/.local/share/hc-fill/runs/w1-run.md --name w1 --repo "+DATA+"/repo") {
		t.Fatalf("hc-lane --revive --continue-from <run 지시문> 한 번이어야:\n%s", k.read("lane.log"))
	}
	if rb := k.read(".local/share/hc-fill/runs/w1-run.md"); !strings.HasPrefix(rb, "# 본 일") || !strings.Contains(rb, "재시동 뒤 이어 가기") {
		t.Fatalf("run 지시문에 본문 + 이어 가기 절: %q", rb)
	}
	if k.snap() != before {
		t.Fatal("커밋 안 된 변경이 바뀌었다")
	}
	if !strings.Contains(k.read("revive.tsv"), "w1\tr1\tm/w1\t이어 띄움\tm/w1b\t") {
		t.Fatalf("기록: %q", k.read("revive.tsv"))
	}
	// 두 번째 실행(revive 두 번 → 터미널 하나): 이번 표에 새 터미널이 보이면 살아 있음
	TERMS = []Term{{"m", "m/w1b", "claude-code", "w1", "working", k.wt}}
	if rc, line := reviveRow("w1", "명령"); rc != rvLive || k.launches() != 1 {
		t.Fatalf("두 번째는 살아 있음(새 세션 0): %d %q", rc, line)
	}
	if rvNote(readRows(rvFile()), qrow("w1"), "working") != "이어 띄움(재시동 뒤, 1번째)" {
		t.Fatalf("표시: %q", rvNote(readRows(rvFile()), qrow("w1"), "working"))
	}
}

// ② 그 작업 트리에 같은 이름 세션이 이미 살아 있음(사람이 손으로 — 라벨 없음, 같은 폴더의 다른 철자 포함) → 묶기만, 새로 띄우지 않음
func TestReviveBindOnly(t *testing.T) {
	cwds := []string{"WT"}
	if _, err := os.Stat(strings.ToUpper(DATA)); err == nil { // 대소문자를 가리지 않는 파일 시스템(macOS 기본)
		cwds = append(cwds, "UPPER")
	}
	for _, c := range cwds {
		k := rvFx(t)
		cwd := k.wt
		if c == "UPPER" {
			cwd = strings.Replace(k.wt, "/repo.wt/", "/REPO.wt/", 1)
		}
		k.wr("terms.tsv", "m/w2\tw1\t1700000005000\tidle\t"+cwd+"\n")
		TERMS = []Term{{"m", "m/w2", "claude-code", "w1", "idle", cwd}}
		rc, line := reviveRow("w1", "명령")
		if rc != rvDone || line != "묶기만 w1: m/w1 → m/w2 (hc-lane 라벨 없음 — 손으로 띄운 세션이라 레인 임대 값이 없을 수 있다: 임대를 손으로 풀어야 할 수 있음)" {
			t.Fatalf("%s: 묶기만이어야: %d %q", c, rc, line)
		}
		if qrow("w1").F(qAddr) != "m/w2" || k.launches() != 0 {
			t.Fatalf("%s: 줄만 묶고 띄우지 않아야: %v\n%s", c, qrow("w1"), k.read("lane.log"))
		}
	}
	// 외부 가정: Windows 꼴 cwd의 같은 이름 터미널은 다른 곳 — 묶지도 띄우지도 않는다
	k := rvFx(t)
	TERMS = []Term{{"m", "m/w3", "claude-code", "w1", "idle", `C:\Users\x\repo.wt\w1`}}
	if rc, line := reviveRow("w1", "명령"); rc != rvLive || !strings.Contains(line, "다른 곳") || k.launches() != 0 {
		t.Fatalf("다른 곳의 같은 이름이면 살아 있음: %d %q", rc, line)
	}
	// 다른 이름이 그 작업 트리를 쓴다(감사 터미널 등) → 새로 띄우지 않음
	TERMS = []Term{{"m", "m/a9", "claude-code", "audit-a9", "working", k.wt + "/sub"}}
	if rc, line := reviveRow("w1", "명령"); rc != rvLive || !strings.Contains(line, "작업 트리") || k.launches() != 0 {
		t.Fatalf("작업 트리를 쓰는 다른 터미널: %d %q", rc, line)
	}
}

// ③ 연결 모름·사건 모름·hc get 모름 → 아무것도 안 함(종료 1) ④ 터미널이 사실 살아 있음(끊겼다 돌아온 기계) → 아무것도 안 함(10)
func TestReviveUnknownOrAlive(t *testing.T) {
	k := rvFx(t)
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tup\tm\tm/w1\t", "\tup\tm2\tm2/w1\t", 1))
	if rc, _ := reviveRow("w1", "명령"); rc != rvUnk {
		t.Fatalf("연결 안 됨은 모름: %d", rc)
	}
	k = rvFx(t)
	EVBAD = true
	if rc, _ := reviveRow("w1", "명령"); rc != rvUnk {
		t.Fatalf("사건 모름: %d", rc)
	}
	EVBAD = false
	k.wr("hc", strings.Replace(k.read("hc"), "case \"$1\" in\n", "[ \"$1\" != get ] || exit 9\ncase \"$1\" in\n", 1))
	if rc, line := reviveRow("w1", "명령"); rc != rvUnk || !strings.Contains(line, "hc get") {
		t.Fatalf("hc get 모름: %d %q", rc, line)
	}
	k = rvFx(t)
	k.wr("terms.tsv", "m/w1\tw1\t1700000000000\tworking\t"+k.wt+"\n") // 표(이번 바퀴 목록)에는 없었지만 hc get은 살아 있음
	if rc, line := reviveRow("w1", "명령"); rc != rvLive || !strings.Contains(line, "돌아옴") || k.launches() != 0 || qrow("w1").F(qAddr) != "m/w1" {
		t.Fatalf("살아 있으면 아무것도 안 함: %d %q", rc, line)
	}
	if k.launches() != 0 {
		t.Fatal("띄우면 안 된다")
	}
}

// ⑤ 끝 상태 보고·병합기 줄·감사 넘기기 중·감사 줄·대기 줄 → 거절(11)
func TestReviveRefuses(t *testing.T) {
	for _, st := range []string{"착지", "정리됨", "주차", "제출", "완료"} {
		rvFx(t)
		EV = []string{auTEv("2026-11-28T01:00:00Z", "m/w1", "w1", st, "-", "끝", "w1@1700000000000")}
		if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "끝난 실행") {
			t.Fatalf("%s: 끝난 실행이면 거절: %d %q", st, rc, line)
		}
	}
	k := rvFx(t)
	MQP, MQS = ",app", []Row{{"app", "refs/merge-queue/1700000000-w1", auTSha}}
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "병합기") {
		t.Fatalf("병합기 줄: %d %q", rc, line)
	}
	MQP, MQS = "", nil
	k.wr("audit.tsv", strings.Join([]string{"a1", "w1", "1", "w1@1700000000000", auTSha, "m", "m/w1", "escalate:noverdict", "0", "-", "0", "0", "-", "-", "0", "-", "글"}, "\t")+"\n")
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "지휘에 넘기는 중") {
		t.Fatalf("감사 넘기기 중: %d %q", rc, line)
	}
	k.wr("audit.tsv", "")
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tup\tm\tm/w1\t", "\twait\t-\t-\t", 1))
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "떠 있는 줄이 아니다") {
		t.Fatalf("대기 줄: %d %q", rc, line)
	}
	k.wr("queue.tsv", strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")+"\n")
	if rc, line := reviveRow("audit-a1", "명령"); rc != rvNo || !strings.Contains(line, "감사 줄") {
		t.Fatalf("감사 줄: %d %q", rc, line)
	}
	if k.launches() != 0 {
		t.Fatal("거절은 띄우지 않는다")
	}
}

// ⑥ 묶인 뒤 완료 보고 → 그 바퀴에 감사 차수가 열리고 부모 실행은 새 실행 ⑦ 옛 실행에 묶인 감사 차수는 지휘에 넘기지 않고 superseded
func TestReviveAudit(t *testing.T) {
	k := rvFx(t)
	// ⑦ 옛 실행의 차수(판정이 왔다) — 줄이 옮겨 가기 전에는 지휘에 넘긴다(그대로), 옮겨 간 뒤에는 superseded
	k.wr("queue.tsv", k.read("queue.tsv")+strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")+"\n")
	k.setRound("queued", "-")
	EV = append(EV, auTEv("2026-11-28T01:10:00Z", "m/audit-a1", "audit-a1", "판정", auTSha, "감사 완료 a1 높음 0·중간 0·낮음 0", "-"))
	if rc, line := reviveRow("w1", "명령"); rc != rvDone {
		t.Fatalf("이어 띄움: %d %q", rc, line)
	}
	k.round()
	if s := arow("a1").F(atState); s != "superseded" {
		t.Fatalf("옛 실행 차수는 superseded여야(지휘로 넘기지 않음): %s", s)
	}
	// ⑥ 새 실행의 완료 → 새 차수(부모 실행 = 새 실행)
	h := strings.TrimSpace(k.git(k.wt, "rev-parse", "HEAD"))
	EV = append(EV, auTEv("2026-11-28T02:00:00Z", "m/w1b", "w1", "완료", h[:8], "끝", "w1b@1700000009000"))
	TERMS = []Term{{"m", "m/w1b", "claude-code", "w1", "idle", k.wt}}
	aOpen("w1", "app", "m", "m/w1b")
	ok := false
	for _, r := range readRows(AT) {
		if r.F(atLane) == "w1" && r.F(atRun) == "w1b@1700000009000" && r.F(atAddr) == "m/w1b" && r.F(atSHA) == h {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("새 실행의 완료로 차수가 열려야:\n%s", k.read("audit.tsv"))
	}
}

// 감사 atp6fql M1: 레인과 감사 터미널이 함께 죽으면 판정이 오지 않는다 — 판정 없음 셈(3바퀴 → 지휘)으로 가지 않고, 줄이 옮겨 갔으면 superseded
func TestReviveAuditNoVerdict(t *testing.T) {
	k := rvFx(t)
	k.wr("queue.tsv", k.read("queue.tsv")+strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")+"\n")
	k.setRound("queued", "-")
	if rc, line := reviveRow("w1", "명령"); rc != rvDone {
		t.Fatalf("이어 띄움: %d %q", rc, line)
	}
	for i := 0; i < 3; i++ {
		if s := arow("a1").F(atState); s == "queued" {
			k.round()
		}
	}
	if s := arow("a1").F(atState); s != "superseded" || strings.Contains(k.read("lane.log"), "판정 없음") {
		t.Fatalf("판정 없이 옮겨 간 차수는 superseded: %s\n%s", s, k.read("lane.log"))
	}
}

// 옮겨 가지 않은 옛 차수(부모가 그냥 끝남)는 지금처럼 지휘에 넘긴다
func TestReviveAuditNotRebound(t *testing.T) {
	k := rvFx(t)
	k.wr("queue.tsv", k.read("queue.tsv")+strings.Join([]string{"audit-a1", "app", "b", "-", "m", "up", "m", "m/audit-a1", "-", "t", "-", "-", "urgent", "no-early", "audit=w1", "-", "r2"}, "\t")+"\n")
	k.setRound("queued", "-")
	EV = append(EV, auTEv("2026-11-28T01:10:00Z", "m/audit-a1", "audit-a1", "판정", auTSha, "감사 완료 a1 높음 0·중간 0·낮음 0", "-"))
	k.round()
	if s := arow("a1").F(atState); s != "coord" || !strings.Contains(k.read("lane.log"), "부모 실행(w1@1700000000000)이 끝났다") {
		t.Fatalf("그대로 지휘에: %s", s)
	}
}

// ⑧ 꺼져 죽음: 터미널이 없어진 바퀴(앞 바퀴 seen.tsv에 있었음)에 주인 지휘에 막힘 한 번(되풀이 없음)·표시. 옛 줄(앞 바퀴에도 없던)·끝 보고는 조용
func TestDeadTickOnce(t *testing.T) {
	k := rvFx(t)
	k.wr("seen.tsv", "m\tw1\tm/w1\n")
	deadTick()
	deadTick()
	e := k.read("lane.log")
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "w1 꺼져 죽음 — @m 터미널 m/w1 이 없다(마지막 보고 중간)") ||
		!strings.Contains(e, "hc-fill revive w1") || !strings.Contains(e, "--to t0") || !strings.Contains(e, "--key hf:w1:dead:m/w1") {
		t.Fatalf("막힘 한 번이어야:\n%s", e)
	}
	x := rvNote(readRows(rvFile()), qrow("w1"), "")
	if x != "꺼져 죽음 — hc-fill revive w1" || stStage0(stIn{State: "up", Rev: x, Last: Row{"w1", "중간"}}) != x {
		t.Fatalf("표시: %q", x)
	}
	// 이어 띄운 뒤(바퀴 순서 반대 — 새 주소가 표에 있다) 꺼져 죽음 표시가 없어지고 다시 알리지 않는다
	if rc, _ := reviveRow("w1", "명령"); rc != rvDone {
		t.Fatal("이어 띄움")
	}
	TERMS = []Term{{"m", "m/w1b", "claude-code", "w1", "working", k.wt}}
	deadTick()
	if strings.Count(k.read("lane.log"), "report 막힘") != 1 || rvNote(readRows(rvFile()), qrow("w1"), "working") != "이어 띄움(재시동 뒤, 1번째)" {
		t.Fatalf("이어 띄운 뒤 조용·표시:\n%s", k.read("lane.log"))
	}
}

// 운영 장면: 이미 쌓인 상태에 처음 돌 때 — 앞 바퀴에도 없던 옛 「끝남」 줄과 끝 보고가 있는 줄은 알리지도 표시를 바꾸지도 않는다
func TestDeadTickOldRowsQuiet(t *testing.T) {
	k := rvFx(t)
	deadTick() // seen.tsv 없음 — 오래전에 죽은 줄
	k.wr("seen.tsv", "m\tw1\tm/w1\n")
	EV = []string{auTEv("2026-11-28T01:00:00Z", "m/w1", "w1", "정리됨", "-", "hc-lane clean", "w1@1700000000000")}
	deadTick()
	if strings.Contains(k.read("lane.log"), "report") || rvNote(readRows(rvFile()), qrow("w1"), "") != "" {
		t.Fatalf("옛 줄·끝 보고는 조용:\n%s", k.read("lane.log"))
	}
}

// ⑨ --dry-run은 말만 ⑩ 작업 트리가 없어짐·가지가 다름 → 거절과 까닭
func TestReviveDryAndTree(t *testing.T) {
	k := rvFx(t)
	dry, P = true, "[dry-run] "
	rc, line := reviveRow("w1", "명령")
	dry = false
	if rc != rvDone || !strings.HasPrefix(line, "[dry-run] 이어 띄울 w1 @m: 작업 트리 "+k.wt) || k.launches() != 0 || qrow("w1").F(qAddr) != "m/w1" || exists(rvFile()) {
		t.Fatalf("말만: %d %q", rc, line)
	}
	k.git(k.wt, "checkout", "-q", "-b", "other")
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "가지가 other") {
		t.Fatalf("가지가 다름: %d %q", rc, line)
	}
	if err := os.RemoveAll(k.wt); err != nil {
		t.Fatal(err)
	}
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || !strings.Contains(line, "작업 트리가 없다") || k.launches() != 0 {
		t.Fatalf("작업 트리 없음: %d %q", rc, line)
	}
}

// 재시동 꼴 ②(2026-11-28 워커 기계 정상 재부팅 실측): 코어가 레인 터미널을 같은 주소·이름·라벨로 되살렸는데 안은 빈 로그인 셸(status null·
// argv zsh -l·전경 = 터미널 pid) — 「작업 중(-)」으로 보여 묻혔다. 셸만 남은 그 터미널은 꺼져 죽음이고, revive는 그 주소를 닫고 이어 띄운다.
// 에이전트가 전경이면(status만 아직 없음) 살아 있음 그대로
func (k *rvT) shell(fg string) {
	k.wr("terms.tsv", "m/w1\tw1\t1700000000000\t-\t"+k.wt+"\n")
	TERMS = []Term{{"m", "m/w1", "claude-code", "w1", "-", k.wt}}
	k.wr("hc", strings.Replace(k.read("hc"), "case \"$1\" in\n",
		"[ \"$1\" != get ] || [ \"$2\" != m/w1 ] || ! grep -q '^m/w1\t' \"$d/terms.tsv\" || { cat \"$d/shell.json\"; exit 0; }\ncase \"$1\" in\n", 1))
	k.wr("shell.json", `{"address":"m/w1","name":"w1","created_ms":1700000000000,"state":"live","cwd":"`+k.wt+`","status":null,"keep":false,`+
		`"argv":["/bin/zsh","-l"],"pid":40170,"foreground":`+fg+`,"turn":1}`)
}

func TestReviveShellLeft(t *testing.T) {
	sh := `{"argv":["/bin/zsh","-l"],"cwd":"/w","name":"zsh","pid":40170}`
	k := rvFx(t)
	k.shell(sh)
	k.wr("seen.tsv", "m\tw1\tm/w1\n")
	deadTick()
	deadTick()
	if strings.Count(k.read("lane.log"), "report 막힘") != 1 || rvNote(readRows(rvFile()), qrow("w1"), "-") != "꺼져 죽음 — hc-fill revive w1" {
		t.Fatalf("빈 셸로 되살아난 터미널은 꺼져 죽음(막힘 한 번·표시):\n%s", k.read("lane.log"))
	}
	if g := strings.Count(k.read("hc.log"), "get m/w1 "); g != 1 { // 알린 뒤 바퀴는 hc get을 다시 부르지 않는다(감사 atp6gby N2)
		t.Fatalf("hc get은 알리기 전 한 번: %d\n%s", g, k.read("hc.log"))
	}
	// 거절될 입력(작업 트리 가지가 다름)이면 빈 셸을 닫지 않는다(감사 atp6gby N1)
	k.git(k.wt, "checkout", "-q", "-b", "other")
	if rc, _ := reviveRow("w1", "명령"); rc != rvNo || strings.Contains(k.read("hc.log"), "kill") {
		t.Fatalf("거절이면 닫지 않음: %d\n%s", rc, k.read("hc.log"))
	}
	k.git(k.wt, "checkout", "-q", "w1")
	TERMS = []Term{{"m", "m/w1", "claude-code", "w1", "-", k.wt}}
	dry, P = true, "[dry-run] "
	rc, line := reviveRow("w1", "명령")
	dry = false
	if rc != rvDone || !strings.Contains(line, "빈 셸 m/w1 을 닫고") || strings.Contains(k.read("hc.log"), "kill") {
		t.Fatalf("dry-run은 닫지 않고 말만: %d %q", rc, line)
	}
	rc, line = reviveRow("w1", "명령")
	if rc != rvDone || strings.Count(k.read("hc.log"), "kill m/w1\n") != 1 || qrow("w1").F(qAddr) != "m/w1b" || k.launches() != 1 {
		t.Fatalf("빈 셸을 닫고 이어 띄워야: %d %q\n%s", rc, line, k.read("hc.log"))
	}
	// 에이전트가 전경(status는 아직 없음) → 살아 있음, 닫지 않음
	k = rvFx(t)
	k.shell(`{"argv":["claude","--model","x"],"cwd":"/w","name":"claude","pid":40180}`)
	k.wr("seen.tsv", "m\tw1\tm/w1\n")
	deadTick()
	if rc, _ := reviveRow("w1", "명령"); rc != rvLive || strings.Contains(k.read("hc.log"), "kill") || strings.Contains(k.read("lane.log"), "report") {
		t.Fatalf("에이전트가 돌면 살아 있음: %d\n%s", rc, k.read("hc.log"))
	}
}

// 감사 atp6fql M2: 쌓아 띄운 줄·먼저 띄운 줄을 이어 띄우면 run 지시문을 새로 쓰면서 그 절과 HC_LANE_STACK이 빠졌다 — 줄 모드대로 다시 짓는다
func TestReviveKeepsStackAndEarly(t *testing.T) {
	k := rvFx(t)
	p := strings.Repeat("c", 40)
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tnormal\t-\tauto\t", "\tnormal\tstacked\tauto\t", 1))
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\t-\t-\tup\t", "\tpre1\t-\tup\t", 1))
	k.wr("stack.tsv", strings.Join([]string{"w1", "r1", "pre1", p, "1700000000-pre1", "up", "2026-11-28T01:00:00Z"}, "\t")+"\n")
	if rc, line := reviveRow("w1", "명령"); rc != rvDone {
		t.Fatalf("쌓인 줄 이어 띄움: %d %q", rc, line)
	}
	rb := k.read(".local/share/hc-fill/runs/w1-run.md")
	if !strings.Contains(k.read("lane.env"), "STACK=pre1@"+p) || !strings.Contains(rb, "## 쌓아 띄움 — 기준은 선행 pre1") {
		t.Fatalf("쌓아 띄움 절·HC_LANE_STACK이 있어야:\n%s\n%s", k.read("lane.env"), rb)
	}
	if q := qrow("w1"); rowMode(q) != mStacked {
		t.Fatalf("14칸 그대로: %v", q)
	}
	k = rvFx(t)
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tnormal\t-\tauto\t", "\tnormal\tearly\tauto\t", 1))
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\t-\t-\tup\t", "\tpre1\t-\tup\t", 1))
	if rc, line := reviveRow("w1", "명령"); rc != rvDone {
		t.Fatalf("먼저 띄운 줄 이어 띄움: %d %q", rc, line)
	}
	if rb := k.read(".local/share/hc-fill/runs/w1-run.md"); !strings.Contains(rb, "선행 대기 초과") || strings.Contains(k.read("lane.env"), "STACK=pre1") {
		t.Fatalf("먼저 띄움 절이 있어야:\n%s", rb)
	}
}

// 감사 atp6hkn M1: 스스로 이어 띄운 쌓인 줄·먼저 띄운 줄의 절(ESEC)·STACK이 같은 바퀴에 다음으로 뜨는 다른 레인의 run 지시문·환경에 샜다.
// 이어 띄운 뒤 한 번 띄우기용 전역이 모두 비고, 바로 다음 띄우기(launch — place가 부르는 것)의 지시문·환경에 남의 절이 없다
func TestReviveNoLeakToNextLaunch(t *testing.T) {
	for _, mode := range []string{"stacked", "early"} {
		k := rvFx(t)
		k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tnormal\t-\tauto\t", "\tnormal\t"+mode+"\tauto\t", 1))
		k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\t-\t-\tup\t", "\tpre1\t-\tup\t", 1))
		k.wr("stack.tsv", strings.Join([]string{"w1", "r1", "pre1", strings.Repeat("c", 40), "1700000000-pre1", "up", "2026-11-28T01:00:00Z"}, "\t")+"\n")
		if rc, line := reviveRow("w1", "스스로"); rc != rvDone {
			t.Fatalf("%s: 이어 띄움: %d %q", mode, rc, line)
		}
		if RSRC != srcNew || RSEC != "" || ESEC != "" || STACK != "" || STREF != "" {
			t.Fatalf("%s: 이어 띄운 뒤 전역이 남았다: RSRC=%v RSEC=%q ESEC=%q STACK=%q STREF=%q", mode, RSRC, RSEC, ESEC, STACK, STREF)
		}
		k.wr("w3.md", "# 다른 일\n")
		RID, AK, SB, OWNF, OWND, PAFT, PSOLO = "r3", "-", DATA+"/w3.md", DATA+"/own.tsv", DATA+"/own", "-", "-" // 바퀴가 채우는 쓰기 소유 자리
		if lr := launch("w3", DATA+"/w3.md", "m", DATA+"/repo", "-", "-"); lr != 0 {
			t.Fatalf("%s: 다음 띄우기: %d", mode, lr)
		}
		rb, env := k.read(".local/share/hc-fill/runs/w3-run.md"), lines(k.read("lane.env"))
		if strings.Contains(rb, "쌓아 띄움") || strings.Contains(rb, "선행 대기") || env[len(env)-1] != "STACK=" {
			t.Fatalf("%s: 다음 레인에 남의 절·환경이 샜다:\n%s\n%v", mode, rb, env)
		}
	}
}

// 감사 atp6hkn L6: 다시 지을 절의 거절(쌓인 줄인데 기록 없음)이 빈 셸 닫기·dry-run보다 뒤에 있었다 — dry-run도 거절하고 셸을 닫지 않는다
func TestReviveSecsBeforeShellClose(t *testing.T) {
	k := rvFx(t)
	k.shell(`{"argv":["/bin/zsh","-l"],"cwd":"/w","name":"zsh","pid":40170}`)
	k.wr("queue.tsv", strings.Replace(k.read("queue.tsv"), "\tnormal\t-\tauto\t", "\tnormal\tstacked\tauto\t", 1))
	dry, P = true, "[dry-run] "
	rc, line := reviveRow("w1", "명령")
	dry = false
	if rc != rvNo || !strings.Contains(line, "쌓음 기록") {
		t.Fatalf("dry-run도 거절: %d %q", rc, line)
	}
	TERMS = []Term{{"m", "m/w1", "claude-code", "w1", "-", k.wt}}
	if rc, line := reviveRow("w1", "명령"); rc != rvNo || strings.Contains(k.read("hc.log"), "kill") {
		t.Fatalf("거절이면 빈 셸을 닫지 않음: %d %q\n%s", rc, line, k.read("hc.log"))
	}
}

// hc-lane 관문이 거절(임대 없음 11·이미 돎 10) → 줄 그대로, 까닭
func TestReviveLaneGate(t *testing.T) {
	for _, c := range []struct {
		rc   string
		want int
	}{{"11", rvNo}, {"10", rvLive}} {
		k := rvFx(t)
		k.wr("lanerc", c.rc)
		k.wr("lanerc.out", "hc-lane: --revive: 임대 refs/hc-lanes/w1 가 없다\n")
		rc, line := reviveRow("w1", "명령")
		if rc != c.want || qrow("w1").F(qAddr) != "m/w1" || exists(rvFile()) {
			t.Fatalf("종료 %s: %d %q", c.rc, rc, line)
		}
	}
}
