package toolcfg

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CompileERE: 셸 판 LC_ALL=C grep -E(대소문자 무시면 -i)와 같은 판정 — publish.secrets처럼 ERE 한 줄을 받는 곳이 쓴다.
// grep이 있으면 같은 사례를 셸로도 돌려 기대값을 맞춘다
func TestCompileERELikeShell(t *testing.T) {
	cases := []struct {
		pat, line string
		icase     bool
		want      bool
	}{
		{`customtok-[A-Z]{3}`, "customtok-ABC", false, true},
		{`customtok-[A-Z]{3}`, "customtok-abc", false, false}, // -i 없으면 대소문자를 가린다
		{`customtok-[A-Z]{3}`, "customtok-abc", true, true},
		{`a.b`, "a\xc3\xa9b", false, false}, // 「.」은 바이트 하나(é는 두 바이트)
		{`a..b`, "a\xc3\xa9b", false, true},
		{`[\d]`, "7", false, false}, // 괄호 식 안 역슬래시는 글자 그대로(\ 또는 d)
		{`[\d]`, `x\y`, false, true},
		{`\<tok`, "a tok", false, true},
		{`\<tok`, "atok", false, false},
		{`[[:<:]]tok`, "a tok", false, true},
		{"x\xc3\xa9", "X\xc3\xa9", true, true},
		{"\xc3\xa9", "\xc3\x89", true, false}, // 대소문자 무시는 ASCII만(C 로캘)
		{`\/x`, "/x", false, true}, {`a\-b`, "a-b", false, true}, {`\}`, "}", false, true}, {`a\.b`, "axb", false, false},
		{`\<tok\>`, "a tok b", false, true}, {`\<tok\>`, "atok b", false, false}, {`\<tok\>`, "a toks", false, false},
		{`[[:alpha:]_][[:digit:]]`, "_7", false, true}, {`[^[:space:]]x`, " x", false, false},
		{`\<wbtok[0-9]{3}\>`, "id wbtok123 here", false, true}, {`\<wbtok[0-9]{3}\>`, "awbtok123b", false, false},
		{`\<wbtok[0-9]{3}\>`, "wbtok1234", false, false}, {`\<[A-Za-z_]+\>`, "--", false, false}, {`\<[A-Za-z_]+\>`, "-x-", false, true},
	}
	grep, _ := exec.LookPath("grep")
	for _, c := range cases {
		re, err := CompileERE(c.pat, c.icase)
		if err != nil {
			t.Fatalf("%q: %v", c.pat, err)
		}
		if got := re.Match([]byte(c.line)); got != c.want {
			t.Errorf("%q ~ %q (icase %v): %v, 셸 판은 %v", c.pat, c.line, c.icase, got, c.want)
		}
		if grep == "" {
			continue
		}
		flags := "-qE"
		if c.icase {
			flags = "-qEi"
		}
		g := exec.Command(grep, flags, "-e", c.pat)
		g.Env = append(os.Environ(), "LC_ALL=C")
		g.Stdin = strings.NewReader(c.line + "\n")
		if sh := g.Run() == nil; sh != c.want {
			t.Errorf("시험 기대값이 셸 grep과 다름: %q ~ %q (icase %v): grep %v", c.pat, c.line, c.icase, sh)
		}
	}
	if _, err := CompileERE(`(a)\1`, false); err == nil {
		t.Error("Go가 못 읽는 꼴(역참조)은 오류여야 — 조용히 통과하지 않게")
	}
}

// 재감사 ops R-18 · base R-09: C 로캘 ERE와 같은 뜻을 보장 못 하는 꼴은 읽을 때 거절(조용히 다르게 걸지 않게).
// 맥 /usr/bin/grep(BSD 2.6.0) LC_ALL=C 실측: \d는 숫자, \v·\A·\z·\p·\0은 그 글자, \b는 아무 데나, [[=a=]]·[[.a.]]는 a,
// \>tok·tok\<는 늘 안 맞음 — Go 옮김(\d 숫자·\v 세로 탭·\A 앞 고정·\b 낱말 경계, 동등 클래스·정렬 요소 못 읽음)과 어긋난다
func TestCompileERERejectsUnsure(t *testing.T) {
	bad := []string{`\d`, `\w`, `\s`, `\b`, `\B`, `\v`, `\A`, `\z`, `\p`, `\0`, `\x41`, `\Q`, `\n`, `\t`, `\e`,
		`[[=a=]]`, `[[.a.]]`, `x[[=a=]b]`, `[[:word:]]`, `[[:^alpha:]]`,
		`\>tok`, `tok\<`, `a.\>`, `\<.a`, `\<a?b`, `\<a*`, `[[:>:]]x`, `(?i)a`, `a{,2}`,
		`\<[^a]x`, `[0-9]{0,3}\>`, `[[:punct:]]\>`, `[a-]\>`, `(ab)\>`, `\<[a-z]{0}x`}
	for _, p := range bad {
		if _, err := CompileERE(p, false); err == nil {
			t.Errorf("%q: C 로캘 ERE와 뜻이 같다고 보장 못 하는 꼴인데 받았다", p)
		}
		d, base := t.TempDir(), t.TempDir()
		os.WriteFile(d+"/private-patterns.txt", []byte(p+"\n"), 0o644)
		os.WriteFile(base+"/f.txt", []byte("x\n"), 0o644)
		var out, errb bytes.Buffer
		if rc := LintAt(base, d, false, []string{"f.txt"}, &out, &errb); rc != 2 {
			t.Errorf("lint 패턴 %q: 종료 %d(검사 못 함 2여야)", p, rc)
		}
	}
	// 받는 꼴: 문장 부호의 역슬래시(글자 그대로), 낱말 글자 바로 앞의 \< · 바로 뒤의 \>, POSIX 문자 부류 12개, 게시기 기본 패턴
	good := []string{`a\.b`, `\/x`, `\-x`, `\}`, `\]`, `\\d`, `\<tok\>`, `\<t+`, `[[:<:]]tok[[:>:]]`, `[[:alpha:][:digit:]_]`,
		`[^[:space:]]x`, `(^|[^A-Za-z0-9])sk-[A-Za-z0-9_-]{30,}`, `-----BEGIN[A-Z0-9 ]*PRIVATE KEY`, `a{2,3}`, "x\\\xc3\xa9",
		`\<wbtok[0-9]{3}\>`, `\<[A-Za-z_]+\>`, `\<[[:alpha:]][[:alnum:]]{2,}\>`}
	for _, p := range good {
		if _, err := CompileERE(p, false); err != nil {
			t.Errorf("%q: 받아야 할 꼴을 거절: %v", p, err)
		}
	}
}

func TestGet(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(d+"/config", []byte("# c\n a.b  x  y \r\nc.d\nBad.Key z\n"), 0o644)
	if v, ok, err := Get(d, "a.b"); err != nil || !ok || v != "x  y" {
		t.Errorf("a.b = %q %v %v", v, ok, err)
	}
	if v, ok, _ := Get(d, "c.d"); !ok || v != "" {
		t.Errorf("키 혼자 = %q %v", v, ok)
	}
	if _, ok, _ := Get(d, "e.f"); ok {
		t.Errorf("없는 키가 있다고 함")
	}
	os.WriteFile(d+"/config", []byte("a.b 1\nc.d 2\na.b 3\n"), 0o644)
	var dup *ErrDup
	if _, _, err := Get(d, "c.d"); !errors.As(err, &dup) {
		t.Errorf("같은 키 두 번이 오류가 아님: %v", err)
	}
}

// B14·M5: 옛 셸 판(LC_ALL=C grep -E -i, 바이트 단위)과 같게 건다. want는 옛 bin/herdr-tools-config를 HERDR_TOOLS_CONFIG_IMPL=bash로
// 부른 실제 종료 코드를 기록한 골든이다(셸 판은 2026-11-22 go-shell-remove로 걷었다 — 이제 기록과만 견준다)
func TestLintBytesLikeShell(t *testing.T) {
	cases := []struct {
		pat, line string
		want      int
	}{
		{`\<tok\>`, "a tok b", 1}, {`\<tok\>`, "atokb", 0}, {`a...b`, "a한b", 1},
		{`[[:<:]]tok[[:>:]]`, "x tok y", 1}, {`[[:<:]]tok[[:>:]]`, "xtoky", 0}, {`^예...끝$`, "예한끝", 1},
		{`TOK`, "tok", 1}, {`é`, "É", 0}, {`x[가]y`, "x\xb0y", 1}, {`a.b`, "a\xffb", 1},
		{`[\.]x`, `\x`, 1}, {`a\.b`, "axb", 0}, {`\\<x`, `\<x`, 1}, {`\<tok`, "한tok", 1}, {`tok\>`, "tok한", 1},
		{`[^a]b`, "\xffb", 1},
	}
	for _, c := range cases {
		d, base := t.TempDir(), t.TempDir()
		os.WriteFile(d+"/private-patterns.txt", []byte(c.pat+"\n"), 0o644)
		os.WriteFile(base+"/f.txt", []byte(c.line+"\n"), 0o644)
		var out, errb bytes.Buffer
		if rc := LintAt(base, d, false, []string{"f.txt"}, &out, &errb); rc != c.want {
			t.Errorf("패턴 %q 줄 %q: Go %d, 기록한 셸 판 %d (%s)", c.pat, c.line, rc, c.want, errb.String())
		}
	}
}

func TestLintAt(t *testing.T) {
	c, base := t.TempDir(), t.TempDir()
	os.WriteFile(c+"/private-patterns.txt", []byte("# 주석\nsecret-[0-9]+\n"), 0o644)
	os.MkdirAll(filepath.Join(base, "d"), 0o755)
	os.WriteFile(filepath.Join(base, "d/a.txt"), []byte("ok\nSECRET-1 here\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := LintAt(base, c, false, []string{"d/a.txt"}, &out, &errb); rc != 1 || out.String() != "d/a.txt:2: secret-[0-9]+\n" {
		t.Errorf("LintAt rc=%d out=%q err=%q", rc, out.String(), errb.String())
	}
	out.Reset()
	if rc := LintAt(base, c, false, []string{"d/없음"}, &out, &errb); rc != 2 {
		t.Errorf("못 읽는 파일은 2: %d", rc)
	}
	if !HasPatterns(c+"/private-patterns.txt") || HasPatterns(c+"/없음") {
		t.Errorf("HasPatterns")
	}
}
