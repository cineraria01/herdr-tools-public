//go:build windows

package selfexec

import "errors"

// execve: Windows에는 제자리 exec가 없다 — 빌드만 된다(그 판으로 계속)
func execve(string, []string, []string) error { return errors.ErrUnsupported }
