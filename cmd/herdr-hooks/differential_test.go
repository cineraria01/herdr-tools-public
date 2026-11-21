package main

// 두 판 대조: 셸 판 훅 파일에서 perl·awk 프로그램을 그대로 뽑아 돌리고, 같은 입력에 Go 판이 같은 답을 내는지 본다.
// 입력 = 정해진 씨앗으로 만든 조각 섞음(diffCorpusN개) + 시험 파일의 명령들 + (있으면) HERDR_HOOKS_CORPUS 파일(NUL로 가른 실제 명령).
// perl·awk가 없으면 건너뛴다.

import (
	"bytes"
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
		for _, c := range strings.Split(string(b), "\x00") {
			if c != "" && !strings.Contains(c, "\x00") {
				out = append(out, c)
			}
		}
	}
	return out
}

// perlBody: 셸 판 훅의 `"$PERL" -e '…'` 본문
func perlBody(t *testing.T, hook string) string {
	b, err := os.ReadFile(filepath.Join("..", "..", "claude", "hooks", hook+".sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)"\$PERL" -e '\n(.*?)\n'\n`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("%s: perl 본문을 못 찾음", hook)
	}
	return m[1]
}

// perlRun: 본문의 exit N을 die로 바꿔 입력마다 돌린 종료 코드
func perlRun(t *testing.T, body string, in []string) []string {
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("perl 없음")
	}
	body = regexp.MustCompile(`\bexit (\d)`).ReplaceAllString(body, `die "RC $1\n"`)
	prog := "local $/ = \"\\0\"; while (my $rec = <STDIN>) { chomp $rec; $ENV{CMD} = $rec; my $rc = 0; eval {\n" + body +
		"\n}; if ($@ =~ /^RC (\\d+)/) { $rc = $1 } elsif ($@) { $rc = \"ERR $@\" } print \"$rc\\n\"; }"
	c := exec.Command(perl, "-e", prog)
	var stdin bytes.Buffer
	for _, s := range in {
		stdin.WriteString(s + "\x00")
	}
	c.Stdin = &stdin
	out, err := c.Output()
	if err != nil {
		t.Fatalf("perl: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

func TestDiffKillGuard(t *testing.T) {
	in := corpus(t)
	want := perlRun(t, perlBody(t, "kill-by-lookup-guard"), in)
	bad := 0
	for i, s := range in {
		got := "0"
		if killVerdict(s) {
			got = "2"
		}
		if got != want[i] {
			bad++
			if bad <= 15 {
				t.Errorf("kill %q: go %s, perl %s", s, got, want[i])
			}
		}
	}
	t.Logf("kill-by-lookup-guard %d개 대조(perl이 막음 %d), 다름 %d", len(in), strings.Count(strings.Join(want, "\n")+"\n", "2\n"), bad)
}

func TestDiffShellEdit(t *testing.T) {
	in := corpus(t)
	want := perlRun(t, perlBody(t, "lane-no-shell-edit"), in)
	bad := 0
	for i, s := range in {
		if got := strconv.Itoa(shellEditVerdict(s)); got != want[i] {
			bad++
			if bad <= 15 {
				t.Errorf("edit %q: go %s, perl %s", s, got, want[i])
			}
		}
	}
	dist := map[string]int{}
	for _, w := range want {
		dist[w]++
	}
	t.Logf("lane-no-shell-edit %d개 대조(perl 종료 코드 분포 %v), 다름 %d", len(in), dist, bad)
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
				t.Errorf("heredoc %q: go %q, perl %q", s, got, want[i])
			}
		}
	}
}

// TestDiffTokenizer: 셸 판 AWK 프로그램(LC_ALL=C /usr/bin/awk)과 낱말 가르기 결과 줄이 같은가
func TestDiffTokenizer(t *testing.T) {
	awk := "/usr/bin/awk"
	if _, err := os.Stat(awk); err != nil {
		t.Skip("awk 없음")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "claude", "hooks", "cotenant-guard.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)\nAWK='\n(.*?)\n}'\n`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("AWK 프로그램을 못 찾음")
	}
	prog := m[1] + "\n}"
	in := corpus(t)
	if os.Getenv("HERDR_HOOKS_CORPUS") == "" && len(in) > 2500 { // 명령 하나에 awk 한 번 — 보통 돌림은 앞쪽(시험 명령 + 섞음 일부)만
		in = in[:2500]
	}
	bad := 0
	for _, s := range in {
		c := exec.Command(awk, prog)
		c.Env = []string{"LC_ALL=C", "CG_CWD=/w/d", "HOME=/h"}
		c.Stdin = strings.NewReader(strings.ReplaceAll(s, "\x00", "") + "\n")
		out, err := c.Output()
		if err != nil {
			t.Fatalf("awk %q: %v", s, err)
		}
		want := strings.TrimRight(string(out), "\n")
		got := strings.Join(cgAnalyze(s, "/w/d", "/h"), "\n")
		if got != want {
			bad++
			if bad <= 10 {
				t.Errorf("tok %q:\n go  %q\n awk %q", s, got, want)
			}
		}
	}
	t.Logf("낱말 가르기 %d개 대조, 다름 %d", len(in), bad)
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
