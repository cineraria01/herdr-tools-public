package util_test

// agent-skill-guides ①: 도구마다 `<도구> --skill`이 cmd/<도구>/skill.md를 그대로 찍고 종료 0 — 설정·기계 확인 없이(빈 HOME, 설정 폴더 없음,
// hc 없는 PATH). 안내 내용과 등록 대조는 tests/repo/skill_test.go(파일만 읽는 싼 검사)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillFlagPrintsGuide(t *testing.T) {
	for _, name := range []string{"hc-fill", "hc-lane", "hc-merger", "herdr-sync", "hc-sweep"} {
		want, err := os.ReadFile(filepath.Join(repo, "cmd", name, "skill.md"))
		if err != nil {
			t.Fatal(err)
		}
		T := tmpdir(t)
		e := isolated(T).With("HOME="+T, "HERDR_TOOLS_CONFIG="+T+"/없음", "HC_FILL_DATA="+T+"/data", "HERDR_TOOLS_OPLOG="+T+"/ops.tsv", "PATH=/usr/bin:/bin")
		out, se, rc := run2(T, e, tool(t, name), "--skill")
		w := strings.TrimRight(string(want), "\n") // run2는 끝 줄바꿈을 뗀다
		if rc != 0 || out != w || se != "" {
			t.Errorf("%s --skill: 종료 0·안내 그대로·오류 출력 없음이어야 — 종료 %d, 같음 %v, 오류 %q", name, rc, out == w, se)
		}
		if _, err := os.Stat(T + "/data"); !os.IsNotExist(err) {
			t.Errorf("%s --skill: 아무것도 안 만들어야 한다(data 폴더 %v)", name, err)
		}
	}
}
