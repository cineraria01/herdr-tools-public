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

// PatchID: 두 커밋 사이 변경(a → b 트리 차이)의 patch-id(--stable)와 종료 코드. 변경이 없으면 「-」. 0이 아니면 모름 —
// 상한에 걸렸으면 proc.TimeoutCode(부르는 쪽이 「다시」로 읽게, gofix M01), 빈·잘린·여러 줄 patch-id면 1.
// 감사 승인의 판정이다 — hc-lane land(승인 착지·제출)와 병합기 묶음(구성원별 승인 확인)이 같은 이 함수를 쓴다
func PatchID(stderr io.Writer, a, b string) (string, int) {
	d, rc := Out("", stderr, "--literal-pathspecs", "diff", "-U0", "--binary", a, b, "--")
	if rc != 0 {
		return "", rc
	}
	if d == "" {
		return "-", 0
	}
	var o bytes.Buffer
	c := exec.Command("git", "patch-id", "--stable")
	c.Stdin, c.Stdout, c.Stderr = strings.NewReader(d+"\n"), &o, stderr
	if rc := proc.Code(proc.Run(c)); rc != 0 {
		return "", rc
	}
	// 결과는 「<patch-id> <커밋 id>」 한 줄 — 두 칸 모두 같은 길이(SHA-1 40·SHA-256 64)의 16진수. 잘린 지문·여러 줄·빈 출력은 모름(1)
	// (R-22: 세 글자 지문이 승인으로 큐·메타 ref에 기록됐다)
	line := strings.TrimRight(o.String(), "\n")
	f := strings.Fields(line)
	if strings.Contains(line, "\n") || len(f) != 2 || !hexID(f[0]) || len(f[1]) != len(f[0]) || !hexID(f[1]) {
		return "", 1
	}
	return f[0], 0
}

// hexID: git 객체 이름 꼴(소문자 16진 40자 또는 64자)
func hexID(s string) bool {
	return (len(s) == 40 || len(s) == 64) && strings.Trim(s, "0123456789abcdef") == ""
}
