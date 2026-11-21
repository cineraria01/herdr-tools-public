package main

import (
	"regexp"
	"testing"
)

func TestSelfTest(t *testing.T) {
	if err := selfTest(); err != nil {
		t.Fatal(err)
	}
}

func TestBasenameLikePython(t *testing.T) {
	for in, want := range map[string]string{"/r/proj": "proj", "/r/proj/": "", "": "", "proj": "proj"} {
		if g := basename(in); g != want {
			t.Errorf("%q → %q, 바람 %q", in, g, want)
		}
	}
}

func TestUUID4(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if a, b := uuid4(), uuid4(); !re.MatchString(a) || a == b {
		t.Fatal(a, b)
	}
}

func TestPlaceSkipsRemoteAndNoneCwd(t *testing.T) {
	me := obj{"address": "t9"} // cwd 없음 — 파이썬처럼 cwd 없는 후보와는 같은 폴더로 친다
	p, _ := place(me, []obj{{"address": "w1/t2"}, {"address": "t3"}}, func(a string) string { return "sp-" + a })
	if p[space] != "sp-t3" || p[wsName] != "ovis" {
		t.Fatal(p)
	}
}
