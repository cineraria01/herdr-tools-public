package util_test

// cli-usage-version(2026-11-24): hc-lane 밖 도구의 인자 오류도 운영 기록에 die가 아니라 usage 줄 — 표준 오류 글·종료 코드는 그대로

import (
	"strconv"
	"testing"
)

func TestOpLogBadArgOtherTools(t *testing.T) {
	for _, c := range []struct {
		tool string
		args []string
		rc   int
		err  string
	}{
		{"hc-merger", []string{"--bogus"}, 1, "hc-lane: merger: 알 수 없는 인자: --bogus\n"},
		{"hc-merger", []string{"--tick"}, 1, "hc-lane: --tick 값이 없다\n"},
		{"herdr-settings-merge", []string{"--bogus"}, 2, "herdr-settings-merge: 모르는 인자: --bogus\n"},
		{"herdr-tools-config", []string{"get"}, 2, "herdr-tools-config: get <키> [<기본값>]\n"},
	} {
		T := tmpdir(t)
		ops := T + "/ops.tsv"
		_, se, rc := run2(T, isolated(T).With("HOME="+T, "HERDR_TOOLS_OPLOG="+ops), tool(t, c.tool), c.args...)
		if rc != c.rc || se != c.err {
			t.Errorf("%s %q: 글·종료 코드 그대로여야: rc=%d err=%q", c.tool, c.args, rc, se)
			continue
		}
		if rs := opRows(t, ops); len(rs) != 1 || rs[0][1] != c.tool || rs[0][4] != "usage" || rs[0][5] != strconv.Itoa(c.rc) {
			t.Errorf("%s %q: usage 줄 하나여야: %q", c.tool, c.args, rs)
		}
	}
}
