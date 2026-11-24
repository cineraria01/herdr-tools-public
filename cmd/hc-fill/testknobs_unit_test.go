package main

import "os"

// 단위 시험은 태그 없이 돈다 — 시험 빌드(testknobs_herdrtest.go)와 같은 중단 주입을 이 꾸러미 시험에만
func init() {
	testCrashAt = func() string { return os.Getenv("HC_FILL_CRASH_AT") }
}
