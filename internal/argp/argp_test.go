package argp

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (Result, bool, int, string, string) {
	var o, e bytes.Buffer
	code := -1
	p := &Parser{Prog: "t", Usage: "usage: t\n", Help: "도움\n", Stdout: &o, Stderr: &e, Exit: func(c int) { code = c },
		Opts: []Opt{{"main", One}, {"add", Star}, {"drop", Plus}, {"watch", Flag}}}
	r, ok := p.Parse(args)
	return r, ok, code, o.String(), e.String()
}

func TestParse(t *testing.T) {
	r, ok, _, _, _ := run("--ma", "t1", "--add", "a", "b", "--add", "c", "--w", "--drop=x")
	if !ok || r.Get("main") != "t1" || strings.Join(r.Vals["add"], ",") != "c" || !r.Has["watch"] || r.Vals["drop"][0] != "x" {
		t.Fatalf("%+v", r)
	}
	if r, ok, _, _, _ := run("--add", "--main", "m"); !ok || !r.Has["add"] || len(r.Vals["add"]) != 0 || r.Get("main") != "m" {
		t.Fatalf("빈 --add: %+v", r)
	}
	cases := []struct {
		args []string
		code int
		err  string
	}{
		{[]string{"x"}, 2, "usage: t\nt: error: unrecognized arguments: x\n"},
		{[]string{"--main"}, 2, "usage: t\nt: error: argument --main: expected one argument\n"},
		{[]string{"--main", "-x"}, 2, "usage: t\nt: error: argument --main: expected one argument\n"},
		{[]string{"--drop"}, 2, "usage: t\nt: error: argument --drop: expected at least one argument\n"},
		{[]string{"--watch=1"}, 2, "usage: t\nt: error: argument --watch: ignored explicit argument '1'\n"},
		{[]string{"--bogus"}, 2, "usage: t\nt: error: unrecognized arguments: --bogus\n"},
	}
	for _, c := range cases {
		_, ok, code, _, e := run(c.args...)
		if ok || code != c.code || e != c.err {
			t.Errorf("%v: ok=%v code=%d %q", c.args, ok, code, e)
		}
	}
	if _, ok, code, o, _ := run("-h"); ok || code != 0 || o != "도움\n" {
		t.Errorf("-h: %v %d %q", ok, code, o)
	}
	if r, ok, _, _, _ := run("--main", "-5"); !ok || r.Get("main") != "-5" {
		t.Errorf("음수는 값: %+v", r)
	}
}
