//go:build !windows

package atomic

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

// 계약 시험(재감사 2바퀴 ②-D) — 「없음」은 ENOENT뿐, 읽기 실패는 장부를 안 바꾼다, rename 앞·뒤 오류를 가른다

func umaskNow() os.FileMode { u := syscall.Umask(0); syscall.Umask(u); return os.FileMode(u) }

func readDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root는 0o000도 읽는다")
	}
}

func TestReadContract(t *testing.T) {
	d := t.TempDir()
	if b, ok, err := Read(filepath.Join(d, "없음")); b != nil || ok || err != nil {
		t.Errorf("ENOENT: (%q,%v,%v), 바람 (nil,false,nil)", b, ok, err)
	}
	p := filepath.Join(d, "있음")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, ok, err := Read(p); string(b) != "x" || !ok || err != nil {
		t.Errorf("있음: (%q,%v,%v)", b, ok, err)
	}
	if _, ok, err := Read(d); err == nil || ok {
		t.Errorf("폴더: 오류여야 (ok=%v err=%v)", ok, err)
	}
	readDenied(t)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0o600)
	if _, ok, err := Read(p); err == nil || ok {
		t.Errorf("EACCES: 오류여야 (ok=%v err=%v)", ok, err)
	}
}

func TestUpdateContract(t *testing.T) {
	boom := errors.New("f 실패")
	cases := []struct {
		name  string
		setup func(t *testing.T, p string) // 고치기 전 상태
		f     func(old []byte, exists bool) ([]byte, error)
		check func(t *testing.T, p string, called bool, err error)
	}{
		{"파일 없음", func(*testing.T, string) {},
			func(old []byte, exists bool) ([]byte, error) {
				if old != nil || exists {
					return nil, errors.New("f(nil,false)이어야")
				}
				return []byte("new"), nil
			},
			func(t *testing.T, p string, called bool, err error) {
				st, serr := os.Stat(p)
				b, _ := os.ReadFile(p)
				if err != nil || !called || serr != nil || string(b) != "new" || st.Mode().Perm() != 0o666&^umaskNow() {
					t.Errorf("err=%v called=%v %v %q", err, called, st.Mode(), b)
				}
			}},
		{"읽기 금지 0o000", func(t *testing.T, p string) {
			readDenied(t)
			if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0); err != nil {
				t.Fatal(err)
			}
		}, nil,
			func(t *testing.T, p string, called bool, err error) {
				st, _ := os.Stat(p)
				os.Chmod(p, 0o600)
				b, _ := os.ReadFile(p)
				if err == nil || called || st.Mode().Perm() != 0 || string(b) != "old" {
					t.Errorf("err=%v called=%v %v %q — 그대로여야", err, called, st.Mode(), b)
				}
			}},
		{"path가 폴더", func(t *testing.T, p string) {
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
		}, nil,
			func(t *testing.T, p string, called bool, err error) {
				st, serr := os.Stat(p)
				if err == nil || called || serr != nil || !st.IsDir() {
					t.Errorf("err=%v called=%v — 폴더 그대로여야", err, called)
				}
			}},
		{"f 오류", func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, func([]byte, bool) ([]byte, error) { return []byte("new"), boom },
			func(t *testing.T, p string, called bool, err error) {
				b, _ := os.ReadFile(p)
				if !errors.Is(err, boom) || !called || string(b) != "old" {
					t.Errorf("err=%v called=%v %q — 쓰지 않고 그 오류여야", err, called, b)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			p := filepath.Join(d, "ledger")
			c.setup(t, p)
			called := false
			err := Update(p, 0o666, func(old []byte, exists bool) ([]byte, error) {
				called = true
				if c.f == nil {
					return []byte("new"), nil
				}
				return c.f(old, exists)
			})
			c.check(t, p, called, err)
			if es, _ := os.ReadDir(d); len(es) > 1 {
				t.Errorf("임시 파일이 남았다: %v", es)
			}
		})
	}
}

// 단계 순서와 각 단계 실패: 파일 fsync → rename → 폴더 열기 → 폴더 fsync (hooks/R-M209 — Sync를 빼면 순서가 달라져 실패한다).
// rename 전 실패는 옛 판·임시 파일 없음·CommitError 아님, rename 뒤 실패는 *CommitError·새 판 (hooks/R-M202)
func TestWriteStepsContract(t *testing.T) {
	all := []string{"syncFile", "rename", "openDir", "syncDir"}
	for fail := -1; fail < len(all); fail++ {
		name := "모두 성공"
		if fail >= 0 {
			name = all[fail] + " 실패"
		}
		t.Run(name, func(t *testing.T) {
			var got []string
			boom := errors.New("끼워 넣은 실패")
			step := func(s string) error {
				got = append(got, s)
				if fail >= 0 && s == all[fail] {
					return boom
				}
				return nil
			}
			old := SwapSteps(Steps{
				SyncFile: func(f *os.File) error {
					if err := step("syncFile"); err != nil {
						return err
					}
					return f.Sync()
				},
				Rename: func(a, b string) error {
					if err := step("rename"); err != nil {
						return err
					}
					return os.Rename(a, b)
				},
				OpenDir: func(p string) (*os.File, error) {
					if err := step("openDir"); err != nil {
						return nil, err
					}
					return os.Open(p)
				},
				SyncDir: func(f *os.File) error {
					if err := step("syncDir"); err != nil {
						return err
					}
					return f.Sync()
				},
			})
			defer SwapSteps(old)
			for _, w := range []func(string, []byte, os.FileMode) error{WriteFile, WriteFileMode} {
				got = nil
				d := t.TempDir()
				p := filepath.Join(d, "ledger")
				if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
				err := w(p, []byte("new"), 0o600)
				want := all
				if fail >= 0 {
					want = all[:fail+1]
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("단계 %v, 바람 %v", got, want)
				}
				b, _ := os.ReadFile(p)
				var ce *CommitError
				switch {
				case fail < 0:
					if err != nil || string(b) != "new" {
						t.Errorf("성공이어야: %v %q", err, b)
					}
				case fail <= 1: // rename 전
					if !errors.Is(err, boom) || errors.As(err, &ce) || string(b) != "old" {
						t.Errorf("rename 전 실패: err=%v commit=%v %q", err, errors.As(err, &ce), b)
					}
				default: // rename 뒤
					if !errors.Is(err, boom) || !errors.As(err, &ce) || ce.Path != p || string(b) != "new" {
						t.Errorf("rename 뒤 실패: err=%v commit=%v %q", err, errors.As(err, &ce), b)
					}
				}
				if es, _ := os.ReadDir(d); len(es) != 1 {
					t.Errorf("임시 파일이 남았다: %v", es)
				}
			}
		})
	}
}

func TestWriteFileModeNarrowsExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "coord")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileMode(p, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("기존 0666을 0600으로 좁혀야: %v", st.Mode())
	}
}

func TestSyncDir(t *testing.T) {
	if err := SyncDir(t.TempDir()); err != nil {
		t.Errorf("있는 폴더: %v", err)
	}
	if err := SyncDir(filepath.Join(t.TempDir(), "없음")); err == nil {
		t.Error("없는 폴더는 오류여야")
	}
}
