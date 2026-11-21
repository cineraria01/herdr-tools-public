// Package atomic: 파일을 반쪽 없이 바꿔 넣는다 — 같은 폴더의 임시 파일에 다 쓰고 fsync → rename → 폴더 fsync.
// 끊겨도 옛 판이나 새 판 가운데 하나만 남는다(같은 파일을 함께 쓰는 실행끼리도 반쪽 파일을 읽지 않는다).
package atomic

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// rename: 시험이 rename 직전 실패를 끼워 넣는 자리
var rename = os.Rename

// WriteFile: path를 data로 바꿔 넣는다. 권한은 perm에서 umask를 뺀 것(셸 `>`·os.WriteFile로 새로 만들 때와 같다)
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, false)
}

// WriteFileMode: WriteFile과 같되 권한을 umask와 상관없이 perm 그대로(기존 파일 권한을 지켜 바꿔 넣을 때)
func WriteFileMode(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, true)
}

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
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err == nil && exact {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("쓰기 %s: %w", tmp, err)
	}
	if err = rename(tmp, path); err != nil {
		return fmt.Errorf("바꿔 넣기 %s: %w", path, err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("폴더 열기 %s: %w", dir, err)
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("폴더 fsync %s: %w", dir, err)
	}
	return nil
}
