package main

// 자동 다시 맞춤(Go 판에서 더함, 2026-11-22 go-auto-reland — 11/21 병합기 rebase 충돌 반송이 하루 열 번 넘었고, 그때마다 지휘가
// 「lane 가지를 가져와 rebase·검증·다시 제출」 지시문을 손으로 써 띄웠다). 병합기 기계가 적은 막힘 사건 가운데 rebase 충돌
// (「병합기: 착지 실패(종료 3, 충돌 …)」 — cmd/hc-merger/judge.go)만 골라 대기열에 <레인>-reland(같은 프로젝트·같은 주인·우선 높음·
// 자동 감사 끔)를 넣고 주인 지휘에 「자동 다시 맞춤 넣음」 사건 한 줄. 같은 레인(원래 레인 — 장부의 바탕)은 설정 reland_max번
// (기본 2, 0이면 끔)까지, 넘으면 주인 지휘에 막힘만. 검증 실패(종료 1)·일시 실패(75 등)는 꼴이 달라 건드리지 않는다.
// 처리한 반송은 reland.tsv(줄 꼴은 relandTick), 첫 줄 「#since <UTC>」 — 켠 뒤 첫 정상 바퀴의 시각이고,
// 그 앞의 반송(지휘가 이미 손으로 다시 띄운 것)은 보지 않는다.
// 원래 레인이 살아 있으면(2026-11-22 reland-live-lane — sv-go-verify-lock 반송 때 살아 있던 원래 레인이 스스로 풀어 다시 냈는데
// -reland도 떠 같은 일을 둘이 했다): 실행 기록(OREC, 없으면 터미널 표)의 그 이름 터미널이 연결된 기계에 살아 있고 작업 트리
// (<저장소>.wt/<레인>)가 남았으면 다시 맞춤 대신 그 터미널에 hc send 한 줄, 주인 지휘에 「원래 레인에 보냄」(중간), 기록 결과 「보냄」
// (다시맞춤 칸은 그 주소). 설정 reland_wait분(기본 30) 안에 그 레인이 다시 내면(제출·착지 사건, 병합기의 새 사건, 큐에 같은 레인)
// 「다시 냄」, 아니면 그때 다시 맞춤을 넣는다. 기계가 연결 안 됨·작업 트리를 못 봄(모름)·닫힘이면 지금처럼 바로 다시 맞춤.
// 낸 터미널은 반송된 제출 기록의 host=·term=으로 먼저 본다(2026-11-22 clean-renamed-submit — HC_LANE=<원래>로 낸 다시 맞춤·고침
// 레인이 살아 있는데 원래 이름 터미널만 봐서 중복 레인을 띄웠다).

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	relandRe  = regexp.MustCompile(`^병합기: 착지 실패\(종료 3, 충돌 (.*?)\) — `)
	relandQRe = regexp.MustCompile(`refs/merge-failed/([0-9]+-[a-z0-9_-]+) `)
	relandLog = regexp.MustCompile(`, 로그 (\S+)`)
	relandSuf = regexp.MustCompile(`-reland[0-9]*$`)
)

// relandMax: 설정 reland_max(빼면 2, 0이면 끔)
func relandMax() int {
	v := conf("reland_max")
	if v == "" {
		return 2
	}
	if !isDigits(v) {
		say("알림: 설정 reland_max는 횟수(숫자)다, 2로: %s", v)
		return 2
	}
	return atoi10(v)
}

// relandWait: 설정 reland_wait(빼면 30분)
func relandWait() int {
	v := conf("reland_wait")
	if v == "" {
		return 30
	}
	if !isDigits(v) {
		say("알림: 설정 reland_wait는 분(숫자)이다, 30으로: %s", v)
		return 30
	}
	return atoi10(v)
}

// relandName <바탕> <k>: k번째 다시 맞춤 이름 — <바탕>-reland, 둘째부터 -reland<k>. 32자를 넘으면 바탕을 줄인다
func relandName(base string, k int) string {
	suf := "-reland"
	if k > 1 {
		suf += strconv.Itoa(k)
	}
	if len(base)+len(suf) > 32 {
		base = strings.TrimRight(base[:32-len(suf)], "-_")
	}
	return base + suf
}

// relandOn: 자동 다시 맞춤이 켜졌나 — 설정 merger 줄(프로젝트·기계)이 있고 reland_max가 0이 아님(사건 읽기·빈 대기열 바퀴도 이것으로)
func relandOn() bool {
	if v := conf("reland_max"); isDigits(v) && atoi10(v) == 0 {
		return false
	}
	for _, f := range confRows("merger") {
		if len(f) >= 3 {
			return true
		}
	}
	return false
}

// relandEvOK: 이번 바퀴에 병합기 사건을 다 읽었나 — 병합기 큐(켬 표식)를 못 읽은 프로젝트, 연결 안 됨·사건 모름인 병합기 기계가
// 있으면 아니다(그 바퀴에 #since를 적으면 그 사이 반송을 앞 이력으로 버린다)
func relandEvOK() bool {
	for _, f := range confRows("merger") {
		if len(f) >= 3 && inlist(f[1], MQBAD) {
			return false
		}
	}
	unk := evUnk()
	for _, m := range fields(MGM) {
		if m != LOCAL && (!connHas(m) || has(unk, m)) {
			return false
		}
	}
	return true
}

// relandKey: 주인 알림 키 — 넣음은 hf:reland:<큐 이름>, 원래 레인에 보냄은 그 뒤 「:sent」(같은 반송이 보냄 뒤 넣음이 되면
// hc-lane report --key가 같은 키를 이미 있다고 보고 넣음 알림을 삼켰다 — 작업 중 발견해 함께 고침)
func relandKey(n, res string) string {
	if res == "보냄" {
		return "hf:reland:" + n + ":sent"
	}
	return "hf:reland:" + n
}

// relandTell: 주인 지휘에 알림(중간) — 되면 장부에 「알림 <결과>」 줄, 안 되면 그대로 두어 다음 바퀴가 같은 키로 다시(R-19)
func relandTell(rf, n, lane, base, res, owner, msg string) {
	if hcLaneReport("중간", msg, lane, owner, relandKey(n, res)) != 0 {
		say("알림 실패(hc-lane report) — 다음 바퀴에 다시: %s", msg)
		return
	}
	if err := appendLine(rf, strings.Join([]string{n, lane, base, "알림", res, utcnow()}, "\t")); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
	say("%s", msg)
}

// relandTick: run 바퀴가 사건(EV)을 읽은 뒤 부른다. dry-run·사건 못 읽음은 건너뜀.
// 장부 reland.tsv 줄: 「큐 이름 레인 바탕 결과 다시맞춤 시각 [주인 알림글]」 — 넣음·보냄은 주인·알림글을 함께 적고, 알림이 되면
// 「큐 이름 레인 바탕 알림 <결과> 시각」을 따로 적는다(알림 줄이 없으면 다음 바퀴가 다시 보낸다). 바탕은 원래 레인 이름 —
// 다시 맞춤 레인의 반송은 그 이름을 넣은 줄의 바탕을 잇는다(잘린 이름에서 새 바탕을 만들면 횟수가 다시 시작됐다, R-18)
func relandTick() {
	if !relandOn() || dry || EVBAD {
		return
	}
	mx := relandMax()
	rf := DATA + "/reland.tsv"
	if !isFile(rf) {
		// 켠 첫 정상 바퀴(사건 0개여도)에 기준 시각 — 그 앞 반송만 뺀다(R-17). 병합기 사건을 다 못 읽었으면 다음 바퀴에
		if !relandEvOK() {
			say("다시 맞춤 기준 보류: 병합기 사건을 다 읽지 못함 — 다음 바퀴에")
			return
		}
		if err := writeAtomic(rf, "#since\t"+utcnow()+"\n"); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
		return
	}
	text, ok := readFile(rf)
	if !ok {
		say("다시 맞춤 보류: %s 를 못 읽음", rf)
		return
	}
	since, seen, cnt, pend := "~", map[string]bool{}, map[string]int{}, map[string]Row{}
	bases, told := map[string]string{}, map[string]bool{}
	var tell []Row
	for _, l := range lines(text) {
		r := splitRow(l)
		switch {
		case r.F(rlFailed) == "#since":
			since = r.F(rlSinceAt)
		case r.F(rlWhat) == "알림":
			told[r.F(rlFailed)+"\t"+r.F(rlArg)] = true
		case r.F(rlWhat) == "보냄": // 원래 레인에 보내고 다시 냄을 기다리는 반송 — 뒤 줄(넣음·다시 냄 등)이 있으면 끝난 것
			if !seen[r.F(rlFailed)] {
				pend[r.F(rlFailed)] = r
			}
			if r.F(rlMsg) != "" {
				tell = append(tell, r)
			}
		case r.F(rlFailed) != "":
			seen[r.F(rlFailed)] = true
			delete(pend, r.F(rlFailed))
			if r.F(rlWhat) == "넣음" {
				cnt[r.F(rlBase)]++
				bases[r.F(rlArg)] = r.F(rlBase)
				if r.F(rlMsg) != "" {
					tell = append(tell, r)
				}
			}
		}
	}
	for _, r := range tell { // 등록·보냄은 됐는데 주인 알림이 안 간 것 — 같은 키로 다시
		if !told[r.F(rlFailed)+"\t"+r.F(rlWhat)] {
			ow := r.F(rlOwner)
			if ow == "-" {
				ow = ""
			}
			relandTell(rf, r.F(rlFailed), r.F(rlLane), r.F(rlBase), r.F(rlWhat), ow, r.F(rlMsg))
		}
	}
	mg := map[string]bool{}
	for _, m := range fields(MGM) {
		mg[m] = true
	}
	for _, e := range EV {
		r := splitRow(e)
		if r.F(evState) != "막힘" || !mg[r.F(evMach)] || r.F(evTime) < since {
			continue
		}
		mm, qm := relandRe.FindStringSubmatch(r.F(evMsg)), relandQRe.FindStringSubmatch(r.F(evMsg))
		if mm == nil || qm == nil || seen[qm[1]] {
			continue
		}
		seen[qm[1]] = true
		base := bases[r.F(evLane)]
		if base == "" {
			base = relandSuf.ReplaceAllString(r.F(evLane), "")
		}
		if pr, ok := pend[qm[1]]; ok {
			relandPend(rf, qm[1], mm[1], base, r, mx, cnt, pr, mg)
			continue
		}
		relandOne(rf, qm[1], mm[1], base, r, mx, cnt, true)
	}
}

// relandPend: 원래 레인에 보낸 반송 — 보낸 뒤 그 레인이 다시 냈으면 「다시 냄」, reland_wait분이 지나도 안 냈으면 다시 맞춤
func relandPend(rf, n, files, base string, r Row, mx int, cnt map[string]int, pr Row, mg map[string]bool) {
	lane, at := r.F(evLane), pr.F(rlAt)
	re := func() {
		if err := appendLine(rf, strings.Join([]string{n, lane, base, "다시 냄", pr.F(rlArg), utcnow()}, "\t")); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
		say("다시 맞춤 안 함 %s: 원래 레인이 다시 냈다(refs/merge-failed/%s)", lane, n)
	}
	for _, e := range EV {
		x := splitRow(e)
		if strings.Contains(x.F(evMsg), "refs/merge-failed/"+n+" ") {
			continue // 이 반송 사건 자체
		}
		if x.F(evLane) == lane && x.F(evTime) > at && (x.F(evState) == "제출" || x.F(evState) == "착지" || (mg[x.F(evMach)] && x.F(evState) == "막힘")) {
			re()
			return
		}
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", at)
	if err == nil && time.Since(t) < time.Duration(relandWait())*time.Minute {
		return
	}
	// 시간이 지났다 — 큐에 같은 레인(다시 냄)·착지 중이면 relandOne이 아무것도 하지 않는다
	say("다시 맞춤 %s: 원래 레인(%s)에 보낸 뒤 %d분 — 병합기 큐를 보고 다시 맞춤", lane, pr.F(rlArg), relandWait())
	relandOne(rf, n, files, base, r, mx, cnt, false)
}

// relandSubmitter <프로젝트> <큐 이름>: 반송된 제출 기록(원격 refs/merge-failed-meta/<n>)의 host=·term=으로 낸 터미널 주소 —
// 다시 맞춤·고침 레인이 HC_LANE=<원래>로 내면 사건의 레인 이름은 <원래>라, 낸 터미널은 이것으로만 안다. 못 읽으면 ""
func relandSubmitter(p, n string) string {
	r := repoOf(p, LOCAL)
	if r == "" {
		return ""
	}
	ref := "refs/merge-failed-meta/" + n
	o, rc := run("git", "-C", r, "ls-remote", "origin", ref)
	if rc != 0 || !relandLsHas(o, ref) {
		return ""
	}
	sha, _, _ := strings.Cut(o, "\t")
	if _, rc := run("git", "-C", r, "cat-file", "-e", sha+"^{commit}"); rc != 0 {
		if _, rc := run("git", "-C", r, "fetch", "-q", "--no-tags", "--no-write-fetch-head", "origin", ref); rc != 0 {
			return ""
		}
	}
	s, rc := run("git", "-C", r, "log", "-1", "--format=%s", sha)
	if rc != 0 {
		return ""
	}
	host, term := "", ""
	for _, kv := range strings.Fields(s) {
		if v, ok := strings.CutPrefix(kv, "host="); ok {
			host = v
		} else if v, ok := strings.CutPrefix(kv, "term="); ok {
			term = v
		}
	}
	switch {
	case term == "" || term == "-":
		return ""
	case strings.Contains(term, "/") || host == "" || host == "-" || host == LOCAL:
		return term
	}
	return host + "/" + term
}

// relandLive <레인> <프로젝트> <큐 이름>: 원래 레인 터미널 주소와 상태 — 살아 있음(연결된 기계에 그 주소·이름 터미널 + 작업 트리
// 남음)·닫힘·모름(기계 연결 안 됨·작업 트리를 못 봄). 먼저 반송된 제출 기록의 낸 터미널(이름이 달라도 — 그 터미널 이름의 작업 트리),
// 다음 실행 기록(OREC), 기록이 없으면 터미널 표에서 같은 이름
func relandLive(lane, p, n string) (string, string) {
	var cs [][3]string
	if sa := relandSubmitter(p, n); sa != "" {
		for _, t := range TERMS {
			if t.Addr == sa && t.Name != "-" {
				cs = append(cs, [3]string{t.M, t.Addr, t.Name})
			}
		}
	}
	sn := len(cs)
	for _, o := range OREC {
		if o.F(owLane) == lane && !strings.HasPrefix(o.F(owAddr), "pending:") {
			cs = append(cs, [3]string{o.F(owMach), o.F(owAddr), lane})
		}
	}
	if len(cs) == sn {
		for _, t := range TERMS {
			if t.Name == lane {
				cs = append(cs, [3]string{t.M, t.Addr, lane})
			}
		}
	}
	unk := false
	for _, c := range cs {
		m, a, ln := c[0], c[1], c[2]
		if m != LOCAL && !connHas(m) {
			unk = true
			continue
		}
		if !lalive(a, ln) {
			continue
		}
		repo := repoOf(p, m)
		if repo == "" {
			unk = true
			continue
		}
		o, rc := onm(nil, m, `if [ -d "$d" ]; then echo 있음; else echo 없음; fi`, strings.TrimSuffix(repo, "/")+".wt/"+ln)
		switch strings.TrimSpace(o) {
		case "있음":
			if rc == 0 {
				return a, "살아 있음"
			}
			unk = true
		case "없음":
			if rc != 0 {
				unk = true
			}
		default:
			unk = true
		}
	}
	if unk {
		return "", "모름"
	}
	return "", "닫힘"
}

// relandOne: 반송 하나(큐 이름 n) — 프로젝트를 찾고(원격에 refs/merge-failed/<n>이 아직 있어야), 상한이면 막힘, 아니면 줄을 넣는다.
// 원격을 못 읽거나 이름이 아직 대기열에 있으면 적지 않고 다음 바퀴에
// live면 먼저 원래 레인을 본다(relandLive) — 살아 있으면 그 터미널에 보내고 끝(보냄, 횟수에 안 셈)
func relandOne(rf, n, files, base string, r Row, mx int, cnt map[string]int, live bool) {
	lane, mach, owner := r.F(evLane), r.F(evMach), r.F(evTo)
	if owner == "-" {
		owner = ""
	}
	rec := func(res, name string) {
		if err := appendLine(rf, strings.Join([]string{n, lane, base, res, name, utcnow()}, "\t")); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
	}
	// recTell: 결과 줄에 주인·알림글을 함께 적고(등록), 알림은 따로(relandTell — 안 되면 다음 바퀴가 다시)
	recTell := func(res, name, msg string) {
		if err := appendLine(rf, strings.Join([]string{n, lane, base, res, name, utcnow(), dash(owner), msg}, "\t")); err != nil {
			fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
		}
		relandTell(rf, n, lane, base, res, owner, msg)
	}
	p, st := relandFind(n)
	switch st {
	case "모름":
		say("다시 맞춤 보류 %s: 원격 refs/merge-failed/%s 를 못 읽음 — 다음 바퀴에", lane, n)
		return
	case "없음":
		say("다시 맞춤 안 함 %s: refs/merge-failed/%s 가 원격에 없다(이미 치움)", lane, n)
		rec("없음", "-")
		return
	}
	// 그 레인의 새 제출이 병합기에 있으면(지금 착지 중·큐에 섬 — 반송은 큐 참조를 같은 원자 push로 지운다) 아무것도 하지 않는다
	switch lr, qr := mqlanding(p, lane), mqueued(p, lane); {
	case lr == 2 || qr == 2:
		say("다시 맞춤 보류 %s: 병합기 큐를 못 읽음 — 다음 바퀴에", lane)
		return
	case lr == 0:
		say("다시 맞춤 안 함 %s: 병합기가 그 레인의 새 제출을 지금 착지 중(refs/merge-failed/%s)", lane, n)
		rec("착지 중", "-")
		return
	case qr == 0:
		say("다시 맞춤 안 함 %s: 그 레인의 새 제출이 병합기 큐에 있다(refs/merge-failed/%s)", lane, n)
		rec("다시 냄", "-")
		return
	}
	rule := relandDocsRule(p)
	if live {
		switch a, st := relandLive(lane, p, n); st {
		case "살아 있음":
			msg := relandSendMsg(files, n)
			if rule != "" { // hc send 글은 한 줄 — 규칙 줄마다 「 / 」
				var ls []string
				for _, l := range lines(rule) {
					if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- ")); l != "" {
						ls = append(ls, l)
					}
				}
				msg += " — 이 저장소의 문서 규칙: " + strings.Join(ls, " / ")
			}
			// 보내기 전에 시도를 반송 ID(n)로 「보냄」에 적는다 — hc send가 받은 뒤 응답을 잃으면(255) 받았는지 모르는데 같은 바퀴에
			// 다시 맞춤까지 넣어 두 레인이 같은 일을 했다(재감사2 fill/R-215). 적은 뒤에는 다음 바퀴들이 relandPend로 원래 레인의
			// 다시 냄을 대조하고 reland_wait분이 지나야 다시 맞춤(보내지는 않음). 적지 못하면 보내지도 넣지도 않고 다음 바퀴에
			if err := appendLine(rf, strings.Join([]string{n, lane, base, "보냄", a, utcnow()}, "\t")); err != nil {
				say("다시 맞춤 보류 %s: 보냄 기록을 못 씀(%v) — 다음 바퀴에", lane, err)
				return
			}
			if !auSend(a, msg) {
				say("다시 맞춤 %s: 원래 레인(%s)에 hc send 결과 모름(실패·응답 잃음) — 받았을 수 있어 %d분 동안 그 레인의 다시 냄을 본 뒤 다시 맞춤", lane, a, relandWait())
				return
			}
			recTell("보냄", a, fmt.Sprintf("원래 레인에 보냄 %s(%s) — 병합기 rebase 충돌 반송(refs/merge-failed/%s, 충돌 %s), %d분 안에 다시 안 내면 다시 맞춤", lane, a, n, files, relandWait()))
			return
		case "모름":
			say("다시 맞춤 %s: 원래 레인이 살아 있는지 모름 — 다시 맞춤으로", lane)
		}
	}
	if cnt[base] >= mx {
		msg := fmt.Sprintf("%s 병합기 rebase 충돌로 또 반송 — 자동 다시 맞춤 %d번 넘음, 지휘 판단 (refs/merge-failed/%s, 충돌 %s)", lane, mx, n, files)
		if aesc(lane, dash(owner), msg, "hf:reland:"+n) == 0 {
			rec("상한", "-")
		}
		return
	}
	name := relandName(base, cnt[base]+1)
	msg := fmt.Sprintf("자동 다시 맞춤 넣음 %s — %s 병합기 rebase 충돌 반송(refs/merge-failed/%s, 충돌 %s)", name, lane, n, files)
	if qHasName(name) {
		// 넣은 뒤 기록 전에 끊긴 바퀴면 그 줄의 지시문이 이 반송을 가리킨다 — 기록을 잇고 알림도 보낸다
		if b, _ := readFile(qrow(name).F(qBrief)); strings.Contains(b, "refs/merge-failed/"+n+" ") {
			cnt[base]++
			recTell("넣음", name, msg)
		} else {
			say("다시 맞춤 보류 %s: %s 가 아직 대기열에 있다 — 빠진 뒤에", lane, name)
		}
		return
	}
	lg := "모름(사건 글이 잘림)"
	if m := relandLog.FindStringSubmatch(r.F(evMsg)); m != nil {
		lg = m[1]
	}
	bf := DATA + "/briefs/added/" + name + ".md"
	tmp := bf + ".tmp." + strconv.Itoa(os.Getpid())
	if err := os.MkdirAll(DATA+"/briefs/added", 0o755); err != nil {
		say("다시 맞춤 보류 %s: %v", lane, err)
		return
	}
	if err := os.WriteFile(tmp, []byte(relandBrief(name, lane, n, mach, files, lg, r.F(evMsg), rule)), 0o644); err != nil {
		say("다시 맞춤 보류 %s: 지시문을 못 씀: %v", lane, err)
		return
	}
	if qAdd(tmp, name, p, bf, "-", "-", "wait", "-", "-", "-", nowMin(), "-", dash(owner), "high", "-", "no-audit", "-") != 0 {
		os.Remove(tmp)
		say("다시 맞춤 보류 %s: 대기열에 못 넣음 — 다음 바퀴에", lane)
		return
	}
	cnt[base]++
	recTell("넣음", name, msg)
}

// relandFind <큐 이름>: 병합기 설정 프로젝트 가운데 원격에 refs/merge-failed/<n>이 있는 것. 있음·없음·모름(원격을 못 읽은 프로젝트가 있음)
func relandFind(n string) (string, string) {
	var ps []string
	for _, f := range confRows("merger") {
		if len(f) >= 2 {
			ps = append(ps, f[1])
		}
	}
	unk := false
	for _, p := range uniq(ps) {
		r := repoOf(p, LOCAL)
		if r == "" {
			unk = true
			continue
		}
		ref := "refs/merge-failed/" + n
		o, rc := run("git", "-C", r, "ls-remote", "--exit-code", "origin", ref)
		if rc == 2 && o == "" { // --exit-code: 맞는 참조가 없다고 확정
			continue
		}
		if rc == 0 && relandLsHas(o, ref) {
			return p, "있음"
		}
		unk = true // 종료 0이 아님·빈·잘린·꼴 틀린 출력·요청한 이름 없음 — 모름(R-15)
	}
	if unk {
		return "", "모름"
	}
	return "", "없음"
}

var relandLsLine = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?\t(\S+)$`)

// relandLsHas: ls-remote 출력이 온전한 줄(「<40/64자 16진>\t<참조>」, 끝 줄바꿈)로만 되어 있고 그 가운데 요청한 참조가 정확히 있나
func relandLsHas(o, ref string) bool {
	if !strings.HasSuffix(o, "\n") {
		return false
	}
	hit := false
	for _, l := range strings.Split(strings.TrimSuffix(o, "\n"), "\n") {
		m := relandLsLine.FindStringSubmatch(l)
		if m == nil {
			return false
		}
		hit = hit || m[2] == ref
	}
	return hit
}

// relandDocsRuleFile: 저장소마다 둘 수 있는 문서 규칙(범용 — 글은 그 저장소가 정한다)
const relandDocsRuleFile = ".claude/herdr-docs-rule.md"

// relandDocsRule <프로젝트>: 대상 저장소 기본 가지의 문서 규칙 글(2026-11-22 docs-rule-guard — docs-split 뒤 README에서 부딪힌
// 다시 맞춤 레인이 저장소 문서 규칙을 몰라 README에 글을 되돌려 넣을 수 있었다). 없으면(빈 파일도) ""; 저장소·기본 가지·파일을
// 못 읽으면 경고 한 줄 뒤 "" — 규칙 없이 지금처럼 넣는다
func relandDocsRule(p string) string {
	repo := repoOf(p, LOCAL)
	ref, ok := "", repo != ""
	if ok {
		ref, ok = projectRef(repo)
	}
	if ok {
		o, rc := run("git", "-C", repo, "ls-tree", "--name-only", ref, "--", relandDocsRuleFile)
		switch {
		case rc == 0 && sub(o) == "":
			return ""
		case rc == 0 && sub(o) == relandDocsRuleFile:
			if b, rc := run("git", "-C", repo, "show", ref+":"+relandDocsRuleFile); rc == 0 {
				return strings.TrimSpace(b)
			}
		}
	}
	say("알림: 다시 맞춤 지시문에 문서 규칙을 못 붙임 — 프로젝트 %s 기본 가지의 %s 를 못 읽음", p, relandDocsRuleFile)
	return ""
}

// relandSendMsg: 살아 있는 원래 레인에 보내는 한 줄. 전체 검증은 넣지 않는다(conflict-fast C, 2026-11-23 — 레인 쪽 전체 검증 뒤 병합기가
// 같은 전체 검증을 또 돌았다. 기본 브랜치 관문은 병합기 검증 그대로)
func relandSendMsg(files, n string) string {
	return fmt.Sprintf("병합기 반송(rebase 충돌 %s, refs/merge-failed/%s) — origin/main으로 rebase해 양쪽 살려 풀고 푼 파일 둘레의 빌드·시험만 돌린 뒤 hc-lane land main 다시(전체 검증은 병합기가 돈다)", files, n)
}

// relandBrief: 다시 맞춤 레인 지시문(착지·정리 절차는 hc-fill이 run 지시문 끝에 붙인다). rule이 있으면 「## 이 저장소의 문서 규칙」 절로
func relandBrief(name, lane, n, mach, files, lg, ev, rule string) string {
	if rule != "" {
		rule = "\n## 이 저장소의 문서 규칙\n" + rule + "\n"
	}
	return fmt.Sprintf(`# 레인 %[1]s — %[2]s 병합기 rebase 충돌 반송 다시 맞춤 (hc-fill 자동, %[8]s)

병합기(기계 %[4]s)가 %[2]s 의 제출을 rebase 충돌(착지 실패 종료 3)로 refs/merge-failed/%[3]s 에 돌려보냈다. 그 커밋을 지금 origin/main 위로 다시 맞춰 넣는다(같은 커밋이 이미 감사를 거쳤으니 자동 감사는 끔).
- 충돌 파일(병합기가 3자 합침으로 본 것): %[5]s
- 병합기 로그: %[6]s (기계 %[4]s의 경로 — 이 기계가 아니면 hc file read --on %[4]s <경로>, 못 읽으면 rebase로 직접 본다)
- 병합기 사건: %[7]s

## 할 것
1. 이 작업 트리(가지 %[1]s, origin/main에서 시작)를 반송된 제출로 옮긴다: git fetch origin refs/merge-failed/%[3]s 뒤 git reset --keep FETCH_HEAD
2. git rebase origin/main — 충돌은 양쪽 내용을 살려 풀고 고친 파일을 경로로 git add 한 뒤 git rebase --continue. git stash·rebase --skip·rebase --abort 금지.
3. 커밋 제목은 원래 그대로 둔다(머리 %[2]s: — 이 이름을 --after로 기다리는 레인이 풀리게). 풀이로 새 커밋을 더하면 머리는 %[1]s:.
4. 충돌을 푼 파일 둘레의 빌드·시험만 돌린다(그 파일이 든 묶음·꾸러미의 빌드와 그 파일을 다루는 시험). 실패하면 원인을 고쳐 커밋하고 다시. 전체 검증은 병합기가 착지 때 돈다 — 레인에서 다시 돌리지 않는다.
5. 충돌을 풀며 기능 코드의 뜻(동작)을 바꿔야 했으면 착지하지 말고 hc-lane report 막힘 - "<까닭>" 을 부르고 멈춘다. 아니면 아래 착지·정리 절차(hc-lane land main → hc-lane clean)대로.
6. 또 rebase 충돌로 반송되면 hc-fill이 설정 횟수까지 다시 넣고, 넘으면 지휘에 막힘으로 넘긴다 — 이 레인은 따로 할 일 없다.

보고는 한국어. 질문 창 금지(되돌릴 수 있는 판단은 추천안으로 정해 보고에 「가정/결정」). 멈출 때는 네가 띄운 정확한 pid로만.
`, name, lane, n, mach, files, lg, ev, nowMin()) + rule
}
