package hclane_test

// gofix3-gitref(재감사 아스트라 2바퀴 ②-C·②-I): hc-lane land(병합기 제출)의 원격 판독과 병합기 착지 중 표식과의 경합.
//   lane/R-212 착지 표식 줄이 깨진 조회 응답 → 큐·메타·표식 그대로, 75
//   lane/R-213 재제출의 조회 뒤·원자 push 직전에 병합기가 표식을 올림 → 재제출 push 거절, 옛 큐·메타 그대로
//   lane/R-214 같은 SHA·다른 우선 재제출인데 표식 있음(또는 push 직전에 생김) → 메타 그대로, 0

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// grFake: PATH 앞에 둘 가짜 git 폴더 — script가 처리하지 않으면 진짜 git으로
func grFake(k *K, script string) string {
	k.t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		k.t.Fatal(err)
	}
	d := k.t.TempDir()
	body := "#!/bin/sh\nREAL='" + real + "'\n" + script + "\nexec \"$REAL\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(d, "git"), []byte(body), 0o755); err != nil {
		k.t.Fatal(err)
	}
	return "PATH=" + d + ":" + k.env["PATH"]
}

func grRefs(k *K, o, p string) string {
	return k.git(o, "for-each-ref", "--format=%(objectname) %(refname)", p)
}

func TestGitrefSubmitLanding(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("gr")
		k.mklane("grl")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		env := []string{"HC_LANE=grl", "HC_LANE_MGON_WAITS=0"}
		k.lndE(env, k.wt, "main")
		q1, m1 := grRefs(k, o, "refs/merge-queue/"), grRefs(k, o, "refs/merge-queue-meta/")
		f := strings.Fields(q1)
		k.check("첫 제출", k.rc == 0 && len(f) == 2)
		if len(f) != 2 {
			t.Fatalf("큐: %q", q1)
		}
		s1, qn := f[0], strings.TrimPrefix(f[1], "refs/merge-queue/")
		mk := "refs/herdr/landing/" + qn
		lane1 := grRefs(k, o, "refs/heads/lane/")
		same := func() bool {
			return grRefs(k, o, "refs/merge-queue/") == q1 && grRefs(k, o, "refs/merge-queue-meta/") == m1 && grRefs(k, o, "refs/heads/lane/") == lane1
		}

		// R-212: 표식 줄 이름이 잘린(SHA만) 응답 — 모름, 75, 아무것도 안 바꿈
		k.git(o, "update-ref", mk, s1)
		k.sh(k.wt, `echo 2 >gr2.txt && git add -- gr2.txt && git commit -qm gr2 -- gr2.txt`)
		cut := grFake(k, `case "$*" in *"refs/herdr/landing/*"*) "$REAL" "$@" | sed 's#	refs/herdr/landing/.*##'; exit 0 ;; esac`)
		k.lndE(append(env, cut), k.wt, "main")
		k.check("R-212 잘린 표식 줄: 75·큐·메타·lane 그대로", k.rc == 75 && same() && k.git(o, "rev-parse", mk) == s1)
		notail := grFake(k, `case "$*" in *"refs/herdr/landing/*"*) "$REAL" "$@" | tr -d '\n'; exit 0 ;; esac`)
		k.lndE(append(env, notail), k.wt, "main")
		k.check("R-212 끝 줄바꿈 없는 응답: 75·그대로", k.rc == 75 && same())

		// R-213: 표식이 없는 동안 조회 → push 직전에 병합기가 표식(같은 제출 SHA) → 재제출 거절, 옛 큐·메타·표식 그대로
		k.git(o, "update-ref", "-d", mk)
		race := grFake(k, `[ "$1 $2 $3" = "push --atomic --porcelain" ] && "$REAL" --git-dir='`+o+`' update-ref `+mk+` `+s1)
		k.lndE(append(env, race), k.wt, "main")
		k.check("R-213 경합: 재제출 거절·큐·메타·lane 그대로·표식 그대로", k.rc != 0 && same() && k.git(o, "rev-parse", mk) == s1)
		k.lndE(env, k.wt, "main")
		k.check("R-213 다시: 착지 중이라 종료 2·그대로", k.rc == 2 && strings.Contains(k.errf(), "병합기가 지금 착지 중") && same())

		// R-214: 같은 SHA(s1)를 다른 우선으로 다시 냄 — 표식 있음 → 메타 그대로, 0
		k.sh(k.wt, `git reset -q --keep HEAD~1`)
		k.lndE(append(env, "HC_LANE_PRIORITY=긴급"), k.wt, "main")
		k.check("R-214 착지 중 같은 SHA·다른 우선: 0·메타 그대로", k.rc == 0 && same() && strings.Contains(k.out, "지금 착지 중"))
		// 조회 때는 표식이 없었는데 메타를 바꾸는 push 직전에 생김 → 메타 그대로(우선 그대로 안내)
		k.git(o, "update-ref", "-d", mk)
		race2 := grFake(k, `[ "$1 $2 $3 $4" = "push -q --atomic --porcelain" ] && "$REAL" --git-dir='`+o+`' update-ref `+mk+` `+s1)
		k.lndE(append(env, race2, "HC_LANE_PRIORITY=긴급"), k.wt, "main")
		k.check("R-214 경합: 메타 그대로", k.rc == 0 && same() && k.git(o, "rev-parse", mk) == s1)

		// 경합이 없으면 갈아 끼움은 그대로 된다(남은 표식 = 큐 SHA와 다름)
		k.git(o, "update-ref", mk, k.git(k.wt, "rev-parse", "main"))
		k.sh(k.wt, `echo 3 >gr3.txt && git add -- gr3.txt && git commit -qm gr3 -- gr3.txt`)
		k.lndE(env, k.wt, "main")
		k.check("남은 표식(SHA 다름): 갈아 끼움", k.rc == 0 && strings.HasPrefix(grRefs(k, o, "refs/merge-queue/"), k.git(k.wt, "rev-parse", "HEAD")+" "))
	})
}
