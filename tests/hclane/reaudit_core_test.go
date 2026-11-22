package hclane_test

// gofix2-core(Go 전환 재감사 아스트라 1바퀴) — 고치기 전 코드에서 실패하던 시험(Go 판만 단언, 셸 판은 곧 걷는다)

import (
	"path/filepath"
	"strings"
	"testing"
)

// R-27·R-C03: report는 쓰기 전에 사건 전체를 공통 검사(events.Event.Check)로 본다 —
// 긴 HC_LANE_JOB(10칸)이 줄 예산을 넘으면 본문 없는 사건을 쓰지 않고 입력 오류 2, 예산 안의 201자 키는 받는다
func TestReauditReportBudget(t *testing.T) {
	withK(t, func(k *K) {
		evf := filepath.Join(k.T, ".local/state/hc-lane/events.tsv")
		before := read(evf)
		k.runE([]string{"HC_LANE=gf2r", "HC_LANE_JOB=" + strings.Repeat("a", 1100)}, "report", "중간", "-", "확인")
		k.check("R-27 report HC_LANE_JOB 1100자: 종료 2, 사건 안 씀", k.rc == 2 && read(evf) == before)
		key := strings.Repeat("k", 201)
		k.runE([]string{"HC_LANE=gf2r"}, "report", "중간", "-", "확인", "--key", key)
		ln := lastLine(strings.TrimRight(read(evf), "\n"))
		k.check("R-C03 report --key 201자(예산 안): 종료 0, 12칸·키 그대로·본문 있음",
			k.rc == 0 && len(strings.Split(ln, "\t")) == 12 && strings.HasSuffix(ln, "\t"+key) && strings.Contains(ln, "\t확인\t"))
	})
}
