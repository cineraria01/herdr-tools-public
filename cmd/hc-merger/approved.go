package main

// 묶음 구성원의 감사 승인 확인(merger-batch-audited).

import (
	"fmt"
	"os"

	"herdr-tools/internal/gitx"
)

// approved: 묶음에 담을 구성원의 감사 승인 확인 — 판정이 붙은 제출이면 그 제출의 갈래점(기준과의 merge-base) 뒤 변경의 patch-id가
// 자기 승인 patch-id와 같아야 한다. 혼자일 때 착지(hc-lane land의 HERDR_LAND_PATCHID 대조)가 보는 것과 같은 판정·같은 함수(gitx.PatchID).
// 판정 없는 제출은 늘 통과. git이 실패하면 통과가 아니다(모름을 같음으로 읽지 않는다)
func (m *M) approved(e entry) bool {
	if e.v == "" {
		return true
	}
	mb, rc := out("merge-base", e.s, m.B)
	got := ""
	if rc == 0 {
		got, rc = gitx.PatchID(os.Stderr, mb, e.s)
	}
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(판정 %s 승인 patch-id를 확인하지 못했다(git 실패), 혼자 차례에)\n", e.l, e.v)
		return false
	}
	if got != e.p {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(변경 patch-id %s ≠ 판정 %s 승인 %s, 혼자 차례에)\n", e.l, first12(got), e.v, first12(e.p))
		return false
	}
	fmt.Printf("hc-lane merger: 승인 확인 — %s 제출 ID %s 판정 %s patch-id %s 일치\n", e.l, e.d, e.v, first12(e.p))
	return true
}

func first12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
