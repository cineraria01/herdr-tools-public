package ghpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PR 글 읽기: GraphQL 응답 한 장 → 종류(변경 요청·승인·의견 리뷰·줄 댓글·대화 댓글), 보류 리뷰·본문 없는 의견 리뷰는 뺌, 봇·지운 계정 표시
func TestNotes(t *testing.T) {
	d := t.TempDir()
	js := `{"data":{"viewer":{"login":"me-1"},"repository":{"pullRequests":{"nodes":[{"number":7,
"comments":{"nodes":[{"databaseId":11,"body":"대화","createdAt":"2030-01-01T00:00:01Z","author":{"__typename":"User","login":"u1"}},
 {"databaseId":12,"body":"봇","createdAt":"2030-01-01T00:00:02Z","author":{"__typename":"Bot","login":"ci"}},
 {"databaseId":13,"body":"지운 계정","createdAt":"2030-01-01T00:00:03Z","author":null}]},
"reviews":{"nodes":[{"databaseId":21,"state":"CHANGES_REQUESTED","body":"고쳐","submittedAt":"2030-01-01T00:00:04Z","author":{"__typename":"User","login":"u1"},
   "comments":{"nodes":[{"databaseId":31,"body":"여기","path":"a.go","line":null,"originalLine":12,"createdAt":"2030-01-01T00:00:05Z","author":{"__typename":"User","login":"u1"}}]}},
 {"databaseId":22,"state":"COMMENTED","body":"","submittedAt":"2030-01-01T00:00:06Z","author":{"__typename":"User","login":"u1"},
   "comments":{"nodes":[{"databaseId":32,"body":"줄만","path":"b.go","line":3,"createdAt":"2030-01-01T00:00:07Z","author":{"__typename":"User","login":"u1"}}]}},
 {"databaseId":23,"state":"PENDING","body":"쓰는 중","submittedAt":null,"author":{"__typename":"User","login":"u1"},
   "comments":{"nodes":[{"databaseId":33,"body":"보류 줄","path":"c.go","line":1,"createdAt":"2030-01-01T00:00:08Z","author":{"__typename":"User","login":"u1"}}]}},
 {"databaseId":24,"state":"APPROVED","body":"","submittedAt":"2030-01-01T00:00:09Z","author":{"__typename":"User","login":"u2"},"comments":{"nodes":[]}},
 {"databaseId":25,"state":"DISMISSED","body":"무름","submittedAt":"2030-01-01T00:00:10Z","author":{"__typename":"User","login":"u2"},"comments":{"nodes":[]}}]}}]}}}}`
	if err := os.WriteFile(filepath.Join(d, "out.json"), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(d, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\necho \"$*\" >\""+d+"/args\"\ncat \""+d+"/out.json\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Client{Bin: gh, Max: 5}
	me, ns, err := c.Notes(Repo{Host: "ghe.example.org", Owner: "o", Name: "n"}, 30)
	if err != nil || me != "me-1" {
		t.Fatalf("Notes: %q %v", me, err)
	}
	var got []string
	for _, n := range ns {
		got = append(got, n.ID+" "+n.Kind+" "+n.Author+" "+n.Where+" "+strings.Repeat("B", btoi(n.Bot)))
	}
	want := []string{"c11 comment u1  ", "c12 comment ci  B", "c13 comment   ", "r21 changes u1  ", "l31 line u1 a.go:12 ", "l32 line u1 b.go:3 ", "r24 approve u2  "}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("글:\n%s\n기대:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	a, _ := os.ReadFile(filepath.Join(d, "args"))
	if !strings.Contains(string(a), "api graphql") || !strings.Contains(string(a), "owner=o") || !strings.Contains(string(a), "--hostname ghe.example.org") || c.Calls() != 1 {
		t.Fatalf("부른 꼴(%d번): %s", c.Calls(), a)
	}
	if err := os.WriteFile(filepath.Join(d, "out.json"), []byte(`{"errors":[{"message":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Notes(Repo{Owner: "o", Name: "n"}, 30); err == nil {
		t.Fatal("저장소 없는 응답을 빈 목록으로 읽었다")
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
