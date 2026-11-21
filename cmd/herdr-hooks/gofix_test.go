package main

// gofix-hooks(2026-11-22 Go 전환 전체 감사 지적) 단위 시험 — 지적 ID는 감사 보고서의 것(아스트라 H·M·E, Fable H·M).

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// M05(아스트라): Windows 교차 빌드가 된다(레인은 돌리지 않지만 교차 빌드·패키징을 깨면 안 된다 — syscall.Kill·Setpgid를 직접 쓰던 것)
func TestCrossBuildWindows(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go 없음")
	}
	c := exec.Command(goBin, "build", "-o", os.DevNull, ".")
	c.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("windows/amd64 빌드 실패: %v\n%s", err, out)
	}
}

// E05(아스트라): ps로 시작 시각을 못 읽으면(실패·시간 넘김) 산 pid를 「죽음」으로 읽지 않는다 — 임대 주인을 지킨다
func TestAliveWhenPsUnknown(t *testing.T) {
	pid := strconv.Itoa(os.Getpid())
	g := &cg{psc: map[string]string{pid: psUnknown}, aok: map[string]bool{}}
	if !g.alive(pid, "Mon Jan  1 00:00:00 2001") {
		t.Error("ps 모름인데 산 pid를 죽은 것으로 봤다")
	}
	g = &cg{psc: map[string]string{}, aok: map[string]bool{}}
	hookBudget(4e9)
	if !g.alive(pid, "") {
		t.Error("이 프로세스를 죽은 것으로 봤다")
	}
	if g.alive("999999999", "") || g.alive("1", "") || g.alive("0", "") {
		t.Error("없는 pid·1·0을 산 것으로 봤다")
	}
}

// M01(아스트라): 값 없는 마지막 -o·-O에서 cgAnalyze가 배열 끝을 넘어 패닉하지 않고 awk처럼 리다이렉션 대상을 낸다.
func TestCgAnalyzeShellOptNoValue(t *testing.T) {
	for c, want := range map[string]string{"bash -O > options.log": "options.log", "sh -o > o.log": "o.log", "bash --rcfile > r.log": "r.log", "zsh -o": ""} {
		out := cgAnalyze(c, "/w", "/h")
		if (want == "" && len(out) != 0) || (want != "" && (len(out) != 1 || out[0] != "WRITE\x1f/w\x1f"+want)) {
			t.Errorf("%q → %q (want WRITE %q)", c, out, want)
		}
	}
}

// H04(아스트라): 실제 명령(HERDR_HOOKS_CORPUS)은 실패 로그에 원문 대신 번호로
func TestCorpusLogHidesRealCommands(t *testing.T) {
	p := t.TempDir() + "/corpus"
	if err := os.WriteFile(p, []byte("a\x00echo SYNTH_ONLY_MARK\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_HOOKS_CORPUS", p)
	corpus(t)
	if s, ext := show("echo SYNTH_ONLY_MARK"); !ext || s != "HERDR_HOOKS_CORPUS #2" {
		t.Errorf("실제 명령 표시 %q(번호만이어야)", s)
	}
	if s, ext := show("git push -f"); ext || s != `"git push -f"` {
		t.Errorf("합성 명령 표시 %q", s)
	}
}

// L2(Fable): 뜻밖의 패닉 — 편의 막 넷은 0(fail-open), cotenant-guard는 1(onFailure 막음). Go 기본 2(막음+스택)가 아니다
func TestPanicExitCode(t *testing.T) {
	saved := hooks["hc-lane-wait-guard"]
	t.Cleanup(func() { hooks["hc-lane-wait-guard"] = saved })
	hooks["hc-lane-wait-guard"] = func([]byte, io.Writer, io.Writer) int { panic("x") }
	var e bytes.Buffer
	if rc := call("hc-lane-wait-guard", nil, io.Discard, &e); rc != 0 || !strings.Contains(e.String(), "내부 오류") {
		t.Errorf("편의 막 패닉 rc=%d(want 0) err=[%s]", rc, e.String())
	}
	savedCG := hooks["cotenant-guard"]
	t.Cleanup(func() { hooks["cotenant-guard"] = savedCG })
	hooks["cotenant-guard"] = func([]byte, io.Writer, io.Writer) int { panic("x") }
	if rc := call("cotenant-guard", nil, io.Discard, io.Discard); rc != 1 {
		t.Errorf("cotenant-guard 패닉 rc=%d(want 1)", rc)
	}
}

// FuzzCgAnalyze: 어떤 명령 글도 낱말 가르기를 패닉시키지 않는다(패닉은 훅 종료 2 — 판정이 아니라 사고).
func FuzzCgAnalyze(f *testing.F) {
	for _, s := range []string{"bash -O > a", "git -C x push -f", "ssh -o", "hc run --on", "env -u", "timeout -s", "xargs -I", "cd", "sed -i", "cp a", `bash -c "rm x"`, "eval", "hc --json exec"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { cgAnalyze(s, "/w", "/h") })
}
