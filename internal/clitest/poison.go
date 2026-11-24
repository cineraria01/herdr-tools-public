package clitest

import (
	"strings"
	"testing"
)

// LaneEnv: 레인 터미널·병합기·운영 셸에 있을 수 있는 바깥 값 — 시험 틀이 하나도 물려받지 않아야 하는 것(2026-11-24 test-env-allowlist).
// 옛 지우기 목록 여섯 벌의 합에 레인 터미널(LANE_*·HC_TERMINAL_SOCKET·HC_SCOPE)·병합기(HERDR_MERGER·HERDR_LAND_*)·도구 손잡이·
// git 설정 주입을 더했고, 끝의 HERDR_FUTURE_KNOB은 아직 없는 새 이름 자리다(허용 목록이라 이름을 몰라도 막혀야 한다 —
// 시험 손잡이 꼴(…_TEST…·…_CRASH_AT)은 TestTestKnobsListed가 태그 파일에만 두게 하니 그 꼴은 쓰지 않는다)
var LaneEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE", "GIT_PREFIX", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_GLOBAL", "GIT_SSH_COMMAND",
	"GIT_EDITOR", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
	"HC_LANE", "HC_LANE_OWNER", "HC_LANE_OWNER_RUN", "HC_LANE_LEASE", "HC_LANE_PRIORITY", "HC_LANE_MERGER_BIN", "HC_LANE_MACHINE",
	"HC_LANE_JOB", "HC_LANE_COORD", "HC_LANE_FILL", "HC_LANE_BIN", "HC_LANE_SELF", "HC_LANE_MERGER_BATCH", "HC_MERGER_BIN",
	"HC_TERMINAL_ID", "HC_TERMINAL_SOCKET", "HC_SCOPE", "LANE_BASE", "LANE_BRANCH", "LANE_REPO", "LANE_WORKTREE",
	"HERDR_MERGER", "HERDR_LAND_PRIORITY", "HERDR_LAND_PUSH_EXTRA", "HERDR_LAND_PATCHID", "HERDR_LAND_SHA", "HERDR_LAND_VERDICT",
	"HERDR_LAND_AFTER", "HERDR_TOOLS_CONFIG", "HERDR_TOOLS_REPO", "HERDR_MACHINE", "HERDR_PROC_DIR", "HERDR_VERIFIED_TREE",
	"HC_FILL_VERDICTS", "HC_FILL_INNER", "HC_FILL_SELF", "HC_FILL_DATA", "HC_FILL_RUNS", "HC_FILL_COORD",
	"CLAUDE_CONFIG_DIR", "GOFLAGS", "HERDR_FUTURE_KNOB",
}

// Poison: LaneEnv를 모두 「poison-<이름>」으로 시험 동안만 넣는다(끝나면 되돌림) — 시험 틀을 지은 뒤 Leaked로 새는 값을 본다
func Poison(t *testing.T) {
	t.Helper()
	for _, k := range LaneEnv {
		t.Setenv(k, "poison-"+k)
	}
}

// Leaked: 자식 환경 env에 Poison이 넣은 값이 그대로 남은 이름들
func Leaked(env map[string]string) []string {
	var l []string
	for _, k := range LaneEnv {
		if v, ok := env[k]; ok && strings.HasPrefix(v, "poison-") {
			l = append(l, k)
		}
	}
	return l
}
