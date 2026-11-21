package tests

import (
	"os"
	"path/filepath"
	"testing"
)

// hc-lane-wait-guard.test.sh를 옮긴 것: hc-lane wait를 분리 실행(& · nohup · disown · setsid)하면 막는다.
func TestHcLaneWaitGuard(t *testing.T) {
	hook := hookPath(t, "hc-lane-wait-guard")
	// bg: "" = run_in_background 없음, "true"/"false" = 그 값
	cases := []struct {
		name string
		want int
		cmd  string
		bg   string
	}{
		// 막아야 할 것: 분리 실행
		{"끝 &", 2, "hc-lane wait --mine --timeout 5 &", ""},
		{"파이프 뒤 /dev/null &", 2, "hc-lane wait --mine --timeout 5 2>&1 | tail -3 >/dev/null &", ""},
		{"nohup", 2, "nohup hc-lane wait --mine --timeout 5 >/tmp/w.log 2>&1", ""},
		{"중간 &", 2, "hc-lane wait --mine & sleep 1", ""},
		{"disown", 2, "hc-lane wait --mine >/tmp/w.log 2>&1 & disown", ""},
		{"setsid", 2, "setsid hc-lane wait --mine >/tmp/w.log", ""},
		{"bash -c 안의 &", 2, "bash -c 'hc-lane wait --mine >/tmp/w &'", ""},
		{"run_in_background false", 2, "hc-lane wait --mine &", "false"},
		// 통과할 것
		{"포그라운드 파이프", 0, "hc-lane wait --mine --timeout 5 2>&1 | tail -3", ""},
		{"run_in_background true", 0, "hc-lane wait --mine --timeout 600", "true"},
		{"&& echo", 0, "hc-lane wait --mine --timeout 5 && echo done", ""},
		{"2>&1 · &>", 0, "hc-lane wait --mine 2>&1 | tail -3; hc-lane wait --mine &>/tmp/w.log", ""},
		{"다른 명령의 &", 0, "sleep 5 & wait", ""},
		{"명령 없음", 0, "", ""},
	}
	forImpls(t, "hc-lane-wait-guard", func(t *testing.T, impl []string) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				ti := map[string]any{"command": c.cmd}
				if c.bg != "" {
					ti["run_in_background"] = c.bg == "true"
				}
				in := toolInput(map[string]any{"tool_name": "Bash", "tool_input": ti})
				rc, _, errOut := runHook(t, hook, in, envWith(nil, impl...), "")
				checkRC(t, rc, c.want, errOut, "run_in_background")
			})
		}
	})
}

// kill-by-lookup-guard.test.sh를 옮긴 것: 포트·이름으로 찾아 죽이기와 범위 kill을 막는다.
func TestKillByLookupGuard(t *testing.T) {
	hook := hookPath(t, "kill-by-lookup-guard")
	cases := []blockCase{
		// 막아야 할 것: 포트·이름으로 찾아 죽이기, 범위 kill
		{"lsof -ti | xargs kill", 2, "lsof -ti :3000 | xargs kill"},
		{"kill $(lsof -t)", 2, "kill $(lsof -t -i:8080)"},
		{"kill -9 `lsof -ti`", 2, "kill -9 `lsof -ti:3000`"},
		{"lsof -nP -t 변수 경유", 2, `pids=$(lsof -nP -t -iTCP:3000 -sTCP:LISTEN); [ -n "$pids" ] && kill $pids`},
		{`"$(lsof -t)" 큰따옴표 안`, 2, `kill "$(lsof -ti:3000)"`},
		{"fuser -k", 2, "fuser -k 3000/tcp"},
		{"pkill -f", 2, `pkill -f "hc-lane wait --mine"`},
		{"pkill 이름", 2, "pkill socat"},
		{"killall", 2, "killall node"},
		{"/usr/bin/killall", 2, "/usr/bin/killall -9 socat"},
		{"kill $(pgrep -f)", 2, "kill $(pgrep -f next-server)"},
		{"pgrep | xargs kill", 2, `pgrep -f "next dev" | xargs kill -TERM`},
		{"ps | grep | awk | xargs kill", 2, `ps aux | grep socat | awk '{print $2}' | xargs kill`},
		{"kill -9 -1", 2, "kill -9 -1"},
		{"kill 0", 2, "kill 0"},
		{"kill -s TERM -- -1", 2, "kill -s TERM -- -1"},
		{"bash -c 안의 pkill", 2, "bash -c 'pkill -f socat'"},
		{"ssh 원격 fuser -k", 2, "ssh mini-m4 'fuser -k 3000/tcp'"},
		{"앞 명령 뒤 && pkill", 2, "cd /tmp && pkill -f next"},
		// 통과할 것
		{"kill 숫자 pid", 0, "kill 12345"},
		{"kill $PID", 0, `kill "$PID"; wait "$PID" 2>/dev/null`},
		{"kill -9 $SERVER_PID", 0, "kill -9 $SERVER_PID"},
		{"kill %1", 0, "kill %1"},
		{"kill -0 확인", 0, "kill -0 $PID 2>/dev/null && echo alive"},
		{"kill -1 <pid> (HUP)", 0, "kill -1 4321"},
		{"kill $(cat pid 파일)", 0, "kill $(cat /tmp/next.pid)"},
		{"lsof -i 조회만", 0, "lsof -nP -iTCP:3000 -sTCP:LISTEN"},
		{"lsof -ti 조회만", 0, "lsof -ti :3000"},
		{"pgrep 조회만", 0, "pgrep -fl socat"},
		{"hc kill 주소", 0, "hc kill mini-m4/w3"},
		{"pkill -P 부모 pid", 0, "pkill -P $$"},
		{"kill $(pgrep -P 부모)", 0, "kill $(pgrep -P 4321)"},
		{"echo 설명", 0, "echo '포트 정리: lsof -ti :3000 | xargs kill 은 쓰지 말 것'"},
		{"grep 패턴", 0, `grep -rn "pkill -f" ~/.claude/hooks`},
		{"커밋 메시지", 0, `git commit -m "fix: killall·fuser -k 막기" -- a.sh`},
		{"heredoc 본문", 0, "git commit -F - <<'EOF'\nkill-by-lookup-guard: pkill -f·killall·kill $(lsof -ti) 막기\nEOF"},
		{"주석", 0, "kill $PID  # pkill -f 대신"},
		{"skill 낱말", 0, "ls ~/.claude/skills/kill-switch"},
		{"명령 없음", 0, ""},
	}
	forImpls(t, "kill-by-lookup-guard", func(t *testing.T, impl []string) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rc, _, errOut := runHook(t, hook, bashInput(c.cmd), envWith(nil, impl...), "")
				checkRC(t, rc, c.want, errOut, "pid만 kill")
			})
		}
	})
}

// lane-no-shell-edit.test.sh를 옮긴 것: 레인 세션(HC_LANE)에서 셸로 파일 고치기·heredoc 커밋 메시지·zsh 확장 경로를 막는다.
func TestLaneNoShellEdit(t *testing.T) {
	hook := hookPath(t, "lane-no-shell-edit")
	msg := map[string]string{"write": "Edit·Write 도구로", "commit": "git commit -F", "zsh": "경로를 글자로"}
	// want: "0" | write | commit | zsh. noLane: HC_LANE 없이(지휘·사람 세션)
	cases := []struct {
		name, want, cmd string
		noLane          bool
	}{
		// 막아야 할 것: 셸로 파일 고치기
		{"python heredoc open w", "write", "python3 - <<'EOF'\np = \"bin/hc-lane\"\ns = open(p).read().replace(\"a\", \"b\")\nopen(p, \"w\").write(s)\nEOF", false},
		{"python heredoc write_text", "write", "python3 <<EOF\nfrom pathlib import Path\nPath(\"README.md\").write_text(\"x\")\nEOF", false},
		{"python heredoc open a", "write", "cd src && python3 - <<'PY'\nwith open(\"notes.md\", 'a') as f: f.write(\"x\")\nPY", false},
		{"node heredoc writeFileSync", "write", "node - <<'EOF'\nconst fs = require(\"fs\"); fs.writeFileSync(\"a.json\", \"{}\")\nEOF", false},
		{"python -c open w", "write", `python3 -c 'open("x.go","w").write("package x")'`, false},
		{"sed -i ''", "write", "sed -i '' 's/foo/bar/' bin/hc-lane", false},
		{"sed -E -i", "write", `sed -E -i "s/a/b/g" docs/x.md`, false},
		{"sed --in-place", "write", "sed --in-place s/a/b/ main.go", false},
		{"perl -pi -e", "write", "perl -pi -e 's/a/b/' README.md", false},
		{"> 소스 파일", "write", `echo "package main" > cmd/main.go`, false},
		{">> 소스 파일", "write", `printf "x\n" >> docs/CHANGELOG.md`, false},
		{`> "따옴표 파일"`, "write", `jq . a.json > "settings.json"`, false},
		{"cat > 파일 heredoc", "write", "cat > CONTRACT.md <<'EOF'\n# x\nEOF", false},
		{"tee 소스 파일", "write", "echo x | tee -a src/app.ts", false},
		{"앞 명령 뒤 && sed -i", "write", "cd frontend && sed -i '' s/a/b/ src/App.tsx", false},
		// 막아야 할 것: heredoc 커밋 메시지
		{`commit -m "$(cat <<EOF`, "commit", "git commit -m \"$(cat <<'EOF'\nfix: x\n\nCo-Authored-By: y\nEOF\n)\" -- a.sh", false},
		{"commit -F - <<EOF", "commit", "git commit -F - -- a.sh <<'EOF'\nfix: x\nEOF", false},
		{"git -C 경로 commit heredoc", "commit", "git -C /repo commit -F - <<EOF\nfix\nEOF", false},
		// 막아야 할 것: zsh 배열·확장 플래그
		{`files=(${(f)…}) git add`, "zsh", `files=(${(f)"$(git status --porcelain | cut -c4-)"}); git add -- $files`, false},
		{`${(@)…}`, "zsh", `for f in ${(@f)$(git diff --name-only)}; do echo $f; done`, false},
		// 통과할 것: 읽기·검사·시험·임시 폴더·.qa/
		{"python heredoc 읽기", "0", "python3 - <<'EOF'\nimport json\nprint(json.load(open(\"a.json\"))[\"k\"])\nEOF", false},
		{"python heredoc open r", "0", "python3 - <<'EOF'\nprint(open(\"a.md\", \"r\").read()[:100])\nEOF", false},
		{"python heredoc /tmp 쓰기", "0", "python3 - <<'EOF'\nopen(\"/tmp/x.json\", \"w\").write(\"{}\")\nEOF", false},
		{"python heredoc .qa/ 쓰기", "0", "python3 - <<'EOF'\nopen(\".qa/2026-11-18/out.md\", \"w\").write(\"x\")\nEOF", false},
		{"sed -n | grep", "0", "sed -n '1,40p' bin/hc-lane | grep -n report", false},
		{"sed 출력만", "0", "sed 's/a/b/' a.md", false},
		{"sed -i /tmp", "0", "sed -i '' s/a/b/ /tmp/x.md", false},
		{"> /tmp 파일", "0", "jq . a.json > /tmp/b.json", false},
		{"> .qa/ 로그", "0", "bash .claude/herdr-verify.sh > .qa/2026-11-18/verify.md 2>&1", false},
		{"> $TMPDIR", "0", `go test ./... > "$TMPDIR/out.json"`, false},
		{"> 변수 대상", "0", `echo x > "$out"`, false},
		{"> /dev/null·2>&1·>&2", "0", "bash tests/hc-lane.test.sh >/dev/null 2>&1; echo done >&2", false},
		{"> 소스 아닌 파일(.log)", "0", "make > build.log", false},
		{"시험 실행", "0", "bash claude/hooks/tests/lane-no-shell-edit.test.sh", false},
		{"go test", "0", "go test -race ./...", false},
		{"commit -m 글자", "0", `git commit -m "fix: sed -i · > a.md 막기" -- a.sh`, false},
		{"commit -F 파일", "0", "git commit -F /tmp/msg.txt -- a.sh", false},
		{"git add 글자 경로", "0", "git add -- bin/hc-lane claude/hooks/x.sh", false},
		{"grep 패턴", "0", `grep -rn 'open(p, "w")' bin/ | head`, false},
		{`echo 설명 안 ${(f)`, "0", `echo '${(f)…} 대신 글자 경로'`, false},
		{"cat heredoc 읽기만", "0", "cat <<'EOF'\n> a.md 는 예시\nEOF", false},
		{"명령 없음", "0", "", false},
		// HC_LANE 없으면(지휘·사람 세션) 늘 통과
		{"HC_LANE 없음: sed -i", "0", "sed -i '' s/a/b/ bin/hc-lane", true},
		{"HC_LANE 없음: heredoc 커밋", "0", "git commit -F - <<EOF\nfix\nEOF", true},
		{"HC_LANE 없음: python 쓰기", "0", "python3 - <<EOF\nopen(\"a.go\",\"w\").write(\"x\")\nEOF", true},
		{`HC_LANE 없음: ${(f)…}`, "0", `files=(${(f)"$(git status)"}); git add -- $files`, true},
	}
	forImpls(t, "lane-no-shell-edit", func(t *testing.T, impl []string) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				env := envWith([]string{"HC_LANE"}, append([]string{"HC_LANE=lane-x"}, impl...)...)
				if c.noLane {
					env = envWith([]string{"HC_LANE"}, impl...)
				}
				rc, _, errOut := runHook(t, hook, bashInput(c.cmd), env, "")
				if c.want == "0" {
					checkRC(t, rc, 0, errOut, "")
				} else {
					checkRC(t, rc, 2, errOut, msg[c.want])
				}
			})
		}
	})
}

// loop-no-question.test.sh(hooks-go 준비 커밋)를 옮긴 것: 이름이나 job 라벨에 loop가 든 hc 터미널에서 질문 창을 막는다.
// hc는 PATH 앞의 가짜 hc: FAKE_JSON을 그대로 찍고 FAKE_RC로 끝난다.
func TestLoopNoQuestion(t *testing.T) {
	hook := hookPath(t, "loop-no-question")
	T := t.TempDir()
	for _, d := range []string{"bin", "home"} {
		if err := os.MkdirAll(filepath.Join(T, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake := "#!/bin/bash\n[ \"$1 $3\" = \"get --json\" ] || exit 9\nprintf '%s' \"$FAKE_JSON\"\nexit \"${FAKE_RC:-0}\"\n"
	if err := os.WriteFile(filepath.Join(T, "bin", "hc"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		want       int
		term, json string
		rc         string // FAKE_RC, 빈 값이면 0
		path       string // 빈 값이면 가짜 hc가 앞에 선 PATH
	}{
		// 막을 것: 이름이나 job 라벨에 loop(대소문자 무관)
		{"이름 loop", 2, "t1", `{"name":"d1-loop-150","labels":{}}`, "", ""},
		{"이름 대문자 LOOP", 2, "t1", `{"name":"Inquiry-LOOP"}`, "", ""},
		{"job 라벨 loop", 2, "t1", `{"name":"worker","labels":{"job":"inquiry-loop"}}`, "", ""},
		{"이름 없음·job loop", 2, "t1", `{"labels":{"job":"loop"}}`, "", ""},
		// 통과할 것
		{"loop 아닌 이름", 0, "t1", `{"name":"hooks-go","labels":{"job":"lane"}}`, "", ""},
		{"hc 터미널 아님", 0, "", `{"name":"d1-loop"}`, "", ""},
		{"hc get 실패", 0, "t1", `{"name":"d1-loop"}`, "1", ""},
		{"hc 없음", 0, "t1", `{"name":"d1-loop"}`, "0", "/usr/bin:/bin"},
		{"JSON 아님", 0, "t1", "not json loop", "", ""},
		{"다른 키의 loop", 0, "t1", `{"name":"w","labels":{"role":"loop"},"cwd":"/loop"}`, "", ""},
	}
	forImpls(t, "loop-no-question", func(t *testing.T, impl []string) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				path, fakeRC := c.path, c.rc
				if path == "" {
					path = filepath.Join(T, "bin") + ":/usr/bin:/bin"
				}
				if fakeRC == "" {
					fakeRC = "0"
				}
				env := envWith(nil, append([]string{"HOME=" + filepath.Join(T, "home"), "PATH=" + path,
					"HC_TERMINAL_ID=" + c.term, "FAKE_JSON=" + c.json, "FAKE_RC=" + fakeRC}, impl...)...)
				in := []byte(`{"tool_name":"AskUserQuestion","tool_input":{}}`)
				rc, _, errOut := runHook(t, hook, in, env, "")
				checkRC(t, rc, c.want, errOut, "질문 창을 띄우지 않는다")
			})
		}
	})
}
