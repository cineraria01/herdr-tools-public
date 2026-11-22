package tests

// gofix-hooks(2026-11-21 Go 전환 전체 감사 지적): 고치기 전 코드에서 실패하는 시험. 지적 ID는 감사 보고서의 것(아스트라 H·M, Fable H·M).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newCG: cotenant-guard 시험 준비(임시 폴더·가짜 HOME — 그 ~/.local/bin/herdr-hooks에 시험 빌드·에이전트 정리)를 시험마다 따로.
func newCG(t *testing.T) *cg {
	t.Helper()
	T, err := os.MkdirTemp(os.Getenv("TMPDIR"), "cotenant-test.")
	if err != nil {
		t.Fatal(err)
	}
	if T, err = filepath.EvalSymlinks(T); err != nil {
		t.Fatal(err)
	}
	c := &cg{t: t, T: T, H: T + "/home", hook: "cotenant-guard"}
	t.Cleanup(func() {
		for _, k := range c.kids {
			k.Process.Kill() //nolint:errcheck // 이미 끝났을 수 있다
			k.Wait()         //nolint:errcheck
		}
		os.RemoveAll(T) //nolint:errcheck // 임시 폴더
	})
	c.fatal(os.MkdirAll(T+"/bin", 0o755))
	c.fatal(os.MkdirAll(c.H+"/.claude/sessions", 0o755))
	c.write(c.H+"/.claude/settings.json", "{\"env\":{}}\n")
	c.fatal(os.WriteFile(T+"/bin/hc", []byte("#!/bin/sh\necho '{\"terminals\":[]}'\n"), 0o755)) // 이 기계의 진짜 hc를 부르지 않게
	putBin(t, c.H, buildHooks(t))
	return c
}

// fakeGit: 진짜 git 앞에 sh 조각 pre($REAL = 진짜 git)를 끼운 가짜 git — 시험 빌드의 손잡이 HERDR_HOOKS_TEST_GIT로 건다
func fakeGit(c *cg, pre string) {
	real, err := exec.LookPath("git")
	c.fatal(err)
	w := c.T + "/fakegit/git" // 이름이 git이라야 진짜와 같은 상한·무리 판정을 받는다
	c.fatal(os.MkdirAll(c.T+"/fakegit", 0o755))
	c.fatal(os.WriteFile(w, []byte("#!/bin/sh\nREAL="+real+"\n"+pre+"\nexec \"$REAL\" \"$@\"\n"), 0o755))
	c.hookEnv = append(c.hookEnv, "HERDR_HOOKS_TEST_GIT="+w)
}

// slowGit: status에서만 secs초 잠든 뒤 진짜 git으로 넘기는 가짜 git(git 상한 1초)
func slowGit(c *cg, secs string) {
	c.hookEnv = []string{"HERDR_TIMEOUT_GIT=1"}
	fakeGit(c, "case \" $* \" in *\" status \"*) sleep "+secs+" ;; esac")
}

// repoAB: 저장소 R과 산 에이전트 A(sa)·B(sb). 둘 다 등록부 cwd는 저장소 밖 — 임대로만 서로를 안다
func repoAB(c *cg) (R string, A, B int) {
	R = c.T + "/r"
	c.R = R
	c.mkrepo(R)
	A, B = c.agent(), c.agent()
	c.reg(A, "sa", c.T, "agent-a")
	c.reg(B, "sb", c.T, "agent-b")
	return R, A, B
}

func TestCotenantGofix(t *testing.T) {
	// H01(아스트라) · M2(Fable): git status가 상한을 넘기면 「깨끗함」이 아니라 「모름」 — 임대를 새로 쓰지 않고 막는다.
	t.Run("H01 git status 상한 넘김", func(t *testing.T) {
		c := newCG(t)
		R := c.T + "/r"
		c.R = R
		c.mkrepo(R)
		A := c.agent()
		c.reg(A, "sa", R, "agent-a")
		c.appendf(R+"/d.txt", "y\n")
		slowGit(c, "2")
		c.exec1(c.hookIn("sa", "Bash", "echo x | tee d.txt", "", R), c.env(c.stdPath()))
		rcv, errOut := c.RC, c.ERR
		c.hookEnv = nil
		if exists(c.leasefile(R, "d.txt")) {
			t.Errorf("시간 넘김(모름)인데 d.txt 임대를 새로 썼다 rc=%d err=[%s]", rcv, errOut)
		}
		if rcv != 2 {
			t.Errorf("git status 시간 넘김은 막아야(rc 2) — rc=%d err=[%s]", rcv, errOut)
		}
	})
	// M01(아스트라): `bash -O > options.log` — 값 없는 마지막 -O에서 배열 끝을 넘어 패닉(종료 2·임대 0)하던 것. 0·임대여야
	t.Run("M01 bash -O 값 없음", func(t *testing.T) {
		c := newCG(t)
		R := c.T + "/r"
		c.R = R
		c.mkrepo(R)
		A := c.agent()
		c.reg(A, "sa", R, "agent-a")
		c.run("sa", "Bash", "bash -O > options.log", "", R)
		if c.RC != 0 || !exists(c.leasefile(R, "options.log")) {
			t.Errorf("rc=%d(want 0), options.log 임대 %v(want 있음) err=[%s]", c.RC, exists(c.leasefile(R, "options.log")), c.ERR)
		}
	})
	// H03(아스트라)·M2(Fable): 훅이 꺼지면 git도 함께 끝난다 — 재감사 R-M01로 감독 무리가 되어 TestCotenantHookKilled(gofix2_test.go)가 본다
	// E01(아스트라)·E4(Fable): rev-parse 종료 128이라도 「저장소 아님」이 아니면(dubious ownership 등) 모름 → 막음
	t.Run("E01 rev-parse 128 dubious", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		fakeGit(c, "case \" $* \" in *\" rev-parse \"*) echo 'fatal: detected dubious ownership in repository' >&2; exit 128 ;; esac")
		c.exec1(c.hookIn("sa", "Edit", R+"/a.txt", "", R), c.env(c.stdPath()))
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
	t.Run("E01 rev-parse 한 줄", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		fakeGit(c, "case \" $* \" in *\" rev-parse \"*) echo \""+R+"\"; exit 0 ;; esac")
		c.exec1(c.hookIn("sa", "Edit", R+"/a.txt", "", R), c.env(c.stdPath()))
		if c.RC != 2 || exists(R+"/agent-leases") {
			t.Errorf("rc=%d(want 2), 체크아웃에 agent-leases %v err=[%s]", c.RC, exists(R+"/agent-leases"), c.ERR)
		}
	})
	// E02(아스트라): status -z가 마지막 기록의 NUL 전에 128로 끝나면 실패로 — 실패 표식이 경로에 붙어 놓치던 것
	t.Run("E02 status -z 잘림", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.run("sa", "Edit", R+"/d.txt")
		c.appendf(R+"/d.txt", "y\n")
		fakeGit(c, "case \" $* \" in *\" status \"*\" -z \"*) \"$REAL\" \"$@\" | /usr/bin/perl -0777 -pe 's/\\0\\z//'; exit 128 ;; esac")
		c.exec1(c.hookIn("sb", "Bash", "git add -A", "", R), c.env(c.stdPath()))
		if c.RC != 2 || !exists(c.leasefile(R, "d.txt")) {
			t.Errorf("rc=%d(want 2), A 임대 남음 %v err=[%s]", c.RC, exists(c.leasefile(R, "d.txt")), c.ERR)
		}
	})
	// E03(아스트라)·E2(Fable): hc ls 실패(불완전 JSON·종료 7)를 「hc 에이전트 없음」으로 읽지 않는다 — 주인 없는 미커밋 파일 편집을 막는다
	t.Run("E03 hc ls 실패", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.appendf(R+"/a.txt", "y\n")
		c.write(c.T+"/bin/hc", "#!/bin/sh\nprintf '{\"terminals\":['\nexit 7\n")
		c.run("sb", "Edit", R+"/a.txt")
		if c.RC != 2 || !strings.Contains(c.ERR, "hc ls") {
			t.Errorf("rc=%d(want 2, hc ls 실패 이유) err=[%s]", c.RC, c.ERR)
		}
	})
	// E04(아스트라): hc가 끝났는데 손자가 출력 관을 쥐고 있으면 2초 안에 끊고 모름(막음)
	t.Run("E04 hc ls 관 붙잡음", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.appendf(R+"/a.txt", "y\n")
		// 손자가 hc-hold가 있는 동안(최대 9초) 관을 쥔다 — 훅은 기한(2초·예산 4초) 안에 끊는다. 부하에 흔들리지 않게 상한은 넉넉히 7초.
		// 하위 시험이 끝나면 hc-hold를 지워 손자가 스스로 끝난다 — 파일에 적은 숫자 PID에 신호를 보내지 않는다(재감사 hooks R-M208:
		// 끝난 뒤 그 번호가 다른 프로세스에 다시 쓰일 수 있다). 경로는 스크립트 자리($0) 기준이라 셸 글에 잇지 않는다
		c.write(c.T+"/hc-hold", "")
		c.write(c.T+"/bin/hc", "#!/bin/sh\nd=$(dirname \"$0\")/..\n"+
			"( : >\"$d/hc-gc-start\"; i=0; while [ -e \"$d/hc-hold\" ] && [ $i -lt 90 ]; do /bin/sleep 0.1; i=$((i+1)); done ) &\n"+
			"echo '{\"terminals\":[]}'\n")
		t.Cleanup(func() { os.Remove(c.T + "/hc-hold") }) //nolint:errcheck // 손자가 스스로 끝나게
		t0 := time.Now()
		c.run("sb", "Edit", R+"/a.txt")
		if d := time.Since(t0); !exists(c.T+"/hc-gc-start") || c.RC != 2 || d > 7*time.Second {
			t.Errorf("관을 쥔 손자가 떴고(%v) rc=%d(want 2), %v(want 7초 안 — 손자 9초를 기다리지 않음) err=[%s]",
				exists(c.T+"/hc-gc-start"), c.RC, d, c.ERR)
		}
	})
	// E06(아스트라): 임대 폴더를 만들 수 없으면 기록 없이 허용하지 않는다
	t.Run("E06 임대 폴더 못 만듦", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.fatal(os.Chmod(R+"/.git", 0o555))
		t.Cleanup(func() { os.Chmod(R+"/.git", 0o755) }) //nolint:errcheck // 임시 폴더 지우기 전
		c.run("sa", "Edit", R+"/a.txt")
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
	// E10(아스트라): 안내문의 git diff 명령은 파일명을 셸 낱말 하나로 인용한다
	t.Run("E10 작은따옴표 파일명", func(t *testing.T) {
		c := newCG(t)
		R, A, _ := repoAB(c)
		c.write(R+"/O'Brien.txt", "o\n")
		c.git(R, "add", "--", "O'Brien.txt")
		c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "o")
		c.run("sa", "Edit", R+"/O'Brien.txt")
		c.appendf(R+"/O'Brien.txt", "y\n")
		c.killAgent(A)
		c.run("sb", "Edit", R+"/O'Brien.txt")
		// JSON 안이라 \ 는 \\
		if want := `git diff -- 'O'\\''Brien.txt'`; c.RC != 0 || !strings.Contains(c.OUT, want) {
			t.Errorf("rc=%d(want 0), 안내에 %s out=[%s] err=[%s]", c.RC, want, c.OUT, c.ERR)
		}
	})
	// M02(아스트라): LC_ALL=C에서 글롭 ?는 한 바이트(bash와 같게) — 「한.txt」(3바이트)는 ???.txt에 맞고 ?.txt에 안 맞는다. UTF-8이면 글자
	t.Run("M02 C 로캘 글롭", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.write(R+"/한.txt", "h\n")
		c.git(R, "add", "-A")
		c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "h")
		c.run("sa", "Edit", R+"/한.txt")
		c.appendf(R+"/한.txt", "y\n")
		for _, x := range []struct {
			loc, cmd string
			want     int
		}{{"C", "tee ???.txt", 2}, {"C", "tee ?.txt", 0}, {"en_US.UTF-8", "tee ?.txt", 2}, {"en_US.UTF-8", "tee ???.txt", 0}} {
			c.hookEnv = []string{"LC_ALL=" + x.loc}
			c.run("sb", "Bash", x.cmd+" </dev/null")
			if c.RC != x.want {
				t.Errorf("LC_ALL=%s %s: rc=%d(want %d) err=[%s]", x.loc, x.cmd, c.RC, x.want, c.ERR)
			}
		}
	})
	// E07(아스트라): 따옴표로 지킨 공백이 든 낱말은 다시 가르지 않고 통째로 펼친다
	t.Run("E07 따옴표 공백 글롭", func(t *testing.T) {
		c := newCG(t)
		R, _, _ := repoAB(c)
		c.write(R+"/팀 문서/안내.txt", "a\n")
		c.git(R, "add", "-A")
		c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "t")
		c.run("sa", "Edit", R+"/팀 문서/안내.txt")
		c.appendf(R+"/팀 문서/안내.txt", "y\n")
		c.run("sb", "Bash", `tee "팀 문서"/*.txt </dev/null`)
		if c.RC != 2 {
			t.Errorf("rc=%d(want 2) err=[%s]", c.RC, c.ERR)
		}
	})
}

// H1(Fable): 디스크에 자모 분리(NFD)로 저장된 한글 폴더 — 임대 열쇠는 적은 철자(NFC)라 남의 미커밋 파일을 막는다.
// getcwd(디스크 철자)를 쓰던 때는 git add가 통과했다(A는 편집하고 고친 쪽, B는 다른 산 에이전트). 셸 판과 섞이는 갈래는 셸 판과 함께 걷었다
func TestCotenantNFDFolder(t *testing.T) {
	nfd, nfc := "가", "가" // 자모 분리 ᄀ+ᅡ · 한 글자 가
	c := newCG(t)
	R, _, _ := repoAB(c)
	c.write(R+"/"+nfd+"/x.txt", "x\n")
	c.git(R, "add", "-A")
	c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "nfd")
	c.run("sa", "Edit", R+"/"+nfc+"/x.txt")
	if c.RC != 0 {
		t.Fatalf("A 편집 rc=%d err=[%s]", c.RC, c.ERR)
	}
	c.appendf(R+"/"+nfd+"/x.txt", "y\n")
	for _, x := range []struct{ tool, arg string }{{"Edit", R + "/" + nfc + "/x.txt"}, {"Bash", "git add " + nfc + "/x.txt"}, {"Bash", "rm -rf " + nfc}} {
		c.run("sb", x.tool, x.arg)
		if c.RC != 2 {
			t.Errorf("B %s %s: rc=%d(want 2) err=[%s]", x.tool, x.arg, c.RC, c.ERR)
		}
	}
}

// H02(아스트라): hc get이 상한을 넘기면 루프인지 모른다 — 무인 루프일 수 있으니 질문 창을 막는다.
// R-M210: 응답은 즉시 오면 허용되는 정상 비루프 작업자 응답이다(대조로 먼저 확인) — 막힌 까닭이 시간 넘김뿐임을 전용 진단으로 본다
func TestLoopNoQuestionTimeout(t *testing.T) {
	T := t.TempDir()
	for _, d := range []string{"bin", "home"} {
		if err := os.MkdirAll(filepath.Join(T, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake := "#!/bin/bash\nsleep \"$FAKE_SLEEP\"\nprintf '%s' '{\"address\":\"audit-term\",\"state\":\"live\",\"cwd\":\"/w\",\"name\":\"worker\"}'\n"
	if err := os.WriteFile(filepath.Join(T, "bin", "hc"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	in := []byte(`{"tool_name":"AskUserQuestion","tool_input":{}}`)
	env := func(sleep string) []string {
		return envWith(nil, "HOME="+filepath.Join(T, "home"), "PATH="+filepath.Join(T, "bin")+":/usr/bin:/bin",
			"HC_TERMINAL_ID=audit-term", "HERDR_TIMEOUT_REMOTE=1", "FAKE_SLEEP="+sleep)
	}
	t.Run("대조: 즉시 답하면 허용", func(t *testing.T) {
		rc, _, errOut := runHook(t, "loop-no-question", in, env("0"), "")
		checkRC(t, rc, 0, errOut, "")
	})
	t.Run("늦으면 시간 넘김으로 막음", func(t *testing.T) {
		rc, _, errOut := runHook(t, "loop-no-question", in, env("3"), "")
		if rc != 2 || !strings.Contains(errOut, "시간 안에 답하지 않아") {
			t.Errorf("rc=%d(want 2·시간 넘김 진단) err=[%s]", rc, errOut)
		}
	})
}
