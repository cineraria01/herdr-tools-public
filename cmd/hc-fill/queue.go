package main

// 대기열(queue.tsv)·받은편지 장부(inbox.tsv) 쓰기 — 셸 판 common.sh의 qlock·qset·lset·rmigrate·rfix·q_*.
// 대기열 쓰기는 모두 queue.lock 안에서(셸 판과 같은 파일·같은 flock — 두 판이 함께 돌아도 서로 덮지 않는다).
// 셸 판의 qlock은 자식 프로세스라 그 안의 die·crash는 그 자식만 끝냈다 — withQ가 그 꼴을 그대로 받는다(종료 코드로 돌려줌)

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/lockx"
)

var qHeld bool // queue.lock을 쥐고 있다(셸 판 HC_FILL_Q=1)

// lockWait: path를 timeout까지 기다려 잠근다. 못 잡으면 lockf처럼 한 줄 찍고 nil(종료 75 자리)
func lockWait(path string, timeout time.Duration) *os.File {
	end := time.Now().Add(timeout)
	for {
		f, busy, err := lockx.TryLock(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lockf: %s: %v\n", path, err)
			return nil
		}
		if !busy {
			return f
		}
		if !time.Now().Before(end) {
			fmt.Fprintf(os.Stderr, "lockf: %s: already locked\n", path)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// withQ: 대기열 잠금 안에서 fn. 0 됨 · 75 잠금 기다림 넘김 · 그 밖은 fn 안의 die·exit·crash 코드. 이미 쥐었으면 그대로 부른다
func withQ(fn func()) (rc int) {
	if qHeld {
		fn()
		return 0
	}
	if err := os.MkdirAll(DATA, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		return 1
	}
	f := lockWait(DATA+"/queue.lock", 30*time.Second)
	if f == nil {
		return 75
	}
	qHeld = true
	defer func() {
		qHeld = false
		f.Close()
		if r := recover(); r != nil {
			le, ok := r.(lockExit)
			if !ok {
				panic(r)
			}
			rc = le.code
		}
	}()
	fn()
	return 0
}

func qRows() []Row { return readRows(Q) }

// qRowsWHook: 시험 자리(평소 nil) — 쓰기 명령의 재읽기를 실패시켜 그 명령이 qRowsW로 읽는지 본다(R-29)
var qRowsWHook func()

// qRowsW: 고쳐 다시 쓸 대기열 줄 — 파일이 있는데 못 읽으면 종료 1(빈 목록으로 읽어 빈 대기열을 써 넣지 않게, Fable L1 — 셸 판은 awk 실패면 mv를 안 했다)
func qRowsW() []Row {
	if qRowsWHook != nil {
		qRowsWHook()
	}
	text, ok := readFile(Q)
	if !ok && exists(Q) {
		fmt.Fprintf(os.Stderr, "hc-fill: 대기열을 읽지 못해 고치지 않는다: %s\n", Q)
		exit(1)
	}
	var rs []Row
	for _, l := range lines(text) {
		rs = append(rs, splitRow(l))
	}
	return rs
}

// qset: 대기열 줄의 상태·기계·주소·남긴것(6~9칸)과 먼저(14)·감사(15) 칸. useID면 17칸이 qrid인 줄만. 0 바꿈 · 1 그 줄 없음 · 2 못 읽음
func qset(useID bool, qrid, n, s, m, a, k, e, u string) int {
	if !qHeld {
		// 시험: 잠금을 잡기 직전에 끼어들기(셸 판 시험은 가짜 lockf로 같은 자리에 끼어든다) — HC_FILL_TEST_QSET="<이름> <상태>"
		if os.Getenv("HC_FILL_TEST_QSET") == n+" "+s {
			Cmd{Name: "sh", Args: []string{"-c", os.Getenv("HC_FILL_TEST_QSET_CMD")}, Err: errPass}.Run()
		}
		rc := 0
		if w := withQ(func() { rc = qset(useID, qrid, n, s, m, a, k, e, u) }); w != 0 {
			return w
		}
		return rc
	}
	text, ok := readFile(Q)
	if !ok {
		return 2
	}
	var out []Row
	found := false
	for _, l := range lines(text) {
		r := splitRow(l)
		if r.F(1) == n && (!useID || r.F(17) == qrid) {
			r = r.Set(6, s).Set(7, m).Set(8, a).Set(9, k)
			found = true
			pad := func(upto int) {
				for i := r.NF() + 1; i < upto; i++ {
					v := "-"
					if i == 13 {
						v = "normal"
					}
					r = r.Set(i, v)
				}
			}
			if e != "" {
				pad(14)
				r = r.Set(14, e)
			}
			if u != "" {
				pad(15)
				r = r.Set(15, u)
			}
		}
		out = append(out, r)
	}
	if !found {
		return 1
	}
	if err := writeAtomic(Q, rowsText(out)); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		return 2
	}
	return 0
}

// pad16: 16칸까지 -로 채우고 17칸 줄 ID
func pad16(f ...string) Row {
	r := Row(append([]string(nil), f...))
	for len(r) < 16 {
		r = append(r, "-")
	}
	return append(r, rowid())
}

func qHasName(n string) bool {
	for _, r := range qRows() {
		if r.F(1) == n {
			return true
		}
	}
	return false
}

// qHasNameW: 쓰기 전 같은 이름 확인 — 대기열이 있는데 못 읽으면 종료 1(모름을 「없음」으로 읽어 같은 이름을 더하지 않게, R-21)
func qHasNameW(n string) bool {
	for _, r := range qRowsW() {
		if r.F(1) == n {
			return true
		}
	}
	return false
}

// thenofW: 쓰기 전 thenof — 대기열이나 줄의 지시문(없는 파일은 「안 붙음」)을 못 읽으면 종료 1(R-21)
func thenofW(name string) string {
	pre := "## 이어서: " + name + " "
	for _, r := range qRowsW() {
		b, err := os.ReadFile(r.F(3))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "hc-fill: 대기열 줄 %s의 지시문을 읽지 못해 더하지 않는다: %s\n", r.F(1), r.F(3))
			exit(1)
		}
		for _, l := range lines(string(b)) {
			if strings.HasPrefix(l, pre) {
				return r.F(1)
			}
		}
	}
	return ""
}

// qAdd: 대기열 끝에 한 줄(셸 판 _add). tmp가 있으면 이름이 새것일 때만 그 사본을 f[2](지시문 자리)로 옮긴다. 0 됨
func qAdd(tmp string, f ...string) int {
	return withQ(func() {
		if qHasNameW(f[0]) {
			die("이미 대기열에 있다: %s (hc-fill ls)", f[0])
		}
		// 잠금 밖 검사와 이 사이에 같은 이름이 --then으로 붙었을 수 있다 — 잠금 안에서 다시(아스트라 E03)
		if x := thenofW(f[0]); x != "" {
			die("%s은 이미 %s에 이어 붙였다(--then) — 다른 이름으로", f[0], x)
		}
		if tmp != "" {
			if err := os.Rename(tmp, f[2]); err != nil {
				die("지시문 사본을 두지 못했다: %s", f[2])
			}
		}
		if err := appendLine(Q, pad16(f...).Line()); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
			exit(1)
		}
	})
}

// thenof <이름>: 그 이름이 「## 이어서: <이름> 」으로 붙은 대기열 줄 이름(없으면 빈 값)
func thenof(name string) string {
	pre := "## 이어서: " + name + " "
	for _, r := range qRows() {
		for _, l := range readLines(r.F(3)) {
			if strings.HasPrefix(l, pre) {
				return r.F(1)
			}
		}
	}
	return ""
}

// ---- 받은편지 장부: 요청ID 레인 되돌릴곳 상태 종류 해시 크기 비고 시각 ----

// lget <요청ID>: 그 요청의 마지막 줄(없으면 nil)
func lget(rid string) Row {
	var last Row
	for _, r := range readRows(LED) {
		if r.F(1) == rid {
			last = r
		}
	}
	return last
}

// lset <요청ID> <칸> <값> …: 그 줄의 칸을 바꾸고 9칸에 시각. 0 됨
func lset(rid string, kv ...string) int {
	if !qHeld {
		rc := 0
		if w := withQ(func() { rc = lset(rid, kv...) }); w != 0 {
			return w
		}
		return rc
	}
	text, ok := readFile(LED)
	if !ok {
		return 2
	}
	t := nowMin()
	var out []Row
	for _, l := range lines(text) {
		r := splitRow(l)
		if r.F(1) == rid {
			for i := 0; i+1 < len(kv); i += 2 {
				c, _ := strconv.Atoi(kv[i])
				r = r.Set(c, flat1(dash(kv[i+1])))
			}
			r = r.Set(9, t)
		}
		out = append(out, r)
	}
	if err := writeAtomic(LED, rowsText(out)); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		return 1
	}
	return 0
}

// replyOf: 대기열 11칸 「<되돌릴곳>#<요청ID>」 → 되돌릴곳·요청ID(# 없으면 ok=false)
func replyOf(f11 string) (dest, rid string, ok bool) {
	i := strings.LastIndexByte(f11, '#')
	if i < 0 {
		return "", "", false
	}
	return f11[:i], f11[i+1:], true
}

// rmigrate: 옛 받은편지 줄 옮기기·남은 고정본 회수·끊긴 접수 장부 복원·결과 고정 줄 빼기(잠금 안). 0 됨
func rmigrate() int {
	if nonEmpty(LED) {
		for _, r := range readRows(LED) {
			if r.F(4) != "open" || !isFile(DATA+"/replies/"+r.F(1)+".md") {
				continue
			}
			if rfix(r.F(1), "-", "", false) != 0 {
				return 1
			}
		}
	}
	if !nonEmpty(Q) {
		return 0
	}
	var old []Row
	for _, r := range qRows() {
		if r.F(11) != "" && r.F(11) != "-" && !strings.Contains(r.F(11), "#") {
			old = append(old, r)
		}
	}
	for _, o := range old {
		n, st, d := o.F(1), o.F(6), o.F(11)
		rr := fmt.Sprintf("rl%08x", cksum([]byte(n+" "+d)))
		if lget(rr) == nil {
			s, lg := "open", "-"
			if st == "sent" {
				s = "sent"
			}
			if st == "up" {
				lg = "legacy"
			}
			if appendLine(LED, strings.Join([]string{rr, n, d, s, "-", "-", "-", lg, nowMin()}, "\t")) != nil {
				return 1
			}
		}
		var out []Row
		for _, r := range qRowsW() {
			if r.F(1) == n && !strings.Contains(r.F(11), "#") {
				r = r.Set(11, r.F(11)+"#"+rr)
			}
			out = append(out, r)
		}
		if writeAtomic(Q, rowsText(out)) != nil {
			return 1
		}
	}
	// _inadd가 대기열만 쓴 뒤 끊겼으면 rm도 그 요청을 취소할 수 있게 장부를 복원한다
	seen := map[string]bool{}
	for _, r := range readRows(LED) {
		seen[r.F(1)] = true
	}
	for _, r := range qRows() {
		d, rid, ok := replyOf(r.F(11))
		if !ok || seen[rid] {
			continue
		}
		s := "open"
		if r.F(6) == "sent" {
			s = "sent"
		}
		if appendLine(LED, strings.Join([]string{rid, r.F(1), d, s, "-", "-", "-", "-", r.F(10)}, "\t")) != nil {
			return 1
		}
	}
	if !nonEmpty(LED) {
		return 0
	}
	x := map[string]string{}
	for _, r := range readRows(LED) {
		if r.F(4) != "open" {
			x[r.F(1)] = r.F(5)
		}
	}
	text, _ := readFile(Q)
	var out []Row
	for _, l := range lines(text) {
		r := splitRow(l)
		if _, rid, ok := replyOf(r.F(11)); ok {
			if k, in := x[rid]; in && (r.F(6) == "wait" || k == "cancel") {
				continue
			}
		}
		out = append(out, r)
	}
	if rowsText(out) == text {
		return 0
	}
	if writeAtomic(Q, rowsText(out)) != nil {
		return 1
	}
	say("대기열 복구: 결과를 고정한 받은편지 줄을 뺐다(rm 중단 뒤)")
	return 0
}

// rfix <요청 ID> <종류> <본문>: 고정본을 배타 생성(이미 있으면 그것이 확정본)하고 장부 deliver(종류·해시·크기). 0 됨.
// hasBody가 거짓이면 본문 없음(셸 판 「-」 — 고정본이 이미 있을 때만 부른다)
func rfix(rid, kind, body string, hasBody bool) int {
	if !qHeld {
		rc := 0
		if w := withQ(func() { rc = rfix(rid, kind, body, hasBody) }); w != 0 {
			return w
		}
		return rc
	}
	row := lget(rid)
	if row.F(4) != "open" {
		return 0
	}
	if os.MkdirAll(DATA+"/replies", 0o755) != nil {
		return 1
	}
	rf := DATA + "/replies/" + rid + ".md"
	if !exists(rf) {
		t, err := os.CreateTemp(DATA+"/replies", ".fix.")
		if err != nil {
			return 1
		}
		data := ""
		if hasBody {
			data = body + "\n"
		}
		_, werr := t.WriteString(data)
		if serr := t.Sync(); werr == nil {
			werr = serr
		}
		if cerr := t.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil || lset(rid, "5", kind) != 0 || os.Rename(t.Name(), rf) != nil {
			os.Remove(t.Name())
			return 1
		}
	} else {
		kind = row.F(5)
		if kind == "-" {
			last := lastLine(func() string { s, _ := readFile(rf); return s }())
			switch {
			case strings.HasPrefix(last, "결과: 실패 취소 — hc-fill rm "):
				kind = "cancel"
			case last == "결과: 실패 보고 없음" || last == "결과: 실패 보고 불완전":
				kind = "noreport"
			default:
				kind = "result"
			}
		}
	}
	crash("after-reply-fix")
	b, err := os.ReadFile(rf)
	if err != nil {
		return 1
	}
	return lset(rid, "4", "deliver", "5", kind, "6", sha256hex(b), "7", strconv.Itoa(len(b)))
}

// q_rmx <이름> <상태>: 그 줄이 그 상태일 때만 뺀다(잠금 안). 없으면 1
func qRmx(n, s string) int {
	text, ok := readFile(Q)
	if !ok {
		return 1
	}
	var out []Row
	found := false
	for _, l := range lines(text) {
		r := splitRow(l)
		if r.F(1) == n && r.F(6) == s {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return 1
	}
	if writeAtomic(Q, rowsText(out)) != nil {
		return 1
	}
	return 0
}

// q_inadd: 받은편지 요청을 대기열 줄과 장부 open으로 한 잠금에서(멱등). 「ADDED」·「HAD」, 실패면 rc
func qInadd(n, p, brief, after, only, prio, dest, rid string) (string, int) {
	ia := ""
	rc := withQ(func() {
		switch lget(rid).F(4) {
		case "", "open":
		default:
			ia = "HAD"
			return
		}
		ex, found := "", false
		for _, r := range qRows() {
			if r.F(1) == n {
				ex, found = dash(r.F(11)), true
				break
			}
		}
		if found {
			if ex != dest+"#"+rid {
				die("이미 대기열에 있다(다른 요청): %s", n)
			}
			ia = "HAD"
		} else {
			if appendLine(Q, pad16(n, p, brief, after, only, "wait", "-", "-", "-", nowMin(), dest+"#"+rid, "-", prio).Line()) != nil {
				exit(1)
			}
			ia = "ADDED"
		}
		if lget(rid) == nil {
			if appendLine(LED, strings.Join([]string{rid, n, dest, "open", "-", "-", "-", "-", nowMin()}, "\t")) != nil {
				exit(1)
			}
		}
	})
	return ia, rc
}

// q_rm <이름>: 받은편지 줄은 결과가 아직 없으면 취소문을 고정해 요청자에게 간다(잠금 안)
func qRm(n string) {
	if !qHasNameW(n) {
		die("대기열에 없다: %s", n)
	}
	if rmigrate() != 0 {
		die("받은편지 줄 옮기기 실패")
	}
	var rc11 string
	for _, r := range qRows() {
		if r.F(1) == n {
			rc11 = r.F(11)
			break
		}
	}
	if _, rrid, ok := replyOf(rc11); ok {
		if lget(rrid).F(4) == "open" && rfix(rrid, "cancel", "결과: 실패 취소 — hc-fill rm "+nowMin(), true) != 0 {
			die("취소 결과를 두지 못했다: %s", n)
		}
		crash("rm-after-cancel")
	}
	var out []Row
	for _, r := range qRowsW() {
		if r.F(1) != n {
			out = append(out, r)
		}
	}
	if writeAtomic(Q, rowsText(out)) != nil {
		exit(1)
	}
}

// anote <레인>: ls 비고의 자동 감사 상태(그 레인의 마지막 감사 차수)
func anote(n string) string {
	fin := conf("audit_final")
	if _, ok := readFile(DATA + "/audit.tsv"); !ok {
		return "" // 셸 판: awk가 파일을 못 열면 빈 글
	}
	var r, s, f, v string
	g := false
	for _, x := range readRows(DATA + "/audit.tsv") {
		if x.NF() >= 17 && x.F(2) == n {
			r, s, f, v, g = x.F(3), x.F(8), x.F(9), x.F(10), true
		} else if x.NF() >= 5 && x.NF() < 17 && x.F(1) == n {
			r, f, s, g = x.F(3), x.F(4), "old", true
		}
	}
	switch {
	case !g:
		return "자동 감사(완료 기다림)"
	case s == "opening" || s == "queued":
		return "감사 중(" + r + "번째)"
	case s == "deliver":
		if v == "pass" {
			return "감사 통과 — 수거 기다림"
		}
		return "감사 지적 " + f + "번째 — 수거 기다림"
	case s == "taken":
		if v == "pass" {
			if fin != "" {
				return "감사 통과 받음(최종 대기)"
			}
			return "감사 통과 받음"
		}
		return "감사 지적 " + f + "번째"
	case s == "superseded":
		return "새 완료 — 다음 감사 기다림"
	case s == "nosha" || s == "escalate:nosha":
		return "지휘로 넘김(완료 sha 없음)"
	case s == "coord" || strings.HasPrefix(s, "escalate:"):
		return "지휘로 넘김"
	}
	return "옛 감사 기록"
}

var thenHeadRe = regexp.MustCompile(`^## 이어서: ([a-z0-9_-]*) `)

// thenNames <지시문>: 그 지시문의 「## 이어서: <이름> 」 이름들
func thenNames(path string) []string {
	var o []string
	for _, l := range readLines(path) {
		if m := thenHeadRe.FindStringSubmatch(l); m != nil {
			o = append(o, m[1])
		}
	}
	return o
}
