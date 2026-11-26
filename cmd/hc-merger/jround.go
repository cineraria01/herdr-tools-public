package main

// 일지에 바퀴마다 흔적(merger-journal-rounds, 2026-11-23 — 지휘가 일지만 보고 「줄이 있는데 병합기가 쉰 틈」으로 잘못 읽었다: 그 틈은
// 모두 일지에 안 남는 바퀴였다). 검증을 돌았거나 줄을 붙잡은 바퀴는 착지·돌려보냄이 아니어도 결과 칸에 아래 값으로 구성원마다 한 줄
// (jrec과 같은 12칸). 같은 바퀴의 줄은 7칸(바퀴 시작)·8칸(바퀴 끝)이 같다 — 착지를 불렀으면 착지 시작·끝, 아니면 이 바퀴 시작 ~ 지금.
// 추측 기다림은 한 번의 기다림(맨 앞 제출이 같은 바퀴들 — 뒤 구성원은 견주지 않는다)을 한 줄로: $mj/specwait에 모았다가, 추측 판정이
// 기다림이 아니면 그 자리에서(그 바퀴의 착지·반송 줄보다 먼저) 적는다. 판정 전에 끝난 바퀴는 고른 맨 앞이 바뀌었을 때만 적는다.
// 쓰기 실패는 경고만 — 바퀴의 판정·종료를 바꾸지 않는다.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	jBatchFail = "묶음 실패"    // 묶음 착지 실패(일시 실패 아님) — 반송 없이 마지막을 빼고 다시(cap)
	jLease     = "lease 바뀜" // 착지 종료 6 — 후보를 버림
	jTrans     = "일시 실패"    // 기다림 상한·push 거절 횟수·원격 받기 실패 등 — 큐에 두고 다음 바퀴에(3번까지)
	jFront     = "맨 앞 충돌"   // 묶음 맨 앞을 기준 위로 다시 얹으면 충돌 — 착지 없이 묶음 크기 1로
	jSpecWait  = "추측 기다림"   // 맨 앞 추측이 검증 기계에서 도는 중 — 착지 없이 쉼(한 번의 기다림 = 한 줄, 까닭에 되풀이 횟수)
)

// jSolo: 묶음 실패 뒤 앞 k−1을 직접 검증하는 동안 맨 앞 혼자를 검증 기계에 맡긴 바퀴(merger-batch-culprit)
const jSolo = "맨 앞 혼자 추측"

// jround: 지금 묶음(m.b)의 구성원마다 한 줄. what = 무슨 바퀴였나, tail = 구성원 뒤에 붙일 근거(로그·첫 실패 줄)
func (m *M) jround(result, what, tail string) {
	t0, t1 := m.ls0, m.ls1
	if t0 == 0 {
		t0, t1 = m.r0, time.Now().Unix()
	}
	m.jmembers(m.b, result, what, tail, t0, t1)
}

func (m *M) jmembers(b []entry, result, what, tail string, t0, t1 int64) {
	labs := make([]string, len(b))
	for i, e := range b {
		labs[i] = e.l
		if t := m.stackTag(e.s); t != "" {
			labs[i] += "(" + t + ")"
		}
	}
	why := what + " — 구성원 " + strings.Join(labs, "+") + tail
	for _, e := range b {
		mt, ok := readMeta(e.m, e.s)
		if !ok {
			mt = meta{id: e.d}
		}
		m.jline(mt, strings.TrimPrefix(e.q, "refs/merge-queue/"), e.l, result, "-", why, t0, t1, fmt.Sprint(len(b)))
	}
}

// 실패한 시험의 첫 줄(go test 「--- FAIL」·「FAIL」, bun 「(fail)」)
var failLine = regexp.MustCompile(`^\s*(--- FAIL|FAIL\b)|\(fail\)`)

// 시험 실패로 세지 않는 줄(merger-culprit-env-gap, 2026-11-23): 꾸러미를 못 불러온 go test 「FAIL <꾸러미> [setup failed]」(네트워크·모듈
// 캐시)와 맨 「FAIL」(요약 — 실패한 꾸러미가 있으면 그 줄이 따로 있다). 「[build failed]」는 제출이 컴파일을 깨뜨린 것이라 센다
var envFail = regexp.MustCompile(`^\s*FAIL\s*$|^\s*FAIL\s+\S+\s+\[setup failed\]\s*$`)

// runTail: 착지 로그 경로와 첫 근거(실패 줄 첫 줄 — 없으면 까닭 앞의 종료 코드가 근거). 일지 근거라 환경 실패 줄도 그대로 보인다
func (m *M) runTail() string {
	t := ", 로그 " + m.lgf
	if l := scanFail(m.lgf, 0, false); l != "" {
		return t + ", 첫 실패 「" + l + "」"
	}
	return t
}

// firstFail: 추측 검증의 시험 실패 근거 — 로그의 off바이트 뒤에서 실패한 시험 첫 줄. 환경 실패 줄(envFail)과, go test 시간 넘김
// (「panic: test timed out」 — 느린 기계와 가를 수 없다) 뒤 그 꾸러미의 「FAIL」 줄은 세지 않는다. 없거나 못 읽으면 빈 값(모름 — 병합기가 직접 검증)
func firstFail(path string, off int64) string { return scanFail(path, off, true) }

// scanFail: 로그의 off바이트 뒤에서 실패 줄 첫 줄(한 줄로 펴 120바이트 안). test면 firstFail의 규칙으로 건너뛴다. 없거나 못 읽으면 빈 값
func scanFail(path string, off int64, test bool) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return ""
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return pickFail(lines, firstLine(lines, test, false), path, test)
}

// firstLine: 옛 규칙의 첫 실패 줄. noTop이면 「--- FAIL」 줄(시험 이름 줄)과 맨 「FAIL」(요약)은 건너뛴다(꾸러미 줄만)
func firstLine(lines []string, test, noTop bool) string {
	timedOut := false
	for _, l := range lines {
		if noTop && (strings.HasPrefix(strings.TrimSpace(l), "--- FAIL") || strings.TrimSpace(l) == "FAIL") {
			continue
		}
		if test {
			switch {
			case strings.HasPrefix(l, "panic: test timed out"):
				timedOut = true
				continue
			case envFail.MatchString(l):
				continue
			case timedOut && strings.HasPrefix(l, "FAIL\t"):
				timedOut = false
				continue
			}
		}
		if failLine.MatchString(l) {
			return bcut(120, flat(strings.TrimSpace(l)))
		}
	}
	return ""
}

// specWaitNote: 이 바퀴가 맨 앞 추측을 기다렸다 — 모아 둔 기다림과 맨 앞이 같으면 횟수만 늘리고, 다르면 앞 기다림을 적고 새로 센다
func (m *M) specWaitNote() {
	if st, err := os.Stat(m.lgf); err == nil && st.Size() == 0 {
		os.Remove(m.lgf) // 기다린 바퀴의 빈 착지 로그(후보를 쌓은 git -q) — 바퀴마다 하나씩 쌓였다(2026-11-25 한 저장소에 74개)
	}
	now := time.Now().Unix()
	first, n, re := now, 0, ""
	// 「*<기계>」 = 병합 검증 자리로 맡김(specrun.go — 맡긴 바퀴만 안다). 같은 기다림의 다음 바퀴들은 같은 기계면 그 표시를 잇는다
	wm, gate := strings.CutPrefix(m.wmach, "*")
	if f, l, k, b, mach, ok := m.specWaitRead(); ok {
		if sameFront(b, m.b) {
			first, n = f, k
			var was string
			was, re, _ = strings.Cut(mach, "/")
			gate = gate || wm != "" && was == "*"+wm
		} else {
			m.specWaitLine(f, l, k, b, mach)
		}
	}
	if m.wre != "" { // 바쁨으로 다시 고름 — 사슬로 잇는다(v1→v2, 또 바쁨이면 v1→v2→v3)
		if re == "" {
			re = m.wre
		} else {
			_, to, _ := strings.Cut(m.wre, "→")
			re += "→" + to
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %d %d", first, now, n+1)
	if wm != "" {
		sb.WriteString(" ")
		if gate {
			sb.WriteString("*")
		}
		sb.WriteString(wm)
		if re != "" {
			sb.WriteString("/" + re) // 「<기계>/<다시 고른 사슬>」 — 옛 판은 「/」가 없다
		}
	}
	sb.WriteString("\n")
	for _, e := range m.b {
		fmt.Fprintf(&sb, "%s %s %s %s\n", e.q, e.s, e.l, e.m)
	}
	if err := writeVia(m.mj+"/specwait", sb.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 기다림을 적지 못했다(일지 줄만 빠진다): %v\n", err)
	}
}

// specWaitEnd: 추측 판정이 기다림이 아닐 때 바로(그 바퀴의 착지·반송 줄보다 먼저) — 모아 둔 기다림이 있으면 한 줄로 적고 지운다
func (m *M) specWaitEnd() {
	if f, l, k, b, mach, ok := m.specWaitRead(); ok {
		m.specWaitLine(f, l, k, b, mach)
		removeQuiet(m.mj + "/specwait")
		removeQuiet(m.mj + "/specshard") // 그 기다림의 나눔 상태(일지 줄에 적었다)
	}
}

// specWaitEarly: 추측 판정 전에 끝난 바퀴(round의 defer) — 고른 묶음의 맨 앞이 모아 둔 기다림과 다를 때만(그 기다림은 끝났다) 적는다.
// 묶음을 고르지 못한 바퀴(원격 실패·꺼짐 등)나 맨 앞이 같은 바퀴는 기다림을 건드리지 않는다(감사 atoy0ft-aprr7a — 한 기다림이 두 줄로 쪼개졌다)
func (m *M) specWaitEarly() {
	if len(m.sfront) == 0 {
		return
	}
	if _, _, _, b, _, ok := m.specWaitRead(); ok && !sameFront(b, m.sfront) {
		m.specWaitEnd()
	}
}

// specWaitLine: 한 번의 기다림 = 한 줄. mach = 마지막 바퀴에 맨 앞을 맡긴 검증 기계(「검증 맡김(<기계>) <기다린 초>초」 —
// merger-verify-dispatch), 뒤에 「/<사슬>」이 있으면 「바쁨으로 다시 고름(<사슬>)」(merger-verifier-pick),
// -면 손잡이 0으로 맡기지 못해 기다림, 빈 값이면 옛 기록(덧붙이지 않음)
func (m *M) specWaitLine(first, last int64, n int, b []entry, mach string) {
	what := fmt.Sprintf("추측 검증 기다림 %d바퀴", n)
	mach, re, _ := strings.Cut(mach, "/")
	mach, gate := strings.CutPrefix(mach, "*")
	switch mach {
	case "":
	case "-":
		what += ", 맡길 기계를 기다림(직접 검증 안 함)"
	default:
		what += fmt.Sprintf(", 검증 맡김(%s) %d초", mach, last-first)
		if gate { // 한가한 기계가 아니라 병합 검증 자리로 맡김(verifierpick.go) — --why가 따로 센다
			what += ", 병합 검증 자리(" + mach + ")"
		}
	}
	if re != "" {
		what += ", 바쁨으로 다시 고름(" + re + ")"
	}
	if len(b) > 0 {
		what += m.shardNote(b[0].s, last) // 나눠 돈 맨 앞 — 「, 나눔 K(<기계>,…), 마무리 <기계> n초」(specshards.go)
	}
	m.jmembers(b, jSpecWait, what, "", first, last)
}

// specWaitRead: 「<처음 초> <마지막 초> <횟수> [[*]<검증 기계|->[/<사슬>]]」(* = 병합 검증 자리로 맡김) + 구성원마다 「<큐 참조> <제출 SHA> <레인> <기록 SHA|->」(cur와 같은 꼴).
// 없으면 ok=false, 깨졌으면 지우고 ok=false(줄 하나가 빠질 뿐)
func (m *M) specWaitRead() (first, last int64, n int, b []entry, mach string, ok bool) {
	p := m.mj + "/specwait"
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	ls := strings.Split(strings.TrimSpace(string(data)), "\n")
	h := strings.Fields(ls[0])
	var e1, e2, e3 error
	hok := len(h) == 3 || len(h) == 4
	if hok {
		first, e1 = strconv.ParseInt(h[0], 10, 64)
		last, e2 = strconv.ParseInt(h[1], 10, 64)
		n, e3 = strconv.Atoi(h[2])
		if len(h) == 4 {
			mach = h[3]
		}
	}
	for _, l := range ls[1:] {
		if f := strings.Fields(l); len(f) == 4 {
			b = append(b, entry{q: f[0], s: f[1], l: f[2], m: f[3]})
		}
	}
	if !hok || e1 != nil || e2 != nil || e3 != nil || len(b) == 0 || len(b) != len(ls)-1 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 기다림 기록 %s 이 깨졌다 — 버린다(일지 줄 하나가 빠진다)\n", p)
		removeQuiet(p)
		return 0, 0, 0, nil, "", false
	}
	return first, last, n, b, mach, true
}

// sameFront: 같은 기다림인가 — 맨 앞 제출(큐 참조·SHA)만 본다(뒤 구성원이 바뀌어도 같은 맨 앞을 기다린다)
func sameFront(a, b []entry) bool {
	return len(a) > 0 && len(b) > 0 && a[0].q == b[0].q && a[0].s == b[0].s
}
