package main

// cotenant-guard — 같은 git 작업 트리를 두 에이전트가 함께 고치지 않게 하는 훅의 Go 판.
// 셸 판 claude/hooks/cotenant-guard.sh를 함수마다 옮겼다(이름도 같다: phys·tree·relof·intree·reg·me·alive·dirty·owner·
// mine·own·claim·claimgen·takeover·refuse_live·lease·changed_leases·scan_foreign·hc_who·cotenant·foreign_under·
// bash_git·bash_write·analyze·check_bash). 임대 파일 꼴·이름(md5)·claim 이름·문구·종료 코드가 같아 두 판이 섞여 돌아도 된다.
// 셸 판처럼 정말 cd 한다(cd -P → 물리 경로, 그 뒤 상대 경로도 셸과 같게 풀린다).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const us = "\x1f"

// cgExit: 셸 판의 exit — 훅 안 어디서든 판정을 끝낸다
type cgExit struct{ code int }

type cg struct {
	stderr, stdout io.Writer

	cfg, hcwd, sid, aid, tool, fp, cmd, newc string
	git, home, hcTerm                        string

	start  int64 // SECONDS=0 한 때(유닉스 초)
	now    int64
	nowSet bool

	tDir                 string
	tSet, tRC            bool
	root, gitdir, leases string
	pd, prest, full, rel string

	regDone, meDone        bool
	reg                    string
	mypid, mystart, myname string
	psc                    map[string]string
	aok                    map[string]bool

	oSID, oPID, oStart, oAID, oName, oSince, oPath string

	warn     string
	claimAge int64

	fSet   bool
	fRoot  string
	fdirty string

	cSet  bool
	cRoot string
	cRC   bool
	who   string

	pend string
}

func cotenantGuard(in []byte, stdout, stderr io.Writer) (code int) {
	g := &cg{stdout: stdout, stderr: stderr, start: time.Now().Unix() + testSecondsOffset(),
		psc: map[string]string{}, aok: map[string]bool{}}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(cgExit)
			if !ok {
				panic(r)
			}
			code = e.code
		}
	}()
	g.run(in)
	return 0
}

func (g *cg) exit(code int) { panic(cgExit{code}) }

func (g *cg) block(msg string) {
	fmt.Fprintf(g.stderr, "cotenant-guard: %s\n", msg)
	g.exit(2)
}

// arithFail: 셸 판 `$(( NOW - $(stat …) ))`에서 stat이 실패하면 bash가 산술 오류로 끝난다(종료 1 — 설정 onFailure로 막힘)
func (g *cg) arithFail(path string) {
	fmt.Fprintf(g.stderr, "cotenant-guard: %s: 시각을 읽지 못했다(산술 오류)\n", path)
	g.exit(1)
}

func (g *cg) secs() int64 { return time.Now().Unix() - g.start }

func (g *cg) nowT() int64 {
	if !g.nowSet {
		g.now, g.nowSet = time.Now().Unix()+testNowOffset(), true
	}
	return g.now
}

var leaseOff = regexp.MustCompile(`"AGENT_LEASE_OFF"[\t\n\v\f\r ]*:[\t\n\v\f\r ]*"?1`)
var settingsPath = regexp.MustCompile(`/\.claude/settings[^/]*\.json\n?\z`)

const offMsg = "훅·가드를 끄거나 임대를 지우는 일(disableAllHooks·AGENT_LEASE_OFF·.git/agent-leases)은 사람만 한다. 막혔으면 다른 일을 먼저 하고 나중에 다시 시도하거나 지휘에 알리거나 git worktree add …에서 하라."

func (g *cg) run(in []byte) {
	g.home = os.Getenv("HOME")
	g.hcTerm = os.Getenv("HC_TERMINAL_ID")
	g.cfg = os.Getenv("CLAUDE_CONFIG_DIR")
	if g.cfg == "" {
		g.cfg = g.home + "/.claude"
	}
	g.git = testGit()
	if g.git == "" {
		g.git = "/var/db/xcode_select_link/usr/bin/git"
		if !isExec(g.git) {
			g.git = "/usr/bin/git"
		}
	}
	if b, err := os.ReadFile(g.cfg + "/settings.json"); err == nil {
		if leaseOff.MatchString(capture(string(b))) {
			g.exit(0)
		}
	}
	if !g.parseInput(in) {
		fmt.Fprintln(g.stderr, "cotenant-guard: 훅 입력 JSON을 읽지 못했다")
		g.exit(1)
	}
	if g.hcwd == "" {
		g.hcwd, _ = os.Getwd()
	}
	if g.tool == "Bash" {
		if strings.Contains(g.cmd, "disableAllHooks") || strings.Contains(g.cmd, "AGENT_LEASE_OFF") || strings.Contains(g.cmd, ".git/agent-leases") {
			g.block(offMsg)
		}
	} else if strings.Contains(g.newc, "disableAllHooks") || strings.Contains(g.newc, "AGENT_LEASE_OFF") {
		g.block(offMsg)
	}
	switch g.tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		if g.fp != "" && g.relof(g.fp) {
			g.lease(g.rel, "1")
		}
	case "Bash":
		if g.cmd != "" {
			g.checkBash()
		}
	}
	if g.warn != "" {
		type hso struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		}
		out := struct {
			HookSpecificOutput hso `json:"hookSpecificOutput"`
		}{hso{"PreToolUse", "cotenant-guard: " + g.warn}}
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		if err := e.Encode(out); err == nil {
			g.stdout.Write(b.Bytes())
		}
	}
}

// parseInput: 셸 판의 `eval "$(jq -r '… @sh "SID=… AID=… HCWD=… TOOL=… FP=… CMD=… NEW=…"')"` — 값마다 한 줄, 마지막 값이 이긴다
func (g *cg) parseInput(in []byte) bool {
	vals, err := jqValues(in)
	if err != nil {
		return false
	}
	for _, v := range vals {
		f := map[string]string{}
		get := func(name string, keys ...string) bool {
			x, ok := jqPath(v, keys...)
			if !ok {
				return false
			}
			s, ok := shWord(x)
			f[name] = s
			return ok
		}
		ti, ok := jqPath(v, "tool_input")
		if !ok {
			return false
		}
		fpv, _ := jqPath(ti, "file_path")
		if !jqTruthy(fpv) {
			fpv, _ = jqPath(ti, "notebook_path")
		}
		if !jqTruthy(fpv) {
			fpv = ""
		}
		fps, isStr := fpv.(string)
		if !isStr { // test()는 문자열만 받는다
			return false
		}
		if !get("SID", "session_id") || !get("AID", "agent_id") || !get("HCWD", "cwd") || !get("TOOL", "tool_name") || !get("CMD", "tool_input", "command") {
			return false
		}
		f["FP"] = fps
		nw := ""
		if settingsPath.MatchString(fps) {
			var parts []string
			for _, k := range []string{"content", "new_string"} {
				if s, ok := mapGet(ti, k).(string); ok {
					parts = append(parts, s)
				}
			}
			ed := mapGet(ti, "edits")
			var elems []any
			switch x := ed.(type) {
			case []any:
				elems = x
			case map[string]any:
				for _, e := range x {
					elems = append(elems, e)
				}
			}
			for _, e := range elems {
				ns, ok := jqPath(e, "new_string")
				if !ok {
					return false
				}
				if s, ok := ns.(string); ok {
					parts = append(parts, s)
				}
			}
			nw = strings.Join(parts, "\n")
		}
		f["NEW"] = nw
		strip := func(s string) string { return strings.ReplaceAll(s, "\x00", "") }
		g.sid, g.aid, g.hcwd, g.tool = strip(f["SID"]), strip(f["AID"]), strip(f["HCWD"]), strip(f["TOOL"])
		g.fp, g.cmd, g.newc = strip(f["FP"]), strip(f["CMD"]), strip(f["NEW"])
	}
	return true
}

func mapGet(v any, k string) any {
	if m, ok := v.(map[string]any); ok {
		return m[k]
	}
	return nil
}

// shWord: `\(.x // "")`를 @sh로 낸 뒤 eval한 값 — 문자열·수·true. 객체·배열은 셸 판에서 오류이거나 eval이 깨진다(막는 쪽으로 false)
func shWord(x any) (string, bool) {
	if !jqTruthy(x) {
		return "", true
	}
	switch v := x.(type) {
	case string:
		return v, true
	case json.Number:
		return v.String(), true
	case bool:
		return "true", true
	}
	return "", false
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// norm: 공백만으로 가르고 한 칸으로 잇는다(ps는 lstart를 채워 쓰고 레지스트리는 안 그런다)
func norm(s string) string {
	var f []string
	for _, w := range strings.Split(s, " ") {
		if w != "" {
			f = append(f, w)
		}
	}
	return strings.Join(f, " ")
}

// cdP: cd -P -- p — 성공하면 물리 경로(bash PWD)
func cdP(p string) (string, bool) {
	if p == "" || os.Chdir(p) != nil {
		return "", false
	}
	wd, err := syscall.Getwd()
	if err != nil {
		return "", false
	}
	return wd, true
}

// phys: PD = $1의 가장 가까운 있는 폴더의 물리 경로, PREST = 그 아래 아직 없는 부분
func (g *cg) phys(p string) bool {
	rest := ""
	if !strings.HasPrefix(p, "/") {
		p = g.hcwd + "/" + p
	}
	for !isDir(p) {
		i := strings.LastIndexByte(p, '/')
		if i < 0 { // 셸 판은 여기서 끝없이 돈다(훅 시간 넘김 → 막힘)
			fmt.Fprintf(g.stderr, "cotenant-guard: 경로를 풀지 못했다: %s\n", p)
			g.exit(1)
		}
		if rest == "" {
			rest = p[i+1:]
		} else {
			rest = p[i+1:] + "/" + rest
		}
		p = p[:i]
		if p == "" {
			p = "/"
		}
	}
	wd, ok := cdP(p)
	if !ok {
		return false
	}
	g.pd, g.prest = wd, rest
	return true
}

// gitRun: 이 git으로 실행 — 표준 출력, 종료 코드(실행 못 함 127). 표준 오류는 버린다
func (g *cg) gitRun(env []string, stdin string, args ...string) (string, int) {
	c := exec.Command(g.git, args...)
	if env != nil {
		c.Env = append(os.Environ(), env...)
	}
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	out, err := c.Output()
	if err == nil {
		return string(out), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(out), ee.ExitCode()
	}
	return string(out), 127
}

// tree: 물리 폴더 $1의 ROOT GITDIR LEASES — 작업 트리 밖(.git 안 포함)이면 false
func (g *cg) tree(dir string) bool {
	if g.tSet && dir == g.tDir {
		return g.tRC
	}
	g.tDir, g.tSet, g.tRC = dir, true, false
	out, rc := g.gitRun(nil, "", "-C", dir, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if rc == 128 {
		return false
	}
	if rc != 0 {
		g.block(fmt.Sprintf("git을 실행하지 못했다(%s, 종료 %d). 판정할 수 없어 막았다. 사람에게 알려라.", g.git, rc))
	}
	o := capture(out)
	g.root, g.gitdir = o, o
	if i := strings.IndexByte(o, '\n'); i >= 0 {
		g.root, g.gitdir = o[:i], o[i+1:]
	}
	g.leases = g.gitdir + "/agent-leases"
	g.tRC = true
	return true
}

// relof: REL = $1의 ROOT 기준 경로(ROOT 자체나 밖이면 false), FULL = 물리 경로
func (g *cg) relof(p string) bool {
	if !g.phys(p) || !g.tree(g.pd) {
		return false
	}
	g.full = g.pd
	if g.prest != "" {
		g.full += "/" + g.prest
	}
	g.rel = strings.TrimPrefix(g.full, g.root+"/")
	return g.rel != g.full && g.rel != ""
}

// intree: 폴더 $1이 ROOT이거나 그 아래(안에 둔 다른 작업 트리 제외)
func (g *cg) intree(d string) bool {
	p, ok := cdP(d)
	if !ok {
		return false
	}
	if p == g.root {
		return true
	}
	if !strings.HasPrefix(p, g.root+"/") {
		return false
	}
	for p != g.root {
		if exists(p + "/.git") {
			return false
		}
		p = p[:strings.LastIndexByte(p, '/')]
	}
	return true
}
