package main

// 병합기 상태 한 장 읽기(ht-merger-state-record, 2026-11-26 사용자 「구조를 다시 짜기」 — docs/hc-lane/merger-state.md). 병합기가 쓴
// 기록(internal/mergerstate)을 run 바퀴가 병합기마다 한 번 받아 merger-alive.tsv 그 줄에 두고(maReadRec), state·advise·status는 그
// 줄만 읽는다. 이 파일은 병합기의 터미널 이름·기록 파일 꼴을 모른다 — 기다리는 대상은 기록의 주소로, 진행은 기록의 진행 시각으로.

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/mergerstate"
	"herdr-tools/internal/opsstate"
)

// maOldMerger: 기록이 없는 병합기(이 판 전에 띄운 병합기) — 짐작하지 않는다
const maOldMerger = "모름(옛 판 병합기 — 다시 띄우면 기록)"

// maReadRec <기계> <병합기 터미널 폴더>: 병합기 기록 한 장(JSON 그대로)과 못 읽은 까닭. 읽는 길은 하나 — hc-merger --state(이 기계면 sh,
// 다른 기계면 hc exec --on, onm). 종료 3 = 기록 없음(옛 판 병합기), 2 = 그 기계 hc-merger가 --state를 모름(옛 판)
func maReadRec(m, clone string) (rec, why string) {
	if clone == "" || clone == "-" {
		return "", "모름(병합기 터미널 폴더 모름)"
	}
	out, rc := onm(nil, m, `PATH="$PATH:$HOME/.local/bin"; exec hc-merger --state "$d"`, clone)
	switch rc {
	case 0:
		s := strings.TrimSpace(out)
		if _, err := mergerstate.Parse([]byte(s)); err != nil {
			return "", "모름(" + err.Error() + ")"
		}
		return s, ""
	case 3:
		return "", maOldMerger
	case 2:
		return "", "모름(그 기계 hc-merger가 옛 판 — --state 없음)"
	}
	return "", fmt.Sprintf("모름(병합기 기록 못 읽음 — 종료 %d)", rc)
}

// maRecOf: 그 줄의 기록(살아 있고 읽은 것만)
func maRecOf(x maRow) (mergerstate.Rec, bool) {
	if x.St != "alive" || x.Rec == "" {
		return mergerstate.Rec{}, false
	}
	r, err := mergerstate.Parse([]byte(x.Rec))
	return r, err == nil
}

// recAddr <기록의 주소> <병합기 기계>: 이 기계 hc ls의 주소 꼴로 — 기계 없는 주소는 병합기 기계의 것, 이 기계 것은 기계를 뗀다
func recAddr(a, mm string) string {
	if a == "" {
		return ""
	}
	m, t, ok := strings.Cut(a, "/")
	if !ok {
		m, t = mm, a
	}
	if m == LOCAL {
		return t
	}
	return m + "/" + t
}

// stateRec: 병합 줄 칸을 기록에서 — 단계·까닭·나이, 마지막 바퀴 = 마지막 진행, 기다리는 대상(살아 있나는 이번 hc ls의 그 주소),
// 착지 안쪽 단계, 착지 단계면 맨 앞 레인·워커. 기록이 없으면 까닭만(판정은 모름)
func stateRec(g *opsstate.MergeLine, x maRow, terms []hcjson.Terminal, termsOK bool, now time.Time) {
	r, ok := maRecOf(x)
	if !ok {
		switch {
		case x.St != "alive":
		case x.RecWhy != "":
			g.Why = x.RecWhy
		default:
			g.Why = "모름(병합기 기록을 아직 안 읽음 — run 바퀴)"
		}
		return
	}
	n := now.Unix()
	ago := func(t int64) *int {
		if t <= 0 {
			return nil
		}
		return ptrI(int(max(0, n-t)))
	}
	g.Phase, g.PhaseWhy, g.PhaseAgeS, g.RecAgeS, g.LastRoundAgeS = r.Phase, r.Why, ago(r.Since), ago(r.At), ago(r.Progress)
	live := map[string]bool{}
	for _, t := range terms {
		if t.State == "live" {
			live[t.Address] = true
		}
	}
	for _, w := range r.Waits {
		a, lv := recAddr(w.Addr, r.Machine), "?"
		if termsOK {
			lv = "gone"
			if live[a] {
				lv = "live"
			}
		}
		g.Waits = append(g.Waits, opsstate.MergeWait{Machine: w.Machine, Addr: a, Role: w.Role, AgeS: ago(w.At), Live: lv})
	}
	if r.Inner != nil && r.Inner.Line != "" {
		g.Inner, g.InnerAgeS = r.Inner.Line, ago(r.Inner.At)
	}
	if r.Landing() && len(r.Members) > 0 {
		g.Landing, g.LandingAgeS = r.Members[0], ago(r.Since)
		if r.Phase == mergerstate.LandOn && len(r.Waits) > 0 {
			g.LandOn = r.Waits[0].Machine
		}
	}
}

// mqBatch <병합기 살아 있음 줄> <지금>: 병합기가 지금 도는 묶음의 구성원 → 단계 덧말 — 추측 검증을 기다리는 묶음은 「묶음 검증 중
// <기계,…> n분째」, 착지 중인 맨 앞 레인은 「착지 중 n분째[ — <안쪽 단계> m분째]」. 기록이 없으면 nil(차례만 보인다)
// ponytail: 착지는 맨 앞 레인만 — 묶음 착지의 뒤 구성원은 차례만 보인다
func mqBatch(x maRow, now time.Time) map[string]string {
	r, ok := maRecOf(x)
	if !ok {
		return nil
	}
	mins := func(t int64) int64 { return max(0, now.Unix()-t) / 60 }
	inner := ""
	if r.Inner != nil && r.Inner.Line != "" {
		inner = fmt.Sprintf(" — %s %d분째", r.Inner.Line, mins(r.Inner.At))
	}
	o := map[string]string{}
	switch {
	case r.Phase == mergerstate.SpecWait || r.Phase == mergerstate.LoadWait:
		var ms []string
		for _, w := range r.Waits {
			if !has(ms, w.Machine) {
				ms = append(ms, w.Machine)
			}
		}
		s := fmt.Sprintf("묶음 검증 중 %s %d분째", cmp.Or(strings.Join(ms, ","), "-"), mins(r.Since))
		if r.Phase == mergerstate.LoadWait {
			s += " — 부하 대기" // 까닭(기계·부하·기준)은 state·status 병합기 줄의 단계 글
		}
		for _, l := range r.Members {
			o[l] = s + inner
		}
	case r.Landing() && len(r.Members) > 0:
		o[r.Members[0]] = fmt.Sprintf("착지 중 %d분째", mins(r.Since)) + inner
	}
	return o
}

// maRecLine: status 병합기 줄 끝 — 기록의 단계·마지막 진행·병합기 차례, 착지 실행(지금 워커 착지면 「<기계> <나이>째(<까닭>)」, 아니면
// 마지막 결정). 기록을 못 읽었으면 그 까닭
func maRecLine(x maRow, now int64) string {
	r, ok := maRecOf(x)
	if !ok {
		if x.St == "alive" && x.RecWhy != "" {
			return " · 기록 " + x.RecWhy
		}
		return ""
	}
	l := " · " + cmp.Or(r.Why, r.Phase) + " " + stDur(now-r.Since) + "째 · 마지막 진행 " + stDur(now-r.Progress) + " 전"
	if len(r.Queue) > 0 {
		l += " · 병합기 차례 " + strings.Join(r.Queue, ",")
	}
	switch {
	case r.Phase == mergerstate.LandOn && len(r.Waits) > 0:
		w := r.Waits[0].Machine
		l += " · 착지 실행: " + w + " " + stDur(now-r.Since) + "째" + strings.TrimPrefix(r.LandOn, w)
	case r.LandOn != "":
		l += " · 착지 실행: " + r.LandOn
	}
	return l
}
