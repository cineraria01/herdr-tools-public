// omc-update: OMC(oh-my-claudecode) 플러그인과 omc CLI(npm oh-my-claude-sisyphus)를 기계마다 같은 판으로 맞춘다(2026-11-23 사용자
// 「Omc Update 안되는것도 수정 처리 하고 모든 서버 omc 도 설치하자」). `omc update`는 GitHub releases/latest·npm latest만 봐서 git 태그로만
// 낸 판을 못 올린다 — 여기는 플러그인을 claude 명령으로 올리고, 그 판(installed_plugins.json)을 목표로 CLI를 맞춘다.
//
//	omc-update                 이 기계
//	omc-update --all           hc machine ls의 연결된 기계 모두 + 이 기계(동시에)
//	omc-update --on a,b        그 기계들
//	--check                    아무것도 바꾸지 않고 판만(CLI가 플러그인 판과 다르면 종료 1)
//
// 기계마다 remote.sh를 그 기계의 로그인 셸 환경(macOS zsh + ~/.zshrc, 리눅스 bash -l)에서 돌린다 — 원격은 hc exec(600초 상한),
// 대상 기계에 이 바이너리가 없어도 된다. omc update·setup·update-reconcile은 부르지 않는다(사람 손질을 덮는다). 종료: 0 · 1 실패 있음 · 2 잘못 부름
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/version"
)

//go:embed remote.sh
var remote string

// wrap: 로그인 셸 환경을 읽은 뒤 POSIX sh로 조각을 돈다(zsh 말투로 돌리지 않게 — PATH만 물려받는다). Darwin·Linux가 아니면 86(건너뜀).
// zsh 갈래는 조각을 셸 변수가 아니라 위치 인자로 넘긴다 — ~/.zshrc가 같은 이름 변수를 써도 덮이지 않게(감사 낮음 1). 안쪽 sh의 $0은 조각 글(쓰지 않음)
const wrap = `s=$1; shift
case $(uname 2>/dev/null) in Darwin|Linux) ;; *) echo "omc-update: 건너뜀 OS $(uname 2>/dev/null)" >&2; exit 86 ;; esac
if [ "$(uname)" = Darwin ] && command -v zsh >/dev/null 2>&1; then
  exec zsh -lc '[ -f "$HOME/.zshrc" ] && . "$HOME/.zshrc" >/dev/null 2>&1 </dev/null; exec sh -c "$1" "$@"' zsh "$s" "$@"
fi
command -v bash >/dev/null 2>&1 && exec bash -lc 'exec sh -c "$0" sh "$@"' "$s" "$@"
exec sh -c "$s" sh "$@"`

const (
	skipCode = 86
	local    = "이 기계"
	usage    = "사용: omc-update [--check] [--all | --on <기계>[,<기계>…]]"
)

var (
	verRe = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?`)
	check bool
)

func main() {
	proc.Who = "omc-update"
	var all bool
	var on []string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--version":
			fmt.Println(version.String("omc-update"))
			return
		case a == "-h" || a == "--help":
			fmt.Println(usage)
			return
		case a == "--check":
			check = true
		case a == "--all":
			all = true
		case a == "--on" && i+1 < len(args):
			i++
			on = append(on, strings.Split(args[i], ",")...)
		case strings.HasPrefix(a, "--on="):
			on = append(on, strings.Split(a[5:], ",")...)
		default:
			bad(a)
		}
	}
	for _, m := range on {
		if m == "" || strings.HasPrefix(m, "-") {
			bad(m)
		}
	}
	slices.Sort(on)
	on = slices.Compact(on) // 같은 기계를 두 번 동시에 돌리지 않게(감사 낮음 3)
	if all && len(on) > 0 {
		bad("--all과 --on은 함께 못 쓴다")
	}
	targets := on
	var skipped []string
	if all {
		var err error
		if targets, skipped, err = connected(); err != nil {
			fmt.Fprintln(os.Stderr, "omc-update: hc machine ls를 읽지 못했다 —", err)
			os.Exit(1)
		}
		targets = append([]string{""}, targets...)
	} else if len(on) == 0 {
		targets = []string{""}
	}
	res := make([]result, len(targets))
	var wg sync.WaitGroup
	for i, m := range targets {
		wg.Add(1)
		go func() { defer wg.Done(); res[i] = machine(m) }()
	}
	wg.Wait()
	rc := 0
	for _, r := range res {
		fmt.Print(r.out)
		if r.fail {
			rc = 1
		}
	}
	for _, s := range skipped {
		fmt.Println(s)
	}
	os.Exit(rc)
}

func bad(a string) {
	fmt.Fprintf(os.Stderr, "omc-update: 잘못 부름 (%s)\n%s\n", a, usage)
	os.Exit(2)
}

// connected: hc machine ls의 연결된 기계와, 연결 안 된 기계의 「건너뜀」 줄
func connected() ([]string, []string, error) {
	out, err := proc.Output(exec.Command("hc", "machine", "ls"))
	if err != nil {
		return nil, nil, err
	}
	ms, err := hcjson.ParseMachines(out)
	if err != nil {
		return nil, nil, err
	}
	var on, skip []string
	for _, m := range ms {
		if m.State == "connected" {
			on = append(on, m.Name)
		} else {
			skip = append(skip, fmt.Sprintf("%s  건너뜀 (연결 %s)", m.Name, m.State))
		}
	}
	return on, skip, nil
}

// run: 그 기계에서 remote.sh <일> <인자…> — 표준 출력, 표준 오류 마지막 줄, 종료 코드(못 띄운 기계는 skipCode)
func run(m string, args ...string) (string, string, int) {
	argv := append([]string{"sh", "-c", wrap, "omc-update", remote}, args...)
	if m != "" {
		argv = append([]string{"hc", "exec", "--timeout", "600", "--on", m, "--"}, argv...)
	}
	c := exec.Command(argv[0], argv[1:]...)
	var eb bytes.Buffer
	c.Stderr = &eb
	out, err := proc.OutputMax(c, 12*time.Minute)
	e := strings.TrimSpace(eb.String())
	if i := strings.LastIndexByte(e, '\n'); i >= 0 {
		e = e[i+1:]
	}
	rc := proc.Code(err)
	if m != "" && rc != 0 && (strings.Contains(eb.String(), "spawn_failed") || strings.Contains(eb.String(), "cannot start ")) {
		rc = skipCode // herdr-sync와 같다: sh를 못 띄우는 기계(Windows 등)
	}
	return string(out), e, rc
}

type probeOut struct {
	cli, prefix, md string
	all             []string // 「경로 판」
	plugin, path    string   // 플러그인 판·installPath
}

func probe(m string) (probeOut, string, int) {
	out, e, rc := run(m, "probe")
	var p probeOut
	if rc != 0 {
		return p, e, rc
	}
	head, js, _ := strings.Cut(out, "\nJ\n")
	for _, l := range strings.Split(head, "\n") {
		f := strings.Split(l, "\t")
		switch {
		case f[0] == "C" && len(f) == 3:
			p.cli, p.prefix = verRe.FindString(f[1]), f[2]
		case f[0] == "A" && len(f) == 3:
			p.all = append(p.all, f[1]+" "+or(verRe.FindString(f[2]), "판 모름"))
		case f[0] == "M" && len(f) == 2:
			p.md = verRe.FindString(f[1])
		}
	}
	var v struct {
		Plugins map[string][]struct{ Version, InstallPath string } `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(js), &v); err == nil && len(v.Plugins["oh-my-claudecode@omc"]) > 0 {
		p.plugin, p.path = v.Plugins["oh-my-claudecode@omc"][0].Version, v.Plugins["oh-my-claudecode@omc"][0].InstallPath
	}
	return p, "", 0
}

type result struct {
	out  string
	fail bool
}

// machine: 한 기계를 맞추고(또는 --check로 보고) 출력 줄을 돌려준다
func machine(m string) result {
	name := or(m, local)
	var b strings.Builder
	fail := func(why string) result {
		fmt.Fprintf(&b, "%s  실패: %s\n", name, why)
		return result{b.String(), true}
	}
	if !check {
		if _, e, rc := run(m, "plugin"); rc == skipCode {
			return result{name + "  건너뜀 (sh를 못 띄움·OS)\n", false}
		} else if rc != 0 {
			return fail(fmt.Sprintf("플러그인 올리기(claude plugin …) 종료 %d %s", rc, e))
		}
	}
	p, e, rc := probe(m)
	if rc == skipCode {
		return result{name + "  건너뜀 (sh를 못 띄움·OS)\n", false}
	} else if rc != 0 {
		return fail(fmt.Sprintf("판 읽기 종료 %d %s", rc, e))
	}
	target := p.plugin
	if verRe.FindString(target) != target || target == "" {
		return fail("installed_plugins.json에 oh-my-claudecode@omc 판이 없다(" + strconv.Quote(target) + ")")
	}
	before, how, why := p.cli, "같음", ""
	switch {
	case before == target:
	case check:
		how, why = "다름", "CLI가 플러그인 판과 다르다"
	default:
		out, e, rc := run(m, "install", target, p.prefix, p.path)
		if rc != 0 {
			return fail(fmt.Sprintf("CLI 깔기 종료 %d %s", rc, e))
		}
		var warns []string
		for _, l := range strings.Split(out, "\n") {
			k, v, _ := strings.Cut(l, "\t")
			switch k {
			case "H":
				how = v
			case "L":
				if !strings.HasSuffix(how, "+링크") {
					how += "+링크"
				}
			case "W":
				warns = append(warns, v)
			}
		}
		if p, e, rc = probe(m); rc != 0 {
			return fail(fmt.Sprintf("깐 뒤 판 읽기 종료 %d %s", rc, e))
		}
		for _, w := range warns {
			fmt.Fprintf(&b, "%s  경고: %s\n", name, w)
		}
		if p.cli != target {
			why = "깐 뒤 로그인 셸의 omc --version이 " + or(p.cli, "없음") + " (바람 " + target + ")"
		}
	}
	line := fmt.Sprintf("%s  플러그인 %s  CLI %s→%s (%s)  CLAUDE.md %s", name, target, or(before, "없음"), or(p.cli, "없음"), how, or(p.md, "없음"))
	if check {
		line = fmt.Sprintf("%s  플러그인 %s  CLI %s (%s)  CLAUDE.md %s", name, target, or(before, "없음"), how, or(p.md, "없음"))
	}
	if why != "" {
		line += "  — 실패: " + why
	}
	b.WriteString(line + "\n")
	if len(p.all) > 1 {
		fmt.Fprintf(&b, "%s  경고: PATH에 omc가 %d개 — %s (지우지 않음)\n", name, len(p.all), strings.Join(p.all, " · "))
	}
	if p.md != target {
		fmt.Fprintf(&b, "%s  경고: ~/.claude/CLAUDE.md OMC:VERSION %s ≠ 플러그인 %s (고치지 않음)\n", name, or(p.md, "없음"), target)
	}
	return result{b.String(), why != ""}
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
