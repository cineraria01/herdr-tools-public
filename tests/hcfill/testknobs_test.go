package hcfill_test

// ③ 태그 없이 지은 hc-fill(배포 꼴)은 시험 손잡이를 줘도 아무 일 없다(2026-11-24 test-knobs-buildtag) — 같은 장면을 시험 빌드로
// 먼저 돌려 그 손잡이가 이 장면에서 실제로 먹는 것을 본 뒤(헛돌지 않게), 태그 없는 빌드로 다시 돌린다.

import (
	"os"
	"testing"

	"herdr-tools/internal/clitest"
)

func TestTestKnobsOffInPlainBuild(t *testing.T) {
	k := newK(t)
	tagged := k.FILL
	plain := clitest.BinPlain(t, clitest.TempDir(t), "hc-fill")
	IS := k.IS()
	for _, c := range []struct {
		bin   string
		knobs bool
	}{{tagged, true}, {plain, false}} {
		k.FILL = c.bin
		k.ifresh()
		k.mj("box-1", "shared", "slow") // box-2는 연결 안 됨 — 대기로 남는다
		k.req(IS, "inq-k", "app", "-", "box-2")
		k.run("run")
		// 셸 끼어들기(HC_FILL_TEST_QSET_CMD)는 rm 길에 없다 — 그 손잡이는 바이너리 글자 시험(internal/clitest)이 본다
		k.fill([]string{"HC_FILL_CRASH_AT=rm-after-cancel"}, "rm", "inq-k")
		_, crashed := os.Stat(k.DATA + "/crash.log")
		if c.knobs {
			k.check("시험 빌드: 중단 주입이 먹는다(줄은 대기열에, crash.log)", crashed == nil && k.cState("inq-k") == "wait")
		} else {
			k.check("태그 없는 빌드: 중단 주입 없이 rm이 끝난다(crash.log 없음, 줄 빠짐)", crashed != nil && k.where("inq-k") == "")
		}
	}
	k.FILL = tagged
}
