package merger_test

import (
	"os"
	"testing"

	"herdr-tools/internal/clitest"
)

// test-env-allowlist ②: 레인 터미널·병합기 값을 다 넣어도 틀의 자식 환경에는 하나도 안 새고(옛 지우기 목록은 LANE_*·HERDR_TOOLS_REPO·
// HERDR_PROC_DIR·GIT_CONFIG_KEY_0 등을 빠뜨렸다), 시험 프로세스 환경(Isolate — clitest.Git 등이 물려준다)에도 GO* 말고는 안 남는다
func TestKitEnvNoLeak(t *testing.T) {
	clitest.BinDir(t, "hc-lane", "hc-merger", "herdr-tools-config") // 독(GIT_CONFIG_* 등)을 넣기 전에 짓는다
	clitest.Poison(t)
	k := newK(t)
	if l := clitest.Leaked(k.env); len(l) > 0 {
		t.Errorf("자식 환경에 샌 레인 값: %v", l)
	}
	for _, key := range clitest.LaneEnv {
		if v, ok := os.LookupEnv(key); ok && key != "GOFLAGS" {
			t.Errorf("시험 프로세스에 남은 레인 값: %s=%s", key, v)
		}
	}
	if k.env["HOME"] != k.T {
		t.Errorf("임시 HOME이어야: %q", k.env["HOME"])
	}
}
