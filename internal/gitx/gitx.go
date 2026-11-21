// Package gitx: git 부르기. 결과는 셋(있음·없음·모름) — 종료 코드를 늘 보고, 실패한 명령의 반쪽 출력은 돌려주지 않는다
// (2026-11-20 병합기 결함: git 종료 코드를 놓쳐 「모름」을 「없음」으로 읽음). 병합기 Go 판에서 끌어올렸다.
package gitx

import (
	"bytes"
	"io"
	"os/exec"
	"strings"

	"herdr-tools/internal/proc"
)

// State: 조회 결과
type State int

const (
	Unknown State = iota // 모름: git이 실패했다 — 그 바퀴는 아무것도 바꾸지 않는다
	Yes                  // 있음
	No                   // 없음: git이 명시적으로 없다고 했다
)

func (s State) String() string {
	switch s {
	case Yes:
		return "있음"
	case No:
		return "없음"
	}
	return "모름"
}

// Run: git을 dir(빈 값이면 지금 폴더)에서. stdout·stderr가 nil이면 버린다. 표준 입력은 /dev/null. 셸 종료 코드를 돌려준다
func Run(dir string, stdout, stderr io.Writer, args ...string) int {
	c := exec.Command("git", args...)
	c.Dir, c.Stdout, c.Stderr = dir, stdout, stderr
	return proc.Code(proc.Run(c)) // 상한(proc.Limit) — 넘으면 TimeoutCode(모름)
}

// Out: $(git …) — 표준 출력(끝 줄바꿈 뺌)과 종료 코드. 종료 코드가 0이 아니면 출력은 빈 값(반쪽 출력을 믿지 않는다)
func Out(dir string, stderr io.Writer, args ...string) (string, int) {
	var b bytes.Buffer
	if rc := Run(dir, &b, stderr, args...); rc != 0 {
		return "", rc
	}
	return strings.TrimRight(b.String(), "\n"), 0
}

// Probe: 출력·종료 코드를 셋으로. 0이고 출력이 있으면 있음, 0이고 빈 출력이거나 종료 코드가 absent 가운데 하나면 없음
// (예: ls-remote 빈 출력, rev-parse -q --verify 종료 1), 그 밖은 모름
func Probe(out string, rc int, absent ...int) State {
	if rc == 0 {
		if out != "" {
			return Yes
		}
		return No
	}
	for _, a := range absent {
		if rc == a {
			return No
		}
	}
	return Unknown
}
