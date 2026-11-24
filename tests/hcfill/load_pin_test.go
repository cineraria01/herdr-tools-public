package hcfill_test

import (
	"runtime"
	"testing"
)

// 작업 중 발견해 함께 고침(2026-11-24 contract-subtests): 가짜 hc exec는 명령을 이 기계에서 그대로 돌려, hc-fill의 부하 조각
// (cmd/hc-fill loadSnip — macOS sysctl vm.loadavg)이 실제 부하를 읽었다. 시험을 돌리는 기계의 코어당 부하가 3을 넘으면 「모두 바쁨」으로
// 레인을 안 띄워 f2·g 절 단언이 실패했다(시험 둘을 함께 돌린 기준 실행에서 14개). 틀은 가짜 sysctl(testdata/sysctl)로 부하를 고정한다
func TestKitLoadPinned(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("부하 조각은 리눅스에서 /proc/loadavg를 먼저 읽는다 — 가짜 sysctl은 macOS 길만 고정한다")
	}
	k := newK(t)
	out, rc := k.bash(`hc exec --on box-1 -- sh -c 'sysctl -n vm.loadavg'`)
	if rc != 0 || out != "{ 0.10 0.10 0.10 }" {
		t.Errorf("틀 안의 부하는 고정값이어야: rc=%d %q", rc, out)
	}
}
