// Package lockx: 파일 잠금(flock). Go가 여는 fd는 O_CLOEXEC라 자식에 물리지 않고, 쥔 프로세스가 죽으면 커널이 바로 푼다.
// bash 판 도구가 쓰는 같은 잠금 파일·같은 뜻(flock 배타)이라 두 판이 함께 돌지 못한다. 병합기 Go 판에서 끌어올렸다.
package lockx

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// TryLock: 파일을 열어(없으면 만듦) 비켜 잠근다. busy = 남이 쥠. 잠금은 돌려준 파일이 열려 있는 동안 — 프로세스가 끝날 때까지
// 쥐려면 그 *os.File을 살아 있는 변수에 둔다(GC가 닫으면 풀린다)
func TryLock(path string) (f *os.File, busy bool, err error) {
	return lock(path, syscall.LOCK_EX|syscall.LOCK_NB)
}

// Lock: TryLock과 같되 풀릴 때까지 기다린다
func Lock(path string) (*os.File, error) {
	f, _, err := lock(path, syscall.LOCK_EX)
	return f, err
}

func lock(path string, how int) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return nil, false, fmt.Errorf("열기: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("flock: %w", err)
	}
	return f, false, nil
}
