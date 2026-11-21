package ojson

import (
	"os/exec"
	"strings"
	"testing"
)

var samples = []string{
	`[1.0,1e2,-0,100000000000000000000,1.5e-7,1E+2,0.10,0.000001,0.0000001,123.456e1,-12e-3,0,0.0,7]`,
	`"a\u007fb<>& é\u0001\/\t\b\f\r\n\"\\"`,
	`{"b":{"z":[1,{}],"a":[]},"a":"x","c":null,"d":true,"e":false,"한":"글"}`,
	`{"b":1,"a":2,"b":3}`,
	`[]`, `{}`, `"x"`, `null`,
}

func TestMatchesJq(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq 없음")
	}
	jq := func(arg, in string) string {
		c := exec.Command("jq", arg, ".")
		c.Stdin = strings.NewReader(in)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("jq %s %s: %v", arg, in, err)
		}
		return string(out)
	}
	for _, s := range samples {
		v, err := Parse([]byte(s))
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if got, want := Canon(v)+"\n", jq("-cS", s); got != want {
			t.Errorf("Canon %s\n 얻음 %s 바람 %s", s, got, want)
		}
		if got, want := Compact(v)+"\n", jq("-c", s); got != want {
			t.Errorf("Compact %s\n 얻음 %s 바람 %s", s, got, want)
		}
		if got, want := Pretty(v), jq("-M", s); got != want {
			t.Errorf("Pretty %s\n 얻음 %s 바람 %s", s, got, want)
		}
	}
}

func TestPaths(t *testing.T) {
	v, _ := Parse([]byte(`{"a":{"b":1},"s":"x"}`))
	if g, _ := GetPath(v, []string{"a", "b"}); Canon(g) != "1" {
		t.Errorf("GetPath a.b = %s", Canon(g))
	}
	if g, err := GetPath(v, []string{"n", "m"}); err != nil || g != nil {
		t.Errorf("없는 중간은 null")
	}
	if _, err := GetPath(v, []string{"s", "x"}); err == nil {
		t.Errorf("문자열 아래는 오류")
	}
	v, _ = SetPath(v, []string{"n", "m"}, "y")
	v, _ = DelPath(v, []string{"a"})
	if got := Compact(v); got != `{"s":"x","n":{"m":"y"}}` {
		t.Errorf("SetPath·DelPath = %s", got)
	}
	if _, err := Parse([]byte(`{} {}`)); err == nil {
		t.Errorf("값 둘은 오류")
	}
}
