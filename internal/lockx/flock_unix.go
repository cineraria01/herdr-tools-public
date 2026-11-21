//go:build !windows

package lockx

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func flock(f *os.File, wait bool) (busy bool, err error) {
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	for {
		err = syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("flock: %w", err)
	}
	return false, nil
}
