package main

// 자동 감사 도우미 — 셸 판 lib/hc-fill/audit.sh 앞부분(arow·aupd·aadd·amigrate·audname·lastev·attempt_run·joblast·parkev·runlast·aesc·
// aterm·a_esc·a_send_esc·s_vput·s_vvoid·a_void·abase·abrief·a_put·aproj)과 tick.sh의 aconf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 차수 줄 변수(셸 판 aid ln rnd prun sha am pa ast afx av awk awt alc alw anv afl, 그리고 an·rep) — a_round 계열이 함께 쓴다
var auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst, auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl, auAn, auRep string

// aconf: 깨움 간격·수거 상한(분)
func aconf() {
	WAKE = auMinutes("wake_every", 10)
	ACKMAX = auMinutes("ack_max", 30)
}

// auMinutes: 설정 키의 분 값(없으면 기본, 숫자가 아니면 알림 한 줄 뒤 기본)
func auMinutes(k string, def int) int {
	v := conf(k)
	if v == "" {
		v = strconv.Itoa(def)
	}
	if !isDigits(v) {
		josa := "는"
		if k == "done_stall" {
			josa = "은"
		}
		fmt.Printf("알림: 설정 %s%s 분(숫자)이다, %d으로: %s\n", k, josa, def, v)
		return def
	}
	return atoi10(v)
}

// evF: 사건 줄의 i번째 칸(cut -f — 빈 줄이면 빈 값)
func evF(l string, i int) string { return splitRow(l).F(i) }

// auRest: i번째 칸부터 끝까지(셸 read의 마지막 변수)
func auRest(r Row, i int) string {
	if r.NF() < i {
		return ""
	}
	return strings.Join(r[i-1:], "\t")
}

// arow <aid>: 그 차수의 마지막 줄(17칸 이상)
func arow(aid string) Row {
	var l Row
	for _, r := range readRows(AT) {
		if r.NF() >= 17 && r.F(1) == aid {
			l = r
		}
	}
	return l
}

// aupd <aid> <칸> <값> …: 그 차수 줄의 칸을 바꾼다(dry-run은 안 씀). 값의 탭·줄바꿈은 빈칸, 빈 값은 -
func aupd(aid string, kv ...string) {
	if dry {
		return
	}
	text, ok := readFile(AT)
	if !ok {
		return
	}
	var out []Row
	for _, l := range lines(text) {
		r := splitRow(l)
		if r.NF() >= 17 && r.F(1) == aid {
			for i := 0; i+1 < len(kv); i += 2 {
				c, _ := strconv.Atoi(kv[i])
				r = r.Set(c, flat1(dash(kv[i+1])))
			}
		}
		out = append(out, r)
	}
	if err := writeAtomic(AT, rowsText(out)); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// aadd <17칸>: 빈 칸은 -
func aadd(f ...string) error {
	if dry {
		return nil
	}
	x := make([]string, len(f))
	for i, v := range f {
		x[i] = flat1(dash(v))
	}
	return appendLine(AT, strings.Join(x, "\t"))
}

// amigrate: 옛 감사 기록 옮기기 — 끝 상태는 legacy, queued는 escalate:legacy. aid는 원본 줄의 cksum(결정적)
func amigrate() {
	if dry || !nonEmpty(AT) {
		return
	}
	ls := readLines(AT)
	old := false
	for _, l := range ls {
		if nf := splitRow(l).NF(); nf >= 5 && nf < 17 {
			old = true
		}
	}
	if !old {
		return
	}
	var b strings.Builder
	for _, l := range ls {
		r := splitRow(l)
		if r.NF() >= 17 || r.NF() < 5 {
			b.WriteString(l + "\n")
			continue
		}
		st, nt := "legacy", "-"
		if r.F(5) == "queued" {
			st = "escalate:legacy"
			nt = r.F(1) + " 옛 감사 기록(" + r.F(2) + " " + r.F(3) + "번째) — 판정을 차수에 묶지 못한 채 hc-fill이 바뀌었다. 지휘 판단(옛 감사 줄 정리 포함)"
		}
		fx := r.F(4)
		if fx == "" {
			fx = "0"
		}
		b.WriteString(strings.Join([]string{fmt.Sprintf("al%08x", cksum([]byte(l))), r.F(1), r.F(3), "-", r.F(2), "-", "-", st, fx, "-",
			"0", "0", "-", "-", "0", "-", nt}, "\t") + "\n")
	}
	if writeAtomic(AT, b.String()) != nil {
		return
	}
	fmt.Println("감사 기록 옮김: 옛 줄을 차수 기록으로(legacy·escalate:legacy)")
}

// audname <레인>: 옛 감사 줄 이름. audit-<레인>이 32자를 넘으면 audit-<앞 21자>-<cksum 4자리 16진>
func audname(lane string) string {
	an := "audit-" + lane
	if len([]rune(an)) <= 32 {
		return an
	}
	return fmt.Sprintf("audit-%s-%04x", runeCut(lane, 21), cksum([]byte(lane))%65536)
}

// lastev <이름> <터미널> [<상태>]: 그 터미널이 그 이름으로 적은 마지막 사건 줄
func lastev(name, addr, state string) string {
	l := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(4) == name && r.F(3) == addr && (state == "" || r.F(5) == state) {
			l = e
		}
	}
	return l
}

var auRunKeyRe = regexp.MustCompile(`@[0-9]+$`)

// attempt_run <이름> <주소>: try.tsv에 확정한 실행 키
func attemptRun(name, addr string) string {
	for _, r := range readRows(DATA + "/try.tsv") {
		if r.F(1) == name && r.F(6) == addr && auRunKeyRe.MatchString(r.F(7)) {
			return r.F(7)
		}
	}
	return ""
}

// joblast <이름> <주소> [<상태>]: 확정한 실행 키 → 살아 있는 그 이름 터미널의 실행 → 주소·이름만
func joblast(name, addr, state string) string {
	if key := attemptRun(name, addr); key != "" {
		return runlast(name, addr, key, state)
	}
	// 이번 바퀴 터미널 표에 없는 주소(끝난 떠 있음 줄)는 hc get 없이 주소·이름으로 — hc get이 끝남·모름일 때와 같은 길이다(lsGone, tick.go).
	// 종료를 확정해야 하는 곳(받은편지 회신 등)은 pget을 직접 부르니 그대로 hc get으로 본다
	if lsGone(addr) {
		PRUN, PNAME = "", ""
	} else if pget(addr) == 0 && PNAME == name {
		return runlast(name, addr, PRUN, state)
	}
	return lastev(name, addr, state)
}

// auRunStart: 실행 키의 시작 시각(date -u -r <ms/1000>), 못 풀면 9(어떤 사건 시각보다 뒤)
func auRunStart(run string) string {
	ms := run
	if i := strings.IndexByte(run, '@'); i >= 0 {
		ms = run[i+1:]
	}
	if !isDigits(ms) {
		return "9"
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return "9"
	}
	return time.Unix(n/1000, 0).UTC().Format("2006-01-02T15:04:05Z")
}

// runlast <레인> <주소> <실행> [<상태>]: 10칸(발행 실행)이 있으면 그 값이 같은 것, 없거나 -인 옛 사건은 시각이 그 실행 시작 이후인 것
func runlast(name, addr, run, state string) string {
	t := auRunStart(run)
	l := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(4) != name || r.F(3) != addr || (state != "" && r.F(5) != state) {
			continue
		}
		if r.NF() >= 10 && r.F(10) != "-" {
			if r.F(10) != run {
				continue
			}
		} else if r.F(1) < t {
			continue
		}
		l = e
	}
	return l
}

// parkev <레인> <주소> <기계>: 그 실행의 마지막 report가 「주차」인가. 0 주차 · 1 아님 · 2 모름. PKL = 그 줄(「정리됨」은 건너뜀)
func parkev(lane, addr, m string) int {
	PKL = ""
	if EVBAD {
		return 2
	}
	if m != LOCAL && (!connHas(m) || has(evUnk(), m)) {
		return 2
	}
	saved := EV
	var f []string
	for _, e := range EV {
		if splitRow(e).F(5) != "정리됨" {
			f = append(f, e)
		}
	}
	EV = f
	PKL = joblast(lane, addr, "")
	EV = saved
	if evF(PKL, 5) == "주차" {
		return 0
	}
	return 1
}

// aesc <레인> <주인|-> <글> [<키>]: 그 레인 주인 지휘(없으면 모든 지휘)에 막힘 사건. 0 남김(dry-run 포함)
func aesc(lane, owner, msg, key string) int {
	if dry {
		fmt.Println(P + "지휘로 넘길 " + lane + ": " + msg)
		return 0
	}
	ao := owner
	if ao == "-" {
		ao = ""
	}
	if hcLaneReport("막힘", msg, lane, ao, key) == 0 {
		to := ao
		if to == "" {
			to = "모든 지휘"
		}
		fmt.Println("지휘로 넘김 " + lane + " → " + to + ": " + msg)
		return 0
	}
	fmt.Println("지휘로 넘기기 실패 " + lane + "(hc-lane report) — 다음 바퀴에 다시")
	return 1
}

// aterm: 끝 상태
func aterm(s string) bool {
	switch s {
	case "taken", "superseded", "coord", "nosha", "legacy":
		return true
	}
	return false
}

// a_esc <aid> <까닭> <글>: 기록을 먼저(escalate:<까닭> + 글), 사건은 a_send_esc가(키로 한 번)
func aEsc(aid, why, text string) {
	aupd(aid, "8", "escalate:"+why, "17", text)
	crash("after-escalate-record")
	aSendEsc(aid)
}

func aSendEsc(aid string) {
	r := arow(aid)
	a, l, s, t := r.F(1), r.F(2), r.F(8), auRest(r, 17)
	if a == "" {
		return
	}
	if aesc(l, qown(l), t, "hf:"+a+":"+strings.TrimPrefix(s, "escalate:")) != 0 {
		return
	}
	crash("after-escalate-event")
	if s == "escalate:nosha" {
		aupd(aid, "8", "nosha")
	} else {
		aupd(aid, "8", "coord")
	}
}

// s_vput: VPUT <aid> <sha256> <크기>(표준 입력 = 지시). 찍는 것 「WROTE|ALREADY <표시> <끝>」 · CONFLICT · BADINPUT
const sVput = `mkdir -p "$d" || exit 1; v=$d/$1.md; x=$d/$1.ack; t=$(mktemp "$d/.put.XXXXXX") || exit 1
cat >"$t" || { rm -f "$t"; exit 1; }
 hs() { shasum -a 256 "$1" | cut -d " " -f 1; }
if [ "$(wc -c <"$t" | tr -d " ")" != "$3" ] || [ "$(hs "$t")" != "$2" ]; then rm -f "$t"; echo BADINPUT; exit 0; fi
if ln "$t" "$v" 2>/dev/null; then r=WROTE; elif [ -f "$v" ] && [ "$(hs "$v")" = "$2" ]; then r=ALREADY; else rm -f "$t"; echo CONFLICT; exit 0; fi
rm -f "$t"; a=none; [ ! -f "$x" ] || a=$(cut -f 1 "$x" | head -n 1); e=no; [ ! -f "$d/$1.done" ] || e=yes; echo "$r ${a:-none} $e"`

// s_vvoid: VVOID <aid> <시각>: 표시 void를 배타 생성 — 이미 있으면 그대로. 찍는 것 = 지금 표시
const sVvoid = `mkdir -p "$d" || exit 1; x=$d/$1.ack; t=$(mktemp "$d/.void.XXXXXX") || exit 1
printf "void\t%s\n" "$2" >"$t" && { ln "$t" "$x" 2>/dev/null; rm -f "$t"; }; [ -f "$x" ] || exit 1; cut -f 1 "$x" | head -n 1`

// auOK: 셸 판 aok — local이거나 연결됐고 사건을 읽은 기계
func auOK(m string) bool { return m == LOCAL || (connHas(m) && !has(AUNK, m)) }

// a_void <aid> <레인> <기계>: 0 void를 잡음, 2 레인이 이미 수거·착지 시작(→ taken), 1 모름
func aVoid(aid, lane, m string) int {
	if !auOK(m) {
		return 1
	}
	if dry {
		return 0
	}
	vo, rc := onm(nil, m, sVvoid, "~/.local/share/hc-fill/verdicts/"+lane, aid, utcnow())
	if rc != 0 {
		return 1
	}
	switch vo = sub(vo); vo {
	case "void":
		return 0
	case "taken", "landing":
		aupd(aid, "8", "taken")
		fmt.Println("감사 " + aid + ": 레인이 이미 판정을 받음(" + vo + ") — 끝")
		return 2
	}
	return 1
}

// abase <프로젝트> <기계>: 기본 가지 참조(local repo 줄이 있으면 여기서, 없으면 그 기계에서). ok=false면 모름
func abase(p, m string) (string, bool) {
	if path := repoOf(p, LOCAL); path != "" {
		return projectRef(path)
	}
	path := repoOf(p, m)
	if path == "" {
		return "", false
	}
	out, rc := onm(nil, m, projectRefSh+"\nproject_ref \"$d\"", path)
	if rc != 0 {
		return "", false
	}
	return sub(out), true
}

// auShq: 셸 낱말로 안전한 경로는 그대로, 공백·셸 특수 글자가 있으면 작은따옴표 인용(앞 ~/는 펼쳐지게 따옴표 밖에)
func auShq(s string) string {
	safe := s != ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("/._-+,:@%=~", c) >= 0 {
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	pre := ""
	if strings.HasPrefix(s, "~/") {
		pre, s = "~/", s[2:]
	}
	return pre + "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// abrief <aid> <레인> <기계> <sha40> <프로젝트>: 감사 지시문 briefs/audit/<aid>.md를 확정(이미 있으면 그대로). 참 = 됨
func abrief(aid, lane, m, sha40, p string) bool {
	bf := DATA + "/briefs/audit/" + aid + ".md"
	if nonEmpty(bf) {
		return true
	}
	repo := repoOf(p, m)
	if repo == "" {
		fmt.Println("자동 감사 못 함 " + lane + ": 설정에 " + p + " @" + m + " repo 줄이 없다")
		return false
	}
	raw, ok := readFile(AUD)
	if !ok {
		fmt.Println("자동 감사 못 함 " + lane + ": 틀을 읽지 못함 " + AUD + " — 다음 바퀴에")
		return false
	}
	t := sub(raw)
	if !strings.Contains(t, "{audit_id}") { // 옛 설치 틀: 새 판정 줄 꼴을 덧붙인다
		t += "\n\n## 판정 줄 (hc-fill — 이 절이 위 판정 줄보다 앞선다)\n" +
			"마지막에 정확히 이 꼴로 한 번 부른다(숫자만 바꾼다. 차수 ID와 40자 sha가 맞아야 hc-fill이 판정을 받는다):\n" +
			"`hc-lane report 판정 {sha} \"감사 완료 {audit_id} 높음 <n>·중간 <n>·낮음 <n>\"`"
	}
	wt := strings.TrimSuffix(repo, "/") + ".wt/" + lane
	t = strings.ReplaceAll(t, "{lane}", lane)
	t = strings.ReplaceAll(t, "{machine}", m)
	// 셸 판과 다름: 「-C {worktree}」(명령 인자 자리)는 경로에 공백·특수 글자가 있으면 셸 인용 값으로
	t = strings.ReplaceAll(t, "-C {worktree}", "-C "+auShq(wt))
	t = strings.ReplaceAll(t, "{worktree}", wt)
	t = strings.ReplaceAll(t, "{branch}", lane)
	t = strings.ReplaceAll(t, "{sha}", sha40)
	t = strings.ReplaceAll(t, "{audit_id}", aid)
	ref, ok := abase(p, m)
	if !ok {
		fmt.Println("자동 감사 미룸 " + lane + ": 기본 가지 조회 실패")
		return false
	}
	branch := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "origin/")
	t = strings.ReplaceAll(t, "origin/{base}", ref)
	t = strings.ReplaceAll(t, "{base}", branch)
	t = strings.ReplaceAll(t, "{brief}", "~/.local/share/hc-fill/runs/"+lane+"-run.md")
	t = strings.ReplaceAll(t, "{report}", "~/.local/share/hc-fill/reports/audit-"+aid+".md")
	if t, ok = aprevFill(t, aid, lane, m); !ok {
		fmt.Println("자동 감사 미룸 " + lane + ": " + m + " 에서 지휘 지시 파일을 확인하지 못했다")
		return false
	}
	if os.MkdirAll(filepath.Dir(bf), 0o755) != nil || writeAtomic(bf, t+"\n") != nil {
		fmt.Println("자동 감사 못 함 " + lane + ": 감사 지시문을 쓰지 못함 " + bf)
		return false
	}
	return true
}

// a_put <aid> <레인> <기계> <프로젝트>: 감사 줄 audit-<aid>를 대기열에(이미 있으면 그대로). 주인은 레인 줄 주인. 참 = 됨
func aPut(aid, lane, m, p string) bool {
	if qrow("audit-"+aid) != nil {
		return true
	}
	return qAdd("", "audit-"+aid, p, DATA+"/briefs/audit/"+aid+".md", "-", m, "wait", "-", "-", "-", nowMin(), "-", qown(lane),
		"urgent", "no-early", "audit="+lane, "-") == 0
}

// aproj <aid>: 그 차수의 프로젝트(audit-proj.tsv)
func aproj(aid string) string {
	for _, r := range readRows(DATA + "/audit-proj.tsv") {
		if r.F(1) == aid {
			return r.F(2)
		}
	}
	return ""
}

// auSend: hc send <주소> <글>을 20초 시한으로(셸 판 perl alarm 20 — 넘기면 그 프로세스만 끈다). 참 = 종료 0
func auSend(addr, msg string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tickCall("hc", []string{"send"})
	return exec.CommandContext(ctx, "hc", "send", addr, msg).Run() == nil
}
