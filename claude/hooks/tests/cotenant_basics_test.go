package tests

// cotenant-guard 시나리오 앞 절: 저장소 R(혼자 → 동거), 동시 첫 편집, 죽은 주인, 끄기, 나무 경계, hc ls.

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func cotenantBasics(c *cg) (A, B, C int) {
	T, H := c.T, c.H
	c.fatal(os.MkdirAll(H+"/.claude/sessions", 0o755))
	c.fatal(os.MkdirAll(T+"/bin", 0o755))
	c.write(H+"/.claude/settings.json", "{\"env\":{}}\n")
	c.fatal(os.WriteFile(T+"/bin/hc", []byte(fmt.Sprintf("#!/bin/sh\ncat \"%s/hc.json\" 2>/dev/null || echo '{\"terminals\":[]}'\n", T)), 0o755))

	// ---------- repo R: agent A alone, then B joins ----------
	R := T + "/r"
	c.R = R
	c.mkrepo(R)
	A = c.agent()
	c.reg(A, "sa", R, "agent-a")
	c.run("sa", "Edit", R+"/a.txt")
	c.check("동거 없음: 편집 허용", 0)
	c.checkOK("동거 없음: 임대는 기록(pid A)", exists(c.leasefile(R, "a.txt")) && field(c.leasefile(R, "a.txt"), 2) == strconv.Itoa(A), 0)
	c.checkOK("임대에 레지스트리 이름 기록", field(c.leasefile(R, "a.txt"), 5) == "agent-a", 0)
	c.appendf(R+"/a.txt", "more\n")
	c.run("sa", "Bash", "git add -A && git status")
	c.check("동거 없음: git add -A 차단 없음", 0)
	c.run("sa", "Edit", R+"/a.txt")
	c.check("내 임대는 계속 허용", 0)
	// 명령 분석만 보는 단언(강제 push·감싼 명령·감싸기 문법 표)은 TestCotenantCommands가 따로 돈다(cotenantCommands)
	return cotenantBasicsB(c, R, A)
}

// TestCotenantCommands: 명령 분석만 보는 단언 — 동거 에이전트 없이 저장소 하나로 따로 돈다(아스트라 M07: 시나리오 한가운데 있어
// -run으로 이것만 골라도 앞 준비(에이전트 등록·ps)가 다 돌고, 준비가 깨지면 하나도 못 돌았다). 단언마다 훅 호출과 같은 하위 시험이라
// -run으로 하나를 고르면 그 훅만 돈다(재감사 R-M05). sa는 고정 시작 시각으로 등록해 정상 쓰기도 ps에 기대지 않는다
func TestCotenantCommands(t *testing.T) {
	c := newCG(t)
	c.R = c.T + "/r"
	c.mkrepo(c.R)
	c.reg(c.agent(), "sa", c.T, "agent-a", deadST)
	cotenantCommands(c, c.R)
}

// runCheck NAME WANT PATTERN SESSION TOOL ARG: 준비가 필요 없는 단언 하나 — 훅 호출과 단언을 같은 하위 시험 안에서
func (c *cg) runCheck(name string, want int, pat, s, tool, arg string) {
	c.t.Helper()
	c.t.Run(name, func(t *testing.T) {
		cc := *c
		cc.t = t
		cc.run(s, tool, arg)
		if cc.RC != want || (pat != "" && !regexp.MustCompile(pat).MatchString(cc.ERR+"\n"+cc.OUT)) {
			t.Errorf("rc=%d, want %d %q err=[%s] out=[%s]", cc.RC, want, pat, cc.ERR, cc.OUT)
		}
	})
}

// cotenantCommands: 임대·동거와 상관없는 명령 분석 단언(세션 sa, 저장소 R)
func cotenantCommands(c *cg, R string) {
	// force push is refused always, cotenant or not (2026-11-17: the coordinator force-pushed a shared temp ref)
	for _, cmd := range []string{"git push -f origin main", "git push --force origin main", "git push -qf origin main", "git push --force-with-lease origin main",
		"git push --force-with-lease=main origin main", "git push -f --force-with-lease=main:abc origin main",
		"git push --force-with-lease=main:abc origin +main", "git push --force-with-lease --force-with-lease=main:abc origin main",
		"git push --force-if-includes origin main", "git push --force-if-includes --force-with-lease origin main", "git push --mirror origin",
		"git push -q origin +HEAD:refs/integ/x", "git push origin '+HEAD:refs/integ/x'", `git push origin "+main"`,
		"git -C " + R + " push -f origin main", "env HERDR_LAND=1 git push --force origin main", "git -c x.y=1 push -f origin main",
		"git fetch && git push -f origin main", "true; git push -f origin main", "echo x | git push -f origin main",
		"bash -c 'git push -f origin main'", "x=$(git push -f origin main)", "git -C /nonexistent push -f origin main"} {
		c.runCheck("강제 push 차단(동거 없음): "+cmd, 2, "강제 push.*기대값을 적은 --force-with-lease", "sa", "Bash", cmd)
	}
	// audit M1: remote wrappers (hc exec/run --on … --, ssh <host> …) and nested shells; L1: git -c alias/mirror/+push
	for _, cmd := range []string{"hc exec --on mini -- git push -f origin main", "hc exec --on mini --timeout 60 -- bash -lc 'git push -f origin main'",
		"ssh mini 'cd x && git push -f origin main'", "ssh -p 22 -o BatchMode=yes mini git push --force origin main",
		"ssh mini -- git push origin +main", "hc run --bg --on mini --name x -- git push --mirror origin",
		`sh -c "bash -lc 'zsh -c \"git push -f origin main\"'"`, `ssh mini "hc exec --on m2 -- sh -c 'git push -f origin main'"`,
		"git -c alias.p='push -f' p origin main", "git -c remote.origin.mirror=true push origin", "git -c remote.origin.push=+refs/heads/*:refs/heads/* push origin"} {
		c.runCheck("감싼 강제 push 차단: "+cmd, 2, "강제 push", "sa", "Bash", cmd)
	}
	for _, cmd := range []string{"hc exec --on mini -- git push origin main", "ssh mini 'git push --force-with-lease=r:abc origin HEAD:r'",
		"hc exec --on mini -- git fetch origin +refs/heads/*:refs/remotes/origin/*", "ssh mini 'echo git push -f'",
		`hc exec --on mini -- bash -lc 'git push -q origin "${c}:refs/claims/$n" --force-with-lease="refs/claims/$n:"'`,
		"hc send t1 'git push -f 금지'", "git -c alias.st=status st", "ssh -t mini", "hc exec --on mini -- ls"} {
		c.runCheck("감싼 정상 명령은 허용: "+cmd, 0, "", "sa", "Bash", cmd)
	}
	// 2026-11-17 cg-multibyte: awk splits the command into bytes and cuts long words at 1024 bytes, mid-character for
	// Korean/emoji; in a UTF-8 locale awk's regex then died (towc: multibyte conversion failure) = "Bash 명령 분석 실패"
	// on a plain hc send. Pads 0..3 move the cut across every byte of a 3- and 4-byte character; verdicts stay the same
	for _, loc := range []string{"ko_KR.UTF-8", "C"} {
		for p := 0; p <= 3; p++ {
			pad := strings.Repeat("x", p)
			for _, body := range []string{strings.Repeat("가나다라 ", 120), strings.Repeat("🙂한글 ", 120), strings.Repeat("깨�진 ", 150)} {
				b2 := string([]rune(body)[:2])
				c.hookEnv = []string{"LC_ALL=" + loc}
				c.runCheck(fmt.Sprintf("긴 한국어 글 hc send 허용(%s, 자름 위치 +%d, %s)", loc, p, b2), 0, "",
					"sa", "Bash", "MSG='"+pad+body+`'; for t in a/t1 a/t2; do hc send "$t" "$MSG" 2>&1 | tail -1; done; hc read b/t3 2>&1 | grep -E "Model:|effort" | tail -1`)
				c.runCheck(fmt.Sprintf("긴 한국어 글 뒤 강제 push는 그대로 차단(%s, +%d, %s)", loc, p, b2), 2, "강제 push",
					"sa", "Bash", "MSG='"+pad+body+"'; git push -f origin main")
				c.runCheck(fmt.Sprintf("긴 한국어 글이 든 bash -c 안 강제 push 차단(%s, +%d, %s)", loc, p, b2), 2, "강제 push",
					"sa", "Bash", "bash -c 'git push -f origin main; echo "+pad+body+"'")
				c.hookEnv = nil
			}
		}
	}
	// audit 2 M1-b: wrappers by their whole option grammar (hc exec/run --help, ssh(1)), -- or not, value options before/after,
	// nested; one table: verdict<TAB>command
	out, err := exec.Command("bash", "-c", `deep='git push origin main'; for i in 1 2 3 4 5 6; do deep="sh -c $(printf %q "$deep")"; done; printf %s "$deep"`).Output()
	c.fatal(err)
	deep := string(out)
	table := `2	hc exec --on mini-m4 git push -f origin main
2	hc exec --on mini-m4 bash -lc 'git push -f origin main'
2	hc exec --on mini-m4 --cwd /x git push --force origin main
2	hc exec --on=mini --timeout=30 -i --env K=V git push -f origin main
2	hc exec --json --on mini --stdin git push origin +main
2	hc run --bg --on mini-m4 --name x 'cd app && git push -f origin main'
2	hc run --on vps "git push --mirror origin"
2	hc run --bg --on m --name x --cwd /w zsh -c 'git push -f origin main'
2	hc run --label a=b --parent t1 --no-parent --no-notify --outlive-parent --rm --keep --timeout 9 --on m git push origin +main
2	ssh -i ~/.ssh/k -J jump -l me -F cfg mini 'git push -f origin main'
2	ssh -tp 22 mini git push -f origin main
2	ssh -oBatchMode=yes -A mini git push --force origin main
2	ssh -p22 -E log -b 1.2.3.4 me@mini "bash -lc 'git push -f origin main'"
2	hc exec --on a ssh b "hc run --on c 'sh -c \"git push -f origin main\"'"
2	ssh a hc exec --on b zsh -c "'git push --mirror origin'"
2	git -c alias.p=push p origin +main
2	git -c alias.p='!git push -f' p origin main
2	` + deep + `
0	hc run --bg --on m --name x claude "git push -f 쓰지 마"
0	hc run reviewer "review src/x.rs"
0	hc exec --on m git push origin main
0	hc exec --on m ls -f
0	hc exec --on m git push -q origin "c:refs/claims/n" --force-with-lease=refs/claims/n:
0	hc run --on m 'git push -q origin --delete refs/claims/n --force-with-lease=refs/claims/n:abc'
0	git -c alias.pf='push --force-with-lease=main:abc' pf origin main
0	hc-lane send --all -- 'git push --force 금지'
0	ssh mini 'git log -f'
0	ssh -f -p 22 mini 'sleep 1'
2	git -c alias.p='push -f' -c alias.q=status p origin main
2	git -c alias.P='push -f' p origin main
2	bash +x -c 'git push -f origin main'
2	hc exec --on m bash +o posix -c 'git push -f origin main'
2	hc --json exec --on m git push -f origin main
0	git -c alias.p='push -f' -c alias.p=status p
0	bash +x script.sh -f`
	for _, line := range strings.Split(table, "\n") {
		want, cmd, _ := strings.Cut(line, "\t")
		if cmd == "" {
			continue
		}
		if want == "2" {
			c.runCheck("감싸기 문법 표 막힘: "+cmd, 2, "강제 push|겹침", "sa", "Bash", cmd)
		} else {
			c.runCheck("감싸기 문법 표 허용: "+cmd, 0, "", "sa", "Bash", cmd)
		}
	}
	for _, cmd := range []string{"git push origin main", "git push -u origin HEAD", "git push origin --delete refs/integ/x", "git push origin :refs/integ/x",
		"git push -o ci.skip origin main", "git fetch origin +refs/heads/*:refs/remotes/origin/*", "git fetch -f origin",
		"git commit -m 'git push -f 금지' -- a.txt", "echo 'git push --force'",
		"git push --force-with-lease=main:abc origin main", "git push --force-with-lease=main:abc --force-if-includes origin main",
		`git push -q origin "${c}:refs/claims/$n" --force-with-lease="refs/claims/$n:" && echo "$n $c" >> ev/claims.txt`,
		`git push -q origin "${c}:refs/claims/$n" --force-with-lease="refs/claims/$n:0123abcd"`,
		`git push -q origin "${c2}:refs/claims/$n" --force-with-lease="refs/claims/$n:$old"`,
		`git push -q origin --delete "refs/claims/$n" --force-with-lease="refs/claims/$n:$c"`,
		`git push -q origin --delete refs/wt-claims/x --force-with-lease=refs/wt-claims/x:0123abcd`} {
		c.runCheck("강제 push 아님(기대값 적은 lease·선점 포함)은 허용: "+cmd, 0, "", "sa", "Bash", cmd)
	}
}

// cotenantBasicsB: 기본 시나리오 뒤쪽 — B 합류부터
func cotenantBasicsB(c *cg, R string, a int) (A, B, C int) {
	A = a
	T, H := c.T, c.H
	c.write(H+"/.claude/sessions/1.json", `{"pid": 1, "sessionId": "broken`) // a half-written registry file
	B = c.agent()
	c.reg(B, "sb", R, "agent-b")
	c.run("sb", "Edit", R+"/a.txt")
	c.check("혼자 고치던 파일을 뒤에 합류한 쪽이 못 가져감", 2, "agent-a")
	c.run("sb", "Write", R+"/a.txt")
	c.check("Write도 같은 판정", 2, "고치는 중")
	c.run("sb", "Bash", "git add -A")
	c.check("동거: git add -A 차단", 2, "git add -- <경로>")
	c.run("sb", "Bash", "cd sub && git add .")
	c.check("동거: git add . 차단", 2)
	c.run("sb", "Bash", "git commit -am fix")
	c.check("동거: git commit -am 차단", 2)
	c.run("sb", "Bash", "git commit --all -m x")
	c.check("동거: git commit --all 차단", 2)
	c.run("sb", "Bash", "git stash")
	c.check("동거: git stash 차단", 2, "지우거나 숨길")
	c.run("sb", "Bash", "ssh mini 'git add -A && git stash'")
	c.check("동거여도 원격(ssh) 명령은 이 트리 동거 판정 안 함", 0)
	c.run("sb", "Bash", "hc exec --on mini -- git stash")
	c.check("동거여도 원격(hc exec) 명령은 이 트리 동거 판정 안 함", 0)
	c.run("sb", "Bash", "hc exec --on mini -- git push -f origin main")
	c.check("동거 중 원격 강제 push는 막힘", 2, "강제 push")
	c.run("sb", "Bash", "hc exec -- git stash")
	c.check("동거 중 --on 없는 로컬 hc exec는 이 트리 규칙(감사2 L-c)", 2, "지우거나 숨길")
	c.run("sb", "Bash", "git stash list")
	c.check("git stash list는 허용", 0)
	c.run("sb", "Bash", "git reset --hard HEAD")
	c.check("동거: git reset --hard 차단", 2)
	c.run("sb", "Bash", "git checkout -- b.txt")
	c.check("동거: git checkout -- 차단", 2)
	c.run("sb", "Bash", "git checkout .")
	c.check("동거: git checkout . 차단", 2)
	c.run("sb", "Bash", "git restore b.txt")
	c.check("동거: git restore 차단", 2)
	c.run("sb", "Bash", "git restore --staged b.txt")
	c.check("git restore --staged는 허용", 0)
	c.run("sb", "Bash", "git clean -fd")
	c.check("동거: git clean 차단", 2)
	c.run("sb", "Bash", "git clean -n")
	c.check("git clean -n은 허용", 0)
	c.run("sb", "Edit", R+"/b.txt")
	c.check("남이 안 잡은 깨끗한 파일 편집 허용", 0)
	c.appendf(R+"/b.txt", "more\n")
	c.run("sb", "Bash", "git add b.txt && git commit -m 'b: git add -A 금지' -- b.txt")
	c.check("git add <내 파일> 허용, 메시지 속 글자는 무시", 0)
	c.run("sb", "Bash", "git add a.txt")
	c.check("git add에 남의 임대 파일 차단", 2, "agent-a")
	c.run("sb", "Bash", "git -C "+R+" add -- .")
	c.check("git -C 경로 add . 차단", 2)
	c.run("sb", "Bash", "git rebase origin/main")
	c.check("남의 미커밋 파일이 있으면 rebase 차단 + 탈출로", 2, "worktree add --detach ../r.wt/land-")
	c.run("sb", "Bash", "git rebase origin/main")
	c.check("착지 탈출로 문구: origin 기준 + cherry-pick", 2, "origin/<기본 브랜치>.*cherry-pick")
	c.run("sb", "Bash", "git pull --rebase")
	c.check("pull 차단", 2)
	c.run("sb", "Bash", "git checkout -b x")
	c.check("checkout -b 차단", 2)
	c.run("sb", "Bash", "git merge --abort")
	c.check("merge --abort는 허용", 0)
	c.run("sb", "Bash", "sed -i '' 's/a/b/' a.txt")
	c.check("남의 임대 파일 sed -i 차단", 2)
	c.run("sb", "Bash", "echo hi > a.txt")
	c.check("남의 임대 파일 > 차단", 2)
	c.run("sb", "Bash", "echo hi >> "+R+"/a.txt")
	c.check("남의 임대 파일 절대 경로 >> 차단", 2)
	c.run("sb", "Bash", "rm -f ./a.txt")
	c.check("남의 임대 파일 rm 차단", 2)
	c.run("sb", "Bash", "cp b.txt a.txt")
	c.check("남의 임대 파일 cp 대상 차단", 2)
	c.run("sb", "Bash", "mv a.txt z.txt")
	c.check("남의 임대 파일 mv 차단", 2)
	c.run("sb", "Bash", "cat a.txt > /dev/null 2>&1; wc -l a.txt")
	c.check("남의 파일 읽기만은 허용", 0)
	c.run("sb", "Bash", "printf 'x\\n' > b.txt")
	c.check("내 파일 > 허용", 0)
	c.run("sb", "Bash", "cat > notes.txt <<'EOF'\ngit add -A\nrm a.txt\nEOF")
	c.check("heredoc 본문은 명령이 아님", 0)
	c.run("sb", "Bash", "git commit -m \"$(cat <<'EOF'\nfix: git add -A 규칙\nEOF\n)\" -- b.txt")
	c.check("commit 메시지 heredoc 속 git add -A 무시", 0)
	c.run("sb", "Bash", "echo 'git add -A'")
	c.check("따옴표 속 명령은 무시", 0)
	c.run("sb", "Bash", "x=$(git stash)")
	c.check("$( ) 안의 명령도 검사", 2)

	// Bash writes lease too: B's own redirect target is B's afterwards
	c.run("sb", "Bash", "echo new > made-by-b.txt")
	c.check("Bash 쓰기 대상도 임대", 0)
	c.write(R+"/made-by-b.txt", "new\n")
	c.run("sb", "Edit", R+"/made-by-b.txt")
	c.check("내가 Bash로 만든 파일은 Edit 허용", 0)
	c.run("sa", "Edit", R+"/made-by-b.txt")
	c.check("남이 Bash로 만든 파일은 막힘", 2, "agent-b")

	// ---------- concurrent first edits of the same new file: exactly one wins ----------
	for k := 1; k <= 5; k++ {
		var wg sync.WaitGroup
		var ra, rb int
		for i, s := range []string{"sa", "sb"} {
			wg.Add(1)
			go func(i int, s string) {
				defer wg.Done()
				cmd := hookExec(c.hook, c.hookIn(s, "Write", fmt.Sprintf("%s/new%d.txt", R, k), "", ""), c.env(c.stdPath()), T)
				r := 0
				if err := cmd.Run(); err != nil {
					r = -100
					if ee, ok := err.(*exec.ExitError); ok {
						r = ee.ExitCode()
					}
				}
				if i == 0 {
					ra = r
				} else {
					rb = r
				}
			}(i, s)
		}
		wg.Wait()
		// 종료 코드가 정확히 {0, 2}이고 임대 주인이 이긴 쪽 세션(합이 2인 것만 보면 둘 다 1로 실패해도 통과 — 아스트라 E08)
		win := "sa"
		if ra != 0 {
			win = "sb"
		}
		ok := ((ra == 0 && rb == 2) || (ra == 2 && rb == 0)) && field(c.leasefile(R, fmt.Sprintf("new%d.txt", k)), 1) == win
		c.RC, c.OUT, c.ERR = rc(ok), fmt.Sprintf("sa %d · sb %d", ra, rb), ""
		c.check(fmt.Sprintf("동시 두 획득 중 하나만 성공 (%d)", k), 0)
	}

	// ---------- dead owner and pid reuse ----------
	C = c.agent()
	c.reg(C, "sc", R, "agent-c")
	c.run("sc", "Edit", R+"/d.txt")
	c.appendf(R+"/d.txt", "more\n")
	c.killAgent(C)
	c.run("sb", "Edit", R+"/d.txt")
	c.check("죽은 소유자 파일은 인수 + 경고", 0, "끝난 에이전트 agent-c")
	c.checkOK("인수 뒤 소유자는 B", field(c.leasefile(R, "d.txt"), 2) == strconv.Itoa(B), 0)
	c.write(c.leasefile(R, "e.txt"), fmt.Sprintf("sz\x1f%d\x1fMon Jan  1 00:00:00 2001\x1f\x1freused\x1f2026-01-01 00:00:00\x1fe.txt\n", A))
	c.appendf(R+"/e.txt", "more\n")
	c.run("sb", "Edit", R+"/e.txt")
	c.check("pid 재사용(procStart 불일치)은 죽은 것으로 인수 + 경고", 0, "끝난 에이전트 reused")

	// ---------- unknown owner, idle hand-over, subagent ----------
	c.appendf(R+"/f.txt", "more\n")
	c.run("sb", "Edit", R+"/f.txt")
	c.check("주인 불명 dirty 파일 차단(동거)", 2, "임대 기록 없는")
	c.run("sa", "Edit", R+"/g.txt")
	c.run("sb", "Edit", R+"/g.txt")
	c.check("남의 최근 임대(깨끗해도) 차단", 2, "10분 안")
	c.touchOld(c.leasefile(R, "g.txt"))
	c.run("sb", "Edit", R+"/g.txt")
	c.check("10분 넘게 손 안 댄 깨끗한 남의 파일은 넘겨받음", 0)
	c.run("sa", "Edit", R+"/a.txt", "agent-sub1")
	c.check("같은 세션 서브에이전트는 경고만", 0, "같은 세션의 다른 에이전트")

	// ---------- off switch ----------
	c.write(H+"/.claude/settings.json", "{\"env\":{\"AGENT_LEASE_OFF\":\"1\"}}\n")
	c.run("sb", "Edit", R+"/a.txt")
	c.check("settings.json env AGENT_LEASE_OFF=1이면 허용", 0)
	c.write(H+"/.claude/settings.json", "{\"env\":{}}\n")
	c.hookEnv = []string{"AGENT_LEASE_OFF=1"}
	c.run("sb", "Edit", R+"/a.txt")
	c.hookEnv = nil
	c.check("환경 변수만으로는 꺼지지 않음", 2)

	// ---------- tree boundaries: subfolder, symlink, linked worktree, nested worktree, sibling name ----------
	R2 := T + "/r2"
	c.mkrepo(R2)
	X := c.agent()
	c.reg(X, "sx", R2, "agent-x")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("R2 혼자: git add -A 허용", 0)
	Y := c.agent()
	c.reg(Y, "sy", R2+"/sub", "agent-y")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("하위 폴더 cwd도 같은 트리(동거)", 2, "agent-y")
	c.unreg(Y)
	c.fatal(os.Symlink(R2, T+"/r2link"))
	c.reg(Y, "sy", T+"/r2link", "agent-y")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("심볼릭 링크 cwd도 같은 트리(동거)", 2, "agent-y")
	c.run("sx", "Edit", T+"/r2link/a.txt", "", R2)
	c.appendf(R2+"/a.txt", "more\n")
	c.run("sy", "Edit", R2+"/a.txt", "", R2)
	c.check("심볼릭 링크로 잡은 임대 = 같은 파일", 2, "agent-x")
	c.unreg(Y)
	c.git(R2, "worktree", "add", "-q", T+"/r2wt", "-b", "wt")
	c.reg(Y, "sy", T+"/r2wt", "agent-y")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("linked worktree는 다른 트리(동거 아님)", 0)
	c.run("sy", "Edit", T+"/r2wt/a.txt", "", T+"/r2wt")
	c.check("linked worktree의 같은 경로 파일은 따로", 0)
	c.run("sy", "Bash", "cd "+T+"/r2wt && git rebase main", "", R2)
	c.check("cd로 다른 워크트리에서 rebase 허용", 0)
	c.unreg(Y)
	c.git(R2, "worktree", "add", "-q", R2+"/.claude/worktrees/w", "-b", "nested")
	c.reg(Y, "sy", R2+"/.claude/worktrees/w", "agent-y")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("트리 안에 둔 worktree는 다른 트리", 0)
	c.unreg(Y)
	c.mkrepo(T + "/r2.web")
	c.reg(Y, "sy", T+"/r2.web", "agent-y")
	c.run("sx", "Bash", "git add -A", "", R2)
	c.check("이름이 비슷한 형제 폴더(r2.web)는 다른 트리", 0)
	c.run("sx", "Edit", "/etc/hosts", "", R2)
	c.check("git 밖 파일은 허용", 0)

	// ---------- hc ls as a second signal ----------
	c.write(T+"/hc.json", fmt.Sprintf(`{"terminals":[{"address":"t9","agent":"codex","cwd":"%s","state":"live","name":"codex-lane"}]}`+"\n", R2))
	c.run("sx", "Bash", "git stash", "", R2)
	c.check("hc ls의 codex 터미널도 동거", 2, "codex-lane")
	c.write(T+"/hc.json", fmt.Sprintf(`{"terminals":[{"address":"t9","agent":"zsh","cwd":"%s","state":"live"},{"address":"w1/t1","agent":"codex","cwd":"%s","state":"live"}]}`+"\n", R2, R2))
	c.run("sx", "Bash", "git stash", "", R2)
	c.check("셸·다른 기계 터미널은 동거 아님", 0)
	c.fatal(os.Remove(T + "/hc.json"))
	return A, B, C
}
