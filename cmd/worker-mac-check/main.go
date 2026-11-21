// worker-mac-check: 워커 맥 하나를 맥에서 SSH·hc로 점검한다(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」). 읽기 전용.
// 계약(인자·줄 꼴·개수 줄·종료 코드·설정 키)은 셸 판 lib/worker-mac-check/worker-mac-check.sh와 같다 — 머리말 설명도 그쪽.
// 원격(SSH 문맥) 점검은 remote.sh를 bash로 보낸다(원격에는 Go가 없다). 받은 줄 파싱·맥과 판 비교·hc 문맥 점검·상한 시간은 여기.
// 비밀은 찍지 않는다: 키체인 값·토큰·전체 argv를 읽거나 출력하지 않는다(gh는 계정 이름, 프록시는 상태 줄만).
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"herdr-tools/internal/version"
	wm "herdr-tools/internal/workermac"
)

//go:embed remote.sh
var remote string

// toolPath: hc 터미널 점검에 덮는 PATH($HOME은 그 기계에서 펼친다)
const toolPath = `$HOME/.local/bin:$HOME/go/bin:$HOME/.bun/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin`

// 상한 초: SSH 연결 확인·원격 점검(시험이 줄인다)
var connectSec, remoteSec = 15, 90

type settings struct {
	repos, trust, slots, tools, px string
	brew                           string // WORKER_MAC_BREW_DIR(시험이 바꾼다), 없으면 /opt/homebrew/bin
	miss                           []string
}

func main() {
	wm.Who = "worker-mac-check"
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" { // 공통 실행기가 바이너리를 쓰기 전에 본다
		fmt.Println(version.String("worker-mac-check")) // 릴리스 커밋이 보이게(B25)
		return
	}
	if len(args) < 1 || dashName(args) {
		fmt.Fprintln(os.Stderr, "사용: worker-mac-check <기계> [<기계>...]")
		os.Exit(64)
	}
	ctx := wm.Signals()
	s, err := load(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	rc := 0
	for _, m := range args {
		if r := checkOne(ctx, os.Stdout, s, m); r > rc {
			rc = r
		}
		fmt.Println()
	}
	os.Exit(rc)
}

// dashName: -로 시작하는 기계 이름이 있는가 — ssh·hc가 옵션으로 읽는다(E10, 셸 판도 64)
func dashName(ms []string) bool {
	for _, m := range ms {
		if strings.HasPrefix(m, "-") {
			return true
		}
	}
	return false
}

// load: 설정. 키가 없으면 기본값, 기본값도 없으면 빈 값 + miss(그 항목은 「모름 … 설정 없음」). 설정 오류는 err(종료 2)
func load(ctx context.Context) (settings, error) {
	c := &wm.Config{Ctx: ctx}
	s := settings{
		repos: c.Get("worker.repo"), trust: c.Get("worker.trust", "Documents/app"), slots: c.Get("worker.slots_file"),
		tools: c.Get("worker.tools", "git node"), px: c.Get("worker.proxy"), brew: os.Getenv("WORKER_MAC_BREW_DIR"),
	}
	if s.brew == "" {
		s.brew = "/opt/homebrew/bin"
	}
	s.miss = c.Miss
	return s, c.Err
}

// pre: 원격 스크립트 앞에 붙는 설정 값(셸 안전 인용)
func (s settings) pre() string {
	return "REPOS=" + wm.Quote(s.repos) + " TRUST=" + wm.Quote(s.trust) + " SLOTS_FILE=" + wm.Quote(s.slots) + " TOOLS=" + wm.Quote(s.tools)
}

type checker struct {
	w       io.Writer
	p, f, u int
}

func (c *checker) line(st, item, detail string) {
	fmt.Fprintf(c.w, "%s\t%s\t%s\n", st, item, detail)
	switch st {
	case "통과":
		c.p++
	case "실패":
		c.f++
	default:
		c.u++
	}
}

func (c *checker) footer(m string) {
	fmt.Fprintf(c.w, "-- %s: 통과 %d · 실패 %d · 모름 %d\n", m, c.p, c.f, c.u)
}

var missItem = map[string]string{"worker.repo": "저장소", "worker.slots_file": "검증 자리 파일", "worker.proxy": "프록시 status"}

// checkOne: 기계 하나. 0 실패 없음 · 1 실패 있음 · 2 SSH 연결 안 됨
func checkOne(ctx context.Context, w io.Writer, s settings, m string) int {
	c := &checker{w: w}
	fmt.Fprintf(w, "== %s (%s)\n", m, time.Now().Format("2006-01-02 15:04:05"))
	if _, rc := wm.Output(ctx, connectSec, nil, "ssh", "-n", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", "--", m, "true"); rc != 0 {
		c.line("실패", "SSH 연결", "ssh "+m+" 안 됨 (Tailscale·원격 로그인·키 확인)")
		c.footer(m)
		return 2
	}
	c.line("통과", "SSH 연결", "키 로그인")
	for _, k := range s.miss {
		i := missItem[k]
		if i == "" {
			i = k
		}
		c.line("모름", i, "설정 없음("+k+") — "+wm.ConfigDir()+"/config, 예: config.example/config")
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(s.pre() + "\n" + remote))
	out, rc := wm.Output(ctx, remoteSec, nil, "ssh", "-n", "-o", "BatchMode=yes", "--", m, `bash -c "$(echo `+b64+` | base64 -D)"`)
	c.remoteLines(ctx, out)
	if why := cut(out, rc, remoteSec); why != "" { // 받은 줄은 두고, 다 받지 못한 점검은 통과로 세지 않는다(E01·B27)
		c.line("실패", "원격 점검", why+" — 받은 줄까지만 봤다")
	}
	c.hcChecks(ctx, s, m)
	c.footer(m)
	if c.f == 0 {
		return 0
	}
	return 1
}

// endMark: remote.sh가 끝까지 돌았을 때 찍는 마지막 줄
const endMark = "E\t끝"

// cut: 원격 점검이 끝까지 오지 않았으면 까닭(시간 초과·ssh 종료 코드·끝 표식 없음), 다 왔으면 빈 값. 셸 판 check_one과 같은 말
func cut(out string, rc, sec int) string {
	switch {
	case rc == wm.TimeoutCode:
		return fmt.Sprintf("시간 초과(%d초)", sec)
	case rc != 0:
		return fmt.Sprintf("끊김(종료 %d)", rc)
	case !strings.Contains("\n"+out, "\n"+endMark+"\n"):
		return "끝 표식 없음"
	}
	return ""
}

// remoteLines: 원격 줄(R 상태 항목 세부 / V 도구 판). 판은 맥과 견준다. 시간이 지나 끊긴 출력도 받은 줄까지는 본다
func (c *checker) remoteLines(ctx context.Context, out string) {
	for _, l := range strings.Split(out, "\n") {
		f := readTab(l, 4)
		switch f[0] {
		case "R":
			c.line(f[1], f[2], f[3])
		case "V":
			if m := macVer(ctx, f[1]); f[2] == m {
				c.line("통과", "판 "+f[1], f[2]+" (맥과 같음)")
			} else {
				c.line("실패", "판 "+f[1], f[2]+" (맥 "+or(m, "없음")+")")
			}
		}
	}
}

// readTab: bash 「IFS=$'\t' read -r」과 같이 n칸으로 — 탭이 이어지면 한 칸으로, 앞뒤 탭은 버리고, 마지막 칸이 나머지를 다 갖는다
func readTab(line string, n int) []string {
	f := make([]string, n)
	s := strings.Trim(line, "\t")
	for i := 0; i < n-1 && s != ""; i++ {
		j := strings.IndexByte(s, '\t')
		if j < 0 {
			f[i], s = s, ""
			break
		}
		f[i], s = s[:j], strings.TrimLeft(s[j:], "\t")
	}
	if s != "" {
		f[n-1] = s
	}
	return f
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// macVer: 이 맥의 판 — 원격 ver와 같은 꼴(go·git은 셋째 낱말, wails3는 표준 오류까지 첫 줄, 그 밖은 --version 첫 줄). 없으면 빈 값
func macVer(ctx context.Context, tool string) string {
	home := os.Getenv("HOME")
	path := home + "/.local/bin:" + home + "/go/bin:" + home + "/.bun/bin:/opt/homebrew/bin:" + os.Getenv("PATH")
	bin := wm.LookIn(path, tool)
	if bin == "" {
		return ""
	}
	args := []string{"--version"}
	if tool == "go" || tool == "wails3" {
		args = []string{"version"}
	}
	var b strings.Builder
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second) // 셸 판에는 없던 상한(맥 쪽 --version이 멈춰도 점검이 끝나게)
	defer cancel()
	cmd := wm.Command(tctx, bin, args...)
	cmd.Env = append(os.Environ(), "PATH="+path)
	cmd.Stdout = &b
	if tool == "wails3" {
		cmd.Stderr = &b
	}
	wm.Run(tctx, cmd) // 셸 판처럼 종료 코드가 아니라 출력으로 본다(없으면 「없음」)
	out := b.String()
	if tool == "go" || tool == "git" {
		var r []string
		for _, l := range strings.Split(wm.Trim(out), "\n") {
			fs := strings.Fields(l)
			if len(fs) >= 3 {
				r = append(r, fs[2])
			} else {
				r = append(r, "")
			}
		}
		return wm.Trim(strings.Join(r, "\n"))
	}
	first, _, _ := strings.Cut(out, "\n")
	return first
}

// hcChecks: hc 터미널 문맥(코어가 띄운 프로그램) 점검
func (c *checker) hcChecks(ctx context.Context, s settings, m string) {
	items := []string{"키체인(hc 문맥)", "brew PATH(hc 문맥)", "문서 폴더(hc 문맥)", "gh 로그인"}
	if s.px != "" {
		items = append(items, s.px+" status")
	}
	ls, rc := wm.Output(ctx, 20, nil, "hc", "machine", "ls")
	if rc != 0 || !connectedRe(m).MatchString(ls) {
		for _, i := range items {
			c.line("모름", i, "hc 기계 "+m+" 연결 안 됨")
		}
		return
	}
	hcx := func(sec int, script string) string {
		out, _ := wm.Output(ctx, sec, nil, "hc", "exec", "--on", m, "--", "/bin/sh", "-c", `export PATH="`+toolPath+`"; `+script)
		return out
	}
	switch kc := wm.Trim(hcx(20, `security show-keychain-info "$HOME/Library/Keychains/login.keychain-db" >/dev/null 2>&1; echo $?`)); kc {
	case "0":
		c.line("통과", "키체인(hc 문맥)", "열림")
	case "36":
		c.line("실패", "키체인(hc 문맥)", "User interaction is not allowed (코어가 SSH 문맥)")
	default:
		c.line("모름", "키체인(hc 문맥)", "rc "+or(kc, "시간 초과"))
	}
	// hcx는 PATH를 덮으니 여기는 코어가 준 PATH 그대로 본다
	hp, _ := wm.Output(ctx, 15, nil, "hc", "exec", "--on", m, "--", "/bin/sh", "-c", "[ -d "+wm.Quote(s.brew)+` ] || { echo -; exit 0; }; echo "$PATH"`)
	switch p := ":" + wm.Trim(hp) + ":"; {
	case p == ":-:":
		c.line("통과", "brew PATH(hc 문맥)", "해당 없음: "+s.brew+" 없음")
	case p == "::":
		c.line("모름", "brew PATH(hc 문맥)", "15초 안 응답")
	case strings.Contains(p, ":"+s.brew+":"):
		c.line("통과", "brew PATH(hc 문맥)", s.brew+" 있음")
	default:
		c.line("실패", "brew PATH(hc 문맥)", s.brew+" 없음 — 코어가 로그인 환경 없이 떴다 (hc-core-relaunch "+m+")")
	}
	switch wm.Trim(hcx(15, `ls "$HOME/Documents" >/dev/null 2>&1 && echo ok || echo no`)) {
	case "ok":
		c.line("통과", "문서 폴더(hc 문맥)", "~/Documents 읽힘")
	case "no":
		c.line("실패", "문서 폴더(hc 문맥)", "읽기 거부 (TCC 파일 및 폴더 → herdr-core → 문서 폴더)")
	default:
		c.line("실패", "문서 폴더(hc 문맥)", "15초 안 응답 — 그 맥 화면에 폴더 접근 질문이 떠 있을 수 있다")
	}
	gh := wm.Trim(hcx(20, `gh auth status --hostname github.com 2>&1 | grep -m1 -E "Logged in to github.com|not logged|token .* invalid"`))
	if acct := ghAccount(gh); acct != "" {
		c.line("통과", "gh 로그인", acct)
	} else if gh != "" {
		c.line("실패", "gh 로그인", "로그인 안 됨")
	} else {
		c.line("실패", "gh 로그인", "gh 없음 또는 응답 없음")
	}
	if s.px != "" {
		tc := squeeze(hcx(30, wm.Quote(s.px)+` status 2>&1 | grep -m2 -i -E "running|stopped|not running|usable now"`))
		switch {
		case usableRe.MatchString(tc):
			c.line("통과", s.px+" status", tc)
		case tc != "":
			c.line("실패", s.px+" status", tc)
		default:
			c.line("실패", s.px+" status", "응답 없음 (프록시·계정 없음?)")
		}
	}
}

var (
	usableRe  = regexp.MustCompile(`(?i)usable now: *[1-9]`)
	accountRe = regexp.MustCompile(`.*account ([^ ]+)`)
	spacesRe  = regexp.MustCompile(` +`)
)

// connectedRe: hc machine ls 한 줄에 그 기계가 connected로
func connectedRe(m string) *regexp.Regexp {
	return regexp.MustCompile(`"name":"` + regexp.QuoteMeta(m) + `",[^}\n]*"state":"connected"`)
}

// ghAccount: sed -nE 's/.*account ([^ ]+).*/\1/p'
func ghAccount(s string) string {
	var r []string
	for _, l := range strings.Split(s, "\n") {
		if x := accountRe.FindStringSubmatch(l); x != nil {
			r = append(r, x[1])
		}
	}
	return strings.Join(r, "\n")
}

// squeeze: tr '\n' ' ' | tr -s ' '
func squeeze(s string) string {
	return spacesRe.ReplaceAllString(strings.ReplaceAll(s, "\n", " "), " ")
}
