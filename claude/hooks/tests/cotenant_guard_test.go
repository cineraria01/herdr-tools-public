package tests

// cotenant-guard.test.sh(PreToolUse 임대 가드 시험)를 옮긴 것. 임시 git 저장소, 가짜 세션 등록부(가짜 HOME), PATH 앞의 가짜 hc,
// 에이전트 노릇을 하는 sleep 프로세스. 검사 하나가 부분 시험 하나이고 이름은 셸 시험의 check 이름 그대로다.
// 검사들은 앞 검사가 남긴 상태(임대·파일·등록부)에 기대므로 순서대로 돈다.

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const deadST = "Mon Jan  1 00:00:00 2001"

type cg struct {
	t        *testing.T
	T, H     string
	hook     string   // HOOK
	impl     []string // <훅>_IMPL=…
	hookEnv  []string // HOOK_ENV
	R        string   // hook의 기본 cwd
	kids     []*exec.Cmd
	RC       int
	OUT, ERR string
	calmW    int
	busy     string
}

// obj: 키 순서를 지키는 JSON 객체(jq -cn과 같은 꼴). 값이 nil이면 null.
func obj(kv ...any) json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(toolInput(kv[i].(string)))
		b.WriteByte(':')
		if r, ok := kv[i+1].(json.RawMessage); ok {
			b.Write(r)
		} else {
			b.Write(toolInput(kv[i+1]))
		}
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}

func md5hex(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

func rc(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func (c *cg) fatal(err error) {
	c.t.Helper()
	if err != nil {
		c.t.Fatal(err)
	}
}

func (c *cg) git(dir string, args ...string) string {
	c.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		c.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

func (c *cg) write(p, s string) {
	c.t.Helper()
	c.fatal(os.MkdirAll(filepath.Dir(p), 0o755))
	c.fatal(os.WriteFile(p, []byte(s), 0o644))
}

func (c *cg) appendf(p, s string) {
	c.t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	c.fatal(err)
	_, err = f.WriteString(s)
	c.fatal(err)
	c.fatal(f.Close())
}

// touchOld: touch -t 202601010000
func (c *cg) touchOld(p string) {
	c.t.Helper()
	tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	c.fatal(os.Chtimes(p, tm, tm))
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func isDir(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

func mtime(c *cg, p string) int64 {
	c.t.Helper()
	fi, err := os.Stat(p)
	c.fatal(err)
	return fi.ModTime().Unix()
}

// lsCount: ls <dir> | grep -c '^<prefix>' (점으로 시작하는 것은 ls가 안 보인다)
func (c *cg) lsCount(dir, prefix string) int {
	c.t.Helper()
	es, err := os.ReadDir(dir)
	c.fatal(err)
	n := 0
	for _, e := range es {
		if !strings.HasPrefix(e.Name(), ".") && strings.HasPrefix(e.Name(), prefix) {
			n++
		}
	}
	return n
}

func (c *cg) mkrepo(dir string) {
	c.t.Helper()
	out, err := exec.Command("git", "init", "-q", "-b", "main", dir).CombinedOutput()
	if err != nil {
		c.t.Fatalf("git init: %v\n%s", err, out)
	}
	c.write(dir+"/a.txt", "a\n")
	c.write(dir+"/b.txt", "b\n")
	c.write(dir+"/sub/c.txt", "c\n")
	for _, f := range []string{"d", "e", "f", "g", "h"} {
		c.write(dir+"/"+f+".txt", f+"\n")
	}
	c.git(dir, "add", "-A")
	c.git(dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init")
}

func (c *cg) leaseDirs(repo string) string {
	return c.git(repo, "rev-parse", "--absolute-git-dir") + "/agent-leases"
}

func (c *cg) leasefile(repo, rel string) string {
	return c.leaseDirs(repo) + "/" + md5hex(rel) + "/owner"
}

// field: cut -d $'\037' -f<n> <file> (한 줄)
func field(p string, n int) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	f := strings.Split(strings.TrimRight(string(b), "\n"), "\x1f")
	if n-1 < len(f) {
		return f[n-1]
	}
	return ""
}

func (c *cg) agent() int {
	c.t.Helper()
	cmd := exec.Command("sleep", "900")
	c.fatal(cmd.Start())
	c.kids = append(c.kids, cmd)
	return cmd.Process.Pid
}

func (c *cg) killAgent(pid int) {
	for _, k := range c.kids {
		if k.Process.Pid == pid {
			k.Process.Signal(syscall.SIGTERM) //nolint:errcheck // 이미 끝났어도 된다
			k.Wait()                          //nolint:errcheck // TERM으로 끝난 종료 코드
			return
		}
	}
}

func procStart(c *cg, pid int) string {
	c.t.Helper()
	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = envWith(nil, "LC_ALL=C", "TZ=UTC")
	out, err := cmd.Output()
	c.fatal(err)
	return strings.TrimRight(string(out), " \n")
}

// reg PID SESSION CWD NAME [PROCSTART] — Claude처럼 줄바꿈 없는 압축 JSON
func (c *cg) reg(pid int, session, cwd, name string, st ...string) {
	c.t.Helper()
	s := ""
	if len(st) > 0 {
		s = st[0]
	} else {
		s = procStart(c, pid)
	}
	c.write(fmt.Sprintf("%s/.claude/sessions/%d.json", c.H, pid),
		string(obj("pid", pid, "sessionId", session, "cwd", cwd, "name", name, "procStart", s, "status", "busy")))
}

func (c *cg) unreg(pid int) { c.fatal(os.Remove(fmt.Sprintf("%s/.claude/sessions/%d.json", c.H, pid))) }

// exec1: 훅을 bash(PATH에서 찾음, bashBin이 있으면 그것)로 부른다. RC·OUT·ERR을 채운다.
func (c *cg) exec1(bashBin, hook string, in []byte, env []string) {
	c.t.Helper()
	cmd := exec.Command(bashBin, hook)
	cmd.Stdin = strings.NewReader(string(in))
	cmd.Env = env
	cmd.Dir = c.T
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	c.RC = 0
	if ee, ok := err.(*exec.ExitError); ok {
		c.RC = ee.ExitCode()
	} else if err != nil {
		c.t.Fatalf("훅 실행: %v", err)
	}
	c.OUT, c.ERR = strings.TrimRight(o.String(), "\n"), strings.TrimRight(e.String(), "\n")
}

func (c *cg) env(path string) []string {
	set := append([]string{"HOME=" + c.H, "PATH=" + path}, c.hookEnv...)
	return envWith([]string{"CLAUDE_CONFIG_DIR", "HC_TERMINAL_ID"}, append(set, c.impl...)...)
}

func (c *cg) stdPath() string { return c.T + "/bin:" + os.Getenv("PATH") }

// hookIn: hook SESSION TOOL PATH-OR-COMMAND [AGENT_ID] [CWD]의 입력
func (c *cg) hookIn(s, tool, arg, agentID, cwd string) []byte {
	if cwd == "" {
		cwd = c.R
	}
	var a any
	if agentID != "" {
		a = agentID
	}
	ti := obj("file_path", arg)
	if tool == "Bash" {
		ti = obj("command", arg)
	}
	return obj("session_id", s, "agent_id", a, "cwd", cwd, "tool_name", tool, "tool_input", ti)
}

// run: hook SESSION TOOL ARG [AGENT_ID [CWD]]
func (c *cg) run(s, tool, arg string, rest ...string) {
	c.t.Helper()
	agentID, cwd := "", ""
	if len(rest) > 0 {
		agentID = rest[0]
	}
	if len(rest) > 1 {
		cwd = rest[1]
	}
	c.exec1("bash", c.hook, c.hookIn(s, tool, arg, agentID, cwd), c.env(c.stdPath()))
}

// check NAME WANT_RC [PATTERN in stderr+stdout] (패턴은 Go 정규식으로 옮긴 grep 패턴)
func (c *cg) check(name string, want int, pat ...string) {
	c.t.Helper()
	rcv, out, errOut := c.RC, c.OUT, c.ERR
	c.t.Run(name, func(t *testing.T) {
		if rcv != want || (len(pat) > 0 && !regexp.MustCompile(pat[0]).MatchString(errOut+"\n"+out)) {
			t.Errorf("rc=%d, want %d %v err=[%s] out=[%s]", rcv, want, pat, errOut, out)
		}
	})
}

func (c *cg) checkOK(name string, ok bool, want int) {
	c.t.Helper()
	c.RC, c.OUT, c.ERR = rc(ok), "", ""
	c.check(name, want)
}

// 시간 시험은 맥 부하에 기대지 않게: calm이 1분 부하가 CPU 수 아래로 내리기를 10초마다 기다린다(이 시나리오 전체에서 합쳐 최대
// COTENANT_CALM_MAX초, 기본 600 — 단 go test 기한을 넘지 않게 그 앞 90초까지). 그래도 높으면 시간 검사만 건너뛴다.
func (c *cg) calm() {
	mx := 600
	if v, err := strconv.Atoi(os.Getenv("COTENANT_CALM_MAX")); err == nil {
		mx = v
	}
	if dl, ok := c.t.Deadline(); ok {
		if left := int(time.Until(dl).Seconds()) - 90; left < mx {
			mx = left
		}
	}
	nc := 4
	if out, err := exec.Command("sysctl", "-n", "hw.ncpu").Output(); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			nc = v
		}
	}
	for {
		l := -1
		if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
			if f := strings.Fields(string(out)); len(f) > 1 {
				if v, err := strconv.ParseFloat(f[1], 64); err == nil {
					l = int(v)
				}
			}
		}
		if l < 0 || l < nc {
			c.busy = ""
			return
		}
		if c.calmW >= mx {
			c.busy = fmt.Sprintf("1분 부하 %d ≥ CPU %d", l, nc)
			c.t.Logf("calm: %s — 합쳐 %d초 기다려도 안 내려 그대로 잰다", c.busy, c.calmW)
			return
		}
		time.Sleep(10 * time.Second)
		c.calmW += 10
	}
}

func (c *cg) timeok(ms, limit int64) bool {
	if c.busy != "" {
		c.t.Logf("부하로 건너뜀: 시간 검사(%dms, 상한 %dms) — %s", ms, limit, c.busy)
		return true
	}
	return ms < limit
}

// rec LEASES REL PID PROCSTART [old] — 임대 기록(old: 주인이 2026-01-01 뒤로 안 건드림). 그 폴더를 돌려준다
func (c *cg) rec(leases, rel string, pid int, st string, old bool) string {
	d := leases + "/" + md5hex(rel)
	c.write(d+"/owner", fmt.Sprintf("s\x1f%d\x1f%s\x1f\x1fowner-%d\x1f2026-01-01 00:00:00\x1f%s\n", pid, st, pid, rel))
	if old {
		c.touchOld(d + "/owner")
	}
	return d
}

// race N PREFIX TOOL ARG CWD -> 통과 수, 걸린 ms: PREFIX1..N이 한순간에 훅을 부른다
func (c *cg) race(n int, prefix, tool, arg, cwd string) (passed int, ms int64) {
	t0 := time.Now().Add(500 * time.Millisecond)
	rcs := make([]int, n)
	var wg sync.WaitGroup
	for k := 1; k <= n; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			ti := obj("file_path", arg)
			if tool == "Bash" {
				ti = obj("command", arg)
			}
			in := obj("session_id", fmt.Sprintf("%s%d", prefix, k), "cwd", cwd, "tool_name", tool, "tool_input", ti)
			time.Sleep(time.Until(t0))
			cmd := exec.Command("bash", c.hook)
			cmd.Stdin = strings.NewReader(string(in))
			cmd.Env = c.env(c.stdPath())
			cmd.Dir = c.T
			err := cmd.Run()
			if ee, ok := err.(*exec.ExitError); ok {
				rcs[k-1] = ee.ExitCode()
			} else if err != nil {
				rcs[k-1] = -1
			}
		}(k)
	}
	wg.Wait()
	ms = time.Since(t0).Milliseconds()
	for _, r := range rcs {
		if r == 0 {
			passed++
		}
	}
	return passed, ms
}

// sedLines: 줄마다 처음 하나를 바꾼다(sed s|old|new|)
func sedLines(s, old, repl string) string {
	ls := strings.Split(s, "\n")
	for i := range ls {
		ls[i] = strings.Replace(ls[i], old, repl, 1)
	}
	return strings.Join(ls, "\n")
}

func TestCotenantGuard(t *testing.T) {
	hook := hookPath(t, "cotenant-guard")
	forImpls(t, "cotenant-guard", func(t *testing.T, impl []string) {
		T, err := os.MkdirTemp(os.Getenv("TMPDIR"), "cotenant-test.")
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
		cotenantScenario(c)
	})
}

// cotenantScenario: 셸 시험 차례 그대로 — 뒤 절이 앞 절의 저장소·에이전트(A·B·C)를 이어 쓴다. 600줄 한도로 절을 파일 셋에 나눴다.
func cotenantScenario(c *cg) {
	A, B, C := cotenantBasics(c)
	cotenantFix12(c, A, B, C)
	cotenantFix34(c)
}

// settingsHookCommand: settings.json의 PreToolUse 훅 가운데 command에 name이 든 첫 것
// (jq -r '.hooks.PreToolUse[]?.hooks[]? | select(.command | test(name)) | .command' | head -1). 못 읽으면 "".
func settingsHookCommand(path, name string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s struct {
		Hooks struct {
			PreToolUse []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if json.Unmarshal(b, &s) != nil {
		return ""
	}
	for _, m := range s.Hooks.PreToolUse {
		for _, h := range m.Hooks {
			if strings.Contains(h.Command, name) {
				return h.Command
			}
		}
	}
	return ""
}
