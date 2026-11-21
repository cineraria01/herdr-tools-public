package main

// 결과 정리(bash mdone·mfail·mfretry·mjrec·mjonce): 원격 참조 정리 → 병합기 일지 → 보낼 함(out/) → 사건·레인 알림(notify.go).

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
)

// 사건 줄과 같은 다듬기(internal/events)
var (
	utc  = events.UTC
	flat = events.Flat
	bcut = events.Bcut
)

// awkNum: awk의 「문자열+0」 — 앞의 숫자만, 없으면 0
func awkNum(s string) float64 {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	return n
}

func dashIf(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// jrec: 병합기 일지 …/hc-lane/merger-journal.tsv에 제출마다 한 줄(12칸): 적은 시각 · 큐 이름 · 레인 · 제출 ID · 작업 ID · 제출 시각 ·
// 묶음 크기 · 착지 시작 · 착지 끝 · 결과(착지|돌려보냄) · 후보 SHA(돌려보냄은 -) · 까닭. 한 번 O_APPEND(1000바이트 안)
func (m *M) jrec(mt meta, n, ln, result, cand, why string) bool {
	t0, t1, sub := "-", "-", ""
	if m.ls0 != 0 {
		t0 = utc(time.Unix(m.ls0, 0))
	}
	if m.ls1 != 0 {
		t1 = utc(time.Unix(m.ls1, 0))
	}
	head, _, _ := strings.Cut(n, "-")
	if sec, err := strconv.ParseInt(head, 10, 64); err == nil {
		sub = utc(time.Unix(sec, 0))
	}
	line := strings.Join([]string{utc(time.Now()), n, ln, dashIf(mt.id), dashIf(mt.job), sub, dashIf(m.mbn), t0, t1, result, cand, bcut(300, flat(why))}, "\t") + "\n"
	f, err := os.OpenFile(gEvdir+"/merger-journal.tsv", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err == nil {
		_, err = f.WriteString(line)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 병합기 일지를 쓰지 못했다")
		return false
	}
	return true
}

// jonce: 같은 큐 이름·제출 ID·결과의 일지 줄이 없을 때만 적는다(끊긴 실행을 다음 바퀴가 다시 정리해도 한 줄). 읽기 실패면 false
func (m *M) jonce(mt meta, n, ln, result, cand, why string) bool {
	path := gEvdir + "/merger-journal.tsv"
	if _, err := os.Stat(path); err == nil {
		f, err := os.Open(path)
		if err != nil {
			return false
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			c := strings.Split(sc.Text(), "\t")
			if len(c) >= 10 && c[1] == n && c[3] == dashIf(mt.id) && c[9] == result {
				return true
			}
		}
		if sc.Err() != nil {
			return false
		}
	}
	return m.jrec(mt, n, ln, result, cand, why)
}

// done <큐> <제출 SHA> <레인> <기록 SHA|-> <들어간 후보 C> <까닭> [claim]: 기본 브랜치가 C를 담은 뒤의 정리 — 한 번의 원자 push로,
// 원격에 지금 남은 것만: 기록을 refs/merge-done/<레인>으로, 큐 표식·lane 가지·기록은 그 제출 값일 때만 lease로 지우고, 제출 ID·작업
// 표식이 없으면 C로, 레인 표식 refs/hc-landed/<레인>/<초>. 정리가 확인된 뒤에만 결과 사건을 보낼 함에. 0 끝 · 1 망 오류·거절(일지를
// 두고 다음 바퀴) · 2 claim인데 큐가 이 제출 것이 아님
func (m *M) done(q, s, ln, rec, C, why string, claim bool) int {
	n := strings.TrimPrefix(q, "refs/merge-queue/")
	rec = strings.TrimPrefix(rec, "-")
	mt, ok := readMeta(rec, s)
	if !ok {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 제출 기록(%s)을 읽지 못했다 — 다음 바퀴에\n", n)
		return 1
	}
	dn := rec
	if dn == "" {
		dn = C
	}
	sidref, lanep := "", "refs/hc-landed/"+ln+"/"
	if mt.id != "" {
		sidref = "refs/hc-landed/" + mt.id
	}
	qa := []string{"ls-remote", "origin", q, "refs/heads/lane/" + ln, "refs/merge-queue-meta/" + n, "refs/merge-done/" + ln, lanep + "*"}
	if sidref != "" {
		qa = append(qa, sidref)
	}
	if mt.jobref != "" {
		qa = append(qa, mt.jobref)
	}
	rr, rc := out(qa...)
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: 정리할 참조를 보지 못했다(git ls-remote) — 다음 바퀴에 일지로 판정")
		return 1
	}
	tip := refsha(rr, q)
	ownmeta := refsha(rr, "refs/merge-queue-meta/"+n) == rec
	if claim && !(tip == s && ownmeta) {
		return 2
	}
	olddone := refsha(rr, "refs/merge-done/"+ln)
	current := ownmeta && (rec != "" || tip == s)
	var p []string
	if olddone != dn && (olddone == "" || current) {
		p = append(p, dn+":refs/merge-done/"+ln, "--force-with-lease=refs/merge-done/"+ln+":"+olddone)
	}
	if ownmeta {
		if tip == s {
			p = append(p, ":"+q, "--force-with-lease="+q+":"+s)
		}
		if refsha(rr, "refs/heads/lane/"+ln) == s {
			p = append(p, ":refs/heads/lane/"+ln, "--force-with-lease=refs/heads/lane/"+ln+":"+s)
		}
		if rec != "" {
			p = append(p, ":refs/merge-queue-meta/"+n, "--force-with-lease=refs/merge-queue-meta/"+n+":"+rec)
		}
	}
	if sidref != "" && refsha(rr, sidref) == "" {
		p = append(p, C+":"+sidref)
	}
	if mt.jobref != "" && mt.jobref != sidref && refsha(rr, mt.jobref) == "" {
		p = append(p, C+":"+mt.jobref)
	}
	// 레인 표식: 이 제출 뒤(큐 이름의 초 이후)에 C로 둔 것이 이미 있으면 다시 두지 않는다
	head, _, _ := strings.Cut(n, "-")
	have := false
	for _, line := range strings.Split(rr, "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == C && strings.HasPrefix(f[1], lanep) && awkNum(f[1][len(lanep):]) >= awkNum(head) {
			have = true
		}
	}
	if !have {
		p = append(p, fmt.Sprintf("%s:%s%d", C, lanep, time.Now().Unix()))
	}
	if len(p) > 0 && runQ(append([]string{"push", "-q", "--atomic", "--porcelain", "origin"}, p...)...) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: %s 의 큐·lane·기록 참조를 정리하지 못했다(git push) — 다음 바퀴에 일지로 판정\n", ln)
		return 1
	}
	removeQuiet(m.mj + "/" + ln + ".tr")
	if !m.jonce(mt, n, ln, "착지", C, why) {
		return 1
	}
	msg := fmt.Sprintf("%s — 제출 %s, %s%s", why, first8(s), mt.key(n), mt.infoTail())
	if !m.outbox(mt, n, "착지", C, msg, ln) {
		return 1
	}
	m.send(n)
	fmt.Printf("hc-lane merger: %s 착지 %s (%s)\n", ln, first8(C), why)
	return 0
}

// fail <큐> <제출 SHA> <레인> <기록 SHA|-> <로그|-> <까닭>: 큐 표식을 refs/merge-failed/<이름>, 기록을 refs/merge-failed-meta/<이름>으로
// 한 번의 원자 push로 옮기고(lane 가지는 둠) 막힘 사건. 옮기기 전에 할 일을 fail/<이름>에 적어 원격을 못 보면 다음 바퀴에 다시 —
// 그동안 그 제출만 건너뛴다. false = 아직 못 옮김
func (m *M) fail(q, s, ln, rec, lg, why string) bool {
	n := strings.TrimPrefix(q, "refs/merge-queue/")
	fr := "refs/merge-failed/" + n
	rec, lg = strings.TrimPrefix(rec, "-"), strings.TrimPrefix(lg, "-")
	mt, ok := readMeta(rec, s)
	if !ok {
		// 이름 꼴이 틀린 항목은 기록이 맞지 않아도 반송한다(그 큐만 막히지 않게)
		if !strings.HasPrefix(why, "큐 이름 꼴이 아니다") {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 제출 기록(%s)을 읽지 못했다 — 다음 바퀴에\n", n)
			return false
		}
		mt = meta{}
	}
	pending := strings.Join([]string{q, s, ln, dashIf(rec), dashIf(lg), flat(why)}, "\n") + "\n"
	if err := writeVia(m.mj+"/fail/"+n, pending); err != nil {
		die("merger: 돌려보낼 일을 적지 못했다")
	}
	logTail := ""
	if lg != "" {
		logTail = ", 로그 " + lg
	}
	ob := fmt.Sprintf("병합기: %s — 원본 %s, 기준 %s, %s%s, %s 로 옮김(lane/%s 는 그대로)%s", why, first8(s), first8(dashIf(m.B)), mt.key(n), mt.infoTail(), fr, ln, logTail)
	// 옮길 제출 객체가 이 클론에 아직 없으면(큐를 받기 전에 반송 — 이름 꼴) 그 큐 참조를 받는다
	if runQ("cat-file", "-e", s+"^{commit}") != 0 && fetchr(true, "-q", "origin", "+"+q+":refs/hc-merger/q/"+n) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 실패한 제출 %s 를 받지 못했다(git fetch) — 다음 바퀴에 다시\n", q)
		return false
	}
	p := []string{s + ":" + fr, ":" + q, "--force-with-lease=" + fr + ":", "--force-with-lease=" + q + ":" + s}
	qm, fm := "refs/merge-queue-meta/"+n, "refs/merge-failed-meta/"+n
	if rec != "" {
		p = append(p, rec+":"+fm, ":"+qm, "--force-with-lease="+fm+":", "--force-with-lease="+qm+":"+rec)
	}
	if runQ(append([]string{"push", "-q", "--atomic", "--porcelain", "origin"}, p...)...) != 0 {
		rb, rc := out("ls-remote", "origin", q, qm, fr, fm)
		if rc != 0 {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 실패한 제출을 옮겼는지 확인하지 못했다(%s) — 다음 바퀴에 다시\n", q)
			return false
		}
		switch {
		case refsha(rb, fr) == s && refsha(rb, fm) == rec: // 응답만 끊김
		case refsha(rb, q) == s && refsha(rb, qm) == rec:
			fmt.Fprintf(os.Stderr, "hc-lane merger: 실패한 제출 %s 를 %s 로 옮기지 못했다 — 다음 바퀴에 다시\n", q, fr)
			return false
		default: // 철회·다른 병합기의 착지 — 반송 사건을 쓰지 않는다
			if err := os.Remove(m.mj + "/fail/" + n); err != nil && !errors.Is(err, os.ErrNotExist) {
				return false
			}
			return true
		}
	}
	if !m.jonce(mt, n, ln, "돌려보냄", "-", why) || !m.outbox(mt, n, "막힘", "-", ob, ln) {
		return false
	}
	removeQuiet(m.mj + "/fail/" + n)
	removeQuiet(m.mj + "/" + ln + ".tr")
	m.send(n)
	fmt.Fprintf(os.Stderr, "hc-lane merger: %s 실패 — %s → %s\n", ln, why, fr)
	return true
}

// fretry: 옮기다 멈춘 돌려보내기를 다시(못 하면 그 제출만 계속 건너뜀). 깨진 기록은 버린다 — 그 제출은 큐에 그대로라 다시 판정된다
func (m *M) fretry() {
	des, err := os.ReadDir(m.mj + "/fail")
	if err != nil {
		return
	}
	for _, d := range des {
		name := d.Name()
		if !d.Type().IsRegular() || strings.HasSuffix(name, ".tmp") {
			continue
		}
		f := m.mj + "/fail/" + name
		v := readLines(f, 6)
		q, s, ln, rec, lg, why := v[0], v[1], v[2], v[3], v[4], v[5]
		if !strings.HasPrefix(q, "refs/merge-queue/") || len(q) == len("refs/merge-queue/") ||
			strings.TrimPrefix(q, "refs/merge-queue/") != name || !sha40.MatchString(s) || ln == "" || why == "" {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 돌려보낼 기록 %s 이 깨졌다 — 버리고 큐에서 다시 판정\n", name)
			removeQuiet(f)
			continue
		}
		if !m.fail(q, s, ln, rec, lg, why) {
			m.mer = true
		}
	}
}

// readLines: 파일 앞 n줄(read -r처럼 앞뒤 빈칸·탭을 뺌). 모자라면 빈 값
func readLines(path string, n int) []string {
	v := make([]string, n)
	b, err := os.ReadFile(path)
	if err != nil {
		return v
	}
	lines := strings.Split(string(b), "\n")
	for i := 0; i < n && i < len(lines); i++ {
		if i == len(lines)-1 && lines[i] == "" {
			break
		}
		v[i] = strings.Trim(lines[i], " \t")
	}
	return v
}
