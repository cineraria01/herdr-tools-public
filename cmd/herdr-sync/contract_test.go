package main

// herdr-sync 계약 시험(옛 tests/herdr-sync.test.sh, 2026-11-21 go-tests-ops): 임시 HOME 셋(맥, 워커 맥 w1, 리눅스 lx — lx는 uname이
// Linux이고 원격 조각을 dash로 돌린다), 가짜 hc(PATH 앞, machine ls·exec [-i] --on만), 임시 저장소 둘(범용 repo·개인 conf)과 그 bare
// origin을 두고 herdr-sync 바이너리(clitest.Bin)를 밖에서 부른다. 실제 기계·네트워크는 쓰지 않는다. 한 흐름이라 단계 순서대로 상태를 이어 쓴다.
// 다른 사본은 HERDR_SYNC=<경로>(시험 전용). 진짜 herdr-settings-merge·herdr-tools-config를 범용 저장소에 넣어 쓴다(HERDR_SETTINGS_MERGE·HERDR_TOOLS_CONFIG_BIN=<경로>).
// 가짜 저장소의 tools:bin/<이름> 줄은 일반 파일 맞추기의 보기다(그 저장소엔 cmd/가 없다) — 바이너리 줄은 16(release)

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

// sx: 시험 하나의 자리와 도우미(셸 시험의 전역 변수·함수)
type sx struct {
	t              *testing.T
	c              *ct.C
	r              ct.Res
	here, sync     string
	T, MAC, W1, LX string
	A, C, W1A      string // 나무 단계(15)의 맥 .agents/skills·.claude/skills, w1 .agents/skills
	sigRC          int    // signalMid로 끈 실행의 종료 코드
}

// fakeHC: 가짜 hc — machine ls와 exec [-i] --on만. 입력·출력은 진짜 hc처럼 8MB까지
const fakeHC = `#!/bin/bash
case $1 in
  machine) printf '{"machines":[%s]}\n' "$HC_FAKE_MACHINES" ;;
  exec) shift; in=0
    while [ "$1" != -- ]; do case $1 in -i) in=1 ;; --on) m=$2; shift ;;
      --timeout) # 진짜 hc처럼 1~600초만 받는다(2026-11-22 sync-hc-timeout)
        { [ "$2" -ge 1 ] && [ "$2" -le 600 ]; } 2>/dev/null || { echo "{\"error\":{\"kind\":\"invalid_params\",\"message\":\"timeout must be more than 0 and at most 600 s, not $2\"}}" >&2; exit 6; }
        shift ;;
      esac; shift; done; shift
    h=$HCT/m/$m
    echo "$m" >> "$HCT/calls"   # 기계마다 hc 호출 수를 센다
    { [ -d "$h" ] && [ ! -e "$h/OFFLINE" ]; } || { echo "connect failed: $m" >&2; exit 1; }
    # NOSH: PATH에 sh가 없는 기계(Windows 등) — 진짜 hc의 오류 글 그대로
    [ ! -e "$h/NOSH" ] || { echo '{"error":{"hint":"","kind":"spawn_failed","message":"cannot start sh: program not found"}}' >&2; exit 1; }
    # SPAWN: sh를 띄우지 못함 — 까닭(파일 내용, 예: 「Permission denied (os error 13)」)을 진짜 hc 꼴로 (ops/R-3A13)
    [ ! -e "$h/SPAWN" ] || { printf '{"error":{"hint":"check the program","kind":"spawn_failed","message":"cannot start sh: %s"}}\n' "$(cat "$h/SPAWN")" >&2; exit 1; }
    [ $in = 1 ] || exec < /dev/null
    if [ $in = 1 ]; then   # 진짜 hc처럼 입력은 8MB까지만 받는다
      cat > "$HCT/in.$$"
      [ "$(wc -c < "$HCT/in.$$")" -le 8388608 ] || { rm -f "$HCT/in.$$"; echo '{"error":{"kind":"invalid_params","message":"at most 8 MB piped in"}}' >&2; exit 6; }
      exec < "$HCT/in.$$" && rm -f "$HCT/in.$$"
    fi
    if [ $in = 1 ] && [ "${HC_FAKE_CORRUPT-}" = "$m" ]; then head -c 3 | PATH="$h/.fakebin:$PATH" HOME=$h "$@"; exit; fi
    # HC_FAKE_NOO: 그 기계 조회 출력에서 OS 줄(O)이 잘린 꼴 — 종료 0
    if [ "${HC_FAKE_NOO-}" = "$m" ]; then PATH="$h/.fakebin:$PATH" HOME=$h "$@" | grep -v '^O'; exit 0; fi
    # HC_FAKE_T255: 그 기계의 나무 보내기(T)는 다 보낸 뒤 255로 끝난다 (ops/R-3A23)
    if [ "${HC_FAKE_T255-}" = "$m" ] && [ "${!#}" = T ]; then PATH="$h/.fakebin:$PATH" HOME=$h "$@"; exit 255; fi
    # 출력도 8MB에서 자른다
    PATH="$h/.fakebin:$PATH" HOME=$h "$@" | head -c 8388608; exit "${PIPESTATUS[0]}" ;;
  *) echo "fake hc: $*" >&2; exit 2 ;;
esac
`

func (s *sx) run(args ...string) { s.runEnv(nil, args...) }

func (s *sx) runEnv(env []string, args ...string) {
	s.r = ct.Cmd{Env: env, Combined: true}.Run(s.sync, args...)
}

func (s *sx) ok(name string, b bool) { s.t.Helper(); s.c.Ok(name, b) }

// has: grep -Fxq (줄 통째로)
func (s *sx) has(l string) bool { return ct.HasLine(s.r.Out, l) }

// contains: grep -Fq
func (s *sx) contains(x string) bool { return strings.Contains(s.r.Out, x) }

// rx: grep -q <정규식> — 줄마다
func (s *sx) rx(pat string) bool {
	re := regexp.MustCompile(pat)
	for _, l := range ct.Lines(s.r.Out) {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

func (s *sx) put(p, content string)              { ct.Put(s.t, p, content) }
func (s *sx) putx(p, content string)             { ct.Put(s.t, p, content); ct.Chmod(s.t, p, 0o755) }
func (s *sx) is(p, want string) bool             { return ct.Is(p, want) }
func (s *sx) snap() string                       { return ct.Snap(s.T, "lock", "mac", "m") }
func (s *sx) origin(a ...string) string          { return ct.Git(s.T+"/origin.git", a...) }
func (s *sx) corigin(a ...string) string         { return ct.Git(s.T+"/conf.git", a...) }
func (s *sx) git(dir string, a ...string) string { return ct.MustGit(s.t, dir, a...) }

// gitLand: HERDR_LAND=1 git -C dir …
func (s *sx) gitLand(dir string, a ...string) {
	r := ct.Cmd{Env: []string{"HERDR_LAND=1"}, Combined: true}.Run("git", append([]string{"-C", dir}, a...)...)
	if r.RC != 0 {
		s.t.Fatalf("HERDR_LAND=1 git -C %s %v: %s", dir, a, r.Out)
	}
}

// commitPush: git add -A && commit -qm && push -q origin main (pre-push 관문 없는 준비 단계)
func (s *sx) commitPush(dir, msg string) {
	s.git(dir, "add", "-A")
	s.git(dir, "commit", "-qm", msg)
	s.git(dir, "push", "-q", "origin", "main")
}

// lane: 개인 저장소의 다른 clone(레인 착지 흉내)에서 고치고 push
func (s *sx) lane(msg string, edit func()) {
	edit()
	s.git(s.T+"/lane", "commit", "-qam", msg)
	s.git(s.T+"/lane", "push", "-q", "origin", "main")
}

func (s *sx) heads() string {
	return strings.Join([]string{s.origin("rev-parse", "main"), s.corigin("rev-parse", "main"),
		ct.Git(s.T+"/repo", "rev-parse", "HEAD"), ct.Git(s.T+"/conf", "rev-parse", "HEAD")}, " ")
}

// cpR: cp -R (rm -rf 먼저)
func (s *sx) cpR(src, dst string) {
	os.RemoveAll(dst)
	if r := (ct.Cmd{Combined: true}).Run("cp", "-R", src, dst); r.RC != 0 {
		s.t.Fatalf("cp -R %s %s: %s", src, dst, r.Out)
	}
}

// editLines: 파일을 줄마다 고친다(sed 흉내 — f가 false를 돌려주면 그 줄을 지운다)
func (s *sx) editLines(p string, f func(string) (string, bool)) {
	var out []string
	for _, l := range ct.Lines(ct.Read(p)) {
		if n, keep := f(l); keep {
			out = append(out, n)
		}
	}
	s.put(p, strings.Join(out, "\n")+"\n")
}

// jsonAt: jq -r .<키> 파일 (글이 아니면 JSON 꼴)
func jsonAt(p string, k string) string {
	var m map[string]any
	if json.Unmarshal([]byte(ct.Read(p)), &m) != nil {
		return ""
	}
	v, ok := m[k]
	if !ok {
		return "null"
	}
	if x, ok := v.(string); ok {
		return x
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// hookCommands: jq -c "[.hooks.PreToolUse[].hooks[].command]"
func hookCommands(p string) string {
	var m struct {
		Hooks struct {
			PreToolUse []struct {
				Hooks []struct{ Command string } `json:"hooks"`
			}
		} `json:"hooks"`
	}
	json.Unmarshal([]byte(ct.Read(p)), &m)
	cs := []string{}
	for _, e := range m.Hooks.PreToolUse {
		for _, h := range e.Hooks {
			cs = append(cs, h.Command)
		}
	}
	b, _ := json.Marshal(cs)
	return string(b)
}

// countLine: grep -cx <줄> 파일
func countLine(p, l string) int {
	n := 0
	for _, x := range ct.Lines(ct.Read(p)) {
		if x == l {
			n++
		}
	}
	return n
}

// snapFL: (cd d && find . \( -type f -o -type l \) -exec cksum {} + | sort) 같은 판정
func snapFL(d string) string {
	var rows []string
	filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(d, p)
		if e.Type()&fs.ModeSymlink != 0 {
			rows = append(rows, rel+" -> "+ct.Readlink(p))
		} else if e.Type().IsRegular() {
			rows = append(rows, rel+" "+ct.Read(p))
		}
		return nil
	})
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

func copyExec(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("%s를 찾지 못했다: %v", src, err)
	}
	ct.Put(t, dst, string(b))
	ct.Chmod(t, dst, 0o755)
}

func TestContract(t *testing.T) {
	s := &sx{t: t, here: ct.Repo(t), sync: ct.Tool(t, "herdr-sync", "HERDR_SYNC")}
	merge := ct.Tool(t, "herdr-settings-merge", "HERDR_SETTINGS_MERGE")   // 진짜 herdr-settings-merge를 기계에 깔아 쓴다
	cfgtool := ct.Tool(t, "herdr-tools-config", "HERDR_TOOLS_CONFIG_BIN") // 개인 값 검사·prebuild 읽기에 진짜 도우미를 쓴다
	T := ct.TempDir(t)
	s.T, s.MAC, s.W1, s.LX = T, T+"/mac", T+"/m/w1", T+"/m/lx"
	ct.Isolate(t, T)
	s.c = &ct.C{T: t, Detail: func() string {
		ls := ct.Lines(s.r.Out)
		if len(ls) > 30 {
			ls = ls[len(ls)-30:]
		}
		return "    | " + strings.Join(ls, "\n    | ")
	}}
	for _, d := range []string{T + "/bin", s.MAC + "/Applications/Herdr.app", s.W1, s.LX + "/.fakebin", T + "/tmp"} {
		os.MkdirAll(d, 0o755)
	}
	s.putx(T+"/bin/hc", fakeHC)
	s.putx(s.LX+"/.fakebin/uname", "#!/bin/sh\necho Linux\n")
	os.Symlink("/bin/dash", s.LX+"/.fakebin/sh")
	for k, v := range map[string]string{"TMPDIR": T + "/tmp", "PATH": T + "/bin:" + os.Getenv("PATH"),
		"HCT": T, "HC_FAKE_MACHINES": `{"name":"w1"},{"name":"lx"}`, "HERDR_TOOLS_REPO": T + "/repo", "HERDR_TOOLS_CONFIG": T + "/conf",
		"HOME": s.MAC, "HERDR_SYNC_SYSAPPS": T + "/nosys", // 이 기계의 /Applications/Herdr.app 유무가 시험에 끼지 않게
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t", "GIT_CONFIG_NOSYSTEM": "1"} {
		t.Setenv(k, v)
	}

	// 범용 저장소(repo): 깔 도구 하나(hc-lane), herdr-settings-merge, herdr-tools-config.
	// 개인 저장소(conf): sync.list(tools: 줄은 repo), keys.json, config(prebuild), 개인 값 패턴. push는 둘 다 HERDR_LAND=1만 받는다
	for _, r := range []string{"origin", "conf"} {
		s.git(T, "init", "-q", "--bare", "-b", "main", T+"/"+r+".git")
	}
	s.git(T, "clone", "-q", T+"/origin.git", T+"/repo")
	s.git(T, "clone", "-q", T+"/conf.git", T+"/conf")
	s.put(T+"/conf/sync.list", `# 시험용
지침본문 .claude/CLAUDE.md#body          claude/CLAUDE.body.md      all
주제지침 .claude/docs/*.md               claude/docs/               all
Codex공통 .codex/AGENTS.md#codex-shared  codex/AGENTS.shared.md     all
스킬     .agents/skills/herdr-desktop/SKILL.md  skills/herdr-desktop/SKILL.md  macos
스킬     .claude/skills/herdr-desktop@   .agents/skills/herdr-desktop   macos
도구     .local/bin/hc-lane              tools:bin/hc-lane                macos
도구     .local/bin/herdr-settings-merge tools:bin/herdr-settings-merge   all
설정     .local/share/herdr-sync/keys.json settings/keys.json       all
맥도구   .local/bin/herdr-balance        tools:bin/herdr-balance          local
맥도구   .local/share/claude-md/*.py     share/claude-md/           local
지움     .agents/skills/herdr-ovis       -                          macos
지움     .local/bin/claude-md-sync       -                          local
`)
	s.putx(T+"/repo/bin/hc-lane", "#!/bin/sh\necho lane\n")
	copyExec(t, merge, T+"/repo/bin/herdr-settings-merge")
	copyExec(t, cfgtool, T+"/repo/bin/herdr-tools-config")
	// 가짜 공개 내보내기: 부른 것을 적는다(PUB_FAKE_RC로 실패시킴). 성공이면 「검사한」 커밋(origin/main, PUB_FAKE_CHECKED로 바꿈)을 결과 파일에
	s.putx(T+"/bin/herdr-tools-publish", "#!/bin/sh\necho \"$HERDR_TOOLS_REPO $HERDR_TOOLS_CONFIG $*\" >> \"$HCT/published\"\n"+
		"[ \"${PUB_FAKE_RC:-0}\" = 0 ] || exit $PUB_FAKE_RC\n"+
		// PUB_FAKE_HANG=<파일>: 게시기가 배경 자식(push 흉내)을 띄워 기다린다 — 자기 pid는 <파일>, 자식 pid는 <파일>.bg(재감사 ops R-09)
		"[ -z \"${PUB_FAKE_HANG-}\" ] || { sleep 60 & echo $! > \"$PUB_FAKE_HANG.bg\"; echo $$ > \"$PUB_FAKE_HANG\"; wait; exit 0; }\n"+
		"[ -z \"${HERDR_TOOLS_PUBLISH_RESULT-}\" ] || echo \"${PUB_FAKE_CHECKED:-$(git -C \"$HERDR_TOOLS_REPO\" rev-parse origin/main)}\" > \"$HERDR_TOOLS_PUBLISH_RESULT\"\n")
	s.put(T+"/conf/settings/keys.json", `{"values":[".language",".model",".effortLevel",".nope"],"hooks":{"PreToolUse":["cotenant-guard.sh"]},"refs":[".permissions"],"exceptions":{"lx":[".effortLevel"]}}`+"\n")
	s.put(T+"/conf/config", "sync.prebuild  python3 ~/.local/share/claude-md/build_codex.py\n")
	s.put(T+"/conf/private-patterns.txt", "secret-word\n")
	for _, r := range []string{"repo", "conf"} {
		s.commitPush(T+"/"+r, "init")
		s.putx(T+"/"+r+"/.git/hooks/pre-push", "#!/bin/sh\n[ \"$HERDR_LAND\" = 1 ]\n")
	}

	// 맥: 지침·주제 지침·Codex·스킬·링크·맥 전용 도구·지울 것·설정
	M, W1, LX := s.MAC, s.W1, s.LX
	s.put(M+"/.claude/CLAUDE.md", "OMC mac\n<!-- User customizations -->\n본문 1\n")
	s.put(M+"/.claude/docs/a.md", "a1\n")
	s.put(M+"/.claude/docs/b.md", "b1\n")
	s.put(M+"/.codex/AGENTS.md", "mac 전용\n<!-- codex-shared:start -->\ncodex 0\n<!-- codex-shared:end -->\n")
	s.put(M+"/.agents/skills/herdr-desktop/SKILL.md", "skill 1\n")
	os.MkdirAll(M+"/.claude/skills", 0o755)
	os.Symlink(M+"/.agents/skills/herdr-desktop", M+"/.claude/skills/herdr-desktop")
	s.putx(M+"/.local/bin/herdr-balance", "#!/bin/sh\necho bal\n")
	s.put(M+"/.local/bin/claude-md-sync", "old\n")
	s.put(M+"/.local/share/claude-md/build_codex.py", `import os
p = os.path.expanduser("~/.codex/AGENTS.md"); s = open(p).read()
a, rest = s.split("<!-- codex-shared:start -->\n", 1); _, b = rest.split("<!-- codex-shared:end -->\n", 1)
open(p, "w").write(a + "<!-- codex-shared:start -->\ncodex built\n<!-- codex-shared:end -->\n" + b)
`)
	s.put(M+"/.claude/settings.json", `{"language":"한국어","model":"opus","effortLevel":"xhigh","permissions":{"allow":["x"]},
"hooks":{"PreToolUse":[{"matcher":"Edit|Write","hooks":[{"type":"command","command":"`+M+`/.claude/hooks/cotenant-guard.sh"}]},
{"matcher":"x","hooks":[{"type":"command","command":"moshi.sh"}]}]}}
`)
	// w1: 본문은 같고 OMC 블록만 다름, b.md는 base 없이 다름(첫 실행 충돌), 지울 옛 스킬 있음, Codex 지침 없음
	s.put(W1+"/.claude/CLAUDE.md", "OMC w1\n<!-- User customizations -->\n본문 1\n")
	s.put(W1+"/.claude/docs/a.md", "a1\n")
	s.put(W1+"/.claude/docs/b.md", "b-old\n")
	s.put(W1+"/.agents/skills/herdr-ovis/SKILL.md", "ovis\n")
	s.put(W1+"/.claude/settings.json", `{"language":"한국어","model":"opus","effortLevel":"xhigh","theme":"w1"}`+"\n")
	// lx: 본문이 다름(첫 실행 충돌), 주제 지침 없음, 표식 없는 Codex 지침, 옛 스킬(리눅스는 mac7을 받지 않는다)
	s.put(LX+"/.claude/CLAUDE.md", "OMC lx\n<!-- User customizations -->\n본문 0\n")
	s.put(LX+"/.codex/AGENTS.md", "lx 머리\n<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n꼬리\n")
	s.put(LX+"/.agents/skills/herdr-ovis/SKILL.md", "ovis\n")
	s.put(LX+"/.claude/settings.json", `{"language":"한국어","model":"opus","effortLevel":"medium","permissions":{"allow":["y"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"moshi-lx.sh"}]}]}}`+"\n")

	s.firstRuns()   // 1~4
	s.repoAndMore() // 5~12
	s.trees()       // 15(나무)
	s.lockAndRest() // 13·14·15(공개)·16
	s.ok("임시 파일이 남지 않는다 (맥·기계 쪽 조각 모두)", ct.Empty(T+"/tmp"))
	s.failures() // 17(실패 주입 — SIGKILL한 실행은 임시 폴더를 남기니 위 확인 뒤에)
	s.gofix4()   // 18(재감사 바퀴 3)
	s.c.Done()
}
