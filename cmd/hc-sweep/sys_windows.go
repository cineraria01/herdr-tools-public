package main

// Windows: 레인을 돌리지 않는다 — 빌드만 된다(B24). 지우는 판정은 모두 「모름」 쪽(mine false·디스크 여유 오류), 신호는 그 프로세스 하나

import (
	"errors"
	"os"
)

func executable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func blocks(st os.FileInfo) int64 { return st.Size() }

func mine(st os.FileInfo) bool { return false }

func diskUsage(path string) (free, used float64, err error) {
	return 0, 0, errors.New("디스크 여유 읽기: Windows는 지원하지 않는다")
}

func alive(p int) bool {
	if p <= 0 {
		return false
	}
	_, err := os.FindProcess(p) // Windows FindProcess는 그 프로세스를 연다 — 없으면 오류
	return err == nil
}

func term(p int) { hardKill(p) }

func hardKill(p int) {
	if pr, err := os.FindProcess(p); p > 0 && err == nil {
		pr.Kill()
	}
}
