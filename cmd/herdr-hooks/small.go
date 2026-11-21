package main

// 작은 훅 넷: hc-lane-wait-guard · kill-by-lookup-guard · lane-no-shell-edit · loop-no-question.
// 각 셸 판(claude/hooks/<훅>.sh)의 판정 그대로 — 주석의 정규식은 셸 판의 원문이다.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"herdr-tools/internal/proc"
)

// ---------- hc-lane-wait-guard ----------

const waitMsg = "hc-lane wait는 분리(&) 실행하지 말고 Bash run_in_background로 돌려 알림을 받으세요(또는 포그라운드로). 분리하면 대기 결과를 아무도 못 봅니다."

func hcLaneWaitGuard(in []byte, _, stderr io.Writer) int {
	// [ "$(jq -r '.tool_input.run_in_background // false')" = true ] && exit 0
	vals, _ := jqValues(in)
	var bg strings.Builder
	for _, v := range vals {
		x, ok := jqPath(v, "tool_input", "run_in_background")
		if !ok {
			break
		}
		if !jqTruthy(x) {
			x = false
		}
		bg.WriteString(jqRaw(x) + "\n")
	}
	if capture(bg.String()) == "true" {
		return 0
	}
	cmd, ok := jqCommand(in)
	if !ok {
		return 0
	}
	// grep -Eq 'hc-lane[[:space:]]+wait' (줄마다)
	if !grepLines(cmd, waitWord) {
		return 0
	}
	// sed -E 's/&&//g; s/[<>|]&//g; s/&>//g' 뒤 홀로 선 & · 또는 nohup|disown|setsid 낱말
	amp := strings.ReplaceAll(cmd, "&&", "")
	amp = ampRedir.ReplaceAllString(amp, "")
	amp = strings.ReplaceAll(amp, "&>", "")
	if strings.Contains(amp, "&") || grepLines(cmd, detachWord) {
		fmt.Fprintln(stderr, waitMsg)
		return 2
	}
	return 0
}

var ampRedir = regexp.MustCompile(`[<>|]&`)

// grepLines: grep -E처럼 줄마다 f로 본다
func grepLines(s string, f func(string) bool) bool {
	for _, l := range strings.Split(s, "\n") {
		if f(l) {
			return true
		}
	}
	return false
}

// waitWord: hc-lane[[:space:]]+wait
func waitWord(l string) bool {
	for i := 0; ; {
		j := strings.Index(l[i:], "hc-lane")
		if j < 0 {
			return false
		}
		k := i + j + len("hc-lane")
		n := k
		for n < len(l) {
			r, w := utf8.DecodeRuneInString(l[n:])
			if !grepSpace(r) {
				break
			}
			n += w
		}
		if n > k && strings.HasPrefix(l[n:], "wait") {
			return true
		}
		i = k
	}
}

// detachWord: (^|[^[:alnum:]_-])(nohup|disown|setsid)([^[:alnum:]_-]|$)
func detachWord(l string) bool {
	edge := func(r rune) bool { return !grepAlnum(r) && r != '_' && r != '-' }
	for _, w := range []string{"nohup", "disown", "setsid"} {
		for i := 0; ; {
			j := strings.Index(l[i:], w)
			if j < 0 {
				break
			}
			st := i + j
			en := st + len(w)
			okL := st == 0
			if !okL {
				r, _ := utf8.DecodeLastRuneInString(l[:st])
				okL = edge(r)
			}
			okR := en == len(l)
			if !okR {
				r, _ := utf8.DecodeRuneInString(l[en:])
				okR = edge(r)
			}
			if okL && okR {
				return true
			}
			i = st + 1
		}
	}
	return false
}

// ---------- kill-by-lookup-guard ----------

const killMsg = "포트·이름으로 찾은 프로세스를 죽이지 마세요 — 자기가 띄운 프로세스의 pid를 기록해 그 pid만 kill 하세요(launchd 포워드·다른 레인 서버를 함께 죽입니다)."

const (
	killSeg   = "[^;&|\\n)`]*"
	killSegC  = "[^\\w.;&|\\n)`-]" // 앞 살피기 (?![\w.-])를 통과하고 $seg를 여는 첫 글자
	killSegSp = "[;&|\\n)`]"
)

// kwSeg: ${W}<kw>$E($seg) — 무리 1이 $seg (끝은 그 뒤 구분자나 끝)
func kwSeg(kw string) *regexp.Regexp {
	return regexp.MustCompile(`\A(?:` + reNS + `*/)?` + kw + `((?:` + killSegC + killSeg + `)?)(?:` + killSegSp + `|\z)`)
}

// kwE: ${W}<kw>$E
func kwE(kw string) *regexp.Regexp {
	return regexp.MustCompile(`\A(?:` + reNS + `*/)?` + kw + `(?:[^\w.-]|\z)`)
}

// kwSegOpt: ${W}<kw>$E$seg\s<opt>
func kwSegOpt(kw, opt string) *regexp.Regexp {
	return regexp.MustCompile(`\A(?:` + reNS + `*/)?` + kw + `(?:` + killSegC + killSeg + `)?` + reS + opt)
}

var (
	killUnless = regexp.MustCompile(`(?:^|[^\w.-])(?:bash|sh|zsh|dash)` + reS + `+(?:-\w+` + reS + `+)*-\w*c\b|(?:^|[^\w.-])(?:eval|ssh)(?:[^\w.-]|$)|\bhc` + reS + `+(?:exec|run)\b`)
	killHcKill = regexp.MustCompile(`\bhc` + reS + `+kill\b`)
	killKill   = kwE("kill")
	killKillS  = kwSeg("kill")
	killAll    = kwE("killall")
	killPkill  = kwSeg("pkill")
	killPgrep  = kwSeg("pgrep")
	killFuser  = kwSegOpt("fuser", `-[a-zA-Z]*k`)
	killLsof   = kwSegOpt("lsof", `-[a-zA-Z]*t`)
	killPs     = kwE("ps")
	killParent = regexp.MustCompile(`(?:^|` + reS + `)(?:-P|--parent)(?:` + reS + `|=|\d)`)
)

func killByLookupGuard(in []byte, _, stderr io.Writer) int {
	cmd, ok := jqCommand(in)
	if !ok || !strings.Contains(cmd, "kill") && !strings.Contains(cmd, "fuser") {
		return 0
	}
	if killVerdict(cmd) {
		fmt.Fprintln(stderr, killMsg)
		return 2
	}
	return 0
}

// killVerdict: 셸 판 perl 본문 — 참이면 막는다(exit 2)
func killVerdict(s string) bool {
	s = heredocStrip(s)
	if !killUnless.MatchString(s) {
		s = quoteStrip(s)
	}
	s = killHcKill.ReplaceAllString(s, "hc_k")
	kill := perlFind(s, 0, lbW, killKill) != nil
	if perlFind(s, 0, lbW, killAll) != nil {
		return true
	}
	if segLoop(s, killPkill, func(a string) bool { return !killParent.MatchString(a) }) {
		return true
	}
	if perlFind(s, 0, lbW, killFuser) != nil {
		return true
	}
	if kill && perlFind(s, 0, lbW, killLsof) != nil {
		return true
	}
	if kill && segLoop(s, killPgrep, func(a string) bool { return !killParent.MatchString(a) }) {
		return true
	}
	if psPipeKill(s) || killSubPs(s) {
		return true
	}
	return segLoop(s, killKillS, func(a string) bool {
		t := perlFields(a)
		i := 0
		if len(t) > 0 && (t[0] == "-s" || t[0] == "-n") {
			i = 2
		} else if len(t) > 0 && len(t[0]) >= 2 && t[0][0] == '-' && t[0] != "--" {
			i = 1
		}
		if i < len(t) && t[i] == "--" {
			i++
		}
		for ; i < len(t); i++ {
			if t[i] == "0" || t[i] == "-1" {
				return true
			}
		}
		return false
	})
}

// segLoop: while (/${W}<kw>$E($seg)/g) { … } — bad(seg)가 참이면 참
func segLoop(s string, re *regexp.Regexp, bad func(string) bool) bool {
	for from := 0; ; {
		loc := perlFind(s, from, lbW, re)
		if loc == nil {
			return false
		}
		if bad(s[loc[2]:loc[3]]) {
			return true
		}
		from = loc[3]
	}
}

// wKw: k의 낱말 앞에 ${W}(뒤 살피기 + 경로)가 설 수 있는가 — 앞 글자가 [\w.-]가 아니면(/ 포함) 된다
func wKw(s string, k int) bool {
	return k == 0 || !(perlWord(s[k-1]) || s[k-1] == '.' || s[k-1] == '-')
}

// eAt: (?![\w.-])
func eAt(s string, p int) bool {
	return p >= len(s) || !(perlWord(s[p]) || s[p] == '.' || s[p] == '-')
}

// psPipeKill: /${W}ps$E[^;&\n]*\|[^;&\n]*${W}kill$E/
func psPipeKill(s string) bool {
	for k := 0; k+2 <= len(s); k++ {
		if s[k:k+2] != "ps" || !wKw(s, k) || !eAt(s, k+2) {
			continue
		}
		end := k + 2
		for end < len(s) && s[end] != ';' && s[end] != '&' && s[end] != '\n' {
			end++
		}
		q := strings.IndexByte(s[k+2:end], '|')
		if q < 0 {
			continue
		}
		for r := k + 2 + q + 1; r <= end; r++ {
			if lbW(s[r-1]) && killKill.MatchString(s[r:]) {
				return true
			}
		}
	}
	return false
}

// killSubPs: /${W}kill$E[^;&\n]*\$\([^)]*${W}ps$E/
func killSubPs(s string) bool {
	for k := 0; k+4 <= len(s); k++ {
		if s[k:k+4] != "kill" || !wKw(s, k) || !eAt(s, k+4) {
			continue
		}
		for d := k + 4; d+1 < len(s) && s[d] != ';' && s[d] != '&' && s[d] != '\n'; d++ {
			if s[d] != '$' || s[d+1] != '(' {
				continue
			}
			pe := len(s)
			if x := strings.IndexByte(s[d+2:], ')'); x >= 0 {
				pe = d + 2 + x
			}
			for r := d + 2; r <= pe; r++ {
				if lbW(s[r-1]) && killPs.MatchString(s[r:]) {
					return true
				}
			}
		}
	}
	return false
}

// ---------- lane-no-shell-edit ----------

var shellEditMsg = map[int]string{
	3: "레인 세션에서는 heredoc 커밋 메시지를 쓰지 마세요 — 메시지를 Write 도구로 파일(스크래치 폴더 등)에 쓰고 git commit -F <파일>. heredoc은 권한 창을 띄워 무인 레인이 멈춥니다.",
	4: "레인 세션에서는 zsh 배열·확장(${(f)…})으로 경로를 만들지 마세요 — git add -- 경로를 글자로 적으세요. 권한 창이 떠 무인 레인이 멈춥니다.",
	5: "레인 세션에서는 파일을 셸로 고치지 마세요 — 파일 고치기는 Edit·Write 도구로 하세요. 셸로 고치면 권한 창이 떠 무인 레인이 멈춥니다(읽기·시험 실행·임시 폴더·.qa/ 쓰기는 그대로 됩니다).",
}

const (
	editSeg  = `[^;&|\n]*`
	editSegC = `[^\w.;&|\n-]`
)

var (
	editCommit  = regexp.MustCompile(`\bgit` + reS + `(?:[^;&|\n]*` + reS + `)?commit\b`)
	editHdOpen  = regexp.MustCompile(`<<-?[ \t]*['"]?\w`)
	editZsh     = regexp.MustCompile(`\$\{\([^)]*\)`)
	editTmp     = regexp.MustCompile(`/tmp\b|/private/|/var/folders/|\$\{?TMPDIR\b|\bmktemp\b|\btempfile\b|\bmkdtemp\b|scratchpad|(?:^|[\t\n\x0B\f\r /"'=])\.qa/`)
	editInterp  = regexp.MustCompile(`\A(?:` + reNS + `*/)?(?:python[\d.]*|node|bun|deno)(?:[^\w.-]|\z)`)
	editWrite   = regexp.MustCompile(`\bopen\([^)]*['"][wax][bt+]*['"]|\.write_(?:text|bytes)\(|\b(?:writeFileSync|appendFileSync)\(|\bBun\.write\(`)
	editRedirQ  = regexp.MustCompile(`(>{1,2}\|?[ \t]*)(?:'([^'"\n]*)'|"([^'"\n]*)")`)
	editSed     = regexp.MustCompile(`\A(?:` + reNS + `*/)?sed(?:` + editSegC + editSeg + `)?` + reS + `(?:-[a-zA-Z]*i|--in-place)`)
	editPerl    = regexp.MustCompile(`\A(?:` + reNS + `*/)?perl(?:` + editSegC + editSeg + `)?` + reS + `-[pnlew0-9]*i`)
	editSrc     = regexp.MustCompile(`\.(?:go|tsx?|jsx?|mjs|cjs|md|json|sh|bash|zsh|py|rs|swift|kt|java|rb|toml|ya?ml|css|scss|html|sql)$`)
	editRedir   = regexp.MustCompile(`\A>{1,2}\|?[ \t]*([^\t\n\x0B\f\r ;&|<>()]+)`)
	editTee     = regexp.MustCompile(`\A(?:` + reNS + `*/)?tee((?:` + editSegC + editSeg + `)?)(?:[;&|\n]|\z)`)
	editSkipTgt = regexp.MustCompile(`^[$&]|^/dev/`)
)

func laneNoShellEdit(in []byte, _, stderr io.Writer) int {
	if os.Getenv("HC_LANE") == "" {
		return 0
	}
	cmd, ok := jqCommand(in)
	if !ok || cmd == "" {
		return 0
	}
	if rc := shellEditVerdict(cmd); rc != 0 {
		fmt.Fprintln(stderr, shellEditMsg[rc])
		return 2
	}
	return 0
}

// shellEditVerdict: 셸 판 perl 본문의 종료 코드(0 통과 · 3 heredoc 커밋 · 4 zsh 확장 · 5 셸 쓰기)
func shellEditVerdict(raw string) int {
	if editCommit.MatchString(raw) && editHdOpen.MatchString(raw) {
		return 3
	}
	s := heredocStrip(raw)
	if editZsh.MatchString(perlSQ.ReplaceAllString(s, "''")) {
		return 4
	}
	if editTmp.MatchString(raw) {
		return 0
	}
	if perlFind(raw, 0, lbW, editInterp) != nil && editWrite.MatchString(raw) {
		return 5
	}
	s = editRedirQ.ReplaceAllString(s, "${1}${2}${3}")
	s = quoteStrip(s)
	if perlFind(s, 0, lbW, editSed) != nil || perlFind(s, 0, lbW, editPerl) != nil {
		return 5
	}
	lbRedir := func(b byte) bool { return !(b == '<' || b == '>' || b == '&' || b >= '0' && b <= '9') }
	for from := 0; ; {
		loc := perlFind(s, from, lbRedir, editRedir)
		if loc == nil {
			break
		}
		if t := s[loc[2]:loc[3]]; !editSkipTgt.MatchString(t) && editSrc.MatchString(t) {
			return 5
		}
		from = loc[1]
	}
	for from := 0; ; {
		loc := perlFind(s, from, lbW, editTee)
		if loc == nil {
			break
		}
		for _, f := range perlFields(s[loc[2]:loc[3]]) {
			if f[0] != '-' && f[0] != '$' && editSrc.MatchString(f) {
				return 5
			}
		}
		from = loc[3]
	}
	return 0
}

// ---------- loop-no-question ----------

func loopNoQuestion(_ []byte, _, stderr io.Writer) int {
	id := os.Getenv("HC_TERMINAL_ID")
	if id == "" {
		return 0
	}
	hc, err := exec.LookPath("hc")
	if err != nil {
		hc = os.Getenv("HOME") + "/.local/bin/hc"
	}
	if fi, err := os.Stat(hc); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
		return 0
	}
	c := exec.Command(hc, "get", id, "--json")
	c.Stdin = nil
	out, err := proc.Output(c)
	if err != nil {
		return 0
	}
	j := capture(string(out))
	name := plutilRaw(j, "name")
	job := plutilRaw(j, "labels", "job")
	if !strings.Contains(asciiLower(name+" "+job), "loop") {
		return 0
	}
	who := name
	if who == "" {
		who = id
	}
	fmt.Fprintf(stderr, "무인 루프(%s)에서는 질문 창을 띄우지 않는다. 답할 사람이 없어 루프 전체가 멈춘다.\n"+
		"- 되돌릴 수 있는 판단은 추천안으로 정해 상태 파일에 「가정/결정」으로 적고 진행한다.\n"+
		"- 사용자만 정할 일(되돌릴 수 없는 일·비용·정책)은 상태 파일 「결정 필요」에 적고 그 건만 보류한 채 다음 일을 계속한다.\n", who)
	return 2
}

// plutilRaw: $(printf '%s' "$j" | plutil -extract <a.b> raw -o - -) — JSON 한 값만(뒤에 다른 글이 있으면 실패),
// 문자열 그대로 · 정수 · 실수 %f · true/false · 사전은 열쇠를 정렬해 줄마다 · 배열은 개수 · 없거나 못 읽으면 ""
func plutilRaw(j string, keys ...string) string {
	var v any
	dec := json.NewDecoder(strings.NewReader(j))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil || dec.More() {
		return ""
	}
	if _, err := dec.Token(); err != io.EOF {
		return ""
	}
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		if v, ok = m[k]; !ok {
			return ""
		}
	}
	switch x := v.(type) {
	case string:
		return capture(x)
	case bool:
		return fmt.Sprint(x)
	case json.Number:
		if i, err := x.Int64(); err == nil && !strings.ContainsAny(x.String(), ".eE") {
			return fmt.Sprint(i)
		}
		f, err := x.Float64()
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%f", f)
	case map[string]any:
		ks := make([]string, 0, len(x))
		for k := range x {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return capture(strings.Join(ks, "\n"))
	case []any:
		return fmt.Sprint(len(x))
	}
	return "" // null
}
