package main

// 끝에서 끝 대조(느림 — HERDR_HOOKS_E2E=1일 때만): 같은 입력 JSON을 셸 판 훅 파일 통째(HERDR_HOOKS_IMPL=bash — jq·perl·awk 포함)와
// Go 판(이 시험 바이너리를 훅으로 다시 띄움)에 넣어 종료 코드·표준 오류·표준 출력(JSON은 뜻으로)이 같은지 본다.
// 두 판 모두 이 시험 프로세스의 자식이라 부모 pid(임대 주인)가 같다. cotenant-guard는 가짜 HOME·임시 저장소·동거 에이전트에서,
// 두 판을 같은 처음 상태(임대 폴더를 다시 깖)에서 따로 돌려 종료 코드·출력·남은 임대를 견준다.
// 실패 로그에 HERDR_HOOKS_CORPUS의 실제 명령 원문·출력은 적지 않는다(번호만 — diffErr).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if h := os.Getenv("HERDR_HOOKS_E2E_CHILD"); h != "" {
		in, _ := readAllStdin()
		os.Exit(hooks[h](in, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func readAllStdin() ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(os.Stdin)
	return b.Bytes(), err
}

type hookResult struct {
	rc       int
	out, err string
}

func runHook(t *testing.T, name string, goImpl bool, env []string, in []byte) hookResult {
	var c *exec.Cmd
	if goImpl {
		c = exec.Command(os.Args[0])
		env = append(env, "HERDR_HOOKS_E2E_CHILD="+name)
	} else {
		sh, _ := filepath.Abs(filepath.Join("..", "..", "claude", "hooks", name+".sh"))
		c = exec.Command("/bin/bash", sh)
		env = append(env, "HERDR_HOOKS_IMPL=bash")
	}
	c.Env = env
	c.Stdin = bytes.NewReader(in)
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return hookResult{rc, o.String(), e.String()}
}

func sameOut(a, b string) bool {
	if a == b {
		return true
	}
	var x, y any
	return json.Unmarshal([]byte(a), &x) == nil && json.Unmarshal([]byte(b), &y) == nil && fmt.Sprint(x) == fmt.Sprint(y)
}

func e2eInputs(t *testing.T, limit int) []string {
	in := corpus(t) // 고르게 솎는다(시험 명령·섞음·실제 명령이 다 들게)
	if len(in) <= limit {
		return in
	}
	var out []string
	for i := 0; i < limit; i++ {
		out = append(out, in[i*len(in)/limit])
	}
	return out
}

func bashJSON(cmd string, bg bool) []byte {
	ti := map[string]any{"command": cmd}
	if bg {
		ti["run_in_background"] = true
	}
	b, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": ti})
	return b
}

func TestE2ESmallHooks(t *testing.T) {
	if os.Getenv("HERDR_HOOKS_E2E") == "" {
		t.Skip("HERDR_HOOKS_E2E=1일 때만")
	}
	base := []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "HC_LANE=lane-x"}
	bad := 0
	ins := e2eInputs(t, 3000)
	for i, c := range ins {
		for _, h := range []string{"hc-lane-wait-guard", "kill-by-lookup-guard", "lane-no-shell-edit"} {
			in := bashJSON(c, i%7 == 0)
			if i%11 == 0 { // 깨진·이상한 입력도
				in = []byte(c)
			}
			a, b := runHook(t, h, false, base, in), runHook(t, h, true, base, in)
			if a.rc != b.rc || a.err != b.err || a.out != b.out {
				bad++
				if bad <= 10 {
					diffErr(t, h, c, fmt.Sprintf("bash %d·go %d", a.rc, b.rc), a.err, b.err)
				}
			}
		}
	}
	t.Logf("작은 훅 셋 × %d개 대조, 다름 %d", len(ins), bad)
}

func TestE2ECotenant(t *testing.T) {
	if os.Getenv("HERDR_HOOKS_E2E") == "" {
		t.Skip("HERDR_HOOKS_E2E=1일 때만")
	}
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	home := tmp + "/home"
	repo := tmp + "/r"
	must := func(args ...string) {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = tmp
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	must("mkdir", "-p", home+"/.claude/sessions", repo+"/sub", repo+"/src")
	os.WriteFile(home+"/.claude/settings.json", []byte(`{"env":{}}`), 0o644)
	for _, f := range []string{"a.txt", "b.txt", "sub/c.txt", "src/x.go", "README.md"} {
		os.WriteFile(repo+"/"+f, []byte("x\n"), 0o644)
	}
	must("git", "-C", repo, "init", "-q", "-b", "main")
	must("git", "-C", repo, "add", "-A")
	must("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init")
	// 동거 에이전트: 산 프로세스 하나, 레지스트리에 그 cwd = repo, 그가 a.txt를 고치는 중(미커밋 + 임대)
	ag := exec.Command("sleep", "900")
	if err := ag.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { ag.Process.Kill(); ag.Wait() }() // 거둬서(아스트라 L01)
	pid := fmt.Sprint(ag.Process.Pid)
	c := exec.Command("/bin/ps", "-o", "lstart=", "-p", pid)
	c.Env = []string{"LC_ALL=C", "TZ=UTC"}
	st, _ := c.Output()
	reg, _ := json.Marshal(map[string]any{"pid": ag.Process.Pid, "sessionId": "other", "cwd": repo, "name": "agent-b", "procStart": strings.TrimSpace(string(st))})
	os.WriteFile(home+"/.claude/sessions/"+pid+".json", reg, 0o644)
	os.WriteFile(repo+"/a.txt", []byte("changed\n"), 0o644)
	leases := repo + "/.git/agent-leases"
	ld := leases + "/" + md5hex("a.txt")
	owner := []byte("other\x1f" + pid + "\x1f" + strings.TrimSpace(string(st)) + "\x1f\x1fagent-b\x1f2026-01-01 00:00:00\x1fa.txt\n")
	// 판마다 같은 처음 상태(동거 에이전트의 a.txt 임대 하나)에서 돌리고, 종료 코드·출력과 함께 남은 임대(이름·owner 칸, 시각 빼고)를 견준다
	// (아스트라 M06: 셸 판 뒤에 Go 판을 같은 상태에 이어 돌려 첫 호출만 경고하는 것을 다름으로 세고, 임대만 다른 것은 놓쳤다)
	seed := func() {
		for i := 0; i < 3 && os.RemoveAll(leases) != nil; i++ { // 앞 판정이 뒤에 띄운 rm -rf(휴지통)와 겹치면 한 번 더
		}
		os.MkdirAll(ld, 0o755)
		os.WriteFile(ld+"/owner", owner, 0o644)
	}
	snap := func() string {
		ents, _ := os.ReadDir(leases)
		var out []string
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			b, _ := os.ReadFile(leases + "/" + e.Name() + "/owner")
			f := strings.Split(strings.TrimRight(string(b), "\n"), "\x1f")
			if len(f) == 7 {
				f[5] = ""
			}
			out = append(out, e.Name()+" "+strings.Join(f, "|"))
		}
		sort.Strings(out)
		return strings.Join(out, "\n")
	}
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + home}
	pair := func(in []byte) (a, b hookResult, same bool) {
		seed()
		a = runHook(t, "cotenant-guard", false, env, in)
		la := snap()
		seed()
		b = runHook(t, "cotenant-guard", true, env, in)
		lb := snap()
		return a, b, a.rc == b.rc && a.err == b.err && sameOut(a.out, b.out) && la == lb
	}
	bad, n := 0, 0
	for _, cmd := range e2eInputs(t, 4000) {
		// 이 기계의 실제 저장소를 건드리지 않게: 절대 경로·홈·위로 가는 경로는 뺀다
		if strings.Contains(cmd, "/Users") || strings.Contains(cmd, "~") || strings.Contains(cmd, "$HOME") || strings.Contains(cmd, "..") ||
			strings.Contains(cmd, "/private/") || strings.Contains(cmd, "/var/") || strings.Contains(cmd, "/tmp") {
			continue
		}
		n++
		in, _ := json.Marshal(map[string]any{"session_id": "me", "cwd": repo, "tool_name": "Bash", "tool_input": map[string]any{"command": cmd}})
		a, b, same := pair(in)
		if !same {
			bad++
			if bad <= 10 {
				diffErr(t, "cotenant", cmd, fmt.Sprintf("bash %d·go %d", a.rc, b.rc), a.err, a.out, b.err, b.out)
			}
		}
	}
	// 편집 도구도 몇 가지
	for _, fp := range []string{repo + "/a.txt", repo + "/b.txt", repo + "/new.txt", repo + "/sub/c.txt", "/etc/hosts", repo + "/.claude/settings.local.json"} {
		for _, tool := range []string{"Edit", "Write"} {
			in, _ := json.Marshal(map[string]any{"session_id": "me", "cwd": repo, "tool_name": tool, "tool_input": map[string]any{"file_path": fp, "content": "x"}})
			a, b, same := pair(in)
			n++
			if !same {
				bad++
				t.Errorf("cotenant %s %s:\n bash %d %q\n go   %d %q", tool, fp, a.rc, a.err, b.rc, b.err)
			}
		}
	}
	t.Logf("cotenant-guard %d개 대조, 다름 %d", n, bad)
}
