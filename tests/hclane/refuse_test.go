package hclane_test

// fill-reject-hold(2026-11-24): 띄우기의 입력 거절(다시 해도 같음)은 종료 11 — hc-fill이 그 줄을 멈추고 지휘에 알린다.
// 있나 조회 자체가 실패(hc 실패)하면 거절이 아니라 일시 실패(종료 1) 그대로. 둘 다 작업 트리를 만들지 않는다

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchRefuseExit(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("trf")
		k.sh(k.R, `git branch rf-b`)
		k.run("--name", "rf-b", "--repo", k.R, "--", "true")
		k.check("로컬 가지가 이미 있음 → 종료 11", k.rc == 11 && strings.Contains(k.errf(), "로컬 브랜치 rf-b 가 이미 있다") && !exists(k.R+".wt/rf-b"))

		k.sh(filepath.Join(k.D, "seed"), `git push -q origin HEAD:refs/heads/rf-r`)
		k.run("--name", "rf-r", "--repo", k.R, "--", "true")
		k.check("원격 가지가 이미 있음(fetch 뒤) → 종료 11", k.rc == 11 && strings.Contains(k.errf(), "원격 브랜치 origin/rf-r 가 이미 있다") && !exists(k.R+".wt/rf-r"))

		if err := os.MkdirAll(k.R+".wt/rf-w", 0o755); err != nil {
			t.Fatal(err)
		}
		k.run("--name", "rf-w", "--repo", k.R, "--", "true")
		k.check("워크트리 폴더가 이미 있음 → 종료 11", k.rc == 11 && strings.Contains(k.errf(), "워크트리 폴더가 이미 있다"))

		k.run("--name", "rf-n", "--repo", k.R, "--brief", k.T+"/없는-지시문.md", "--", "agent", "{brief}")
		k.check("--brief 지시문 없음 → 종료 11", k.rc == 11 && strings.Contains(k.errf(), "지시문 파일이 없다") && !exists(k.R+".wt/rf-n"))

		k.runE([]string{"HC_FAKE_EXEC_FAIL=*show-ref --verify -q refs/remotes/origin/rf-x*"}, "--on", "box", "--name", "rf-x", "--repo", k.R, "--", "true")
		k.check("있나 조회가 hc 실패 → 거절 아님(종료 1, 일시)", k.rc == 1 && !exists(k.R+".wt/rf-x"))

		// --resume 옛 주차 길(refs/park 없고 lane/<레인>만): 주차 사건이 없으면 11, 사건을 못 읽으면(hc 실패) 모름 — 1(감사 중간-1)
		k.sh(filepath.Join(k.D, "seed"), `git push -q origin HEAD:refs/heads/lane/rf-p`)
		k.run("--resume", "--name", "rf-p", "--repo", k.R, "--", "true")
		k.check("--resume: 주차 사건 없는 옛 제출 가지 → 종료 11", k.rc == 11 && strings.Contains(k.errf(), "주차 사건이 없는 제출 가지") && !exists(k.R+".wt/rf-p"))
		k.runE([]string{"HC_FAKE_EXEC_FAIL=*events.tsv*"}, "--resume", "--on", "box", "--name", "rf-p", "--repo", k.R, "--", "true")
		k.check("--resume: 사건 읽기가 hc 실패 → 거절 아님(종료 1, 일시)", k.rc == 1 && strings.Contains(k.errf(), "사건(events.tsv)을 읽지 못해") && !exists(k.R+".wt/rf-p"))

		k.run("--name", "rf-ok", "--repo", k.R, "--", "true")
		k.check("대조: 아무것도 없으면 띄움", k.rc == 0 && k.regd("rf-ok"))
	})
}
