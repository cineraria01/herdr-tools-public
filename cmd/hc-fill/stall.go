package main

// 완료 뒤 멈춤 알림(Go 판에서 더함) — run 바퀴가 audit_tick 다음에 stallTick을 부른다

import (
	"fmt"
	"strings"
	"time"
)

// stallDone <대기열 줄> <완료 사건> <완료 sha>: 멈춤이 아니라 끝난 레인인가(2026-11-21 헛알림 — flaky-land 1420분·land-turn-short
// 3999분 등 넷 다 터미널이 닫혔거나 착지할 것이 없었다). ① ls의 「끝남」(그 주소에 그 이름 터미널 없음) ② 커밋 없는 완료(사건 글
// 「커밋 0」) ③ 완료 sha가 이 기계 클론의 기본 브랜치에 들어 있음(조사만 한 레인은 sha가 띄울 때 기준이라 여기 걸린다). ③을 모르면
// (fetch·merge-base 실패) 알리지 않는다. 그 sha가 이 클론에 없으면(남의 기계에만 있는 커밋) 기본 브랜치에도 없다 — 알린다
func stallDone(q Row, ev, sha string) bool {
	if !lalive(q.F(8), q.F(1)) || strings.Contains(evF(ev, 7), "커밋 0") {
		return true
	}
	if lcFetch(q.F(2), true) != 0 {
		return true // 기본 브랜치를 모름
	}
	// 「이 클론에 없음」은 rev-parse 종료 1에 출력이 없을 때만 — 그 밖의 실패·시간 넘김은 모름(알리지 않는다, gofix-fill F12·L5)
	if o, rc := (Cmd{Name: "git", Args: []string{"-C", FR, "rev-parse", "--verify", "--quiet", sha + "^{commit}"}, Err: errMerge}).Run(); rc != 0 {
		return rc != 1 || strings.TrimSpace(o) != ""
	}
	_, rc := run("git", "-C", FR, "merge-base", "--is-ancestor", sha, FB)
	return rc != 1
}

// stallTick(Go 판에서 더함 — 받은편지 레인이 「완료 — 감사 기다림」으로 9시간 멈춘 일 뒤): up 줄(감사 줄 빼고)의 그 실행 마지막
// 사건이 「완료」이고 그 시각 뒤로 감사·착지·제출 없이 설정 done_stall분(기본 60, 0이면 끔) 지났으면 그 레인 주인 지휘에 한 번(키
// hf:<레인>:stall:<완료 sha> — hc-lane이 한 번만 보낸다). 그 sha 차수가 끝 상태가 아니면 알리지 않는다. 끝난 차수(수거됨 등)가 있으면
// 그 차수의 마지막 수거 확인·깨운 시각(13·14칸)부터 센다(감사가 오래 걸려 수거 직후 착지 중인 레인을 멈춤으로 보지 않게).
// dry-run·사건 못 읽음·끊긴 기계는 건너뜀. run 바퀴가 audit_tick 다음에 부른다
func stallTick() {
	lim := auMinutes("done_stall", 60)
	if lim == 0 || dry || EVBAD || !nonEmpty(Q) {
		return
	}
	unk := evUnk()
	at := readRows(AT)
	now := time.Now()
	for _, q := range qRows() {
		n, m, a := q.F(1), q.F(7), q.F(8)
		if q.F(6) != "up" || strings.HasPrefix(q.F(15), "audit=") || n == "" {
			continue
		}
		if m != LOCAL && (!connHas(m) || has(unk, m)) {
			continue
		}
		l := joblast(n, a, "")
		if evF(l, 5) != "완료" {
			continue
		}
		t0, err := time.Parse("2006-01-02T15:04:05Z", evF(l, 1))
		if err != nil {
			continue
		}
		s, open := evF(l, 6), false
		for _, r := range at {
			if r.NF() < 17 || r.F(2) != n || !auSameRound(r, s) {
				continue
			}
			if !aterm(r.F(8)) {
				open = true
				break
			}
			for _, c := range []string{r.F(13), r.F(14)} {
				if isDigits(c) {
					if ts := time.Unix(int64(atoi10(c)), 0); ts.After(t0) {
						t0 = ts
					}
				}
			}
		}
		mins := int(now.Sub(t0) / time.Minute)
		if open || mins < lim || stallDone(q, l, s) {
			continue
		}
		own := dash(q.F(12))
		if _, _, inbox := replyOf(q.F(11)); inbox {
			own = "-"
		}
		aesc(n, own, fmt.Sprintf("%s 완료 뒤 멈춤 — %d분째 감사·착지 없음 (%s, 완료 %s)", n, mins, a, head(s, 8)), "hf:"+n+":stall:"+s)
	}
}
