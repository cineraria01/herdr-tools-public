package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 조각(agent.sh)은 셸 판 bin/herdr-sync heredoc과 같은 글자여야 한다 — 두 판이 같은 base·서명을 번갈아 읽는다(셸 판을 걷는 날 이 시험도 걷는다)
func TestAgentMatchesShell(t *testing.T) {
	b, err := os.ReadFile("../../bin/herdr-sync")
	if err != nil {
		t.Skip("셸 판 없음(깔린 곳)")
	}
	s := string(b)
	start := strings.Index(s, "IFS= read -r -d '' AGENT << 'EOF'\n")
	if start < 0 {
		t.Fatal("셸 판에서 AGENT heredoc을 못 찾았다")
	}
	s = s[start+len("IFS= read -r -d '' AGENT << 'EOF'\n"):]
	end := strings.Index(s, "\nEOF\n")
	if end < 0 {
		t.Fatal("AGENT heredoc 끝을 못 찾았다")
	}
	if got, want := agent, s[:end+1]; got != want {
		t.Errorf("cmd/herdr-sync/agent.sh가 셸 판 AGENT와 다르다 — 셸 판 heredoc을 agent.sh로 그대로 옮긴다")
	}
}

// srcDigest는 lib/go-run.sh go_run_src와 같은 값이어야 받은 바이너리를 기계가 알아본다
func TestSrcDigestMatchesGoRun(t *testing.T) {
	gr, err := os.ReadFile("../../lib/go-run.sh")
	if err != nil {
		t.Skip("lib/go-run.sh 없음(깔린 곳)")
	}
	d := t.TempDir()
	for p, c := range map[string]string{
		"lib/go-run.sh": string(gr), "go.mod": "module x\n", "cmd/a/main.go": "package main\n", "cmd/a/OWNER": "b\n",
		"internal/z z/b.go": "package z\n", "internal/y/c_test.go": "package y\n", "internal/y/README": "x\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(d, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := exec.Command("bash", "-c", `. "$1/lib/go-run.sh" && go_run_src`, "_", d).Output()
	if err != nil {
		t.Fatalf("go_run_src: %v", err)
	}
	got, err := srcDigest(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSpace(string(out)); got != want {
		t.Errorf("srcDigest %s, go_run_src %s", got, want)
	}
	os.WriteFile(filepath.Join(d, "cmd/a/main.go"), []byte("package main // 바뀜\n"), 0o644)
	if g2, _ := srcDigest(d); g2 == got {
		t.Errorf("소스가 바뀌어도 판이 같다")
	}
}

func TestGithubSlug(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git": "o/r", "git@github.com:o/r.git": "o/r", "ssh://git@github.com/o/r": "o/r",
		"https://github.com/o/r": "o/r", "/x/public.git": "", "https://gitlab.com/o/r.git": "",
	} {
		if got := githubSlug(in); got != want {
			t.Errorf("%s: 얻음 %q 바람 %q", in, got, want)
		}
	}
}
