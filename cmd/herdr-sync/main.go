// herdr-sync: 맥의 실제 파일을 정본으로 두 저장소(범용 도구·개인 설정)와 hc에 등록된 기계에 맞춘다 (계약 CONTRACT.md).
// Go 판(2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go). 인자·출력·종료 코드·상태 파일(base.tsv·repo-base.tsv·bak)은 셸 판 bin/herdr-sync와 같다.
// 짜임: 기계마다(맥 포함) 도는 조각(agent.sh — 셸 판 heredoc과 같은 글자, agent_test.go가 본다)은 그대로 sh로 돌린다 — 원격 기계에는
// Go가 없을 수 있고(리눅스 dash), 서명·쓰기·백업이 두 판에서 같은 코드여야 같은 base를 번갈아 읽는다. Go 판은 맥 쪽 지휘(sync.list 풀기,
// 3-way 판정, 저장소 모음·깖, 기계 쓰기 묶음, 설정, 표)를 맡는다. 되돌리기 HERDR_SYNC_IMPL=bash(또는 HERDR_IMPL=bash).
// 잠금은 셸 판 lockf와 같은 파일·같은 flock이라 두 판이 함께 돌지 못한다.
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/proc"
	"herdr-tools/internal/toolcfg"
	"herdr-tools/internal/version"
)

//go:embed agent.sh
var agent string

const usage = `맥의 실제 파일을 정본으로 두 저장소(범용 도구·개인 설정)와 hc에 등록된 기계에 맞춘다 (계약 CONTRACT.md).
무엇을 맞추는지는 개인 저장소의 sync.list와 settings/keys.json이 정한다. 맥에서만 돈다.

  herdr-sync                      저장소 ↔ 맥, 그다음 hc machine ls의 모든 기계에 맞춘다
  herdr-sync <기계>…              고른 기계만
  herdr-sync --check [<기계>…]    고치지 않고 비교만 한다 (다름·충돌이 있으면 종료 1, 첫 줄 「맥:」은 맥 ↔ 저장소 방향)
  herdr-sync --take <기계>:<경로|*>   충돌을 맥 값으로 덮는다 (백업을 남긴다)
  herdr-sync --pull <기계>:<경로|*>   그 기계 값을 맥 실제 파일로 가져온 뒤 맞춘다

경로는 sync.list의 맥 경로(~ 아래, 나무 줄은 그 안 파일 하나)나 설정 키(.model, hook:PreToolUse:cotenant-guard 꼴), *는 그 기계의 충돌 전부, <폴더>/**는 그 나무의 충돌 전부다.
저장소와의 충돌은 기계 자리에 repo를 쓴다: --take repo:<경로>는 맥 값을, --pull repo:<경로>는 저장소 값을 남긴다.
범용 저장소는 HERDR_TOOLS_REPO (기본 ~/Documents/app/herdr-tools), 개인 저장소는 HERDR_TOOLS_CONFIG (기본 ~/.config/herdr-tools).
sh를 못 띄우는 기계(Windows 등)와 uname이 Darwin·Linux가 아닌 기계는 「건너뜀」 한 줄만 찍고 아무것도 쓰지 않는다(--take·--pull도, 종료 코드에 안 넣음).
`

// unit: 맞추는 단위 하나(sync.list 줄 또는 나무 안 파일)
type unit struct {
	b, k, r, d, t string // 묶음 · 맥 경로 · 저장소 경로(링크면 가리킬 곳) · 저장소 폴더 · 대상
	tree          bool   // 나무 파일
	tj            int    // 나무 번호(TR)
	url           string // sync.list 꼴 저장소 경로(tools:<경로> 또는 <경로>)
}

// root: 나무 줄 하나(제외로 빠진 것 빼고)
type root struct{ k, d, r, x string } // 뿌리 · 저장소 폴더 · 저장소 안 뿌리 · 제외 상대 경로 파일

// qres: 한 자리(맥 또는 기계)의 조회 결과
type qres struct {
	C, B, L []string        // 단위마다 현재 서명 · base · 덮은 링크
	RM, Y   map[string]bool // 있는 지울 경로 · 있는 링크 대상
	H, O    string          // $HOME · uname
	nosh    bool            // 그 기계에 sh가 없다(Windows 등)
}

var (
	home, REPO, CONF, STATE, KEYS, TMP, STAMP string
	check                                     bool
	takes, pulls                              []string
	U                                         []unit
	RMK, RMT, EXK                             []string
	TR                                        []root
	PK, LKT                                   []string
	lockf                                     *os.File // 쥔 잠금 — 끝날 때까지
)

func cleanup() {
	if TMP != "" {
		os.RemoveAll(TMP)
	}
}

func exit(code int) {
	cleanup()
	os.Exit(code)
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "herdr-sync: "+f+"\n", a...)
	exit(1)
}

func say(f string, a ...any) { fmt.Printf(f+"\n", a...) }

func warn(f string, a ...any) { fmt.Fprintf(os.Stderr, "herdr-sync: "+f+"\n", a...) }

func main() {
	proc.Who = "herdr-sync"
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("herdr-sync"))
		return
	}
	// 상한을 건 git은 뒤쪽 무리라 단말에서 암호·호스트 키를 물을 수 없다 — 묻지 말고 바로 실패하게(상한까지 멈추지 않게, gofix-ops O-07)
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		os.Setenv("GIT_SSH_COMMAND", "ssh -o BatchMode=yes")
	}
	home = os.Getenv("HOME")
	REPO = envOr("HERDR_TOOLS_REPO", home+"/Documents/app/herdr-tools")
	CONF = envOr("HERDR_TOOLS_CONFIG", home+"/.config/herdr-tools")
	STATE = home + "/.local/state/herdr-sync"

	// 맥 확인: Herdr 앱이 있고 hc가 다른 기계를 아는 곳. 워커 맥의 hc는 다른 기계를 모른다.
	// hc 조회가 실패하거나 시간을 넘기면(모름) 「맥이 아님」과 가른다(gofix-ops B03)
	all, herr := machinesFromHC()
	if herr != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: hc machine ls를 읽지 못했다 (%v) — 모름, 아무것도 쓰지 않음\n", herr)
		os.Exit(1)
	}
	if (!isDir(home+"/Applications/Herdr.app") && !isDir(envOr("HERDR_SYNC_SYSAPPS", "/Applications")+"/Herdr.app")) || len(all) == 0 {
		fmt.Fprintln(os.Stderr, "herdr-sync: 맥에서만 돈다 (~/Applications/Herdr.app 또는 /Applications/Herdr.app이 있고 hc machine ls에 기계가 있는 곳)")
		os.Exit(2)
	}
	if err := os.MkdirAll(STATE, 0o755); err != nil {
		os.Exit(1)
	}
	if os.Getenv("HERDR_SYNC_LOCKED") == "" {
		lockf = takeLock(STATE + "/lock")
		os.Setenv("HERDR_SYNC_LOCKED", "1")
		// 앞 실행이 SIGKILL로 죽으며 남긴 자식 무리(push·commit 등)는 첫 자식을 띄울 때 proc이 거둔다(고아 기록). 거두지 못한 것
		// (모름 — ps를 못 읽음 등)이 남았으면 그 자식과 겹쳐 쓰지 않게 멈춘다(gofix-ops A06)
		if n := orphansLeft(); n > 0 {
			fmt.Fprintf(os.Stderr, "herdr-sync: 앞 실행이 남긴 자식 무리 %d개가 아직 도는지 모른다 (%s) — 다시 돌린다\n", n, proc.RecordDir())
			os.Exit(75)
		}
	}

	var machines []string
	a := os.Args[1:]
	for i := 0; i < len(a); i++ {
		switch {
		case a[i] == "--check":
			check = true
		case a[i] == "--take" || a[i] == "--pull":
			if i+1 >= len(a) || !specShape(a[i+1]) {
				fmt.Fprint(os.Stderr, usage)
				os.Exit(2)
			}
			if a[i] == "--take" {
				takes = append(takes, a[i+1])
			} else {
				pulls = append(pulls, a[i+1])
			}
			i++
		case strings.HasPrefix(a[i], "-"):
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		default:
			machines = append(machines, a[i])
		}
	}
	if check && len(takes)+len(pulls) > 0 {
		fmt.Fprintln(os.Stderr, "herdr-sync: --check는 --take·--pull과 함께 쓰지 않는다")
		os.Exit(2)
	}
	if len(machines) == 0 {
		machines = all
	}

	// 개인 저장소가 없으면 아무것도 쓰지 않고 멈춘다 (범용 저장소 pull도 하지 않는다)
	if !isFile(CONF + "/sync.list") {
		fmt.Fprintf(os.Stderr, "herdr-sync: 개인 저장소가 없다: %s (sync.list·settings/keys.json, 꼴은 범용 저장소 config.example 참고)\n", CONF)
		os.Exit(2)
	}
	onSignals()
	var err error
	if TMP, err = os.MkdirTemp(os.Getenv("TMPDIR"), "herdr-sync."); err != nil {
		os.Exit(1)
	}
	if !check {
		recoverJournal() // 앞 실행이 모으다 죽어 남긴 스테이징(gofix-ops B05)
	}
	ready(REPO)
	ready(CONF)
	// 걷힌 셸 판 줄·틀린 바이너리 줄·딸린 줄(Go 소스·곁 도구) 빠짐이면 맞추기 전에 멈춘다(--check도)
	if toolcfg.SyncLint(REPO, CONF+"/sync.list", os.Stdout, os.Stderr) != 0 {
		exit(2)
	}
	STAMP = time.Now().Format("20060102-150405")
	KEYS = CONF + "/settings/keys.json"
	parseList()
	for _, s := range append(append([]string(nil), takes...), pulls...) {
		if !known(after(s)) {
			die("sync.list에 없는 경로: %s", s)
		}
	}
	doPulls()
	build()
	repoSync()
	if body {
		build()
		repoSync()
	}
	if len(RBH) > 0 {
		RSUF = " (맥 판 기준 — 저장소 새 판은 아직)"
	}
	mq, ok := query("")
	if !ok {
		die("맥 값을 읽지 못했다")
	}
	syncMachines(machines, mq)
	exit(rc)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// specShape: --take·--pull 값 꼴 <기계>:<경로> (:의 앞뒤에 글자)
func specShape(s string) bool {
	i := strings.IndexByte(s, ':')
	return i > 0 && i < len(s)-1
}

func before(s string) string { i := strings.IndexByte(s, ':'); return s[:i] }
func after(s string) string  { i := strings.IndexByte(s, ':'); return s[i+1:] }

// git: 표준 출력(끝 줄바꿈 뺌)과 종료 코드. 표준 오류는 그대로 흘린다
func git(args ...string) (string, int) {
	c := exec.Command("git", args...)
	c.Stderr = os.Stderr
	out, err := proc.Output(c)
	return strings.TrimRight(string(out), "\n"), proc.Code(err)
}

func gitq(args ...string) (string, int) { // 표준 오류를 버린다
	out, err := proc.Output(exec.Command("git", args...))
	return strings.TrimRight(string(out), "\n"), proc.Code(err)
}

// ready <저장소>: git·main·깨끗함 확인 뒤 pull --ff-only. git 조회가 실패하거나 시간을 넘기면(124 — 모름) 깨끗함·main으로 읽지 않고 멈춘다
// (gofix-ops B01·O-01·K-01: 실패한 status의 빈 출력을 깨끗함으로 읽어 커밋 안 된 내용을 맥·기계에 깔았다)
func ready(r string) {
	if _, rc := gitq("-C", r, "rev-parse", "--git-dir"); rc == proc.TimeoutCode {
		die("저장소 상태를 읽지 못했다 (git rev-parse 시간 넘김): %s", r)
	} else if rc != 0 {
		die("저장소가 없다: %s", r)
	}
	if b, rc := gitq("-C", r, "symbolic-ref", "--short", "HEAD"); rc == proc.TimeoutCode {
		die("저장소 상태를 읽지 못했다 (git symbolic-ref 시간 넘김): %s", r)
	} else if b != "main" {
		die("저장소가 main 브랜치가 아니다: %s", r)
	}
	if s, rc := git("-C", r, "status", "--porcelain"); rc != 0 {
		die("저장소 상태를 읽지 못했다 (git status 종료 %d — 모름): %s", rc, r)
	} else if s != "" {
		die("저장소에 커밋하지 않은 변경이 있다. 커밋하거나 되돌린 뒤 다시: %s", r)
	}
	if _, rc := git("-C", r, "pull", "-q", "--ff-only"); rc != 0 {
		die("git pull --ff-only 실패: %s (지난번 push 못 한 커밋이 있으면 거기서 git pull --rebase 뒤 HERDR_LAND=1 git push origin HEAD:main)", r)
	}
}

// ---- sync.list ----

func add(b, k, r, d, t string, tree bool, tj int) {
	u := unit{b: b, k: k, r: r, d: d, t: t, tree: tree, tj: tj, url: r}
	if d == REPO {
		u.url = "tools:" + r
	}
	U = append(U, u)
}

// names: 폴더에서 이름 꼴(sh glob, 한 층)에 맞는 일반 파일 이름. 점으로 시작하는 이름은 꼴이 점으로 시작할 때만
func names(dir, pat string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	var r []string
	for _, p := range m {
		b := filepath.Base(p)
		if strings.HasPrefix(b, ".") && !strings.HasPrefix(pat, ".") {
			continue
		}
		if isFile(p) {
			r = append(r, b)
		}
	}
	return r
}

func parseList() {
	data, err := os.ReadFile(CONF + "/sync.list")
	if err != nil {
		die("sync.list를 읽지 못했다: %s", CONF)
	}
	// 끝 줄바꿈이 없거나 줄 끝에 CR이 있으면 맞추기 전에 멈춘다 — 셸 판은 줄바꿈 없는 마지막 줄을 버리고 CR 줄을 못 맞춘다(Go 판만
	// 그 줄을 따르면 같은 목록으로 판마다 지움이 갈린다, gofix-ops C04·O-10). 칸은 셸 read처럼 빈칸·탭으로만 나눈다
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Fprintf(os.Stderr, "herdr-sync: sync.list 끝 줄바꿈이 없다 — 마지막 줄 뒤에 줄바꿈을 붙인다 (%s/sync.list)\n", CONF)
		exit(2)
	}
	type tl struct{ b, k, r, d, t string }
	var tls []tl
	for n, line := range splitLines(string(data)) {
		if strings.HasSuffix(line, "\r") {
			fmt.Fprintf(os.Stderr, "herdr-sync: sync.list %d번 줄 끝에 CR이 있다 — 줄바꿈은 LF만 (%s/sync.list)\n", n+1, CONF)
			exit(2)
		}
		f := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		for len(f) < 4 {
			f = append(f, "")
		}
		b, k, r, t := f[0], f[1], f[2], f[3]
		if t == "" {
			die("sync.list 줄에 칸이 모자란다: %s %s", b, k)
		}
		// 맥 경로는 ~ 아래, 저장소 경로는 그 저장소 아래여야 한다 — 절대 경로·..로 바깥에 쓰거나 지우지 않게(gofix-ops B11)
		if !inside(strings.TrimSuffix(strings.SplitN(k, "#", 2)[0], "@")) || (r != "-" && !inside(strings.TrimPrefix(r, "tools:"))) {
			fmt.Fprintf(os.Stderr, "herdr-sync: sync.list %d번 줄 경로가 뿌리 밖이다(절대 경로·..): %s %s\n", n+1, k, r)
			exit(2)
		}
		for _, o := range strings.Split(t, ",") {
			w := map[string]string{"all9": "all", "mac7": "macos", "mac-only": "local"}[o]
			if w != "" {
				fmt.Fprintf(os.Stderr, "herdr-sync: 옛 묶음 이름 %s — %s로 바꾼다 (%s/sync.list: %s %s)\n", o, w, CONF, b, k)
				exit(2)
			}
		}
		if b == "지움" {
			RMK, RMT = append(RMK, k), append(RMT, t)
			continue
		}
		if b == "제외" {
			EXK = append(EXK, strings.TrimSuffix(k, "/"))
			continue
		}
		// 바이너리 줄(tools:cmd/<도구>): 파일로 맞추지 않고 그 도구의 Go 바이너리를 ~/.local/bin/<도구>에 깐다(release.go)
		if x := toolcfg.BinLine(k, r); x != "" {
			if k != ".local/bin/"+x || !isDir(REPO+"/cmd/"+x) {
				fmt.Fprintf(os.Stderr, "herdr-sync: sync.list %d번 줄 바이너리 줄은 「.local/bin/%s tools:cmd/%s」 꼴이고 범용 저장소에 cmd/%s가 있어야 한다: %s %s\n", n+1, x, x, x, k, r)
				exit(2)
			}
			BINS = append(BINS, binUnit{x, t})
			continue
		}
		d := CONF
		if strings.HasPrefix(r, "tools:") {
			d, r = REPO, r[6:]
		}
		switch {
		case strings.HasSuffix(k, "/**"):
			tls = append(tls, tl{b, strings.TrimSuffix(k, "/**"), strings.TrimSuffix(r, "/"), d, t})
		case strings.Contains(k, "*"):
			kd, kb := k, k
			if i := strings.LastIndexByte(k, '/'); i >= 0 {
				kd, kb = k[:i], k[i+1:]
			}
			set := map[string]bool{}
			for _, n := range append(names(home+"/"+kd, kb), names(d+"/"+r, kb)...) {
				set[n] = true
			}
			var fs []string
			for n := range set {
				fs = append(fs, n)
			}
			sort.Strings(fs)
			for _, n := range fs {
				add(b, kd+"/"+n, strings.TrimSuffix(r, "/")+"/"+n, d, t, false, -1)
			}
		default:
			add(b, k, r, d, t, false, -1)
		}
	}
	// 나무 줄 → 맥과 저장소에 있는 파일 하나하나를 단위로. 따로 적은 줄·앞 나무 줄이 이미 맡은 경로는 건너뛴다.
	// 맥이나 저장소 쪽이 7MB 넘으면 단위로 삼지 않는다. 뿌리가 제외 아래면 나무 전체를 뺀다
	seen := map[string]bool{}
	for _, u := range U {
		seen[u.k] = true
	}
	for _, l := range tls {
		out := false
		for _, e := range EXK {
			if l.k == e || strings.HasPrefix(l.k, e+"/") {
				out = true
			}
		}
		if out {
			continue
		}
		jt := len(TR)
		var ex []string
		for _, e := range EXK {
			if strings.HasPrefix(e, l.k+"/") {
				ex = append(ex, e[len(l.k)+1:])
			}
		}
		x := writeLines(fmt.Sprintf("ex%d", jt), ex)
		TR = append(TR, root{l.k, l.d, l.r, x})
		var tl bytes.Buffer
		c1 := macCmd("", "t", home+"/"+l.k, l.k, x)
		c1.Stdout = &tl
		if run(c1) != 0 {
			die("나무를 읽지 못했다: %s", l.k)
		}
		c2 := macCmd("", "t", l.d+"/"+l.r, l.k, x)
		c2.Stdout = &tl
		if run(c2) != 0 {
			die("나무를 읽지 못했다: %s", l.k)
		}
		big, shown, got := map[string]bool{}, map[string]bool{}, map[string]bool{}
		var cand []string
		for _, line := range splitLines(tl.String()) {
			f := strings.Split(line, "\t")
			if len(f) < 2 {
				continue
			}
			switch f[0] {
			case "G":
				big[f[1]] = true
				if !shown[f[1]] {
					shown[f[1]] = true
					say("건너뜀 %s (7MB 넘음)", f[1])
				}
			case "F":
				if !seen[f[1]] && !got[f[1]] {
					got[f[1]] = true
					cand = append(cand, f[1])
				}
			}
		}
		var tk []string
		for _, k := range cand {
			if !big[k] {
				tk = append(tk, k)
			}
		}
		sort.Strings(tk)
		// 저장소 .gitignore에 걸리는 것은 맥·저장소·기계 모두에서 뺀다 (모을 수 없으니 한쪽만 「없음」으로 보이지 않게)
		if len(tk) > 0 {
			var in bytes.Buffer
			for _, k := range tk {
				in.WriteString(l.r + "/" + k[len(l.k)+1:] + "\x00")
			}
			c := exec.Command("git", "-C", l.d, "check-ignore", "-z", "--stdin")
			c.Stdin, c.Stderr = &in, os.Stderr
			out, err := proc.Output(c)
			if rc := proc.Code(err); rc != 0 && rc != 1 {
				die("git check-ignore 실패: %s", l.d)
			}
			ig := map[string]bool{}
			for _, p := range strings.Split(string(out), "\x00") {
				ig[p] = true
			}
			var keep []string
			for _, k := range tk {
				if ig[l.r+"/"+k[len(l.k)+1:]] {
					say("건너뜀 %s (저장소 .gitignore)", k)
				} else {
					keep = append(keep, k)
				}
			}
			tk = keep
		}
		for _, k := range tk {
			add(l.b, k, l.r+"/"+k[len(l.k)+1:], l.d, l.t, true, jt)
			seen[k] = true
		}
	}
	if len(U) == 0 && len(TR) == 0 {
		die("sync.list에 맞출 것이 없다")
	}
	for _, u := range U {
		if u.tree {
			continue
		}
		PK = append(PK, u.k)
		if strings.HasSuffix(u.k, "@") {
			LKT = append(LKT, u.r)
		}
	}
}

// inside: 상대 경로이고 ..·빈 칸으로 뿌리를 벗어나지 않는가(끝 /·/**·* 꼴은 그대로 받는다)
func inside(p string) bool {
	p = strings.TrimSuffix(strings.TrimSuffix(p, "/**"), "/")
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return false
	}
	for _, s := range strings.Split(p, "/") {
		if s == ".." || s == "." || s == "" {
			return false
		}
	}
	return true
}

func unitKey(k string) bool {
	for _, u := range U {
		if u.k == k {
			return true
		}
	}
	return false
}

// treespec: <뿌리>/**
func treespec(k string) bool {
	if !strings.HasSuffix(k, "/**") {
		return false
	}
	r := strings.TrimSuffix(k, "/**")
	for _, t := range TR {
		if t.k == r {
			return true
		}
	}
	return false
}

func setkey(k string) bool {
	if strings.HasPrefix(k, "hook:") {
		if p := strings.SplitN(k[5:], ":", 2); len(p) == 2 && p[0] != "" && p[1] != "" {
			return true
		}
	}
	kv, err := readJSONFile(KEYS)
	if err != nil {
		return false
	}
	for _, x := range strList(objGet(kv, "values")) {
		if x == k {
			return true
		}
	}
	return false
}

func known(k string) bool { return k == "*" || unitKey(k) || treespec(k) || setkey(k) }

// spec: 목록에 <기계>:<키>, <기계>:*, 또는 그 키를 담은 <기계>:<뿌리>/**가 있나
func spec(list []string, m, k string) bool {
	for _, s := range list {
		if s == m+":"+k || s == m+":*" {
			return true
		}
		if strings.HasPrefix(s, m+":") && strings.HasSuffix(s, "/**") {
			r := strings.TrimSuffix(s[len(m)+1:], "/**")
			if strings.HasPrefix(k, r+"/") {
				return true
			}
		}
	}
	return false
}

// targets: 대상이 그 기계를 덮나
func targets(t, m, osname string) bool {
	switch t {
	case "all":
		return true
	case "macos":
		return osname == "Darwin"
	case "local":
		return m == "맥"
	}
	for _, x := range strings.Split(t, ",") {
		if x == m {
			return true
		}
	}
	return false
}
