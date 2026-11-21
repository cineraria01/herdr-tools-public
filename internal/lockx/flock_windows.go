//go:build windows

package lockx

import (
	"errors"
	"os"
)

func flock(f *os.File, wait bool) (bool, error) {
	return false, errors.New("flock: Windows에서는 잠그지 않는다(레인을 돌리지 않음)")
}
