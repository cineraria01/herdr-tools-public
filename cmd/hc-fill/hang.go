package main

// 멈춘 레인(2026-11-25 ht-lane-hang-detect, docs/hc-fill/hang.md — 한 레인이 「Ionizing… (20m 39s)」로 도구 호출 없이 20분 넘게
// 멈췄고, 그 앞에는 「1 shell still running」을 기다리는데 검증 프로세스가 없어 1시간 넘게 아무도 몰랐다). run 바퀴가 stallTick 안에서 부른다.
// (a) working인데 진행 표시 줄 「… (Nm Ns」이 hang_min분 넘고 그 위 화면이 hang_min분 그대로이며 도구 셸이 없음 → 알림, hang_esc면
// Escape 한 번 + 이어 가라는 글(한 레인에 한 시간에 한 번) (b) 마지막 턴 끝 줄의 「N shell still running」을 기다리는데(idle·done) 그 터미널 아래
// 도구 셸이 hangShellGrace 넘게 없음 → 알림만. 대상은 떠 있는 대기열 줄의 주소에 그 레인 이름 터미널이 있을 때만(감사 줄 빼고) — 사람·병합기
// 터미널은 대기열에 없고, 주소가 재사용되면 이름이 달라 빠진다

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// hangShellMark: Claude Code의 Bash 도구 셸(포그라운드·배경 모두)이 명령 줄에 담는 글자 — hc-lane lane.wait_procs 기본과 같다.
// 에이전트 자손 가운데 MCP 서버 등은 작업 트리를 cwd로 써도 도구 실행이 아니라 cwd 대신 이것으로 센다
const hangShellMark = "shell-snapshots"

// ponytail: 고정 3분 — 셸이 막 끝나 에이전트가 깨기 직전을 멈춤으로 보지 않을 만큼. 헛알림이 나면 손잡이로
const hangShellGrace = 3 * 60

var (
	hangSpinRe  = regexp.MustCompile(`^\s*\S+\s+\S.*…\s*\((?:(\d+)h\s*)?(?:(\d+)m\s*)?(\d+)s\b`)
	hangShellRe = regexp.MustCompile(`(\d+)\s+shells?\s+still running`)
	// 턴 끝 줄 머리 — hc-lane lane.wait_line 기본과 같다
	hangTurnRe = regexp.MustCompile(`^\s*(✻|✶|✳|✢|✽)`)
	hangGap    = 2 * time.Second // Escape 뒤 글을 보내기까지
)

// hangSpin <화면>: 마지막 진행 표시 줄의 분과 그 위 화면(끝 빈칸 뗌). 없으면 ok=false
func hangSpin(scr string) (int, string, bool) {
	ls := strings.Split(scr, "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		m := hangSpinRe.FindStringSubmatch(ls[i])
		if m == nil {
			continue
		}
		return atoi10(m[1])*60 + atoi10(m[2]), strings.TrimRight(strings.Join(ls[:i], "\n"), " \n"), true
	}
	return 0, "", false
}

// hangShellWait <화면>: 마지막 턴 끝 줄에 「N shell still running」이 있나 — 옛 턴 줄·본문 인용은 화면에 남아도 기다림이 아니다
// (감사 atp1rzf 중간, hc-lane wait와 같은 방식)
func hangShellWait(scr string) bool { return hangShellRe.MatchString(hangLastTurn(scr)) }

// hangLastTurn <화면>: 마지막 턴 끝 줄(없으면 빈 값)
func hangLastTurn(scr string) string {
	ls := strings.Split(scr, "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if hangTurnRe.MatchString(ls[i]) {
			return ls[i]
		}
	}
	return ""
}

// hangFP <화면 윗부분>: 바뀜 비교용 지문
func hangFP(body string) string { return head(sha256hex([]byte(body)), 16) }

// hangShells <기계> <터미널 pid>: 그 터미널 프로세스 자손 가운데 도구 셸 수. ok=false면 모름(ps·hc exec 실패)
func hangShells(m string, pid int) (int, bool) {
	o, rc := onm(nil, m, `o=$(ps -A -ww -o pid=,ppid=,command=) || exit 3
printf '%s\n' "$o" | awk -v r="$1" -v w="$2" '{p[$1]=$2} index($0, w) {h[$1]=1}
END {n=0; for (k in h) {q=p[k]; for (i=0; q>1 && i<10000; i++) {if (q==r) {n++; break}; q=p[q]}}; print n}'`, "~", strconv.Itoa(pid), hangShellMark)
	if v := strings.TrimSpace(o); rc == 0 && isDigits(v) {
		return atoi10(v), true
	}
	return 0, false
}

// hangTick <지금>: 떠 있는 대기열 줄마다 (a)·(b)를 보고 의심 줄을 <DATA>/hang.tsv(레인·주소·종류·처음 본 초·지문)에 다시 적는다
func hangTick(now time.Time) {
	lim := knobInt("hang_min")
	if lim == 0 || dry || !nonEmpty(Q) {
		return
	}
	prev := map[string]Row{}
	for _, r := range readRows(DATA + "/hang.tsv") {
		prev[r.F(hgLane)] = r
	}
	var keep []string
	for _, q := range qRows() {
		n, a, m := q.F(qName), q.F(qAddr), q.F(qMach)
		if q.F(qState) != "up" || strings.HasPrefix(q.F(qAudit), "audit=") || n == "" || dash(a) == "-" || !lalive(a, n) {
			continue
		}
		if m != LOCAL && !connHas(m) {
			continue
		}
		if r := hangLane(q, alive(a), prev[n], now, lim); r != nil {
			keep = append(keep, r.Line())
		}
	}
	if err := writeAtomic(DATA+"/hang.tsv", strings.Join(keep, "\n")+"\n"); err != nil {
		say("알림: 멈춘 레인 기록을 못 썼다(%v) — 다음 바퀴에 처음부터 센다", err)
	}
}

// hangLane: 한 레인 — 의심이면 hang.tsv 줄(아니면 nil), 문턱을 넘고 도구 셸이 없으면 알림(키로 한 번)
func hangLane(q Row, st string, p Row, now time.Time, lim int) Row {
	n, a, m := q.F(qName), q.F(qAddr), q.F(qMach)
	if st != "working" && st != "idle" && st != "done" {
		return nil // (a)는 working, (b)는 idle·done에서만 — 그 밖에는 화면을 읽지 않는다(감사 atp1rzf 낮음)
	}
	scr, rc := run("hc", "read", a)
	if rc != 0 {
		return p // 모름 — 지난 기록 그대로
	}
	kind, fp, spin := "", "-", 0
	if st == "working" {
		if mins, body, ok := hangSpin(scr); ok {
			kind, fp, spin = "a", hangFP(body), mins
		}
	} else if hangShellWait(scr) {
		kind = "b"
	}
	if kind == "" {
		return nil
	}
	since := now.Unix()
	if p != nil && p.F(hgAddr) == a && p.F(hgKind) == kind && p.F(hgFP) == fp && isDigits(p.F(hgSince)) {
		since = int64(atoi10(p.F(hgSince)))
	}
	row := Row{n, a, kind, strconv.FormatInt(since, 10), fp}
	age := int(now.Unix()-since) / 60
	if kind == "a" && (age < lim || spin < lim) {
		return row
	}
	t, found, err := hcGet(a)
	if err != nil || !found || t.Name != n || t.PID <= 0 {
		return row
	}
	sh, ok := hangShells(m, t.PID)
	switch {
	case !ok:
		return row
	case sh > 0 && kind == "b":
		return nil // 배경 셸이 살아 있다 — 기다리는 것이 맞다
	case sh > 0:
		return row // 도구가 도는 중(긴 검증 등) — 생각 중 멈춤이 아니다
	case kind == "b" && now.Unix()-since < hangShellGrace:
		return row
	}
	own := dash(q.F(qOwner))
	if _, _, inbox := replyOf(q.F(qReply)); inbox {
		own = "-"
	}
	key := fmt.Sprintf("hf:%s:hang-%s:%d", n, kind, since)
	if kind == "b" {
		aesc(n, own, fmt.Sprintf("%s 멈춘 레인 — 「%s」을 기다리는데 그 터미널 아래 도구 셸 프로세스가 없다(%d분째, %s) — 배경 작업이 소리 없이 죽었다. 다음: hc read %s 로 보고 hc send %s '배경 작업이 끝나지 않고 사라졌다 — 다시 돌려라'",
			n, hangShellRe.FindString(hangLastTurn(scr)), age, a, a, a), key)
		return row
	}
	did := hangEsc(n, a, spin, now)
	aesc(n, own, fmt.Sprintf("%s 멈춘 레인 — 「생각 중」 표시 %d분째, 그 위 화면 %d분 그대로·도구 셸 없음(%s) — %s. 다음: hc read %s", n, spin, age, a, did, a), key)
	return row
}

// hangEsc: hang_esc가 켜져 있고 그 레인에 한 시간 안에 끊은 적이 없으면 Escape + 이어 가라는 글 — 한 일을 글로
func hangEsc(n, a string, spin int, now time.Time) string {
	if knobInt("hang_esc") == 0 {
		return "끊지 않음(손잡이 hang_esc 0)"
	}
	var keep []string // 한 시간 안의 줄만 남긴다(감사 atp1rzf 낮음 — 더하기만 하던 파일)
	for _, r := range readRows(DATA + "/hang-esc.tsv") {
		if !isDigits(r.F(nuAt)) || now.Unix()-int64(atoi10(r.F(nuAt))) >= 3600 {
			continue
		}
		if r.F(nuLane) == n {
			return "한 시간 안에 이미 끊음 — 이번에는 끊지 않음"
		}
		keep = append(keep, r.Line())
	}
	// 보내기 전에 적는다 — 보냈는지 모를 때(시간 넘김) 바퀴마다 다시 끊지 않게
	keep = append(keep, Row{n, a, strconv.FormatInt(now.Unix(), 10)}.Line())
	if err := writeAtomic(DATA+"/hang-esc.tsv", strings.Join(keep, "\n")+"\n"); err != nil {
		return fmt.Sprintf("끊지 않음(기록 실패: %v)", err)
	}
	if _, rc := run("hc", "keys", a, "esc"); rc != 0 {
		jotBy("hc-fill 스스로", "hang-esc", n, "생각 중 멈춤", "실패", "-", "주소 "+a)
		return "Escape 보내기 실패"
	}
	time.Sleep(hangGap)
	msg := fmt.Sprintf("앞 요청이 %d분 멈춰 끊었다 — 마지막 지시를 이어서 하라", spin)
	if !auSend(a, msg) {
		jotBy("hc-fill 스스로", "hang-esc", n, "생각 중 멈춤", "글 실패", "-", "주소 "+a)
		return "Escape는 보냈고 이어 가라는 글은 실패 — hc send로 다시"
	}
	jotBy("hc-fill 스스로", "hang-esc", n, "생각 중 멈춤", "보냄", "-", "주소 "+a)
	return "Escape 한 번과 「" + msg + "」를 보냄"
}
