package hclane_test

import (
	"testing"

	"herdr-tools/internal/clitest"
)

// test-env-allowlist ②: 레인 터미널·병합기 값을 다 넣어도 두 틀(newK·stlNewK)의 자식 환경에는 하나도 안 샌다 — 옛 지우기 목록은
// LANE_*·HERDR_TOOLS_CONFIG·HERDR_MERGER·GIT_CONFIG_* 등을 빠뜨렸다
func TestKitEnvNoLeak(t *testing.T) {
	laneBin(t) // 독(GIT_DIR 등)을 넣기 전에 짓는다 — 시험 프로세스의 go build는 바깥 환경을 쓴다
	clitest.Poison(t)
	for name, k := range map[string]*K{"newK": newK(t), "stlNewK": stlNewK(t)} {
		if l := clitest.Leaked(k.env); len(l) > 0 {
			t.Errorf("%s: 샌 레인 값: %v", name, l)
		}
		if k.env["HOME"] != k.T {
			t.Errorf("%s: 임시 HOME이어야: %q", name, k.env["HOME"])
		}
	}
}
