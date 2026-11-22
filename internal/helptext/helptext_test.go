package helptext

import (
	"strings"
	"testing"
	"testing/fstest"
)

// 첫 조각 먼저, 나머지는 파일 이름순(정렬이 깨지면 도움말 차례가 등록·파일 추가 순서에 따라 흔들린다)
func TestJoinOrder(t *testing.T) {
	m := fstest.MapFS{
		"h/wait.txt":   {Data: []byte("W\n")},
		"h/launch.txt": {Data: []byte("L\n")},
		"h/clean.txt":  {Data: []byte("C\n")},
		"h/land.txt":   {Data: []byte("D\n")},
		"h/note.md":    {Data: []byte("X\n")},
	}
	if got := Join(m, "h", "launch"); got != "L\nC\nD\nW\n" {
		t.Fatalf("first 먼저: %q", got)
	}
	if got := Join(m, "h", ""); got != "C\nD\nL\nW\n" {
		t.Fatalf("이름순: %q", got)
	}
	if got := strings.Join(Names(m, "h"), ","); got != "clean,land,launch,wait" {
		t.Fatalf("Names: %q", got)
	}
}
