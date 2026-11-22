//go:build !windows

package main

// gofix3-shx: 기계 조회(q)·가져오기(--pull 나무 묶음)의 「모름」을 「없음·성공」으로 읽지 않는다.
//   - ops/R-206: 기계 base.tsv를 못 읽으면(awk: can't open) 일반 줄·나무 줄의 base 조회가 빈 이력처럼 지나가 「같음」·종료 0이었다
//   - ops/R-207: 성공으로 끝난 hc 응답이 「O<TAB>Darw」에서 잘렸는데 「건너뜀 (OS Darw)」·종료 0이었다
//   - ops/R-217: 원격 나무 전송 T가 tar를 다 낸 뒤 255로 끝났는데 호출자가 종료 코드를 버려 맥 파일을 기계 값으로 바꿨다
// 조각은 진짜 sh·dash(리눅스 기계 흉내)로 돌린다. 시험이 띄운 자식만 쓴다(Output·pipeline이 회수)

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/cksum"
	"herdr-tools/internal/shx"
)

func shells() []string {
	s := []string{"/bin/sh"}
	if _, err := os.Stat("/bin/dash"); err == nil {
		s = append(s, "/bin/dash")
	}
	return s
}

// runQ: 그 셸로 조회 조각(agentQ)을 돌려 Cut한 본문과 ok
func runQ(t *testing.T, sh, home, hsb string, args ...string) (string, bool) {
	t.Helper()
	c := exec.Command(sh, append([]string{"-c", agentQ, "_", "q"}, args...)...)
	c.Env = append(os.Environ(), "HOME="+home, "HSB="+hsb, "TMPDIR="+t.TempDir())
	out, err := c.Output()
	rc := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		rc = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return shx.Cut(string(out), rc)
}

func TestQueryBaseUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root는 권한 없는 파일도 읽는다")
	}
	for _, sh := range shells() {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			h := t.TempDir()
			ct := "a1\n"
			if err := os.MkdirAll(h+"/.claude/docs", 0o755); err != nil {
				t.Fatal(err)
			}
			os.MkdirAll(h+"/skills", 0o755)
			os.WriteFile(h+"/.claude/docs/a.md", []byte(ct), 0o644)
			os.WriteFile(h+"/skills/s.md", []byte("s\n"), 0o644)
			sig := cksum.Dot([]byte(ct)) + ".-"
			st := h + "/.local/state/herdr-sync"
			os.MkdirAll(st, 0o755)
			b := st + "/base.tsv"
			os.WriteFile(b, []byte(".claude/docs/a.md\t"+sig+"\nskills/s.md\tx\n"), 0o644)
			gen := []string{".claude/docs/a.md", "--", "--", "--", "--"}
			tree := []string{"--", "--", "skills", "--", "--"}
			// 정상: 일반 줄 base·나무 줄 B
			if out, ok := runQ(t, sh, h, b, gen...); !ok || !strings.Contains(out, "U\t"+sig+"\t"+sig+"\n") {
				t.Fatalf("정상 일반 줄: ok=%v\n%s", ok, out)
			}
			if out, ok := runQ(t, sh, h, b, tree...); !ok || !strings.Contains(out, "B\tskills/s.md\tx\n") {
				t.Fatalf("정상 나무 줄: ok=%v\n%s", ok, out)
			}
			// 없음 확정: 빈 이력(-)
			if out, ok := runQ(t, sh, h, st+"/없음.tsv", gen...); !ok || !strings.Contains(out, "U\t"+sig+"\t-\n") {
				t.Fatalf("base 없음: ok=%v\n%s", ok, out)
			}
			// 못 읽음: 일반 줄·나무 줄 모두 모름
			os.Chmod(b, 0)
			defer os.Chmod(b, 0o644)
			for name, a := range map[string][]string{"일반 줄": gen, "나무 줄": tree} {
				if out, ok := runQ(t, sh, h, b, a...); ok {
					t.Errorf("base 못 읽음(%s)인데 조회 성공:\n%s", name, out)
				}
			}
			os.Chmod(b, 0o644)
			// 위 폴더를 뒤질 수 없음: 있는지 모름
			os.Chmod(st, 0)
			defer os.Chmod(st, 0o755)
			if out, ok := runQ(t, sh, h, b, gen...); ok {
				t.Errorf("base 위 폴더를 못 뒤지는데 조회 성공:\n%s", out)
			}
		})
	}
}

func TestParseQClipped(t *testing.T) {
	oldU := U
	t.Cleanup(func() { U = oldU })
	U = nil
	m := shx.Mark + "\n"
	for _, c := range []struct {
		name, out, os string
		ok            bool
	}{
		{"표식 앞에서 잘림 — ops/R-207", "H\t/h\nO\tDarw", "", false},
		{"온전한 Darwin", "H\t/h\nO\tDarwin\n" + m, "Darwin", true},
		{"온전한 비지원 OS(정상 건너뜀)", "H\t/h\nO\tMINGW64_NT-10.0\n" + m, "MINGW64_NT-10.0", true},
		{"O 줄 없음", "H\t/h\n" + m, "", false},
		{"O 줄 둘", "H\t/h\nO\tDarwin\nO\tLinux\n" + m, "", false},
		{"모르는 줄", "Z\tx\nH\t/h\nO\tDarwin\n" + m, "", false},
		{"칸 수 틀림", "H\t/h\nO\tDar\twin\n" + m, "", false},
		{"남는 U 줄", "U\ta\tb\nH\t/h\nO\tDarwin\n" + m, "", false},
	} {
		q, ok := parseQ(c.out)
		if ok != c.ok || ok && q.O != c.os {
			t.Errorf("%s: parseQ ok=%v O=%q, want ok=%v O=%q", c.name, ok, q.O, c.ok, c.os)
		}
	}
	// 진짜 조각의 온전한 응답은 받는다
	for _, sh := range shells() {
		out, ok := runQ(t, sh, t.TempDir(), "-", "--", "--", "--", "--")
		if !ok {
			t.Fatalf("%s 조회 조각 실패", sh)
		}
		if q, ok := parseQ(out + m); !ok || q.O == "" {
			t.Errorf("%s 온전한 응답을 못 받음: %q", sh, out)
		}
	}
}

// 가짜 hc: exec … -- <명령>을 기계 폴더 HOME으로 돌리고, 성공이면 FAKE_RC로 끝난다(전송을 다 낸 뒤 255 흉내)
const fakeHCPull = `#!/bin/sh
while [ "$1" != -- ]; do shift; done; shift
HOME=$FAKE_M "$@" || exit $?
exit ${FAKE_RC:-0}
`

func TestPullTreeProducerFails(t *testing.T) {
	oldT, oldS := TMP, STAMP
	t.Cleanup(func() { TMP, STAMP = oldT, oldS })
	TMP, STAMP = t.TempDir(), "t"
	bin, mac, w := t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(bin+"/hc", []byte(fakeHCPull), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", mac)
	t.Setenv("FAKE_M", w)
	k := "skills/a/SKILL.md"
	for d, s := range map[string]string{mac: "mac\n", w: "from worker\n"} {
		os.MkdirAll(d+"/skills/a", 0o755)
		os.WriteFile(d+"/"+k, []byte(s), 0o644)
	}
	c := cksum.Dot([]byte("from worker\n")) + ".-"
	pl := []string{k + "\t-\t" + c + "\t" + k}
	t.Setenv("FAKE_RC", "255")
	ok, bad := pullTree("w1", pl)
	if len(ok) != 0 {
		t.Errorf("원격 전송 255인데 가져옴: %v", ok)
	}
	// ops/R-3A23: 받기 실패를 알리는 줄이 빠지면(bad 없음) 진입점이 「같음」·종료 0으로 끝난다 — 그 키 하나와 양쪽 종료 코드
	if len(bad) != 1 || bad[0][0] != k || !strings.Contains(bad[0][1], "보냄 255 · 받음 0") {
		t.Errorf("원격 전송 255: bad %v, 기대 [%s 「보냄 255 · 받음 0」]", bad, k)
	}
	if b, _ := os.ReadFile(mac + "/" + k); string(b) != "mac\n" {
		t.Fatalf("원격 전송 255인데 맥 파일이 바뀌었다: %q", b)
	}
	t.Setenv("FAKE_RC", "0")
	if ok, bad := pullTree("w1", pl); len(ok) != 1 || len(bad) != 0 {
		t.Fatalf("정상 전송: ok %v bad %v", ok, bad)
	}
	if b, _ := os.ReadFile(mac + "/" + k); string(b) != "from worker\n" {
		t.Fatalf("정상 전송인데 맥 파일: %q", b)
	}
}
