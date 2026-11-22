package verified

import "strings"

// 검증 기계 목록(추측 검증의 켬 표식): 원격 참조 VerifiersRef → 빈 트리 커밋, 제목 「hc-spec-verifiers」, 빈 줄 뒤 줄마다 「<기계>\t<클론 경로>」.
// hc-fill이 설정 줄 「merger_verifiers <프로젝트> <기계,…>」와 그 기계의 repo 줄로 쓰고(날짜를 0으로 고정해 같은 목록이면 같은 SHA),
// 병합기가 읽는다. 참조가 없으면 추측 없음
const (
	VerifiersRef  = "refs/herdr/spec-verifiers"
	verifiersHead = "hc-spec-verifiers"
)

// Verifier: 검증 기계 하나
type Verifier struct{ Machine, Path string }

// FormatVerifiers: 목록 커밋의 메시지
func FormatVerifiers(vs []Verifier) string {
	var b strings.Builder
	b.WriteString(verifiersHead + "\n\n")
	for _, v := range vs {
		b.WriteString(v.Machine + "\t" + v.Path + "\n")
	}
	return b.String()
}

// MachineOK: 기계 이름 글자(a-z 0-9 _ -, 32자까지, 맨 앞 - 아님)
func MachineOK(m string) bool {
	return m != "" && m[0] != '-' && len(m) <= 32 && strings.Trim(m, "abcdefghijklmnopqrstuvwxyz0123456789_-") == ""
}

// ParseVerifiers: 목록 커밋의 메시지를 읽는다. 꼴이 틀린 줄은 건너뛴다(경로는 / 로 시작)
func ParseVerifiers(msg string) []Verifier {
	ls := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	if len(ls) == 0 || ls[0] != verifiersHead {
		return nil
	}
	var vs []Verifier
	for _, l := range ls[1:] {
		m, p, ok := strings.Cut(l, "\t")
		if ok && MachineOK(m) && strings.HasPrefix(p, "/") {
			vs = append(vs, Verifier{m, p})
		}
	}
	return vs
}
