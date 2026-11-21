package version

import "testing"

// 릴리스 빌드는 -ldflags -X …version.Commit=<12자>로 넣는다 — --version이 그 커밋을 보여야 운영에서 판을 확인한다
func TestString(t *testing.T) {
	defer func(c string) { Commit = c }(Commit)
	Commit = ""
	if g := String("hc-lane"); g != "hc-lane (Go)" {
		t.Fatal(g)
	}
	Commit = "0123456789ab"
	if g := String("hc-lane"); g != "hc-lane (Go 0123456789ab)" {
		t.Fatal(g)
	}
}
