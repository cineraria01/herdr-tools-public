package main

// 완료 뒤 멈춤 알림(Go 판에서 더함) — run 바퀴가 audit_tick 다음에 stallTick을 부른다

import (
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/tri"
)

// stallDone <대기열 줄> <완료 사건> <완료 sha>: 멈춤이 아니라 끝난 레인인가 — (끝남, 모름)(2026-11-21 헛알림 — flaky-land 1420분·
// land-turn-short 3999분 등 넷 다 터미널이 닫혔거나 착지할 것이 없었다). ① ls의 「끝남」(그 주소에 그 이름 터미널 없음) ② 커밋 없는 완료
// (사건 글 「커밋 0」) ③ 완료 sha가 이 기계 클론의 기본 브랜치에 들어 있음(조사만 한 레인은 sha가 띄울 때 기준이라 여기 걸린다)
// ④ 병합기가 맡음(stallMerger). ③을 모르면(fetch·merge-base 실패) 지금처럼 알리지 않는다. ④를 모르면(병합기 참조를 못 읽음) 알리되
// 글에 「(착지 판정 못 함)」(2026-11-22 stall-false-alarm). 그 sha가 이 클론에 없으면(남의 기계에만 있는 커밋) 기본 브랜치에도 없다
func stallDone(q Row, ev, sha string) (bool, bool) {
	if !lalive(q.F(qAddr), q.F(qName)) || strings.Contains(evF(ev, evMsg), "커밋 0") {
		return true, false
	}
	if lcFetch(q.F(qProj), true) != 0 {
		return true, false // 기본 브랜치를 모름
	}
	// 「이 클론에 없음」은 rev-parse 종료 1에 출력이 없을 때만 — 그 밖의 실패·시간 넘김은 모름(알리지 않는다, gofix-fill F12·L5)
	if o, rc := (Cmd{Name: "git", Args: []string{"-C", FR, "rev-parse", "--verify", "--quiet", sha + "^{commit}"}, Err: errMerge}).Run(); rc != 0 {
		if rc != 1 || strings.TrimSpace(o) != "" {
			return true, false
		}
	} else if _, rc := run("git", "-C", FR, "merge-base", "--is-ancestor", sha, FB); rc != 1 {
		return true, false
	}
	return stallMerger(q, ev, sha)
}

// stallMerger <대기열 줄> <완료 사건> <완료 sha>: 병합기가 그 레인을 맡았나 — (맡음, 모름)(2026-11-22 헛알림 둘 — d1-inbox-claim은
// 병합기 묶음 착지로 들어갔고(rebase해 완료 sha가 기본 브랜치의 조상이 아님), sv-shell-remove-reland 완료 sha는 반송된 뒤 자동 다시
// 맞춤이 이어받았다). (a) 착지: 레인 표식 refs/hc-landed/<레인>/<초>가 그 줄 접수(없으면 완료) 뒤이고 기본 브랜치에 듦 — 병합기는
// 묶음이어도 구성원마다 refs/merge-done/<레인>과 이 표식을 한 원자 push로 둔다(result.go done) (b) 줄에 있음: 큐 refs/merge-queue/
// *-<레인>이나 그 sha의 큐 항목(착지 중 표식 refs/herdr/landing은 큐 항목이 있을 때만 착지 중 — internal/landing) (c) 반송 뒤
// 이어받음: refs/merge-failed/<이름>이 그 sha이고 reland.tsv에 그 이름의 「넣음」(다시 맞춤 레인)이나 「보냄」(원래 레인에 보내 다시
// 내기를 기다림 — 안 내면 reland가 넣는다). 반송 큐 이름의 레인 칸은 다시 맞춤 레인이 아니라 가지 이름일 수 있어 sha로 맞춘다.
// 원격 없는 저장소(기본이 refs/heads/)에는 병합기가 없다. lcFetch가 FR·FB를 채운 뒤 부른다.
// (a)는 onlandHit — 표식에 더해 기본 브랜치의 접수 뒤 커밋 제목 머리 「<이름>:」도 착지로 본다(2026-11-22 지휘 — clean-renamed-submit 등
// 착지한 레인에 경보). 이름은 레인 이름과 그 레인이 낸 이름(stallNames). refs/merge-done/<레인>은 hc-landed 표식과 한 push라 따로 보지
// 않는다(시각이 없어 같은 이름 옛 기록을 이번 착지로 볼 수 있다)
func stallMerger(q Row, ev, sha string) (bool, bool) {
	if !strings.HasPrefix(FB, "origin/") {
		return false, false
	}
	p, n := q.F(qProj), q.F(qName)
	st, ok := since(n)
	if !ok {
		t, err := time.Parse("2006-01-02T15:04:05Z", evF(ev, evTime))
		if err != nil {
			return false, true
		}
		st = fmt.Sprint(t.Unix())
	}
	lns, unk := stallNames(q)
	for _, ln := range lns {
		lms, lmf := LMS, LMFROM // lmark(onlandHit 안)는 선행 판정용 전역을 쓴다 — 이 바퀴의 다른 판정을 흐리지 않게 되돌린다
		_, hit := onlandHit(ln, int64(num(st)))
		LMS, LMFROM = lms, lmf
		switch hit {
		case tri.Yes:
			return true, false
		case tri.Unknown:
			unk = true
		case tri.No:
		}
	}
	same := func(full string) bool { return len(sha) >= 7 && strings.HasPrefix(full, sha) }
	switch mqueued(p, n) {
	case 0:
		return true, false
	case 2:
		unk = true
	default:
		for _, r := range MQS {
			if r.F(mqProj) == p && strings.HasPrefix(r.F(mqRef), "refs/merge-queue/") && same(r.F(mqSHA)) {
				return true, false
			}
		}
	}
	o, rc2 := run("git", "-C", FR, "ls-remote", "origin", "refs/merge-failed/*")
	if rc2 != 0 {
		return false, true
	}
	var names []string
	for _, l := range lines(o) {
		if f := fields(l); len(f) == 2 && same(f[0]) {
			names = append(names, strings.TrimPrefix(f[1], "refs/merge-failed/"))
		}
	}
	if len(names) > 0 {
		for _, r := range readRows(DATA + "/reland.tsv") {
			if has(names, r.F(rlFailed)) && (r.F(rlWhat) == "넣음" || r.F(rlWhat) == "보냄") {
				return true, false
			}
		}
	}
	return false, unk
}

// stallNames <대기열 줄>: 그 레인이 착지에 쓸 수 있는 이름들 — 레인 이름, 다시 맞춤 레인이면 reland.tsv의 원래 레인·바탕(HC_LANE=<원래>로
// 낸다), 이 클론의 가지 설정 branch.<레인>.hcLane(hc-lane이 이름 바꿔 낸 제출에 남김 — 이 기계 레인만 보인다). 설정 조회가 「없음」(종료 1)
// 말고 실패하면 모름
func stallNames(q Row) ([]string, bool) {
	n := q.F(qName)
	names := []string{n}
	b, _, err := atomic.Read(DATA + "/reland.tsv")
	for _, l := range lines(string(b)) {
		if r := splitRow(l); r.F(rlArg) == n && r.F(rlWhat) == "넣음" {
			names = append(names, r.F(rlLane), r.F(rlBase))
		}
	}
	o, rc := run("git", "-C", FR, "config", "--get", "branch."+n+".hcLane")
	switch v := strings.TrimSpace(o); {
	case rc == 0 && okname(v):
		names = append(names, v)
	case rc != 1:
		return uniqOK(names), true
	}
	return uniqOK(names), err != nil
}

// uniqOK: 겹침·빈 값·이름 꼴이 아닌 것을 뺀다
func uniqOK(a []string) []string {
	var out []string
	for _, s := range uniq(a) {
		if okname(s) {
			out = append(out, s)
		}
	}
	return out
}

// stallTick(Go 판에서 더함 — 받은편지 레인이 「완료 — 감사 기다림」으로 9시간 멈춘 일 뒤): up 줄(감사 줄 빼고)의 그 실행 마지막
// 사건이 「완료 <sha>」(sha 없음·「-」은 뺀다)이고 그 시각 뒤로 감사·착지·제출 없이 설정 done_stall분(기본 60, 0이면 끔) 지났으면 그 레인 주인 지휘에 한 번(키
// hf:<레인>:stall:<완료 sha> — hc-lane이 한 번만 보낸다). 그 sha 차수가 끝 상태가 아니면 알리지 않는다. 끝난 레인·병합기가 맡은 레인(stallDone)도 알리지 않는다. 끝난 차수(수거됨 등)가 있으면
// 그 차수의 마지막 수거 확인·깨운 시각(13·14칸)부터 센다(감사가 오래 걸려 수거 직후 착지 중인 레인을 멈춤으로 보지 않게).
// dry-run·사건 못 읽음·끊긴 기계는 건너뜀. run 바퀴가 audit_tick 다음에 부른다
func stallTick() {
	// 병합기 살아 있음(mergeralive.go)을 먼저 — stallMerger가 「큐에 있음 = 병합기가 맡음」으로 보는 것은 병합기가 살아 있을 때만 맞다
	mergerAliveTick()
	unreachTick() // 레인 있는 기계가 오래 닿지 않음(unreach.go)
	// 멈춤 판정을 손댈 것보다 먼저 — 턴이 끝난 첫 바퀴에 hang.tsv b-live(배경 일 기다림)가 아직 없어 lane-done이 나갔다(감사 atp2gk8-mt7ny8 중간)
	hangTick(time.Now())     // 「생각 중」에 멈춤·배경 셸이 소리 없이 죽음(hang.go)
	adviseTick(time.Now())   // 지금 손댈 것에 새 항목이면 사건 한 번(advise.go)
	opsAgentTick(time.Now()) // 켜져 있고 새 손댈 것이 있으면 운영 에이전트 한 바퀴, 도는 것은 끝을 본다(opsagent.go)
	lim := auMinutes("done_stall", 60)
	if lim == 0 || dry || EVBAD || !nonEmpty(Q) {
		return
	}
	unk := evUnk()
	at := readRows(AT)
	now := time.Now()
	for _, q := range qRows() {
		n, m, a := q.F(qName), q.F(qMach), q.F(qAddr)
		if q.F(qState) != "up" || strings.HasPrefix(q.F(qAudit), "audit=") || n == "" {
			continue
		}
		if m != LOCAL && (!connHas(m) || has(unk, m)) {
			continue
		}
		l := joblast(n, a, "")
		if evF(l, evState) != "완료" {
			continue
		}
		t0, err := time.Parse("2006-01-02T15:04:05Z", evF(l, evTime))
		if err != nil {
			continue
		}
		s, open := evF(l, evSHA), false
		if s == "" || s == "-" { // 커밋 없는 완료(감사·조사 레인의 「완료 -」) — 감사할 것도 착지할 것도 없다(2026-11-23 stall-no-sha)
			continue
		}
		for _, r := range at {
			if r.NF() < atNF || r.F(atLane) != n || !auSameRound(r, s) {
				continue
			}
			if !aterm(r.F(atState)) {
				open = true
				break
			}
			for _, c := range []string{r.F(atChecked), r.F(atWoke)} {
				if isDigits(c) {
					if ts := time.Unix(int64(atoi10(c)), 0); ts.After(t0) {
						t0 = ts
					}
				}
			}
		}
		mins := int(now.Sub(t0) / time.Minute)
		if open || mins < lim {
			continue
		}
		done, unk := stallDone(q, l, s)
		if done {
			continue
		}
		tail := ""
		if unk {
			tail = " (착지 판정 못 함)"
		}
		own := dash(q.F(qOwner))
		if _, _, inbox := replyOf(q.F(qReply)); inbox {
			own = "-"
		}
		what := "감사·착지" // 자동 감사 줄(감사 칸 auto)만 감사를 기다린다 — 그 밖(no-audit·감사 꺼짐·받은편지)은 착지만
		if q.F(qAudit) != "auto" {
			what = "착지"
		}
		aesc(n, own, fmt.Sprintf("%s 완료 뒤 멈춤 — %d분째 %s 없음 (%s, 완료 %s)%s", n, mins, what, a, head(s, 8), tail), "hf:"+n+":stall:"+s)
	}
}
