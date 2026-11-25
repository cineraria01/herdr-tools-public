package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ht-send-shell-guard(2026-11-25): 지휘가 `hc send t3yx "…"`를 보냈는데 t3yx는 에이전트가 끝나 zsh만 남은 터미널이라 글이 셸 프롬프트에
// 쳐졌다(이번엔 첫머리 `<hc from=…>`가 입력 돌리기라 parse error로 끝났지만 다른 글이면 명령으로 실행된다). hc-lane-wait-guard가 Bash 명령의
// `hc send|keys <주소>`마다 hc get으로 받는 쪽을 보고, 셸만 남았으면 막는다. 판정 못 하면 막지 않고 경고(additionalContext)만.
// 견본 JSON은 cmd/hc-lane/testdata/agent(실측)의 꼴.
func TestHcSendShellGuard(t *testing.T) {
	const hook = "hc-lane-wait-guard"
	T := t.TempDir()
	for _, d := range []string{"bin", "home", "get"} {
		if err := os.MkdirAll(filepath.Join(T, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 가짜 hc: get <주소> --json 만 — get/<주소의 / 를 _ 로>.json 을 찍고 .rc(없으면 0)로 끝난다. 견본이 없으면 not_found(종료 3)
	fake := `#!/bin/bash
[ "$1" = get ] && [ "$3" = --json ] || exit 9
f="$FAKE_DIR/$(printf %s "$2" | tr / _)"
[ -f "$f.json" ] || { echo '{"error":{"kind":"not_found","message":"no such terminal"}}' >&2; exit 3; }
cat "$f.json"; exit "$(cat "$f.rc" 2>/dev/null || echo 0)"
`
	if err := os.WriteFile(filepath.Join(T, "bin", "hc"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	shell := func(addr string) string {
		return `{"address":"` + addr + `","argv":["zsh","-l"],"created_ms":1,"cwd":"/u/repo","foreground":{"argv":["zsh","-l"],"cwd":"/u/repo","name":"zsh","pid":50980},"name":"wag","parent":null,"pid":50980,"state":"live","status":null,"turn":2}`
	}
	agent := `{"address":"t2","argv":["zsh","-l"],"created_ms":1,"cwd":"/u/repo","foreground":{"argv":["/x/node","/x/agent.js","run"],"cwd":"/u/repo","name":"node","pid":57907},"name":"w","parent":null,"pid":50980,"state":"live","status":{"app":"claude-code","state":"idle"},"turn":0}`
	files := map[string]string{
		"t1.json":    shell("t1"),
		"m1_t9.json": shell("m1/t9"),
		"sh.json":    shell("t5"), // 이름으로 부른 셸 — 응답 주소는 tN
		"t2.json":    agent,
		"t7.json":    agent, "t7.rc": "7", // hc get 실패
		"t8.json": `{"address":"t8"`, // 깨진 응답
		// 서비스 터미널(감사 atp1om3 중간 1): 프록시 서버 — 상태 보고 한 번도 없음, 전경 argv가 … server(실측 꼴, 경로는 지어냄)
		"px.json":  `{"address":"t11","argv":["/usr/bin/env","-i","HOME=/u","/u/.local/bin/pool","server"],"created_ms":1,"cwd":"/u","foreground":{"argv":["/u/.local/bin/node","/u/lib/pool.js","server"],"cwd":"/u","name":"node","pid":700},"name":"pool-proxy","parent":null,"pid":690,"state":"live","status":null,"status_ms":null,"turn":0}`,
		"t12.json": `{"address":"t12","argv":["env","hc-merger","spec"],"created_ms":1,"cwd":"/u","foreground":{"argv":["hc-merger","spec"],"cwd":"/u","name":"hc-merger","pid":710},"labels":{"hc-merger-spec":"1"},"name":"spec-1","parent":null,"pid":705,"state":"live","status":null,"status_ms":null,"turn":0}`,
		"t13.json": `{"address":"t13","argv":["zsh","-lc","hc-lane watch"],"created_ms":1,"cwd":"/u","foreground":{"argv":["hc-lane","watch"],"cwd":"/u","name":"hc-lane","pid":720},"labels":{"hc-lane-watch":"1"},"name":"watch","parent":null,"pid":715,"state":"live","status":null,"status_ms":null,"turn":0}`,
		// 막 뜬 에이전트(신뢰 질문에서 상태 보고 전) — 통과해야 hc keys <주소> Down → Enter가 깨지지 않는다
		"t14.json": `{"address":"t14","argv":["zsh","-c","agent run"],"created_ms":1,"cwd":"/u","foreground":{"argv":["/x/node","/x/agent.js","run"],"cwd":"/u","name":"node","pid":730},"name":"lane-a","parent":null,"pid":725,"state":"live","status":null,"status_ms":null,"turn":0}`,
		// 상태를 보고한 에이전트는 argv가 server로 끝나도 에이전트
		"t15.json": `{"address":"t15","argv":["zsh","-l"],"created_ms":1,"cwd":"/u","foreground":{"argv":["/x/node","/x/agent.js","server"],"cwd":"/u","name":"node","pid":740},"name":"w","parent":null,"pid":735,"state":"live","status":{"app":"claude-code","state":"working"},"status_ms":5,"turn":1}`,
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(T, "get", n), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const block, warn = "보내지 않는다", "판정하지 못"
	cases := []struct {
		name, cmd string
		want      int
		out       string // 통과할 때 stdout에 들어야 할 글(경고), 빈 값이면 stdout이 비어야 한다
	}{
		// 막을 것: 받는 쪽이 셸만 남은 터미널
		{"send 셸", `hc send t1 "[t30x→t1] 다음 일"`, 2, ""},
		{"keys 셸 Enter", "hc keys t1 Enter", 2, ""},
		{"옵션 뒤 대상", "hc send --if idle --timeout 30 --wait t1 hi", 2, ""},
		{"= 옵션", "hc send --if=idle t1 hi", 2, ""},
		{"원격 셸", "hc send m1/t9 'hi'", 2, ""},
		{"이름으로", "hc send sh hi", 2, ""},
		{"경로로 부른 hc", "~/.local/bin/hc send t1 hi", 2, ""},
		{"여러 줄 안", "echo start\nhc send t2 ok && hc send m1/t9 'x'\necho end", 2, ""},
		{"; 뒤", "sleep 1; hc send t1 x", 2, ""},
		{"$( ) 안", "r=$(hc send t1 x)", 2, ""},
		{"프록시 서버 send", "hc send px hi", 2, ""},
		{"프록시 서버 ctrl+c", "hc keys px ctrl+c", 2, ""},
		{"병합기 검증 터미널", "hc keys t12 ctrl+c", 2, ""},
		{"감시 터미널", "hc send t13 hi", 2, ""},
		{"막 뜬 에이전트 신뢰 질문", "hc keys t14 Down && hc keys t14 Enter", 0, ""},
		{"server로 끝나는 에이전트", "hc send t15 hi", 0, ""},
		// 통과할 것
		{"send 에이전트", `hc send t2 "다음 일"`, 0, ""},
		{"keys 에이전트", "hc keys t2 esc", 0, ""},
		{"없는 터미널", "hc send t404 hi", 0, ""}, // hc send가 스스로 실패한다
		{"따옴표 안 글", `grep -n "hc send t1" notes.md`, 0, ""},
		{"다른 hc 명령", "hc get t1 --json; hc read t1", 0, ""},
		{"hc-lane wait 그대로", "hc-lane wait --mine --timeout 5", 0, ""},
		// 판정 못 함: 통과 + 경고
		{"hc get 실패", "hc send t7 hi", 0, warn},
		{"깨진 응답", "hc send t8 hi", 0, warn},
		{"변수 주소", `hc send "$T" hi`, 0, warn},
		{"주소 없음", "hc send", 0, warn},
	}
	env := func(path string) []string {
		return envWith(nil, "HOME="+filepath.Join(T, "home"), "PATH="+path, "FAKE_DIR="+filepath.Join(T, "get"))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc, out, errOut := runHook(t, hook, bashInput(c.cmd), env(filepath.Join(T, "bin")+":/usr/bin:/bin"), "")
			checkRC(t, rc, c.want, errOut, block)
			if c.want == 0 && (c.out == "") != (out == "") || c.out != "" && !strings.Contains(out, c.out) {
				t.Errorf("stdout=%q, want %q", out, c.out)
			}
		})
	}
	// hc가 없으면(PATH·~/.local/bin 어디에도) 판정하지 못함 — 통과(경고)
	t.Run("hc 없음", func(t *testing.T) {
		rc, out, _ := runHook(t, hook, bashInput("hc send t1 hi"), env("/usr/bin:/bin"), "")
		if rc != 0 || !strings.Contains(out, warn) {
			t.Errorf("rc=%d out=%q", rc, out)
		}
	})
}
