package atomic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReplacesWhole(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(p, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "new\n" {
		t.Fatalf("새 판이어야: %q %v", b, err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o644 {
		t.Fatalf("권한 0644: %v", st.Mode().Perm())
	}
	if es, _ := os.ReadDir(d); len(es) != 1 {
		t.Fatalf("임시 파일이 남으면 안 된다: %v", es)
	}
}

func TestWriteFileFailsWithoutDir(t *testing.T) {
	if err := WriteFile(filepath.Join(t.TempDir(), "없음", "x"), []byte("x"), 0o644); err == nil {
		t.Fatal("폴더가 없으면 실패해야")
	}
}
