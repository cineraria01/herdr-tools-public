package rest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 판정 재료·규칙은 hc-sweep 시험(cmd/hc-sweep/rest_test.go — quiet가 Of를 부른다)이 본다. 여기는 이 레인이 더한 것:
// Claude인데 자료 없음 = 모름, 묻기 한 줄의 꼴(Line·Parse), 어디서 묻는가(Ask)

func dirs(t *testing.T) {
	d := t.TempDir()
	oS, oP, oT, oU := SessDir, ProjDir, TaskRoot, UpFile
	SessDir, ProjDir, TaskRoot, UpFile = d+"/sessions", d+"/projects", d+"/ctmp/claude-1", d+"/state/hc-sweep/upstream.json"
	t.Cleanup(func() { SessDir, ProjDir, TaskRoot, UpFile = oS, oP, oT, oU })
}

func noPS() (string, bool) { return "", true }

// 세션 파일 없음: Claude(status.app에 claude)·앱 칸 없음은 모름, Claude가 아닌 CLI·셸(상태 없음)은 신호 없음(쉼 — 지금 규칙 그대로).
// 세션 폴더가 아예 없는 기계(Claude를 안 띄운 리눅스 워커)에서도 같다 — Claude 터미널이면 자료가 없는 것이다
func TestOfClaudeWithoutSession(t *testing.T) {
	dirs(t)
	now := float64(time.Now().UnixMilli())
	for _, mk := range []bool{false, true} {
		if mk {
			os.MkdirAll(SessDir, 0o755)
		}
		for _, c := range []struct {
			status any
			want   State
		}{
			{map[string]any{"app": "claude-code", "state": "done"}, Unknown},
			{map[string]any{"app": "Claude", "state": "idle"}, Unknown}, // 이름이 바뀐 판도 Claude로(닫지 않는 쪽)
			{map[string]any{"state": "done"}, Unknown},                  // 앱 칸 없음
			{"done", Unknown}, // 모르는 꼴
			{map[string]any{"app": "codex", "state": "done"}, Quiet},
			{nil, Quiet}, // 셸(hc keep이 남긴 셸 포함)
		} {
			if v := Of(map[string]any{"pid": 7.0, "status": c.status}, now, 3.6e7, noPS, nil); v.State != c.want {
				t.Errorf("세션 폴더 %v·status %v: %v, 바람 %v (%s)", mk, c.status, v.State, c.want, v.Sig)
			}
		}
	}
	os.WriteFile(SessDir+"/7.json", []byte(`{"sessionId":"s7"}`), 0o644)
	if v := Of(map[string]any{"pid": 7.0, "status": map[string]any{"app": "claude-code"}}, now, 3.6e7, noPS, nil); v.State != Quiet || v.Sid != "s7" {
		t.Fatalf("세션 있음·기록 없음 = 쉼: %+v", v)
	}
	// 사고 꼴: 끝 표시 없고 프로세스가 쥔 배경 작업 → 활동
	p := filepath.Join(TaskRoot, "-p", "s7", "tasks", "b1.output")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, nil, 0o644)
	rp, _ := filepath.EvalSymlinks(p)
	if v := Of(map[string]any{"pid": 7.0}, now, 3.6e7, noPS, func() map[string]bool { return map[string]bool{rp: true} }); v.State != Busy || v.Sig != "배경 작업 1개 도는 중" {
		t.Fatalf("배경 작업: 활동 %+v", v)
	}
}

// ht-rest-task-held: 끝 표시 없는 배경 작업 파일은 「살아 있는 프로세스가 쥐고 있을 때만」 도는 것(운영체제 사실). 쥔 경로는 held가 한 번에 준다
// (hc-sweep은 openPaths — lsof). 오늘의 두 사례: 상한에 걸려 죽은 전경 명령이 끝 표시 없이 남긴 파일, 쥔 프로세스 0
func TestBgTasksHeld(t *testing.T) {
	dirs(t)
	now := float64(time.Now().UnixMilli())
	os.MkdirAll(SessDir, 0o755)
	os.WriteFile(SessDir+"/7.json", []byte(`{"sessionId":"s7"}`), 0o644)
	tasks := filepath.Join(TaskRoot, "-p", "s7", "tasks")
	os.MkdirAll(tasks, 0o755)
	mk := func(name, body string, age time.Duration) string {
		p := filepath.Join(tasks, name)
		os.WriteFile(p, []byte(body), 0o644)
		at := time.Now().Add(-age)
		os.Chtimes(p, at, at)
		return p
	}
	real := func(p string) string {
		r, err := filepath.EvalSymlinks(p) // lsof는 링크를 푼 경로를 낸다(맥 /tmp → /private/tmp, 임시 폴더 /var → /private/var)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	calls, holds := 0, map[string]bool{}
	held := func() map[string]bool { calls++; return holds }
	check := func(what string, h Held, state State, sig string, wantCalls int) Verdict {
		t.Helper()
		calls = 0
		v := Of(map[string]any{"pid": 7.0}, now, 3.6e7, noPS, h)
		if v.State != state || v.Sig != sig || calls != wantCalls {
			t.Errorf("%s: %v %q 부름 %d, 바람 %v %q 부름 %d", what, v.State, v.Sig, calls, state, sig, wantCalls)
		}
		return v
	}
	// ⑤ 후보 없음 → 부르지 않음
	check("⑤ 파일 없음", held, Quiet, "", 0)
	// ④ 끝 표시 있음 → 묻지 않음
	mk("bdone.output", "x\n\n[exited with code 0]\n", 40*time.Minute)
	check("④ 끝 표시", held, Quiet, "", 0)
	// ⑦ r3 넘은 미완 · ⑧ 링크 → 지금처럼 안 셈(후보 아님)
	mk("bold.output", "", 11*time.Hour) // r3 10시간
	os.Symlink(filepath.Join(tasks, "bdone.output"), filepath.Join(tasks, "a1.output"))
	check("⑦⑧ 오래된 미완·링크", held, Quiet, "", 0)
	// ① 안 쥠 → 안 셈(사례 꼴: 상한에 걸린 go test가 남긴 44바이트). 수정 시각은 지금처럼 마지막 활동
	fg := mk("bfg.output", "FAIL\therdr-tools/x\t600.268s\n", 30*time.Minute)
	if v := check("① 안 쥠", held, Quiet, "", 1); now-v.Last > 31*6e4 || now-v.Last < 29*6e4 {
		t.Errorf("① 마지막 활동 30분 전: %+v", v)
	}
	// ② 쥠 → 셈
	holds[real(fg)] = true
	check("② 쥠", held, Busy, "배경 작업 1개 도는 중", 1)
	// ③ 쥔 것 하나 + 안 쥔 것 하나 → 1개(부름은 한 번)
	mk("bfree.output", "", 2*time.Minute)
	check("③ 하나만 쥠", held, Busy, "배경 작업 1개 도는 중", 1)
	// ⑥ 못 읽음(도구 없음·실패·시간 넘김·꼴 다름 — openPaths는 모두 nil)·묻는 길 없음 → 모름
	check("⑥ 못 읽음", func() map[string]bool { calls++; return nil }, Unknown, "배경 작업을 쥔 프로세스 못 읽음", 1)
	check("⑥ 묻는 길 없음", nil, Unknown, "배경 작업을 쥔 프로세스 못 읽음", 0)
	// ⑩ 세는 도중 사라짐(held가 도는 사이 Claude가 지움) → 건너뜀
	check("⑩ 사라짐", func() map[string]bool { calls++; os.Remove(fg); return holds }, Quiet, "", 1)
	// 두 번째 실행 → 같은 답
	check("두 번째", held, Quiet, "", 1)
}

// 묻기 한 줄: Line ↔ Parse 왕복, 옛 판·잘림·경고 섞임·모르는 상태
func TestLineParse(t *testing.T) {
	for _, v := range []Verdict{{State: Quiet, Sid: "s1"}, {State: Busy, Sid: "s1", Sig: "배경 작업 1개 도는 중"}, {Sig: "ps 실패"}} {
		got := Parse("경고: 손잡이를 못 읽음\n"+Line(v)+"\n", 0)
		if got.State != v.State || got.Sid != v.Sid || got.Sig != v.Sig {
			t.Errorf("왕복 %+v → %+v", v, got)
		}
	}
	if l := Line(Verdict{State: Busy, Sig: "a\tb\nc"}); strings.Count(l, "\t") != 4 {
		t.Fatalf("까닭 안 탭·줄바꿈은 빈칸: %q", l)
	}
	for _, c := range []struct {
		out string
		rc  int
	}{
		{"flag provided but not defined: -rest\n", 2}, // 옛 판 hc-sweep
		{Line(Verdict{State: Quiet}), 255},            // 연결 안 됨
		{"", 124},                                     // 시간 넘김
		{"REST\tquiet\t-\t-", 0},                      // 잘림(끝 칸 없음)
		{"REST\tqui", 0},
		{"REST\tsleepy\t-\t-\tend", 0}, // 모르는 상태
		{"", 0},
	} {
		if v := Parse(c.out, c.rc); v.State != Unknown || v.Sig == "" {
			t.Errorf("%q 종료 %d: 모름이어야 %+v", c.out, c.rc, v)
		}
	}
	if Why(Verdict{State: Quiet}) != "" || Why(Verdict{State: Busy, Sig: "x"}) != "안에서 일이 도는 중(x)" || Why(Verdict{Sig: "y"}) != "쉼 모름(y)" {
		t.Fatal("Why 꼴")
	}
}

// 어디서 묻는가: 접두 없음·이 기계 접두는 바로 hc-sweep, 다른 기계 접두는 그 기계에서(hc exec --on — 그 기계의 터미널 이름만 넘긴다)
func TestAskWhere(t *testing.T) {
	var got []string
	run := func(a []string) (string, int) { got = a; return Line(Verdict{State: Quiet}), 0 }
	for _, c := range []struct{ addr, local, want string }{
		{"t3", "m", "hc-sweep --rest t3"},
		{"m/t3", "m", "hc-sweep --rest t3"},
		{"w1/t3", "m", "hc exec --on w1 --timeout 60 -- hc-sweep --rest t3"},
		{"t3", "", "hc-sweep --rest t3"},
	} {
		if v := Ask(run, c.addr, c.local); v.State != Quiet || strings.Join(got, " ") != c.want {
			t.Errorf("%s@%s: %q, 바람 %q", c.addr, c.local, strings.Join(got, " "), c.want)
		}
	}
}
