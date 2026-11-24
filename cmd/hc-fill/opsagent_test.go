package main

// ops-agent(2026-11-24): 운영 에이전트 바퀴 단계 — 가짜 띄울 명령(sh 한 줄, 진짜 에이전트 CLI·원격 없음)과 advFx(손댈 것이 있는 가짜 대기열).
// ① 켬·새 항목 → 한 번 띄움, 지시문에 목록·상태 경로 ② 같은 항목 그대로 → 다시 안 띄움, 못 띄움 N번 뒤 「사람이 정한다」 한 번
// ③ 끔 → 안 띄움 ⑤ 시간당 상한 ⑧ 띄우기 실패가 바퀴를 안 막음(다음 바퀴에 다시) ⑨ 끝난 뒤 요약이 일지·사건에.
// 고치기 전 코드로는 opsAgentTick·oaLoad가 없어 컴파일부터 실패한다

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// oaFx: advFx + 설정 폴더의 틀 + 띄울 명령(실행 ID·지시문 경로를 launched에 적고 주소 t9). advise_every 0 — 바퀴마다 본다
func oaFx(t *testing.T, extra ...string) {
	t.Helper()
	advFx(t)
	oc := CONF
	t.Cleanup(func() { CONF = oc })
	if err := os.MkdirAll(DATA+"/conf", 0o755); err != nil {
		t.Fatal(err)
	}
	CONF = DATA + "/conf/hc-fill.tsv"
	if err := os.WriteFile(DATA+"/conf/ops-agent.md", []byte("# 운영 에이전트 틀\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	confLines = append(confLines, extra...)
	confLines = append(confLines, "advise_every 0", "ops_agent_max_hour 10",
		`ops_agent_launch printf '%s %s\n' "$HC_FILL_AGENT_RUN" "$HC_FILL_AGENT_BRIEF" >>'`+DATA+`/launched'; echo t9`)
}

func oaLaunched(t *testing.T) []string {
	t.Helper()
	b, _ := os.ReadFile(DATA + "/launched")
	return lines(string(b))
}

func oaRunNow(t *testing.T) oaRun {
	t.Helper()
	st, err := oaLoad()
	if err != nil || st.Run == nil {
		t.Fatalf("도는 실행이 없다: %v %+v", err, st)
	}
	return *st.Run
}

func grepN(ls []string, s string) int {
	n := 0
	for _, l := range ls {
		if strings.Contains(l, s) {
			n++
		}
	}
	return n
}

// ① 켬·새 항목 → 한 번 띄움. 지시문 = 틀 + 목록·상태 한 장 경로·결과 파일, 띄울 명령에 실행 표식. 도는 동안은 다시 안 띄움
func TestOpsAgentLaunchOnce(t *testing.T) {
	oaFx(t, "ops_agent 조치")
	now := time.Now()
	opsAgentTick(now)
	l := oaLaunched(t)
	if len(l) != 1 {
		t.Fatalf("한 번 띄워야: %q", l)
	}
	r := oaRunNow(t)
	if r.Addr != "t9" || r.Mode != "조치" || !strings.HasPrefix(l[0], r.ID+" ") || len(r.Keys) == 0 {
		t.Fatalf("실행: %+v 띄움 %q", r, l[0])
	}
	b, err := os.ReadFile(strings.Fields(l[0])[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"# 운영 에이전트 틀", r.Dir + "/items.json", r.Dir + "/state.json", r.Dir + "/result", "free-wait:lane-c"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("지시문에 %q 없음:\n%s", w, b)
		}
	}
	if it, _ := os.ReadFile(r.Dir + "/items.json"); !strings.Contains(string(it), `"key":"free-wait:lane-c"`) {
		t.Errorf("목록 json: %s", it)
	}
	if s, _ := os.ReadFile(r.Dir + "/state.json"); !strings.Contains(string(s), `"fingerprint"`) {
		t.Errorf("상태 json: %s", s)
	}
	opsAgentTick(now.Add(time.Minute))
	if len(oaLaunched(t)) != 1 {
		t.Fatalf("도는 동안 다시 띄움: %q", oaLaunched(t))
	}
}

// ③ 끔(기본) → 안 띄움. 설정 없음(띄울 명령·틀)도 안 띄움
func TestOpsAgentOff(t *testing.T) {
	oaFx(t)
	opsAgentTick(time.Now())
	if l := oaLaunched(t); len(l) != 0 {
		t.Fatalf("끔인데 띄움: %q", l)
	}
	oaFx(t, "ops_agent 조치")
	if err := os.Remove(DATA + "/conf/ops-agent.md"); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(time.Now())
	if l := oaLaunched(t); len(l) != 0 {
		t.Fatalf("틀 없는데 띄움: %q", l)
	}
}

// ⑨ 결과 파일 → 끝: 요약 한 줄 사건·일지, 「사람이 정한다」 줄은 사건, 「하려던 것」은 일지. ② 같은 항목이 그대로면 다시 안 띄움
func TestOpsAgentFinishSummary(t *testing.T) {
	oaFx(t, "ops_agent 조치")
	now := time.Now()
	opsAgentTick(now)
	r := oaRunNow(t)
	res := "겹침 대기 lane-c를 풀었다\n사람이 정한다: lane-b 선행 — 산출물이 필요한지 모름, 권함 기다림\n하려던 것: herdr-sync — 병합 줄이 차 있어 미룸\n"
	if err := os.WriteFile(r.Dir+"/result", []byte(res), 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(time.Minute))
	rep := reports(t)
	if grepN(rep, "--name hc-fill-agent") != 2 || grepN(rep, "중간 - 운영 에이전트 끝: 겹침 대기 lane-c를 풀었다 (끝남, 조치") != 1 ||
		grepN(rep, "막힘 - 사람이 정한다: lane-b 선행") != 1 {
		t.Fatalf("사건: %q", rep)
	}
	jb, _ := os.ReadFile(DATA + "/journal.jsonl")
	if !strings.Contains(string(jb), `"action":"agent 끝"`) || !strings.Contains(string(jb), `"action":"하려던 것"`) {
		t.Errorf("일지: %s", jb)
	}
	st, _ := oaLoad()
	if st.Run != nil || len(st.Recent) != 1 || st.Recent[0].Summary != "겹침 대기 lane-c를 풀었다" {
		t.Fatalf("끝 기록: %+v", st)
	}
	for i := 2; i < 5; i++ {
		opsAgentTick(now.Add(time.Duration(i) * time.Minute))
	}
	if l := oaLaunched(t); len(l) != 1 {
		t.Fatalf("같은 항목으로 다시 띄움: %q", l)
	}
}

// ② ⑧ 띄우기 실패는 바퀴를 막지 않고 다음 바퀴에 다시 — ops_agent_tries(2)번 뒤에는 안 띄우고 항목마다 「사람이 정한다」 한 번
func TestOpsAgentTriesThenHuman(t *testing.T) {
	oaFx(t, "ops_agent 조치")
	confLines = append([]string{`ops_agent_launch printf 'x\n' >>'` + DATA + `/launched'; exit 1`}, confLines...)
	now := time.Now()
	for i := 0; i < 5; i++ {
		opsAgentTick(now.Add(time.Duration(i) * time.Minute))
	}
	if l := oaLaunched(t); len(l) != 2 {
		t.Fatalf("두 번까지 띄워 봐야: %q", l)
	}
	st, _ := oaLoad()
	if st.Run != nil || len(st.Keys) == 0 {
		t.Fatalf("상태: %+v", st)
	}
	asks := grepN(reports(t), "번 띄웠는데 못 풀림")
	if asks != len(st.Keys) {
		t.Fatalf("항목마다 한 번(%d개)이어야: %d — %q", len(st.Keys), asks, reports(t))
	}
	jb, _ := os.ReadFile(DATA + "/journal.jsonl")
	if strings.Count(string(jb), `"result":"실패: 종료 1"`) != 2 {
		t.Errorf("일지에 실패 두 줄: %s", jb)
	}
}

// ⑤ 시간당 상한 — 1이면 실패한 뒤에도 그 시간 안에는 다시 안 띄움
func TestOpsAgentHourCap(t *testing.T) {
	oaFx(t, "ops_agent 조치", "ops_agent_max_hour 1")
	confLines = append([]string{`ops_agent_launch printf 'x\n' >>'` + DATA + `/launched'; exit 1`}, confLines...)
	now := time.Now()
	opsAgentTick(now)
	opsAgentTick(now.Add(10 * time.Minute))
	if l := oaLaunched(t); len(l) != 1 {
		t.Fatalf("상한 1: %q", l)
	}
	opsAgentTick(now.Add(61 * time.Minute))
	if l := oaLaunched(t); len(l) != 2 {
		t.Fatalf("한 시간 뒤에는 다시: %q", l)
	}
}

// 시간 넘김 → 끝(결과 없음 — 끝까지 못 봄)이라 다음 바퀴에 다시 띄움 · 실행 중 틀이 바뀌면 손잡이를 끔으로 두고 사람에게
func TestOpsAgentTimeoutAndTamper(t *testing.T) {
	oaFx(t, "ops_agent 조치")
	now := time.Now()
	opsAgentTick(now)
	opsAgentTick(now.Add(21 * time.Minute)) // ops_agent_timeout 기본 20분
	if grepN(reports(t), "(시간 넘김, 조치") != 1 {
		t.Fatalf("시간 넘김 끝: %q", reports(t))
	}
	opsAgentTick(now.Add(22 * time.Minute))
	if l := oaLaunched(t); len(l) != 2 {
		t.Fatalf("시간 넘김 뒤 다시: %q", l)
	}
	if err := os.WriteFile(DATA+"/conf/ops-agent.md", []byte("바뀐 틀 — 권한 넓힘\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opsAgentTick(now.Add(50 * time.Minute))
	es, err := knob.Load(knob.Path(HOME))
	if err != nil || es["ops_agent"].Value != "끔" {
		t.Fatalf("틀이 바뀌면 끔으로: %v %+v", err, es)
	}
	if grepN(reports(t), "틀(ops-agent.md)이 바뀌었다") != 1 {
		t.Fatalf("사건: %q", reports(t))
	}
}
