//go:build !windows

package shx

// 셸 조각 계약 시험(gofix3-shx ②-E): 실제 /bin/sh로 Script를 돌리고 Cut이 성공 경로 끝 표식으로만 ok를 내는지 본다.
// 계약 2(각 단계는 `|| exit N`으로 명시, set -e를 쓰지 않음)대로 파이프 줄에는 `|| exit`을 붙인다 — 머리의 pipefail이 없으면
// 같은 줄이 성공으로 지나간다는 것을 plain 칸(머리 없는 sh)으로 함께 확인한다.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// shRun: sh -c script — 표준 출력과 종료 코드(시험이 띄운 자식만, Output이 끝까지 기다린다)
func shRun(t *testing.T, sh, script string, env ...string) (string, int) {
	t.Helper()
	c := exec.Command(sh, "-c", script, "_")
	c.Env = append(os.Environ(), env...)
	out, err := c.Output()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &ee):
		return string(out), ee.ExitCode()
	}
	t.Fatalf("%s 못 띄움: %v", sh, err)
	return "", -1
}

func TestScriptContract(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name, body string
		wantBody   string // ok일 때 본문
		ok         bool
		rcNZ       bool // rc≠0이어야 한다
		plainRC0   bool // 머리 없는 sh에서는 rc 0으로 지나간다(머리가 막는 것)
	}{
		{"printf", `printf 'a\n'`, "a\n", true, false, false},
		{"앞 단계 실패 파이프", `false | cat || exit 3`, "", false, true, true},
		{"find 실패를 sed가 가림", `t=$(mktemp "$D/x.XXXXXX") || exit 2
find "$D/없는곳" | sed s/x/y/ >"$t" || exit 2`, "", false, true, true},
		{"마지막 단계 1", "echo a\n" + `[ -n "" ] && echo x`, "a\n", true, false, false},
		{"중간 exit 0", "echo a\nexit 0\necho b", "", false, false, false},
		{"미정의 변수", `echo "$hc_shx_nope"`, "", false, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, rc := shRun(t, "/bin/sh", Script(c.body), "D="+tmp)
			b, ok := Cut(out, rc)
			if ok != c.ok || (ok && b != c.wantBody) || (c.rcNZ && rc == 0) {
				t.Errorf("Script(%q): rc %d out %q → Cut (%q, %v), want ok=%v body %q rc≠0=%v", c.body, rc, out, b, ok, c.ok, c.wantBody, c.rcNZ)
			}
			if c.plainRC0 {
				if _, prc := shRun(t, "/bin/sh", c.body, "D="+tmp); prc != 0 {
					t.Errorf("머리 없는 sh에서 rc %d — 이 경우가 머리를 시험하지 못한다", prc)
				}
			}
		})
	}
}

func TestCutContract(t *testing.T) {
	cases := []struct {
		name, out string
		rc        int
		body      string
		ok        bool
	}{
		{"표식", "a\n" + Mark + "\n", 0, "a\n", true},
		{"표식만(빈 본문)", Mark + "\n", 0, "", true},
		{"끝 LF를 뗀 출력(부르는 쪽이 끝 줄바꿈을 뺌)", "a\n" + Mark, 0, "a\n", true},
		{"표식 앞에서 잘림 — ops/R-207", "H\t/h\nO\tDarw", 0, "", false},
		{"표식 뒤에 줄이 더 있음", "a\n" + Mark + "\nb\n", 0, "", false},
		{"표식 뒤에 빈 줄", "a\n" + Mark + "\n\n", 0, "", false},
		{"rc 1 + 표식", "a\n" + Mark + "\n", 1, "", false},
		{"표식이 줄 가운데", "a" + Mark + "\n", 0, "", false},
		{"빈 출력", "", 0, "", false},
	}
	for _, c := range cases {
		if b, ok := Cut(c.out, c.rc); ok != c.ok || b != c.body {
			t.Errorf("%s: Cut(%q, %d) = (%q, %v), want (%q, %v)", c.name, c.out, c.rc, b, ok, c.body, c.ok)
		}
	}
}

// Tail: 머리 없이 표식만 — 마지막 단계 1은 ok, 중간 종료·0 아닌 exit는 모름. pipefail 없는 셸(가짜 sh)에서도 돈다
func TestTail(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "sh")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ns=$(printf '%s' \"$2\" | sed 's/pipefail/pipefail_x/g'); shift 2; exec /bin/sh -c \"$s\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		body string
		ok   bool
	}{{"echo a\n[ -n \"\" ] && echo x", true}, {"echo a\nexit 0", false}, {"echo a\nfalse || exit 4", false}} {
		for _, sh := range []string{"/bin/sh", fake} {
			out, rc := shRun(t, sh, Tail(c.body))
			if b, ok := Cut(out, rc); ok != c.ok || ok && b != "a\n" {
				t.Errorf("%s Tail(%q): rc %d → (%q, %v), want ok=%v", sh, c.body, rc, b, ok, c.ok)
			}
		}
	}
}

// pipefail을 못 켜는 셸: 진짜 sh에 넘기기 전에 pipefail 글자를 모르는 옵션 이름으로 바꾼다(옛 dash·posh 흉내) — 97(모름)
func TestNoPipefail(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "sh")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ns=$(printf '%s' \"$2\" | sed 's/pipefail/pipefail_x/g'); shift 2; exec /bin/sh -c \"$s\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, rc := shRun(t, fake, Script(`printf 'a\n'`))
	if b, ok := Cut(out, rc); rc != 97 || ok {
		t.Errorf("pipefail 못 켬: rc %d out %q → (%q, %v), want rc 97 ok=false", rc, out, b, ok)
	}
}
