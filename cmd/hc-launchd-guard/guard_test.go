package main

// 계약 시험 — tests/hc-launchd-guard.test.sh (나)·(다)를 옮겼다(2026-11-22 go-shell-remove). 셸 판으로 떨어지던 단언은 새 계약(75)으로.
// (가) lib/go-run.sh의 launchd 문맥 판별은 셸 판이 없어지며 함께 없어졌다. 가짜 hc·도구는 셸 스크립트다(시험용)

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// fakeHC: 부른 인자를 T_LOG에, cwd를 T_LOG.cwd에 적는다. 멈춘 hc 흉내(…_SLEEP)·출력 관을 쥔 자손(…_LS_HOLD)은 그 pid를 T_LOG.pid에
const fakeHC = `#!/bin/bash
echo "$*" >>"$T_LOG"
pwd -P >>"$T_LOG.cwd"
hang() { sleep "$1" & echo $! >"$T_LOG.pid"; wait; }
case $1 in
  ls) [ -z "${HC_FAKE_LS_SLEEP-}" ] || hang "$HC_FAKE_LS_SLEEP"
      [ "${HC_FAKE_LS_RC:-0}" = 0 ] || exit "$HC_FAKE_LS_RC"
      n=$(cat "$T_STATE" 2>/dev/null); [ "$n" = FAIL ] && exit 1; [ -n "$n" ] && HC_FAKE_LS=$n
      [ -n "${HC_FAKE_LS-}" ] || HC_FAKE_LS='{"terminals":[]}'
      printf '%s\n' "$HC_FAKE_LS"
      if [ -n "${HC_FAKE_LS_HOLD-}" ]; then sleep "$HC_FAKE_LS_HOLD" & echo $! >"$T_LOG.pid"; fi
      exit 0 ;;
  run) [ -z "${HC_FAKE_RUN_SLEEP-}" ] || hang "$HC_FAKE_RUN_SLEEP"
       [ -z "${HC_FAKE_RUN_TAKES-}" ] || echo "$HC_FAKE_RUN_TAKES" >"$T_STATE"
       if [ -n "${HC_FAKE_RUN_EXEC-}" ]; then while [ "$1" != -- ]; do shift; done; shift; "$@"; exit; fi
       exit "${HC_FAKE_RUN_RC:-0}" ;;
esac
`

type env struct {
	t     *testing.T
	T     string // 임시 뿌리
	home  string
	stub  string
	guard string
}

func setup(t *testing.T) *env {
	t.Helper()
	T := clitest.TempDir(t)
	e := &env{t: t, T: T, home: filepath.Join(T, "home"), stub: filepath.Join(T, "stub"), guard: clitest.Bin(t, "hc-launchd-guard")}
	os.MkdirAll(e.home, 0o755)
	clitest.Put(t, filepath.Join(e.stub, "hc"), fakeHC)
	clitest.Put(t, filepath.Join(e.stub, "faketool"), "#!/bin/sh\necho \"tool $*\"\nexit 7\n")
	// 가짜 python3: 파이썬 파일(첫 줄 #!…python)이 아니면 진짜처럼 SyntaxError로 1, 맞으면 sh로 돌린다
	clitest.Put(t, filepath.Join(e.stub, "python3"), "#!/bin/sh\nIFS= read -r l <\"$1\"\ncase $l in \"#!\"*python*) ;; *) echo \"SyntaxError: invalid syntax ($1)\" >&2; exit 1 ;; esac\nexec /bin/sh \"$@\"\n")
	clitest.Put(t, filepath.Join(e.stub, "wrap"), "#!/bin/bash\necho \"wrap $*\"\n")
	clitest.Put(t, filepath.Join(e.stub, "pytool"), "#!/usr/bin/env python3\necho \"pytool $*\"\n")
	for _, n := range []string{"hc", "faketool", "python3", "wrap", "pytool"} {
		clitest.Chmod(t, filepath.Join(e.stub, n), 0o755)
	}
	return e
}

func (e *env) log() string    { return clitest.Read(filepath.Join(e.T, "log")) }
func (e *env) runs() int      { return strings.Count("\n"+e.log(), "\nrun ") }
func (e *env) cwds() []string { return clitest.Lines(clitest.Read(filepath.Join(e.T, "log.cwd"))) }

// g: 지킴이 한 번(셸 g_) — 바깥 상한 15초(지킴이가 스스로 안 끝나면 124로 드러난다). 「표준 출력\nrc N」
func (e *env) g(ga []string, kv ...string) string {
	return e.gArgs(append([]string{"job1", "--"}, ga...), kv...)
}

func (e *env) gArgs(args []string, kv ...string) string {
	for _, p := range []string{"log", "log.cwd", "state", "log.pid"} {
		os.Remove(filepath.Join(e.T, p))
	}
	os.WriteFile(filepath.Join(e.T, "log"), nil, 0o644)
	envs := append([]string{"XDG_STATE_HOME", "T_LOG=" + filepath.Join(e.T, "log"), "T_STATE=" + filepath.Join(e.T, "state"),
		"HOME=" + e.home, "PATH=" + e.stub + ":" + os.Getenv("PATH"), "HC_LAUNCHD_GUARD_LS_SECS", "HC_LAUNCHD_GUARD_RUN_SECS", "HC_LAUNCHD_GUARD_LOG_MAX"}, kv...)
	r := clitest.Cmd{Env: envs, Max: 15 * time.Second}.Run(e.guard, args...)
	e.t.Logf("지킴이 %v %v → %d\n%s", kv, args, r.RC, r.Err)
	if r.Out == "" {
		return "rc " + strconv.Itoa(r.RC)
	}
	return r.Out + "\nrc " + strconv.Itoa(r.RC)
}

// gHang: 멈춘 hc 흉내를 지킴이 상한(1초)이 끊는 한 번과 그 걸린 시간 — 흉내가 pid를 적은 뒤 끊겨야 「거둠」을 볼 수 있다. proc의 띄우기
// 준비(ps·고아 기록)도 그 1초 안에서 세니 부하(조각 여럿 -race)에서는 흉내가 pid를 적기 전에 거둬질 수 있다 — 그때는 준비가 안 된
// 것이라 다시(5번까지, 2026-11-23 shard-load-flaky)
func (e *env) gHang(ga []string, kv ...string) (string, time.Duration) {
	r, d := "", time.Duration(0)
	for i := 0; i < 5; i++ {
		s := time.Now()
		r, d = e.g(ga, kv...), time.Since(s)
		if clitest.Read(filepath.Join(e.T, "log.pid")) != "" {
			break
		}
	}
	return r, d
}

// gone: 멈춘 hc 흉내가 남긴 pid가 끝났는가(좀비가 거둬질 때까지 잠깐 기다림)
func (e *env) gone() string {
	p, _ := strconv.Atoi(strings.TrimSpace(clitest.Read(filepath.Join(e.T, "log.pid"))))
	if p <= 1 {
		return "pid 없음"
	}
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if syscall.Kill(p, 0) == syscall.ESRCH {
			return "거둠"
		}
	}
	return fmt.Sprintf("남음(%d)", p)
}

func live(addr, name string) string {
	return fmt.Sprintf(`{"terminals":[{"address":"%s","name":"%s","state":"live"}]}`, addr, name)
}

func runLine(home string, cmd ...string) string {
	return "run --bg --no-parent --outlive-parent --rm --name job1 --cwd " + home + " -- " + strings.Join(cmd, " ")
}

func (e *env) envCmd(tool string, args ...string) []string {
	return append([]string{"/usr/bin/env", "-u", "HC_TERMINAL_ID", "-u", "HC_SCOPE", "-u", "HC_TERMINAL_SOCKET", tool}, args...)
}

func TestGuard(t *testing.T) {
	e := setup(t)
	c := &clitest.C{T: t, Detail: func() string { return "hc 기록:\n" + e.log() }}
	ga := []string{"faketool", "a", "b c"}
	U := filepath.Join(e.home, ".local/state/herdr-go/guard-unknown-job1") // 옛 셸 판의 「모름」 셈 — 이제 만들지 않는다

	c.Ok("같은 이름 터미널이 살아 있으면 아무것도 안 함", e.g(ga, "HC_FAKE_LS="+live("t5", "job1")) == "rc 0")
	c.Ok("그때 hc run 없음", e.runs() == 0)
	c.Ok("다른 기계(주소에 /)의 같은 이름은 안 침 — 띄움", e.g(ga, "HC_FAKE_LS="+live("box/t5", "job1")) == "rc 0")
	c.Ok("다른 이름만 있으면 띄움", e.g(ga, "HC_FAKE_LS="+live("t5", "job2")) == "rc 0" && e.runs() == 1)
	e.g(ga)
	c.Ok("띄우는 명령(코어 문맥, 터미널 표식을 지우고 도구 절대 경로)", clitest.HasLine(e.log(), runLine(e.home, e.envCmd(filepath.Join(e.stub, "faketool"), "a", "b", "c")...)))
	c.Ok("hc run이 실패하고 터미널도 없으면 75(그 자리에서 도구를 돌리지 않음)", e.g(ga, "HC_FAKE_RUN_RC=1") == "rc 75")
	c.Ok("hc run 실패: 도구별 스위치(FAKETOOL_IMPL)가 있어도 그 자리에서 안 돌림", e.g(ga, "HC_FAKE_RUN_RC=1", "FAKETOOL_IMPL=bash", "HERDR_IMPL=bash") == "rc 75")
	c.Ok("모름: 터미널이 살아 있는데 hc ls만 실패 — 띄우지 않고 75", e.g(ga, "HC_FAKE_LS_RC=3", "HC_FAKE_LS="+live("t5", "job1")) == "rc 75")
	c.Ok("그때 hc run 없음", e.runs() == 0)
	c.Ok("모름: 목록 꼴이 아니면(최상위 배열) 75", e.g(ga, "HC_FAKE_LS=[]") == "rc 75")
	e.g(ga, "HC_FAKE_LS_RC=3")
	c.Ok("모름이 세 번 이어져도 75(셸 판 없음)", e.g(ga, "HC_FAKE_LS_RC=3") == "rc 75" && e.runs() == 0)
	c.Ok("모름이 이어져도 셈 파일을 만들지 않음", !clitest.Lexists(U))
	c.Ok("모름: 같은 이름 항목에 state가 없으면 띄우지 않고 75", e.g(ga, `HC_FAKE_LS={"terminals":[{"address":"t5","name":"job1"}]}`) == "rc 75")
	c.Ok("그때 hc run 없음", e.runs() == 0)
	c.Ok("모름: hc run 실패 뒤 hc ls가 실패해도 75", e.g(ga, "HC_FAKE_RUN_RC=1", "HC_FAKE_RUN_TAKES=FAIL") == "rc 75")
	r := clitest.Cmd{Env: []string{"XDG_STATE_HOME", "T_LOG=" + filepath.Join(e.T, "log"), "HOME=" + e.home, "PATH=" + e.stub + ":" + os.Getenv("PATH")}}.Run(e.guard, append([]string{"job1", "--"}, ga...)...)
	c.Ok("띄움에 성공하면 그 줄을 남김(Herdr 터미널로 띄움)", r.RC == 0 && strings.Contains(r.Err, "hc-launchd-guard job1: Herdr 터미널로 띄움"))
	// 상한(B06·M3): hc ls·hc run이 멈춰도 지킴이는 끝나고, 멈춘 무리를 거둔다
	r1, d1 := e.gHang(ga, "HC_FAKE_LS_SLEEP=12", "HC_LAUNCHD_GUARD_LS_SECS=1")
	c.Ok("hc ls가 멈추면 상한에서 끊고 모름(75)", r1 == "rc 75")
	c.Ok("그 hc 무리를 거둠", e.gone() == "거둠")
	r2, d2 := e.gHang(ga, "HC_FAKE_RUN_SLEEP=12", "HC_LAUNCHD_GUARD_RUN_SECS=1")
	c.Ok("hc run이 멈추면 상한에서 끊고(그사이 안 떴으면) 75", r2 == "rc 75")
	c.Ok("그 hc 무리를 거둠", e.gone() == "거둠")
	c.Ok("두 멈춤 모두 바깥 상한 전에 끝남", d1+d2 < 10*time.Second)
	// python3로 감싼 진입점(B01·B08): 이미 깔린 plist가 python3로 넘겨도 산다. 파이썬 파일이면 해석기를 그대로
	e.g([]string{"python3", filepath.Join(e.stub, "wrap"), "--watch"})
	c.Ok("python3 + bash 포장: 해석기를 빼고 포장을 직접 띄움", clitest.HasLine(e.log(), runLine(e.home, e.envCmd(filepath.Join(e.stub, "wrap"), "--watch")...)))
	// 옛 plist `python3 ~/.local/bin/hc-sweep` — 그 자리가 이제 Go 바이너리(여기서는 지킴이 바이너리 자신을 대역으로)
	e.g([]string{"python3", e.guard, "x"})
	c.Ok("python3 + Go 바이너리: 해석기를 빼고 바이너리를 직접 띄움", clitest.HasLine(e.log(), runLine(e.home, e.envCmd(e.guard, "x")...)))
	e.g([]string{"python3", filepath.Join(e.stub, "pytool"), "z"})
	c.Ok("python3 + 파이썬 파일: 해석기 그대로", clitest.HasLine(e.log(), runLine(e.home, e.envCmd(filepath.Join(e.stub, "python3"), filepath.Join(e.stub, "pytool"), "z")...)))
	c.Ok("python3 + 파이썬 파일: hc run 실패면 75(그 자리에서 안 돌림)", e.g([]string{"python3", filepath.Join(e.stub, "pytool"), "z"}, "HC_FAKE_RUN_RC=1") == "rc 75")
	c.Ok("hc run 실패가 이름 겹침(그사이 다른 지킴이가 띄움)이면 0으로 끝", e.g(ga, "HC_FAKE_RUN_RC=1", "HC_FAKE_RUN_TAKES="+live("t9", "job1")) == "rc 0")
	jl := filepath.Join(e.T, "job.log")
	c.Ok("--log: 지킴이 줄이 그 파일로(hc run 실패 → 75와 까닭 한 줄)",
		e.gArgs([]string{"--log", jl, "job1", "--", "faketool", "z"}, "HC_FAKE_RUN_RC=1") == "rc 75" && strings.Contains(lastLine(clitest.Read(jl)), "job1: hc run 실패(종료 1)이고 터미널도 없음"))
	e.gArgs([]string{"--log", jl, "job1", "--", "faketool", "z"})
	c.Ok("--log: 띄울 때 출력을 그 파일로 잇는 sh 포장", clitest.HasLine(e.log(),
		runLine(e.home, append([]string{"/bin/sh", "-c", `exec "$@" >>"$0" 2>&1`, jl}, e.envCmd(filepath.Join(e.stub, "faketool"), "z")...)...)))
	c.Ok("이름이 hc 꼴(a-z 0-9 - _, 32자)이 아니면 2", e.gArgs([]string{"Job.1", "--", "faketool"}) == "rc 2")
	c.Ok("-- 뒤 도구가 없으면 2", e.gArgs([]string{"job1", "--"}) == "rc 2")
	c.Ok("도구를 못 찾으면 127", e.gArgs([]string{"job1", "--", "없는도구"}) == "rc 127")
	c.Done()
}

func lastLine(s string) string {
	l := clitest.Lines(s)
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// TestUnknownRows: 같은 이름 행의 주소·상태를 모르면 띄우지 않는다(R-12·R-05 — 옛 셸 판은 주소 빠진 live 행을 다른 기계로 치고,
// 빈 상태 문자열을 「살아 있지 않음」으로 읽어 hc run을 불렀다)
func TestUnknownRows(t *testing.T) {
	e := setup(t)
	c := &clitest.C{T: t, Detail: func() string { return "hc 기록:\n" + e.log() }}
	ga := []string{"faketool"}
	for _, k := range []struct{ name, ls string }{
		{"R-12: 주소 빠진 같은 이름 live 행", `{"terminals":[{"name":"job1","state":"live"}]}`},
		{"R-12: 주소가 빈 문자열", `{"terminals":[{"address":"","name":"job1","state":"live"}]}`},
		{"R-12: 주소가 문자열이 아님", `{"terminals":[{"address":5,"name":"job1","state":"live"}]}`},
		{"R-05: 상태가 빈 문자열", `{"terminals":[{"address":"t5","name":"job1","state":""}]}`},
		{"R-05: 상태 빠짐", `{"terminals":[{"address":"t5","name":"job1"}]}`},
		{"R-05: 모르는 상태 값", `{"terminals":[{"address":"t5","name":"job1","state":"starting"}]}`},
		{"목록이 null", `{"terminals":null}`},
		{"목록 칸 없음", `{}`},
		{"항목이 객체가 아님", `{"terminals":["job1"]}`},
	} {
		c.Ok(k.name+" → 75", e.g(ga, "HC_FAKE_LS="+k.ls) == "rc 75")
		c.Ok(k.name+": hc run 없음", e.runs() == 0)
	}
	c.Ok("알려진 상태 finished(이 기계)는 살아 있지 않음 — 띄움", e.g(ga, `HC_FAKE_LS={"terminals":[{"address":"t5","name":"job1","state":"finished"}]}`) == "rc 0" && e.runs() == 1)
	c.Ok("주소 빠진 행이 있어도 이 기계에 live가 있으면 0(띄우지 않음)",
		e.g(ga, `HC_FAKE_LS={"terminals":[{"name":"job1","state":"live"},{"address":"t5","name":"job1","state":"live"}]}`) == "rc 0" && e.runs() == 0)
	c.Done()
}

// TestLsHeldPipe (R-04): 모의 hc가 목록을 쓰고 0으로 끝났는데 자손이 출력 관을 4초 쥔다. LS_SECS=1이면 출력 수집까지 그 마감 —
// 2초 안에 75, hc run 안 부름, 관을 쥔 자손을 거둠. 옛 셸 판은 $(…)가 관을 기다려 4.1초 뒤 RUN·0이었다
func TestLsHeldPipe(t *testing.T) {
	e := setup(t)
	c := &clitest.C{T: t, Detail: func() string { return "hc 기록:\n" + e.log() }}
	s := time.Now()
	c.Ok("자손이 출력 관을 쥐면 마감에 모름(75)", e.g([]string{"faketool"}, "HC_FAKE_LS_HOLD=4", "HC_LAUNCHD_GUARD_LS_SECS=1") == "rc 75")
	c.Ok("2초 안에 끝남", time.Since(s) < 2*time.Second)
	c.Ok("hc run 안 부름", e.runs() == 0)
	c.Ok("관을 쥔 자손을 거둠", e.gone() == "거둠")
	c.Done()
}

// TestTCC: 지킴이는 cwd를 $HOME으로 옮기고 hc 클라이언트만 부르며 ~/Documents를 열지 않는다 — launchd에서 Go로 돌아도
// macOS 개인 정보 보호 창이 안 뜨는 근거. ~/Documents를 읽기·찾기 금지(000)로 두고, 지킴이를 저장소 폴더(실제 ~/Documents 아래일 수 있다)에서 부른다
func TestTCC(t *testing.T) {
	e := setup(t)
	c := &clitest.C{T: t, Detail: func() string { return "hc 기록:\n" + e.log() + "\ncwd:\n" + strings.Join(e.cwds(), "\n") }}
	docs := filepath.Join(e.home, "Documents")
	clitest.Put(t, filepath.Join(docs, "secret"), "x")
	clitest.Chmod(t, docs, 0)
	t.Cleanup(func() { os.Chmod(docs, 0o755) })
	ga := []string{"faketool"}
	c.Ok("Documents 000: 살아 있음 판정 0", e.g(ga, "HC_FAKE_LS="+live("t5", "job1")) == "rc 0" && e.runs() == 0)
	c.Ok("Documents 000: 없음이면 띄움 0", e.g(ga) == "rc 0" && e.runs() == 1)
	c.Ok("hc run에 넘기는 --cwd가 $HOME", regexp.MustCompile(`(?m)^run .* --cwd `+regexp.QuoteMeta(e.home)+` -- `).MatchString(e.log()))
	cw := e.cwds()
	all := len(cw) == 2
	for _, d := range cw {
		all = all && d == e.home
	}
	c.Ok("지킴이가 부른 hc는 모두 cwd $HOME(ls·run 두 번)", all)
	only := true
	for _, l := range clitest.Lines(e.log()) {
		only = only && (strings.HasPrefix(l, "ls --json") || strings.HasPrefix(l, "run "))
	}
	c.Ok("지킴이가 부르는 것은 hc ls·hc run뿐", only)
	c.Done()
}

// TestPlists: launchd/*.plist의 sh -c 줄을 가짜 HOME에서 그대로 돌린다 — ~/.local/bin에 진짜 지킴이, 가짜 hc(run은 -- 뒤 명령을
// 실제로 돌림), 도구 자리에 대역. python3로 넘기지 않고(도구는 Go 바이너리), 도구가 실제로 돌아 0이어야 한다
func TestPlists(t *testing.T) {
	root := clitest.Repo(t)
	e := setup(t)
	c := &clitest.C{T: t}
	ps, _ := filepath.Glob(filepath.Join(root, "launchd", "*.plist"))
	if len(ps) == 0 {
		t.Fatal("launchd/*.plist 없음")
	}
	PH := filepath.Join(e.T, "plhome")
	bin := filepath.Join(PH, ".local/bin")
	os.MkdirAll(filepath.Join(PH, "Library/Logs"), 0o755)
	os.MkdirAll(bin, 0o755)
	if err := os.Symlink(e.guard, filepath.Join(bin, "hc-launchd-guard")); err != nil {
		t.Fatal(err)
	}
	clitest.Put(t, filepath.Join(bin, "hc"), fakeHC)
	clitest.Chmod(t, filepath.Join(bin, "hc"), 0o755)
	for _, p := range ps {
		b := filepath.Base(p)
		out, err := exec.Command("plutil", "-extract", "ProgramArguments.2", "raw", "-o", "-", p).Output()
		if err != nil {
			t.Fatalf("%s: plutil: %v", b, err)
		}
		cmd := strings.TrimSpace(string(out))
		_, w, _ := strings.Cut(cmd, " -- ")
		n, _, _ := strings.Cut(w, " ")
		c.Ok(b+": python3로 넘기지 않음(도구 "+n+")", n != "" && !strings.HasPrefix(n, "python"))
		clitest.Put(t, filepath.Join(bin, n), "#!/bin/sh\necho ran "+n+"\n")
		clitest.Chmod(t, filepath.Join(bin, n), 0o755)
		r := clitest.Cmd{Env: []string{"XDG_STATE_HOME", "HOME=" + PH, "T_LOG=" + filepath.Join(e.T, "pl.log"), "T_STATE=" + filepath.Join(e.T, "pl.state"),
			"HC_FAKE_RUN_EXEC=1", "HC_LAUNCHD_GUARD_LS_SECS", "HC_LAUNCHD_GUARD_RUN_SECS"}, Max: 15 * time.Second}.Run("/bin/sh", "-c", cmd)
		logs, _ := filepath.Glob(filepath.Join(PH, "Library/Logs/*.log"))
		more, _ := filepath.Glob(filepath.Join(PH, ".local/state/*/*.log"))
		ran := 0
		for _, l := range append(logs, more...) {
			ran += strings.Count("\n"+clitest.Read(l), "\nran "+n+"\n")
			os.Remove(l)
		}
		c.Ok(b+": -- 뒤 명령이 가짜 hc 아래서 실제로 돌아 0", r.RC == 0 && ran == 1)
		if r.RC != 0 || ran != 1 {
			t.Logf("%s: rc %d ran %d\n%s", b, r.RC, ran, r.Err)
		}
	}
	c.Done()
}

func TestVersion(t *testing.T) {
	r := clitest.Cmd{}.Run(clitest.Bin(t, "hc-launchd-guard"), "--version")
	if r.RC != 0 || r.Out != "hc-launchd-guard (Go)" {
		t.Fatalf("--version: %d %q", r.RC, r.Out)
	}
}
