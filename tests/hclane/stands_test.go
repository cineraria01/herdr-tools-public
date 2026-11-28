package hclane_test

// ht-submit-still-stands(2026-11-28 — 레인이 완료 뒤 막힘·주차를 보고했는데 앞서 돌던 감사의 통과로 대기열이 그 커밋을 제출했다): 제출 직전의
// 마지막 막음(cmd/hc-lane/stands.go)을 병합기가 켜진 저장소에서 못 박는다.
// ⑧ 원격 refs/park/<레인>이 제출할 커밋이고 그 레인의 마지막 완료·주차가 그 커밋의 주차 → 종료 2·큐 없음(판정 없는 land도).
// 이어 띄운 레인이 같은 커밋으로 완료를 다시 적으면 낸다(rebase가 아무것도 안 바꿔 refs/park가 그대로인 꼴). 주차 안 됨은 지금 그대로.
// 판정 착지(--verdict)는 그 판정 실행의 마지막 완료·막힘·주차가 그 커밋의 완료여야 한다 — 아니면 종료 2·시작 표시 없음(바퀴 순서 반대·옛 판 대기열)

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLandStillStands(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("sst")
		o := k.D + "/origin.git"
		k.git(o, "update-ref", "refs/herdr/merger/box", "main")
		VD := filepath.Join(k.T, ".local/share/hc-fill/verdicts")
		EVF := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
		queued := func(l string) string {
			return k.git(o, "for-each-ref", "--format=%(refname)", "refs/merge-queue/*-"+l)
		}
		head := func() string { return k.git(k.wt, "rev-parse", "HEAD") }
		evl := func(l, st, sha, run string) {
			appendf(t, EVF, ev("2026-11-28T01:00:00Z", "box", "t1", l, st, sha, "글", "0", "-", run))
		}
		vput := func(l, id string) {
			mkdirs(t, VD+"/"+l)
			write(t, VD+"/"+l+"/"+id+".md", fmt.Sprintf("aid\t%s\nlane\t%s\nrun\trv-1\nsha\t%s\nverdict\tpass\nreport\t~/rep/%s.md\nround\t1\nbase\tmain\n", id, l, head(), id))
		}
		plain := func(l string) { k.lndE([]string{"HC_LANE=" + l, "HC_LANE_MGON_WAITS=0"}, k.wt, "main") }
		verdict := func(l, id string) {
			k.lndE([]string{"HC_LANE=" + l, "HC_LANE_JOB=rv-1", "HC_LANE_MGON_WAITS=0"}, k.wt, "main", "--verdict", id)
		}

		// ⑧ 주차된 커밋
		k.mklane("ss1")
		s1 := head()
		k.git(k.wt, "push", "-q", "origin", "HEAD:refs/park/ss1")
		evl("ss1", "완료", s1, "rv-1")
		evl("ss1", "주차", s1, "rv-1")
		plain("ss1")
		k.check("⑧ 주차된 커밋: 종료 2, 큐 없음, 까닭과 푸는 법", k.rc == 2 && queued("ss1") == "" &&
			strings.Contains(k.errf(), "주차돼 있다") && strings.Contains(k.errf(), "hc-lane report 완료"))
		evl("ss1", "완료", s1, "rv-2") // 이어 띄운 실행이 같은 커밋으로 완료
		plain("ss1")
		k.check("⑧ 주차 뒤 같은 커밋 완료(이어 띄움, refs/park 그대로): 제출", k.rc == 0 && queued("ss1") != "")
		k.mklane("ss2")
		plain("ss2")
		k.check("⑧ 주차 안 됨: 지금 그대로 제출", k.rc == 0 && queued("ss2") != "")

		// 판정 착지: 완료 뒤 막힘
		k.mklane("ss3")
		vput("ss3", "v3")
		evl("ss3", "완료", head(), "rv-1")
		evl("ss3", "막힘", "-", "rv-1")
		verdict("ss3", "v3")
		_, ackErr := os.Stat(VD + "/ss3/v3.ack")
		k.check("판정 착지 — 완료 뒤 막힘: 종료 2, 큐 없음, 시작 표시 없음", k.rc == 2 && queued("ss3") == "" && ackErr != nil &&
			strings.Contains(k.errf(), "서 있지 않다"))
		evl("ss3", "완료", head(), "rv-1")
		verdict("ss3", "v3")
		k.check("판정 착지 — 같은 커밋 완료를 다시: 제출", k.rc == 0 && queued("ss3") != "")
		k.mklane("ss4")
		vput("ss4", "v4")
		evl("ss4", "완료", head(), "rv-1")
		evl("ss4", "중간", "-", "rv-1")
		evl("ss4", "막힘", "-", "hcfill@1") // hc-fill이 그 레인 이름으로 남긴 막힘(다른 실행)
		verdict("ss4", "v4")
		k.check("판정 착지 — 중간 글·다른 실행의 막힘은 세지 않음: 제출", k.rc == 0 && queued("ss4") != "")
	})
}

// 감사 중간 1: 주차한 레인의 clean이 그 폴더를 쓰던 감사 터미널에 막혀 작업 트리가 남아도, 이어 띄우기(--resume)가 지금 있는 clean을 한 번
// 불러 「주차됨」으로 지우고 띄운다. clean이 못 지우는 꼴(커밋 안 된 파일)이면 지금처럼 「이미 있다」로 거절하고 파일은 그대로
func TestResumeCleansParkedLeftover(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("srl")
		for _, n := range []string{"pl-a", "pl-b"} {
			k.mklane(n)
			k.runIn(k.wt, []string{"HC_LANE=" + n, "HC_LANE_LEASE=" + s22lref(k, n)}, "park")
			k.check(n+": 주차됨(작업 트리는 남음 — clean이 막힌 꼴)", k.rc == 0 && exists(k.wt))
		}
		ps := k.git(k.R+".wt/pl-a", "rev-parse", "HEAD")
		k.run("--resume", "--name", "pl-a", "--repo", k.R, "--", "claude")
		k.check("남은 작업 트리를 clean으로 지우고 주차한 가지에서 다시 띄움", k.rc == 0 && k.git(k.R+".wt/pl-a", "rev-parse", "HEAD") == ps &&
			strings.Contains(k.errf(), "주차 뒤 남은 작업 트리"))
		write(t, k.R+".wt/pl-b/left.txt", "아직 안 커밋\n")
		k.run("--resume", "--name", "pl-b", "--repo", k.R, "--", "claude")
		k.check("clean이 못 지우면 지금처럼 거절, 커밋 안 된 파일 그대로", k.rc != 0 && strings.Contains(k.errf(), "이미 있다") &&
			read(k.R+".wt/pl-b/left.txt") == "아직 안 커밋\n")
	})
}
