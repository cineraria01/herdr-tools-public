package toolcfg

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(d+"/config", []byte("# c\n a.b  x  y \r\nc.d\nBad.Key z\n"), 0o644)
	if v, ok, err := Get(d, "a.b"); err != nil || !ok || v != "x  y" {
		t.Errorf("a.b = %q %v %v", v, ok, err)
	}
	if v, ok, _ := Get(d, "c.d"); !ok || v != "" {
		t.Errorf("키 혼자 = %q %v", v, ok)
	}
	if _, ok, _ := Get(d, "e.f"); ok {
		t.Errorf("없는 키가 있다고 함")
	}
	os.WriteFile(d+"/config", []byte("a.b 1\nc.d 2\na.b 3\n"), 0o644)
	var dup *ErrDup
	if _, _, err := Get(d, "c.d"); !errors.As(err, &dup) {
		t.Errorf("같은 키 두 번이 오류가 아님: %v", err)
	}
}

func TestLintAt(t *testing.T) {
	c, base := t.TempDir(), t.TempDir()
	os.WriteFile(c+"/private-patterns.txt", []byte("# 주석\nsecret-[0-9]+\n"), 0o644)
	os.MkdirAll(filepath.Join(base, "d"), 0o755)
	os.WriteFile(filepath.Join(base, "d/a.txt"), []byte("ok\nSECRET-1 here\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := LintAt(base, c, false, []string{"d/a.txt"}, &out, &errb); rc != 1 || out.String() != "d/a.txt:2: secret-[0-9]+\n" {
		t.Errorf("LintAt rc=%d out=%q err=%q", rc, out.String(), errb.String())
	}
	out.Reset()
	if rc := LintAt(base, c, false, []string{"d/없음"}, &out, &errb); rc != 2 {
		t.Errorf("못 읽는 파일은 2: %d", rc)
	}
	if !HasPatterns(c+"/private-patterns.txt") || HasPatterns(c+"/없음") {
		t.Errorf("HasPatterns")
	}
}
