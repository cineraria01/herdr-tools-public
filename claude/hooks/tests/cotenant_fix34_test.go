package tests

// cotenant-guard 시나리오: 수정 3차(10개 동시 시험 stress10)·4차(재확인 1), 작업 중 발견(글롭).

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func cotenantFix34(c *cg) {
	T := c.T
	R, R2 := c.R, T+"/r2"
	var d string
	// ---------- 수정 3차 (10개 동시 시험 stress10: S1~S7, 막힘 문구) ----------
	R5 := T + "/r5"
	c.mkrepo(R5)
	c.write(R5+"/.gitignore", ".qa/\n")
	c.git(R5, "add", ".gitignore")
	c.git(R5, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "ignore")
	A5 := c.agent()
	c.reg(A5, "s5a", R5, "agent-a5")
	B5 := c.agent()
	c.reg(B5, "s5b", R5, "agent-b5")
	ST5 := procStart(c, A5)
	L5D := c.leaseDirs(R5)
	c.fatal(os.MkdirAll(L5D, 0o755))

	g := c.rec(L5D, "ghost1.txt", 99992, deadST, false)
	c.write(L5D+"/.cleaning", "")
	c.run("s5b", "Bash", "git add b.txt", "", R5)
	c.checkOK("S1 다른 쪽이 정리 중(정리 잠금 60초 안)이면 정리를 건너뜀", isDir(g), 0)
	c.touchOld(L5D + "/.cleaning")
	c.run("s5b", "Bash", "git add b.txt", "", R5)
	c.checkOK("S1 묵은 정리 잠금(60초 넘음)은 치우고 정리", isDir(g), 1)
	c.checkOK("S1 정리가 끝나면 정리 잠금을 남기지 않음", exists(L5D+"/.cleaning"), 1)

	i1 := c.rec(L5D, "idle.txt", A5, ST5, true)
	i2 := c.rec(L5D, "recent.txt", A5, ST5, false)
	i3 := c.rec(L5D, "d.txt", A5, ST5, true)
	c.appendf(R5+"/d.txt", "y\n")
	c.write(R5+"/.qa/r.md", "r\n")
	i4 := c.rec(L5D, ".qa/r.md", A5, ST5, false)
	c.run("s5b", "Bash", "git add b.txt", "", R5)
	c.checkOK("S2 산 소유자의 깨끗하고 10분 넘게 안 쓴 임대는 정리(넘겨받기와 같은 조건)", isDir(i1), 1)
	c.checkOK("S2 산 소유자의 최근 임대와 미커밋 임대는 남김", isDir(i2) && isDir(i3), 0)
	c.checkOK("S2 산 소유자의 무시된 파일 임대는 10분 안이어도 정리(유예 없음 규칙과 같음)", isDir(i4), 1)

	d = c.rec(L5D, "s3.txt", 99993, deadST, false)
	cl := d + "/claim-99993-20260101000000"
	c.write(cl, "")
	c.touchOld(cl)
	c.write(fmt.Sprintf("%s.steal-%d", cl, mtime(c, cl)), "")
	c.run("s5b", "Edit", R5+"/s3.txt", "", R5)
	c.check("S3 묵은 claim을 다른 쪽이 이미 치우는 중(steal 표식)이면 막힘", 2, "넘겨받기")
	o := L5D + "/" + md5hex("s4.txt")
	c.write(o+"/claim-orphan", "")
	c.touchOld(o)
	c.run("s5b", "Write", R5+"/s4.txt", "", R5)
	c.check("S4 주인 기록 없는 임대 폴더는 고정 이름 claim(새것이 있으면 막힘)", 2, "넘겨받기")

	c.appendf(R5+"/b.txt", "y\n")
	c.run("s5b", "Bash", "git commit -m 'b'", "", R5)
	c.check("S5 동거 중 경로 없는 git commit은 stage에 남의 파일이 없어도 막고 경로를 안내", 2, "git commit -m … -- <경로>")
	c.run("s5b", "Bash", "git commit -m b --", "", R5)
	c.check("S5 -- 뒤에 경로가 없어도 경로 없는 commit", 2)
	c.run("s5b", "Bash", "git commit -m b -- b.txt", "", R5)
	c.check("S5 경로 지정 commit은 허용", 0)
	c.run("sx", "Bash", "git commit -m x", "", R2)
	c.check("S5 혼자면 경로 없는 git commit 허용", 0)
	c.run("s5b", "Bash", "git commit --amend -m b -- b.txt", "", R5)
	c.check("S6 동거 중 --amend는 경로가 있어도 막힘", 2, "amend")
	c.run("s5b", "Bash", "git commit --amen --no-edit", "", R5)
	c.check("S6 --amend 줄임(--amen)도 막힘", 2)
	c.run("sx", "Bash", "git commit --amend --no-edit", "", R2)
	c.check("S6 혼자면 --amend 허용", 0)

	c.run("s5a", "Edit", R5+"/e.txt", "", R5)
	c.appendf(R5+"/e.txt", "y\n")
	c.exec1("/bin/bash", c.hook, obj("session_id", "s5b", "cwd", R5, "tool_name", "Edit", "tool_input", obj("file_path", R5+"/e.txt")), c.env(T+"/bin"))
	c.check("S7 PATH에 git이 없어도 같은 판정(남의 미커밋 파일 차단)", 2, "agent-a5")
	c.fatal(os.WriteFile(T+"/badgit", []byte("#!/bin/sh\necho broken >&2\nexit 1\n"), 0o755))
	// 셸 판은 훅 파일을 sed로 고친 사본, Go 판은 시험 빌드의 손잡이(사본은 go-run.sh를 못 찾아 셸 판으로 돌았을 것)
	src, err := os.ReadFile(c.hook)
	c.fatal(err)
	goImpl := isGo(c.impl)
	badgitHook := c.hook
	if goImpl {
		c.hookEnv = []string{"HERDR_HOOKS_TEST_GIT=" + T + "/badgit"}
	} else {
		badgitHook = T + "/hook-badgit.sh"
		c.write(badgitHook, regexp.MustCompile(`(?m)^GIT=.*$`).ReplaceAllLiteralString(string(src), "GIT="+T+"/badgit"))
	}
	c.exec1("/bin/bash", badgitHook, obj("session_id", "sx", "cwd", R2, "tool_name", "Edit", "tool_input", obj("file_path", R2+"/a.txt")), c.env(c.stdPath()))
	c.hookEnv = nil
	c.check("S7 git을 실행하지 못하면(종료 128 밖) 허용하지 않고 막음", 2, "git을 실행하지 못했다")
	c.run("s5b", "Edit", R5+"/e.txt", "", R5)
	c.check("문구: 기다리라 대신 다른 일을 먼저 하고 나중에 다시 시도하라", 2, "다른 일을 먼저 하고 나중에 다시 시도")
	c.write(L5D+"/.cleaning", "")
	c.run("s5b", "Bash", "git add e.txt", "", R5)
	c.check("S2 정리 잠금을 남이 쥔 동안(바뀐 경로의 임대만 읽음)도 남의 미커밋 파일 git add 차단", 2, "agent-a5")
	os.Remove(L5D + "/.cleaning") //nolint:errcheck // rm -f

	R6 := T + "/r6"
	c.mkrepo(R6)
	L6D := c.leaseDirs(R6)
	c.fatal(os.MkdirAll(L6D, 0o755))
	P6 := 0
	for k := 1; k <= 10; k++ {
		p := c.agent()
		c.reg(p, fmt.Sprintf("s6c%d", k), R6, fmt.Sprintf("agent-6c%d", k))
		if k == 1 {
			P6 = p
		}
	}
	ST6 := procStart(c, P6)
	f6 := c.rec(L6D, "sub", P6, ST6, false)
	c.appendf(R6+"/sub/c.txt", "z\n")
	c.run("s6c2", "Bash", "git rebase main", "", R6)
	c.check("S2 옛 판의 폴더 임대 아래 미커밋 파일도 남의 것(모든 임대를 읽을 때)", 2, `sub\(`)
	c.write(L6D+"/.cleaning", "")
	c.run("s6c2", "Bash", "git rebase main", "", R6)
	c.check("S2 같은 것을 정리 잠금을 남이 쥔 동안(바뀐 경로·그 위 폴더의 임대만 읽음)에도", 2, `sub\(`)
	os.Remove(L6D + "/.cleaning") //nolint:errcheck // rm -f
	c.git(R6, "checkout", "-q", "--", "sub/c.txt")
	c.fatal(os.RemoveAll(f6))
	for rep := 1; rep <= 3; rep++ {
		d := c.rec(L6D, fmt.Sprintf("s3-%d.txt", rep), 99994, deadST, false)
		cl := d + "/claim-99994-20260101000000"
		c.write(cl, "")
		c.touchOld(cl)
		c.RC, _ = c.race(10, "s6c", "Write", fmt.Sprintf("%s/s3-%d.txt", R6, rep), R6)
		c.OUT, c.ERR = "", ""
		c.check(fmt.Sprintf("S3 묵은 claim이 남은 죽은 임대를 10개가 동시에 넘겨받으면 하나만 (%d)", rep), 1)
	}
	for rep := 1; rep <= 3; rep++ {
		o := L6D + "/" + md5hex(fmt.Sprintf("s4-%d.txt", rep))
		c.fatal(os.MkdirAll(o, 0o755))
		c.touchOld(o)
		c.RC, _ = c.race(10, "s6c", "Write", fmt.Sprintf("%s/s4-%d.txt", R6, rep), R6)
		c.OUT, c.ERR = "", ""
		c.check(fmt.Sprintf("S4 주인 기록 없는 임대 폴더를 10개가 동시에 넘겨받으면 하나만 (%d)", rep), 1)
	}
	for k := 1; k <= 1000; k++ {
		c.write(fmt.Sprintf("%s/g%d/owner", L6D, k), fmt.Sprintf("sz\x1f99995\x1f%s\x1f\x1fgone\x1f2026-01-01 00:00:00\x1fghost/g%d.txt\n", deadST, k))
	}
	c.calm() // 시간 단언 앞에 부하 확인(아스트라 E12 — 부하가 높으면 timeok가 시간 검사만 건너뛴다. 통과 수는 늘 본다)
	passed, raceMS := c.race(5, "s6c", "Bash", "git add a.txt", R6)
	c.RC, c.OUT, c.ERR = passed, "", ""
	c.check("S1 죽은 임대 1000개를 5개가 동시에 정리해도 모두 통과", 5)
	c.checkOK(fmt.Sprintf("S1 그때 가장 늦은 판정도 4초 안(%dms, 훅 제한 5초)", raceMS), c.timeok(raceMS, 4000), 0)
	for k := 1; k <= 3000; k++ {
		c.write(fmt.Sprintf("%s/v%d/owner", L6D, k), fmt.Sprintf("s\x1f%d\x1f%s\x1f\x1fagent-6c1\x1f2026-11-04 23:00:00\x1flive/x%d.txt\n", P6, ST6, k))
	}
	c.calm()
	t0 := time.Now()
	c.run("s6c2", "Bash", "git add a.txt", "", R6)
	ms := time.Since(t0).Milliseconds()
	c.checkOK(fmt.Sprintf("S2 남의 산 임대 3000개가 있어도 git add 판정 1.5초 안(%dms)", ms), c.RC == 0 && c.timeok(ms, 1500), 0)
	c.checkOK("S2 최근에 쓴 산 임대 3000개는 정리하지 않음", c.lsCount(L6D, "v") == 3000, 0)

	// ---------- 수정 4차 (재확인 1: R1 steal 표식만 남은 claim, R2 정리자 시간 상한) ----------
	c.fatal(os.WriteFile(T+"/date61", []byte("#!/bin/sh\n# the hook's clock 61 s on: a claim re-armed just now has gone stale again\n[ \"$1\" = +%s ] && { echo $(( $(/bin/date +%s) + 61 )); exit 0; }\nexec /bin/date \"$@\"\n"), 0o755))
	if !goImpl {
		c.write(T+"/hook-61.sh", sedLines(string(src), "DATE=/bin/date", "DATE="+T+"/date61"))
	}
	hook61 := func(s, tool, arg string, rest ...string) {
		if goImpl {
			c.hookEnv = []string{"HERDR_HOOKS_TEST_NOW_OFFSET=61"}
			c.run(s, tool, arg, rest...)
			c.hookEnv = nil
			return
		}
		h := c.hook
		c.hook = T + "/hook-61.sh"
		c.run(s, tool, arg, rest...)
		c.hook = h
	}
	d = c.rec(L5D, "r1.txt", 99996, deadST, false)
	cl = d + "/claim-99996-20260101000000"
	c.write(cl, "")
	c.touchOld(cl)
	m := mtime(c, cl)
	c.write(fmt.Sprintf("%s.steal-%d", cl, m), "") // a taker killed right after its marker: the claim and the marker are left
	c.run("s5b", "Edit", R5+"/r1.txt", "", R5)
	c.check("R1 steal 표식만 남은 묵은 claim: 지금은 막고 60초 뒤 풀린다고 알림", 2, "60초가 지나면")
	c.checkOK("R1 묵은 claim을 만난 판정이 그 claim을 다시 무장(mtime = 지금)", mtime(c, cl) > m, 0)
	hook61("s5b", "Edit", R5+"/r1.txt", "", R5)
	c.check("R1 60초 뒤에는 누군가 넘겨받을 수 있음(문구의 '60초 뒤 풀림'이 참)", 0)
	c.checkOK("R1 넘겨받은 쪽이 주인", field(d+"/owner", 2) == strconv.Itoa(B5), 0)
	for k := 1; k <= 50; k++ {
		c.rec(L5D, fmt.Sprintf("ghost/r1-%d.txt", k), 99997, deadST, false)
	}
	gl := func() int {
		n := 0
		for k := 1; k <= 50; k++ {
			if isDir(L5D + "/" + md5hex(fmt.Sprintf("ghost/r1-%d.txt", k))) {
				n++
			}
		}
		return n
	}
	c.write(L5D+"/.cleaning", "")
	c.touchOld(L5D + "/.cleaning")
	m = mtime(c, L5D+"/.cleaning")
	c.write(fmt.Sprintf("%s/.cleaning.steal-%d", L5D, m), "")
	c.run("s5b", "Bash", "git add b.txt", "", R5)
	c.check("R1 steal 표식만 남은 묵은 정리 잠금: 판정은 그대로", 0)
	hook61("s5b", "Bash", "git add b.txt", "", R5)
	c.RC, c.OUT, c.ERR = gl(), "", ""
	c.check("R1 정리 잠금도 60초 뒤 누군가 다시 잡아 정리(죽은 임대 50개 남은 수)", 0)
	c.checkOK("R1 그 정리도 잠금을 남기지 않음", exists(L5D+"/.cleaning"), 1)
	for k := 1; k <= 5; k++ {
		c.rec(L5D, fmt.Sprintf("ghost/r1-%d.txt", k), 99997, deadST, false)
	}
	c.hookEnv = []string{"SECONDS=99"}
	c.run("s5b", "Bash", "git add b.txt", "", R5)
	c.hookEnv = nil
	c.RC, c.OUT, c.ERR = gl(), "", ""
	c.check("R2 환경의 SECONDS가 시간 상한을 앞당기지 않음(남은 죽은 임대 수)", 0)
	idx5, err := os.ReadFile(R5 + "/.git/index")
	c.fatal(err)
	c.write(R5+"/.git/index", "garbage")
	c.write(L5D+"/.cleaning", "") // someone else cleans: the changed-paths way
	c.run("s5b", "Bash", "git add e.txt", "", R5)
	c.check("작업 중 발견: git status가 실패하면 남의 미커밋 파일 git add를 허용하지 않고 막음(바뀐 경로만 읽는 판정)", 2, "git status가 실패")
	os.Remove(L5D + "/.cleaning") //nolint:errcheck // rm -f
	c.run("s5b", "Bash", "git add e.txt", "", R5)
	c.check("작업 중 발견: 같은 것을 정리자가 판정할 때도 막음", 2, "git status가 실패")
	c.checkOK("작업 중 발견: 그때 정리 잠금을 남기지 않음", exists(L5D+"/.cleaning"), 1)
	c.write(R5+"/.git/index", string(idx5))

	R7 := T + "/r7"
	c.mkrepo(R7)
	L7D := c.leaseDirs(R7)
	c.fatal(os.MkdirAll(L7D, 0o755))
	A7 := c.agent()
	c.reg(A7, "s7a", R7, "agent-a7")
	B7 := c.agent()
	c.reg(B7, "s7b", R7, "agent-b7")
	c.run("s7a", "Edit", R7+"/h.txt", "", R7) // its lease folder (md5 f54c…) sorts after the 0g* ones below
	c.appendf(R7+"/h.txt", "y\n")
	for k := 1; k <= 20000; k++ {
		c.write(fmt.Sprintf("%s/0g%d/owner", L7D, k), fmt.Sprintf("sz\x1f99998\x1f%s\x1f\x1fgone\x1f2026-01-01 00:00:00\x1fghost/g%d.txt\n", deadST, k))
	}
	c.calm()
	t0 = time.Now()
	c.run("s7b", "Bash", "git add b.txt", "", R7)
	ms = time.Since(t0).Milliseconds()
	c.checkOK(fmt.Sprintf("R2 죽은 임대 2만 개(옛 판 맥 4.5초)도 정리 판정이 상한 안에 끝남(%dms, 훅 제한 5초)", ms), c.RC == 0 && c.timeok(ms, 3500), 0)
	// 부하가 크면 준비에 2초를 다 써 한 판정이 하나도 못 읽을 수 있다(설계대로) — 판정을 5번까지 거듭해 정리가 나아가는지 본다
	g7 := func() int { return c.lsCount(L7D, "0g") }
	n1, i := g7(), 1
	for n1 >= 20000 && i < 5 {
		c.calm()
		c.run("s7b", "Bash", "git add b.txt", "", R7)
		n1, i = g7(), i+1
	}
	c.checkOK(fmt.Sprintf("R2 상한까지 읽은 것은 지움(남은 %d, 판정 %d번)", n1, i), n1 < 20000, 0)
	c.checkOK("R2 상한에 걸려도 정리 잠금을 풀고 끝냄", exists(L7D+"/.cleaning"), 1)
	c.calm()
	t0 = time.Now()
	c.run("s7b", "Bash", "git add h.txt", "", R7)
	ms = time.Since(t0).Milliseconds()
	if !c.timeok(ms, 3500) {
		c.RC = -1 // 셸 판의 RC="$RC(${ms}ms)": 시간 초과면 어떤 기대값과도 안 맞는다
	}
	c.check(fmt.Sprintf("R2 상한에 걸린 정리 판정도 못 읽은 임대의 남의 미커밋 파일 git add 차단(%dms)", ms), 2, "agent-a7")
	n2, i := g7(), 1
	for n2 >= n1 && i < 5 {
		c.calm()
		c.run("s7b", "Bash", "git add b.txt", "", R7)
		n2, i = g7(), i+1
	}
	c.checkOK(fmt.Sprintf("R2 다음 판정이 이어서 정리(남은 %d → %d, 판정 %d번)", n1, n2, i), n2 < n1, 0)
	os.Remove(L7D + "/.idle") //nolint:errcheck // written by a scanner that has read some leases
	if goImpl {               // a verdict that reaches the scan already past the limit
		c.hookEnv = []string{"HERDR_HOOKS_TEST_SECONDS=5"}
		c.run("s7b", "Bash", "git add h.txt", "", R7)
		c.hookEnv = nil
	} else {
		c.write(T+"/hook-late.sh", regexp.MustCompile(`(?m)^SECONDS=0`).ReplaceAllLiteralString(string(src), "SECONDS=5"))
		h := c.hook
		c.hook = T + "/hook-late.sh"
		c.run("s7b", "Bash", "git add h.txt", "", R7)
		c.hook = h
	}
	c.check("R2 늦게 시작해 하나도 못 읽은 정리 판정도 남의 미커밋 파일 git add 차단", 2, "agent-a7")
	c.checkOK("R2 시간 상한이 지난 정리 판정은 임대를 읽지 않음", exists(L7D+"/.idle"), 1)
	c.checkOK("R2 그때도 정리 잠금을 남기지 않음", exists(L7D+"/.cleaning"), 1)
	R8 := T + "/r8"
	c.mkrepo(R8)
	L8D := c.leaseDirs(R8)
	c.fatal(os.MkdirAll(L8D, 0o755))
	p8 := c.agent()
	c.reg(p8, "s8a", R8, "agent-8a")
	long := "ghost/" + strings.Repeat("x", 120)
	for k := 1; k <= 8000; k++ {
		c.write(fmt.Sprintf("%s/0g%d/owner", L8D, k), fmt.Sprintf("sz\x1f99998\x1f%s\x1f\x1fgone\x1f2026-01-01 00:00:00\x1f%s/g%d.txt\n", deadST, long, k))
	}
	c.calm()
	c.run("s8a", "Bash", "git add b.txt", "", R8)
	n := c.lsCount(L8D, "0g")
	c.checkOK(fmt.Sprintf("R2 경로가 긴 죽은 임대 8000개(한 status 인자 1.2MB)도 정리가 진행(남은 %d)", n), c.RC == 0 && n < 8000, 0)

	// 작업 중 발견(2026-11-21 hooks-go 두 판 대조): 글롭이 펼친 결과를 다시 펼쳐, 닫히지 않은 [ 가 든 대상(펼쳐도 그대로)이나
	// 저를 다시 맞추는 이름(q*·q*b)이면 bash_write가 끝없이 되돌아 셸 판이 죽었다(신호로 끝남 → onFailure로 막힘). 펼친 결과는 구체 경로다
	c.write(R+"/q*", "")
	c.write(R+"/q*b", "")
	for _, cmd := range []string{"echo x >> build.log[", "rm -f q*", "printf y > 'q*'"} {
		c.run("sb", "Bash", cmd)
		c.check("작업 중 발견: 글롭 결과를 다시 펼치지 않음(끝없이 안 돎): "+cmd, 0)
	}
	c.checkOK("작업 중 발견: 닫히지 않은 [ 대상도 보통 파일로 임대", exists(c.leasefile(R, "build.log[")), 0)
	c.checkOK("글롭으로 맞은 파일도 이미 미커밋이면 Bash 쓰기가 임대하지 않음(남의 변경을 떠안지 않게)", exists(c.leasefile(R, "q*b")), 1)
	os.Remove(R + "/q*")  //nolint:errcheck // 셸 판 rm -f
	os.Remove(R + "/q*b") //nolint:errcheck
}
