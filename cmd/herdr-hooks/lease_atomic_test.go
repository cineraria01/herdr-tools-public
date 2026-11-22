//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 재감사 hooks R-M03: own은 임대 파일을 internal/atomic.WriteFile로 바꿔 넣는다(임시 파일 O_EXCL·fsync·닫기·rename·폴더 fsync).
// 옛 os.WriteFile(owner.<pid>)+Rename은 그 이름에 미리 놓인 링크를 따라가 남의 파일을 덮었고, fsync 없이 편집을 허용했다
func TestOwnWritesLeaseAtomically(t *testing.T) {
	ld := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, ld+"/owner."+strconv.Itoa(os.Getpid())); err != nil {
		t.Fatal(err)
	}
	var eb bytes.Buffer
	g := &cg{meDone: true, sid: "s1", mypid: "1", mystart: "x", stderr: &eb}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("own이 막았다: %v %s", r, eb.String())
			}
		}()
		g.own(ld, "a.go")
	}()
	if b, _ := os.ReadFile(victim); string(b) != "keep\n" {
		t.Errorf("미리 놓인 링크를 따라가 남의 파일을 덮었다: %q", b)
	}
	fi, err := os.Lstat(ld + "/owner")
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("owner가 보통 파일이 아니다: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(ld + "/owner"); !strings.HasPrefix(string(b), "s1"+us+"1"+us+"x"+us) || !strings.HasSuffix(string(b), us+"a.go\n") {
		t.Errorf("owner 줄: %q", b)
	}
}
