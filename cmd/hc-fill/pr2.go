package main

// PR 거울의 일 셋(pr.go가 부른다): opens(완료 → PR 열기·다시 완료 댓글), verdicts(감사 판정 → 리뷰 또는 댓글 + 커밋 상태),
// mergerEvents(제출·착지·돌려보냄·병합 쪽 풀이 → 댓글·상태, 착지면 Merged 확인 또는 닫기)

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"herdr-tools/internal/events"
	"herdr-tools/internal/ghpr"
	"herdr-tools/internal/proc"
)

// sPrSrc <작업 트리> <sha> <머리 레인> <레인>: 레인 기계에서 커밋을 숨은 참조 refs/herdr/pr-src/<머리 레인>로 올리고(그 기계의 git 로그인 —
// 토큰을 워커 기계로 보내지 않는다) 원격 주소를 찍는다. 머리 가지 pr/<레인>은 대기열 기계가 gh로 만든다(PR 작성자 인증).
// 머리 레인이 레인과 다르면(원래 PR을 이어 씀) 짝 참조 refs/herdr/pr-of/<레인>/<머리 레인>도 — 병합기가 그 레인의 머리 가지를 찾는 길.
// 제출 보고($4 sub — 커밋이 이미 제출 참조로 원격에 있다, pr-inbox-lanes)는 작업 트리가 정리됐으면 저장소 폴더($5)에서 원격만 읽고, 올리기 실패도 넘긴다
const sPrSrc = `s=$4 r=$5; case $r in "~/"*) r=$HOME/${r#"~/"} ;; esac
cd "$d" 2>/dev/null || { [ "$s" = sub ] && cd "$r" 2>/dev/null; } || { echo "PRX cd"; exit 0; }
u=$(git remote get-url origin 2>/dev/null) || { echo "PRX noremote"; exit 0; }
printf 'URL %s\n' "$u"
git push -q origin "+$1:refs/herdr/pr-src/$2" >/dev/null 2>&1 || [ "$s" = sub ] || { echo "PRX push"; exit 0; }
[ "$3" = "$2" ] || git push -q origin "+$1:refs/herdr/pr-of/$3/$2" >/dev/null 2>&1 || [ "$s" = sub ] || { echo "PRX push"; exit 0; }
echo PUSHED`

const prBodyCap = 60000 // 보고서 본문 상한(글자)

var prSumRe = regexp.MustCompile(`\s*\(감사 hc-fill 자동\)\s*$`)

// prSummary: 완료 보고 한 줄(꼬리 「(감사 hc-fill 자동)」 뺌)
func prSummary(msg string) string { return strings.TrimSpace(prSumRe.ReplaceAllString(msg, "")) }

// opens: 떠 있는 대기열 레인(감사 줄 빼고, pr 켠 프로젝트)의 그 실행 마지막 완료 — 아직 안 썼으면 열거나(첫 완료) 다시 완료 댓글.
// 완료 없이 제출하는 레인(감사 칸이 auto가 아님 — 받은편지·--no-audit, 손잡이 pr_inbox)은 SHA가 든 마지막 보고(중간·완료·제출)로(prinbox.go)
func (r *prRun) opens() {
	inb := prInboxOn()
	for _, q := range qRows() {
		if r.stop {
			return
		}
		n, p := q.F(qName), q.F(qProj)
		if q.F(qState) != "up" || strings.HasPrefix(q.F(qAudit), "audit=") || !prOn(p) {
			continue
		}
		self := inb && q.F(qAudit) != "auto"
		l := ""
		if self {
			l = prLastSHA(n, q.F(qAddr))
		} else {
			l = joblast(n, q.F(qAddr), "완료")
		}
		ev := events.Cols(l)
		key := "done:" + evKey(l)
		if l == "" || len(ev.SHA) != 40 || r.sent[key] == "sent" {
			continue
		}
		t := n // 다시 맞춤 레인·--pr-of 레인은 원래 레인 이름으로 낸다 — 원래 PR을 이어 쓴다(제 PR이 이미 있으면 그것)
		if b := prBaseOf(n); b != n && r.ents[n] == nil {
			if e := r.ents[b]; e != nil && e.State == "open" {
				t = b
			} else if prOfGet(n) != "" && r.sent[key] == "" {
				say("PR 거울 %s: --pr-of %s 의 PR이 없거나 이미 닫힘·Merged — 새 PR로", n, b)
			}
		}
		e := r.ents[t]
		if (e == nil || e.State != "open") && !r.fresh(q, self, ev, t) { // 새 PR은 살아 있는 레인의 켠 뒤 보고에만(prfresh.go) — 끝난 옛 레인은 로그도 없이
			continue
		}
		if prLandedSince(t, ev.Time) || self && prLandedSince(n, ev.Time) { // 거울이 늦어 이미 착지했다 — 빈 PR을 열려다 매 바퀴 실패하지 않게(감사 낮음 7)
			if self && (e == nil || e.State != "open") {
				say("PR 없음 %s — 착지가 먼저", n)
			}
			r.sent[key] = "sent"
			continue
		}
		// 같은 커밋의 뒤 보고(「중간 <sha> 감사 완료」)·열린 PR의 제출 — 머리·댓글 그대로. 제출이 커밋을 바꿨어도(rebase) 머리를 옮기지 않는다:
		// 병합기는 바퀴 머리에 본 머리 값으로 lease하고 끝 커밋을 제출 SHA의 신원으로 찾는다 — 옮기면 lease가 깨져 Merged 대신 닫힌다(감사 1차 중간)
		if self && e != nil && e.State == "open" && (e.Head == ev.SHA || ev.State == "제출") {
			// 원래 PR을 이어 쓰는 레인의 첫 보고가 제출이어도 짝 참조는 올린다 — 병합기가 원래 머리 가지를 찾는 길(2026-11-24 pr-mirror-gaps)
			if pk := "prof:" + n + ":" + t; t != n && r.sent[pk] != "sent" {
				if _, ok := r.pushSrc(n, p, q.F(qMach), ev, key, t); !ok {
					continue
				}
				r.sent[pk] = "sent"
			}
			r.sent[key] = "sent"
			continue
		}
		r.open(t, n, p, q.F(qMach), q.F(qBrief), ev, key)
	}
}

// prRelandBase <레인>: 다시 맞춤 레인이면 맨 처음 레인 이름 — 다시 맞춤 기록(reland.tsv 「넣음」 줄)을 먼저, 없으면 꼬리 -reland… 를 뗀 이름
// (원래 이름이 길어 줄인 다시 맞춤 이름은 꼬리를 떼도 원래 이름이 아니다 — 감사 낮음 6). 「넣음」 줄의 레인 칸은 반송된 레인 — 다시 맞춤 레인이
// 또 반송되면 바로 앞 다시 맞춤 레인이라, 그 이름이 또 「넣음」 줄에 있으면 사슬을 끝까지 따라간다(둘째 …-reland2부터 새 PR이 열렸다, pr-mirror-gaps)
func prRelandBase(n string) string {
	up := map[string]string{}
	for _, r := range readRows(DATA + "/reland.tsv") {
		if _, ok := up[r.F(rlArg)]; r.F(rlWhat) == "넣음" && r.F(rlArg) != "" && r.F(rlLane) != "" && !ok {
			up[r.F(rlArg)] = r.F(rlLane)
		}
	}
	b, ok := up[n]
	if !ok {
		return relandSuf.ReplaceAllString(n, "")
	}
	for seen := map[string]bool{n: true}; !seen[b] && up[b] != ""; {
		seen[b], b = true, up[b]
	}
	return b
}

// prLandedSince <레인> <시각>: 그 시각 뒤 그 레인 이름의 착지 사건이 있나
func prLandedSince(lane, at string) bool {
	for _, l := range EV {
		if e := events.Cols(l); e.Lane == lane && e.State == "착지" && e.Time >= at {
			return true
		}
	}
	return false
}

func keyRepo(v string) (ghpr.Repo, bool) {
	x := strings.Split(v, "/")
	if len(x) != 3 {
		return ghpr.Repo{}, false
	}
	return ghpr.Repo{Host: x[0], Owner: x[1], Name: x[2]}, true
}

// open: 커밋을 원격에 두고 머리 가지 pr/<t>를 그 커밋으로, PR이 열려 있으면 다시 완료 댓글, 아니면 PR을 연다.
// 단계마다 해낸 것을 열쇠로 남겨(값 = 저장소) 다음 바퀴는 남은 단계만 — 커밋 올리기(src)·머리 가지(ref)
func (r *prRun) open(t, n, p, m, brief string, ev events.Event, key string) {
	rk, sk := "ref:"+t+":"+ev.SHA, "src:"+t+":"+ev.SHA
	if repo, ok := keyRepo(r.sent[rk]); ok {
		r.opened(t, n, p, m, brief, ev, key, repo)
		return
	}
	repo, ok := keyRepo(r.sent[sk])
	if !ok {
		if repo, ok = r.pushSrc(n, p, m, ev, key, t); !ok {
			return
		}
		r.sent[sk] = repo.Host + "/" + repo.Owner + "/" + repo.Name
		if t != n {
			r.sent["prof:"+n+":"+t] = "sent"
		}
	}
	if !r.do("머리 가지 "+t, r.bot.SetRef(repo, "pr/"+t, ev.SHA)) {
		return
	}
	r.sent[rk] = repo.Host + "/" + repo.Owner + "/" + repo.Name
	r.opened(t, n, p, m, brief, ev, key, repo)
}

// pushSrc: 레인 기계에서 커밋을 숨은 참조로 올리고 저장소를 읽는다. 바퀴당 호출 상한에 한 번으로 세고, 시간 넘김이면 이 바퀴를 멈춘다
func (r *prRun) pushSrc(n, p, m string, ev events.Event, key, t string) (ghpr.Repo, bool) {
	if !r.gh.Take() {
		r.stop = true
		return ghpr.Repo{}, false
	}
	sub := "-"
	if ev.State == "제출" {
		sub = "sub"
	}
	rp := strings.TrimSuffix(repoOf(p, m), "/")
	out, rc := onm(nil, m, sPrSrc, rp+".wt/"+n, ev.SHA, t, n, sub, rp)
	if rc != 0 {
		r.bad++
		r.last = fmt.Sprintf("커밋 올리기 %s(%s, 종료 %d)", n, m, rc)
		r.stop = rc == proc.TimeoutCode // 느린 기계가 바퀴를 묶지 않게
		return ghpr.Repo{}, false
	}
	url, why := "", ""
	for _, l := range lines(out) {
		if u, ok := strings.CutPrefix(l, "URL "); ok {
			url = u
		} else if w, ok := strings.CutPrefix(l, "PRX "); ok {
			why = w
		}
	}
	repo, ok := ghpr.ParseRemote(url)
	switch {
	case why == "cd":
		say("PR 거울 건너뜀 %s: 작업 트리가 없다", n)
		r.sent[key] = "sent"
		return repo, false
	case why == "noremote" || (why == "" && !ok):
		prAlert("remote:"+p, "PR 거울: "+p+" 원격 주소에서 GitHub 저장소를 못 읽었다 — 그 프로젝트는 PR을 열지 않는다")
		r.sent[key] = "sent"
		return repo, false
	case why != "":
		r.bad++
		r.last = "커밋 올리기 " + n + ": git push 실패"
		return repo, false
	}
	return repo, true
}

// opened: 머리 가지가 그 커밋인 뒤 — PR이 열려 있으면 다시 완료 댓글, 아니면 PR을 연다
func (r *prRun) opened(t, n, p, m, brief string, ev events.Event, key string, repo ghpr.Repo) {
	sum := prSummary(ev.Msg)
	if e := r.ents[t]; e != nil && e.State == "open" {
		w := ""
		if t != n {
			w = " (다시 맞춤 레인 " + n + ")"
		}
		if r.comment(e, key, "다시 완료 "+head(ev.SHA, 8)+" — "+sum+w) {
			e.Head = ev.SHA
		}
		return
	}
	ref, ok := abase(p, m)
	if !ok {
		r.bad++
		r.last = "기본 가지 조회 " + p
		return
	}
	base := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "origin/")
	pr, err := r.bot.Create(repo, base, "pr/"+t, r.mask(runeCut(t+" — "+sum, 200)), r.mask(prBody(t, sum, brief)))
	if err != nil && strings.Contains(err.Error(), "No commits between") { // 머리가 이미 기준에 들었다(착지가 거울보다 먼저) — 실패로 세지 않는다
		r.ok++
		r.sent[key] = "sent"
		say("PR 없음 %s — 착지가 먼저(머리가 이미 기준에 들었다)", t)
		return
	}
	if !r.do("PR 열기 "+t, err) {
		return
	}
	by := "self"
	if r.bot.Token != "" {
		by = "bot"
	}
	if r.ents[t] == nil {
		r.order = append(r.order, t)
	}
	r.ents[t] = &prEnt{t, p, repo.Host + "/" + repo.Owner + "/" + repo.Name, strconv.Itoa(pr.Num), pr.URL, "open", ev.Time, by, ev.SHA}
	r.sent[key] = "sent"
	r.save()
	say("PR #%d 열음 %s — %s", pr.Num, t, pr.URL)
}

// prBody: PR 본문 — 완료 요약·지시문 제목·「도구가 관리」
func prBody(lane, sum, brief string) string {
	title := "-"
	for _, l := range readLines(brief) {
		if s := strings.TrimSpace(strings.TrimLeft(l, "# ")); s != "" {
			title = s
			break
		}
	}
	return "레인 `" + lane + "` 완료 보고: " + sum + "\n\n지시문: " + runeCut(title, 300) +
		"\n\n이 PR은 도구(hc-fill PR 거울)가 관리한다 — 여기서 손으로 머지하지 않는다. 감사·검증·병합 결과가 이 PR에 댓글·커밋 상태로 남고, 병합기가 착지하면 Merged로 표시된다."
}

// verdicts: 받은 감사 판정(차수마다 한 번) — PR 작성자가 다른 계정(토큰으로 엶)이면 진짜 리뷰, 아니면 댓글. 커밋 상태는 둘 다
func (r *prRun) verdicts() {
	for _, a := range readRows(AT) {
		if r.stop {
			return
		}
		v, hml := a.F(atVerdict), ""
		for _, f := range strings.Split(a.F(atFlags), ",") {
			if x, ok := strings.CutPrefix(f, "hml="); ok {
				hml = x
			}
		}
		e := r.entOf(a.F(atLane))
		if a.NF() < atNF || (v != "pass" && v != "fix") || hml == "" || e == nil || e.State != "open" {
			continue
		}
		c := strings.Split(hml, ".")
		if len(c) != 3 {
			continue
		}
		cnt := "높음 " + c[0] + "·중간 " + c[1] + "·낮음 " + c[2] + " · " + a.F(atRound) + "차"
		word, st := "통과", "success"
		if v == "fix" {
			word, st = "지적", "failure"
		}
		id := a.F(atID)
		if r.sent["audit-st:"+id] == "sent" && r.sent["audit:"+id] == "sent" { // 이미 올렸다 — 레인 기계의 보고서를 다시 읽지 않는다(pr-mirror-gaps)
			continue
		}
		if !r.status(e, "audit-st:"+id, a.F(atSHA), st, "audit", cnt) {
			continue
		}
		// 보고서 읽기(레인 기계)도 바퀴당 상한에 한 번으로 센다 — 읽고 올릴 자리(앞 시도가 응답 없이 끝났으면 표식 확인까지)가 없으면 다음 바퀴에.
		// 자리를 모르는 보고서(원격 읽기 없음)는 세지 않는다
		need, path := 2, auRest(a, atNote)
		if r.sent["audit:"+id] == "try" {
			need = 3
		}
		if strings.HasPrefix(path, "~/") && (r.gh.Max > 0 && r.gh.Max-r.gh.Calls() < need || !r.gh.Take()) {
			r.stop = true
			return
		}
		rep, ok := r.report(a.F(atMach), path)
		if !ok {
			return
		}
		body := "감사 " + a.F(atRound) + "차 " + word + " — 높음 " + c[0] + "·중간 " + c[1] + "·낮음 " + c[2] + " (" + head(a.F(atSHA), 8) + ")\n\n" + rep
		r.review(e, "audit:"+id, v == "pass", body)
	}
}

// report <기계> <경로>: 감사 보고서 본문(길면 앞 prBodyCap자와 「나머지는 <기계>:<경로>」). 못 읽으면 그 자리만 알린다.
// 시간 넘김이면 거짓 — 이 바퀴는 멈춘다(느린 기계가 바퀴를 묶지 않게, pushSrc와 같다)
func (r *prRun) report(m, path string) (string, bool) {
	if !strings.HasPrefix(path, "~/") {
		return "(보고서 자리 모름)", true
	}
	out, rc := onm(nil, m, `cat -- "$d"`, path)
	if rc == proc.TimeoutCode {
		r.stop = true
		return "", false
	}
	if rc != 0 {
		return "(보고서를 못 읽음 — " + m + ":" + path + ")", true
	}
	if rs := []rune(out); len(rs) > prBodyCap {
		return string(rs[:prBodyCap]) + "\n\n…나머지는 " + m + ":" + path, true
	}
	return out, true
}

// review: 작성자가 다른 계정이면 승인·변경 요청(자기 PR이라 거절되면 댓글로), 아니면 댓글. 리뷰도 숨은 표식으로 두 번 달지 않는다
func (r *prRun) review(e *prEnt, key string, approve bool, body string) bool {
	if e.By != "bot" {
		return r.comment(e, key, body)
	}
	switch r.sent[key] {
	case "sent":
		return true
	case "try":
		has, err := r.gh.HasReviewMarker(e.repo(), e.num(), prMark(key))
		if !r.do("리뷰 확인 "+e.Lane, err) {
			return false
		}
		if has {
			r.sent[key] = "sent"
			return true
		}
	}
	if r.stop {
		return false
	}
	r.sent[key] = "try"
	r.save()
	err := r.gh.Review(e.repo(), e.num(), approve, r.mask(body)+"\n\n"+prMark(key))
	if errors.Is(err, ghpr.ErrOwnPR) { // 같은 계정이었다 — 댓글로(앞으로도)
		r.ok++
		e.By = "self"
		delete(r.sent, key)
		return r.comment(e, key, body)
	}
	if !r.do("리뷰 "+e.Lane, err) {
		return false
	}
	r.sent[key] = "sent"
	return true
}

var prResLane = regexp.MustCompile(`레인 ([a-z0-9_-]+)[ ,]`)

// mergerEvents: 열린 PR 레인의 사건(PR을 연 뒤) — 제출·착지·돌려보냄(병합기 기계의 막힘)·병합 쪽 풀이(병합기 「merger」 사건의 레인)
func (r *prRun) mergerEvents() {
	mg := map[string]bool{}
	for _, m := range strings.Fields(MGM) {
		mg[m] = true
	}
	for _, l := range EV {
		if r.stop {
			return
		}
		ev := events.Cols(l)
		lane := ev.Lane
		if ev.Lane == "merger" && strings.HasPrefix(ev.Key, "hc-merger-resolve") {
			if x := prResLane.FindStringSubmatch(ev.Msg + " "); x != nil {
				lane = x[1]
			}
		}
		e := r.entOf(lane)
		if ev.N < 7 || e == nil || e.State != "open" || ev.Time < e.Opened {
			continue
		}
		key := "ev:" + evKey(l)
		switch {
		case ev.Lane == "merger":
			if lane != "merger" {
				r.comment(e, key, "병합 쪽 풀이 — "+ev.Msg)
			}
		case ev.State == "제출" && ev.SubmitID() != "":
			if r.comment(e, key, "제출됨 — 병합기 줄에 넣음(제출 "+ev.SubmitID()+", "+head(ev.SHA, 8)+")") {
				r.selfAudit(e, lane, ev, key)
			}
		case ev.State == "착지":
			r.landed(e, ev, key, mg[ev.Machine])
		case ev.State == "막힘" && mg[ev.Machine] && strings.Contains(ev.Msg, "제출 ID"):
			if r.status(e, key+":st", e.Head, "failure", "verify", ev.Msg) {
				r.comment(e, key, "돌려보냄 — "+ev.Msg+prRelands(lane))
			}
		case ev.State == "정리됨":
			r.swept(e, lane, key)
		}
	}
}

// swept: 「정리됨」(hc-sweep이 터미널을 닫음)으로 끝난 레인의 PR — 그 PR로 가는 레인(원래 레인·이 레인·대기열에서 이어 쓰는 줄) **모두가** 착지·제출
// 없이 끝났을 때만(prSettled) 까닭 댓글 뒤 닫는다(rm 닫기와 같은 길). 이어 쓰는 줄이 아직 안 떴거나·돌고 있거나·제출한 뒤 정리됐으면(병합기가
// 착지할 것) 닫지 않는다 — 하나라도 모르면 닫지 않는다(2026-11-24 pr-mirror-gaps — 열린 채 남아 판정 보고서를 바퀴마다 다시 읽었다. 감사 1차
// 중간 1: 처음엔 이어 쓰는 줄의 터미널 생사만 봐, 제출하자마자 정리된 --pr-of 레인의 착지 전에 원래 PR을 닫았다)
func (r *prRun) swept(e *prEnt, lane, key string) {
	fam := []string{lane}
	if e.Lane != lane {
		fam = append(fam, e.Lane)
	}
	for _, q := range qRows() {
		if n := q.F(qName); n == lane || n == e.Lane || r.entOf(n) == e {
			if !lsGone(q.F(qAddr)) { // 터미널이 표에 있음(같은 이름으로 다시 뜸 포함)·안 떴음·표를 못 믿음
				return
			}
			if !has(fam, n) {
				fam = append(fam, n)
			}
		}
	}
	rep := ""
	for _, n := range fam {
		x, ok := prSettled(n, e.Opened)
		if !ok {
			return
		}
		if n == lane {
			rep = x
		}
	}
	r.close(e, key, "정리 — 착지 없이 끝났다(레인 "+lane+" 터미널이 「"+rep+"」 보고 뒤 닫힘)")
}

// prSettled <레인> <PR 연 때>: 그 레인이 착지·제출 없이 끝났나 — 그 이름의 마지막 사건이 「정리됨」이고(뒤에 다시 띄움·병합기 반송 없음), 그 터미널의
// 마지막 보고가 제출·주차·착지가 아니고, PR 연 뒤 착지가 없음. 참이면 그 마지막 보고. 사건이 없거나 보고가 없으면 모름(거짓)
func prSettled(lane, since string) (string, bool) {
	last := -1
	var lv events.Event
	for i, l := range EV {
		x := events.Cols(l)
		if x.Lane != lane {
			continue
		}
		if x.State == "착지" && x.Time >= since {
			return "", false
		}
		last, lv = i, x
	}
	if last < 0 || lv.State != "정리됨" {
		return "", false
	}
	rep := ""
	for _, l := range EV[:last] {
		if x := events.Cols(l); x.Lane == lane && x.Term == lv.Term && x.State != "정리됨" && x.State != "멈춤" {
			rep = x.State
		}
	}
	return rep, rep != "" && rep != "제출" && rep != "주차" && rep != "착지"
}

// prRelands: 대기열에 넣은 다시 맞춤 레인 이름(있으면 「 — 다시 맞춤 레인 x, y」)
func prRelands(lane string) string {
	var o []string
	for _, q := range qRows() {
		if n := q.F(qName); n != lane && relandSuf.ReplaceAllString(n, "") == lane {
			o = append(o, n)
		}
	}
	if len(o) == 0 {
		return ""
	}
	return " — 다시 맞춤 레인 " + strings.Join(o, ", ")
}

// landed: 착지 — 검증 상태(병합기가 착지 글에 적은 검증 길) + 착지 댓글. 병합기가 PR 머리를 착지 커밋에 둔 것을 알렸으면(「PR 머리 … 들어감」)
// GitHub가 Merged로 표시하니 거울은 merged로만 적고, 아니면(머리 가지 갱신 실패·병합기 밖 착지) 그 까닭 댓글 뒤 닫는다
func (r *prRun) landed(e *prEnt, ev events.Event, key string, byMerger bool) {
	if !r.status(e, key+":st", ev.SHA, "success", "verify", ev.Msg) || !r.comment(e, key, "착지 "+head(ev.SHA, 8)+" — "+ev.Msg) {
		return
	}
	if byMerger && strings.Contains(ev.Msg, "PR 머리 ") && !strings.Contains(ev.Msg, "PR 머리 안 들어감") {
		r.landedState(e, ev, key)
		return
	}
	why := "가지 갱신 실패로 Merged 표시 없음"
	if !byMerger {
		why = "병합기 밖 착지라 Merged 표시 없음"
	}
	r.close(e, key+":close", "착지 "+head(ev.SHA, 8)+" — "+why)
}
