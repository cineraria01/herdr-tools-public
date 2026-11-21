package main

// 파이썬 흉내(splitlines·repr·strip)를 파이썬 실제 결과와 견준다. 나누기·대조 전체는 git archive 사본에서 hc-split.py와 견줬다
// (2026-11-21 util-go: check 97fff7a @ e5782f4 두 판 출력 같음, split 97fff7a 결과 트리 = e5782f4).

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestPythonHelpers(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음 — 파이썬과 견줄 수 없다(이 도구는 hc-split.py와 견주는 개발용이라 python3가 있어야 한다)")
	}
	ins := []string{"a\nb\r\nc\rd\ve\x1cf g", "x\n", "", "\n\n", "it's", `say "hi"`, `both ' "`, "\t탭\x01\x7f ​", " \x1f　 "}
	src := `import json,sys
for s in json.load(sys.stdin):
    print(json.dumps([s.splitlines(True), repr(s), s.strip()]))`
	in, _ := json.Marshal(ins)
	c := exec.Command(py, "-I", "-c", src)
	c.Stdin = strings.NewReader(string(in))
	out, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	for i, ln := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		var want []any
		if err := json.Unmarshal([]byte(ln), &want); err != nil {
			t.Fatal(err)
		}
		gotLines, _ := json.Marshal(append([]string{}, splitlines(ins[i])...))
		wantLines, _ := json.Marshal(want[0])
		if string(gotLines) != string(wantLines) {
			t.Errorf("splitlines %q: %s != %s", ins[i], gotLines, wantLines)
		}
		if g := pyRepr(ins[i]); g != want[1] {
			t.Errorf("repr %q: %s != %s", ins[i], g, want[1])
		}
		if g := strip(ins[i]); g != want[2] {
			t.Errorf("strip %q: %q != %q", ins[i], g, want[2])
		}
	}
}
