package main

// state의 병합 줄 칸(2026-11-24 ops-state-trim): 병합기가 이 기계(LOCAL)에 있는 프로젝트는 이 기계 병합기 일지
// (<XDG_STATE_HOME, 없으면 ~/.local/state>/hc-lane/merger-journal.tsv — hc-merger result.go jrec)에서 마지막 바퀴·최근 검증·건너뜀·맡김을
// 읽는다(읽기만, 원격 호출 없음). 다른 기계 병합기는 모름+까닭(원격으로 읽지 않는다).

import (
	"os"
	"time"

	"herdr-tools/internal/opsstate"
)

// stateJournalN: 최근 착지·맡김을 셀 수(hc-lane merger --why 기본과 같은 20)
const stateJournalN = 20

// stateMergeJournal: 병합 줄 하나를 채우는 함수. 일지는 한 번만 읽는다. 이 기계 병합기 프로젝트가 둘 이상이면 일지 줄(프로젝트 칸 없음)을
// 대기열 줄의 레인 → 프로젝트로 가른다
// ponytail: 대기열에서 정리된 레인의 일지 줄은 프로젝트를 몰라 둘 이상일 때 빠진다(덜 셈) — 일지에 프로젝트 칸이 생기면 그것으로
func stateMergeJournal(mr map[string]maRow, rows []Row) func(g *opsstate.MergeLine, x maRow, now time.Time) {
	local := 0
	for _, x := range mr {
		if x.M == LOCAL {
			local++
		}
	}
	proj := map[string]string{}
	for _, r := range rows {
		proj[r.F(1)] = r.F(2)
	}
	var ls []string
	bad := ""
	if local > 0 {
		dir := os.Getenv("XDG_STATE_HOME")
		if dir == "" {
			dir = HOME + "/.local/state"
		}
		b, err := os.ReadFile(dir + "/hc-lane/merger-journal.tsv")
		switch {
		case os.IsNotExist(err):
			bad = "이 기계 병합기 일지 없음(merger-journal.tsv)"
		case err != nil:
			bad = "이 기계 병합기 일지 못 읽음: " + err.Error()
		default:
			ls = lines(string(b))
		}
	}
	return func(g *opsstate.MergeLine, x maRow, now time.Time) {
		if x.M != LOCAL {
			m := dash0(x.M)
			if m == "" {
				m = "모름"
			}
			g.Why = "병합기 바퀴 시각·검증 시간·건너뜀은 병합기 기계(" + m + ") 일지에만 — 원격으로 안 읽음"
			return
		}
		if bad != "" {
			g.Why = bad
			return
		}
		keep := func(string) bool { return true }
		if local > 1 {
			keep = func(l string) bool { return proj[l] == g.Project }
		}
		f := opsstate.MergeJournal(ls, stateJournalN, keep)
		g.Lands, g.Skipped, g.Delegated = ptrI(f.Lands), ptrI(f.Skipped), ptrI(f.Delegated)
		g.RecentVerifyS = f.VerifyS
		if f.Lands > 0 {
			g.SkipRatio = ptrF(float64(f.Skipped) / float64(f.Lands))
		}
		if f.LastAt.IsZero() {
			g.Why = "이 기계 병합기 일지에 이 프로젝트 줄 없음"
			return
		}
		g.LastRoundAgeS = ptrI(int(now.Sub(f.LastAt).Seconds()))
	}
}
