package main

// 일지에 바퀴마다 흔적(merger-journal-rounds, 2026-11-23 — 지휘가 일지만 보고 「줄이 있는데 병합기가 쉰 틈」으로 잘못 읽었다: 그 틈은
// 모두 일지에 안 남는 바퀴였다). 검증을 돌았거나 줄을 붙잡은 바퀴는 착지·돌려보냄이 아니어도 결과 칸에 아래 값으로 구성원마다 한 줄
// (jrec과 같은 12칸). 같은 바퀴의 줄은 7칸(바퀴 시작)·8칸(바퀴 끝)이 같다 — 착지를 불렀으면 착지 시작·끝, 아니면 이 바퀴 시작 ~ 지금.
// 추측 기다림은 한 번의 기다림(같은 구성원이 이어 기다린 바퀴들)을 한 줄로: $mj/specwait에 모았다가 기다리지 않은 바퀴가 끝날 때 적는다.
// 쓰기 실패는 경고만 — 바퀴의 판정·종료를 바꾸지 않는다.

import (
	"bufio"
	"fmt"
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

// runTail: 착지 로그 경로와 첫 근거(실패한 시험 첫 줄 — 없으면 까닭 앞의 종료 코드가 근거)
func (m *M) runTail() string {
	t := ", 로그 " + m.lgf
	f, err := os.Open(m.lgf)
	if err != nil {
		return t
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if l := sc.Text(); failLine.MatchString(l) {
			return t + ", 첫 실패 「" + bcut(120, flat(strings.TrimSpace(l))) + "」"
		}
	}
	return t
}

// specWaitNote: 이 바퀴가 맨 앞 추측을 기다렸다 — 모아 둔 기다림과 구성원이 같으면 횟수만 늘리고, 다르면 앞 기다림을 적고 새로 센다
func (m *M) specWaitNote() {
	m.swait = true
	now := time.Now().Unix()
	first, n := now, 0
	if f, l, k, b, ok := m.specWaitRead(); ok {
		if sameMembers(b, m.b) {
			first, n = f, k
		} else {
			m.specWaitLine(f, l, k, b)
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %d %d\n", first, now, n+1)
	for _, e := range m.b {
		fmt.Fprintf(&sb, "%s %s %s %s\n", e.q, e.s, e.l, e.m)
	}
	if err := writeVia(m.mj+"/specwait", sb.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 기다림을 적지 못했다(일지 줄만 빠진다): %v\n", err)
	}
}

// specWaitEnd: 기다리지 않은 바퀴가 끝날 때(round의 defer) — 모아 둔 기다림이 있으면 한 줄로 적고 지운다
func (m *M) specWaitEnd() {
	if m.swait {
		return
	}
	if f, l, k, b, ok := m.specWaitRead(); ok {
		m.specWaitLine(f, l, k, b)
		removeQuiet(m.mj + "/specwait")
	}
}

func (m *M) specWaitLine(first, last int64, n int, b []entry) {
	m.jmembers(b, jSpecWait, fmt.Sprintf("추측 검증 기다림 %d바퀴", n), "", first, last)
}

// specWaitRead: 「<처음 초> <마지막 초> <횟수>」 + 구성원마다 「<큐 참조> <제출 SHA> <레인> <기록 SHA|->」(cur와 같은 꼴). 없으면 ok=false,
// 깨졌으면 지우고 ok=false(줄 하나가 빠질 뿐)
func (m *M) specWaitRead() (first, last int64, n int, b []entry, ok bool) {
	p := m.mj + "/specwait"
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	ls := strings.Split(strings.TrimSpace(string(data)), "\n")
	h := strings.Fields(ls[0])
	var e1, e2, e3 error
	if len(h) == 3 {
		first, e1 = strconv.ParseInt(h[0], 10, 64)
		last, e2 = strconv.ParseInt(h[1], 10, 64)
		n, e3 = strconv.Atoi(h[2])
	}
	for _, l := range ls[1:] {
		if f := strings.Fields(l); len(f) == 4 {
			b = append(b, entry{q: f[0], s: f[1], l: f[2], m: f[3]})
		}
	}
	if len(h) != 3 || e1 != nil || e2 != nil || e3 != nil || len(b) == 0 || len(b) != len(ls)-1 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 추측 기다림 기록 %s 이 깨졌다 — 버린다(일지 줄 하나가 빠진다)\n", p)
		removeQuiet(p)
		return 0, 0, 0, nil, false
	}
	return first, last, n, b, true
}

func sameMembers(a, b []entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].q != b[i].q || a[i].s != b[i].s {
			return false
		}
	}
	return true
}
