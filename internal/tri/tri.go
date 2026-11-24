// Package tri: 있음·없음·모름 세 값. 0값이 모름이라 깜빡 빠뜨린 대입·return이 「있음」이나 「없음」이 되지 않는다
// (2026-11-24 tri-state-type — 0·1·2 정수 꼴은 0이 「있음」이어서 `return 0` 하나가 곧 있음이었다).
// 세 값으로 모자란 판정(끝남 확인·기계 모름 등)은 그 도구 안에 뜻이 이름에 드러나는 형을 따로 둔다 — 그 형도 0값은 모름.
// 이 형을 고르는 switch는 세 값을 다 적거나 default를 두고, 이 형 자리에 정수 리터럴을 쓰지 않는다
// (tests/repo의 tri_guard_test.go·tri_literal_test.go가 저장소 전체를 본다).
package tri

// State: 판정 결과
type State int

const (
	Unknown State = iota // 모름: 조회가 실패했다·응답이 온전하지 않다 — 「없음」으로 읽지 않는다
	Yes                  // 있음
	No                   // 없음: 명시적으로 없다고 확인했다
)

func (s State) String() string {
	switch s {
	case Yes:
		return "있음"
	case No:
		return "없음"
	default:
		return "모름"
	}
}

// Of: 확인을 마친 참·거짓을 있음·없음으로(확인하지 못했으면 부르는 쪽이 Unknown을 돌려준다)
func Of(b bool) State {
	if b {
		return Yes
	}
	return No
}
