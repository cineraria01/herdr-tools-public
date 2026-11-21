//go:build !hookstest

package main

// 시험 손잡이(-tags hookstest 빌드에만 — knobs_test_build.go). 보통 빌드는 아무것도 바꾸지 않는다:
// 환경 변수로 git·시계를 바꿔 가드를 비켜 가지 못하게.

func testGit() string          { return "" }
func testNowOffset() int64     { return 0 }
func testSecondsOffset() int64 { return 0 }
