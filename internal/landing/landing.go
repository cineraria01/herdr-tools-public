// Package landing: 「그 레인이 지금 병합기에서 착지 중인가」 판정(2026-11-22 reland-live-lane — 한 프로젝트의 병합기가 sv-go-verify-lock 제출을
// 20분 검증한 뒤, 그 사이 같은 레인의 옛 커밋이 다시 제출돼 큐 참조가 바뀌어 push가 lease로 멈췄다).
// 병합기(cmd/hc-merger)는 착지를 부르기 직전 묶음의 제출마다 원격에 표식 refs/herdr/landing/<큐 이름>(= 그 제출 SHA)을 올리고, 다음 바퀴
// 시작에 남은 표식을 지운다. 표식은 그 큐 참조 refs/merge-queue/<큐 이름>이 아직 같은 SHA로 있을 때만 착지 중으로 본다 — 끝난 착지
// (큐 참조가 지워짐·돌려보내짐)나 바꿔 낸 제출(SHA 다름)의 남은 표식은 아니다. hc-lane land(제출)와 hc-fill(자동 다시 맞춤)이 함께 쓴다.
// hc-lane 재제출은 갈아 끼우는 큐의 표식 자리에 새 기록 커밋을 본 값 lease로 같은 원자 push에 써(갈아 끼움 표식 — 큐 SHA가 아니라
// 착지 중이 아님), 조회 뒤 병합기가 먼저 표식을 올렸으면 서버가 push 전체를 거절한다(재감사 lane/R-213·R-214).
package landing

import (
	"sort"
	"strings"

	"herdr-tools/internal/gitx"
)

// Prefix: 착지 중 표식 참조 머리
const Prefix = "refs/herdr/landing/"

const queue = "refs/merge-queue/"

// Ref: git ls-remote 한 줄(참조 이름·SHA)
type Ref struct{ Name, SHA string }

// Parse: git ls-remote 출력(「<SHA>\t<참조>」 줄들, 끝 줄바꿈은 있어도 없어도 — gitx.Out은 뗀다). 줄마다 gitx.LsRemote와 같은 규칙
// (gitx.LsRow): 한 줄이라도 어긋나거나 같은 이름이 두 번이면 ok=false — 잘린 줄을 건너뛰고 「표식 없음」으로 읽지 않는다(재감사 lane/R-212).
// 출력 끝이 잘렸는지(끝 LF)는 문자열만으로 모른다 — 원격 조회는 gitx.LsRemote로 하고 Refs로 바꾼다
func Parse(ls string) ([]Ref, bool) {
	o := []Ref{}
	if ls == "" {
		return o, true
	}
	seen := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSuffix(ls, "\n"), "\n") {
		sha, name, ok := gitx.LsRow(l)
		if !ok || seen[name] {
			return nil, false
		}
		seen[name] = true
		o = append(o, Ref{Name: name, SHA: sha})
	}
	return o, true
}

// Refs: gitx.LsRemote의 맵(이름 → SHA)을 이름순 목록으로
func Refs(m map[string]string) []Ref {
	o := make([]Ref, 0, len(m))
	for n, s := range m {
		o = append(o, Ref{Name: n, SHA: s})
	}
	sort.Slice(o, func(i, j int) bool { return o[i].Name < o[j].Name })
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
