package tests

// gofix-hooks(2026-11-21 Go 전환 전체 감사 지적): 고치기 전 코드에서 실패하는 시험. 지적 ID는 감사 보고서의 것(아스트라 H·M, Fable H·M).

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// newCG: TestCotenantGuard와 같은 준비(임시 폴더·가짜 HOME·에이전트 정리)를 부분 시험마다 따로.
func newCG(t *testing.T, hook string, impl []string) *cg {
	t.Helper()
	T, err := os.MkdirTemp(os.Getenv("TMPDIR"), "cotenant-gofix.")
	if err != nil {
		t.Fatal(err)
	}
	if T, err = filepath.EvalSymlinks(T); err != nil {
		t.Fatal(err)
	}
	c := &cg{t: t, T: T, H: T + "/home", hook: hook, impl: impl}
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
	return c
}

// fakeGit: 진짜 git 앞에 sh 조각 pre($REAL = 진짜 git)를 끼운 가짜 git. Go 판은 시험 손잡이, 셸 판은 GIT= 줄을 바꾼 훅 사본.
func fakeGit(c *cg, pre string) (hook string) {
	real, err := exec.LookPath("git")
	c.fatal(err)
	w := c.T + "/fakegit/git" // 이름이 git이라야 진짜와 같은 상한·무리 판정을 받는다
	c.fatal(os.MkdirAll(c.T+"/fakegit", 0o755))
	c.fatal(os.WriteFile(w, []byte("#!/bin/sh\nREAL="+real+"\n"+pre+"\nexec \"$REAL\" \"$@\"\n"), 0o755))
	if isGo(c.impl) {
		c.hookEnv = append(c.hookEnv, "HERDR_HOOKS_TEST_GIT="+w)
		return c.hook
	}
	src, err := os.ReadFile(c.hook)
	c.fatal(err)
	hook = c.T + "/hook-fakegit.sh"
	c.write(hook, regexp.MustCompile(`(?m)^GIT=.*$`).ReplaceAllLiteralString(string(src), "GIT="+w))
	return hook
}

// slowGit: status에서만 secs초 잠든 뒤 진짜 git으로 넘기는 가짜 git(git 상한 1초)
func slowGit(c *cg, secs string) (hook string) {
	c.hookEnv = []string{"HERDR_TIMEOUT_GIT=1"}
	return fakeGit(c, "case \" $* \" in *\" status \"*) sleep "+secs+" ;; esac")
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
	hook := hookPath(t, "cotenant-guard")
	forImpls(t, "cotenant-guard", func(t *testing.T, impl []string) {
		// H01(아스트라) · M2(Fable): git status가 상한을 넘기면 「깨끗함」이 아니라 「모름」 — 임대를 새로 쓰지 않고 막는다.
		// 셸 판은 상한이 없어 status를 끝까지 기다리고(미커밋 → Bash 쓰기는 임대 없이 통과) — 두 판 모두 임대를 쓰지 않는다.
		t.Run("H01 git status 상한 넘김", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R := c.T + "/r"
			c.R = R
			c.mkrepo(R)
			A := c.agent()
			c.reg(A, "sa", R, "agent-a")
			c.appendf(R+"/d.txt", "y\n")
			h := slowGit(c, "2")
			c.exec1("bash", h, c.hookIn("sa", "Bash", "echo x | tee d.txt", "", R), c.env(c.stdPath()))
			rcv, errOut := c.RC, c.ERR
			c.hookEnv = nil
			if exists(c.leasefile(R, "d.txt")) {
				t.Errorf("시간 넘김(모름)인데 d.txt 임대를 새로 썼다 rc=%d err=[%s]", rcv, errOut)
			}
			if isGo(impl) && rcv != 2 {
				t.Errorf("Go 판: git status 시간 넘김은 막아야(rc 2) — rc=%d err=[%s]", rcv, errOut)
			}
		})
		// M01(아스트라): `bash -O > options.log` — 값 없는 마지막 -O에서 Go 판이 배열 끝을 넘어 패닉(종료 2·임대 0). 셸 판은 0·임대.
		t.Run("M01 bash -O 값 없음", func(t *testing.T) {
			c := newCG(t, hook, impl)
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
		// H03(아스트라)·M2(Fable): 훅이 띄운 git은 훅과 같은 프로세스 무리 — 훅이 무리째 꺼지면 함께 끝난다(셸 판과 같음)
		t.Run("H03 git은 훅 무리 안", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			h := fakeGit(c, "case \" $* \" in *\" status \"*) /bin/ps -o pgid= -p $$ > \""+c.T+"/pg\" ;; esac")
			c.exec1("bash", h, c.hookIn("sa", "Bash", "echo x | tee d.txt", "", R), c.env(c.stdPath()))
			b, err := os.ReadFile(c.T + "/pg")
			if err != nil {
				t.Fatalf("가짜 git status가 돌지 않았다 rc=%d err=[%s]", c.RC, c.ERR)
			}
			if got, want := strings.TrimSpace(string(b)), strconv.Itoa(syscall.Getpgrp()); got != want {
				t.Errorf("git 무리 %s, 훅을 부른 쪽 무리 %s — 따로 무리면 훅이 꺼져도 git이 남는다", got, want)
			}
		})
		// E01(아스트라)·E4(Fable): rev-parse 종료 128이라도 「저장소 아님」이 아니면(dubious ownership 등) 모름 → 막음. 셸 판은 128을 모두 밖으로(알려진 차이)
		t.Run("E01 rev-parse 128 dubious", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			h := fakeGit(c, "case \" $* \" in *\" rev-parse \"*) echo 'fatal: detected dubious ownership in repository' >&2; exit 128 ;; esac")
			c.exec1("bash", h, c.hookIn("sa", "Edit", R+"/a.txt", "", R), c.env(c.stdPath()))
			if isGo(impl) && c.RC != 2 {
				t.Errorf("Go 판: rc=%d(want 2) err=[%s]", c.RC, c.ERR)
			}
		})
		t.Run("E01 rev-parse 한 줄", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			h := fakeGit(c, "case \" $* \" in *\" rev-parse \"*) echo \""+R+"\"; exit 0 ;; esac")
			c.exec1("bash", h, c.hookIn("sa", "Edit", R+"/a.txt", "", R), c.env(c.stdPath()))
			if isGo(impl) && (c.RC != 2 || exists(R+"/agent-leases")) {
				t.Errorf("Go 판: rc=%d(want 2), 체크아웃에 agent-leases %v err=[%s]", c.RC, exists(R+"/agent-leases"), c.ERR)
			}
		})
		// E02(아스트라): status -z가 마지막 기록의 NUL 전에 128로 끝나면 실패로 — 실패 표식이 경로에 붙어 놓치던 것
		t.Run("E02 status -z 잘림", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			c.run("sa", "Edit", R+"/d.txt")
			c.appendf(R+"/d.txt", "y\n")
			h := fakeGit(c, "case \" $* \" in *\" status \"*\" -z \"*) \"$REAL\" \"$@\" | /usr/bin/perl -0777 -pe 's/\\0\\z//'; exit 128 ;; esac")
			c.exec1("bash", h, c.hookIn("sb", "Bash", "git add -A", "", R), c.env(c.stdPath()))
			if isGo(impl) && (c.RC != 2 || !exists(c.leasefile(R, "d.txt"))) {
				t.Errorf("Go 판: rc=%d(want 2), A 임대 남음 %v err=[%s]", c.RC, exists(c.leasefile(R, "d.txt")), c.ERR)
			}
		})
		// E03(아스트라)·E2(Fable): hc ls 실패(불완전 JSON·종료 7)를 「hc 에이전트 없음」으로 읽지 않는다 — 주인 없는 미커밋 파일 편집을 막는다
		t.Run("E03 hc ls 실패", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			c.appendf(R+"/a.txt", "y\n")
			c.write(c.T+"/bin/hc", "#!/bin/sh\nprintf '{\"terminals\":['\nexit 7\n")
			c.run("sb", "Edit", R+"/a.txt")
			if isGo(impl) && (c.RC != 2 || !strings.Contains(c.ERR, "hc ls")) {
				t.Errorf("Go 판: rc=%d(want 2, hc ls 실패 이유) err=[%s]", c.RC, c.ERR)
			}
		})
		// E04(아스트라): hc가 끝났는데 손자가 출력 관을 쥐고 있으면 2초 안에 끊고 모름(막음)
		t.Run("E04 hc ls 관 붙잡음", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			c.appendf(R+"/a.txt", "y\n")
			if !isGo(impl) {
				t.Skip("셸 판은 손자가 관을 놓을 때까지 기다린다(알려진 차이)")
			}
			// 손자가 9초 쥔다 — 훅은 기한(2초·예산 4초) 안에 끊는다. 부하에 흔들리지 않게 상한은 넉넉히 7초
			// 손자 pid를 적어 두고 하위 시험이 끝나면 끈다 — 안 끄면 9초 동안 시험 무리에 고아로 남는다(검증의 고아 검사가 잡음,
			// 2026-11-22 verify-parallel이 훅 꾸러미를 따로 돌리며 드러남 — 작업 중 발견해 함께 고침)
			c.write(c.T+"/bin/hc", "#!/bin/sh\n/bin/sleep 9 &\necho $! >\""+c.T+"/hc-sleep.pid\"\necho '{\"terminals\":[]}'\n")
			t.Cleanup(func() {
				if b, err := os.ReadFile(c.T + "/hc-sleep.pid"); err == nil {
					if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 1 {
						syscall.Kill(p, syscall.SIGKILL) //nolint:errcheck // 이미 끝났으면 그만
					}
				}
			})
			t0 := time.Now()
			c.run("sb", "Edit", R+"/a.txt")
			if d := time.Since(t0); c.RC != 2 || d > 7*time.Second {
				t.Errorf("rc=%d(want 2), %v(want 7초 안 — 손자 9초를 기다리지 않음) err=[%s]", c.RC, d, c.ERR)
			}
		})
		// E06(아스트라): 임대 폴더를 만들 수 없으면 기록 없이 허용하지 않는다. 셸 판은 통과(알려진 차이)
		t.Run("E06 임대 폴더 못 만듦", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			c.fatal(os.Chmod(R+"/.git", 0o555))
			t.Cleanup(func() { os.Chmod(R+"/.git", 0o755) }) //nolint:errcheck // 임시 폴더 지우기 전
			c.run("sa", "Edit", R+"/a.txt")
			if isGo(impl) && c.RC != 2 {
				t.Errorf("Go 판: rc=%d(want 2) err=[%s]", c.RC, c.ERR)
			}
		})
		// E10(아스트라): 안내문의 git diff 명령은 파일명을 셸 낱말 하나로 인용한다
		t.Run("E10 작은따옴표 파일명", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, A, _ := repoAB(c)
			c.write(R+"/O'Brien.txt", "o\n")
			c.git(R, "add", "--", "O'Brien.txt")
			c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "o")
			c.run("sa", "Edit", R+"/O'Brien.txt")
			c.appendf(R+"/O'Brien.txt", "y\n")
			c.killAgent(A)
			c.run("sb", "Edit", R+"/O'Brien.txt")
			// JSON 안이라 \ 는 \\
			if want := `git diff -- 'O'\\''Brien.txt'`; isGo(impl) && (c.RC != 0 || !strings.Contains(c.OUT, want)) {
				t.Errorf("Go 판: rc=%d(want 0), 안내에 %s out=[%s] err=[%s]", c.RC, want, c.OUT, c.ERR)
			}
		})
		// M02(아스트라): LC_ALL=C에서 글롭 ?는 한 바이트(bash와 같게) — 「한.txt」(3바이트)는 ???.txt에 맞고 ?.txt에 안 맞는다. UTF-8이면 글자
		t.Run("M02 C 로캘 글롭", func(t *testing.T) {
			c := newCG(t, hook, impl)
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
		// E07(아스트라): 따옴표로 지킨 공백이 든 낱말은 다시 가르지 않고 통째로 펼친다. 셸 판은 m=($a)로 가른다(알려진 차이)
		t.Run("E07 따옴표 공백 글롭", func(t *testing.T) {
			c := newCG(t, hook, impl)
			R, _, _ := repoAB(c)
			c.write(R+"/팀 문서/안내.txt", "a\n")
			c.git(R, "add", "-A")
			c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "t")
			c.run("sa", "Edit", R+"/팀 문서/안내.txt")
			c.appendf(R+"/팀 문서/안내.txt", "y\n")
			c.run("sb", "Bash", `tee "팀 문서"/*.txt </dev/null`)
			if isGo(impl) && c.RC != 2 {
				t.Errorf("Go 판: rc=%d(want 2) err=[%s]", c.RC, c.ERR)
			}
		})
	})
}

// H1(Fable): 디스크에 자모 분리(NFD)로 저장된 한글 폴더 — 두 판이 같은 임대 열쇠(적은 철자)를 써서, 어느 판끼리 섞여도 남의 미커밋 파일을 막는다.
// Go 판이 getcwd(디스크 철자)를 쓰던 때는 Go→Go의 git add와 섞인 판이 통과했다(A는 편집하고 고친 쪽, B는 다른 산 에이전트)
func TestCotenantNFDFolder(t *testing.T) {
	if len(impls()) != 2 {
		t.Skip("두 판 대조 — 판을 밖에서 하나로 정하지 않았을 때만")
	}
	hook := hookPath(t, "cotenant-guard")
	goEnv := []string{"HERDR_HOOKS_IMPL=go", "HERDR_HOOKS_BIN=" + buildHooks(t)}
	bashEnv := []string{"HERDR_HOOKS_IMPL=bash"}
	nfd, nfc := "가", "가"
	for _, p := range []struct {
		name string
		a, b []string
	}{{"고→고", goEnv, goEnv}, {"셸→고", bashEnv, goEnv}, {"고→셸", goEnv, bashEnv}, {"셸→셸", bashEnv, bashEnv}} {
		t.Run(p.name, func(t *testing.T) {
			c := newCG(t, hook, p.a)
			R, _, _ := repoAB(c)
			c.write(R+"/"+nfd+"/x.txt", "x\n")
			c.git(R, "add", "-A")
			c.git(R, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "nfd")
			c.run("sa", "Edit", R+"/"+nfc+"/x.txt")
			if c.RC != 0 {
				t.Fatalf("A 편집 rc=%d err=[%s]", c.RC, c.ERR)
			}
			c.appendf(R+"/"+nfd+"/x.txt", "y\n")
			c.impl = p.b
			for _, x := range []struct{ tool, arg string }{{"Edit", R + "/" + nfc + "/x.txt"}, {"Bash", "git add " + nfc + "/x.txt"}, {"Bash", "rm -rf " + nfc}} {
				c.run("sb", x.tool, x.arg)
				if c.RC != 2 {
					t.Errorf("B %s %s: rc=%d(want 2) err=[%s]", x.tool, x.arg, c.RC, c.ERR)
				}
			}
		})
	}
}

// H02(아스트라): hc get이 상한을 넘기면 루프인지 모른다 — 무인 루프일 수 있으니 질문 창을 막는다(셸 판은 상한 없이 끝까지 읽어 막음).
func TestLoopNoQuestionTimeout(t *testing.T) {
	hook := hookPath(t, "loop-no-question")
	T := t.TempDir()
	for _, d := range []string{"bin", "home"} {
		if err := os.MkdirAll(filepath.Join(T, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fake := "#!/bin/bash\nsleep 2\nprintf '%s' '{\"name\":\"audit-loop\"}'\n"
	if err := os.WriteFile(filepath.Join(T, "bin", "hc"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	forImpls(t, "loop-no-question", func(t *testing.T, impl []string) {
		env := envWith(nil, append([]string{"HOME=" + filepath.Join(T, "home"), "PATH=" + filepath.Join(T, "bin") + ":/usr/bin:/bin",
			"HC_TERMINAL_ID=audit-term", "HERDR_TIMEOUT_REMOTE=1"}, impl...)...)
		rc, _, errOut := runHook(t, hook, []byte(`{"tool_name":"AskUserQuestion","tool_input":{}}`), env, "")
		checkRC(t, rc, 2, errOut, "질문 창을 띄우지 않는다")
	})
}

// M03·M04(아스트라): 설치 배치(~/.claude/hooks/<훅>.sh + ~/.local/lib/go-run.sh)에서 실행기를 고르는 차례.
func TestHookRunnerLayout(t *testing.T) {
	if _, ok := os.LookupEnv("HERDR_HOOKS_IMPL"); ok && os.Getenv("HERDR_HOOKS_IMPL") != "go" {
		t.Skip("Go 판 실행기 배치 시험 — HERDR_HOOKS_IMPL=go 또는 두 판일 때만")
	}
	src, err := os.ReadFile(hookPath(t, "loop-no-question"))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := os.ReadFile(filepath.Join("..", "..", "..", "lib", "go-run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	put := func(p string, b []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	in := []byte(`{"tool_name":"AskUserQuestion","tool_input":{}}`)
	// M03: ~/lib/go-run.sh(남의 같은 이름 파일)가 있어도 설치 실행기 ~/.local/lib/go-run.sh를 쓴다.
	t.Run("M03 ~/lib 동명 파일", func(t *testing.T) {
		H := t.TempDir()
		put(H+"/.claude/hooks/loop-no-question.sh", src)
		put(H+"/.local/lib/go-run.sh", runner)
		put(H+"/lib/go-run.sh", []byte("# 다른 도구의 같은 이름 파일\n"))
		env := envWith([]string{"HC_TERMINAL_ID"}, "HOME="+H, "HERDR_HOOKS_IMPL=go", "HERDR_HOOKS_BIN="+buildHooks(t))
		rc, _, errOut := runHook(t, H+"/.claude/hooks/loop-no-question.sh", in, env, "")
		checkRC(t, rc, 0, errOut, "")
	})
	// M04: 실행기가 없는데 Go 판을 명시했으면 셸 판으로 조용히 떨어지지 않고 종료 1·진단.
	t.Run("M04 실행기 없음·IMPL=go", func(t *testing.T) {
		H := t.TempDir()
		put(H+"/.claude/hooks/loop-no-question.sh", src)
		env := envWith([]string{"HC_TERMINAL_ID"}, "HOME="+H, "HERDR_HOOKS_IMPL=go", "HERDR_HOOKS_BIN="+H+"/없는-바이너리")
		rc, _, errOut := runHook(t, H+"/.claude/hooks/loop-no-question.sh", in, env, "")
		if rc != 1 || !strings.Contains(errOut, "go-run.sh") {
			t.Errorf("rc=%d(want 1), stderr에 go-run.sh 진단 — err=[%s]", rc, errOut)
		}
	})
}
