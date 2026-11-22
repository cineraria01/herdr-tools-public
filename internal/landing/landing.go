// Package landing: 「그 레인이 지금 병합기에서 착지 중인가」 판정(2026-11-22 reland-live-lane — 한 프로젝트의 병합기가 sv-go-verify-lock 제출을
// 20분 검증한 뒤, 그 사이 같은 레인의 옛 커밋이 다시 제출돼 큐 참조가 바뀌어 push가 lease로 멈췄다).
// 병합기(cmd/hc-merger)는 착지를 부르기 직전 묶음의 제출마다 원격에 표식 refs/herdr/landing/<큐 이름>(= 그 제출 SHA)을 올리고, 다음 바퀴
// 시작에 남은 표식을 지운다. 표식은 그 큐 참조 refs/merge-queue/<큐 이름>이 아직 같은 SHA로 있을 때만 착지 중으로 본다 — 끝난 착지
// (큐 참조가 지워짐·돌려보내짐)나 바꿔 낸 제출(SHA 다름)의 남은 표식은 아니다. hc-lane land(제출)와 hc-fill(자동 다시 맞춤)이 함께 쓴다.
package landing

import "strings"

// Prefix: 착지 중 표식 참조 머리
const Prefix = "refs/herdr/landing/"

const queue = "refs/merge-queue/"

// Ref: git ls-remote 한 줄(참조 이름·SHA)
type Ref struct{ Name, SHA string }

// Parse: git ls-remote 출력(「<SHA>\t<참조>」 줄들)
func Parse(ls string) []Ref {
	var o []Ref
	for _, l := range strings.Split(ls, "\n") {
		if f := strings.Fields(l); len(f) == 2 {
			o = append(o, Ref{Name: f[1], SHA: f[0]})
		}
	}
	return o
}

// LaneOf: 큐 이름 <UTC초 10자리>-<레인>의 레인(꼴이 아니면 빈 값)
func LaneOf(qn string) string {
	if len(qn) < 12 || qn[10] != '-' || strings.Trim(qn[:10], "0123456789") != "" {
		return ""
	}
	return qn[11:]
}

// Of: 레인 lane의 제출 가운데 병합기가 지금 착지 중인 큐 이름(없으면 빈 값). refs에는 refs/merge-queue/*와 refs/herdr/landing/*가 함께 있어야 한다
func Of(refs []Ref, lane string) string {
	q := map[string]string{}
	for _, r := range refs {
		if n, ok := strings.CutPrefix(r.Name, queue); ok {
			q[n] = r.SHA
		}
	}
	for _, r := range refs {
		if n, ok := strings.CutPrefix(r.Name, Prefix); ok && LaneOf(n) == lane && lane != "" && q[n] == r.SHA {
			return n
		}
	}
	return ""
}
