// send.go: hc-lane send --all·broadcast·early-wait·verdict(셸 판 lib/hc-lane/watch.sh send_cmd·early_wait_cmd·verdict_cmd·vload·vmark·vstate·vmyrun)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/gitx"
)

func init() {
	register("send", sendCmd, noHelp)
	register("broadcast", func(a []string) int { return sendCmd(append([]string{"--all"}, a...)) }, noHelp)
	register("early-wait", earlyWaitCmd, nil) // 셸 판도 -h를 레인 이름으로 읽는다
	register("verdict", verdictCmd, noHelp)
}

// sendCmd: send --all [--audits] [--from <지휘>] <글…> — 이 지휘가 띄운 살아 있는 레인 전부에 hc send.
// 종료 0 다 보냄(대상 없음 포함), 1 보내기·hc ls 실패, 2 인자
func sendCmd(a []string) int {
	all, aud := false, false
	from := os.Getenv("HC_TERMINAL_ID")
	var msg []string
	for i := 0; i < len(a); i++ {
		switch a[i] {
		case "--all":
			all = true
		case "--audits":
			aud = true
		case "--from":
			from = val(a, i)
			i++
		case "--":
			msg = append(msg, a[i+1:]...)
			i = len(a)
		default:
			msg = append(msg, a[i])
		}
	}
	m := strings.Join(msg, " ")
	if !all {
		fmt.Fprintln(os.Stderr, "hc-lane send: --all 을 준다(이 지휘의 레인 전부에 보낸다. 한 레인만이면 hc send <주소>)")
		return 2
	}
	if from == "" {
		fmt.Fprintln(os.Stderr, "hc-lane send: 지휘 주소를 모른다(지휘 터미널에서 부르거나 --from <주소>)")
		return 2
	}
	if m == "" {
		fmt.Fprintln(os.Stderr, "hc-lane send: 보낼 글이 없다")
		return 2
	}
	tl, rc := hcRun(30, errDrop, "ls", "--all", "--json", "--label", "hc-lane=1")
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane send: hc ls 실패 — 아무에게도 안 보냈다")
		return 1
	}
	if !jlistOK(tl, "terminals", "address", "state") {
		fmt.Fprintln(os.Stderr, "hc-lane send: hc ls 출력을 읽지 못했다(잘림·형식) — 아무에게도 안 보냈다")
		return 1
	}
	e := &ev{ed: evdir(), pmap: jlist(tl, "terminals", "address", "parent")}
	km := e.lkinds()
	ok, bad := 0, 0
	for _, r := range jlist(tl, "terminals", "address", "state", "name") {
		ad, n := r[0], r[2]
		if r[1] != "live" || e.owner(ad) != lastSeg(from) {
			continue
		}
		if !aud && (strings.HasPrefix(n, "audit-") || km[[2]string{lastSeg(ad), n}] == "read") {
			continue
		}
		var eb strings.Builder
		if _, rc := captErr(60, &eb, "hc", "send", ad, m); rc == 0 {
			fmt.Printf("보냄 %s %s\n", ad, n)
			ok++
		} else {
			why := strings.TrimRight(eb.String(), "\n")
			if why == "" {
				why = "hc send 실패"
			}
			fmt.Printf("실패 %s %s — %s\n", ad, n, flat(why))
			bad++
		}
	}
	fmt.Printf("보냄 %d · 실패 %d\n", ok, bad)
	if bad != 0 {
		return 1
	}
	return 0
}

// earlyWaitCmd: early-wait <선행…> — hc-fill이 써 주는 선행 상태(after.tsv)로 먼저 띄움 레인이 계속 기다릴지
func earlyWaitCmd(a []string) int {
	if len(a) == 0 {
		fmt.Fprintln(os.Stderr, "사용법: hc-lane early-wait <선행 레인>…")
		return 2
	}
	af := os.Getenv("HOME") + "/.local/share/hc-fill/after.tsv"
	ls, _ := readLines(af)
	now := time.Now().Unix()
	bad := false
	for _, p := range a {
		l := ""
		for _, x := range ls {
			if strings.SplitN(x, tab, 2)[0] == p {
				l = x
			}
		}
		// IFS=탭 read -r _ s w t: 빈 칸은 겹쳐 하나, 마지막 칸이 나머지
		f := strings.FieldsFunc(l, func(r rune) bool { return r == '\t' })
		if len(f) > 4 {
			f = append(f[:3], strings.Join(f[3:], tab))
		}
		s, w, t := fld(f, 2), fld(f, 3), fld(f, 4)
		if !digits(t) {
			t = ""
		}
		switch {
		case l == "":
			fmt.Printf("%s: 기록 없음(선행 상태 모름 — %s)\n", p, af)
			bad = true
		case t == "" || now-atoi(t) > 900:
			fmt.Printf("%s: 기록이 %d분 묵음(hc-fill이 안 돎) — %s\n", p, (now-atoi(t))/60, w)
			bad = true
		case s == "go":
			fmt.Printf("%s: %s\n", p, w)
		default:
			fmt.Printf("%s: %s\n", p, w)
			bad = true
		}
	}
	if bad {
		return 1
	}
	fmt.Println("선행이 살아 진행 중 — 기한이 지나도 계속 기다린다")
	return 0
}

// verdict: 자동 감사 판정 하나(hc-fill이 둔 <판정 폴더>/<레인>/<aid>.md)
type verdict struct {
	dir, f, ack, done            string
	aid, run, sha, v, rep, b, ro string
}

// vload: 1 없음, 2 꼴 틀림
func vload(lane, aid string) (*verdict, int) {
	if lane+aid == "" || !onlyChars(lane+aid, alnum+"_-") {
		return nil, 2
	}
	d := os.Getenv("HC_FILL_VERDICTS")
	if d == "" {
		d = os.Getenv("HOME") + "/.local/share/hc-fill/verdicts"
	}
	d += "/" + lane
	vd := &verdict{dir: d, f: d + "/" + aid + ".md", ack: d + "/" + aid + ".ack", done: d + "/" + aid + ".done"}
	if st, err := os.Stat(vd.f); err != nil || !st.Mode().IsRegular() {
		return vd, 1
	}
	ls, err := readLines(vd.f)
	if err != nil {
		return vd, 2
	}
	get := func(k string) string {
		for _, l := range ls {
			if f := strings.Split(l, tab); f[0] == k {
				return fld(f, 2)
			}
		}
		return ""
	}
	vd.aid, vd.run, vd.sha, vd.v, vd.rep, vd.b, vd.ro = get("aid"), get("run"), get("sha"), get("verdict"), get("report"), get("base"), get("round")
	if vd.aid != aid || vd.run == "" || len(vd.sha) != 40 || !onlyChars(vd.sha, "0123456789abcdef") {
		return vd, 2
	}
	if vd.v != "pass" && vd.v != "fix" {
		return vd, 2
	}
	if vd.b == "" {
		vd.b = "main"
	}
	if !onlyChars(vd.b, alnum+"_./-") {
		return vd, 2
	}
	return vd, 0
}

// state: none|taken|landing|void(표시 파일 첫 줄의 첫 칸)
func (vd *verdict) state() string {
	b, err := os.ReadFile(vd.ack)
	if err != nil {
		if _, serr := os.Lstat(vd.ack); serr != nil {
			return "none"
		}
		return ""
	}
	l, _, _ := strings.Cut(string(b), "\n")
	return strings.SplitN(l, tab, 2)[0]
}

// mark: 표시를 배타적으로 만든다(ln). 0 만듦, 1 이미 있음, 2 실패
func (vd *verdict) mark(text string) int {
	t, err := os.CreateTemp(vd.dir, ".ack.")
	if err != nil {
		return 2
	}
	tmp := t.Name()
	defer os.Remove(tmp)
	if _, err := t.WriteString(text + "\n"); err != nil {
		t.Close()
		return 2
	}
	if t.Close() != nil {
		return 2
	}
	if os.Link(tmp, vd.ack) == nil {
		return 0
	}
	if _, err := os.Lstat(vd.ack); err == nil {
		return 1
	}
	return 2
}

func (vd *verdict) round() string {
	if vd.ro == "" {
		return ""
	}
	return vd.ro + "번째 "
}

// myRun: 이 터미널의 실행 ID(HC_LANE_JOB, 없으면 <터미널>@<created_ms>)
func myRun() (string, bool) {
	if j := os.Getenv("HC_LANE_JOB"); j != "" {
		return j, true
	}
	tid := os.Getenv("HC_TERMINAL_ID")
	if tid == "" {
		return "", false
	}
	out, rc := hcRun(10, errDrop, "get", tid, "--json")
	if rc != 0 {
		return "", false
	}
	c, ok := liveCreatedMs(out)
	if !ok {
		return "", false
	}
	return lastSeg(tid) + "@" + c, true
}

func verdictCmd(a []string) int {
	vn, aid := os.Getenv("HC_LANE"), ""
	for i := 0; i < len(a); i++ {
		switch {
		case a[i] == "--name":
			vn = val(a, i)
			i++
		case strings.HasPrefix(a[i], "-"):
			die("verdict: 알 수 없는 인자: %s", a[i])
		case aid != "":
			die("verdict: 판정 ID는 하나: %s", a[i])
		default:
			aid = a[i]
		}
	}
	if aid == "" {
		fmt.Fprintln(os.Stderr, "hc-lane verdict: 판정 ID를 준다(hc-fill이 깨운 글의 「판정 <ID>」): hc-lane verdict <ID>")
		return 2
	}
	if vn == "" || !onlyChars(vn, alnum+"_-") {
		fmt.Fprintf(os.Stderr, "hc-lane verdict: 레인 이름이 없거나 이상하다($HC_LANE 또는 --name): %s\n", vn)
		return 2
	}
	vd, rc := vload(vn, aid)
	switch rc {
	case 1:
		fmt.Fprintf(os.Stderr, "hc-lane verdict: 판정 %s 가 없다(%s)\n", aid, vd.f)
		return 1
	case 2:
		f := ""
		if vd != nil {
			f = vd.f
		}
		fmt.Fprintf(os.Stderr, "hc-lane verdict: 판정 파일 꼴이 틀렸다: %s — 지휘에 알린다(hc-lane report 막힘)\n", f)
		return 1
	}
	me, ok := myRun()
	if !ok {
		fmt.Fprintln(os.Stderr, "hc-lane verdict: 이 터미널의 실행을 못 읽었다($HC_LANE_JOB·hc get) — 아무것도 안 했다, 다시 부른다")
		return 75
	}
	if vd.run != me {
		fmt.Printf("hc-lane verdict: 판정 %s 는 다른 실행(%s)의 것 — 손대지 않았다(이 실행 %s)\n", aid, vd.run, me)
		return 1
	}
	fixMsg := fmt.Sprintf("감사 지적 %s%s — %s. %s를 읽고 높음·중간을 고친 뒤 다시 검증하고 hc-lane report 완료 <새 sha>, 그리고 기다린다", vd.round(), aid, vd.sha, vd.rep)
	switch vd.state() {
	case "void":
		fmt.Printf("판정 %s 는 hc-fill이 취소했다 — 아무것도 하지 않는다. 다음 지시를 기다린다\n", aid)
		return 0
	case "landing":
		if _, err := os.Lstat(vd.done); err == nil {
			d, _ := readLines(vd.done)
			s := ""
			if len(d) > 0 {
				s = fld(strings.Split(d[0], tab), 2)
				if len(s) > 8 {
					s = s[:8]
				}
			}
			fmt.Printf("판정 %s: 이미 착지함(%s)\n", aid, s)
			return 0
		}
		fmt.Printf("판정 %s: 착지를 시작했다 — 같은 착지 줄을 다시: hc-lane land %s --verdict %s\n", aid, vd.b, aid)
		return 0
	case "taken":
		b, _ := os.ReadFile(vd.ack)
		fmt.Printf("이미 받음 %s (%s)\n", aid, strings.TrimRight(strings.ReplaceAll(string(b), "\t", " "), "\n"))
		fmt.Println(fixMsg)
		return 0
	}
	// 레인 작업 트리(연결 작업 트리)가 아니면 HEAD로 짐작하지 않는다
	gd, grc := gitx.Out("", nil, "rev-parse", "--absolute-git-dir", "--git-common-dir")
	g := strings.Split(gd, "\n")
	if grc != 0 || len(g) < 2 || realDir(g[0]) == "" || realDir(g[0]) == realDir(g[1]) {
		pwd, _ := os.Getwd()
		fmt.Printf("판정 %s: 작업 트리 없음 — 제출·착지했으면 수거 불필요(hc-fill이 닫는다). 아무것도 안 했다(여기 %s 는 레인 작업 트리가 아니다)\n", aid, pwd)
		return 0
	}
	vh, hrc := gitx.Out("", nil, "rev-parse", "--verify", "-q", "HEAD")
	if hrc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane verdict: HEAD를 못 읽었다(git 실패) — 아무것도 안 했다, 작업 트리에서 다시 부른다")
		return 75
	}
	if vd.sha != vh {
		fmt.Printf("판정 %s 는 %s의 것인데 HEAD가 %s — 착지·고침 안 한다. 지금 커밋으로 hc-lane report 완료 %s 를 부르고 기다린다\n", aid, vd.sha[:8], short8(vh), short8(vh))
		return 0
	}
	if vd.v == "pass" {
		fmt.Printf("감사 통과 %s%s — %s. 착지: 이 작업 트리에서 hc-lane land %s --verdict %s (run 지시문의 착지 줄 대신 이 줄), 그 뒤 hc-lane report 착지 %s\n", vd.round(), aid, vd.sha, vd.b, aid, vd.sha[:8])
		return 0
	}
	if vd.mark("taken"+tab+utcNow()+tab+me) == 2 {
		fmt.Fprintln(os.Stderr, "hc-lane verdict: 표시를 두지 못했다 — 다시 부른다")
		return 75
	}
	switch s := vd.state(); s {
	case "void":
		fmt.Printf("판정 %s 는 hc-fill이 취소했다 — 아무것도 하지 않는다\n", aid)
	case "taken":
		fmt.Println(fixMsg)
	default:
		fmt.Fprintf(os.Stderr, "hc-lane verdict: 표시가 예상 밖이다(%s): %s — 지휘에 알린다\n", s, vd.ack)
		return 1
	}
	return 0
}

func short8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// realDir: cd <d> && pwd -P(상대 경로는 지금 폴더 기준). 못 가면 빈 값
func realDir(d string) string {
	a, err := filepath.Abs(d)
	if err != nil {
		return ""
	}
	r, err := filepath.EvalSymlinks(a)
	if err != nil {
		return ""
	}
	return r
}
