package main_test

// omc-update 계약 시험: 지은 바이너리를 가짜 hc·claude·npm(접두사 둘)·zsh·bash·uname 앞에서 돌린다. 기계마다 HOME은 $ROOT/<기계>
// (이 기계는 $ROOT/local), 로그인 셸 PATH는 그 HOME의 .zshrc가 정한다. 모든 호출은 $LOG에 한 줄씩.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

func TestMain(m *testing.M) { clitest.Main(m) }

type kit struct {
	t                  *testing.T
	root, fb, log, bin string
}

// npmSh: 가짜 npm — 접두사는 npm_config_prefix, 없으면 이 npm의 기본(%s). view는 $ROOT/registry에 있는 판만 답한다
const npmSh = `#!/bin/sh
echo "npm[$0] $*" >>"$LOG"
p=${npm_config_prefix:-%s}
case $1 in
view) v=${2#oh-my-claude-sisyphus@}; grep -qx "$v" "$ROOT/registry" 2>/dev/null && echo "$v"; exit 0 ;;
prefix) echo "$p" ;;
pack) d=; while [ $# -gt 0 ]; do [ "$1" = --pack-destination ] && d=$2; shift; done
  v=$(cat VERSION) || exit 1; echo "$v" >"$d/oh-my-claude-sisyphus-$v.tgz"; echo "oh-my-claude-sisyphus-$v.tgz" ;;
install) a=$3; case $a in *.tgz) v=$(cat "$a") ;; *) v=${a#oh-my-claude-sisyphus@} ;; esac
  l=$p/lib/node_modules/oh-my-claude-sisyphus/bin; mkdir -p "$l" "$p/bin"
  printf '#!/bin/sh\necho "omc $*" >>"$LOG"\necho %%s\n' "$v" >"$l/cli"; chmod +x "$l/cli"
  for n in omc oh-my-claudecode omc-cli; do ln -sfn "$l/cli" "$p/bin/$n"; done ;;
esac
`

func newKit(t *testing.T) *kit {
	root, err := filepath.EvalSymlinks(t.TempDir()) // realpath가 푼 경로와 견주게(macOS /var → /private/var)
	if err != nil {
		t.Fatal(err)
	}
	k := &kit{t: t, root: root, fb: root + "/fb", log: root + "/log", bin: clitest.Bin(t, "omc-update")}
	k.put(k.fb+"/hc", `#!/bin/sh
echo "hc $*" >>"$LOG"
case $1 in
machine) cat "$ROOT/machines.json" ;;
exec) shift; m=; while [ "$1" != -- ]; do [ "$1" = --on ] && m=$2; shift; done; shift
  HOME=$ROOT/$m exec "$@" ;;
*) exit 9 ;;
esac
`, 0o755)
	k.put(k.fb+"/zsh", "#!/bin/sh\necho \"zsh $1\" >>\"$LOG\"\n[ \"$1\" = -lc ] || exit 9\nshift; c=$1; shift; exec /bin/sh -c \"$c\" \"$@\"\n", 0o755)
	k.put(k.fb+"/bash", "#!/bin/sh\necho \"bash $1\" >>\"$LOG\"\n[ \"$1\" = -lc ] || exit 9\nshift; c=$1; shift; exec /bin/sh -c \"$c\" \"$@\"\n", 0o755)
	k.put(k.fb+"/uname", "#!/bin/sh\necho \"${FAKE_UNAME:-Darwin}\"\n", 0o755)
	k.put(k.fb+"/claude", `#!/bin/sh
echo "claude $*" >>"$LOG"
[ -f "$HOME/claude-fail" ] && { echo "claude 안 됨" >&2; exit 1; }
if [ "$*" = "plugin update oh-my-claudecode@omc" ] && [ -f "$HOME/plugin-next" ]; then
  v=$(cat "$HOME/plugin-next"); mkdir -p "$HOME/cache/$v"; echo "$v" >"$HOME/cache/$v/VERSION"
  printf '{"version":2,"plugins":{"oh-my-claudecode@omc":[{"version":"%s","installPath":"%s"}]}}' "$v" "$HOME/cache/$v" >"$HOME/.claude/plugins/installed_plugins.json"
fi
`, 0o755)
	k.put(k.fb+"/npm", fmt.Sprintf(npmSh, "$HOME/brew"), 0o755)
	os.MkdirAll(root+"/tmp", 0o755)
	return k
}

func (k *kit) put(p, s string, m os.FileMode) {
	k.t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		k.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), m); err != nil {
		k.t.Fatal(err)
	}
}

// home: 기계 하나 — 플러그인 판, CLAUDE.md 표식, 로그인 셸 PATH 앞 폴더(HOME 아래 상대)
func (k *kit) home(m, plugin, md string, path ...string) string {
	h := k.root + "/" + m
	k.put(h+"/cache/"+plugin+"/VERSION", plugin+"\n", 0o644)
	k.put(h+"/.claude/plugins/installed_plugins.json", fmt.Sprintf(`{"version":2,"plugins":{"oh-my-claudecode@omc":[{"version":%q,"installPath":%q}]}}`, plugin, h+"/cache/"+plugin), 0o644)
	k.put(h+"/.claude/CLAUDE.md", "<!-- OMC:START -->\n<!-- OMC:VERSION:"+md+" -->\n", 0o644)
	var ps []string
	for _, p := range path {
		ps = append(ps, `$HOME/`+p)
	}
	// .zshrc가 변수 s를 써도 조각은 그대로 돈다(감사 낮음 1 — 옛 판은 조각을 s에 두고 .zshrc를 읽었다)
	k.put(h+"/.zshrc", "PATH="+strings.Join(append(ps, "$PATH"), ":")+"\nexport PATH\ns='echo 덮인 조각; exit 7'\n", 0o644)
	return h
}

// omc: 접두사 pre(HOME 아래)에 그 판의 omc를 깔아 둔다(가짜 npm install과 같은 꼴)
func (k *kit) omc(h, pre, v string) {
	l := h + "/" + pre + "/lib/node_modules/oh-my-claude-sisyphus/bin"
	k.put(l+"/cli", "#!/bin/sh\necho \"omc $*\" >>\"$LOG\"\necho "+v+"\n", 0o755)
	os.MkdirAll(h+"/"+pre+"/bin", 0o755)
	os.Symlink(l+"/cli", h+"/"+pre+"/bin/omc")
}

func (k *kit) run(env []string, args ...string) (string, int) {
	k.t.Helper()
	c := exec.Command(k.bin, args...)
	c.Env = append([]string{"PATH=" + k.fb + ":/usr/bin:/bin", "HOME=" + k.root + "/local", "ROOT=" + k.root,
		"LOG=" + k.log, "TMPDIR=" + k.root + "/tmp"}, env...)
	out, err := c.CombinedOutput()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else if err != nil {
		k.t.Fatal(err)
	}
	k.noOmcWrite()
	return string(out), rc
}

// (e) omc update·setup·update-reconcile을 절대 부르지 않는다 — omc는 --version만
func (k *kit) noOmcWrite() {
	for _, l := range strings.Split(clitest.Read(k.log), "\n") {
		if strings.HasPrefix(l, "omc ") && l != "omc --version" {
			k.t.Errorf("omc를 --version 말고 불렀다: %q", l)
		}
	}
}

func (k *kit) logHas(s string) bool { return strings.Contains(clitest.Read(k.log), s) }

func want(t *testing.T, out string, rc, wrc int, lines ...string) {
	t.Helper()
	if rc != wrc {
		t.Errorf("종료 %d, 바람 %d\n%s", rc, wrc, out)
	}
	for _, l := range lines {
		if !strings.Contains(out, l) {
			t.Errorf("출력에 %q 없음\n%s", l, out)
		}
	}
}

// (a) 같은 판이면 아무것도 깔지 않는다. 플러그인은 claude 명령으로 올리고, 로그인 셸은 macOS zsh
func TestSame(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.6", "5.8.6", "brew/bin")
	k.omc(h, "brew", "5.8.6")
	out, rc := k.run(nil)
	want(t, out, rc, 0, "이 기계  플러그인 5.8.6  CLI 5.8.6→5.8.6 (같음)  CLAUDE.md 5.8.6\n")
	if k.logHas("npm") || !k.logHas("claude plugin marketplace update omc\nclaude plugin update oh-my-claudecode@omc\n") || !k.logHas("zsh -lc") {
		t.Errorf("호출\n%s", clitest.Read(k.log))
	}
	if strings.Contains(out, "경고") {
		t.Errorf("경고가 없어야\n%s", out)
	}
}

// npm에 그 판이 있으면 npm install -g <꾸러미>@<판>. CLAUDE.md 표식이 다르면 경고만(고치지 않음)
func TestNpm(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.4", "5.8.4", "brew/bin")
	k.put(h+"/plugin-next", "5.8.6\n", 0o644)
	k.omc(h, "brew", "5.8.4")
	k.put(k.root+"/registry", "5.8.6\n", 0o644)
	out, rc := k.run(nil)
	want(t, out, rc, 0, "이 기계  플러그인 5.8.6  CLI 5.8.4→5.8.6 (npm)  CLAUDE.md 5.8.4\n", "경고: ~/.claude/CLAUDE.md OMC:VERSION 5.8.4 ≠ 플러그인 5.8.6")
	if !k.logHas("install -g oh-my-claude-sisyphus@5.8.6") || k.logHas(" pack ") {
		t.Errorf("호출\n%s", clitest.Read(k.log))
	}
	if !strings.Contains(clitest.Read(h+"/.claude/CLAUDE.md"), "VERSION:5.8.4") {
		t.Error("CLAUDE.md를 고쳤다")
	}
}

// (b) npm에 없는 판이면 플러그인 폴더를 npm pack --ignore-scripts로 묶어 깔고 임시 폴더를 지운다
func TestPack(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.4", "5.8.6", "brew/bin")
	k.put(h+"/plugin-next", "5.8.6\n", 0o644)
	k.omc(h, "brew", "5.8.4")
	out, rc := k.run(nil)
	want(t, out, rc, 0, "CLI 5.8.4→5.8.6 (묶음)")
	if !k.logHas(" pack --ignore-scripts --pack-destination "+k.root+"/tmp/omc-update.") || !k.logHas("install -g "+k.root+"/tmp/omc-update.") {
		t.Errorf("호출\n%s", clitest.Read(k.log))
	}
	if !clitest.Empty(k.root + "/tmp") {
		t.Error("임시 폴더가 남았다")
	}
}

// (c) 깐 뒤 로그인 셸에서 omc가 안 잡히면 ~/.local/bin에 링크 — 링크 아닌 파일은 덮지 않고 경고(그 기계는 실패)
func TestLink(t *testing.T) {
	k := newKit(t)
	k.home("local", "5.8.6", "5.8.6", ".local/bin")
	k.put(k.root+"/registry", "5.8.6\n", 0o644)
	out, rc := k.run(nil)
	want(t, out, rc, 0, "CLI 없음→5.8.6 (npm+링크)")
	for _, n := range []string{"omc", "oh-my-claudecode", "omc-cli"} {
		if l := clitest.Readlink(k.root + "/local/.local/bin/" + n); l != k.root+"/local/brew/bin/"+n {
			t.Errorf("%s 링크 %q", n, l)
		}
	}

	k = newKit(t)
	h := k.home("local", "5.8.6", "5.8.6", ".local/bin")
	k.put(h+"/.local/bin/omc", "사람 파일\n", 0o644)
	k.put(k.root+"/registry", "5.8.6\n", 0o644)
	out, rc = k.run(nil)
	want(t, out, rc, 1, "경고: "+h+"/.local/bin/omc 는 링크가 아닌 파일 — 덮지 않음", "— 실패: 깐 뒤 로그인 셸의 omc --version이 없음 (바람 5.8.6)")
	if clitest.Read(h+"/.local/bin/omc") != "사람 파일\n" || !clitest.IsLink(h+"/.local/bin/oh-my-claudecode") {
		t.Error("링크 아닌 파일을 덮었거나 다른 이름을 링크하지 않았다")
	}
}

// (d) --all은 연결된 기계 모두 + 이 기계, 하나라도 실패면 종료 1. 연결 안 된 기계는 건너뜀
func TestAll(t *testing.T) {
	k := newKit(t)
	k.put(k.root+"/machines.json", `{"machines":[{"name":"w1","state":"connected"},{"name":"w2","state":"connected"},{"name":"w3","state":"disconnected"}]}`, 0o644)
	for _, m := range []string{"local", "w1", "w2"} {
		k.omc(k.home(m, "5.8.6", "5.8.6", "brew/bin"), "brew", "5.8.6")
	}
	k.put(k.root+"/w2/claude-fail", "", 0o644)
	out, rc := k.run(nil, "--all")
	want(t, out, rc, 1, "이 기계  플러그인 5.8.6  CLI 5.8.6→5.8.6 (같음)", "w1  플러그인 5.8.6  CLI 5.8.6→5.8.6 (같음)",
		"w2  실패: 플러그인 올리기(claude plugin …) 종료 1 claude 안 됨", "w3  건너뜀 (연결 disconnected)")
	if !k.logHas("hc exec --timeout 600 --on w1 -- sh -c") || k.logHas("--on w3") {
		t.Errorf("호출\n%s", clitest.Read(k.log))
	}
	if _, rc := k.run(nil, "--all", "--on", "w1"); rc != 2 {
		t.Errorf("--all --on 종료 %d", rc)
	}
	// 같은 기계를 두 번 적어도 한 번만(감사 낮음 3)
	if err := os.WriteFile(k.log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, rc = k.run(nil, "--on", "w1,w1", "--on", "w1")
	if want(t, out, rc, 0); strings.Count(out, "w1  플러그인") != 1 || strings.Count(clitest.Read(k.log), "\nclaude plugin update oh-my-claudecode@omc\n") != 1 {
		t.Errorf("w1을 두 번 돌렸다\n%s\n%s", out, clitest.Read(k.log))
	}
}

// (f) --check는 플러그인 올리기·install·link를 부르지 않고, CLI가 플러그인 판과 다르면 종료 1
func TestCheck(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.6", "5.8.6", ".local/bin", "brew/bin")
	k.put(h+"/plugin-next", "5.8.9\n", 0o644)
	k.omc(h, "brew", "5.8.4")
	k.put(k.root+"/registry", "5.8.6\n", 0o644)
	out, rc := k.run(nil, "--check")
	want(t, out, rc, 1, "이 기계  플러그인 5.8.6  CLI 5.8.4 (다름)  CLAUDE.md 5.8.6  — 실패: CLI가 플러그인 판과 다르다")
	if k.logHas("claude") || k.logHas("npm") || clitest.Exists(h+"/.local/bin") {
		t.Errorf("--check가 바꿨다\n%s", clitest.Read(k.log))
	}
}

// 지휘 추가 ①: 이미 깔린 omc가 있으면 그 실제 경로의 node 접두사 npm으로 그 접두사에 깐다(기본 npm 접두사에 둘째 omc를 깔지 않는다)
func TestExistingPrefix(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.4", "5.8.6", ".local/bin")
	k.put(h+"/plugin-next", "5.8.6\n", 0o644)
	k.omc(h, "nvm", "5.8.4")
	k.put(h+"/nvm/bin/npm", fmt.Sprintf(npmSh, "$HOME/nvm"), 0o755)
	os.MkdirAll(h+"/.local/bin", 0o755)
	os.Symlink(h+"/nvm/bin/omc", h+"/.local/bin/omc")
	k.put(k.root+"/registry", "5.8.6\n", 0o644)
	out, rc := k.run(nil)
	want(t, out, rc, 0, "CLI 5.8.4→5.8.6 (npm)")
	if !k.logHas("npm["+h+"/nvm/bin/npm] install -g oh-my-claude-sisyphus@5.8.6") || clitest.Exists(h+"/brew") {
		t.Errorf("다른 접두사에 깔았다\n%s", clitest.Read(k.log))
	}
}

// 지휘 추가 ②: PATH에 omc가 둘 이상이면(실제 경로로 중복 제거) 경고 한 줄에 경로·판 — 지우지 않는다
func TestDupWarn(t *testing.T) {
	k := newKit(t)
	h := k.home("local", "5.8.6", "5.8.6", ".local/bin", "brew/bin", "nvm/bin")
	k.omc(h, "brew", "5.8.6")
	k.omc(h, "nvm", "5.8.4")
	os.MkdirAll(h+"/.local/bin", 0o755)
	os.Symlink(h+"/brew/bin/omc", h+"/.local/bin/omc") // brew와 같은 실제 경로 — 하나로 센다
	out, rc := k.run(nil)
	want(t, out, rc, 0, "CLI 5.8.6→5.8.6 (같음)",
		"이 기계  경고: PATH에 omc가 2개 — "+h+"/.local/bin/omc 5.8.6 · "+h+"/nvm/bin/omc 5.8.4 (지우지 않음)\n")
	if !clitest.Exists(h + "/nvm/bin/omc") {
		t.Error("지웠다")
	}
}

// 리눅스는 bash -l, Darwin·Linux가 아닌 기계는 건너뜀(종료에 안 셈)
func TestShells(t *testing.T) {
	k := newKit(t)
	k.omc(k.home("local", "5.8.6", "5.8.6"), "brew", "5.8.6")
	out, rc := k.run([]string{"FAKE_UNAME=Linux"})
	want(t, out, rc, 1, "CLI 없음→") // bash -l은 .zshrc를 읽지 않아 brew/bin이 PATH에 없다
	if !k.logHas("bash -lc") || k.logHas("zsh") {
		t.Errorf("호출\n%s", clitest.Read(k.log))
	}
	out, rc = k.run([]string{"FAKE_UNAME=MINGW64_NT-10.0"})
	want(t, out, rc, 0, "이 기계  건너뜀 (sh를 못 띄움·OS)")
}
