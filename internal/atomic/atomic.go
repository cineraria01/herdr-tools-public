// Package atomic: 파일을 반쪽 없이 바꿔 넣는다 — 같은 폴더의 임시 파일에 다 쓰고 fsync → rename → 폴더 fsync.
// 끊겨도 옛 판이나 새 판 가운데 하나만 남는다(같은 파일을 함께 쓰는 실행끼리도 반쪽 파일을 읽지 않는다).
// 장부를 읽고 고쳐 쓸 때는 Read·Update — 「없음」은 ENOENT뿐이고, 못 읽은 것(권한·폴더·입출력)은 오류다.
package atomic

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// Steps: 바꿔 넣기의 단계 — 시험이 순서를 기록하거나 실패를 끼워 넣는 자리(제품 코드는 바꾸지 않는다)
type Steps struct {
	SyncFile func(*os.File) error                // 임시 파일 fsync
	Rename   func(oldpath, newpath string) error // 임시 파일 → path
	OpenDir  func(dir string) (*os.File, error)  // 폴더 열기(폴더 fsync용)
	SyncDir  func(*os.File) error                // 폴더 fsync
}

var steps = Steps{(*os.File).Sync, os.Rename, os.Open, (*os.File).Sync}

// SwapSteps: 시험 전용 — 단계를 s로 바꾸고 옛 것을 돌려준다(되돌릴 때 그것을 다시 넣는다). 동시에 부르지 않는다
func SwapSteps(s Steps) Steps { old := steps; steps = s; return old }

// CommitError: rename은 됐는데 그 뒤(폴더 열기·폴더 fsync)가 실패했다 — 새 판이 이미 보인다.
// 부르는 쪽은 「안 바뀜」으로 읽지 않는다. 재시도는 SyncDir로 내구성을 마친 뒤 성공으로 친다
type CommitError struct {
	Path string
	Err  error
}

func (e *CommitError) Error() string {
	return "바꿔 넣은 뒤 동기화 " + e.Path + ": " + e.Err.Error()
}

func (e *CommitError) Unwrap() error { return e.Err }

// Read: (내용, 있음, 오류) — ENOENT만 있음=false·오류 nil. 그 밖(EACCES·EISDIR·EIO)은 오류(=모름)
func Read(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return b, true, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("읽기 %s: %w", path, err)
}

// Update: 옛 내용을 Read로 읽어 f(old, exists)로 새 내용을 정해 WriteFile로 바꿔 넣는다(잠금은 부르는 쪽).
// 읽기 오류면 f를 부르지 않고 그 오류 — 파일은 그대로. f가 오류면 쓰지 않고 그 오류
func Update(path string, perm os.FileMode, f func(old []byte, exists bool) ([]byte, error)) error {
	old, ok, err := Read(path)
	if err != nil {
		return err
	}
	b, err := f(old, ok)
	if err != nil {
		return err
	}
	return WriteFile(path, b, perm)
}

// SyncDir: 폴더 fsync만 — CommitError 뒤 재시도가 내구성을 마치는 자리
func SyncDir(dir string) error {
	d, err := steps.OpenDir(dir)
	if err != nil {
		return fmt.Errorf("폴더 열기 %s: %w", dir, err)
	}
	defer d.Close()
	if err := steps.SyncDir(d); err != nil {
		return fmt.Errorf("폴더 fsync %s: %w", dir, err)
	}
	return nil
}

// WriteFile: path를 data로 바꿔 넣는다. 새 파일의 권한은 perm에서 umask를 뺀 것(셸 `>`·os.WriteFile로 새로 만들 때와 같다),
// 이미 있는 파일은 그 권한 그대로 — 임시 파일도 처음부터 그 권한으로 만든다(재감사 fill R-11: 0600 장부가 다음 실행의 umask 022로 0644가 되지 않게)
func WriteFile(path string, data []byte, perm os.FileMode) error {
	st, err := os.Stat(path)
	switch {
	case err == nil && st.Mode().IsRegular():
		return write(path, data, st.Mode().Perm(), true)
	case err != nil && !errors.Is(err, os.ErrNotExist): // 기존 권한을 모르면 느슨한 권한으로 바꿔 넣지 않는다
		return fmt.Errorf("기존 파일 권한 읽기 %s: %w", path, err)
	}
	return write(path, data, perm, false)
}

// WriteFileMode: WriteFile과 같되 권한을 umask와 상관없이 perm 그대로(기존 0666 장부도 0600으로 좁힌다)
func WriteFileMode(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, true)
}

// write: 임시 파일 쓰기 → 파일 fsync → rename → 폴더 fsync. rename 전 실패는 옛 판 그대로·임시 파일 지움,
// rename 뒤 실패는 *CommitError
func write(path string, data []byte, perm os.FileMode, exact bool) (err error) {
	dir := filepath.Dir(path)
	var f *os.File
	var tmp string
	for range 100 {
		tmp = filepath.Join(dir, "."+filepath.Base(path)+".tmp"+strconv.FormatUint(rand.Uint64(), 36))
		// O_EXCL·perm: 커널이 umask를 적용한다(CreateTemp의 0600 + Chmod는 umask를 덮었다)
		if f, err = os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm); !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("임시 파일 만들기: %w", err)
	}
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmp) //nolint:errcheck // 옛 판은 그대로다 — 임시 파일만 치운다
		}
	}()
	if _, err = f.Write(data); err == nil && exact {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = steps.SyncFile(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("쓰기 %s: %w", tmp, err)
	}
	if err = steps.Rename(tmp, path); err != nil {
		return fmt.Errorf("바꿔 넣기 %s: %w", path, err)
	}
	renamed = true
	if err = SyncDir(dir); err != nil {
		return &CommitError{Path: path, Err: err}
	}
	return nil
}
