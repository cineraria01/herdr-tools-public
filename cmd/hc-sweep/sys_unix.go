//go:build !windows

package main

// 유닉스 시스템 호출(B24 — Windows 교차 빌드가 되게 나눔). 실행 동작 보장은 macOS만

import (
	"fmt"
	"os"
	"syscall"
)

// executable: 실행할 수 있는 파일인가(X_OK)
func executable(p string) bool { return syscall.Access(p, 1) == nil }

// blocks: 디스크에서 차지하는 바이트(st_blocks × 512)
func blocks(st os.FileInfo) int64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return s.Blocks * 512
	}
	return st.Size()
}

// mine: 이 사용자 것인가(모르면 false — 지우지 않는 쪽)
func mine(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Getuid())
}

// diskUsage: shutil.disk_usage처럼 여유(Bavail)와 사용(Blocks-Bfree) 바이트
func diskUsage(path string) (free, used float64, err error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, 0, fmt.Errorf("디스크 여유 읽기: %w", err)
	}
	return float64(s.Bavail) * float64(s.Bsize), float64(s.Blocks-s.Bfree) * float64(s.Bsize), nil
}

func alive(p int) bool { return p > 0 && syscall.Kill(p, 0) == nil }

func term(p int) {
	if p > 0 {
		syscall.Kill(p, syscall.SIGTERM)
	}
}

func hardKill(p int) {
	if p > 0 {
		syscall.Kill(p, syscall.SIGKILL)
	}
}
