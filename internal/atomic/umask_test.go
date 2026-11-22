//go:build !windows

package atomic

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// umask는 프로세스 전체라 하위 프로세스(이 시험 바이너리를 다시 부름)에서 바꾼다
func TestUmaskChild(t *testing.T) {
	m := os.Getenv("ATOMIC_UMASK_CHILD")
	if m == "" {
		return // 하위 프로세스로 불릴 때만 일한다
	}
	u, _ := strconv.ParseUint(m, 8, 32)
	syscall.Umask(int(u))
	d := os.Getenv("ATOMIC_UMASK_DIR")
	if err := WriteFile(filepath.Join(d, "plain"), []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileMode(filepath.Join(d, "exact"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(d, "kept"), []byte("new"), 0o666); err != nil { // 시험이 0600으로 미리 둔 파일
		t.Fatal(err)
	}
}

// 재감사 fill R-11: 기존 파일을 바꿔 넣을 때는 umask가 더 느슨해도 기존 권한(0600) 그대로 — 내용이 다른 사용자에게 열리지 않게
func TestWriteFileKeepsExistingMode(t *testing.T) {
	d := t.TempDir()
	kept := filepath.Join(d, "kept")
	if err := os.WriteFile(kept, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(kept, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestUmaskChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), "ATOMIC_UMASK_CHILD=022", "ATOMIC_UMASK_DIR="+d)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("하위 프로세스: %v\n%s", err, out)
	}
	st, err := os.Stat(kept)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(kept)
	if st.Mode().Perm() != 0o600 || string(b) != "new" {
		t.Errorf("기존 0600 파일: %v %q, 바람 0600 \"new\"", st.Mode().Perm(), b)
	}
}

// WriteFile은 umask를 따르고(셸 `>`와 같게 0666 → 022면 0644, 077이면 0600), WriteFileMode는 perm 그대로
func TestWriteFileUmask(t *testing.T) {
	for _, c := range []struct {
		umask string
		plain os.FileMode
	}{{"022", 0o644}, {"077", 0o600}} {
		d := t.TempDir()
		cmd := exec.Command(os.Args[0], "-test.run=^TestUmaskChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), "ATOMIC_UMASK_CHILD="+c.umask, "ATOMIC_UMASK_DIR="+d)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("umask %s 하위 프로세스: %v\n%s", c.umask, err, out)
		}
		for name, want := range map[string]os.FileMode{"plain": c.plain, "exact": 0o640} {
			st, err := os.Stat(filepath.Join(d, name))
			if err != nil {
				t.Fatal(err)
			}
			if st.Mode().Perm() != want {
				t.Errorf("umask %s %s: %v, 바람 %v", c.umask, name, st.Mode().Perm(), want)
			}
		}
	}
}
