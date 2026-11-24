package hcfill_test

import (
	"testing"

	"herdr-tools/internal/clitest"
)

// test-env-allowlist ②: 레인 터미널·병합기 값을 다 넣어도 틀의 자식 환경에는 하나도 안 샌다 — 옛 지우기 목록은 HC_LANE_COORD·
// HC_LANE_MACHINE·HC_FILL_DATA·HC_FILL_VERDICTS·HERDR_MERGER 등을 빠뜨렸다. 틀이 스스로 정하는 값(HOME·HERDR_TOOLS_CONFIG 등)은 틀 값이다
func TestKitEnvNoLeak(t *testing.T) {
	clitest.BinDir(t, "hc-fill", "hc-lane", "hc-merger", "herdr-tools-config") // 독(GIT_DIR 등)을 넣기 전에 짓는다
	clitest.Poison(t)
	k := newK(t)
	if l := clitest.Leaked(k.env); len(l) > 0 {
		t.Errorf("샌 레인 값: %v", l)
	}
	if k.env["HOME"] != k.HOME {
		t.Errorf("임시 HOME이어야: %q", k.env["HOME"])
	}
}
