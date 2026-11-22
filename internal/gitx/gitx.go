// Package gitx: git 부르기. 결과는 셋(있음·없음·모름) — 종료 코드를 늘 보고, 실패한 명령의 반쪽 출력은 돌려주지 않는다
// (2026-11-20 병합기 결함: git 종료 코드를 놓쳐 「모름」을 「없음」으로 읽음). 병합기 Go 판에서 끌어올렸다.
package gitx

import (
	"bytes"
	"io"
	"os/exec"
	"strconv"
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

// PatchID: 두 커밋 사이 변경(a → b 트리 차이)의 patch-id(--stable)와 종료 코드. 변경이 없으면(빈 diff이고 두 트리 ID가 같음 — Tree로
// 따로 확인, 다르면 1) 「-」. 0이 아니면 모름 —
// 상한에 걸렸으면 proc.TimeoutCode(부르는 쪽이 「다시」로 읽게, gofix M01), 빈·잘린·여러 줄 patch-id면 1.
// 감사 승인의 판정이다 — hc-lane land(승인 착지·제출)와 병합기 묶음(구성원별 승인 확인)이 같은 이 함수를 쓴다
func PatchID(stderr io.Writer, a, b string) (string, int) {
	d, rc := Out("", stderr, "--literal-pathspecs", "diff", "-U0", "--binary", a, b, "--")
	if rc != 0 {
		return "", rc
	}
	if d == "" {
		// 빈 diff는 두 트리가 같다고 따로 확인했을 때만 「변경 없음」(merger/R-202: diff가 빈 출력·0을 주자 실제 변경을 「-」로 읽었다)
		ta, rc := Tree("", a, stderr)
		if rc != 0 {
			return "", rc
		}
		tb, rc := Tree("", b, stderr)
		if rc != 0 {
			return "", rc
		}
		if ta != tb {
			return "", 1
		}
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

// ObjID: git 객체 이름 꼴(소문자 16진 40자 또는 64자)
func ObjID(s string) bool { return hexID(s) }

func hexID(s string) bool {
	return (len(s) == 40 || len(s) == 64) && strings.Trim(s, "0123456789abcdef") == ""
}

// Tree: 커밋의 트리 ID(rev-parse --verify <c>^{tree}). 40·64자 소문자 16진이 아니면 1, git 실패면 그 종료 코드(상한이면 TimeoutCode)
func Tree(dir, commit string, stderr io.Writer) (string, int) {
	t, rc := Out(dir, stderr, "rev-parse", "--verify", "--end-of-options", commit+"^{tree}")
	if rc != 0 {
		return "", rc
	}
	if !hexID(t) {
		return "", 1
	}
	return t, 0
}

// LsRemote: git [-C dir] ls-remote --exit-code <remote> <패턴…> — 참조 이름 → 객체 ID. 패턴은 정확한 이름 또는 「접두/*」(그 접두 아래 전부).
// 있음 = 종료 0이고 모든 줄이 정확히 「<40|64자 소문자 16진>\t<refs/…>\n」, 이름이 요청 패턴 안, 같은 이름이 한 번씩. 없음 = 종료 2
// (맞는 참조 없음)와 빈 출력. 그 밖(빈 출력+0·끝 LF 없음·잘린 줄·요청 밖 이름·128·124 …)은 모름 — 부분 결과는 돌려주지 않는다
// (재감사 R-212·R-214·R-220·R-201: 잘린 줄을 건너뛰고 나머지를 「그 표식 없음」으로 읽었다)
func LsRemote(dir, remote string, stderr io.Writer, patterns ...string) (map[string]string, State) {
	var b bytes.Buffer
	args := []string{"ls-remote", "--exit-code", remote}
	if dir != "" {
		args = append([]string{"-C", dir}, args...) // git -C(작업 폴더를 바꾸지 않는다 — 부르던 쪽 꼴 그대로)
	}
	switch rc := Run("", &b, stderr, append(args, patterns...)...); {
	case rc == 2 && b.Len() == 0:
		return map[string]string{}, No
	case rc != 0:
		return nil, Unknown
	}
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		return nil, Unknown
	}
	m := map[string]string{}
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		sha, name, ok := LsRow(l)
		if !ok || !matches(name, patterns) {
			return nil, Unknown
		}
		if _, dup := m[name]; dup {
			return nil, Unknown
		}
		m[name] = sha
	}
	return m, Yes
}

// LsRow: ls-remote 한 줄(끝 줄바꿈 뺀) 「<40|64자 소문자 16진>\t<refs/…>」의 SHA·이름. 꼴이 틀리면 ok=false(landing.Parse도 이것으로)
func LsRow(l string) (sha, name string, ok bool) {
	sha, name, ok = strings.Cut(l, "\t")
	if !ok || !hexID(sha) || !refName(name) {
		return "", "", false
	}
	return sha, name, true
}

// refName: refs/ 아래 이름 — 빈칸·제어 문자 없음
func refName(s string) bool {
	if !strings.HasPrefix(s, "refs/") || len(s) == len("refs/") {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// matches: 이름이 요청 패턴 하나와 맞나(정확한 이름 또는 「접두/*」). 패턴이 없으면 모든 refs/ 이름
func matches(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if pre, ok := strings.CutSuffix(p, "*"); ok && strings.HasSuffix(pre, "/") {
			if strings.HasPrefix(name, pre) && len(name) > len(pre) {
				return true
			}
		} else if name == p {
			return true
		}
	}
	return false
}

// SplitZ: NUL로 끝나는 레코드 목록(-z 출력). 출력 끝이 NUL이고 빈 레코드가 없을 때만 ok. 빈 출력은 레코드 0개 ok
func SplitZ(out string) ([]string, bool) {
	if out == "" {
		return []string{}, true
	}
	if !strings.HasSuffix(out, "\x00") {
		return nil, false
	}
	r := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for _, e := range r {
		if e == "" {
			return nil, false
		}
	}
	return r, true
}

// Int: 출력 전체가 10진 정수 하나(끝 줄바꿈 하나만 허용) — 부호·빈칸·빈 값·여러 줄·넘침은 거절
func Int(out string) (int64, bool) {
	s := strings.TrimSuffix(out, "\n")
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}
