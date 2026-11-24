package sandbox

import (
	"os"
	"strings"
	"testing"
)

func TestSandboxed(t *testing.T) { Check(t) }

// 지우는 이름과 남기는 이름
func TestDrop(t *testing.T) {
	for k, want := range map[string]bool{
		"HC_LANE_SPEC_IDLE_LOAD": true, "HC_FILL_DATA": true, "HERDR_TOOLS_CONFIG": true, "LANE_REPO": true, "XDG_STATE_HOME": true,
		"CLAUDE_CONFIG_DIR": true, "GIT_DIR": true, "HERDR_FUTURE_KNOB": true,
		"HERDR_VERIFY_JOBS": false, "HERDR_VERIFY_BOOT_TMPDIR": false, "HC_MERGER_TEST_HELPER": false, "HC_FILL_EXAMPLE": false,
		"HC_FILL_CRASH_AT": false, "HERDR_SYNC": false, Marker: false, childEnv: false, "PATH": false, "GOCACHE": false, "TMPDIR": false,
	} {
		if Drop(k) != want {
			t.Errorf("Drop(%s) = %v, 바람 %v", k, !want, want)
		}
	}
}

// go 캐시는 격리 전 자리 그대로(임시 HOME 아래가 아니다 — Bin의 go build가 빈 캐시로 다시 짓지 않게)
func TestGoCacheNotInSandbox(t *testing.T) {
	for _, k := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if v := os.Getenv(k); v != "" && strings.HasPrefix(v, Home()) {
			t.Errorf("%s=%s 가 임시 HOME 아래다", k, v)
		}
	}
	if b, err := os.ReadFile(Home() + "/.gitconfig"); err != nil || !strings.Contains(string(b), "name = herdr-test") {
		t.Errorf("임시 HOME의 git 이름: %v %q", err, b)
	}
}
