package hclane_test

// reland-live-lane: hc-lane land(병합기 제출)는 같은 레인 제출을 병합기가 지금 착지 중(원격 refs/herdr/landing/<큐 이름> = 그 큐 SHA)이면
// 종료 2로 거절하고, 큐에 서 있기만 하면 갈아 끼운다. 내려는 커밋이 원격 lane/<레인>의 조상(옛 커밋)이면 --force-old 없이는 종료 2.

import (
	"strings"
	"testing"
)

func TestLandingGuard(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("lg")
		k.mklane("lgl")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		env := []string{"HC_LANE=lgl", "HC_LANE_MGON_WAITS=0"}
		queue := func() string {
			return k.git(o, "for-each-ref", "--format=%(objectname) %(refname)", "refs/merge-queue/")
		}
		k.lndE(env, k.wt, "main")
		q1 := queue()
		k.check("첫 제출", k.rc == 0 && strings.Contains(q1, "refs/merge-queue/"))
		f := strings.Fields(q1)
		if len(f) != 2 {
			t.Fatalf("큐: %q", q1)
		}
		qn := strings.TrimPrefix(f[1], "refs/merge-queue/")

		// 병합기가 그 제출을 착지 중 → 새 커밋을 내도 거절, 큐 그대로
		k.git(o, "update-ref", "refs/herdr/landing/"+qn, f[0])
		k.sh(k.wt, `echo 2 >lg2.txt && git add -- lg2.txt && git commit -qm lg2 -- lg2.txt`)
		k.lndE(env, k.wt, "main")
		k.check("착지 중: 종료 2·안내 글·큐 그대로", k.rc == 2 && strings.Contains(k.errf(), "lgl은 병합기가 지금 착지 중 — 결과(착지·막힘)를 기다린다") && queue() == q1)

		// 끝난 착지의 남은 표식(큐 SHA와 다름)은 착지 중이 아니다 → 줄에만 선 제출은 갈아 끼움
		k.git(o, "update-ref", "refs/herdr/landing/"+qn, k.git(k.wt, "rev-parse", "HEAD~1^"))
		k.lndE(env, k.wt, "main")
		q2 := queue()
		k.check("줄에만 있음: 갈아 끼움", k.rc == 0 && q2 != q1 && strings.HasPrefix(q2, k.git(k.wt, "rev-parse", "HEAD")+" "))

		// 원격 lane/<레인>보다 옛 커밋 → 거절, --force-old면 낸다
		k.sh(k.wt, `git reset -q --keep HEAD~1`)
		k.lndE(env, k.wt, "main")
		k.check("옛 커밋: 경고·종료 2·큐 그대로", k.rc == 2 && strings.Contains(k.errf(), "원격 lane/lgl이 더 새롭다") && queue() == q2)
		k.lndE(env, k.wt, "main", "--force-old")
		k.check("옛 커밋 --force-old: 제출", k.rc == 0 && strings.HasPrefix(queue(), k.git(k.wt, "rev-parse", "HEAD")+" "))
	})
}
