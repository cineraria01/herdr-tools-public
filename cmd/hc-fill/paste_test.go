package main

import (
	"os"
	"testing"
)

// R-25: 백틱·따옴표로 묶은 공백·한글 파일 경로는 한 항목으로 읽어 붙인다(묶지 않은 공백 경로는 앞 낱말만이라 못 찾는다). 나온 차례대로
func TestLcPasteQuotedSpace(t *testing.T) {
	old, oc, od := HOME, confLines, DATA
	defer func() { HOME, confLines, DATA = old, oc, od }()
	HOME, confLines, DATA = t.TempDir(), nil, ""
	if err := os.MkdirAll(HOME+"/.claude-tasks/r", 0o755); err != nil {
		t.Fatal(err)
	}
	for n, s := range map[string]string{"감사 자료.md": "감사 원문\n", "둘 째.md": "둘째 원문\n", "a.md": "에이\n"} {
		if err := os.WriteFile(HOME+"/.claude-tasks/r/"+n, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	body := "먼저 ~/.claude-tasks/r/a.md 를 보고 `~/.claude-tasks/r/감사 자료.md`를 읽고 \"" + HOME + "/.claude-tasks/r/둘 째.md\"에서 고친다. " +
		"묶지 않은 ~/.claude-tasks/r/감사 자료.md 는 다시 붙이지 않는다"
	got, ok := lcPasteHome(body, "-", "ln")
	want := "\n## (붙임) 지휘 기계 파일 원문 — ~/.claude-tasks/r/a.md\n에이\n" +
		"\n## (붙임) 지휘 기계 파일 원문 — ~/.claude-tasks/r/감사 자료.md\n감사 원문\n" +
		"\n## (붙임) 지휘 기계 파일 원문 — " + HOME + "/.claude-tasks/r/둘 째.md\n둘째 원문\n"
	if !ok || got != want {
		t.Errorf("붙임\n got %q (ok %v)\nwant %q", got, ok, want)
	}
}
