package clitest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AltSpelling: dir과 같은 폴더를 다른 글자로 가리키는 경로(gofix5 — 「그 폴더를 쓰는 터미널」을 글자로만 견주던 결함 시험).
// 끝 이름의 대문자 꼴(nfd면 끝 이름의 「한」을 NFD로)이 이 파일 시스템에서 같은 폴더면 그것(macOS 기본 — 대소문자·정규화를 가리지
// 않는다), 가리는 파일 시스템이면 새 임시 폴더의 심볼릭 링크
func AltSpelling(t *testing.T, dir string, nfd bool) string {
	t.Helper()
	b := filepath.Base(dir)
	alt := strings.ToUpper(b)
	if nfd {
		alt = strings.ReplaceAll(b, "한", "한")
	}
	p := filepath.Join(filepath.Dir(dir), alt)
	if p == dir {
		t.Fatalf("AltSpelling: %s의 다른 철자가 없다", dir)
	}
	a, e1 := os.Stat(dir)
	c, e2 := os.Stat(p)
	if e1 == nil && e2 == nil && os.SameFile(a, c) {
		return p
	}
	p = filepath.Join(t.TempDir(), "alt")
	if err := os.Symlink(dir, p); err != nil {
		t.Fatal(err)
	}
	return p
}
