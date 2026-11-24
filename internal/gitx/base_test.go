package gitx

import (
	"strings"
	"testing"
)

// dedupe-helpers ③: hc-lane repoDefaults·hc-merger originBase 두 옛 판의 꼴을 덮는 표
func TestDefaultBaseTable(t *testing.T) {
	cases := []struct {
		name   string
		getURL int
		refs   string
		refsRC int
		remote bool
		base   string
		rc     int
	}{
		{"원격 HEAD가 가리키는 가지", 0, "refs/remotes/origin/trunk", 0, true, "trunk", 0},
		{"원격 HEAD 없음이면 main", 0, "", 0, true, "main", 0},
		{"원격 HEAD 조회 실패", 0, "", 128, false, "", 128},
		{"원격 HEAD 시간 넘김", 0, "", 124, false, "", 124},
		{"origin 없음 main·master 둘", 2, "refs/heads/main\nrefs/heads/master", 0, false, "main", 0},
		{"origin 없음 master만", 2, "refs/heads/master", 0, false, "master", 0},
		{"origin 없음 둘 다 없음", 2, "", 0, false, "main", 0},
		{"origin 없음 로컬 조회 실패", 2, "", 1, false, "", 1},
		{"get-url 실패", 128, "", 0, false, "", 128},
		{"get-url 시간 넘김", 124, "", 0, false, "", 124},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			remote, base, rc := DefaultBase(func(quiet bool, args ...string) (string, int) {
				if args[0] == "remote" {
					if !quiet {
						t.Fatal("get-url은 출력을 버린다")
					}
					return "", c.getURL
				}
				if c.getURL == 0 != strings.Contains(strings.Join(args, " "), "refs/remotes/origin/HEAD") {
					t.Fatalf("다른 조회: %v", args)
				}
				return c.refs, c.refsRC
			})
			if remote != c.remote || base != c.base || rc != c.rc {
				t.Fatalf("%v %q %d (바람 %v %q %d)", remote, base, rc, c.remote, c.base, c.rc)
			}
		})
	}
}
