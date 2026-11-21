package pyjson

import (
	"os/exec"
	"strings"
	"testing"
)

// 골든은 파이썬 json.dumps 실제 결과(python3가 있으면 그 자리에서 견준다)
func TestDumpsLikePython(t *testing.T) {
	o := Obj{{"tab", "w1/탭"}, {"n", nil}, {"workers", []string{"a\"b", "c\\d\n\t\x01", "😀<&>"}}, {"x", []any{true, false}}}
	u := `\` + "u" // 이 꼴로 둔다(편집 도구가 역슬래시-u를 풀어 버린다)
	asc := `{"tab": "w1/` + u + `d0ed", "n": null, "workers": ["a\"b", "c\\d\n\t\u0001", "` + u + `d83d` + u + `de00<&>"], "x": [true, false]}`
	raw := `{"tab": "w1/탭", "n": null, "workers": ["a\"b", "c\\d\n\t\u0001", "😀<&>"], "x": [true, false]}`
	if g := Dumps(o, true); g != asc {
		t.Fatalf("ascii:\n%s\n%s", g, asc)
	}
	if g := Dumps(o, false); g != raw {
		t.Fatalf("raw:\n%s\n%s", g, raw)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Log("python3 없음 — 골든만 봄")
		return
	}
	src := `import json; o={"tab":"w1/탭","n":None,"workers":["a\"b","c\\d\n\t\x01","😀<&>"],"x":[True,False]}; print(json.dumps(o)); print(json.dumps(o, ensure_ascii=False))`
	out, err := exec.Command(py, "-I", "-c", src).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := asc + "\n" + raw + "\n"; string(out) != want {
		t.Fatalf("파이썬과 다름:\n%s\n%s", out, want)
	}
}

func TestDecodeKeepsNumber(t *testing.T) {
	var v map[string]any
	if err := Decode([]byte(`{"p": 12, "q": 1.50}`), &v); err != nil {
		t.Fatal(err)
	}
	if g := Dumps(v, true); g != `{"p": 12, "q": 1.50}` {
		t.Fatal(g)
	}
	if err := Decode([]byte(`{} {}`), &v); err == nil || !strings.Contains(err.Error(), "다른 값") {
		t.Fatal("뒤에 붙은 값은 오류여야", err)
	}
	// 파이썬 json.loads 실제 결과: '{}]'·'{}\x0c'·'{} x'는 Extra data, '{} \n\t\r'는 됨
	for s, ok := range map[string]bool{"{}]": false, "[1]]": false, "{}\x0c": false, "{} x": false, "{} \n\t\r": true} {
		if err := Decode([]byte(s), &v); (err == nil) != ok {
			t.Errorf("%q: 됨 %v 이어야, 오류 %v", s, ok, err)
		}
	}
}

// O01: ensure_ascii는 0x7f도 \u007f(파이썬 json.dumps(["a\x7fb"]) 실제 결과 ["a\u007fb"]), 아니면 그대로
func TestDumpsDel(t *testing.T) {
	u := `\` + "u"
	if g := Dumps([]string{"a\x7fb"}, true); g != `["a`+u+`007fb"]` {
		t.Fatalf("ascii: %q", g)
	}
	if g := Dumps([]string{"a\x7fb"}, false); g != "[\"a\x7fb\"]" {
		t.Fatalf("raw: %q", g)
	}
}
