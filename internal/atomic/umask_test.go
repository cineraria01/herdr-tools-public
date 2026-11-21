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
