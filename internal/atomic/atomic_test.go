package atomic

import (
	"errors"
	"io"
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
	if es, _ := os.ReadDir(d); len(es) != 1 {
		t.Fatalf("임시 파일이 남으면 안 된다: %v", es)
	}
}

func TestWriteFileFailsWithoutDir(t *testing.T) {
	if err := WriteFile(filepath.Join(t.TempDir(), "없음", "x"), []byte("x"), 0o644); err == nil {
		t.Fatal("폴더가 없으면 실패해야")
	}
}

// 바꿔 넣기(rename)여야 한다: 바꾸기 전에 연 fd는 옛 내용, 새로 열면 새 내용(제자리 쓰기는 옛 fd에서도 새 내용이 보인다)
func TestWriteFileSwapsInode(t *testing.T) {
	for _, w := range []func(string, []byte, os.FileMode) error{WriteFile, WriteFileMode} {
		p := filepath.Join(t.TempDir(), "state")
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		f0, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := w(p, []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
		b0, err := io.ReadAll(f0)
		f0.Close()
		if err != nil || string(b0) != "old" {
			t.Fatalf("먼저 연 fd는 옛 내용이어야: %q %v", b0, err)
		}
		if b, _ := os.ReadFile(p); string(b) != "new" {
			t.Fatalf("새로 열면 새 내용: %q", b)
		}
	}
}

// rename 직전 실패: 옛 파일 그대로, 임시 파일 없음
func TestWriteFileRenameFailKeepsOld(t *testing.T) {
	defer func(r func(string, string) error) { rename = r }(rename)
	rename = func(string, string) error { return errors.New("끼워 넣은 실패") }
	d := t.TempDir()
	p := filepath.Join(d, "state")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(p, []byte("new"), 0o644); err == nil {
		t.Fatal("rename이 실패하면 오류여야")
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Fatalf("옛 파일 그대로여야: %q", b)
	}
	if es, _ := os.ReadDir(d); len(es) != 1 {
		t.Fatalf("임시 파일이 남으면 안 된다: %v", es)
	}
}
