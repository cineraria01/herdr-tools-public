package hclane_test

// sweep-landed-keep: 레인이 자기 작업 트리를 hc-lane clean으로 정리하면(착지·제출·주차 뒤 마지막 단계) 자기 터미널의 hc keep을 끈다.
// Herdr 창에 붙인 터미널은 keep이 켜져 있어, 에이전트가 끝나면 셸로 바뀐 채 status 없이 남아 그 기계 hc-fill 자리를 붙잡았다
// (2026-11-21 mini-m4 land-claim-net 13시간). 다른 레인을 정리하는 clean(hc-sweep·gc가 부름)과 dry-run은 keep을 건드리지 않는다.

import "os"

func init() { reg(130, "s24-keepoff", keepOffSec) }

// keepOff <주소>: 가짜 hc 기록에 「keep <주소> --off」가 잇달아 있다.
func (k *K) keepOff(addr string) bool {
	l := k.hclog("")
	for i := 0; i+2 < len(l); i++ {
		if l[i] == "keep" && l[i+1] == addr && l[i+2] == "--off" {
			return true
		}
	}
	return false
}

func keepOffSec(k *K) {
	k.mk("t24")
	k.seed(".gitignore", ".omc/")

	k.land("ko-self")
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t5ko", "HC_LANE=ko-self"}, "clean", "--repo", k.R, "--branch", "ko-self")
	k.check("keepoff: 자기 레인 clean → 종료 0·정리", k.rc == 0 && k.gone("ko-self"))
	k.check("keepoff: 자기 레인 clean이 자기 터미널 keep을 끔(hc keep t5ko --off)", k.keepOff("t5ko"))

	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t5ko", "HC_LANE=ko-self"}, "clean", "--repo", k.R, "--branch", "ko-self")
	k.check("keepoff: 이미 정리됨(nothing)이어도 자기 레인이면 keep을 끔", k.rc == 0 && k.last(`"clean":"nothing"`) && k.keepOff("t5ko"))

	k.land("ko-other")
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t5sw", "HC_LANE=ko-self"}, "clean", "--repo", k.R, "--branch", "ko-other")
	k.check("keepoff: 다른 레인 정리(HC_LANE이 가지와 다름)는 keep을 건드리지 않음", k.rc == 0 && k.gone("ko-other") && !k.has("keep"))

	k.land("ko-nolane")
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t5sw"}, "clean", "--repo", k.R, "--branch", "ko-nolane")
	k.check("keepoff: 레인 밖(HC_LANE 없음) clean은 keep을 건드리지 않음", k.rc == 0 && !k.has("keep"))

	k.land("ko-dry")
	os.Remove(k.HCLOG)
	k.runE([]string{"HC_TERMINAL_ID=t5ko", "HC_LANE=ko-dry"}, "clean", "--repo", k.R, "--branch", "ko-dry", "--dry-run")
	k.check("keepoff: dry-run은 keep을 건드리지 않음", k.rc == 0 && !k.has("keep"))
}
