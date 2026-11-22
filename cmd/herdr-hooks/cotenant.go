package main

// cotenant-guard — 같은 git 작업 트리를 두 에이전트가 함께 고치지 않게 하는 훅의 Go 판.
// 셸 판 claude/hooks/cotenant-guard.sh를 함수마다 옮겼다(이름도 같다: phys·tree·relof·intree·reg·me·alive·dirty·owner·
// mine·own·claim·claimgen·takeover·refuse_live·lease·changed_leases·scan_foreign·hc_who·cotenant·foreign_under·
// bash_git·bash_write·analyze·check_bash). 임대 파일 꼴·이름(md5)·claim 이름·문구·종료 코드가 같아 두 판이 섞여 돌아도 된다.
// 셸 판처럼 정말 cd 한다(cd -P → 심볼릭 링크만 푼 경로, 그 뒤 상대 경로도 셸과 같게 풀린다 — cdP).
// 셸 판과 다른 점(일부러, 안전 판정은 모르면 막는다 — 2026-11-22 gofix-hooks): git·ps·hc는 훅 기한 안에서만 기다리고 넘기면 모름(hookexec.go),
// git status·rev-parse·ps·hc ls 실패를 깨끗함·밖·죽음·없음으로 읽지 않는다, 임대 시각을 못 읽으면 종료 1(arithFail), 임대를 못 쓰면 막는다.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	prefix               string // tree가 본 폴더의 ROOT 기준 접두(git 철자, 끝 / 포함 또는 "")

	made []madeLease // 이번 호출이 만들거나 넘겨받은 임대 — Bash 여러 대상이 중간에 막히면 되돌린다(R-M04·R-M201)
	got  madeLease   // 마지막 claimgen이 만든 claim(takeover가 made로 옮긴다)

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
	hookBudget(4 * time.Second) // 훅 제한 5초에서 bash·실행기 몫을 뺀 것
	lastPD, patLocale = "", localeKind()
	patByte = patLocale != locUTF8
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

// arithFail: 임대 시각을 못 읽었다(정리자가 그 사이 치웠거나 읽을 수 없음) — 모르니 종료 1(설정 onFailure로 막힘).
// 셸 판은 `$(( NOW - $(stat …) ))` 산술 오류로 그 case 줄만 버리고 종료 0(통과)이다 — 알려진 차이(Fable M1, 셸 판은 곧 걷는다)
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
			g.checkWritten()
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

// lastPD: 마지막으로 성공한 cdP의 PWD — 상대 경로는 bash처럼 이 PWD 기준으로 푼다
var lastPD string

// cdP: cd -P -- p — 성공하면 bash 3.2의 PWD: 적은 경로에서 심볼릭 링크만 푼 값(철자 — 대소문자·한글 NFC/NFD — 는 적은 그대로).
// 임대 열쇠는 이 철자가 아니라 git 철자로 따로 정한다(canonRel — 재감사 R-H03, Fable H1·E3)
func cdP(p string) (string, bool) {
	if p == "" || os.Chdir(p) != nil {
		return "", false
	}
	if !strings.HasPrefix(p, "/") {
		base := lastPD
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", false
			}
			base = wd
		}
		p = base + "/" + p
	}
	wd, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", false
	}
	lastPD = wd
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

// gitRun: 이 git으로 실행(훅 기한 안 — hookexec.go) — 표준 출력, 종료 코드(실행 못 함 127, 시간 넘김 timeoutRC = 모름). 표준 오류는 버린다
func (g *cg) gitRun(env []string, stdin string, args ...string) (string, int) {
	out, _, rc := g.gitRunE(env, stdin, args...)
	return out, rc
}

func (g *cg) gitRunE(env []string, stdin string, args ...string) (string, string, int) {
	if env != nil {
		env = append(os.Environ(), env...)
	}
	return hookRun(append([]string{"git"}, args...), 0, env, stdin, g.git, args...)
}

// gitFail: 판정에 쓸 git이 실패·시간 넘김 — 모르니 막는다
func (g *cg) gitFail(what string, rc int) {
	why := fmt.Sprintf("종료 %d", rc)
	if rc == timeoutRC {
		why = "시간 넘김"
	}
	g.block(fmt.Sprintf("git을 실행하지 못했다(%s, %s, %s). 판정할 수 없어 막았다. 잠시 뒤 다시 하고, 되풀이되면 사람에게 알려라.", g.git, why, what))
}

// tree: 물리 폴더 $1의 ROOT GITDIR LEASES PREFIX — 작업 트리 밖(.git 안 포함)이면 false. 밖임은 git이 그렇게 말하고
// 위로 .git 표식도 없을 때만(종료 128이라도 dubious ownership·설정 깨짐 등은 모름 → 막음 — 아스트라 E01·Fable E4.
// 재감사 R-H04: .git을 읽을 수 없거나 gitfile이 끊겨도 git은 「not a git repository」라 영문 문구만으로 밖이라 했다).
// ROOT·PREFIX는 git의 철자(디스크의 대소문자, core.precomposeunicode면 NFC) — 임대 열쇠를 git이 내는 경로와 맞춘다(R-H03)
func (g *cg) tree(dir string) bool {
	if g.tSet && dir == g.tDir {
		return g.tRC
	}
	g.tDir, g.tSet, g.tRC = dir, true, false
	out, errOut, rc := g.gitRunE([]string{"LC_ALL=C", "LANGUAGE="}, "", "-C", dir, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--show-prefix")
	if rc == 128 && strings.Contains(errOut, "fatal: this operation must be run in a work tree") {
		return false
	}
	if rc == 128 && strings.Contains(errOut, "fatal: not a git repository") {
		if m := gitMarker(dir); m != "" {
			g.block(m + " 이 있는데 git이 저장소로 열지 못했다(읽을 수 없음·끊긴 gitfile 등). 작업 트리인지 판정할 수 없어 막았다. 사람에게 알려라.")
		}
		return false
	}
	if rc != 0 {
		g.gitFail("작업 트리(git rev-parse)", rc)
	}
	l := strings.Split(out, "\n")
	if len(l) != 4 || l[3] != "" || !strings.HasPrefix(l[0], "/") || !strings.HasPrefix(l[1], "/") || (l[2] != "" && !strings.HasSuffix(l[2], "/")) {
		g.block("git rev-parse가 작업 트리·git 폴더 두 절대 경로와 접두를 내지 않았다(" + g.git + "). 판정할 수 없어 막았다. 사람에게 알려라.")
	}
	g.root, g.gitdir, g.prefix = l[0], l[1], l[2]
	g.leases = g.gitdir + "/agent-leases"
	g.tRC = true
	return true
}

// gitMarker: dir이나 그 위 폴더에 .git이 있으면(또는 있는지 못 읽으면) 그 경로, 없으면 ""
func gitMarker(dir string) string {
	for p := dir; ; p = p[:strings.LastIndexByte(p, '/')] {
		if p == "" {
			p = "/"
		}
		m := strings.TrimSuffix(p, "/") + "/.git"
		if _, err := os.Lstat(m); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return m
		}
		if p == "/" || !strings.Contains(p, "/") {
			return ""
		}
	}
}

// relof: REL = $1의 ROOT 기준 경로(git 철자 — canonRel, ROOT 자체나 밖이면 false), FULL = 물리 경로
func (g *cg) relof(p string) bool {
	if !g.phys(p) || !g.tree(g.pd) {
		return false
	}
	g.full = g.pd
	if g.prest != "" {
		g.full += "/" + g.prest
	}
	g.rel = g.canonRel()
	return g.rel != ""
}

// canonRel: phys·tree가 정한 PD·PREST의 ROOT 기준 경로를 git이 내는 철자로(재감사 R-H03: 적은 철자 — 뿌리·하위 폴더의 대소문자,
// 디렉터리에서 읽은 NFD — 를 그대로 열쇠로 써, 같은 파일에 다른 임대가 생기고 뿌리 대소문자가 다르면 임대가 아예 없었다).
// 있는 폴더 부분은 git -C PD의 접두(PREFIX), 있는 파일 이름은 ls-files :(literal,icase)가 낸 철자, 없는 부분은 적은 대소문자를
// git argv의 NFC로(check-attr — ASCII면 그대로). git이 실패하면 막는다(모름)
func (g *cg) canonRel() string {
	if g.prest == "" {
		return strings.TrimSuffix(g.prefix, "/")
	}
	want := g.prefix + g.prest
	if !isASCII(g.prest) {
		out, rc := g.gitRun(nil, "", "-C", g.pd, "check-attr", "-z", "herdr-hooks-path", "--", g.prest)
		f := strings.Split(out, "\x00")
		if rc != 0 || len(f) != 4 || f[0] == "" || f[3] != "" {
			g.gitFail(g.prest+" 의 경로 철자(git check-attr)", rc)
		}
		want = g.prefix + f[0]
	}
	if strings.Contains(g.prest, "/") {
		return want
	}
	if _, err := os.Lstat(g.pd + "/" + g.prest); err != nil {
		return want
	}
	out, rc := g.gitRun(nil, "", "-C", g.pd, "ls-files", "-z", "-c", "-o", "--full-name", "--", ":(literal,icase)"+g.prest)
	if rc != 0 {
		g.gitFail(g.prest+" 의 경로 철자(git ls-files)", rc)
	}
	var hits []string
	for _, h := range strings.Split(out, "\x00") {
		if h == want { // 대소문자만 다른 두 파일이 함께 있는 파일시스템에서는 적은 쪽
			return want
		}
		if h != "" && (len(hits) == 0 || hits[0] != h) {
			hits = append(hits, h)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return want
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// writtenRel: 옛 판의 임대 열쇠 — 적은 철자 그대로의 FULL에서 ROOT만큼의 폴더 칸을 뗀 나머지(ROOT를 다른 대소문자로 적었어도 —
// 재감사 R-H203: 글자로 ROOT 접두를 찾아 뿌리 철자가 다르면 옛 열쇠를 못 찾았다), 열쇠가 REL과 같거나 FULL이 ROOT 아래가 아니면 "".
// 옛 판(재감사 R-H03 전)이 쓴 임대를 고친 판이 놓치지 않게 함께 본다(혼용 기간)
func (g *cg) writtenRel() string {
	n := strings.Count(g.root, "/")
	parts := strings.SplitN(g.full, "/", n+2)
	r := ""
	if len(parts) == n+2 && strings.EqualFold(strings.Join(parts[:n+1], "/"), g.root) {
		r = parts[n+1]
	} else if s, ok := strings.CutPrefix(g.full, g.root+"/"); ok {
		r = s
	}
	if r == g.rel {
		return ""
	}
	return r
}

// checkWritten: 옛 철자 열쇠(writtenRel)에 산 남의 임대가 있으면 그 규칙대로 막는다
func (g *cg) checkWritten() {
	if alt := g.writtenRel(); alt != "" {
		rel := g.rel
		g.lease(alt, "check")
		g.rel = rel
	}
}

// intree: 폴더 $1이 ROOT이거나 그 아래(안에 둔 다른 작업 트리 제외) — 같은 폴더인지는 파일시스템이 정한다(대소문자·NFD 철자가
// git의 ROOT와 달라도 — R-H03)
func (g *cg) intree(d string) bool {
	p, ok := cdP(d)
	if !ok {
		return false
	}
	rfi, err := os.Stat(g.root)
	if err != nil {
		return false
	}
	for {
		if fi, err := os.Stat(p); err == nil && os.SameFile(fi, rfi) {
			return true
		}
		if exists(p+"/.git") || p == "/" {
			return false
		}
		p = p[:strings.LastIndexByte(p, '/')]
		if p == "" {
			p = "/"
		}
	}
}
