// Package atomic: 파일을 반쪽 없이 바꿔 넣는다 — 같은 폴더의 임시 파일에 다 쓰고 fsync → rename → 폴더 fsync.
// 끊겨도 옛 판이나 새 판 가운데 하나만 남는다(같은 파일을 함께 쓰는 실행끼리도 반쪽 파일을 읽지 않는다).
package atomic

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile: path를 data로 바꿔 넣는다. 권한은 perm
func WriteFile(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("임시 파일 만들기: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err == nil {
		if err = f.Chmod(perm); err == nil {
			err = f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("쓰기 %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, path); err != nil {
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
