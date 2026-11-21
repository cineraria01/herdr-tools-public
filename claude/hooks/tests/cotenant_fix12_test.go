package tests

// cotenant-guard 시나리오: 수정 1차(감사 2026-11-04)·2차(재감사).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func cotenantFix12(c *cg, A, B, C int) {
	T, H := c.T, c.H
	R, R2 := c.R, T+"/r2"
	// ---------- 수정 1차 (감사 2026-11-04: H1 H2 M1 M2 M3 L1 L2 L3 L4 L6 L8) ----------
	// hookw SESSION TOOL PATH CONTENT — Write/Edit carrying file content
	hookw := func(s, tool, p, content string) {
		ti := obj("file_path", p, "old_string", "x", "new_string", content)
		if tool == "Write" {
			ti = obj("file_path", p, "content", content)
		}
		c.exec1("bash", c.hook, obj("session_id", s, "cwd", R, "tool_name", tool, "tool_input", ti), c.env(c.stdPath()))
	}
	R3 := T + "/r3"
	c.mkrepo(R3)
	c.write(R3+"/.gitignore", ".qa/\nlogs/\n")
	c.write(R3+"/src/pkg/x.go", "p\n")
	c.git(R3, "add", "-A")
	c.git(R3, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "more")
	A3 := c.agent()
	c.reg(A3, "s3a", R3, "agent-a3")
	B3 := c.agent()
	c.reg(B3, "s3b", R3, "agent-b3")
	c.run("s3a", "Edit", R3+"/src/pkg/x.go", "", R3)
	c.appendf(R3+"/src/pkg/x.go", "y\n")

	for _, cmd := range []string{"rm -rf src", "rm -rf src/pkg/", "mv src src2", "cp -r /tmp src/pkg", "rm -rf ./*", "rm -rf " + R3 + "/src"} {
		c.run("s3b", "Bash", cmd, "", R3)
		c.check("H1 남의 미커밋 파일이 든 폴더 대상 쓰기 차단: "+cmd, 2, "agent-a3")
	}
	c.run("s3b", "Bash", "rm -rf sub", "", R3)
	c.check("H1 남의 미커밋 파일 없는 폴더는 허용", 0)
	c.checkOK("막힌 Bash 명령은 앞쪽 대상에도 임대를 남기지 않음(rm -rf ./* 의 e.txt)", isDir(filepath.Dir(c.leasefile(R3, "e.txt"))), 1)
	c.run("s3b", "Bash", "cp a.txt sub/", "", R3)
	c.checkOK("L6 폴더 대상은 임대를 잡지 않음", isDir(filepath.Dir(c.leasefile(R3, "sub"))), 1)
	c.appendf(R3+"/sub/c.txt", "z\n")
	c.run("s3a", "Bash", "git pull --rebase", "", R3)
	c.check("L6 폴더 임대 탓에 남의 rebase가 막히지 않음", 0)
	c.git(R3, "checkout", "-q", "--", "sub/c.txt")

	c.run("s3a", "Edit", R3+"/a.txt", "", R3)
	c.appendf(R3+"/a.txt", "y\n")
	c.git(R3, "add", "a.txt")
	c.run("s3b", "Edit", R3+"/b.txt", "", R3)
	c.appendf(R3+"/b.txt", "y\n")
	c.run("s3b", "Bash", "git commit -m 'b fix'", "", R3)
	c.check("M1 경로 없는 commit에 남이 stage한 파일이 있으면 차단", 2, "agent-a3")
	c.run("s3b", "Bash", "git commit --amend --no-edit", "", R3)
	c.check("M1 commit --amend도 같은 판정", 2)
	c.run("s3b", "Bash", "git commit -m 'b fix' -- b.txt", "", R3)
	c.check("M1 경로 지정 commit은 허용", 0)
	for _, cmd := range []string{"git add *.txt", "git add '*.txt'", "git add ':(glob)**/*.go'", "git add $(git ls-files -m)", "git ls-files -m | xargs git add", `for f in a b; do git add "$f"; done`} {
		c.run("s3b", "Bash", cmd, "", R3)
		c.check("M1 글롭·판정 못 하는 git add 차단: "+cmd, 2)
	}
	c.run("s3b", "Bash", "git add 'b*.txt'", "", R3)
	c.check("M1 남의 파일과 안 겹치는 글롭은 허용", 0)
	c.run("sx", "Bash", "git add $(git ls-files -m)", "", R2)
	c.check("M1 혼자면 판정 못 하는 git add도 허용", 0)

	for _, ag := range []string{"bun", "node", "go", "vite", "make"} {
		c.write(T+"/hc.json", fmt.Sprintf(`{"terminals":[{"address":"t9","agent":"%s","cwd":"%s","state":"live","name":"dev"}]}`+"\n", ag, R2))
		c.run("sx", "Bash", "git stash", "", R2)
		c.check("M2 hc의 "+ag+" 터미널은 에이전트 아님", 0)
	}
	for _, ag := range []string{"claude-code", "codex", "agy", "grok", "cursor-agent", "gemini", "opencode"} {
		c.write(T+"/hc.json", fmt.Sprintf(`{"terminals":[{"address":"t9","agent":"%s","cwd":"%s","state":"live","name":"dev"}]}`+"\n", ag, R2))
		c.run("sx", "Bash", "git stash", "", R2)
		c.check("M2 hc의 "+ag+" 터미널은 에이전트", 2, "dev")
	}
	os.Remove(T + "/hc.json") //nolint:errcheck // rm -f

	// 등록 명령은 저장소에 적힌 꼴(훅 헤더의 「The entry runs」 줄)에서 읽는다 — 시험하는 사람의 실제 설정에 기대지 않는다(아스트라 E13).
	// 이 기계 설정에 등록이 있으면 그것도 같은 꼴인지 따로 본다(없으면 건너뜀)
	src, err := os.ReadFile(c.hook)
	c.fatal(err)
	cmdline := ""
	if m := regexp.MustCompile(`(?m)^#\s+(/bin/bash -c '\[ -r .*cotenant-guard\.sh"')\s*$`).FindStringSubmatch(string(src)); m != nil {
		cmdline = m[1]
	}
	safeForm := func(s string) bool {
		ri := strings.Index(s, "[ -r ") // case $CMDLINE in *'[ -r '*'|| exit 0'*)
		return ri >= 0 && strings.Contains(s[ri:], "|| exit 0")
	}
	c.checkOK("M3 훅 헤더의 등록 명령은 훅 파일이 없을 때만 허용하는 꼴", safeForm(cmdline), 0)
	if real := settingsHookCommand(os.Getenv("HOME")+"/.claude/settings.json", "cotenant-guard"); real != "" {
		c.checkOK("M3 이 기계 설정 등록 명령도 같은 꼴", safeForm(real), 0)
	}
	c.fatal(os.MkdirAll(T+"/h2/.claude/hooks", 0o755))
	m3 := func() int {
		cmd := exec.Command("/bin/sh", "-c", cmdline)
		cmd.Stdin = strings.NewReader("{}")
		cmd.Env = envWith(nil, "HOME="+T+"/h2")
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return -1
		}
		return 0
	}
	c.RC, c.OUT, c.ERR = m3(), "", ""
	c.check("M3 훅 파일이 없으면 허용", 0)
	c.write(T+"/h2/.claude/hooks/cotenant-guard.sh", "#!/bin/bash\nexit 2\n")
	c.RC = m3()
	c.check("M3 훅 파일이 있으면 그 판정(2)", 2)
	c.write(T+"/h2/.claude/hooks/cotenant-guard.sh", "#!/bin/bash\nexit 1\n")
	c.RC = m3()
	c.check("M3 훅 내부 오류는 그대로 0이 아님(차단)", 1)

	c.fatal(os.MkdirAll(T+"/badbin", 0o755))
	for _, b := range []string{"stat", "md5", "jq", "awk", "date", "ps", "touch", "mkdir", "rmdir", "mv", "rm"} {
		c.fatal(os.WriteFile(T+"/badbin/"+b, []byte("#!/bin/sh\necho broken\nexit 1\n"), 0o755))
	}
	c.run("s3a", "Edit", R3+"/g.txt", "", R3)
	c.exec1("bash", c.hook, obj("session_id", "s3b", "cwd", R3, "tool_name", "Edit", "tool_input", obj("file_path", R3+"/g.txt")),
		c.env(T+"/badbin:"+c.stdPath()))
	c.check("L3 PATH 앞의 가짜 stat·md5·jq·awk·date·ps 등을 쓰지 않음", 2, "10분 안")

	c.write(T+"/self.sh", `# acts as the Claude process: owns a lease recorded with other spacing, then runs the hook as its direct child
H=$1 HOOK=$2 R=$3 T=$4
st=$(LC_ALL=C TZ=UTC /bin/ps -o lstart= -p $$ | sed 's/ *$//; s/ /  /g')
ld=$(git -C "$R" rev-parse --absolute-git-dir)/agent-leases/$(/sbin/md5 -q -s h.txt); mkdir -p "$ld"
printf 'old-session\037%s\037%s\037\037me\0372026-01-01 00:00:00\037h.txt\n' $$ "$st" > "$ld/owner"
echo z >> "$R/h.txt"
printf '{"session_id":"unregistered","cwd":"%s","tool_name":"Edit","tool_input":{"file_path":"%s/h.txt"}}' "$R" "$R" |
  env -u CLAUDE_CONFIG_DIR -u HC_TERMINAL_ID HOME="$H" PATH="$T/bin:$PATH" /bin/bash "$HOOK"
`)
	{
		cmd := exec.Command("bash", T+"/self.sh", H, c.hook, R3, T)
		cmd.Env, cmd.Dir = envWith(nil, c.impl...), T
		o, err := cmd.CombinedOutput()
		c.RC, c.OUT, c.ERR = 0, strings.TrimRight(string(o), "\n"), ""
		if ee, ok := err.(*exec.ExitError); ok {
			c.RC = ee.ExitCode()
		} else if err != nil {
			c.t.Fatal(err)
		}
		c.check("L1 레지스트리에 없는 세션도 자기 임대(공백 표기만 다름)는 허용", 0)
	}

	c.fatal(os.MkdirAll(R3+"/.qa", 0o755))
	c.fatal(os.MkdirAll(R3+"/logs", 0o755))
	c.run("s3a", "Write", R3+"/.qa/report.md", "", R3)
	c.write(R3+"/.qa/report.md", "r\n")
	c.run("s3b", "Edit", R3+"/.qa/report.md", "", R3)
	c.check("L2 무시된 파일은 10분 유예 없이 넘겨받음", 0)
	c.run("s3a", "Bash", "echo step1 >> logs/progress.log", "", R3)
	c.run("s3b", "Bash", "echo step2 >> logs/progress.log", "", R3)
	c.check("L2 무시된 공유 로그 >> 허용", 0)

	for _, cmd := range []string{"bash -c 'git stash'", `sh -c "git add -A"`, "zsh -lc 'cd sub && git reset --hard'", "eval git stash", "eval 'git add -A'", "timeout 30 git stash", "timeout -s KILL 5m git add -A", "nice git reset --hard", "nice -n 5 git stash", "xargs git add < list"} {
		c.run("s3b", "Bash", cmd, "", R3)
		c.check("L4 감싼 명령도 검사: "+cmd, 2)
	}
	c.run("s3b", "Bash", "bash -c 'echo hi; ls'", "", R3)
	c.check("L4 감싼 무해한 명령은 허용", 0)
	c.run("s3b", "Bash", "git commit -m \"$(cat <<'EOF'\nfix: 27\" 모니터 대응\nrm 금지 규칙 문서화\ngit stash 언급\nEOF\n)\" -- b.txt", "", R3)
	c.check("L4 heredoc 본문의 홀수 따옴표에 짝이 뒤집히지 않음", 0)
	c.checkOK("L4 heredoc 본문 낱말로 유령 임대가 생기지 않음", isDir(filepath.Dir(c.leasefile(R3, "금지"))), 1)

	c.run("s3b", "Bash", `printf '{"disableAllHooks":true}' > .claude/settings.local.json`, "", R3)
	c.check("L8 Bash로 disableAllHooks 쓰기 차단", 2, "사람만")
	c.run("sx", "Bash", "rm -rf .git/agent-leases", "", R2)
	c.check("L8 임대 폴더 지우기 차단(혼자여도)", 2, "사람만")
	c.run("sx", "Bash", "echo AGENT_LEASE_OFF", "", R2)
	c.check("L8 AGENT_LEASE_OFF 글자가 든 Bash 차단", 2)
	hookw("s3b", "Write", R3+"/.claude/settings.local.json", `{"disableAllHooks": true}`)
	c.check("L8 Write로 disableAllHooks 넣기 차단", 2, "사람만")
	hookw("s3b", "Edit", H+"/.claude/settings.json", `"AGENT_LEASE_OFF": "1"`)
	c.check("L8 Edit로 AGENT_LEASE_OFF 넣기 차단", 2)
	hookw("s3b", "Write", R3+"/.claude/settings.local.json", `{"permissions": {}}`)
	c.check("L8 다른 설정 편집은 허용", 0)

	L3D := c.leaseDirs(R3)
	for k := 1; k <= 300; k++ {
		c.write(fmt.Sprintf("%s/dead%d/owner", L3D, k), fmt.Sprintf("sz\x1f%d\x1fMon Jan  1 00:00:00 2001\x1f\x1fgone\x1f2026-01-01 00:00:00\x1fghost%d.txt\n", 90000+k, k))
	}
	c.write(L3D+"/deaddirty/owner", "sz\x1f99999\x1fMon Jan  1 00:00:00 2001\x1f\x1fgone\x1f2026-01-01 00:00:00\x1fd.txt\n")
	c.appendf(R3+"/d.txt", "q\n")
	n0 := c.lsCount(L3D, "")
	c.run("s3b", "Bash", "git add b.txt", "", R3)
	c.check("H2 임대를 정리하는 판정도 통과", 0)
	n1 := c.lsCount(L3D, "")
	c.checkOK(fmt.Sprintf("H2 죽은 소유자의 깨끗한 임대 300개만 지움(n0=%d n1=%d, 미커밋·산 임대는 남김)", n0, n1),
		n0-n1 == 300 && isDir(L3D+"/deaddirty") && exists(c.leasefile(R3, "src/pkg/x.go")), 0)

	// ---------- 수정 2차 (재감사: N1 남은 claim, N3 큰따옴표 안 heredoc, N5 git status 실패) ----------
	R4 := T + "/r4"
	c.mkrepo(R4)
	A4 := c.agent()
	c.reg(A4, "s4a", R4, "agent-a4")
	B4 := c.agent()
	c.reg(B4, "s4b", R4, "agent-b4")
	L4D := c.leaseDirs(R4)
	c.fatal(os.MkdirAll(L4D, 0o755))
	// deadlease REL [old|new|olddir] — a dead owner's lease on REL, optionally with a claim left behind
	deadlease := func(rel, mode string) string {
		d := L4D + "/" + md5hex(rel)
		cl := d + "/claim-99991-20260101000000"
		c.write(d+"/owner", "sz\x1f99991\x1fMon Jan  1 00:00:00 2001\x1f\x1fgone\x1f2026-01-01 00:00:00\x1f"+rel+"\n")
		switch mode {
		case "old":
			c.write(cl, "")
			c.touchOld(cl)
		case "new":
			c.write(cl, "")
		case "olddir":
			c.fatal(os.Mkdir(cl, 0o755))
			c.touchOld(cl)
		}
		return d
	}
	d := deadlease("b.txt", "old")
	c.run("s4b", "Edit", R4+"/b.txt", "", R4)
	c.check("N1 60초 넘게 묵은 claim은 지우고 넘겨받음", 0)
	c.checkOK("N1 넘겨받은 뒤 소유자는 B", field(d+"/owner", 2) == strconv.Itoa(B4), 0)
	deadlease("d.txt", "new")
	c.run("s4b", "Edit", R4+"/d.txt", "", R4)
	c.check("N1 방금 생긴 claim은 막되 곧바로 되풀이하지 말라고 알림", 2, "60초")
	deadlease("e.txt", "olddir")
	c.run("s4b", "Edit", R4+"/e.txt", "", R4)
	c.check("N1 옛 판의 claim 폴더도 묵었으면 넘겨받음", 0)
	d = deadlease("f.txt", "old")
	c.run("s4b", "Bash", "git add a.txt", "", R4)
	c.checkOK("N1 정리도 묵은 claim을 지우고 그 임대를 치움", isDir(d), 1)
	for k := 1; k <= 200; k++ {
		deadlease(fmt.Sprintf("ghost%d.txt", k), "")
	}
	c.run("s4b", "Bash", "git add a.txt", "", R4)
	left := 0
	for k := 1; k <= 200; k++ {
		if isDir(L4D + "/" + md5hex(fmt.Sprintf("ghost%d.txt", k))) {
			left++
		}
	}
	c.RC, c.OUT, c.ERR = left, "", ""
	c.check("N1 정리한 임대는 훅이 끝나기 전에 제자리에서 사라짐(claim만 남는 틈 없음)", 0)

	c.run("s4a", "Edit", R4+"/g.txt", "", R4)
	c.appendf(R4+"/g.txt", "y\n")
	c.run("s4b", "Bash", "git commit -m \"$(cat <<'END-MSG'\nmsg\nEND-MSG\n)\" -- b.txt && git stash", "", R4)
	c.check("N3 하이픈 든 heredoc 구분자 뒤 git stash도 검사", 2)
	c.run("s4b", "Bash", `echo "$(date) <<EOF" ; git stash`, "", R4)
	c.check("N3 heredoc이 아닌 <<낱말 뒤 git stash도 검사", 2)
	c.run("s4b", "Bash", "git commit -m \"$(cat <<'END-MSG'\nfix: 27\" 모니터\ngit stash 언급\nEND-MSG\n)\" -- b.txt", "", R4)
	c.check("N3 하이픈 구분자 heredoc 본문은 여전히 건너뜀", 0)

	d = deadlease("h.txt", "")
	c.appendf(R4+"/h.txt", "z\n")
	idx, err := os.ReadFile(R4 + "/.git/index")
	c.fatal(err)
	c.write(R4+"/.git/index", "garbage")
	c.run("s4b", "Bash", "git add a.txt", "", R4)
	c.checkOK("N5 git status가 실패하면 정리를 건너뜀(죽은 소유자의 미커밋 임대 보존)", isDir(d), 0)
	c.write(R4+"/.git/index", string(idx))
}
