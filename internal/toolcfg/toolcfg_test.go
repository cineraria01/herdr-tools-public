package toolcfg

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(d+"/config", []byte("# c\n a.b  x  y \r\nc.d\nBad.Key z\n"), 0o644)
	if v, ok, err := Get(d, "a.b"); err != nil || !ok || v != "x  y" {
		t.Errorf("a.b = %q %v %v", v, ok, err)
	}
	if v, ok, _ := Get(d, "c.d"); !ok || v != "" {
		t.Errorf("키 혼자 = %q %v", v, ok)
	}
	if _, ok, _ := Get(d, "e.f"); ok {
		t.Errorf("없는 키가 있다고 함")
	}
	os.WriteFile(d+"/config", []byte("a.b 1\nc.d 2\na.b 3\n"), 0o644)
	var dup *ErrDup
	if _, _, err := Get(d, "c.d"); !errors.As(err, &dup) {
		t.Errorf("같은 키 두 번이 오류가 아님: %v", err)
	}
}

// B14·M5: 셸 판(LC_ALL=C grep -E -i, 바이트 단위)과 같게 건다. want는 bin/herdr-tools-config를 HERDR_TOOLS_CONFIG_IMPL=bash로 부른 실제 종료 코드.
// 저장소 안이면(bin/herdr-tools-config가 있으면) 셸 판을 그 자리에서 함께 불러 견준다
func TestLintBytesLikeShell(t *testing.T) {
	cases := []struct {
		pat, line string
		want      int
	}{
		{`\<tok\>`, "a tok b", 1}, {`\<tok\>`, "atokb", 0}, {`a...b`, "a한b", 1},
		{`[[:<:]]tok[[:>:]]`, "x tok y", 1}, {`[[:<:]]tok[[:>:]]`, "xtoky", 0}, {`^예...끝$`, "예한끝", 1},
		{`TOK`, "tok", 1}, {`é`, "É", 0}, {`x[가]y`, "x\xb0y", 1}, {`a.b`, "a\xffb", 1},
		{`[\.]x`, `\x`, 1}, {`a\.b`, "axb", 0}, {`\\<x`, `\<x`, 1}, {`\<tok`, "한tok", 1}, {`tok\>`, "tok한", 1},
		{`[^a]b`, "\xffb", 1},
	}
	shell := ""
	if wd, err := os.Getwd(); err == nil {
		if p := filepath.Join(wd, "../../bin/herdr-tools-config"); isFile(p) {
			shell = p
		}
	}
	if shell == "" {
		t.Log("bin/herdr-tools-config 없음(깔린 곳) — 기록한 셸 판 결과와만 견준다")
	}
	for _, c := range cases {
		d, base := t.TempDir(), t.TempDir()
		os.WriteFile(d+"/private-patterns.txt", []byte(c.pat+"\n"), 0o644)
		os.WriteFile(base+"/f.txt", []byte(c.line+"\n"), 0o644)
		var out, errb bytes.Buffer
		if rc := LintAt(base, d, false, []string{"f.txt"}, &out, &errb); rc != c.want {
			t.Errorf("패턴 %q 줄 %q: Go %d, 셸 %d (%s)", c.pat, c.line, rc, c.want, errb.String())
		}
		if shell == "" {
			continue
		}
		cmd := exec.Command("bash", shell, "lint", "f.txt")
		cmd.Dir = base
		cmd.Env = append(os.Environ(), "HERDR_TOOLS_CONFIG="+d, "HERDR_TOOLS_CONFIG_IMPL=bash")
		err := cmd.Run()
		rc := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			rc = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if rc != c.want {
			t.Errorf("셸 판이 기록과 다르다: 패턴 %q 줄 %q: %d, 기록 %d", c.pat, c.line, rc, c.want)
		}
	}
}

func TestLintAt(t *testing.T) {
	c, base := t.TempDir(), t.TempDir()
	os.WriteFile(c+"/private-patterns.txt", []byte("# 주석\nsecret-[0-9]+\n"), 0o644)
	os.MkdirAll(filepath.Join(base, "d"), 0o755)
	os.WriteFile(filepath.Join(base, "d/a.txt"), []byte("ok\nSECRET-1 here\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := LintAt(base, c, false, []string{"d/a.txt"}, &out, &errb); rc != 1 || out.String() != "d/a.txt:2: secret-[0-9]+\n" {
		t.Errorf("LintAt rc=%d out=%q err=%q", rc, out.String(), errb.String())
	}
	out.Reset()
	if rc := LintAt(base, c, false, []string{"d/없음"}, &out, &errb); rc != 2 {
		t.Errorf("못 읽는 파일은 2: %d", rc)
	}
	if !HasPatterns(c+"/private-patterns.txt") || HasPatterns(c+"/없음") {
		t.Errorf("HasPatterns")
	}
}
