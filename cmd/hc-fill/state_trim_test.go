package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/opsstate"
)

// ops-state-trim(2026-11-24): 병합 줄 절을 이 기계에 있는 병합기 일지로 채운다(읽기만, 원격 호출 없음), 다른 기계 병합기는 모름+까닭.
// 배포 기록에 건너뜀만 있는 기계는 「대상 아님」(모름이 아님).

// stMerge: 병합기 살아 있음 기록(프로젝트 app, 병합기 기계 mach)과 hc-lane 상태 폴더(XDG_STATE_HOME)
func stMerge(t *testing.T, mach string) string {
	t.Helper()
	stFx(t, `{"machines":[{"name":"w","state":"connected"},{"name":"x","state":"connected"}]}`)
	now := time.Now().Unix()
	if err := os.WriteFile(DATA+"/merger-alive.tsv", []byte(strings.Join([]string{"app", mach, "alive", itoa(now - 600), "-", "1", "-", itoa(now - 30), "-", "-"}, "\t")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := t.TempDir()
	t.Setenv("XDG_STATE_HOME", st)
	if err := os.MkdirAll(st+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	return st + "/hc-lane/merger-journal.tsv"
}

func jrow(at time.Time, lane, res, cand, why string, sec int) string {
	u := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	return strings.Join([]string{u(at), "1795000000-" + lane, lane, "id", "-", "-", "1", u(at.Add(-time.Duration(sec) * time.Second)), u(at), res, cand, why}, "\t")
}

// ⑤ 이 기계 병합기: 일지가 있으면 최근 착지 시간(이력)을 채우고 「아직 안 모음」 까닭은 없다. 마지막 바퀴는 일지 마지막 줄이 아니라
// 병합기 기록의 마지막 진행 시각(ht-merger-state-record — 일지 마지막 줄 2분 전, 기록 진행 40분 전 → 40분)
func TestStateMergeLocalJournal(t *testing.T) {
	jf := stMerge(t, "m")
	now := time.Now()
	n := now.Unix()
	if err := os.WriteFile(DATA+"/merger-alive.tsv", []byte(strings.Join([]string{"app", "m", "alive", itoa(n - 600), "-", "1", "-", itoa(n - 30), "-", "-", "-", "-",
		`{"v":1,"at":` + itoa(n) + `,"progress_at":` + itoa(n-2400) + `,"phase":"round","phase_since":` + itoa(n-2400) + `}`, "-"}, "\t")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ls := jrow(now.Add(-20*time.Minute), "lane-a", "착지", "c1", "착지, 검증 건너뜀(추측 abcd1234 · w)", 300) + "\n" +
		jrow(now.Add(-2*time.Minute), "lane-a", "추측 기다림", "-", "추측 검증 기다림 2바퀴, 검증 맡김(w) 60초 — 구성원 lane-a", 60) + "\n"
	if err := os.WriteFile(jf, []byte(ls), 0o644); err != nil {
		t.Fatal(err)
	}
	s := stateCollect(now)
	if len(s.Merge) != 1 {
		t.Fatalf("병합 줄: %+v", s.Merge)
	}
	g := s.Merge[0]
	if g.LastRoundAgeS == nil || *g.LastRoundAgeS < 2390 || *g.LastRoundAgeS > 2410 || g.RecentVerifyS == nil || *g.RecentVerifyS != 300 ||
		g.SkipRatio == nil || *g.SkipRatio != 1 || strings.Contains(g.Why, "아직 안 모음") {
		t.Errorf("이 기계 병합기 일지: %+v", g)
	}
}

// ⑤ 이 기계 병합기인데 일지가 없으면 모름 + 까닭(일지 없음)
func TestStateMergeLocalNoJournal(t *testing.T) {
	stMerge(t, "m")
	g := stateCollect(time.Now()).Merge[0]
	if g.LastRoundAgeS != nil || g.RecentVerifyS != nil || !strings.Contains(g.Why, "일지 없음") {
		t.Errorf("일지 없음: %+v", g)
	}
}

// 다른 기계 병합기는 지금처럼 모름 + 까닭(그 기계 일지에만 — 원격으로 안 읽음), 이 기계 일지가 있어도 쓰지 않는다
func TestStateMergeRemote(t *testing.T) {
	jf := stMerge(t, "w")
	os.WriteFile(jf, []byte(jrow(time.Now(), "lane-a", "착지", "c1", "착지", 60)+"\n"), 0o644)
	g := stateCollect(time.Now()).Merge[0]
	if g.LastRoundAgeS != nil || g.RecentVerifyS != nil || !strings.Contains(g.Why, "병합기 기계") {
		t.Errorf("다른 기계 병합기: %+v", g)
	}
}

// 배포 기록에 건너뜀만 있는 기계는 대상 아님(모름 아님), 실패만 있는 기계는 그대로 모름
func TestStateDeployNotTarget(t *testing.T) {
	stFx(t, "")
	if err := os.MkdirAll(HOME+"/.local/state/herdr-sync", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(HOME+"/.local/state/herdr-sync/deploy.tsv", []byte("2026-11-24T10:00:00+09:00\t배포\t0\taaaaaaaaaaaa\ts\tc\tby\tp=씀 q=건너뜀 r=실패\n"), 0o644)
	var s opsstate.State
	stateDeploy(&s, time.Now())
	by := map[string]opsstate.Deploy{}
	for _, d := range s.Deploy {
		by[d.Machine] = d
	}
	if by["q"].Why != opsstate.DeployNotTarget || by["r"].Why == opsstate.DeployNotTarget || by["r"].Version != "" {
		t.Errorf("배포: %+v", s.Deploy)
	}
}
