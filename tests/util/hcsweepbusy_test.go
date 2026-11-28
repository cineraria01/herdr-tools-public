package util_test

// ht-sweep-busy-idle: 상태는 done인데 안에서 배경 하위 에이전트가 도는 Claude 터미널을 닫지 않는다(docs/hc-sweep/orphans.md 「정말 쉬는가」).
// CLI 실제 경로(hc ls → hc get → settle → closeTerm → hc kill)를 가짜 hc·임시 HOME의 세션 파일·대화 기록으로 본다

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bzSweep: 쉰 지 33분 된 worker 레인 t1(pid 4242 — 세션 파일 sessions/4242.json, 세션 id sid-1)과 그 대화 기록, 작업 임시 폴더 하나.
// meta는 하위 에이전트 meta.json 내용(""이면 하위 기록 없음), metaAge는 그 기록의 나이, submitted면 이 실행의 「제출」 사건
func bzSweep(t *testing.T, meta string, metaAge time.Duration, submitted bool, session bool, args ...string) (T string, e Env, out string, rc int) {
	T, e = bzSetup(t, meta, metaAge, submitted, session)
	out, rc = run(T, e, tool(t, "hc-sweep"), append([]string{"--only-under", T}, args...)...)
	return T, e, out, rc
}

// bzSetup: bzSweep의 꼴만(돌리지 않음). Claude 배경 작업 뿌리는 CLAUDE_CODE_TMPDIR로 임시 폴더 안(bzTask가 쓴다)
func bzSetup(t *testing.T, meta string, metaAge time.Duration, submitted bool, session bool) (T string, e Env) {
	T, e, _ = j2Sweep(t)
	e = e.With("CLAUDE_CODE_TMPDIR=" + T + "/ctmp")
	R := T + "/jroot"
	write(t, T+"/cfg/config", "sweep.disk_free_gb 0\njobtmp.root "+R+"\n")
	created := (time.Now().Unix() - 3*3600) * 1000
	changed := (time.Now().Unix() - 33*60) * 1000
	write(t, T+"/ls.json", `{"terminals":[{"address":"t1","name":"lane-a","state":"live","status":"done"}]}`+"\n")
	write(t, T+"/get-t1.json", fmt.Sprintf(`{"address":"t1","name":"lane-a","state":"live","status":{"app":"claude-code","state":"done"},`+
		`"labels":{"hc-lane":"1"},"created_ms":%d,"status_ms":%d,"turn":4,"pid":4242,"cwd":"/nonexistent-bz/lane-a"}`+"\n", created, changed))
	if session {
		write(t, T+"/home/.claude/sessions/4242.json", `{"pid":4242,"sessionId":"sid-1","cwd":"/nonexistent-bz/lane-a","status":"idle"}`)
	}
	proj := T + "/home/.claude/projects/-nonexistent-bz-lane-a"
	write(t, proj+"/sid-1.jsonl", "{}\n")
	old := time.Now().Add(-40 * time.Minute)
	os.Chtimes(proj+"/sid-1.jsonl", old, old)
	if meta != "" {
		at := time.Now().Add(-metaAge)
		for _, f := range []string{"agent-a1.meta.json", "agent-a1.jsonl"} {
			write(t, proj+"/sid-1/subagents/"+f, meta)
			os.Chtimes(proj+"/sid-1/subagents/"+f, at, at)
		}
	}
	// 남의 세션(같은 폴더의 다른 세션 id)이 막 바뀌었어도 이 터미널의 활동으로 치지 않는다
	write(t, proj+"/sid-other/subagents/agent-z.meta.json", `{"requestShape":"background"}`)
	write(t, R+"/abc123/.hc-job", fmt.Sprintf("t1\nlane-a\n%d\n", time.Now().Unix()-3*3600))
	jold := time.Now().Add(-3 * time.Hour)
	os.Chtimes(R+"/abc123/.hc-job", jold, jold)
	os.Chtimes(R+"/abc123", jold, jold)
	if submitted {
		write(t, T+"/home/.local/state/hc-lane/events.tsv",
			fmt.Sprintf("2026-11-21T10:00:00Z\tm\tt1\tlane-a\t제출\tabc1234\t-\t4\t-\tt1@%d\n", created))
	}
	script(t, T+"/bin/hc", T, `#!/bin/bash
echo "$*" >>"@T@/hc.log"
case $1 in
  ls) exec cat "@T@/ls.json" ;;
  get) [ -e "@T@/killed-$2" ] && exec sed 's/"state":"live"/"state":"finished"/' "@T@/get-$2.json"
       [ -f "@T@/get-$2.json" ] && exec cat "@T@/get-$2.json"; exit 1 ;;
  read) echo "  ⏵⏵ bypass permissions on (shift+tab to cycle)" ;;
  kill) touch "@T@/killed-$2" ;;
  rm) ;;
  *) exit 1 ;;
esac
`)
	return T, e
}

// bzTask: 세션 sid-1의 배경 작업 출력 파일(Claude 꼴 <뿌리>/claude-<uid>/<폴더 이름>/<세션 id>/tasks/<작업 id>.output)
func bzTask(t *testing.T, T, id, body string, age time.Duration) {
	p := fmt.Sprintf("%s/ctmp/claude-%d/-nonexistent-bz-lane-a/sid-1/tasks/%s.output", T, os.Getuid(), id)
	write(t, p, body)
	at := time.Now().Add(-age)
	os.Chtimes(p, at, at)
}

// bzHeld: 가짜 lsof(hcsweepjobtmp_test.go와 같은 꼴) — 부른 hc-sweep($PPID)의 cwd와 배경 작업 ids의 파일(링크를 푼 경로)을 다른 프로세스가 연 것으로.
// ids가 없으면 아무 작업 파일도 안 쥠
func bzHeld(t *testing.T, T string, ids ...string) {
	script(t, T+"/bin/lsof", T, "#!/bin/sh\nprintf 'p%s\\nfcwd\\nn/\\n' \"$PPID\"\n"+
		"while read -r p; do [ -n \"$p\" ] && printf 'p1\\nf1\\nn%s\\n' \"$p\"; done < @T@/lsof-held\nexit 0\n")
	held := ""
	for _, id := range ids {
		p, err := filepath.EvalSymlinks(fmt.Sprintf("%s/ctmp/claude-%d/-nonexistent-bz-lane-a/sid-1/tasks/%s.output", T, os.Getuid(), id))
		if err != nil {
			t.Fatal(err)
		}
		held += p + "\n"
	}
	write(t, T+"/lsof-held", held)
}

// ht-sweep-busy-tasks 사고 꼴: 끝난 하위 에이전트(finished·끝 줄 end_turn, 40분 전) + 그 에이전트가 띄운 배경 셸이 1시간 50분째 조용히 돎(0바이트·
// 끝 표시 없음, 셸이 그 파일을 쥠) + 상태 done 33분 → R2도 제출 뒤 빠른 닫기도 안 닫음, 남김 한 줄(두 번 돌아도 한 줄). 끝나면 다음 바퀴에 닫음 —
// 끝 표시가 붙었거나(제출 안 함 판), 끝 표시 없이 쥔 프로세스가 사라졌거나(제출 판 — ht-rest-task-held: 상한에 걸려 죽은 전경 명령 꼴)
func TestHcSweepBusyBackgroundTask(t *testing.T) {
	const ended = `{"requestShape":"background","finished":true,"type":"assistant","message":{"stop_reason":"end_turn"}}`
	for _, submitted := range []bool{false, true} {
		T, e := bzSetup(t, ended, 40*time.Minute, submitted, true)
		bzTask(t, T, "bshell1", "", 110*time.Minute)
		bzHeld(t, T, "bshell1")
		bzTask(t, T, "bold1", "done\n\n[exited with code 0]\n", 3*time.Hour)
		bzTask(t, T, "bold2", "\n[killed]\n", 3*time.Hour)
		out, rc := run(T, e, tool(t, "hc-sweep"), "--only-under", T)
		c := checker{t: t, out: &out, rc: &rc}
		c.ok(fmt.Sprintf("제출 %v: hc kill 없음", submitted), bzKills(T) == 0)
		c.ok("남김 줄", strings.Contains(out, "남김: t1 lane-a — 안에서 일이 도는 중(배경 작업 1개 도는 중)"))
		c.ok("closed.log 남김 한 줄", bzKept(T) == 1)
		c.ok("작업 임시 폴더 그대로", exists(T+"/jroot/abc123/.hc-job"))
		out, rc = run(T, e, tool(t, "hc-sweep"), "--only-under", T)
		c.ok("두 번째 바퀴: 남김 줄 되풀이 안 함·한 줄", !strings.Contains(out, "남김: t1") && bzKept(T) == 1 && bzKills(T) == 0)
		if submitted {
			bzHeld(t, T)
		} else {
			bzTask(t, T, "bshell1", "\n[exited with code 0]\n", 35*time.Minute)
		}
		out, rc = run(T, e, tool(t, "hc-sweep"), "--only-under", T)
		c.ok("배경 작업 끝남: 닫음", bzKills(T) == 1 && strings.Contains(out, "닫힘: R2 t1"))
	}
}

func bzKills(T string) int { return grepc(strings.Join(j2Calls(T), "\n"), `^kill t1`) }

func bzKept(T string) int {
	return grepc(read(T+"/home/.claude-tasks/closed.log"), "\t남김\\(안에서 일이 도는 중\\)\t")
}

const bzRunning = `{"agentType":"general-purpose","requestShape":"background"}`

// ① 상태 done 33분 + 돌고 있는 하위 에이전트 → 안 닫음·「남김」 한 줄 ⑥ 두 번 돌아도 기록 한 줄 ⑧ 작업 임시 폴더 그대로
func TestHcSweepBusyKept(t *testing.T) {
	T, e, out, rc := bzSweep(t, bzRunning, 2*time.Minute, false, true)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("하위 에이전트 돎: hc kill 없음", bzKills(T) == 0)
	c.ok("남김 줄", strings.Contains(out, "남김: t1 lane-a — 안에서 일이 도는 중(배경 하위 에이전트 1개 안 끝남)"))
	c.ok("closed.log 남김 한 줄", bzKept(T) == 1)
	c.ok("작업 임시 폴더 그대로", exists(T+"/jroot/abc123/.hc-job"))
	c.ok("종료 0", rc == 0)
	out, rc = run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c.ok("두 번째 바퀴: 남김 줄 되풀이 안 함", !strings.Contains(out, "남김: t1"))
	c.ok("두 번째 바퀴: closed.log 그대로 한 줄", bzKept(T) == 1)
	c.ok("두 번째 바퀴: hc kill 없음", bzKills(T) == 0)
}

// ② 하위 에이전트가 끝났고(finished) 기록이 문턱 넘게 조용함 → 지금처럼 닫음(작업 임시 폴더도 닫힘을 따라 지움)
func TestHcSweepBusyFinishedCloses(t *testing.T) {
	T, _, out, rc := bzSweep(t, `{"requestShape":"background","finished":true}`, 35*time.Minute, false, true)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("끝난 하위 에이전트: hc kill 1회", bzKills(T) == 1)
	c.ok("닫힘 줄", strings.Contains(out, "닫힘: R2 t1"))
	c.ok("작업 임시 폴더 지움", !exists(T+"/jroot/abc123"))
}

// ⑤ 하위 에이전트는 끝났지만 그 기록이 5분 전에 바뀜(턴이 끝난 뒤의 활동) → 쉰 시간은 그때부터 — 아직 안 닫음
func TestHcSweepBusyRecentActivity(t *testing.T) {
	T, _, out, rc := bzSweep(t, `{"requestShape":"background","finished":true}`, 5*time.Minute, false, true)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("5분 전 활동: hc kill 없음", bzKills(T) == 0)
	c.ok("남김 까닭: 대화 기록", strings.Contains(out, "안에서 일이 도는 중(대화 기록이 5분 전에 바뀜)"))
}

// ④ 제출한 레인(빠른 닫기 0분) + 돌고 있는 하위 에이전트 → 안 닫음
func TestHcSweepBusySubmittedKept(t *testing.T) {
	T, _, out, rc := bzSweep(t, bzRunning, time.Minute, true, true)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("제출 레인 + 하위 에이전트: hc kill 없음", bzKills(T) == 0)
}

// 세션 파일이 없는 터미널: Claude(status.app claude-code)면 자료가 없는 것이라 쉼 모름(안 닫음 — ht-rest-one-gate), Claude가 아닌 CLI면
// 신호 없음, 지금 규칙 그대로 닫음
func TestHcSweepBusyNoSessionCloses(t *testing.T) {
	T, e, out, rc := bzSweep(t, bzRunning, time.Minute, false, false)
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("Claude·세션 없음: hc kill 없음", bzKills(T) == 0)
	c.ok("Claude·세션 없음: 남김 줄", strings.Contains(out, "남김: t1 lane-a — 쉼 모름(Claude인데 세션 자료 없음)"))
	b, _ := os.ReadFile(T + "/get-t1.json")
	write(t, T+"/get-t1.json", strings.Replace(string(b), `"app":"claude-code"`, `"app":"codex"`, 1))
	out, rc = run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c.ok("Claude 아닌 CLI·세션 없음: hc kill 1회", bzKills(T) == 1)
}

// ht-rest-one-gate 묻기(hc-sweep --rest): 이 기계 터미널은 판정 한 줄, 다른 기계 주소(「기계/tN」)는 이 기계 자료로 판정하지 않고 모름(감사 atp7a3w L2 —
// 그 pid로 이 기계의 ps·세션 파일을 봤다). 도구 길(rest.Ask)은 늘 접두를 떼어 그 기계에서 묻는다
func TestHcSweepRestAsk(t *testing.T) {
	T, e := bzSetup(t, bzRunning, time.Minute, false, true)
	out, rc := run(T, e, tool(t, "hc-sweep"), "--rest", "t1")
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("이 기계: 활동 한 줄", rc == 0 && strings.TrimSpace(out) == "REST\tbusy\tsid-1\t배경 하위 에이전트 1개 안 끝남\tend")
	out, rc = run(T, e, tool(t, "hc-sweep"), "--rest", "box-1/t1")
	c.ok("다른 기계 주소: 모름, hc get 안 부름", rc == 0 && strings.HasPrefix(out, "REST\tunknown\t-\t다른 기계 주소") && !strings.Contains(read(T+"/hc.log"), "get box-1/t1"))
}

// ③ 신호 모름(세션 파일이 JSON 아님) → 안 닫음 ⑦ dry-run은 판정과 까닭만(기록 없음)
func TestHcSweepBusyUnknownKept(t *testing.T) {
	T, e, out, rc := bzSweep(t, bzRunning, time.Minute, false, true, "--dry-run")
	c := checker{t: t, out: &out, rc: &rc}
	c.ok("dry-run: 남김 까닭", strings.Contains(out, "남김: t1 lane-a — 안에서 일이 도는 중(배경 하위 에이전트 1개 안 끝남)"))
	c.ok("dry-run: closed.log 안 씀", bzKept(T) == 0)
	c.ok("dry-run: hc kill 없음", bzKills(T) == 0)
	write(t, T+"/home/.claude/sessions/4242.json", "{깨짐")
	out, rc = run(T, e, tool(t, "hc-sweep"), "--only-under", T)
	c.ok("모름: hc kill 없음", bzKills(T) == 0)
	c.ok("모름: 남김 줄", strings.Contains(out, "남김: t1 lane-a — 쉼 모름(세션 파일 JSON 아님)"))
}
