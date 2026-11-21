package main

import (
	"os"
	"strings"
	"testing"
)

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
		{[]string{"report", "완료"}, subs["report"].run != nil, 1}, // 다른 레인이 옮기기 전에는 아직 셸
		{[]string{"merger", "--once"}, subs["merger"].run != nil, 1},
	}
	for _, c := range cases {
		_, rest, ok := pick(c.args)
		if ok != c.ok || (ok && len(rest) != c.rest) {
			t.Errorf("pick(%q) = %v·%d, 바람 %v·%d", c.args, ok, len(rest), c.ok, c.rest)
		}
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
	}{{"keep/*", "keep/a/b", true}, {"ke?p", "keep", true}, {"rel-[0-9]", "rel-3", true}, {"rel-[!0-9]", "rel-3", false}, {"a.b", "axb", false}} {
		if globRe(c.p).MatchString(c.s) != c.ok {
			t.Errorf("globRe(%q) ~ %q 바람 %v", c.p, c.s, c.ok)
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
