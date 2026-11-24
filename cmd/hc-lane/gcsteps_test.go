package main

import (
	"reflect"
	"testing"
)

// long-func-split: gcMain에서 뗀 판정 단계 — 읽기·쓰기 없이 입력 → 결정
func TestGcStepDecisions(t *testing.T) {
	wl := "worktree /r\nHEAD aaa\nbranch refs/heads/main\n\nworktree /r.wt/x\nHEAD bbb\nbranch refs/heads/x\nlocked\n\nworktree /r.wt/y\nHEAD ccc\ndetached\n\nworktree /b\nbare\n"
	want := []gcRec{{"/r", "aaa", "refs/heads/main", "-"}, {"/r.wt/x", "bbb", "refs/heads/x", "x"}, {"/r.wt/y", "ccc", "-", "-"}, {"/b", "-", "-", "x"}}
	if g := gcRecs(wl); !reflect.DeepEqual(g, want) {
		t.Errorf("gcRecs = %v", g)
	}
	if g := gcRecs("HEAD zzz\n"); len(g) != 0 {
		t.Errorf("worktree 줄 앞 줄은 버린다: %v", g)
	}

	for _, c := range []struct {
		rl     string
		ct     int64
		want   int64
		hasWhy bool
	}{
		{"", 100, 100, false},
		{"refs/heads/x@{200}", 100, 200, false},
		{"refs/heads/x@{50}", 100, 100, false},
		{"refs/heads/x@{abc}", 100, 0, true},
		{"x@{1}\ny@{2}", 100, 0, true},
		{"x@{99999999999999999999}", 100, 0, true},
	} {
		g, why := reflogTime(c.rl, c.ct)
		if g != c.want || (why != "") != c.hasWhy {
			t.Errorf("reflogTime(%q, %d) = %d %q", c.rl, c.ct, g, why)
		}
	}

	res, err := gcKeepPatterns([]string{"keep-*", "[[:alpha:]]x"})
	if err != nil || len(res) != 4 {
		t.Fatalf("gcKeepPatterns = %d %v (무늬마다 둘)", len(res), err)
	}
	if !res[0].MatchString("keep-a/b") || res[0].MatchString("x-keep") || !res[3].MatchString("가x") || res[2].MatchString("가x") {
		t.Error("gcKeepPatterns: C 로캘 꼴·UTF-8 꼴 판정이 틀렸다")
	}
	if _, err := gcKeepPatterns([]string{"ok", "[bad"}); err == nil {
		t.Error("닫히지 않은 [ 는 오류여야")
	}
}
