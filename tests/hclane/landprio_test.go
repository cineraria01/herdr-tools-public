package hclane_test

// fill-edit-after(2026-11-22): 같은 커밋을 다시 hc-lane land(병합기 제출)하면 「이미 큐에 있다 — 새로 내지 않음」으로 끝나 우선을 올릴 길이
// 없었다(지휘가 refs/merge-queue-meta/<항목> 제목의 priority=를 손으로 다시 써 lease push). HC_LANE_PRIORITY를 적었고 지금 메타와 다르면
// 그 메타 ref만 같은 내용·바뀐 priority로 갈아 끼운다(--force-with-lease로 옛 값 확인). 적지 않았으면 그대로(띄울 때 값이 남은 레인이
// 지휘가 올린 우선을 되돌리지 않게).

import (
	"strings"
	"testing"
)

func TestLandPrioritySwap(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("lp")
		k.mklane("lpl")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		env := []string{"HC_LANE=lpl", "HC_LANE_MGON_WAITS=0"}
		refs := func(p string) string {
			return k.git(o, "for-each-ref", "--format=%(objectname) %(refname)", p)
		}
		meta := func() (string, string) { // 메타 SHA · 제목
			f := strings.Fields(refs("refs/merge-queue-meta/"))
			if len(f) != 2 {
				return "", ""
			}
			return f[0], k.git(o, "log", "-1", "--format=%s", f[0])
		}
		k.lndE(env, k.wt, "main")
		q1 := refs("refs/merge-queue/")
		m1, s1 := meta()
		k.check("첫 제출: 우선 보통", k.rc == 0 && q1 != "" && strings.HasSuffix(s1, " priority=보통"))

		k.lndE(append(env, "HC_LANE_PRIORITY=긴급"), k.wt, "main")
		m2, s2 := meta()
		k.check("같은 커밋 + HC_LANE_PRIORITY 다름: 큐 그대로, 메타 priority만 긴급",
			k.rc == 0 && refs("refs/merge-queue/") == q1 && m2 != m1 && s2 == strings.Replace(s1, "priority=보통", "priority=긴급", 1) &&
				strings.Contains(k.out, "priority=긴급"))
		if m2 != m1 {
			for _, f := range []string{"%T %P %an %ae %ad %cn %ce %cd", "%b"} {
				k.check("메타 나머지 그대로 "+f, k.git(o, "log", "-1", "--format="+f, m1) == k.git(o, "log", "-1", "--format="+f, m2))
			}
		}

		k.lndE(append(env, "HC_LANE_PRIORITY=urgent"), k.wt, "main")
		m3, _ := meta()
		k.check("같은 값(영어 이름): 그대로", k.rc == 0 && m3 == m2 && strings.Contains(k.out, "이미 병합기 큐에 있다"))

		k.lndE(env, k.wt, "main")
		m4, _ := meta()
		k.check("HC_LANE_PRIORITY 없음: 지휘가 올린 우선을 되돌리지 않음", k.rc == 0 && m4 == m2)
	})
}
