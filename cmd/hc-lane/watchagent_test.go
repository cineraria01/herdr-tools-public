package main

// watch-agent-guard(2026-11-23): 지휘 에이전트가 끝나 대화형 셸만 남은 터미널은 pid·이름이 그대로라 coordCheck를 통과했고,
// watch가 사건 글을 그 셸 프롬프트에 붙여 넣었다(셸이 명령으로 실행). 받는 터미널에 에이전트가 있는지 hc get으로 가른다.
// 견본은 testdata/agent — 워커 기계 hc 0.1.21에서 잰 것(1·2·3·4·6)과 지휘 맥 hc 0.1.22 것(v22-), 경로·계정은 지어낸 값.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/hcjson"
)

func agentSample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "agent", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// agentEdit: 견본 JSON의 칸을 바꾼다(값 nil이면 칸을 뺀다)
func agentEdit(t *testing.T, b []byte, kv map[string]any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range kv {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	o, _ := json.Marshal(m)
	return o
}

// parseSample: 견본을 그 주소로 hcjson.ParseGet
func parseSample(t *testing.T, b []byte) (hcjson.Terminal, bool, error) {
	t.Helper()
	var a struct{ Address string }
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return hcjson.ParseGet(b, 0, a.Address)
}

// 판정: 0 있음, 1 없음(대화형 셸만 남음), 2 모름
func TestAgentState(t *testing.T) {
	cases := map[string]int{
		"1-shell-agent-working":   0, // ① 대화형 셸 + 에이전트 도는 중(전경 = 실행기) — ⑤ 배경 명령 중도 같은 꼴(실측: 전경은 실행기 그대로)
		"v22-shell-agent-working": 0,
		"2-shell-agent-exited":    1, // ② 에이전트가 끝나 셸만
		"2-shell-fresh":           1,
		"v22-shell-agent-exited":  1,
		"3-zshc-agent-idle":       0, // ③ zsh -c 꼴 — 전경 pid가 맨 위 pid와 같아도 셸이 터미널을 읽지 않는다
		"3-zshc-agent-exited":     0, // ③ 에이전트가 끝난 뒤 zsh가 마지막 명령(sleep)으로 exec — 셸이 아니다
		"4-shell-agent-idle":      0, // ④ 턴 사이 쉼
		"4-shell-agent-done":      0,
		"6-shell-codex":           0, // ⑥ Codex — status를 안 알리지만 전경이 실행기
	}
	for n, want := range cases {
		tm, found, err := parseSample(t, agentSample(t, n))
		if err != nil || !found {
			t.Fatalf("%s: 견본을 못 읽음 %v", n, err)
		}
		if got, why := agentState(tm); got != want {
			t.Errorf("%s: 판정 %d(%s), 바람 %d", n, got, why, want)
		}
	}
	ex := agentSample(t, "2-shell-agent-exited")
	unk := map[string][]byte{
		"셸이 전경인데 status가 남아 있음": agentEdit(t, ex, map[string]any{"status": map[string]any{"app": "claude-code", "state": "working"}}),
		"foreground 칸 없음(옛 hc)": agentEdit(t, ex, map[string]any{"foreground": nil}),
		"foreground null":       agentEdit(t, ex, map[string]any{"foreground": json.RawMessage("null")}),
		"foreground pid 없음":     agentEdit(t, ex, map[string]any{"foreground": map[string]any{"argv": []string{"zsh", "-l"}}}),
		"foreground argv 없음":    agentEdit(t, ex, map[string]any{"foreground": map[string]any{"pid": 50980}}),
		"status 칸 없음":           agentEdit(t, ex, map[string]any{"status": nil}),
	}
	for n, b := range unk {
		tm, found, err := parseSample(t, b)
		if err != nil || !found {
			t.Fatalf("%s: 못 읽음 %v", n, err)
		}
		if got, _ := agentState(tm); got != 2 {
			t.Errorf("%s: 판정 %d, 바람 2(모름)", n, got)
		}
	}
	for _, sh := range [][]string{{"-zsh"}, {"/bin/bash", "--login"}, {"fish", "-l"}, {"sh"}} {
		b := agentEdit(t, ex, map[string]any{"foreground": map[string]any{"argv": sh, "pid": 50980}})
		tm, _, _ := parseSample(t, b)
		if got, _ := agentState(tm); got != 1 {
			t.Errorf("대화형 셸 %q: 판정 %d, 바람 1", sh, got)
		}
	}
	// 중첩 셸(small-usage-fixes): 맨 위 zsh -l, 전경은 pid가 다른 대화형 bash — 맨 위 셸과 같은 판정(status null 없음 · 남음 모름)
	nest := map[string]any{"argv": []string{"bash"}, "pid": 7}
	for st, want := range map[string]int{"null": 1, `{"app":"claude-code","state":"working"}`: 2} {
		b := agentEdit(t, ex, map[string]any{"foreground": nest, "status": json.RawMessage(st)})
		tm, _, _ := parseSample(t, b)
		if got, why := agentState(tm); got != want {
			t.Errorf("중첩 셸, status %s: 판정 %d(%s), 바람 %d", st, got, why, want)
		}
	}
	// 전경 pid가 셸과 다르면 argv 없이도 있음, 판정 칸(foreground·argv)이 둘 다 없으면 확인을 건너뜀(옛 동작 — 가짜 hc 꼴)
	for n, b := range map[string][]byte{
		"전경 pid만 다름":         agentEdit(t, ex, map[string]any{"foreground": map[string]any{"pid": 7}}),
		"foreground·argv 없음": agentEdit(t, ex, map[string]any{"foreground": nil, "argv": nil}),
	} {
		tm, _, _ := parseSample(t, b)
		if got, _ := agentState(tm); got != 0 {
			t.Errorf("%s: 판정 %d, 바람 0", n, got)
		}
	}
	for _, sh := range [][]string{{"bash", "-lc", "x"}, {"zsh", "script.sh"}, {"node", "x.js"}} {
		b := agentEdit(t, ex, map[string]any{"foreground": map[string]any{"argv": sh, "pid": 50980}})
		tm, _, _ := parseSample(t, b)
		if got, _ := agentState(tm); got != 0 {
			t.Errorf("셸이 터미널을 읽지 않는 꼴 %q: 판정 %d, 바람 0", sh, got)
		}
	}
}

// 새 판 exec(쓰는 중이면 미룸·실패 뒤 다시 안 함)는 internal/selfexec 시험이 본다

// watchRun: 가짜 hc(get은 견본, send는 파일에)로 watch --to <견본 주소>를 돌린다. 지휘 정보는 견본의 pid·이름으로 맞춰 둔다
type watchRun struct {
	cmd                  *exec.Cmd
	send, wlog, ed, errf string
	done                 chan error
}

// sendCount: 보낸 글 가운데 sub가 든 줄 수
func (w *watchRun) sendCount(sub string) int {
	b, _ := os.ReadFile(w.send)
	return strings.Count(string(b), sub)
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("기다림 넘김: %s", what)
}

func evLine(lane string) string {
	return strings.Join([]string{utcNow(), "m", "t5", lane, "완료", "-", "요약 $(touch 터짐) `id`; x > y", "1", "-", "-", "*"}, tab) + "\n"
}

func startWatch(t *testing.T, bin string, body []byte) *watchRun {
	t.Helper()
	d := t.TempDir()
	tm, _, err := parseSample(t, body)
	if err != nil {
		t.Fatal(err)
	}
	fake := `#!/bin/sh
case $1 in
get) cat "$D/body"; exit 0 ;;
ls) echo '{"machines":[],"terminals":[]}'; exit 0 ;;
send) printf '%s\t%s\n' "$2" "$3" >>"$D/send"; exit 0 ;;
esac
exit 0
`
	os.MkdirAll(d+"/bin", 0o777)
	os.WriteFile(d+"/bin/hc", []byte(fake), 0o755)
	os.WriteFile(d+"/body", body, 0o644)
	ed := d + "/st/hc-lane"
	os.MkdirAll(ed, 0o777)
	nm := tm.Name
	if nm == "" {
		nm = "-"
	}
	a := tm.Address
	os.WriteFile(ed+"/coord."+ckey(a), []byte(a+tab+strconv.Itoa(tm.PID)+tab+nm+tab+"0\n"), 0o600)
	os.WriteFile(ed+"/events.tsv", []byte(evLine("lane-x")), 0o644)
	c := exec.Command(bin, "watch", "--to", a)
	ef, err := os.Create(d + "/stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ef.Close() })
	c.Stderr = ef
	c.Env = []string{"PATH=" + d + "/bin:/usr/bin:/bin", "HOME=" + d, "XDG_STATE_HOME=" + d + "/st", "D=" + d, "HC_FILL_DATA=" + d + "/fill",
		"HC_LANE_WAIT_TICK=1", "HC_LANE_WATCH_IDLE=600", "HC_LANE_WATCH_HOLD_MAX=2"}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	w := &watchRun{cmd: c, send: d + "/send", wlog: ed + "/watch.log", ed: ed, errf: d + "/stderr", done: make(chan error, 1)}
	go func() { w.done <- c.Wait() }()
	t.Cleanup(func() { c.Process.Kill(); <-w.done })
	return w
}

func TestWatchAgentGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("바이너리를 빌드해 watch를 돌린다")
	}
	bin := buildLane(t, t.TempDir())
	// ② 에이전트가 끝나 셸만: hc send 0번, 「지휘 없음」(에이전트가 끝나 셸만 남음)을 watch.log에 남기고 종료 0
	for _, n := range []string{"2-shell-agent-exited", "v22-shell-agent-exited"} {
		w := startWatch(t, bin, agentSample(t, n))
		select {
		case err := <-w.done:
			w.done <- err
			if err != nil {
				t.Errorf("%s: 종료 0이어야 — %v", n, err)
			}
		case <-time.After(15 * time.Second):
			t.Errorf("%s: 셸만 남은 지휘에 watch가 끝나지 않음", n)
		}
		if b, _ := os.ReadFile(w.send); len(b) != 0 {
			t.Errorf("%s: 셸 프롬프트에 붙여 넣었다: %q", n, b)
		}
		if b, _ := os.ReadFile(w.wlog); !strings.Contains(string(b), "지휘 없음") || !strings.Contains(string(b), "에이전트가 끝나 셸만 남음") {
			t.Errorf("%s: watch.log에 까닭이 없다: %q", n, b)
		}
	}
	// ①③④⑤⑥ 에이전트 있음: 보낸다
	for _, n := range []string{"1-shell-agent-working", "v22-shell-agent-working", "3-zshc-agent-idle", "4-shell-agent-idle", "6-shell-codex"} {
		w := startWatch(t, bin, agentSample(t, n))
		ok := false
		for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
			if b, _ := os.ReadFile(w.send); strings.Contains(string(b), "lane-x 완료") {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s: 에이전트가 있는데 보내지 않았다", n)
		}
	}
	// 새 판이 깔리면(실행 파일이 바뀜) 같은 프로세스가 새 판으로 이어 돈다 — 보낸 사건은 다시 안 보내고, 뒤 사건은 받는다
	nd := t.TempDir()
	nb := nd + "/hc-lane"
	src, _ := os.ReadFile(bin)
	if err := os.WriteFile(nb, src, 0o755); err != nil {
		t.Fatal(err)
	}
	w2 := startWatch(t, nb, agentSample(t, "3-zshc-agent-idle"))
	waitFor(t, "첫 사건 보냄", func() bool { return w2.sendCount("lane-x 완료") == 1 })
	if err := os.WriteFile(nd+"/hc-lane.new", src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(nd+"/hc-lane.new", nb); err != nil { // 까는 꼴: 새 파일로 바꿔 놓기
		t.Fatal(err)
	}
	waitFor(t, "새 판으로 exec", func() bool { b, _ := os.ReadFile(w2.errf); return strings.Contains(string(b), "새 판이 깔렸다") })
	f, _ := os.OpenFile(w2.ed+"/events.tsv", os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(evLine("lane-y"))
	f.Close()
	waitFor(t, "새 판이 뒤 사건을 보냄", func() bool { return w2.sendCount("lane-y 완료") == 1 })
	time.Sleep(2 * time.Second)
	if n := w2.sendCount("lane-x 완료"); n != 1 {
		t.Errorf("새 판이 보낸 사건을 다시 보냈다(%d번)", n)
	}
	select {
	case err := <-w2.done:
		w2.done <- err
		b, _ := os.ReadFile(w2.errf)
		t.Errorf("새 판으로 이어 돌지 않고 끝났다: %v %s", err, b)
	default:
	}
	if b, _ := os.ReadFile(w2.errf); strings.Contains(string(b), "exec 실패") || strings.Contains(string(b), "감시가 이미 돈다") {
		t.Errorf("exec 뒤 잠금·exec 실패: %s", b)
	}
	// 모름(셸이 전경인데 status가 남음): 보내지도 끝내지도 않고, 상한(HC_LANE_WATCH_HOLD_MAX) 뒤 watch.log에 한 번
	b := agentEdit(t, agentSample(t, "2-shell-agent-exited"), map[string]any{"status": map[string]any{"app": "claude-code", "state": "idle"}})
	w := startWatch(t, bin, b)
	time.Sleep(5 * time.Second)
	select {
	case err := <-w.done:
		w.done <- err
		t.Errorf("모름인데 끝났다: %v", err)
	default:
	}
	if b, _ := os.ReadFile(w.send); len(b) != 0 {
		t.Errorf("모름인데 보냈다: %q", b)
	}
	if b, _ := os.ReadFile(w.wlog); strings.Count(string(b), "보내지 않고 계속 본다") != 1 {
		t.Errorf("모름이 상한을 넘으면 watch.log에 한 번: %q", b)
	}
}
