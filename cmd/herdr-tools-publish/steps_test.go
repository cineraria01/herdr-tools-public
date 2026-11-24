package main

import (
	"reflect"
	"testing"
)

// 읽기·쓰기 없는 판정 단계의 본보기 단위 시험 — 계약 시험(가짜 저장소·가짜 공개 원격)보다 훨씬 빠르다

func TestSplitAuthor(t *testing.T) {
	for _, c := range []struct{ in, name, mail string }{
		{"Bot <bot@example.com>", "Bot", "bot@example.com"},
		{"A <b> <c@d>", "A <b>", "c@d"}, // 마지막 「 <」에서 가른다
		{"이름 <>", "이름", ""},
	} {
		if n, m, ok := splitAuthor(c.in); !ok || n != c.name || m != c.mail {
			t.Errorf("splitAuthor(%q) = (%q,%q,%v), 기대 (%q,%q,참)", c.in, n, m, ok, c.name, c.mail)
		}
	}
	for _, in := range []string{"Bot", "Bot<b@c>", "Bot <b@c", "<b@c>"} {
		if _, _, ok := splitAuthor(in); ok {
			t.Errorf("splitAuthor(%q): 꼴이 틀렸는데 받음", in)
		}
	}
}

func TestSnapLabel(t *testing.T) {
	s := &snap{bdir: "/T/b", namesFile: "/T/names",
		names:  []string{"a/x.go", "b/y.go", "top", "a"},
		bpaths: map[string][]string{"s1": {"a/x.go", "c/z.go"}, "s2": {"b/y.go"}}}
	in := []string{
		"/T/b/s1:3: 비밀 꼴 긴 토큰", // 블롭 → 그 블롭의 경로들
		"/T/b/s2:0: 글이 아닌 블롭",  // 「:0:」 그대로
		"/T/names:2: 비밀 꼴 X",   // 경로 줄 → 경로
		"/T/names:3: 비밀 꼴 X",   // 폴더 없는 경로
		"/T/names:9: 비밀 꼴 X",   // 범위 밖 줄 → 빈 경로
		"기타 줄",                 // 그대로
		"/T/b/s1:3: 비밀 꼴 긴 토큰", // 겹침은 한 번
	}
	want := []string{
		":0: 비밀 꼴 X (경로)",
		"a/x.go:3: 비밀 꼴 긴 토큰",
		"b/y.go:0: 글이 아닌 블롭",
		"b/y.go:0: 비밀 꼴 X (경로)",
		"c/z.go:3: 비밀 꼴 긴 토큰",
		"top:0: 비밀 꼴 X (경로)",
		"기타 줄",
	}
	if got := s.label(in, false); !reflect.DeepEqual(got, want) {
		t.Errorf("label(가림 없음)\n 받음 %q\n 기대 %q", got, want)
	}
	// 가림: 경로 줄은 폴더와 몇째 파일만(이름이 곧 값일 수 있다), 블롭 줄은 그대로
	wantM := []string{
		"./(이름 가림, 3번째 파일):0: 비밀 꼴 X (경로)",
		"./(이름 가림, 9번째 파일):0: 비밀 꼴 X (경로)",
		"b/(이름 가림, 2번째 파일):0: 비밀 꼴 X (경로)",
	}
	if got := s.label(in[2:5], true); !reflect.DeepEqual(got, wantM) {
		t.Errorf("label(가림)\n 받음 %q\n 기대 %q", got, wantM)
	}
}
