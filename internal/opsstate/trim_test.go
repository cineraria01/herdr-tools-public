package opsstate

// ops-state-trim(2026-11-24 — 배포 직후 hc-fill state가 942줄이었다: 레인 절 472줄 가운데 끝난 줄 291·그 밖 180, 감시 14줄 중 12줄
// 「안 돎」, 배포 대상 아닌 기계 「모름」, 건강 「모름」 되풀이). 기본 출력은 지금 움직이는 것만, 나머지는 개수로(Trim).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const doneMin = 60

// big: 끝난 레인 300줄(두 시간 전)·움직이는 레인 10줄·안 도는 감시 12·도는 감시 1·배포 대상 아님 1·건강 모름 기계 6
func big() (State, []string) {
	s := fake()
	s.Hidden = nil // 모은 그대로(추리기 전)
	s.Machines = nil
	for i := 0; i < 6; i++ {
		s.Machines = append(s.Machines, Machine{Name: fmt.Sprintf("m%d", i), Reach: "ok", Load1: fp(1), Cores: ip(8), LoadPerCore: fp(0.12),
			Live: ip(1), Seats: ip(3), Lanes: ip(1), LaneCap: ip(3)})
	}
	active := []string{"작업 중(working)", "감사 중(1번째)", "감사 대기(자리 없음 5분)", "제출됨(병합기 줄 맨 앞)", "주차", "대기",
		"막힘: 시험 실패", "완료 abcd1234", "모름(연결 안 됨)", "감사 통과 받음"}
	s.Lanes = nil
	var names []string
	for i := 0; i < 300; i++ {
		st := "끝남"
		if i%2 == 0 {
			st = "착지 abcd1234"
		}
		s.Lanes = append(s.Lanes, Lane{Name: fmt.Sprintf("old-%03d", i), Machine: "m0", Stage: st, StageAgeS: ip(2 * 3600), LastEvent: "정리됨"})
	}
	for i, st := range active {
		n := fmt.Sprintf("live-%02d", i)
		names = append(names, n)
		s.Lanes = append(s.Lanes, Lane{Name: n, Machine: "m1", Stage: st, StageAgeS: ip(10 * 60), LastEvent: "중간"})
	}
	s.Watch = []Watch{{Owner: "t-live", Alive: bp(true)}}
	for i := 0; i < 12; i++ {
		s.Watch = append(s.Watch, Watch{Owner: fmt.Sprintf("t-dead%d", i), Alive: bp(false)})
	}
	s.Deploy = []Deploy{{Machine: "m0", Version: "abc1234", Behind: ip(0)}, {Machine: "skip", Why: DeployNotTarget}}
	return s, names
}

func human(t *testing.T, s State) string {
	t.Helper()
	var b bytes.Buffer
	if err := Human(&b, s); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// ① 끝난 300·움직이는 10 → 기본 출력 80줄 안, 움직이는 10줄 모두, 숨긴 수 한 줄. 감시·배포·건강도 개수 한 줄로
func TestTrimLineBudget(t *testing.T) {
	s, names := big()
	tr := Trim(s, doneMin)
	tr.Notable = Notable(tr, th)
	out := human(t, tr)
	if n := strings.Count(out, "\n"); n > 80 {
		t.Errorf("기본 출력 %d줄(80줄 안이어야)", n)
	}
	for _, n := range names {
		if !strings.Contains(out, n) {
			t.Errorf("움직이는 레인 %s 없음", n)
		}
	}
	for _, w := range []string{"끝난 레인 300줄 숨김(--all)", "안 도는 지휘 흔적 12개", "대상 아님 1대", "건강 값 없음 — 점검이 아직 안 남김", "t-live"} {
		if !strings.Contains(out, w) {
			t.Errorf("기본 출력에 %q 없음:\n%s", w, out)
		}
	}
	for _, w := range []string{"old-", "t-dead", "잠자기", "  skip  "} {
		if strings.Contains(out, w) {
			t.Errorf("기본 출력에 숨길 %q 있음", w)
		}
	}
}

// 건강: 하나라도 값이 있으면 그 기계만 건강 칸, 나머지 줄은 되풀이하지 않는다
func TestTrimHealthOnlyKnown(t *testing.T) {
	s, _ := big()
	s.Machines[2].Health = Health{Sleep: "off", Keychain: "ok"}
	out := human(t, Trim(s, doneMin))
	if strings.Count(out, "잠자기") != 1 || !strings.Contains(out, "잠자기 off") || strings.Contains(out, "건강 값 없음") {
		t.Errorf("건강 칸:\n%s", out)
	}
}

// ② --all(Trim 안 함)은 지금의 전체 출력 — 끝난 줄·안 도는 감시·대상 아님·건강 모름 모두, 숨긴 수 없음
func TestTrimAllIsFull(t *testing.T) {
	s, _ := big()
	out := human(t, s)
	if c := strings.Count(out, "  old-"); c != 300 {
		t.Errorf("--all 끝난 레인 %d줄", c)
	}
	if strings.Contains(out, "숨김") || strings.Count(out, "안 돎") != 12 || !strings.Contains(out, "  skip  ") || strings.Count(out, "잠자기 모름") != 6 {
		t.Errorf("--all 전체가 아님:\n%s", out)
	}
	var j bytes.Buffer
	JSON(&j, s)
	var m map[string]any
	json.Unmarshal(j.Bytes(), &m)
	if h, ok := m["hidden"]; !ok || h != nil {
		t.Errorf("--all json hidden은 null이어야: %v", h)
	}
}

// ③ 지휘가 봐야 하는 줄(막힘·모름·거절·감사 대기)은 나이와 무관하게 남는다. 끝난 줄은 N분 안이면 남고, 나이를 모르면 남는다
func TestTrimKeepsOldBlocked(t *testing.T) {
	s := fake()
	s.Lanes = []Lane{
		{Name: "blk", Stage: "막힘: 시험 실패", StageAgeS: ip(20 * 3600)},
		{Name: "unk", Stage: "모름(연결 안 됨)", StageAgeS: ip(20 * 3600)},
		{Name: "rej", Stage: "대기(띄우기 거절)", StageAgeS: ip(20 * 3600)},
		{Name: "aud", Stage: "감사 대기(자리 없음 300분)", StageAgeS: ip(20 * 3600)},
		{Name: "fresh", Stage: "착지 abcd1234", StageAgeS: ip(30 * 60)},
		{Name: "noage", Stage: "끝남"},
		{Name: "old", Stage: "착지 abcd1234 · 대기열이 제출함", StageAgeS: ip(3 * 3600)},
	}
	tr := Trim(s, doneMin)
	var got []string
	for _, l := range tr.Lanes {
		got = append(got, l.Name)
	}
	if strings.Join(got, ",") != "blk,unk,rej,aud,fresh,noage" || tr.Hidden == nil || tr.Hidden.Lanes != 1 {
		t.Errorf("남은 줄 %v 숨김 %+v", got, tr.Hidden)
	}
	if !Finished(s.Lanes[6]) || Finished(s.Lanes[0]) || !Finished(s.Lanes[5]) {
		t.Errorf("Finished")
	}
}

// ④ 지문은 추린 것 기준 — 숨긴 줄이 늘거나 빠지고, 끝난 줄이 창 밖으로 나가도(나이만 변함) 그대로. 숨긴 수는 지문에 없다
func TestTrimFingerprintStable(t *testing.T) {
	a, _ := big()
	a.Lanes = append(a.Lanes, Lane{Name: "just-landed", Machine: "m1", Stage: "착지 eeee1111", StageAgeS: ip(50 * 60), LastEvent: "착지 eeee1111"})
	fpOf := func(s State) string {
		tr := Trim(s, doneMin)
		tr.Notable = Notable(tr, th)
		return Fingerprint(tr)
	}
	b, _ := big()
	b.Lanes = append(b.Lanes, Lane{Name: "just-landed", Machine: "m1", Stage: "착지 eeee1111", StageAgeS: ip(70 * 60), LastEvent: "착지 eeee1111"})
	b.Lanes = b.Lanes[5:] // 숨긴 끝난 줄 다섯이 대기열에서 빠짐
	for i := range b.Lanes {
		if Finished(b.Lanes[i]) && b.Lanes[i].Name != "just-landed" {
			b.Lanes[i].StageAgeS = ip(5 * 3600)
		}
	}
	b.Watch = b.Watch[:5] // 안 도는 감시 흔적 수가 바뀜
	if fa, fb := fpOf(a), fpOf(b); fa != fb {
		t.Errorf("숨긴 줄만 바뀌었는데 지문이 다름 %s %s", fa, fb)
	}
	// 움직이는 줄이 바뀌면 지문도 바뀐다
	b.Lanes[len(b.Lanes)-2].Stage = "막힘: 새로"
	if fpOf(a) == fpOf(b) {
		t.Errorf("움직이는 줄이 바뀌었는데 지문이 같음")
	}
}

// ⑥ json도 같은 규칙 — 기본은 추린 것과 숨긴 개수 칸
func TestTrimJSONHidden(t *testing.T) {
	s, _ := big()
	var j bytes.Buffer
	if err := JSON(&j, Trim(s, doneMin)); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Hidden *struct {
			Lanes  *int `json:"lanes"`
			Watch  *int `json:"watch"`
			Deploy *int `json:"deploy"`
		} `json:"hidden"`
		Lanes []Lane `json:"lanes"`
	}
	if err := json.Unmarshal(j.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	h := m.Hidden
	if h == nil || h.Lanes == nil || *h.Lanes != 300 || h.Watch == nil || *h.Watch != 12 || h.Deploy == nil || *h.Deploy != 1 || len(m.Lanes) != 10 {
		t.Errorf("json 숨김: %s", j.String())
	}
}

// ⑤ 병합기 일지(이 기계): 마지막 줄 시각·마지막 착지의 착지 시간·최근 착지 수·건너뜀·맡김. 다른 레인(keep false)·「이미 … 에 있음」은 안 센다
func TestMergeJournal(t *testing.T) {
	row := func(at, lane, t0, t1, res, cand, why string) string {
		return strings.Join([]string{at, "1795000000-" + lane, lane, "id", "-", "-", "1", t0, t1, res, cand, why}, "\t")
	}
	ls := []string{
		row("2026-11-24T10:00:00Z", "x", "2026-11-24T09:50:00Z", "2026-11-24T10:00:00Z", "착지", "c1", "착지, 직접 검증(추측 없음)"),
		row("2026-11-24T10:30:00Z", "y", "2026-11-24T10:29:00Z", "2026-11-24T10:30:00Z", "착지", "c2", "착지, 검증 건너뜀(추측 abcd1234 · v)"),
		row("2026-11-24T10:31:00Z", "y", "2026-11-24T10:29:00Z", "2026-11-24T10:30:00Z", "착지", "c2", "착지, 검증 건너뜀(추측 abcd1234 · v)"),
		row("2026-11-24T10:40:00Z", "z", "2026-11-24T10:35:00Z", "2026-11-24T10:40:00Z", "추측 기다림", "-", "추측 검증 기다림 3바퀴, 검증 맡김(v) 90초 — 구성원 z"),
		row("2026-11-24T10:45:00Z", "z", "2026-11-24T10:44:00Z", "2026-11-24T10:45:00Z", "착지", "c3", "이미 main 에 있음"),
		row("2026-11-24T11:00:00Z", "other", "2026-11-24T10:00:00Z", "2026-11-24T11:00:00Z", "착지", "c9", "착지, 검증 건너뜀(추측 x · v)"),
		"broken",
	}
	f := MergeJournal(ls, 20, func(l string) bool { return l != "other" })
	if f.LastAt.Format("15:04") != "10:45" || f.VerifyS == nil || *f.VerifyS != 60 || f.Lands != 2 || f.Skipped != 1 || f.Delegated != 1 {
		t.Errorf("%+v %v", f, f.VerifyS)
	}
	if e := MergeJournal(nil, 20, func(string) bool { return true }); !e.LastAt.IsZero() || e.VerifyS != nil || e.Lands != 0 {
		t.Errorf("빈 일지: %+v", e)
	}
}
