package util_test

import (
	"testing"

	"herdr-tools/internal/clitest"
)

// test-env-allowlist ②: 레인 터미널·병합기 값을 다 넣어도 이 틀의 자식 환경에는 하나도 안 샌다(옛 지우기 목록은 HC_SCOPE·LANE_*·
// HERDR_TOOLS_CONFIG 등을 빠뜨렸다), 임시 HOME
func TestKitEnvNoLeak(t *testing.T) {
	clitest.Poison(t)
	T := tmpdir(t)
	e := isolated(T)
	if l := clitest.Leaked(e); len(l) > 0 {
		t.Errorf("샌 레인 값: %v", l)
	}
	if e["HOME"] != T {
		t.Errorf("임시 HOME이어야: %q", e["HOME"])
	}
}
