package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// gofix L14: 끝 n줄만 거꾸로 읽기(64KB 조각 경계를 넘는 줄 포함)·줄마다 읽기(멈춤)
func TestTailFileLines(t *testing.T) {
	p := t.TempDir() + "/ev"
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString(strings.Repeat("x", i%97) + "|" + strconv.Itoa(i) + "\n")
	}
	os.WriteFile(p, []byte(b.String()), 0o644)
	ls, err := tailFileLines(p, 500)
	if err != nil || len(ls) != 500 || !strings.HasSuffix(ls[0], "|4500") || !strings.HasSuffix(ls[499], "|4999") {
		t.Fatalf("tailFileLines 500: %d줄 %v", len(ls), err)
	}
	all, _ := tailFileLines(p, 0)
	if few, _ := tailFileLines(p, 9000); len(all) != 5000 || len(few) != 5000 {
		t.Fatalf("전체·n이 줄 수보다 클 때: %d %d", len(all), len(few))
	}
	n := 0
	eachLine(p, func(string) bool { n++; return n < 40 })
	if n != 40 {
		t.Fatalf("eachLine 멈춤: %d", n)
	}
}

// gofix E04: 다른 작업 트리가 같은 .qa 경로를 먼저 넣으면(복사 도중) 덮지 않고 d — 가짜 cp가 복사 직전에 목적지를 다른 내용으로 만든다
func TestQaScriptNoOverwriteRace(t *testing.T) {
	d := t.TempDir()
	wt, mw, fk := d+"/wt", d+"/main", d+"/bin"
	for _, c := range [][]string{{"git", "init", "-q", wt}, {"mkdir", "-p", wt + "/.qa/s", mw, fk}} {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", c, err, out)
		}
	}
	os.WriteFile(wt+"/.gitignore", []byte(".qa/\n"), 0o644)
	os.WriteFile(wt+"/.qa/s/r.txt", []byte("A\n"), 0o644)
	os.WriteFile(fk+"/cp", []byte("#!/bin/sh\nmkdir -p \"$OTHER_DIR\" && printf 'B\\n' >\"$OTHER\"; exec /bin/cp \"$@\"\n"), 0o755)
	c := exec.Command("sh", "-c", qaScript, "hc-lane", wt, mw, wt, "0")
	c.Env = append(os.Environ(), "PATH="+fk+":"+os.Getenv("PATH"), "OTHER="+mw+"/.qa/s/r.txt", "OTHER_DIR="+mw+"/.qa/s")
	out, err := c.Output()
	if err != nil {
		t.Fatalf("qaScript: %v", err)
	}
	b, _ := os.ReadFile(mw + "/.qa/s/r.txt")
	if strings.TrimSpace(string(out)) != "d .qa/s/r.txt" || string(b) != "B\n" {
		t.Fatalf("먼저 넣은 쪽을 덮었거나 c로 보고: %q · 목적지 %q", out, b)
	}
	if es, _ := os.ReadDir(mw + "/.qa/s"); len(es) != 1 {
		t.Fatalf("임시 파일이 남음: %v", es)
	}
}

// Windows·Linux 교차 빌드가 깨지지 않는다(gofix L11 — 유닉스 전용 호출은 sys_unix.go에만). 실행 동작 보장은 macOS만
func TestCrossBuild(t *testing.T) {
	gob, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go를 못 찾음: %v", err)
	}
	for _, os2 := range []string{"windows", "linux"} {
		c := exec.Command(gob, "build", "-o", os.DevNull, ".")
		c.Env = append(os.Environ(), "GOOS="+os2, "GOARCH=amd64", "CGO_ENABLED=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Errorf("GOOS=%s 빌드 실패: %v\n%s", os2, err, out)
		}
	}
}

// 도움말 글이 셸 판 usage(lib/hc-lane/common.sh의 heredoc)와 같은지 — 한쪽만 고치면 여기서 진다
func TestUsageMatchesShell(t *testing.T) {
	b, err := os.ReadFile("../../lib/hc-lane/common.sh")
	if err != nil {
		t.Fatalf("셸 판 읽기: %v", err)
	}
	_, after, ok := strings.Cut(string(b), "  cat >&2 <<'EOF'\n")
	if !ok {
		t.Fatal("셸 판 usage heredoc을 못 찾음")
	}
	body, _, ok := strings.Cut(after, "\nEOF\n")
	if !ok {
		t.Fatal("셸 판 usage 끝을 못 찾음")
	}
	if body+"\n" != usageText {
		t.Fatal("cmd/hc-lane/usage.txt가 셸 판 usage와 다르다 — awk로 다시 뽑는다(README 「hc-lane Go 판」)")
	}
}

func TestPick(t *testing.T) {
	cases := []struct {
		args []string
		ok   bool
		rest int
	}{
		{[]string{"--name", "x", "--repo", "/r", "--", "true"}, true, 6}, // 띄우기: 인자 전부
		{[]string{"clean", "--repo", "/r"}, true, 2},
		{[]string{"gc"}, true, 0},
		{[]string{"park", "한 줄"}, true, 1},
		// 기대값은 등록표에서 꺼내지 않고 적는다(gofix L16 — 등록이 빠지면 여기서 진다). merger는 셸 판(병합기는 cmd/hc-merger)
		{[]string{"report", "완료"}, true, 1},
		{[]string{"merger", "--once"}, false, 1},
	}
	for _, c := range cases {
		_, rest, ok := pick(c.args)
		if ok != c.ok || (ok && len(rest) != c.rest) {
			t.Errorf("pick(%q) = %v·%d, 바람 %v·%d", c.args, ok, len(rest), c.ok, c.rest)
		}
	}
}

// goHas: Go 판이 맡는 하위 명령(셸 판 첫 인자 분기 가운데 merger만 빼고 전부) — 실제 바이너리의 __has로 본다(gofix L16)
var goHas = [][]string{{"verdict", "x"}, {"early-wait", "a"}, {"report", "완료"}, {"send", "--all", "x"}, {"broadcast", "x"},
	{"events"}, {"wait"}, {"watch"}, {"coord", "--check", "t1"}, {"clean", "--repo", "/r"}, {"fetch", "--repo", "/r"}, {"gc"},
	{"land"}, {"guard", "status", "--repo", "/r"}, {"lease", "show", "--repo", "/r", "--name", "x"}, {"park"}, {"--name", "x", "--repo", "/r", "--", "true"}}

func TestBinaryHas(t *testing.T) {
	gob, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go를 못 찾음: %v", err)
	}
	bin := t.TempDir() + "/hc-lane"
	if out, err := exec.Command(gob, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	for _, a := range goHas {
		if err := exec.Command(bin, append([]string{"__has"}, a...)...).Run(); err != nil {
			t.Errorf("Go 판 __has %q: %v (맡아야 한다)", a, err)
		}
	}
	if err := exec.Command(bin, "__has", "merger", "--once").Run(); err == nil {
		t.Error("Go 판 __has merger: 셸 판이어야 하는데 0")
	}
}

func TestQ(t *testing.T) {
	if got := q("a b'c"); got != `'a b'\''c'` {
		t.Fatalf("q = %s", got)
	}
}

func TestGlobRe(t *testing.T) {
	for _, c := range []struct {
		p, s string
		ok   bool
	}{{"keep/*", "keep/a/b", true}, {"ke?p", "keep", true}, {"rel-[0-9]", "rel-3", true}, {"rel-[!0-9]", "rel-3", false}, {"a.b", "axb", false},
		{"stable-[[:digit:]]", "stable-1", true}, {"stable-[[:digit:]]", "stable-x", false}, {"보관*", "보관함", true}, {"?관함", "보관함", true},
		{"[]a]", "]", true}, {"[!]a]", "b", true}, {"[가-힣]", "한", true}, {`a\*`, "a*", true}, {`a\*`, "ab", false}} {
		re, err := globRe(c.p)
		if err != nil || re.MatchString(c.s) != c.ok {
			t.Errorf("globRe(%q) ~ %q 바람 %v (%v)", c.p, c.s, c.ok, err)
		}
	}
	for _, p := range []string{"st[a-", "x[[:nope:]]", "[[:digit:"} {
		if _, err := globRe(p); err == nil {
			t.Errorf("globRe(%q): 해석 못 하는 무늬인데 오류 없음", p)
		}
	}
}

func TestFirstTurn(t *testing.T) {
	for _, c := range []struct {
		g  string
		ok bool
	}{
		{`{"turn":1,"status":{"state":"idle"}}`, true},
		{`{"turn":0,"status":{"state":"working"}}`, true},
		{`{"turn":0,"status":{"state":"idle"}}`, false},
		{`{"turn":0,"status":null}`, false},
		{``, false},
	} {
		if firstTurn(c.g) != c.ok {
			t.Errorf("firstTurn(%s) 바람 %v", c.g, c.ok)
		}
	}
}

func TestCkey(t *testing.T) {
	if got := ckey("box/t1a·x"); got != "box_t1a__x" { // 셸 tr -c처럼 바이트마다(· 는 2바이트)
		t.Fatalf("ckey = %s", got)
	}
}
