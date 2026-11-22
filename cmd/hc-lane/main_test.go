package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// R-29: 긴 이력 파일에서 끝 한 줄만 읽을 때 파일 전체를 올리지 않는다(읽은 할당이 파일 크기보다 훨씬 작다)
func TestTailFileLinesAlloc(t *testing.T) {
	p := t.TempDir() + "/ev"
	line := strings.Repeat("y", 99) + "\n"
	os.WriteFile(p, []byte(strings.Repeat(line, 40000)+"last\n"), 0o644) // 약 4MB
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	ls, err := tailFileLines(p, 1)
	runtime.ReadMemStats(&b)
	if err != nil || len(ls) != 1 || ls[0] != "last" {
		t.Fatalf("tailFileLines 1: %v %v", ls, err)
	}
	if got := b.TotalAlloc - a.TotalAlloc; got > 1<<20 {
		t.Fatalf("끝 한 줄에 %d바이트 할당 — 파일 전체를 읽은 꼴", got)
	}
}

// R-28: 64KB를 넘는 한 줄의 뒤쪽 실패 표식도 실패 요약(eachLine → failLine)에 잡힌다
func TestEachLineLongTail(t *testing.T) {
	p := t.TempDir() + "/log"
	os.WriteFile(p, []byte("ok\n"+strings.Repeat("x", 66000)+" --- FAIL: TestZ\nend\n"), 0o644)
	hit, n := false, 0
	eachLine(p, func(l string) bool {
		n++
		hit = hit || failLine.MatchString(l)
		return true
	})
	if !hit {
		t.Fatalf("긴 줄 뒤쪽 실패 표식을 놓침(조각 %d개)", n)
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

// 사용법 글에 셸 판·되돌리기 스위치 글이 남지 않는다(2026-11-22 go-shell-remove — 판은 Go 하나)
func TestUsageNoShell(t *testing.T) {
	if !strings.HasPrefix(usageText, "사용법: hc-lane ") {
		t.Fatal("usage.txt 첫 줄이 「사용법: hc-lane」가 아니다")
	}
	for _, w := range []string{"셸 판", "HC_LANE_IMPL", "HERDR_IMPL", "HC_LANE_GO_BIN", "HC_LANE_SH"} {
		if strings.Contains(usageText, w) {
			t.Errorf("usage.txt에 %q가 남았다", w)
		}
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
		// 기대값은 등록표에서 꺼내지 않고 적는다(gofix L16 — 등록이 빠지면 여기서 진다). merger는 곁의 hc-merger로 넘긴다
		{[]string{"report", "완료"}, true, 1},
		{[]string{"merger", "--once"}, true, 1},
		{[]string{"merger", "-h"}, false, 1},
	}
	for _, c := range cases {
		_, rest, ok := pick(c.args)
		if ok != c.ok || (ok && len(rest) != c.rest) {
			t.Errorf("pick(%q) = %v·%d, 바람 %v·%d", c.args, ok, len(rest), c.ok, c.rest)
		}
	}
}

// buildLane: 이 꾸러미를 dir/hc-lane으로 짓는다
func buildLane(t *testing.T, dir string) string {
	t.Helper()
	gob, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go를 못 찾음: %v", err)
	}
	bin := dir + "/hc-lane"
	if out, err := exec.Command(gob, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("빌드: %v %s", err, out)
	}
	return bin
}

// 도움말·모르는 인자는 사용법 글(오류 출력)과 종료 2 — 옛 Go 판은 120(「셸 판으로」)을 냈다(go-shell-remove)
func TestBinaryUsage(t *testing.T) {
	bin := buildLane(t, t.TempDir())
	for _, a := range [][]string{{"-h"}, {"--help"}, {"report", "-h"}, {"send", "--help"}, {"merger", "-h"}, {"clean", "-h"},
		{"land", "-h"}, {"guard", "-h"}, {"lease", "-h"}, {"fetch", "-h"}, {"coord", "-h"}, {"watch", "-h"}} {
		c := exec.Command(bin, a...)
		var e strings.Builder
		c.Stderr = &e
		err := c.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 || !strings.HasPrefix(e.String(), "사용법: hc-lane ") {
			t.Errorf("%q: 종료 2·사용법이어야 — %v, 오류 출력 %.60q", a, err, e.String())
		}
	}
}

// merger: 바이너리 곁의 hc-merger로 넘긴다 — 인자 그대로, HC_LANE_SELF = 이 hc-lane 실제 경로, 잠금 fd 19는 물려주지 않는다.
// 바꿔 끼우기 환경(HC_LANE_MERGER_BIN·HC_MERGER_BIN)은 받지 않는다. 옛 Go 판은 merger를 셸 판으로 넘겼다(120)
func TestMergerExec(t *testing.T) {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := buildLane(t, d)
	out := d + "/out"
	fake := "#!/bin/sh\n{ printf 'self=%s\\n' \"$HC_LANE_SELF\"; printf 'arg=%s\\n' \"$@\"; if (: <&19) 2>/dev/null; then echo fd19=열림; else echo fd19=닫힘; fi; } >\"$OUT\"\nexit 7\n"
	if err := os.WriteFile(d+"/hc-merger", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	other := d + "/other"
	os.WriteFile(other, []byte("#!/bin/sh\necho 바꿔끼움 >\"$OUT\"\n"), 0o755)
	link := t.TempDir() + "/hc-lane" // 링크로 불려도 실제 파일 곁을 본다
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("sh", "-c", `exec "$0" merger --repo "/공백 있는/r" --once 19<"$0"`, link)
	c.Env = append(os.Environ(), "OUT="+out, "PATH=/usr/bin:/bin", "HC_LANE_MERGER_BIN="+other, "HC_MERGER_BIN="+other)
	err = c.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 7 {
		t.Fatalf("hc-merger의 종료 코드(7)가 그대로여야: %v", err)
	}
	want := "self=" + bin + "\narg=--repo\narg=/공백 있는/r\narg=--once\nfd19=닫힘\n"
	if b, _ := os.ReadFile(out); string(b) != want {
		t.Fatalf("넘김이 다르다:\n%s\n바람:\n%s", b, want)
	}
	// 곁에도 PATH에도 없으면 종료 1과 한 줄
	os.Remove(d + "/hc-merger")
	c = exec.Command(bin, "merger", "--once")
	c.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	var e strings.Builder
	c.Stderr = &e
	err = c.Run()
	if !errors.As(err, &ee) || ee.ExitCode() != 1 || !strings.Contains(e.String(), "hc-merger를 찾지 못했다") {
		t.Fatalf("없으면 종료 1: %v %q", err, e.String())
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
