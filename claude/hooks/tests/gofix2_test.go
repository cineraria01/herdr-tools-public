package tests

// gofix2-hooks(2026-11-22 Go 전환 재감사 아스트라 1바퀴 지적): 고치기 전 코드에서 실패하는 시험. 지적 ID는 재감사 보고서의 것(R-H·R-M·R-L).
// 기대값은 지적 원문과 계약으로 정했다. 시험마다 저장소·에이전트를 따로 만든다(-run으로 하나만 골라도 그것만 돈다).

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCotenantGofix2(t *testing.T) {
	// R-H02: hc ls가 종료 0이라도 목록이 완전하지 않으면(빈 출력·terminals 없음·null·주소 없는 live 항목) 모름 —
	// 주인 없는 미커밋 파일을 넘기지 않고 막는다(종료 7과 같게)
	for _, x := range []struct{ name, out string }{
		{"빈 성공", ""},
		{"terminals 없음", `{}`},
		{"terminals null", `{"terminals":null}`},
		{"주소 없는 live", `{"terminals":[{"state":"live","agent":"claude-code","cwd":"/x","name":"w"}]}`},
		{"항목이 객체 아님", `{"terminals":["t1"]}`},
		{"JSON 둘", `{"terminals":[]}{"terminals":[]}`},
	} {
		t.Run("R-H02 hc ls "+x.name, func(t *testing.T) {
			c := newCG(t)
			R, _, _ := repoAB(c)
			c.appendf(R+"/a.txt", "y\n")
			c.write(c.T+"/bin/hc", "#!/bin/sh\nprintf '%s' '"+x.out+"'\n")
			c.fatal(os.Chmod(c.T+"/bin/hc", 0o755))
			c.run("sb", "Edit", R+"/a.txt")
			if c.RC != 2 || exists(c.leasefile(R, "a.txt")) {
				t.Errorf("rc=%d(want 2), a.txt 임대 %v(want 없음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")), c.ERR)
			}
		})
	}
	// R-H02 대조: 완전한 빈 목록은 「hc 에이전트 없음」 — 막지 않는다(미커밋 파일도 주인이 없으면 넘겨받는다)
	t.Run("R-H02 hc ls 빈 목록은 없음", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.appendf(R+"/a.txt", "y\n")
		c.run("sb", "Edit", R+"/a.txt")
		if c.RC != 0 || !exists(c.leasefile(R, "a.txt")) {
			t.Errorf("rc=%d(want 0), a.txt 임대 %v(want 있음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")), c.ERR)
		}
	})
	// R-H04: 저장소 표식(.git)이 있는데 git이 열지 못하면(읽기 막힘·끊긴 gitfile) 「작업 트리 밖」이 아니라 모름 — 막는다
	t.Run("R-H04 .git 읽을 수 없음", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.fatal(os.Chmod(R+"/.git", 0o000))
		t.Cleanup(func() { os.Chmod(R+"/.git", 0o755) }) //nolint:errcheck // 임시 폴더 지우기 전
		c.run("sa", "Edit", R+"/a.txt")
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
	t.Run("R-H04 끊긴 gitfile", func(t *testing.T) {
		c := newCG(t)
		R := c.T + "/wt"
		c.R = R
		c.fatal(os.MkdirAll(R+"/sub", 0o755))
		c.write(R+"/.git", "gitdir: "+c.T+"/gone/.git/worktrees/wt\n")
		c.run("sa", "Edit", R+"/sub/a.txt")
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
	// R-H04 대조: 표식 없는 평범한 폴더는 작업 트리 밖 — 막지 않는다
	t.Run("R-H04 저장소 아님은 통과", func(t *testing.T) {
		c := newCG(t)
		D := c.T + "/plain"
		c.R = D
		c.fatal(os.MkdirAll(D, 0o755))
		c.run("sa", "Edit", D+"/a.txt")
		if c.RC != 0 {
			t.Errorf("rc=%d(want 0) err=[%s]", c.RC, c.ERR)
		}
	})
	// R-H05: status -z가 종료 0이어도 레코드가 온전하지 않으면(이름 바꿈의 원래 이름 레코드 빠짐) 정리하지 않는다 —
	// 산 주인 A의 d.txt 임대(미커밋: 이름 바꿈의 원래 쪽)를 지우면 안 된다
	t.Run("R-H05 status -z 이름 바꿈 짝 빠짐", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/d.txt")
		c.git(R, "mv", "d.txt", "moved.txt")
		c.run("sa", "Edit", R+"/moved.txt") // 두 쪽 모두 임대 — 정리자의 status 경로 지정에 둘이 함께 들어가 이름 바꿈 레코드가 나온다
		lf := c.leasefile(R, "d.txt")
		if !exists(lf) {
			t.Fatalf("준비: A의 d.txt 임대가 없다 rc=%d err=[%s]", c.RC, c.ERR)
		}
		c.touchOld(lf) // 10분 넘은 임대 — 정리 대상 후보
		fakeGit(c, "case \" $* \" in *\" status \"*\" -z \"*) \"$REAL\" \"$@\" | /usr/bin/perl -0777 -pe 's/((?:^|\\0)R. [^\\0]*\\0)[^\\0]*\\0/$1/'; exit 0 ;; esac")
		c.exec1(c.hookIn("sb", "Bash", "git add -A", "", R), c.env(c.stdPath()))
		if !exists(lf) {
			t.Errorf("잘린 status로 산 주인 A의 d.txt 임대를 지웠다 rc=%d err=[%s]", c.RC, c.ERR)
		}
	})
}

// R-H03·R-M06: 임대 열쇠는 git이 내는 철자 — 뿌리·하위 폴더의 대소문자, 디렉터리에서 읽은 NFD로 적어도 같은 파일은 같은 임대다.
// 옛 판이 적은 철자의 임대도 혼용 기간 동안 지킨다
func TestCotenantPathKeys(t *testing.T) {
	t.Run("R-H03 뿌리 대소문자", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", c.T+"/R/a.txt", "", c.T+"/R")
		if c.RC != 0 || !exists(c.leasefile(R, "a.txt")) {
			t.Fatalf("A rc=%d(want 0), a.txt 임대 %v(want 있음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")), c.ERR)
		}
		c.appendf(R+"/a.txt", "y\n")
		c.run("sb", "Edit", R+"/a.txt")
		if c.RC != 2 {
			t.Errorf("B rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
	t.Run("R-H03 하위 폴더 대소문자", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/SUB/C.TXT")
		if c.RC != 0 || !exists(c.leasefile(R, "sub/c.txt")) {
			t.Fatalf("A rc=%d(want 0), sub/c.txt 임대 %v(want 있음) err=[%s]", c.RC, exists(c.leasefile(R, "sub/c.txt")), c.ERR)
		}
		c.appendf(R+"/sub/c.txt", "y\n")
		for _, x := range []struct{ tool, arg string }{{"Edit", R + "/sub/c.txt"}, {"Bash", "git add sub/c.txt"}} {
			c.run("sb", x.tool, x.arg)
			if c.RC != 2 {
				t.Errorf("B %s %s: rc=%d(want 2) err=[%s]", x.tool, x.arg, c.RC, c.ERR)
			}
		}
	})
	t.Run("R-H03 디렉터리에서 읽은 NFD", func(t *testing.T) {
		nfdDir, nfdFile := "가", "한" // 가·한을 자모 분리(NFD)로 디스크에 둔다
		nfc := "가/한.txt"
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.write(R+"/"+nfdDir+"/"+nfdFile+".txt", "x\n")
		c.git(R, "add", "-A")
		c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "nfd")
		es, err := os.ReadDir(R)
		c.fatal(err)
		dir := ""
		for _, e := range es {
			if e.IsDir() && e.Name() != ".git" && e.Name() != "sub" {
				dir = e.Name()
			}
		}
		fs, err := os.ReadDir(R + "/" + dir)
		c.fatal(err)
		if len(fs) != 1 {
			t.Fatalf("준비: %s 안 %d개", dir, len(fs))
		}
		read := R + "/" + dir + "/" + fs[0].Name() // 디렉터리에서 읽은 철자 그대로
		c.run("sa", "Edit", read)
		if c.RC != 0 || !exists(c.leasefile(R, nfc)) {
			t.Fatalf("A rc=%d(want 0), NFC 임대 %v(want 있음 — git 철자) err=[%s]", c.RC, exists(c.leasefile(R, nfc)), c.ERR)
		}
		c.appendf(read, "y\n")
		for _, x := range []struct{ tool, arg string }{{"Edit", R + "/" + nfc}, {"Bash", "git add " + nfc}, {"Edit", read}} {
			c.run("sb", x.tool, x.arg)
			if c.RC != 2 {
				t.Errorf("B %s %q: rc=%d(want 2) err=[%s]", x.tool, x.arg, c.RC, c.ERR)
			}
		}
	})
	// 옛 판이 적은 철자(NFD·다른 대소문자)로 남긴 산 주인의 임대 — 미커밋이면 B의 넘겨받기·정리가 지나치지 않는다
	t.Run("R-H03 옛 철자 임대 보존", func(t *testing.T) {
		c := newCG(t)
		R, A, _ := repoAB(c)
		nfd, nfc := "가.txt", "가.txt" // 가.txt 자모 분리 · 한 글자
		c.write(R+"/"+nfd, "x\n")
		c.git(R, "add", "-A")
		c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "nfd")
		L := c.leaseDirs(R)
		st := procStart(c, A)
		c.rec(L, nfd, A, st, true)         // 옛 판: 적은 NFD 그대로, 10분 넘게 안 건드림
		c.rec(L, "SUB/C.TXT", A, st, true) // 옛 판: 적은 대소문자 그대로
		c.appendf(R+"/"+nfd, "y\n")
		c.appendf(R+"/sub/c.txt", "y\n")
		for _, x := range []struct{ tool, arg string }{{"Edit", R + "/" + nfc}, {"Edit", R + "/sub/c.txt"}} {
			c.run("sb", x.tool, x.arg)
			if c.RC != 2 {
				t.Errorf("B %s %q: rc=%d(want 2 — A의 옛 철자 임대) err=[%s]", x.tool, x.arg, c.RC, c.ERR)
			}
		}
		for _, rel := range []string{nfd, "SUB/C.TXT"} {
			if !exists(L + "/" + md5hex(rel) + "/owner") {
				t.Errorf("정리자가 산 주인 A의 옛 철자 임대 %q를 지웠다", rel)
			}
		}
	})
	// R-M06(E06 보강): 이미 있는 임대 부모에 새 임대 폴더만 못 만들면 시각 오류(1)로 빠지지 않고 막는다(2), 임대 없음
	t.Run("R-M06 임대 폴더만 못 만듦", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/b.txt")
		L := c.leaseDirs(R)
		c.fatal(os.Chmod(L, 0o555))
		t.Cleanup(func() { os.Chmod(L, 0o755) }) //nolint:errcheck // 임시 폴더 지우기 전
		c.run("sa", "Edit", R+"/a.txt")
		if c.RC != 2 || exists(c.leasefile(R, "a.txt")) {
			t.Errorf("rc=%d(want 2), a.txt 임대 %v(want 없음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")), c.ERR)
		}
	})
}

// R-H06: 주인을 적기 전에 멈춘 첫 획득(빈 임대 폴더)을 넘겨받을 때도 첫 획득의 검사를 한다. 막힌 첫 획득은 빈 폴더를 남기지 않는다
func TestCotenantInterruptedClaim(t *testing.T) {
	setup := func(t *testing.T) (*cg, string, string) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/b.txt") // 임대 부모가 있다
		ld := c.leaseDirs(R) + "/" + md5hex("a.txt")
		c.fatal(os.Mkdir(ld, 0o755))
		c.touchOld(ld) // 10초 넘게 주인 없음 — 그 훅은 죽었다
		c.appendf(R+"/a.txt", "y\n")
		return c, R, ld
	}
	t.Run("동거 모름이면 막음", func(t *testing.T) {
		c, R, ld := setup(t)
		c.write(c.T+"/bin/hc", "#!/bin/sh\nexit 7\n")
		c.fatal(os.Chmod(c.T+"/bin/hc", 0o755))
		c.run("sb", "Edit", R+"/a.txt")
		if c.RC != 2 || exists(ld+"/owner") {
			t.Errorf("rc=%d(want 2), 주인 %v(want 없음) err=[%s]", c.RC, exists(ld+"/owner"), c.ERR)
		}
	})
	t.Run("대조: 동거 없으면 넘겨받음", func(t *testing.T) {
		c, R, ld := setup(t)
		c.run("sb", "Edit", R+"/a.txt")
		if c.RC != 0 || field(ld+"/owner", 1) != "sb" {
			t.Errorf("rc=%d(want 0), 주인 %q(want sb) err=[%s]", c.RC, field(ld+"/owner", 1), c.ERR)
		}
	})
	t.Run("막힌 첫 획득은 빈 폴더를 거둔다", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/b.txt")
		L := c.leaseDirs(R)
		c.write(L+"/.cleaning", "") // 다른 정리자가 일하는 중 — 바뀐 경로만 status -z로
		c.appendf(R+"/a.txt", "y\n")
		fakeGit(c, "case \" $* \" in *\" status \"*\" -z \"*) exit 1 ;; esac")
		c.run("sb", "Edit", R+"/a.txt")
		if ld := L + "/" + md5hex("a.txt"); c.RC != 2 || exists(ld) {
			t.Errorf("rc=%d(want 2), 빈 임대 폴더 남음 %v(want 없음) err=[%s]", c.RC, exists(ld), c.ERR)
		}
	})
}

// R-M04: 여러 파일을 쓰는 Bash가 중간에 막히면 이번 호출이 만든 임대를 되돌린다
func TestCotenantMultiTargetUndo(t *testing.T) {
	c := newCG(t)
	R, _, _ := repoAB(c)
	fakeGit(c, "case \" $* \" in *\" status \"*\"b.txt\"*) exit 7 ;; esac")
	c.run("sa", "Bash", "tee a.txt b.txt </dev/null")
	if c.RC != 2 || exists(c.leasefile(R, "a.txt")) || exists(c.leaseDirs(R)+"/"+md5hex("b.txt")) {
		t.Errorf("rc=%d(want 2), a.txt 임대 %v·b.txt 폴더 %v(want 둘 다 없음) err=[%s]", c.RC, exists(c.leasefile(R, "a.txt")),
			exists(c.leaseDirs(R)+"/"+md5hex("b.txt")), c.ERR)
	}
	c.hookEnv = nil
	c.run("sa", "Bash", "tee a.txt b.txt </dev/null")
	if c.RC != 0 || !exists(c.leasefile(R, "a.txt")) || !exists(c.leasefile(R, "b.txt")) {
		t.Errorf("대조 rc=%d(want 0), 두 임대 있어야 err=[%s]", c.RC, c.ERR)
	}
}

// R-M12: C·UTF-8 밖의 로캘에서는 글롭이 고를 파일을 정하지 못한다 — 작업 트리에 닿으면 막고, 트리 밖은 그대로
func TestCotenantOtherLocaleGlob(t *testing.T) {
	c := newCG(t)
	R, _, _ := repoAB(c)
	c.write(R+"/é.txt", "e\n")
	c.git(R, "add", "-A")
	c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "e")
	c.run("sa", "Edit", R+"/é.txt")
	c.appendf(R+"/é.txt", "y\n")
	c.fatal(os.MkdirAll(c.T+"/plain", 0o755))
	c.write(c.T+"/plain/x.txt", "x\n")
	for _, x := range []struct {
		cmd  string
		want int
	}{{"tee ?.txt </dev/null", 2}, {"tee " + c.T + "/plain/?.txt </dev/null", 0}} {
		c.hookEnv = []string{"LC_ALL=ko_KR.eucKR"}
		// 바이너리를 바로 — eucKR에서는 /bin/sh가 settings 명령 줄의 따옴표 안 UTF-8 한국어를 잘못 읽어(따옴표 짝 없음) 판정 전에 2로 끝난다
		cmd := exec.Command(c.H+"/.local/bin/herdr-hooks", "cotenant-guard")
		cmd.Stdin = strings.NewReader(string(c.hookIn("sb", "Bash", x.cmd, "", R)))
		cmd.Env, cmd.Dir = c.env(c.stdPath()), c.T
		var e strings.Builder
		cmd.Stderr = &e
		rc := 0
		if err := cmd.Run(); err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			rc = ee.ExitCode()
		}
		if rc != x.want || (rc == 2 && !strings.Contains(e.String(), "로캘")) {
			t.Errorf("eucKR %s: rc=%d(want %d) err=[%s]", x.cmd, rc, x.want, e.String())
		}
	}
}

// 작업 중 발견: 정리자가 임대 목록을 못 읽으면(읽기 권한 없음) 「임대 없음」이 아니라 모름 — 바뀐 경로의 임대로 판정해 막는다
func TestCotenantLeaseListUnreadable(t *testing.T) {
	c := newCG(t)
	R, _, _ := repoAB(c)
	c.run("sa", "Edit", R+"/a.txt")
	c.appendf(R+"/a.txt", "y\n")
	L := c.leaseDirs(R)
	c.fatal(os.Chmod(L, 0o333))
	t.Cleanup(func() { os.Chmod(L, 0o755) }) //nolint:errcheck // 임시 폴더 지우기 전
	c.run("sb", "Bash", "git add -A")
	if c.RC != 2 {
		t.Errorf("rc=%d(want 2 — A의 미커밋 a.txt) err=[%s]", c.RC, c.ERR)
	}
}

// R-L02: 끝난 정리자가 남긴 .trash-<pid>는 다음 정리자가 거둔다
func TestCotenantStaleTrash(t *testing.T) {
	c := newCG(t)
	R, _, _ := repoAB(c)
	c.run("sa", "Edit", R+"/b.txt")
	dead := exec.Command("true")
	c.fatal(dead.Run())
	tr := c.leaseDirs(R) + "/.trash-" + strconv.Itoa(dead.Process.Pid)
	c.write(tr+"/x/owner", "x\n")
	c.appendf(R+"/a.txt", "y\n")
	c.run("sb", "Edit", R+"/a.txt") // 주인 없는 미커밋 파일 — 동거 판정이 정리자를 돌린다
	for end := time.Now().Add(10 * time.Second); exists(tr) && time.Now().Before(end); {
		time.Sleep(100 * time.Millisecond)
	}
	if exists(tr) {
		t.Errorf("끝난 정리자의 휴지통 %s가 남았다 rc=%d err=[%s]", tr, c.RC, c.ERR)
	}
}

// R-M01(H03 대신): 훅이 띄운 git은 훅이 어떻게 끝나든 함께 끝난다 — 훅 PID에만 TERM, 훅 무리째 KILL.
// 끝난 뒤 git이 표식을 쓰면 안 된다
func TestCotenantHookKilled(t *testing.T) {
	for _, x := range []struct {
		name  string
		group bool
	}{{"훅 PID에만 TERM", false}, {"훅 무리째 KILL", true}} {
		t.Run(x.name, func(t *testing.T) {
			c := newCG(t)
			R, _, _ := repoAB(c)
			fakeGit(c, "case \" $* \" in *\" status \"*) : > \""+c.T+"/started\"; sleep 1.5; : > \""+c.T+"/marker\" ;; esac")
			cmd := hookExec(c.hook, c.hookIn("sa", "Edit", R+"/a.txt", "", R), c.env(c.stdPath()), c.T)
			if x.group {
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			}
			c.fatal(cmd.Start())
			for end := time.Now().Add(10 * time.Second); !exists(c.T+"/started") && time.Now().Before(end); {
				time.Sleep(20 * time.Millisecond)
			}
			if !exists(c.T + "/started") {
				cmd.Process.Kill() //nolint:errcheck
				cmd.Wait()         //nolint:errcheck
				t.Fatal("가짜 git status가 돌지 않았다")
			}
			if x.group {
				c.fatal(syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL))
			} else {
				c.fatal(cmd.Process.Signal(syscall.SIGTERM))
			}
			cmd.Wait() //nolint:errcheck // 신호로 끝남
			time.Sleep(2500 * time.Millisecond)
			if exists(c.T + "/marker") {
				t.Error("훅이 끝난 뒤 git이 표식을 썼다(자식이 훅보다 오래 살았다)")
			}
		})
	}
}

// R-M10: calm은 남은 상한보다 길게 자지 않는다(상한 1초에 부하가 높아도 몇 초 안에 돌아온다)
func TestCalmBudget(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(d+"/sysctl", []byte("#!/bin/sh\ncase \"$2\" in hw.ncpu) echo 1 ;; *) echo '{ 9.00 9.00 9.00 }' ;; esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	t.Setenv("COTENANT_CALM_MAX", strconv.Itoa(calmWaited+1))
	c := &cg{t: t}
	t0 := time.Now()
	c.calm()
	if el := time.Since(t0); el > 4*time.Second || !strings.Contains(c.busy, "부하") {
		t.Errorf("calm %v(want 몇 초 안), busy=%q", el, c.busy)
	}
}
