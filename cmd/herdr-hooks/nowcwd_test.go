package main

// cwd-now-inuse(2026-11-23): 동거 판정(hcWho)은 hc ls의 cwd(띄운 폴더)에 더해 hc get의 foreground.cwd(지금 폴더)도 본다 —
// 띄운 뒤 cd로 같은 작업 트리에 들어온 다른 에이전트를 동거로. 지금 폴더를 모르면(get 실패·칸 없음) 모름 = 동거(지금 규칙)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHcWhoNowCwd(t *testing.T) {
	R, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	O := t.TempDir()
	os.MkdirAll(R+"/sub", 0o755)
	os.Mkdir(R+"/.git", 0o755)
	H := t.TempDir()
	if err := os.WriteFile(H+"/hc", []byte(`#!/bin/sh
case $1 in
  ls) cat "$HCW/ls.json" ;;
  get) [ -f "$HCW/get-$2.json" ] || { echo "error: cannot reach the core" >&2; exit 1; }; cat "$HCW/get-$2.json" ;;
  *) exit 1 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", H+":"+os.Getenv("PATH"))
	t.Setenv("HCW", H)
	t.Chdir(O) // intree(cdP)가 cwd를 옮긴다 — 시험이 끝나면 되돌린다

	term := func(cwd string) string {
		return `{"address":"t2","agent":"claude-code","name":"w2","state":"live","cwd":"` + cwd + `"}`
	}
	for _, c := range []struct {
		name, launch, get string // get이 빈 글이면 hc get 실패
		want              string // who에 들어야 할 글(빈 글이면 동거 아님)
	}{
		{"띄운 뒤 그 작업 트리 아래로 들어옴", O, `{"address":"t2","state":"live","cwd":"` + O + `","foreground":{"cwd":"` + R + `/sub"}}`, "w2[t2]"},
		{"지금 폴더 모름(hc get 실패)", O, "", "모름"},
		{"지금 폴더 모름(foreground.cwd 없음)", O, `{"address":"t2","state":"live","cwd":"` + O + `","foreground":{"name":"node"}}`, "모름"},
		{"띄운 폴더가 그 트리(지금은 딴 곳) — 지금처럼 동거", R, `{"address":"t2","state":"live","cwd":"` + R + `","foreground":{"cwd":"` + O + `"}}`, "w2[t2]"},
		{"띄운·지금 폴더 모두 딴 곳", O, `{"address":"t2","state":"live","cwd":"` + O + `","foreground":{"cwd":"` + O + `"}}`, ""},
	} {
		os.WriteFile(H+"/ls.json", []byte(`{"terminals":[`+term(c.launch)+`,{"address":"m1/t3","agent":"claude-code","state":"live","cwd":"/x"}]}`), 0o644)
		os.Remove(H + "/get-t2.json")
		if c.get != "" {
			os.WriteFile(H+"/get-t2.json", []byte(c.get), 0o644)
		}
		hookBudget(4 * time.Second)
		g := &cg{root: R}
		g.hcWho()
		if (c.want == "") != (g.who == "") || !strings.Contains(g.who, c.want) {
			t.Errorf("%s: who %q(기대 %q)", c.name, g.who, c.want)
		}
	}
}
