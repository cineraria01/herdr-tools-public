//go:build hookstest

package main

// 시험 빌드(go build -tags hookstest)만: 셸 판 시험이 훅 파일을 sed로 고쳐 하던 것(GIT=가짜 git · DATE=61초 뒤 시계 ·
// SECONDS=5)을 환경 변수로. 보통 빌드에는 없다(knobs.go).

import (
	"os"
	"strconv"
)

func testGit() string { return os.Getenv("HERDR_HOOKS_TEST_GIT") }

func testNowOffset() int64 {
	n, _ := strconv.ParseInt(os.Getenv("HERDR_HOOKS_TEST_NOW_OFFSET"), 10, 64)
	return n
}

// testSecondsOffset: 판정이 이만큼 늦게 시작한 것처럼(셸 판 SECONDS=5)
func testSecondsOffset() int64 {
	n, _ := strconv.ParseInt(os.Getenv("HERDR_HOOKS_TEST_SECONDS"), 10, 64)
	return -n
}
