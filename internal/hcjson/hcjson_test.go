package hcjson

import (
	"strings"
	"testing"
)

// live: 정상 live 항목에 칸 하나를 바꿔 끼운다(kv가 "" 이면 그 칸을 뺀다)
func live(over map[string]string) string {
	f := map[string]string{"address": `"t1"`, "state": `"live"`, "cwd": `"/w/a"`, "agent": `"claude-code"`, "name": `"lane-a"`, "status": `"working"`}
	order := []string{"address", "state", "cwd", "agent", "name", "status", "labels", "parent", "pid", "created_ms"}
	for k, v := range over {
		f[k] = v
	}
	var p []string
	for _, k := range order {
		if v, ok := f[k]; ok && v != "" {
			p = append(p, `"`+k+`":`+v)
		}
	}
	return "{" + strings.Join(p, ",") + "}"
}

func ls(items ...string) string { return `{"terminals":[` + strings.Join(items, ",") + `]}` }

func TestParseListReal(t *testing.T) {
	in := `{"hidden_finished":0,"machines":[{"name":"m1","state":"connected"},{"name":"m2","state":"disconnected"}],"terminals":[` +
		`{"address":"t79x","agent":"zsh","cwd":"/","state":"live","turn":93,"where":"/ / 1"},` +
		`{"address":"m1/t87r","agent":"claude-code","cwd":"/w/go-x/","name":"go-x","parent":"t1cj","state":"live","status":"done","turn":4}]}`
	l, err := ParseList([]byte(" \n" + in + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Terminals) != 2 || !l.HasMachines || len(l.Machines) != 2 {
		t.Fatalf("%+v", l)
	}
	a, b := l.Terminals[0], l.Terminals[1]
	if a.Address != "t79x" || a.Cwd != "/" || a.HasName || a.Status != "" || a.Agent != "zsh" || string(a.Raw["turn"]) != "93" {
		t.Fatalf("첫 항목 %+v", a)
	}
	if b.Address != "m1/t87r" || b.Cwd != "/w/go-x" || b.Name != "go-x" || !b.HasName || b.Parent != "t1cj" || b.Status != "done" {
		t.Fatalf("둘째 항목 %+v", b)
	}
	if m, ok := l.Machine("m2"); !ok || m.State != "disconnected" {
		t.Fatalf("기계 %+v", m)
	}
	if len(l.ByAddress("t79x")) != 1 || len(l.ByAddress("zz")) != 0 {
		t.Fatal("ByAddress")
	}
}

func TestParseListEmpty(t *testing.T) {
	l, err := ParseList([]byte(`{"terminals":[]}`))
	if err != nil || len(l.Terminals) != 0 || l.HasMachines {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestParseListCases(t *testing.T) {
	bad := map[string]string{
		"빈 입력":           ``,
		"공백만":            " \n\t",
		"terminals 없음":   `{}`,
		"terminals null": `{"terminals":null}`,
		"terminals 글":    `{"terminals":"error"}`,
		"terminals 객체":   `{"terminals":{}}`,
		"값 둘":            ls(live(nil)) + ls(live(nil)),
		"뒤 찌꺼기":          ls(live(nil)) + "x",
		"잘림":             `{"terminals":[{"addr`,
		"항목 문자열":         ls(`"t1"`),
		"항목 null":        ls(`null`),
		"address 없음":     ls(live(map[string]string{"address": ""})),
		"address 빈 글":    ls(live(map[string]string{"address": `""`})),
		"address 숫자":     ls(live(map[string]string{"address": `42`})),
		"state 없음":       ls(live(map[string]string{"state": ""})),
		"state 빈 글":      ls(live(map[string]string{"state": `""`})),
		"state starting": ls(live(map[string]string{"state": `"starting"`})),
		"state null":     ls(live(map[string]string{"state": `null`})),
		"status weird":   ls(live(map[string]string{"status": `"weird"`})),
		"status 숫자":      ls(live(map[string]string{"status": `3`})),
		"cwd 빈 글":        ls(live(map[string]string{"cwd": `""`})),
		"cwd 점":          ls(live(map[string]string{"cwd": `"."`})),
		"cwd 상대":         ls(live(map[string]string{"cwd": `"rel/x"`})),
		"cwd null":       ls(live(map[string]string{"cwd": `null`})),
		"cwd 없음":         ls(live(map[string]string{"cwd": ""})),
		"name 숫자":        ls(live(map[string]string{"name": `42`})),
		"name 객체":        ls(live(map[string]string{"name": `{}`})),
		"name 배열":        ls(live(map[string]string{"name": `[]`})),
		"labels 숫자 값":    ls(live(map[string]string{"labels": `{"hc-lane":1}`})),
		"정상+깨짐":          ls(live(nil), live(map[string]string{"address": `"t2"`, "cwd": `"."`})),
		"기계 이름 빈 글":      `{"terminals":[],"machines":[{"name":"","state":"connected"}]}`,
		"기계 이름 숫자":       `{"terminals":[],"machines":[{"name":42,"state":"connected"}]}`,
		"기계 state 없음":    `{"terminals":[],"machines":[{"name":"m1"}]}`,
		"machines null":  `{"terminals":[],"machines":null}`,
	}
	for name, in := range bad {
		if l, err := ParseList([]byte(in)); err == nil {
			t.Errorf("%s: 오류여야 한다, 받음 %+v", name, l)
		}
	}
	good := map[string]func(Terminal) bool{
		ls(live(map[string]string{"status": `null`})):            func(x Terminal) bool { return x.Status == "" },
		ls(live(map[string]string{"status": ""})):                func(x Terminal) bool { return x.Status == "" },
		ls(live(map[string]string{"cwd": `"/a/b/"`})):            func(x Terminal) bool { return x.Cwd == "/a/b" },
		ls(live(map[string]string{"name": `null`})):              func(x Terminal) bool { return !x.HasName && x.Name == "" },
		ls(live(map[string]string{"name": ""})):                  func(x Terminal) bool { return !x.HasName },
		ls(live(map[string]string{"labels": `null`})):            func(x Terminal) bool { return x.Labels != nil && len(x.Labels) == 0 },
		ls(live(map[string]string{"labels": `{"hc-lane":"1"}`})): func(x Terminal) bool { return x.Labels["hc-lane"] == "1" },
		ls(`{"address":"t3","state":"finished","cwd":null}`):     func(x Terminal) bool { return x.State == "finished" && x.Cwd == "" },
	}
	for in, ok := range good {
		l, err := ParseList([]byte(in))
		if err != nil || len(l.Terminals) != 1 || !ok(l.Terminals[0]) {
			t.Errorf("%s: %+v %v", in, l, err)
		}
	}
}

func TestParseMachines(t *testing.T) {
	ms, err := ParseMachines([]byte(`{"machines":[{"name":"m1","state":"connected","host":"h"}]}`))
	if err != nil || len(ms) != 1 || ms[0] != (Machine{"m1", "connected"}) {
		t.Fatalf("%+v %v", ms, err)
	}
	for _, in := range []string{``, `{}`, `{"machines":null}`, `{"machines":[{"name":"m1","state":3}]}`, `{"machines":[]}x`} {
		if _, err := ParseMachines([]byte(in)); err == nil {
			t.Errorf("%q: 오류여야 한다", in)
		}
	}
}

func TestParseGet(t *testing.T) {
	full := `{"address":"m1/t8","cwd":"/w","created_ms":1795355169249,"labels":{"hc-lane":"1"},"name":"lane-a","parent":null,"pid":34110,` +
		`"state":"live","status":{"app":"claude-code","msg":"x","state":"working"},"turn":0}`
	tm, found, err := ParseGet([]byte(full), 0, "m1/t8")
	if err != nil || !found || tm.PID != 34110 || tm.Created != 1795355169249 || tm.Status != "working" || tm.Name != "lane-a" || tm.Labels["hc-lane"] != "1" {
		t.Fatalf("%+v %v %v", tm, found, err)
	}
	nf := `{"error":{"hint":"see hc ls --all","kind":"not_found","message":"no terminal \"t8\""}}`
	if _, found, err := ParseGet([]byte(nf), 3, "t8"); err != nil || found {
		t.Fatalf("not_found: %v %v", found, err)
	}
	if tm, found, err := ParseGet([]byte(strings.Replace(full, `"state":"live"`, `"state":"live"`, 1)), 11, "m1/t8"); err != nil || !found || tm.State != "finished" {
		t.Fatalf("rc 11: %+v %v %v", tm, found, err)
	}
	bad := []struct {
		name, in string
		rc       int
	}{
		{"주소 다름", full, 0},
		{"rc3 빈 출력", ``, 3},
		{"rc3 machine_unknown", `{"error":{"kind":"machine_unknown","message":"x"}}`, 3},
		{"rc3 객체 아님 오류", `{"error":"x"}`, 3},
		{"rc3 정상 객체", full, 3},
		{"rc7", nf, 7},
		{"rc124", full, 124},
		{"rc127", ``, 127},
		{"rc0 빈 출력", ``, 0},
		{"rc0 {}", `{}`, 0},
		{"rc0 오류 객체", nf, 0},
		{"created_ms 글", strings.Replace(full, `1795355169249`, `"?"`, 1), 0},
		{"created_ms 실수", strings.Replace(full, `1795355169249`, `1.5`, 1), 0},
		{"pid 음수", strings.Replace(full, `34110`, `-1`, 1), 0},
		{"status 객체 state 모름", strings.Replace(full, `"state":"working"`, `"state":"weird"`, 1), 0},
		{"status 객체 state 없음", strings.Replace(full, `,"state":"working"`, ``, 1), 0},
		{"rc11 잘림", full[:40], 11},
	}
	for _, c := range bad {
		want := "m1/t8"
		if c.name == "주소 다름" {
			want = "m1/t9"
		}
		if _, _, err := ParseGet([]byte(c.in), c.rc, want); err == nil {
			t.Errorf("%s: 오류여야 한다", c.name)
		}
	}
}
