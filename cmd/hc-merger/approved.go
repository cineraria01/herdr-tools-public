package main

// 묶음 구성원의 감사 승인 확인(merger-batch-audited).

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/gitx"
)

// 승인 확인 결과 — 일치·불일치·모름(R-H02: 모름을 불일치로 읽어 승인 제출을 반송하지 않게)
const (
	apOK      = 0 // 일치(또는 판정 없는 제출)
	apDiffer  = 1 // 실제로 다름 — 그 구성원만 이번 묶음에서 빼고 혼자 차례에 착지가 판정을 본다
	apUnknown = 2 // git 실패 — 이 바퀴는 쉰다(바꾼 것 없이 다음 바퀴에)
)

// approved: 묶음에 담을 구성원의 감사 승인 확인 — 판정이 붙은 제출이면 그 제출의 갈래점(기준과의 merge-base) 뒤 변경의 patch-id가
// 자기 승인 patch-id와 같아야 한다. 혼자일 때 착지(hc-lane land의 HERDR_LAND_PATCHID 대조)가 보는 것과 같은 판정·같은 함수(gitx.PatchID).
// 판정 없는 제출은 늘 통과. git이 실패하면 모름(모름을 같음·다름으로 읽지 않는다)
func (m *M) approved(e entry) int {
	if e.v == "" {
		return apOK
	}
	mb, rc := out("merge-base", e.s, m.B)
	got := ""
	if rc == 0 {
		got, rc = gitx.PatchID(os.Stderr, mb, e.s)
	}
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s(판정 %s) 승인 patch-id를 확인하지 못했다(git 실패, 종료 %d) — 모름\n", e.l, e.v, rc)
		return apUnknown
	}
	if got != e.p {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(변경 patch-id %s ≠ 판정 %s 승인 %s, 혼자 차례에)\n", e.l, first12(got), e.v, first12(e.p))
		return apDiffer
	}
	fmt.Printf("hc-lane merger: 승인 확인 — %s 제출 ID %s 판정 %s patch-id %s 일치\n", e.l, e.d, e.v, first12(e.p))
	return apOK
}

// applied: 후보에 얹은 승인 구성원의 결과 트리가 「앞 후보(prev) + 승인 원본의 순변경」과 같은가(R-H01) — 혼자 착지의 승인 대조
// (hc-lane land lverdictCheck)와 같은 계산: merge-tree --merge-base=<원본 갈래점> <prev> <원본>. rebase가 커밋을 하나씩 얹다 빈 커밋을
// 버리면(되돌린 중간 변경이 기준에 따로 들어감) 승인 범위 밖 파일을 옛 내용으로 되돌릴 수 있다. got = 얹은 뒤 후보 커밋.
// 판정 없는 제출은 늘 통과. 3자 합침이 부딪히면 증명할 수 없어 다름
func (m *M) applied(e entry, prev, got string) int {
	if e.v == "" {
		return apOK
	}
	mb, rc := out("merge-base", e.s, m.B)
	if rc != 0 {
		return apUnknown
	}
	want, rc := outQ("merge-tree", "--write-tree", "--no-messages", "--merge-base="+mb, prev, e.s)
	want, _, _ = strings.Cut(want, "\n") // 첫 줄이 트리
	switch rc {
	case 0:
	case 1:
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(승인 원본을 앞 후보에 합치면 부딪혀 결과를 증명할 수 없다, 혼자 차례에)\n", e.l)
		return apDiffer
	default:
		return apUnknown
	}
	tree, rc := out("rev-parse", "--verify", got+"^{tree}")
	if rc != 0 {
		return apUnknown
	}
	if tree != want {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 묶음에서 뺌 — %s(얹은 결과 트리가 승인 원본의 순변경과 다르다 — 승인 밖 변경이 생김, 혼자 차례에)\n", e.l)
		return apDiffer
	}
	return apOK
}

func first12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
