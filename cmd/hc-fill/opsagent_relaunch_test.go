package main

// ops-agent-relaunch(2026-11-24): 운영 에이전트가 두 번째부터 못 뜨던 것 — 첫 실행의 끝난 터미널(hc-fill-agent, done)이 남아
// 띄울 명령 `hc run --bg --name hc-fill-agent …`가 name_taken으로 실패했고, 일지에는 「실패: 종료 1」뿐이었다.
// 가짜 hc는 terms.tsv(주소 이름 상태) 하나로 run(같은 이름이 살아 있으면 name_taken)·get·kill을 흉내 낸다.
// ① 끝남(done) → 그 주소로 닫기 한 번 ② 그 뒤 다시 띄우기 성공 ③ 띄우기 실패 → 일지에 표준 오류 앞부분, launch.err
// ④ 못 띄움은 시도로 안 셈 — 항목마다 「사람이 정한다」가 아니라 운영 사건 한 번 ⑤ 닫기 손잡이 끔 ⑥ 일하는 중이면 그대로

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const oaRelaunch = `hc run --bg --name "$HC_FILL_AGENT_NAME" --cwd "$HC_FILL_AGENT_DIR" -- agent-cli`

// orFx: oaFx + 가짜 hc(run·get·kill·ls·machine) + 개인 값 무늬(box-[0-9]+). 띄울 명령은 진짜 꼴(hc run --bg --name …)
func orFx(t *testing.T, extra ...string) {
	t.Helper()
	oaFx(t, append([]string{"ops_agent 조치"}, extra...)...)
	confLines = append([]string{"ops_agent_launch " + oaRelaunch}, confLines...)
	cfg := DATA + "/cfg"
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg+"/private-patterns.txt", []byte("box-[0-9]+\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_TOOLS_CONFIG", cfg)
	d := DATA
	sc := "#!/bin/sh\nd='" + d + "'\nprintf '%s\\n' \"$*\" >>\"$d/hc.log\"\ntouch \"$d/terms.tsv\"\ncase \"$1\" in\n" +
		"ls) cat \"$d/lsjson\";;\nmachine) cat \"$d/mls\" || exit 1;;\n" +
		"run) n=; while [ $# -gt 0 ]; do [ \"$1\" = --name ] && n=$2; shift; done\n" +
		"  if awk -F'\\t' -v n=\"$n\" '$2==n{f=1} END{exit !f}' \"$d/terms.tsv\"; then\n" +
		"    echo '{\"error\":{\"kind\":\"name_taken\",\"message\":\"a live terminal is already named '\"$n\"' on box-7\"}}' >&2; exit 1; fi\n" +
		"  a=t$(($(cat \"$d/seq\" 2>/dev/null || echo 0)+1)); echo \"${a#t}\" >\"$d/seq\"; printf '%s\\t%s\\tworking\\n' \"$a\" \"$n\" >>\"$d/terms.tsv\"; echo \"$a\";;\n" +
		"get) awk -F'\\t' -v a=\"$2\" '$1==a{printf \"{\\\"address\\\":\\\"%s\\\",\\\"name\\\":\\\"%s\\\",\\\"created_ms\\\":1700000000000,\\\"state\\\":\\\"live\\\",\\\"cwd\\\":\\\"/\\\",\\\"status\\\":{\\\"app\\\":\\\"claude-code\\\",\\\"state\\\":\\\"%s\\\"}}\\n\",$1,$2,$3; f=1} END{exit !f}' \"$d/terms.tsv\" && exit 0\n" +
		"  echo '{\"error\":{\"kind\":\"not_found\",\"message\":\"없음\"}}' >&2; exit 3;;\n" +
		"kill) awk -F'\\t' -v a=\"$2\" '$1!=a' \"$d/terms.tsv\" >\"$d/terms.new\" && mv \"$d/terms.new\" \"$d/terms.tsv\";;\n" +
		"*) exit 2;;\nesac\n"
	if err := os.WriteFile(d+"/hc", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
}

// orStatus: 가짜 터미널 표의 상태 칸을 모두 s로
func orStatus(t *testing.T, s string) {
	t.Helper()
	b, _ := os.ReadFile(DATA + "/terms.tsv")
	var o []string
	for _, l := range lines(string(b)) {
		f := strings.Split(l, "\t")
		o = append(o, f[0]+"\t"+f[1]+"\t"+s)
	}
	if err := os.WriteFile(DATA+"/terms.tsv", []byte(strings.Join(o, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func orHc(t *testing.T, verb string) []string {
	b, _ := os.ReadFile(DATA + "/hc.log")
	var o []string
	for _, l := range lines(string(b)) {
		if strings.HasPrefix(l, verb+" ") {
			o = append(o, l)
		}
	}
	return o
}

// ① ② 첫 실행이 턴을 끝냄(done) → 끝 처리에서 그 주소(t1)로 닫기 한 번, 다음 바퀴의 띄우기는 name_taken 없이 뜬다
func TestOpsAgentClosesThenRelaunches(t *testing.T) {
	orFx(t)
	now := time.Now()
	opsAgentTick(now)
	if r := oaRunNow(t); r.Addr != "t1" {
		t.Fatalf("첫 실행: %+v", r)
	}
	orStatus(t, "done")
	opsAgentTick(now.Add(3 * time.Minute)) // oaGrace(2분) 뒤 — 턴 끝남
	if k := orHc(t, "kill"); len(k) != 1 || k[0] != "kill t1" {
		t.Fatalf("끝난 실행의 주소로 닫기 한 번: %q", k)
	}
	opsAgentTick(now.Add(4 * time.Minute))
	st, _ := oaLoad()
	if st.Run == nil || st.Run.Addr != "t2" {
		t.Fatalf("다시 띄워야(결과 없이 끝난 항목): %+v — hc %q", st.Run, orHc(t, "run"))
	}
	if k := orHc(t, "kill"); len(k) != 1 {
		t.Fatalf("이미 닫은 것을 또 닫음: %q", k)
	}
}

// ③ 띄울 명령 실패 → 일지 「실패」 줄에 표준 오류 앞부분(개인 값 가림), 실행 폴더에 launch.err 통째로
func TestOpsAgentLaunchErrKept(t *testing.T) {
	orFx(t)
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t1\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(time.Now())
	jb, _ := os.ReadFile(DATA + "/journal.jsonl")
	if !strings.Contains(string(jb), `실패: 종료 1 — {\"error\":{\"kind\":\"name_taken\"`) || strings.Contains(string(jb), "box-7") ||
		!strings.Contains(string(jb), "‹가림›") {
		t.Fatalf("일지에 까닭(가림): %s", jb)
	}
	fs, _ := filepath.Glob(oaDir() + "/oa-*/launch.err")
	if len(fs) != 1 {
		t.Fatalf("launch.err 하나: %q", fs)
	}
	if b, _ := os.ReadFile(fs[0]); !strings.Contains(string(b), "a live terminal is already named hc-fill-agent on box-7") {
		t.Fatalf("launch.err 통째로: %s", b)
	}
}

// ④ 못 띄움은 「시도」가 아니다 — 계속 다시 해 보고(시간당 상한까지), 항목마다 「사람이 정한다」가 아니라 운영 사건 한 번(되풀이 없음).
// 고쳐지면(뜨면) 셈을 비운다
func TestOpsAgentLaunchFailOneEvent(t *testing.T) {
	orFx(t)
	if err := os.WriteFile(DATA+"/terms.tsv", []byte("t1\thc-fill-agent\tdone\n"), 0o644); err != nil {
		t.Fatal(err) // 남의(앞 실행 기록이 없는) 터미널 — 대기열이 닫지 않는다
	}
	now := time.Now()
	for i := 0; i < 4; i++ {
		opsAgentTick(now.Add(time.Duration(i) * time.Minute))
	}
	if n := len(orHc(t, "run")); n != 4 {
		t.Fatalf("못 띄움은 시도로 안 세니 바퀴마다 다시: %d", n)
	}
	rep := reports(t)
	if n := grepN(rep, "번 띄웠는데 못 풀림"); n != 0 {
		t.Fatalf("항목마다 사람에게 넘기면 안 됨: %q", rep)
	}
	if n := grepN(rep, "운영 에이전트를 못 띄움 2번 이어짐 — {\"error\":{\"kind\":\"name_taken\""); n != 1 {
		t.Fatalf("운영 사건 한 번: %q", rep)
	}
	if k := orHc(t, "kill"); len(k) != 0 {
		t.Fatalf("자기가 띄우지 않은 터미널을 닫음: %q", k)
	}
	st, _ := oaLoad()
	for k, x := range st.Keys {
		if x.Tries != 0 || x.Asked {
			t.Fatalf("못 띄움이 시도로 셈 %s: %+v", k, x)
		}
	}
	if err := os.WriteFile(DATA+"/terms.tsv", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(5 * time.Minute))
	if st, _ = oaLoad(); st.Run == nil || st.LaunchFails != 0 || st.FailSent {
		t.Fatalf("뜨면 못 띄움 셈을 비움: %+v", st)
	}
}

// ⑤ 닫기 손잡이 끔 → 끝난 실행의 터미널을 닫지 않는다(그래서 다음 띄우기는 name_taken)
func TestOpsAgentCloseKnobOff(t *testing.T) {
	orFx(t, "ops_agent_close 0")
	now := time.Now()
	opsAgentTick(now)
	orStatus(t, "done")
	opsAgentTick(now.Add(3 * time.Minute))
	opsAgentTick(now.Add(4 * time.Minute))
	if k := orHc(t, "kill"); len(k) != 0 {
		t.Fatalf("끔인데 닫음: %q", k)
	}
	if st, _ := oaLoad(); st.Run != nil {
		t.Fatalf("닫지 않았으니 못 떠야: %+v", st.Run)
	}
}

// ⑥ 일하는 중(working)인 앞 실행 → 닫지 않고 새로 띄우지도 않음
func TestOpsAgentWorkingKept(t *testing.T) {
	orFx(t)
	now := time.Now()
	opsAgentTick(now)
	opsAgentTick(now.Add(3 * time.Minute))
	opsAgentTick(now.Add(10 * time.Minute))
	if k, r := orHc(t, "kill"), orHc(t, "run"); len(k) != 0 || len(r) != 1 {
		t.Fatalf("일하는 중: kill %q run %q", k, r)
	}
}
