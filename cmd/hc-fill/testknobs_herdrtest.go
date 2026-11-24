//go:build herdrtest

package main

import "os"

// 시험 빌드만(testknobs.go): HC_FILL_CRASH_AT=<지점> · HC_FILL_TEST_QSET="<이름> <상태>" + HC_FILL_TEST_QSET_CMD=<셸 명령>
func init() {
	testCrashAt = func() string { return os.Getenv("HC_FILL_CRASH_AT") }
	testQsetCmd = func(name, state string) string {
		if os.Getenv("HC_FILL_TEST_QSET") != name+" "+state {
			return ""
		}
		return os.Getenv("HC_FILL_TEST_QSET_CMD")
	}
}
