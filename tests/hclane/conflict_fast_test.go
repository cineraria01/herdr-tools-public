package hclane_test

// conflict-fast A(2026-11-23 — 병합기 일지 11/20~23에서 rebase 충돌 반송 28건 가운데 8건은 제출할 때 이미 기본 브랜치와 충돌했는데,
// 줄에서 중앙값 29분을 기다린 뒤에야 반송됐다): 병합기 제출은 fetch 뒤 제출 커밋을 origin/<기본>과 3자 합침(merge-tree)으로 견줘
// 충돌이면 큐에 넣지 않고 종료 3. 판정을 못 하면(git 실패) 지금처럼 낸다. rebase로 풀고 같은 줄을 다시 부르면 통과한다.

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitConflictEarly(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("cf")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		env := []string{"HC_LANE=cfl", "HC_LANE_MGON_WAITS=0"}
		queued := func() string {
			return k.git(o, "for-each-ref", "--format=%(refname)", "refs/merge-queue/", "refs/merge-queue-meta/", "refs/heads/lane/")
		}
		k.mklane("cfl")
		k.sh(k.wt, `printf 'lane\n' >b && git commit -qam lane-b`)
		k.seedp("b", "origin")

		k.lndE(env, k.wt, "main")
		e := k.errf()
		k.check("충돌하는 제출: 종료 3, 큐·메타·lane 가지 없음", k.rc == 3 && queued() == "")
		k.check("충돌하는 제출: 충돌 파일과 rebase 안내", strings.Contains(e, "충돌 b") && strings.Contains(e, "git rebase origin/main") &&
			strings.Contains(e, "같은 줄"))
		k.check("충돌하는 제출: 전체 검증 대신 푼 파일 둘레만 다시 본다", strings.Contains(e, "푼 파일 둘레"))

		// 판정 못 함(merge-tree 실패) → 지금처럼 제출
		rg, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		fk := filepath.Join(k.T, "fakegit")
		mkdirs(t, fk)
		write(t, fk+"/git", "#!/bin/bash\nfor a; do [ \"$a\" = merge-tree ] && exit 128; done\nexec '"+rg+"' \"$@\"\n")
		k.sh(fk, `chmod +x git`)
		k.lndE(append(env, "PATH="+fk+":"+k.env["PATH"]), k.wt, "main")
		k.check("판정 못 함: 모름을 충돌로 읽지 않고 제출", k.rc == 0 && strings.Contains(queued(), "refs/merge-queue/"))
		for _, r := range strings.Fields(queued()) { // 다음 단언을 위해 큐를 비운다
			k.git(o, "update-ref", "-d", r)
		}

		// 거절 뒤 rebase로 풀고 같은 줄 → 통과
		k.sh(k.wt, `git rebase -q origin/main >/dev/null 2>&1; printf 'origin\nlane\n' >b && git add -- b && GIT_EDITOR=true git rebase --continue >/dev/null`)
		k.lndE(env, k.wt, "main")
		k.check("rebase 뒤 같은 줄: 제출", k.rc == 0 && strings.Contains(queued(), "refs/merge-queue/") && strings.Contains(k.out, "제출"))

		// 기준과 같은 patch-id 커밋이 든 제출(기준이 그 뒤 같은 파일을 다시 고침)은 3자 합침이 충돌해도 거절하지 않는다 —
		// 일반 rebase는 그 커밋을 건너뛰어 깨끗할 수 있다(감사 atoxvrv-yecgh5 낮음 3)
		k.mklane("cfc")
		k.sh(k.wt, `printf 'x\n' >c && git add -- c && git commit -qm lane-c`)
		k.seedp("c", "x")
		k.seedp("c", "y")
		k.lndE([]string{"HC_LANE=cfc", "HC_LANE_MGON_WAITS=0"}, k.wt, "main")
		k.check("같은 patch-id 커밋이 든 제출: 거절하지 않고 제출", k.rc == 0 && strings.Contains(queued(), "-cfc"))

		// 충돌 없는 제출은 그대로
		k.mklane("cfm")
		k.lndE([]string{"HC_LANE=cfm", "HC_LANE_MGON_WAITS=0"}, k.wt, "main")
		k.check("충돌 없는 제출: 그대로 제출", k.rc == 0 && strings.Contains(queued(), "-cfm"))
	})
}
