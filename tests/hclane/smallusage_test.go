package hclane_test

// small-usage-fixes(2026-11-23):
//   - gc·clean --on에서 hc 자신이 실패하면(모르는 기계 등) 그 한 줄로 끝낸다 — 뒤에 「git 저장소가 아니다」를 덧붙이지 않는다
//   - HEAD가 없는 가지를 가리키는 작업 트리는 「깨진 작업 트리」로 남기고(지우지 않음) 그것만 있는 바퀴는 종료 0

import (
	"strings"
	"testing"
)

func TestSmallUsageFixes(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("suf")
		k.seed(".gitignore", ".omc/")

		// hc 실패(저장소 조회부터) — hc 실패 줄은 하나, 저장소 탓 줄은 없음, 종료 1(고치기 전과 같은 값)
		for _, sub := range [][]string{
			{"gc", "--on", "box", "--repo", k.R, "--dry-run"},
			{"clean", "--on", "box", "--repo", k.R, "--branch", "nope"},
		} {
			k.runE([]string{"HC_FAKE_EXEC_FAIL=*worktree list*"}, sub...)
			e := k.errf()
			k.check(sub[0]+" --on: hc 실패면 그 한 줄로 끝남(종료 1)", k.rc == 1 && strings.Count(e, "hc exec 실패(unavailable") == 1 &&
				!strings.Contains(e, "git 저장소가 아니다"))
		}

		// 깨진 작업 트리: 가지를 지워 HEAD가 없는 가지를 가리킨다
		k.land("g-ok")
		k.run("--name", "g-lost", "--repo", k.R, "--", "true")
		k.check("준비: 가지 지움", k.gitOK(k.R, "update-ref", "-d", "refs/heads/g-lost"))
		k.run("gc", "--repo", k.R, "--min-age", "0")
		k.check("gc: 깨진 작업 트리 줄", strings.Contains(k.out, "남김 깨진 작업 트리 "+k.R+".wt/g-lost — HEAD가 없는 가지 g-lost을 가리킴(사람이 본다)"))
		k.check("gc: 깨진 작업 트리만 남으면 종료 0, 폴더는 그대로", k.rc == 0 && isDir(k.R+".wt/g-lost") && k.regd("g-lost"))
		k.check("gc: 같은 저장소의 정상 작업 트리는 지움", k.gone("g-ok"))
		k.check("gc: 확인 못 함 줄 없음", !strings.Contains(k.out, "확인하지 못함"))
	})
}
