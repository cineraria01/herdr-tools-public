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
	oS, oP, oT := SessDir, ProjDir, TaskRoot
	SessDir, ProjDir, TaskRoot = d+"/sessions", d+"/projects", d+"/ctmp/claude-1"
	t.Cleanup(func() { SessDir, ProjDir, TaskRoot = oS, oP, oT })
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
			if v := Of(map[string]any{"pid": 7.0, "status": c.status}, now, 3.6e7, noPS); v.State != c.want {
				t.Errorf("세션 폴더 %v·status %v: %v, 바람 %v (%s)", mk, c.status, v.State, c.want, v.Sig)
			}
		}
	}
	os.WriteFile(SessDir+"/7.json", []byte(`{"sessionId":"s7"}`), 0o644)
	if v := Of(map[string]any{"pid": 7.0, "status": map[string]any{"app": "claude-code"}}, now, 3.6e7, noPS); v.State != Quiet || v.Sid != "s7" {
		t.Fatalf("세션 있음·기록 없음 = 쉼: %+v", v)
	}
	// 사고 꼴: 끝 표시 없는 배경 작업 → 활동
	p := filepath.Join(TaskRoot, "-p", "s7", "tasks", "b1.output")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, nil, 0o644)
	if v := Of(map[string]any{"pid": 7.0}, now, 3.6e7, noPS); v.State != Busy || v.Sig != "배경 작업 1개 도는 중" {
		t.Fatalf("배경 작업: 활동 %+v", v)
	}
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
