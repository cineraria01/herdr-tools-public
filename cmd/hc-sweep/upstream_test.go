package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/rest"
	"herdr-tools/internal/upstream"
)

// 상대 판 기록(ht-upstream-contract ①②·운영 장면): 가짜 hc --version

func upRun(hcOut string) runner {
	return func(_ time.Duration, _ []string, name string, args ...string) result {
		if name == "hc" && len(args) == 1 && args[0] == "--version" {
			if hcOut == "" {
				return result{rc: 1}
			}
			return result{out: hcOut + "\n"}
		}
		return result{rc: 127}
	}
}

type sent []string

func (s *sent) rep(ok bool) func(st, msg, key string) bool {
	return func(st, msg, key string) bool { *s = append(*s, st+"|"+key+"|"+msg); return ok }
}

func loadUp(t *testing.T, p string) *upstream.Rec {
	t.Helper()
	r, err := upstream.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// 처음 돌 때(이미 쌓인 상태): 세션이 없으면 기준을 안 적고, 있으면 지금 판들을 믿는 기준으로(사건 없음 — 보류 없이 시작, 점검은 차례로)
// → 같은 판 두 번째 실행은 사건 없음 → 새 판(한 기계에 판이 둘 — 옛 세션 옆 새 세션)은 소식 한 번·보류, 같은 판에 되풀이 없음
// → hc 판이 바뀌어도 새 묶음. 알림 실패는 다음 바퀴에 같은 키로
func TestUpstreamCheckBaselineThenChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "upstream.json")
	var s sent
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), nil, s.rep(true))
	if _, err := os.Stat(p); !os.IsNotExist(err) || len(s) != 0 {
		t.Fatalf("세션 없음: 기준을 안 적음 %v %v", err, s)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.1"}, s.rep(true))
	r := loadUp(t, p)
	if b := r.Bundles["claude-2.0.1_hc-0.1.22"]; len(s) != 0 || b == nil || b.State != upstream.Trusted || !b.Base || r.Hold("2.0.1") != "" {
		t.Fatalf("기준: %+v %v", r, s)
	}
	if id, held := r.Owed(); id != "claude-2.0.1_hc-0.1.22" || held {
		t.Fatalf("기준도 점검은 차례로 한 번(보류 없이): %s %v", id, held)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.1"}, s.rep(true))
	if len(s) != 0 {
		t.Fatalf("같은 판 두 번째: 사건 없음 %v", s)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.1", "2.0.2"}, s.rep(false))
	if len(s) != 1 || !strings.HasPrefix(s[0], "중간|hc-upstream:w1:claude-2.0.2_hc-0.1.22:new|에이전트 판 바뀜(w1): claude-2.0.1_hc-0.1.22 → claude-2.0.2_hc-0.1.22") {
		t.Fatalf("판 바뀜 소식: %v", s)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.1", "2.0.2"}, s.rep(true))
	if len(s) != 2 || !strings.Contains(s[1], ":new|") {
		t.Fatalf("알림 실패 → 같은 키로 다시: %v", s)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.2"}, s.rep(true))
	r = loadUp(t, p)
	if len(s) != 2 || r.Hold("2.0.2") == "" || r.Hold("2.0.1") != "" {
		t.Fatalf("되풀이 없음·새 판만 보류: %v %+v", s, r.Bundles)
	}
	if id, held := r.Owed(); id != "claude-2.0.2_hc-0.1.22" || !held {
		t.Fatalf("보류 묶음 먼저 점검: %s %v", id, held)
	}
	upstreamCheck("w1", p, false, upRun("hc 0.1.23"), []string{"2.0.1"}, s.rep(true))
	if len(s) != 3 || !strings.Contains(s[2], "claude-2.0.1_hc-0.1.23") {
		t.Fatalf("hc 판 바뀜: %v", s)
	}
}

// 판 못 읽음(hc --version 실패·꼴이 다름) → 모름: 사건 없음·기록 그대로. dry-run은 쓰지 않는다
func TestUpstreamCheckUnreadable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "upstream.json")
	var s sent
	upstreamCheck("w1", p, false, upRun("hc 0.1.22"), []string{"2.0.1"}, s.rep(true))
	before, _ := os.ReadFile(p)
	for _, out := range []string{"", "herdr-core 0.1.30", "hc"} {
		l := upstreamCheck("w1", p, false, upRun(out), []string{"9.9.9"}, s.rep(true))
		after, _ := os.ReadFile(p)
		if len(s) != 0 || string(after) != string(before) || len(l) != 1 || !strings.Contains(l[0], "모름") {
			t.Fatalf("%q: 모름이어야 %v %v", out, s, l)
		}
	}
	l := upstreamCheck("w1", p, true, upRun("hc 0.1.22"), []string{"9.9.9"}, s.rep(true))
	if after, _ := os.ReadFile(p); string(after) != string(before) || len(s) != 0 || !strings.Contains(strings.Join(l, " "), "보낼 것") {
		t.Fatalf("dry-run: 안 씀·안 보냄 %v %v", s, l)
	}
}

// 실물 점검 결과(⑤⑥): 틀림이 하나면 실패(참고는 안 묶음), 못 봄만 있으면 모름. 실패 → 틀림·막힘 한 번(같은 묶음에 되풀이 없음, 보류 이어짐),
// 모름 → 기록 그대로·사건 없음, 통과 → 믿음·:up 한 번
func TestProbeApply(t *testing.T) {
	ok := []pRow{{"세션 파일", pOK, ""}, {"화면 배경 작업 글자", pRef, "안 보임"}}
	bad := []pRow{{"세션 파일", pOK, ""}, {"② 끝난 배경 셸", pBad, "판정 busy(바람 quiet)"}}
	unk := []pRow{{"세션 파일", pOK, ""}, {"③ 도는 전경 명령", pUnk, ""}}
	if probeVerdict(ok) != "pass" || probeVerdict(bad) != "fail" || probeVerdict(unk) != "unknown" || probeVerdict(nil) != "unknown" {
		t.Fatal("판정 낱말")
	}
	p := filepath.Join(t.TempDir(), "upstream.json")
	id := "claude-2.0.2_hc-0.1.22"
	upstream.Update(p, func(*upstream.Rec) *upstream.Rec {
		return &upstream.Rec{HC: "0.1.22", Bundles: map[string]*upstream.Bundle{id: {State: upstream.Pending}}}
	})
	var s sent
	probeApply("w1", p, id, id, "unknown", unk, s.rep(true))
	if r := loadUp(t, p); len(s) != 0 || r.Bundles[id].State != upstream.Pending || r.Bundles[id].Result != "unknown" {
		t.Fatalf("모름: %v %+v", s, r.Bundles[id])
	}
	probeApply("w1", p, id, id, "fail", bad, s.rep(true))
	probeApply("w1", p, id, id, "fail", bad, s.rep(true))
	r := loadUp(t, p)
	if len(s) != 1 || !strings.HasPrefix(s[0], "막힘|hc-upstream:w1:"+id+"|") || !strings.Contains(s[0], "② 끝난 배경 셸") || !strings.Contains(s[0], "닫기 보류 중") {
		t.Fatalf("실패 막힘 한 번: %v", s)
	}
	if r.Hold("2.0.2") == "" || len(r.Bundles[id].Rows) != 2 {
		t.Fatalf("보류 이어짐·표 남음: %+v", r.Bundles[id])
	}
	probeApply("w1", p, id, id, "pass", ok, s.rep(true))
	probeApply("w1", p, id, id, "pass", ok, s.rep(true))
	if r := loadUp(t, p); len(s) != 2 || s[1] != "중간|hc-upstream:w1:"+id+":up|실물 점검 통과(w1): "+id+" — 이 판의 터미널 닫기는 평소대로" || r.Hold("2.0.2") != "" {
		t.Fatalf("통과 :up 한 번: %v", s)
	}
}

// ③④·합격 기준: 확인 안 된 판·틀린 판의 Claude 터미널은 R2로 닫지 않고(쉼 모름) R3는 지금 규칙대로 닫는다, 통과하면 R2 평소대로
func TestSettleUpstreamHold(t *testing.T) {
	sess, _ := restDirs(t)
	oU := rest.UpFile
	rest.UpFile = filepath.Join(t.TempDir(), "upstream.json")
	t.Cleanup(func() { rest.UpFile = oU })
	now := float64(time.Now().UnixMilli())
	ps := fakeRun(0, "", "", nil)
	tail := ""
	f := facts{worktree: no, unlanded: no, tail: &tail}
	lane := func(idle time.Duration) map[string]any {
		return map[string]any{"address": "t1", "name": "lane-a", "state": "live", "status": map[string]any{"app": "claude-code", "state": "done"},
			"labels": map[string]any{"hc-lane": "1"}, "created_ms": now - 9*60*minute, "status_ms": now - float64(idle.Milliseconds()), "turn": 4.0, "pid": 7.0}
	}
	put(t, filepath.Join(sess, "7.json"), `{"sessionId":"s7","version":"2.0.2"}`, time.Hour)
	set := func(st string) {
		upstream.Update(rest.UpFile, func(*upstream.Rec) *upstream.Rec {
			return &upstream.Rec{HC: "0.1.22", Bundles: map[string]*upstream.Bundle{"claude-2.0.2_hc-0.1.22": {State: st}}}
		})
	}
	for _, st := range []string{upstream.Pending, upstream.Failed} {
		set(st)
		if why, kept, _ := settle(lane(50*time.Minute), f, now, nil, ps); why != "" || !strings.Contains(kept, "쉼 모름(이 판은") {
			t.Fatalf("%s: R2 안 닫음 %q %q", st, why, kept)
		}
		r3 := facts{worktree: no, unlanded: yes, tail: &tail}
		if why, _, _ := settle(lane(7*60*time.Minute), r3, now, nil, ps); why != "R3" {
			t.Fatalf("%s: R3는 그대로 %q", st, why)
		}
	}
	set(upstream.Trusted)
	if why, _, q := settle(lane(50*time.Minute), f, now, nil, ps); why != "R2" || q.State != restQuiet {
		t.Fatalf("통과한 판: R2 평소대로 %q %+v", why, q)
	}
}

// 장면 판정(⑤ — 가짜 세션 자료): 쉼 판정이 기대와 다르면 그 장면이 틀림(일부러 틀리게: 끝 표시 글이 다르면 끝난 배경 셸이 도는 것으로 읽혀 틀림),
// 하위 에이전트 끝 증거가 없으면 틀림, 기록 자리가 다르면 틀림
func TestProbeScenesFakeSession(t *testing.T) {
	d := t.TempDir()
	oS, oP, oT := rest.SessDir, rest.ProjDir, rest.TaskRoot
	rest.SessDir, rest.ProjDir, rest.TaskRoot = d+"/sessions", d+"/projects", d+"/ctmp"
	t.Cleanup(func() { rest.SessDir, rest.ProjDir, rest.TaskRoot = oS, oP, oT })
	lim = sweepKnobs(time.Now())
	sid := "s1"
	task := filepath.Join(rest.TaskRoot, "-w", sid, "tasks", "b1.output")
	os.MkdirAll(filepath.Dir(task), 0o755)
	held := map[string]bool{}
	p := &prober{t: map[string]any{"pid": 7.0}, sid: sid, words: map[string]bool{},
		held: func() map[string]bool { return held },
		r: func(_ time.Duration, _ []string, name string, args ...string) result {
			if name == "ps" {
				return result{out: ""}
			}
			return result{rc: 1}
		}}
	os.MkdirAll(rest.SessDir, 0o755)
	os.WriteFile(rest.SessDir+"/7.json", []byte(`{"sessionId":"s1","version":"2.0.2"}`), 0o644)
	os.WriteFile(task, []byte("x\n"), 0o644)
	real, _ := filepath.EvalSymlinks(task)
	held[real] = true
	p.scene("① 도는 배경 셸", rest.Busy, "")
	os.WriteFile(task, []byte("x\n[done]\n"), 0o644) // 끝 표시 글이 다른 판 — 쥔 프로세스가 아직 있으면 도는 것으로 읽는다
	p.scene("② 끝난 배경 셸", rest.Quiet, "")
	delete(held, real)
	p.scene("④ 죽은 전경 명령 뒤", rest.Quiet, "")
	p.files() // 기록 자리 없음
	got := []string{}
	for _, r := range p.rows {
		got = append(got, r.item+"="+r.res)
	}
	want := "① 도는 배경 셸=맞음 ② 끝난 배경 셸=틀림 ④ 죽은 전경 명령 뒤=맞음 대화 기록 자리=틀림 하위 에이전트 기록=틀림 하위 에이전트 끝 증거=못 봄"
	if strings.Join(got, " ") != want {
		t.Fatalf("장면: %s\n바람 %s", strings.Join(got, " "), want)
	}
	if probeVerdict(p.rows) != "fail" {
		t.Fatal("틀림이 있으면 실패")
	}
	// 하위 에이전트가 끝난 증거 없이 남음 → 틀림
	sub := filepath.Join(rest.ProjDir, "-w", sid, "subagents")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(rest.ProjDir, "-w", sid+".jsonl"), []byte("{}\n"), 0o644)
	os.WriteFile(sub+"/agent-a1.meta.json", []byte(`{"agentType":"x"}`), 0o644)
	os.WriteFile(sub+"/agent-a1.jsonl", []byte(`{"type":"assistant","message":{"stop_reason":"tool_use"}}`+"\n"), 0o644)
	oWait, oTick := probeWait, probeTick
	probeWait, probeTick = 300*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { probeWait, probeTick = oWait, oTick })
	p.end = time.Now().Add(time.Minute)
	p.rows = nil
	p.files()
	if len(p.rows) != 3 || p.rows[0].res != pOK || p.rows[1].res != pOK || p.rows[2].res != pBad {
		t.Fatalf("끝 증거 없음 = 틀림: %+v", p.rows)
	}
	// 실측 경합(2026-11-29): 주 턴이 끝난 직후엔 배경 하위 에이전트가 아직 finished를 안 적었다 — 상한 안에 적히면 맞음
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.WriteFile(sub+"/agent-a1.meta.json", []byte(`{"agentType":"x","finished":true}`), 0o644)
	}()
	p.rows = nil
	p.files()
	if len(p.rows) != 3 || p.rows[2].res != pOK {
		t.Fatalf("늦게 적힌 끝 증거 = 맞음: %+v", p.rows)
	}
	// ①은 「배경 작업」 까닭의 busy여야 맞음 — 하위 에이전트가 안 끝나 busy면 그 장면을 본 것이 아니다
	os.WriteFile(sub+"/agent-a1.meta.json", []byte(`{"agentType":"x"}`), 0o644)
	p.rows = nil
	p.scene("① 도는 배경 셸", rest.Busy, "")
	if p.rows[0].res != pBad {
		t.Fatalf("하위 에이전트 까닭의 busy는 ① 맞음이 아님: %+v", p.rows)
	}
}
