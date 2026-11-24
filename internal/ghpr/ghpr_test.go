package ghpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:example-org/proj.git":          "example-org/proj",
		"https://github.com/example-org/proj":          "example-org/proj",
		"https://user@github.com/example-org/proj.git": "example-org/proj",
		"ssh://git@github.com:22/example-org/proj.git": "example-org/proj",
		"git@ghe.example.org:team/proj.git":            "ghe.example.org/team/proj",
	} {
		r, ok := ParseRemote(in)
		if !ok || r.Arg() != want {
			t.Errorf("%s → %v %q, 기대 %q", in, ok, r.Arg(), want)
		}
	}
	for _, in := range []string{"", "/srv/git/proj.git", "../proj", "file:///srv/proj", "https://github.com/only"} {
		if _, ok := ParseRemote(in); ok {
			t.Errorf("%q: 저장소로 읽으면 안 된다", in)
		}
	}
}

// 상한: Max번 부른 뒤로는 부르지 않고 ErrBudget. As로 만든 짝과 함께 센다. 토큰은 오류 글에서 지운다
func TestBudgetAndToken(t *testing.T) {
	d := t.TempDir()
	gh := filepath.Join(d, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\necho \"bad $GH_TOKEN\" >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Client{Bin: gh, Max: 2}
	bot := c.As("tok-777")
	err := bot.Close(Repo{Owner: "o", Name: "n"}, 1)
	if err == nil || strings.Contains(err.Error(), "tok-777") || !strings.Contains(err.Error(), "‹토큰›") {
		t.Fatalf("오류 글 = %v", err)
	}
	if err := c.Close(Repo{Owner: "o", Name: "n"}, 1); err == nil || err == ErrBudget {
		t.Fatalf("둘째 = %v", err)
	}
	if err := bot.Close(Repo{Owner: "o", Name: "n"}, 1); err != ErrBudget || c.Calls() != 2 {
		t.Fatalf("셋째 = %v, 부른 수 %d", err, c.Calls())
	}
	if err := (&Client{Bin: filepath.Join(d, "없음")}).Close(Repo{}, 1); err != ErrNoGh {
		t.Fatalf("gh 없음 = %v", err)
	}
}
