package main

// 기준 대조: 셸 판(2026-11-22에 지움)의 perl·awk가 낸 답을 기록한 골든(testdata/shell-golden.jsonl — TestGoldenShell)과,
// 지금도 있는 perl·bash의 원래 의미(heredoc 치환·read 칸 나누기)와 견준다.
// 입력 = 정해진 씨앗으로 만든 조각 섞음(diffCorpusN개) + 시험 파일의 명령들 + (있으면) HERDR_HOOKS_CORPUS 파일(NUL로 가른 실제 명령).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const diffCorpusN = 6000

var frags = []string{
	"kill", "pkill", "killall", "pgrep", "fuser", "lsof", "ps", "xargs", "-9", "-P", "--parent", "-f", "-s", "-n", "--", "0", "-1", "1234",
	"$PID", "$(", ")", "`", "'", "\"", "\\", " ", " ", " ", "  ", "\t", "\n", ";", "&", "&&", "|", "||", "#", "/usr/bin/", "./", "x/",
	"-ti", "-t", "-k", ":3000", "socat", "aux", "awk '{print $2}'", "grep", "echo", "bash -c ", "sh -c", "zsh -lc ", "eval ", "ssh m ",
	"hc exec ", "hc run ", "hc kill ", "<<EOF\n", "<<'EOF'\n", "<<-X\n", "EOF\n", "EOF", "X", "\nEOF", "<<<", "sed", "-i", "-E", "--in-place",
	"perl", "-pi", "-e", "python3", "node", "bun", "deno", "python3.11", "open(", "'w'", "\"a\"", ").write(", ".write_text(", "writeFileSync(",
	"Bun.write(", ">", ">>", ">|", "2>&1", "&>", ">&2", "a.md", "x.go", "b.json", "build.log", "/tmp/x", "/dev/null", "$TMPDIR", "${TMPDIR}",
	"mktemp", ".qa/", "scratchpad", "tee", "-a", "${(f)", "${(@f)", "}", "git", "commit", "-m", "-F", "git -C /r ", "add", "-A", ".", "*",
	"push", "+main", "--force", "--force-with-lease=r:", "--mirror", "origin", "main", "stash", "reset", "--hard", "checkout", "rebase",
	"cp", "mv", "rm", "-rf", "unlink", "cd ", "sub", "~/", "~", "env ", "A=1 ", "timeout 5 ", "nice ", "nohup ", "setsid", "disown",
	"hc-lane wait", "hc-lane  wait", "hc-lane\twait", "&>/tmp/w", "가", "한글", "🙂", "\x01", "x", "y", "z", "_", "-", "=", "(", "[", "?",
}

func corpus(t *testing.T) []string {
	var out []string
	// 훅 계약 시험(Go — 옛 셸 시험 *.test.sh를 옮긴 것, 2026-11-21 go-tests-hooks)의 문자열 글자들(대략) — 맨 앞(느린 대조는 앞쪽만 본다).
	// 작은 훅 셋(small_hooks) 먼저, 그다음 cotenant-guard. Go 글자는 풀어서(\" 등), 못 풀면 그대로
	for _, f := range []string{"small_hooks_test.go", "cotenant_basics_test.go", "cotenant_fix12_test.go", "cotenant_fix34_test.go"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "claude", "hooks", "tests", f))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile("`([^`\\n]{1,300})`|\"((?:[^\"\\\\\\n]|\\\\.){1,300})\"").FindAllStringSubmatch(string(b), -1) {
			s := m[1]
			if m[2] != "" {
				if s, err = strconv.Unquote(`"` + m[2] + `"`); err != nil {
					s = m[2]
				}
			}
			out = append(out, s)
		}
	}
	r := rand.New(rand.NewSource(20261121))
	for i := 0; i < diffCorpusN; i++ {
		n := 1 + r.Intn(14)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteString(frags[r.Intn(len(frags))])
		}
		out = append(out, b.String())
	}
	if p := os.Getenv("HERDR_HOOKS_CORPUS"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range strings.Split(string(b), "\x00") {
			if c != "" && !strings.Contains(c, "\x00") {
				out = append(out, c)
				if _, ok := corpusExt[c]; !ok {
					corpusExt[c] = i + 1
				}
			}
		}
	}
	return out
}

// corpusExt: HERDR_HOOKS_CORPUS에서 읽은 실제 명령 → 그 파일 안 번호(1부터). 실패 로그에는 원문·출력 대신 번호만
// (아스트라 H04 — 실제 명령 이력에는 토큰·내부 주소가 섞일 수 있다)
var corpusExt = map[string]int{}

// show: 실패 로그에 적을 입력과 실제 명령인지 — 실제 명령이면 번호만
func show(s string) (string, bool) {
	if n, ok := corpusExt[s]; ok {
		return fmt.Sprintf("HERDR_HOOKS_CORPUS #%d", n), true
	}
	return strconv.Quote(s), false
}

// diffErr: 다름 한 건 — 실제 명령이면 출력(명령 조각이 든다)은 빼고 번호와 detail(종료 코드 등)만
func diffErr(t *testing.T, kind, s, detail string, outs ...any) {
	t.Helper()
	label, ext := show(s)
	if ext {
		t.Errorf("%s %s: 다름 %s(원문·출력은 그 파일에서 번호로)", kind, label, detail)
		return
	}
	t.Errorf("%s %s: %s"+strings.Repeat(" %q", len(outs)), append([]any{kind, label, detail}, outs...)...)
}

// goldenCase: 셸 판(지운 claude/hooks/*.sh — 2026-11-22 go-shell-remove)의 perl·awk 프로그램이 그 입력에 낸 실제 답.
// 셸 판을 지우기 전에 한 번 기록했다(testdata/shell-golden.jsonl — 한 줄에 한 사례). Tok은 앞 2500개만(awk가 명령마다 한 번이라 느렸다).
// in·tok은 바이트를 21바이트(base64 28자)씩 나눈 base64 조각 배열 — 한 덩이 base64는 공개 내보내기의 「긴 토큰」 비밀 꼴 검사에 걸린다
type goldenCase struct {
	In     b64Chunks `json:"in"`
	Kill   string    `json:"kill"`
	Edit   string    `json:"edit"`
	Tok    b64Chunks `json:"tok,omitempty"`
	HasTok bool      `json:"has_tok,omitempty"`
}

// b64Chunks: base64 조각 배열을 이어 푼 바이트
type b64Chunks []byte

func (c *b64Chunks) UnmarshalJSON(b []byte) error {
	var parts []string
	if err := json.Unmarshal(b, &parts); err != nil {
		return err
	}
	var out []byte
	for _, p := range parts {
		d, err := base64.StdEncoding.DecodeString(p)
		if err != nil {
			return fmt.Errorf("골든 조각 %q: %w", p, err)
		}
		out = append(out, d...)
	}
	*c = out
	return nil
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "shell-golden.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var g []goldenCase
	for i, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		var c goldenCase
		if err := json.Unmarshal(l, &c); err != nil {
			t.Fatalf("골든 %d번 줄: %v", i+1, err)
		}
		g = append(g, c)
	}
	if len(g) < diffCorpusN {
		t.Fatalf("골든 %d개 — %d개 이상이어야", len(g), diffCorpusN)
	}
	return g
}

// TestGoldenShell: kill-by-lookup-guard·lane-no-shell-edit 판정과 cotenant-guard 낱말 가르기가 셸 판 골든과 같다
func TestGoldenShell(t *testing.T) {
	bad, toks := 0, 0
	for _, c := range loadGolden(t) {
		s := string(c.In)
		kill := "0"
		if killVerdict(s) {
			kill = "2"
		}
		if kill != c.Kill {
			bad++
			diffErr(t, "kill", s, "go "+kill+", 셸 "+c.Kill)
		}
		if e := strconv.Itoa(shellEditVerdict(s)); e != c.Edit {
			bad++
			diffErr(t, "edit", s, "go "+e+", 셸 "+c.Edit)
		}
		if c.HasTok {
			toks++
			if got := strings.Join(cgAnalyze(s, "/w/d", "/h"), "\n"); got != string(c.Tok) {
				bad++
				diffErr(t, "tok", s, "go·셸 awk", got, string(c.Tok))
			}
		}
		if bad > 30 {
			t.Fatal("다름이 너무 많다")
		}
	}
	if toks < 2000 {
		t.Errorf("낱말 가르기 골든 %d개 — 2000개 이상이어야", toks)
	}
}

// TestDiffHeredoc: perl 원문 치환 하나만
func TestDiffHeredoc(t *testing.T) {
	in := corpus(t)
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("perl 없음")
	}
	c := exec.Command(perl, "-e", `local $/ = "\0"; while (my $s = <STDIN>) { chomp $s; $s =~ s/(<<-?[ \t]*([\x27"]?)(\w+)\2[^\n]*)\n(?:.*?\n)?[ \t]*\3[ \t]*(?=\n|$)/$1/gs; print "$s\0" }`)
	var stdin bytes.Buffer
	for _, s := range in {
		stdin.WriteString(s + "\x00")
	}
	c.Stdin = &stdin
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	bad := 0
	for i, s := range in {
		if got := heredocStrip(s); got != want[i] {
			bad++
			if bad <= 10 {
				diffErr(t, "heredoc", s, "go·perl", got, want[i])
			}
		}
	}
}

// TestBashRead: bash `IFS=$'\037' read -r` 칸 나누기를 /bin/bash와 견준다
func TestBashRead(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash 없음")
	}
	u := "\x1f"
	lines := []string{"", u, u + u, "a", "a" + u, "a" + u + u, "a" + u + "b", "a" + u + "b" + u, "a" + u + "b" + u + "c" + u, u + "a" + u + "b",
		"a" + u + "b" + u + "c" + u + "d", "a" + u + u + "b" + u, "k" + u + "d" + u + "x" + u + "y" + u}
	for _, n := range []int{1, 2, 3} {
		for _, l := range lines {
			c := exec.Command("/bin/bash", "-c", `IFS=$'\037' read -r a b c <<< "$L"; case $N in 1) printf '%s' "$a" ;; 2) printf '%s\036%s' "$a" "$b" ;; 3) printf '%s\036%s\036%s' "$a" "$b" "$c" ;; esac`)
			if n == 1 {
				c = exec.Command("/bin/bash", "-c", `IFS=$'\037' read -r a <<< "$L"; printf '%s' "$a"`)
			} else if n == 2 {
				c = exec.Command("/bin/bash", "-c", `IFS=$'\037' read -r a b <<< "$L"; printf '%s\036%s' "$a" "$b"`)
			}
			c.Env = []string{"L=" + l, "N=" + strconv.Itoa(n)}
			out, err := c.Output()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(bashRead(l, 0x1f, n), "\x1e"); got != string(out) {
				t.Errorf("read %d %q: go %q, bash %q", n, l, got, out)
			}
		}
	}
}
