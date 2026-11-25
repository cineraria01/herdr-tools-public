package opsstate

// 기본 출력 추리기(2026-11-24 ops-state-trim, docs/ops/state.md 「기본과 --all」) — 배포 직후 hc-fill state가 942줄이었다.
// 기본은 지금 움직이는 것만 남기고 나머지는 개수(Hidden)로 센다. 지휘가 봐야 하는 줄(막힘·거절·감사 대기·모름)은 나이와 무관하게 남긴다.

import (
	"strings"
	"time"
)

// Hidden: 기본 출력이 숨긴 개수(json hidden, --all이면 null)
type Hidden struct {
	Lanes  int `json:"lanes"`
	Watch  int `json:"watch"`
	Deploy int `json:"deploy"`
}

// DeployNotTarget: 배포 기록에 건너뜀만 있는 기계의 까닭(대상 아님)
const DeployNotTarget = "대상 아님(배포에서 건너뜀만)"

// Finished: 끝난 레인 — 더 할 일이 없다고 모으는 쪽이 정했거나(Done: 단계 글이 「감사 통과 받음」으로 남아도, stale-resident-check
// 지휘 지시 — 착지·정리까지 끝난 레인 179줄이 기본 출력에 남았다), 단계 글이 착지·끝남(뒤에 「· 대기열이 제출함」이 붙어도).
// 터미널이 살아 있으면(Open) 끝난 레인이 아니다
func Finished(l Lane) bool {
	return !l.Open && (l.Done || strings.HasPrefix(l.Stage, "착지") || strings.HasPrefix(l.Stage, "끝남"))
}

// Trim: 기본 출력. 끝난 레인은 마지막 사건이 doneMin분 안이면 남기고(나이 모름도 남김 — 오래됐다고 말할 수 없다), 안 도는 감시와
// 배포 대상 아닌 기계는 센다. s는 바꾸지 않는다
func Trim(s State, doneMin int) State {
	h := &Hidden{}
	t := s
	t.Lanes = []Lane{}
	for _, l := range s.Lanes {
		if Finished(l) && l.StageAgeS != nil && *l.StageAgeS > doneMin*60 {
			h.Lanes++
			continue
		}
		t.Lanes = append(t.Lanes, l)
	}
	t.Watch = []Watch{}
	t.goneWatch = append([]Watch(nil), s.goneWatch...) // 두 번 추려도 남는다
	for _, w := range s.Watch {
		if w.Alive != nil && !*w.Alive {
			h.Watch++
			t.goneWatch = append(t.goneWatch, w) // 출력에선 숨겨도 손댈 것 「감시 안 돎」은 본다(advise-fixes)
			continue
		}
		t.Watch = append(t.Watch, w)
	}
	t.Deploy = []Deploy{}
	for _, d := range s.Deploy {
		if d.Why == DeployNotTarget {
			h.Deploy++
			continue
		}
		t.Deploy = append(t.Deploy, d)
	}
	t.Hidden = h
	return t
}

// JournalFacts: 병합기 일지에서 읽은 것. LastAt 영 = 줄 없음, VerifyS nil = 시작·끝이 적힌 착지 없음
type JournalFacts struct {
	LastAt                    time.Time
	VerifyS                   *float64
	Lands, Skipped, Delegated int
	// 마지막 줄이 워커 착지 실행 결정(LandRemote)이면 그 기계·레인·줄 시각 — 병합기가 그 워커의 착지를 기다리는 중. 아니면 빈 값
	LandOn, LandOnLane string
	LandOnAt           time.Time
}

// LandRemote: 일지 줄(12칸)이 착지를 워커에서 돌리기로 한 「착지 실행」 줄이면 그 기계(hc-merger landon.go landExec — 까닭 칸
// 「<기계>(<까닭>) — 구성원 …」). 병합기 기계에서 돌림(까닭이 localTag 「병합기 기계 · 」로 시작 — 그때는 착지 자식 기록이 잡는다)·
// 「<기계> 착지 실행 끊김(…)」·다른 결과는 빈 값
func LandRemote(c []string) string {
	if len(c) < 12 || c[9] != "착지 실행" || strings.Contains(c[11], " 착지 실행 끊김(") {
		return ""
	}
	mach, why, ok := strings.Cut(c[11], "(")
	if !ok || mach == "" || strings.ContainsAny(mach, " \t") || strings.HasPrefix(why, "병합기 기계 · ") {
		return ""
	}
	return mach
}

// MergeJournal: 병합기 일지 줄(hc-merger result.go jrec의 12칸) → 마지막 줄 시각, 최근 n번 착지(후보 SHA로 묶음, 「이미 … 에 있음」
// 정리는 뺌 — hc-merger whySummary와 같은 셈) 수·그중 검증 건너뜀 수·마지막 착지의 착지 시간(시작 → 끝, 검증 포함), 최근 n번 검증 맡김
// (추측 기다림 줄, 시작·까닭이 같으면 한 번), 마지막 줄이 워커 착지 실행이면 그것. keep(레인)이 false인 줄은 보지 않는다
func MergeJournal(lines []string, n int, keep func(lane string) bool) JournalFacts {
	var f JournalFacts
	type land struct {
		skip bool
		sec  *float64
	}
	var ls []land
	seen := map[string]bool{}
	type wait struct{ t0, why string }
	waits := 0
	wseen := map[wait]bool{}
	for _, l := range lines {
		c := strings.Split(l, "\t")
		if len(c) < 12 || !keep(c[2]) {
			continue
		}
		at, err := time.Parse(time.RFC3339, c[0])
		if err == nil {
			f.LastAt = at
		}
		f.LandOn, f.LandOnLane, f.LandOnAt = "", "", time.Time{}
		if m := LandRemote(c); m != "" && err == nil {
			f.LandOn, f.LandOnLane, f.LandOnAt = m, c[2], at
		}
		switch {
		case c[9] == "추측 기다림" && strings.Contains(c[11], ", 검증 맡김("):
			if w := (wait{c[7], c[11]}); !wseen[w] {
				wseen[w] = true
				waits++
			}
		case c[9] == "착지" && !seen[c[10]] && !(strings.Contains(c[11], "이미 ") && strings.Contains(c[11], " 에 있음")):
			seen[c[10]] = true
			x := land{skip: strings.Contains(c[11], ", 검증 건너뜀(")}
			t0, e0 := time.Parse(time.RFC3339, c[7])
			t1, e1 := time.Parse(time.RFC3339, c[8])
			if e0 == nil && e1 == nil {
				v := t1.Sub(t0).Seconds()
				x.sec = &v
			}
			ls = append(ls, x)
		}
	}
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	f.Lands = len(ls)
	for _, x := range ls {
		if x.skip {
			f.Skipped++
		}
	}
	if len(ls) > 0 {
		f.VerifyS = ls[len(ls)-1].sec
	}
	f.Delegated = min(waits, n)
	return f
}
