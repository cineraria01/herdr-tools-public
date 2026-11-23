package main

import (
	"strings"
	"testing"

	"herdr-tools/internal/events"
)

// watch-project-route: 같은 저장소는 주소 꼴·클론 경로가 달라도 같은 값, 기계마다 다른 로컬 경로·이상한 주소는 모름("")
func TestRepoID(t *testing.T) {
	for u, want := range map[string]string{
		"git@github.com:Org/Repo.git":           "github.com/org/repo",
		"https://github.com/org/repo":           "github.com/org/repo",
		"https://user:tok@github.com/org/repo/": "github.com/org/repo",
		"ssh://git@github.com:22/org/repo.git":  "github.com/org/repo",
		"github.com:org/repo.git\n":             "github.com/org/repo",
		"/Users/x/repos/repo.git":               "",
		"../repo.git":                           "",
		"file:///srv/repo.git":                  "",
		"C:/repo":                               "",
		"":                                      "",
		"https://github.com/":                   "",
		"git@h:o/re po":                         "",
	} {
		if got := repoID(u); got != want {
			t.Errorf("repoID(%q) = %q, 바람 %q", u, got, want)
		}
	}
}

// 13칸은 1000바이트 안에 — 한 줄 칸을 줄여 싣고, 저장소가 없으면 지금 꼴 12칸 그대로
func TestWithRepo(t *testing.T) {
	e := events.Event{Time: "2026-11-24T00:00:00Z", Machine: "m", Lane: "l", State: "완료", Msg: strings.Repeat("가", 600)}
	if l := withRepo(e, ""); strings.Count(l, "\t") != 11 || l != e.Line() {
		t.Fatal("저장소 모름은 지금 꼴 그대로")
	}
	l := withRepo(e, "github.com/org/repo")
	f := strings.Split(l, "\t")
	if len(f) != 13 || f[12] != "github.com/org/repo" || len(l) > events.MaxLine || f[3] != "l" {
		t.Fatalf("13칸·1000바이트 안: %d칸 %d바이트", len(f), len(l))
	}
	if p, ok := events.Parse(l); !ok || p.Key != "-" || p.Owner != "-" {
		t.Fatalf("옛 판 판독기(events.Parse)도 12칸까지 그대로: %+v", p)
	}
}
