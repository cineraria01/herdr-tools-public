package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// restDirs: 쉼 판정이 읽는 세션·대화 기록 폴더를 임시 폴더로 바꾼다
func restDirs(t *testing.T) (sess, proj string) {
	d := t.TempDir()
	sess, proj = filepath.Join(d, "sessions"), filepath.Join(d, "projects")
	os.MkdirAll(sess, 0o755)
	os.MkdirAll(proj, 0o755)
	oldS, oldP, oldT := sessDir, projDir, taskRoot
	sessDir, projDir, taskRoot = sess, proj, filepath.Join(d, "ctmp", "claude-1")
	t.Cleanup(func() { sessDir, projDir, taskRoot = oldS, oldP, oldT })
	return sess, proj
}

func put(t *testing.T, p, s string, age time.Duration) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-age)
	os.Chtimes(p, at, at)
}

// quiet: 세션은 터미널 자손(ps)에서 찾고, 그 세션 id의 기록만 본다(같은 폴더의 다른 세션은 안 침)
func TestQuietSignals(t *testing.T) {
	sess, proj := restDirs(t)
	now := float64(time.Now().UnixMilli())
	r3 := 360 * minute
	ps := fakeRun(0, " 100 1\n 101 100\n 102 101\n", "", nil)
	term := map[string]any{"pid": 100.0}
	if q := quiet(term, now, r3, ps); q.state != restQuiet || q.sid != "" {
		t.Fatalf("세션 파일 없음 = 신호 없음(쉼): %+v", q)
	}
	put(t, sess+"/102.json", `{"sessionId":"s1"}`, 0)
	put(t, proj+"/-x/s1.jsonl", "{}", 40*time.Minute)
	put(t, proj+"/-x/s2/subagents/agent-o.meta.json", `{}`, 0) // 남의 세션
	q := quiet(term, now, r3, ps)
	if q.state != restQuiet || q.sid != "s1" || now-q.last < 39*minute {
		t.Fatalf("손자 세션 s1, 활동 40분 전: %+v", q)
	}
	put(t, proj+"/-x/s1/subagents/agent-a.meta.json", `{"requestShape":"background"}`, 7*time.Hour)
	if q = quiet(term, now, r3, ps); q.state != restQuiet {
		t.Fatalf("끝 표시 없이 r3 넘게 조용한 하위 에이전트는 활동 아님: %+v", q)
	}
	put(t, proj+"/-x/s1/subagents/agent-a.jsonl", "{}", time.Minute)
	os.Symlink(proj+"/-x", proj+"/-old-name") // 옛 폴더 이름 링크(같은 기록이 두 번 보임)
	if q = quiet(term, now, r3, ps); q.state != restBusy || !strings.Contains(q.sig, "1개") {
		t.Fatalf("돌고 있는 하위 에이전트: %+v", q)
	}
	// 지휘 결정(2026-11-28, 감사 atp6urq M1): 주 기록의 끝남 알림은 증거가 아니다 — 끝난 증거는 그 하위 에이전트 자기 기록(finished·끝 줄 end_turn)뿐.
	// TaskStop으로 멈춘 배경(끝 줄은 user — 끊김)은 r3 안이면 활동, r3 지나면 쉼
	put(t, proj+"/-x/s1.jsonl", `{"content":"<task-notification>\n<task-id>a</task-id>\n<status>killed</status>"}`+"\n", 40*time.Minute)
	put(t, proj+"/-x/s1/subagents/agent-a.jsonl", `{"type":"user","message":{"content":"[Request interrupted by user]"}}`+"\n", 30*time.Minute)
	if q = quiet(term, now, r3, ps); q.state != restBusy {
		t.Fatalf("멈춘 배경 하위 에이전트(알림 있음) r3 안: 활동 %+v", q)
	}
	put(t, proj+"/-x/s1/subagents/agent-a.meta.json", `{"requestShape":"background"}`, 7*time.Hour)
	put(t, proj+"/-x/s1/subagents/agent-a.jsonl", `{"type":"user","message":{"content":"[Request interrupted by user]"}}`+"\n", 7*time.Hour)
	if q = quiet(term, now, r3, ps); q.state != restQuiet {
		t.Fatalf("멈춘 배경 하위 에이전트 r3 지남: 쉼 %+v", q)
	}
	// 다시 불린 꼴(감사 atp6urq): finished 없음 + 끝 줄 tool_use + 주 기록에 앞 알림 있음 → 활동
	put(t, proj+"/-x/s1/subagents/agent-a.meta.json", `{"requestShape":"background"}`, time.Minute)
	put(t, proj+"/-x/s1/subagents/agent-a.jsonl", `{"type":"assistant","message":{"stop_reason":"tool_use"}}`+"\n", time.Minute)
	if q = quiet(term, now, r3, ps); q.state != restBusy {
		t.Fatalf("다시 불린 하위 에이전트(앞 알림 남음, 도구 중): 활동 %+v", q)
	}
	put(t, proj+"/-x/s1.jsonl", "{}\n", 40*time.Minute)
	// 감사 atp6tdq M1·L2: 팀원(name 준 배경 에이전트 — 끝 표시 없음)·전경·꼴 모르는 meta도 끝난 증거(자기 기록 끝 end_turn)가 없으면 활동
	team := `{"requestShape":"background","taskKind":"in_process_teammate","teamName":"session-x"}`
	toolUse := `{"type":"assistant","message":{"stop_reason":"tool_use"}}` + "\n" + `{"type":"attachment"}` + "\n"
	endTurn := `{"type":"user","message":{}}` + "\n" + `{"type":"assistant","message":{"stop_reason":"end_turn"}}` + "\n" + `{"type":"attachment"}` + "\n"
	for _, m := range []string{team, `{"requestShape":"foreground"}`, `{}`} {
		put(t, proj+"/-x/s1/subagents/agent-a.meta.json", m, time.Minute)
		put(t, proj+"/-x/s1/subagents/agent-a.jsonl", toolUse, 4*time.Minute)
		if q = quiet(term, now, r3, ps); q.state != restBusy {
			t.Fatalf("도구를 쓰는 중 %s: 활동이어야 %+v", m, q)
		}
		put(t, proj+"/-x/s1/subagents/agent-a.jsonl", strings.Repeat(`{"type":"progress"}`+"\n", 3000)+endTurn, time.Minute)
		if q = quiet(term, now, r3, ps); q.state != restQuiet || now-q.last > 2*minute {
			t.Fatalf("턴을 마치고 쉼(end_turn) %s: 쉼, 마지막 활동 1분 전 %+v", m, q)
		}
	}
	put(t, proj+"/-x/s1/subagents/agent-a.meta.json", `{"finished":true}`, time.Minute)
	if q = quiet(term, now, r3, ps); q.state != restQuiet || now-q.last > 2*minute {
		t.Fatalf("끝난 하위 에이전트: 쉼, 마지막 활동 1분 전: %+v", q)
	}
	put(t, proj+"/-x/s1/subagents/agent-a.meta.json", `{깨짐`, 0)
	if q = quiet(term, now, r3, ps); q.state != restUnknown {
		t.Fatalf("meta 못 읽음 = 모름: %+v", q)
	}
	if q = quiet(term, now, r3, fakeRun(1, "", "", nil)); q.state != restUnknown || q.sig != "ps 실패" {
		t.Fatalf("ps 실패 = 모름: %+v", q)
	}
	put(t, sess+"/102.json", `{"sessionId":"../x"}`, 0)
	if q = quiet(term, now, r3, ps); q.state != restUnknown {
		t.Fatalf("경로 글자 든 세션 id = 모름: %+v", q)
	}
	os.RemoveAll(proj)
	put(t, sess+"/102.json", `{"sessionId":"s1"}`, 0)
	if q = quiet(term, now, r3, ps); q.state != restQuiet {
		t.Fatalf("기록 폴더 없는 기계 = 신호 없음: %+v", q)
	}
}

// settle: 규칙이 나와도 쉼이 아니면 안 닫고, 모름은 R3만, 턴 뒤 활동은 쉰 시간을 마지막 활동부터 센다
func TestSettle(t *testing.T) {
	sess, proj := restDirs(t)
	now := float64(time.Now().UnixMilli())
	ps := fakeRun(0, "", "", nil)
	tail := ""
	f := facts{worktree: no, unlanded: no, tail: &tail}
	lane := func(idle time.Duration) map[string]any {
		return map[string]any{"address": "t1", "name": "lane-a", "state": "live", "status": map[string]any{"app": "claude-code", "state": "done"},
			"labels": map[string]any{"hc-lane": "1"}, "created_ms": now - 9*60*minute, "status_ms": now - float64(idle.Milliseconds()), "turn": 4.0, "pid": 7.0}
	}
	if why, _, q := settle(lane(33*time.Minute), f, now, nil, ps); why != "R2" || q.state != restQuiet {
		t.Fatalf("세션 없음: 지금 규칙(R2) %q %+v", why, q)
	}
	put(t, sess+"/7.json", `{"sessionId":"s7"}`, 0)
	put(t, proj+"/-p/s7/subagents/agent-a.meta.json", `{"requestShape":"background"}`, 2*time.Minute)
	why, kept, q := settle(lane(33*time.Minute), f, now, nil, ps)
	if why != "" || !strings.Contains(kept, "안에서 일이 도는 중") {
		t.Fatalf("① 하위 에이전트 돎: %q %q", why, kept)
	}
	if r := closeTerm(lane(33*time.Minute), "R2", q); !strings.HasPrefix(r, "남김") {
		t.Fatalf("쉼 아님은 closeTerm도 거절: %s", r)
	}
	sub := f
	sub.submitted = true
	if why, _, _ = settle(lane(time.Minute), sub, now, nil, ps); why != "" {
		t.Fatal("④ 제출 레인 빠른 닫기도 쉼 판정을 거침")
	}
	// 감사 atp6tdq M1: 도구를 쓰는 중인 팀원(4분 전 기록) + 제출 레인(상태 3분 전) → 안 닫음
	put(t, proj+"/-p/s7/subagents/agent-a.meta.json", `{"requestShape":"background","teamName":"session-x"}`, 4*time.Minute)
	put(t, proj+"/-p/s7/subagents/agent-a.jsonl", `{"type":"assistant","message":{"stop_reason":"tool_use"}}`+"\n", 4*time.Minute)
	if why, kept, _ = settle(lane(3*time.Minute), sub, now, nil, ps); why != "" {
		t.Fatalf("④ 제출 레인 + 도구 중인 팀원: 안 닫음 %q %q", why, kept)
	}
	os.Remove(proj + "/-p/s7/subagents/agent-a.jsonl")
	put(t, proj+"/-p/s7/subagents/agent-a.meta.json", `{"finished":true}`, 10*time.Minute)
	if why, kept, _ = settle(lane(33*time.Minute), f, now, nil, ps); why != "" || !strings.Contains(kept, "대화 기록이") {
		t.Fatalf("⑤ 턴 뒤 10분 전 활동: 아직 %q %q", why, kept)
	}
	if why, _, _ = settle(lane(33*time.Minute), sub, now, nil, ps); why != "" {
		t.Fatal("⑤ 턴 뒤 활동이 있으면 제출 레인도 R2 문턱부터")
	}
	put(t, proj+"/-p/s7/subagents/agent-a.meta.json", `{"finished":true}`, 31*time.Minute)
	if why, _, q = settle(lane(50*time.Minute), f, now, nil, ps); why != "R2" || q.state != restQuiet {
		t.Fatalf("⑤ 활동이 끊기고 문턱 지남: R2 %q", why)
	}
	put(t, sess+"/7.json", `{깨짐`, 0)
	if why, kept, _ = settle(lane(33*time.Minute), f, now, nil, ps); why != "" || !strings.Contains(kept, "쉼 모름") {
		t.Fatalf("③ 모름: 안 닫음 %q %q", why, kept)
	}
	if why, _, q = settle(lane(7*60*time.Minute), facts{worktree: no, unlanded: yes, tail: &tail}, now, nil, ps); why != "R3" || q.state != restQuiet {
		t.Fatalf("③ 모름이 r3만큼(상태 그대로 7시간): R3 %q %+v", why, q)
	}
}

// ht-sweep-busy-tasks: 그 세션의 배경 작업 파일(<taskRoot>/<폴더>/<세션 id>/tasks/*.output — 끝나면 끝에 [exited with code N]·[killed]).
// 사고 꼴: 끝난 하위 에이전트(finished·end_turn)가 run_in_background로 띄운 셸이 1시간 50분째 조용히(0바이트) 돎 + 상태 done 33분
func TestQuietBackgroundTasks(t *testing.T) {
	sess, proj := restDirs(t)
	now := float64(time.Now().UnixMilli())
	r3 := 360 * minute
	ps := fakeRun(0, "", "", nil)
	tail := ""
	f := facts{worktree: no, unlanded: no, tail: &tail}
	sub := f
	sub.submitted = true
	lane := func(idle time.Duration) map[string]any {
		return map[string]any{"address": "t1", "name": "lane-a", "state": "live", "status": map[string]any{"app": "claude-code", "state": "done"},
			"labels": map[string]any{"hc-lane": "1"}, "created_ms": now - 9*60*minute, "status_ms": now - float64(idle.Milliseconds()), "turn": 4.0, "pid": 7.0}
	}
	term := lane(33 * time.Minute)
	put(t, sess+"/7.json", `{"sessionId":"s7"}`, 0)
	put(t, proj+"/-p/s7.jsonl", "{}\n", 40*time.Minute)
	put(t, proj+"/-p/s7/subagents/agent-a.meta.json", `{"requestShape":"background","finished":true}`, 40*time.Minute)
	put(t, proj+"/-p/s7/subagents/agent-a.jsonl", `{"type":"assistant","message":{"stop_reason":"end_turn"}}`+"\n", 40*time.Minute)
	tasks := taskRoot + "/-p/s7/tasks/"
	put(t, tasks+"bq1.output", "", 110*time.Minute)
	// ① 사고 꼴: 안 닫음·까닭 ② 제출 레인 빠른 닫기도
	if q := quiet(term, now, r3, ps); q.state != restBusy || q.sig != "배경 작업 1개 도는 중" {
		t.Fatalf("① 끝난 하위 에이전트가 남긴 배경 셸: 활동 %+v", q)
	}
	if why, kept, _ := settle(term, f, now, nil, ps); why != "" || kept != "안에서 일이 도는 중(배경 작업 1개 도는 중)" {
		t.Fatalf("① R2 안 닫음 %q %q", why, kept)
	}
	if why, _, _ := settle(lane(time.Minute), sub, now, nil, ps); why != "" {
		t.Fatal("② 제출 레인 빠른 닫기도 안 닫음")
	}
	// 바퀴가 한참 안 돌다 돎(상태 7시간 그대로 — R3 규칙)이어도 r3 안에 생긴 배경 작업이 돌면 안 닫음
	r3f := facts{worktree: no, unlanded: yes, tail: &tail}
	if judged, _ := judge(lane(7*60*time.Minute), r3f, now, nil); judged != "R3" {
		t.Fatalf("시험 꼴: 규칙만으로는 R3 %q", judged)
	}
	if why, _, _ := settle(lane(7*60*time.Minute), r3f, now, nil, ps); why != "" {
		t.Fatal("늦은 바퀴: 도는 배경 작업이 있으면 R3도 안 닫음")
	}
	// ③ 끝 표시가 붙으면 닫을 후보(수정 시각은 끝난 때 — 문턱 넘게 전)
	for _, end := range []string{"\n[exited with code 0]\n", "out\n\n[killed]\n", "\n[exited with code unknown]\n"} {
		put(t, tasks+"bq1.output", end, 35*time.Minute)
		if why, _, q := settle(term, f, now, nil, ps); why != "R2" || q.state != restQuiet {
			t.Fatalf("③ 끝 표시 %q: R2 %q %+v", end, why, q)
		}
	}
	// 끝난 지 5분(턴 뒤 활동) → 쉰 시간은 그때부터
	put(t, tasks+"bq1.output", "\n[exited with code 0]\n", 5*time.Minute)
	if why, kept, _ := settle(term, f, now, nil, ps); why != "" || !strings.Contains(kept, "분 전에 바뀜") {
		t.Fatalf("배경 작업이 5분 전에 끝남: 아직 %q %q", why, kept)
	}
	put(t, tasks+"bq1.output", "\n[exited with code 0]\n", 35*time.Minute)
	// ④ 끝 표시 없이 r3 넘게 조용함 → 죽은 것(안 침)
	put(t, tasks+"bq2.output", "", 7*time.Hour)
	// ⑤ 하위 에이전트 링크·깨진 링크·output 아닌 파일·폴더 → 안 셈
	os.Symlink(proj+"/-p/s7/subagents/agent-a.jsonl", tasks+"a1.output")
	os.Symlink(tasks+"없음", tasks+"a2.output")
	put(t, tasks+"note.txt", "x", 0)
	os.MkdirAll(tasks+"bdir.output", 0o755)
	put(t, taskRoot+"/-p/s8/tasks/bz.output", "", 0) // 남의 세션
	put(t, taskRoot+"/stray-file", "", 0)            // 뿌리의 파일
	if q := quiet(term, now, r3, ps); q.state != restQuiet {
		t.Fatalf("④⑤ 오래된 미완·링크·파일·남의 세션: 쉼 %+v", q)
	}
	// ⑨ 큰 출력 + 끝 표시 → 끝남, 끝 표시 글이 가운데에만 → 도는 중
	big := strings.Repeat("line of output\n", 80000)
	put(t, tasks+"bq3.output", big+"\n[exited with code 1]\n", 35*time.Minute)
	if q := quiet(term, now, r3, ps); q.state != restQuiet {
		t.Fatalf("⑨ 큰 출력 + 끝 표시: 쉼 %+v", q)
	}
	put(t, tasks+"bq3.output", big+"\n[exited with code 1]\n"+big, 35*time.Minute)
	if q := quiet(term, now, r3, ps); q.state != restBusy {
		t.Fatalf("⑨ 끝 표시 글이 가운데에만: 도는 중 %+v", q)
	}
	// 외부 가정: 끝 표시 글이 다른 Claude 판 → 도는 중으로(닫지 않는 쪽, r3까지)
	put(t, tasks+"bq3.output", "[exit 0]\n", 35*time.Minute)
	if q := quiet(term, now, r3, ps); q.state != restBusy {
		t.Fatalf("모르는 끝 표시: 도는 중 %+v", q)
	}
	os.Remove(tasks + "bq3.output")
	// ⑧ 하위 에이전트 활동 + 배경 작업 활동 → 한 줄에 둘 다
	put(t, proj+"/-p/s7/subagents/agent-b.meta.json", `{"requestShape":"background"}`, time.Minute)
	put(t, tasks+"bq4.output", "", time.Minute)
	put(t, tasks+"bq5.output", "partial", 2*time.Minute)
	if q := quiet(term, now, r3, ps); q.state != restBusy || q.sig != "배경 하위 에이전트 1개 안 끝남 · 배경 작업 2개 도는 중" {
		t.Fatalf("⑧ 둘 다: %+v", q)
	}
	os.Remove(proj + "/-p/s7/subagents/agent-b.meta.json")
	os.Remove(tasks + "bq4.output")
	os.Remove(tasks + "bq5.output")
	// 이미 쌓인 상태에 처음 돎: 끝 표시 붙은 파일 수백 개 + 끝 표시 없는 옛 파일 → 쉼·R2
	for i := 0; i < 300; i++ {
		put(t, fmt.Sprintf("%sbold%03d.output", tasks, i), fmt.Sprintf("x\n\n[exited with code %d]\n", i%3), 2*time.Hour)
	}
	for i := 0; i < 5; i++ {
		put(t, fmt.Sprintf("%sbdead%d.output", tasks, i), "", 20*time.Hour)
	}
	if why, _, q := settle(term, f, now, nil, ps); why != "R2" || q.state != restQuiet {
		t.Fatalf("쌓인 세션: R2 %q %+v", why, q)
	}
	// ⑥ tasks 폴더를 못 읽음 → 모름(R3만)
	os.Chmod(tasks, 0)
	t.Cleanup(func() { os.Chmod(tasks, 0o755) })
	if q := quiet(term, now, r3, ps); q.state != restUnknown || q.sig != "배경 작업 못 읽음" {
		t.Fatalf("⑥ 못 읽음 = 모름: %+v", q)
	}
	if why, kept, _ := settle(term, f, now, nil, ps); why != "" || !strings.Contains(kept, "쉼 모름") {
		t.Fatalf("⑥ 모름은 R2 안 닫음 %q %q", why, kept)
	}
	if why, _, _ := settle(lane(7*60*time.Minute), r3f, now, nil, ps); why != "R3" {
		t.Fatalf("⑥ 모름이 r3만큼: R3 %q", why)
	}
	os.Chmod(tasks, 0o755)
	// ⑦ 그 세션 폴더 없음·임시 뿌리 없음 → 신호 없음(지금 규칙)
	os.RemoveAll(taskRoot + "/-p/s7")
	if why, _, _ := settle(term, f, now, nil, ps); why != "R2" {
		t.Fatalf("⑦ 세션 폴더 없음: R2 %q", why)
	}
	os.RemoveAll(taskRoot)
	if why, _, _ := settle(term, f, now, nil, ps); why != "R2" {
		t.Fatalf("⑦ 임시 뿌리 없음: R2 %q", why)
	}
	// 대화 기록 폴더가 없어도 배경 작업은 본다
	os.RemoveAll(proj)
	put(t, tasks+"bq6.output", "", time.Minute)
	if q := quiet(term, now, r3, ps); q.state != restBusy {
		t.Fatalf("기록 폴더 없음 + 도는 배경 작업: 활동 %+v", q)
	}
}

// 닫는 길은 하나: hc kill은 closeTerm에만, closeTerm은 settle이 준 쉼 판정(rested)으로만 부른다. 닫는 규칙·길을 더하며 이것을 건너뛰면 실패한다
func TestEveryCloseGoesThroughSettle(t *testing.T) {
	if r := closeTerm(map[string]any{"address": "t1"}, "R2", rested{}); !strings.HasPrefix(r, "남김") {
		t.Fatalf("0값 쉼 판정으로 닫음: %s", r)
	}
	files, _ := filepath.Glob("*.go")
	kill := regexp.MustCompile(`hc\("kill"`)
	call := regexp.MustCompile(`closeTerm\(`)
	kills, calls := 0, 0
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(p)
		s := string(b)
		for _, fn := range strings.Split(s, "\nfunc ")[1:] {
			name := fn[:strings.IndexAny(fn, "( ")]
			if n := len(kill.FindAllString(fn, -1)); n > 0 && name != "closeTerm" {
				t.Errorf("%s의 %s가 closeTerm 밖에서 hc kill", p, name)
			} else {
				kills += n
			}
			for _, m := range call.FindAllStringIndex(fn, -1) {
				if strings.HasPrefix(fn, "closeTerm(") && m[0] == 0 {
					continue
				}
				calls++
				if i := strings.Index(fn, ":= settle("); i < 0 || i > m[0] {
					t.Errorf("%s의 %s가 settle 없이 closeTerm을 부름", p, name)
				}
			}
		}
	}
	if kills != 1 || calls != 1 {
		t.Fatalf("hc kill %d곳·closeTerm 부름 %d곳(하나씩이어야)", kills, calls)
	}
}
