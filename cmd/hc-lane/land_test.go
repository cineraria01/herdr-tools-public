package main

// 단위 시험: 셸 판과 같은 판정이어야 하는 순수 함수들. 착지 전체 계약은 tests/hc-lane*.test.sh(Go 판·bash 판 두 번)가 본다.

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		cmd   string
		args  []string
		usage bool
		msg   string // die 글(빈 값이면 통과)
	}{
		{"land", nil, false, ""},
		{"land", []string{"main"}, false, ""},
		{"land", []string{"main", "--verdict", "a1"}, false, ""},
		{"land", []string{"--repo", "x"}, false, "hc-lane: land는 레인 작업 폴더 안에서 인자 없이(또는 기본 브랜치 하나만) 부른다: hc-lane land [<기본 브랜치>]"},
		{"land", []string{"--verdict", "a.b"}, false, "hc-lane: --verdict 판정 ID 글자: a.b"},
		{"land", []string{"--verdict"}, false, "hc-lane: --verdict 값이 없다"},
		{"land", []string{"-h"}, true, ""},
		{"land", []string{"main", "extra"}, false, "hc-lane: 알 수 없는 인자: extra (실행 명령 앞에는 -- 를 둔다)"},
		{"land", []string{"--bogus"}, false, "hc-lane: 알 수 없는 인자: --bogus"},
		{"guard", []string{"status", "--repo", "r"}, false, ""},
		{"guard", []string{"--repo", "r"}, false, "hc-lane: 알 수 없는 인자: r (실행 명령 앞에는 -- 를 둔다)"}, // 셸 판도 동작 자리에 --repo를 먹는다
		{"guard", []string{"bogus", "--repo", "r"}, true, ""},
		{"guard", []string{"install"}, true, ""},
		{"guard", []string{"install", "--repo", "r", "--on", "m"}, false, "hc-lane: guard에는 --repo(와 --branch <기본 브랜치>)만 쓴다. 다른 기계는 hc exec --on <기계> -- hc-lane guard …"},
		{"guard", []string{"status", "--repo", "r", "--verdict", "x"}, false, "hc-lane: --verdict는 land에서만 쓴다"},
		{"lease", []string{"show", "--repo", "r", "--name", "a"}, false, ""},
		{"lease", []string{"take", "--repo", "r", "--name", "a"}, true, ""},
		{"lease", []string{"drop", "--repo", "r", "--name", "A"}, false, "hc-lane: --name은 a-z 0-9 - _ 만 쓴다(hc 규칙): A"},
		{"lease", []string{"drop", "--repo", "r", "--name", strings.Repeat("a", 33)}, false, "hc-lane: --name은 32자까지: " + strings.Repeat("a", 33)},
		{"lease", []string{"show", "--repo", "r"}, false, "hc-lane: lease show|drop에는 --on·--repo·--name만 쓴다"},
		{"fetch", []string{"--repo", "r", "main", "x"}, false, ""},
		{"fetch", []string{"--repo", "r", "main", "-x"}, false, "hc-lane: fetch 참조는 - 로 시작하지 않는다(git 옵션으로 읽힌다): -x"},
		{"fetch", []string{"main"}, true, ""},
		{"fetch", []string{"--repo", "r", "--dry-run"}, false, "hc-lane: fetch에는 --on·--repo와 참조만 쓴다"},
		{"land", []string{"--brief", "b"}, false, "hc-lane: --brief·--start-wait는 레인을 띄울 때만 쓴다"},
		{"land", []string{"--resume"}, false, "hc-lane: --resume은 쓰기 레인을 띄울 때만 쓴다"},
		{"land", []string{"--integ="}, false, "hc-lane: --integ= 뒤에 접두가 없다"},
	}
	for _, c := range cases {
		_, pe := parseArgs(c.cmd, c.args)
		switch {
		case c.usage:
			if pe == nil || !pe.usage {
				t.Errorf("%s %q: usage여야 한다, %+v", c.cmd, c.args, pe)
			}
		case c.msg == "":
			if pe != nil {
				t.Errorf("%s %q: 통과여야 한다, %+v", c.cmd, c.args, pe)
			}
		default:
			if pe == nil || pe.usage || pe.msg != c.msg || pe.rc != 1 {
				t.Errorf("%s %q: %q 여야 한다, %+v", c.cmd, c.args, c.msg, pe)
			}
		}
	}
}

func TestJrec(t *testing.T) {
	js := `{"terminals":[{"address":"t1","agent":"zsh","cwd":"/a b","state":"live","turn":0},{"address":"m/t2","cwd":"/x","state":"live","status":"working","foreground":{"pid":3}}]}`
	got := jrec(js, "address", "state", "status", "cwd")
	if len(got) != 2 || strings.Join(got[0], "|") != "t1|live|-|/a b" || strings.Join(got[1], "|") != "m/t2|live|working|/x" {
		t.Fatalf("jrec: %q", got)
	}
}

func TestRRPaths(t *testing.T) {
	out := "Auto-merging a\nCONFLICT (content): Merge conflict in a b.txt\nCONFLICT (modify/delete): x deleted\nResolved 'a b.txt' using previous resolution.\n"
	if p := rrPaths(out); len(p) != 1 || p[0] != "a b.txt" {
		t.Fatalf("rrPaths: %q", p)
	}
}

func TestSmallHelpers(t *testing.T) {
	if !plain("feat/x_1.2-a") || plain("a b") || plain("") || plain("ㄱ") {
		t.Error("plain")
	}
	if !lnameOK("a-b_1") || lnameOK("-a") || lnameOK("A") || lnameOK(strings.Repeat("a", 33)) || lnameOK("") {
		t.Error("lnameOK")
	}
	if trTo("a/b 한", alnum+"._-") != "a-b----" {
		t.Errorf("trTo: %q", trTo("a/b 한", alnum+"._-"))
	}
	if cutF("a\tb\tc", 2) != "b" || cutF("abc", 2) != "abc" || cutF("a\tb", 3) != "" {
		t.Error("cutF")
	}
	if field2("s1\trefs/a\ns2\trefs/b\n", "refs/b") != "s2" {
		t.Error("field2")
	}
	if firstWord("abc def") != "abc" || firstWord("") != "" {
		t.Error("firstWord")
	}
	if countNonEmpty("a\n\nb") != 2 || countNonEmpty("") != 0 {
		t.Error("countNonEmpty")
	}
	h := guardHook("main")
	if !strings.HasPrefix(h, "#!/bin/sh\n"+gmark+"\n") || !strings.Contains(h, "\nbase=main\n") || !strings.HasSuffix(h, "done\nexit 0\n") {
		t.Errorf("guardHook: %q", h)
	}
}
