package main

// ht-alert-route(2026-11-27): 주인 없는 사건이 지휘 셋에게 가 지시가 갈렸다(받은편지 레인 반송에 「기다려라」·「지우고 다시 내라」).
// alert_route one이면 watch는 그 저장소(모르면 전역)의 대표 지휘 한 명만 밀어 넣는다 — docs/hc-fill/alert-route.md

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/journal"
	"herdr-tools/internal/knob"
	"herdr-tools/internal/tri"
)

// alertKit: cwdKit 위에 손잡이 저장 파일(HOME)·빈 설정 폴더·일지를 시험 폴더에
func alertKit(t *testing.T) *cwdKit {
	k := newCwdKit(t)
	for _, p := range []string{k.d + "/home", k.d + "/cfg"} {
		if err := os.MkdirAll(p, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", k.d+"/home")
	t.Setenv("HERDR_TOOLS_CONFIG", k.d+"/cfg")
	t.Setenv(journal.Env, k.d+"/journal.jsonl")
	t.Setenv(oneEnv, "1") // 기능은 오래전부터 켜짐 — 켜기 전 사건 거름은 TestAlertEscalateStale에서
	return k
}

// acoord: 살아 있는 지휘 a(뜬 시각 created)가 저장소 repos를 맡는다
func (k *cwdKit) acoord(a string, created int, repos ...string) {
	pid := strconv.Itoa(1000 + created)
	k.write("get."+a, `{"address":"`+a+`","cwd":"/","created_ms":`+strconv.Itoa(created)+`,"foreground":{"name":"node","pid":7},"name":"`+a+`","pid":`+pid+`,"state":"live"}`)
	k.write("st/hc-lane/coord."+a, a+tab+pid+tab+a+tab+"0\n")
	var b strings.Builder
	for _, r := range repos {
		b.WriteString(pid + tab + r + "\n")
	}
	if len(repos) > 0 {
		k.write("st/hc-lane/coordrepo."+a, b.String())
	}
}

func setKnob(t *testing.T, name, v string) {
	t.Helper()
	if err := knob.Set(knob.Path(os.Getenv("HOME")), knob.Entry{Name: name, Value: v, By: "시험", Why: "-", At: time.Now().Unix()}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func aev(k *cwdKit, a string) *ev {
	return &ev{ed: k.d + "/st/hc-lane", wto: a, cur: k.d + "/cur." + a, occ: map[string]tri.State{}, seenc: map[string]seenCache{},
		rgrace: time.Hour, rdefer: map[string]time.Time{}, lrepo: map[string]string{}}
}

// aline: 주인 없는 사건(저장소 r, 시각 at)
func aline(at time.Time, lane, state, r string) string {
	return strings.Join([]string{at.UTC().Format(time.RFC3339), "m", "t8", lane, state, "-", "요약", "-", "-", "-", "*", "-", r}, tab)
}

// who: 같은 사건을 밀어 넣는 지휘(차례대로)
func who(k *cwdKit, l string, cs ...string) string {
	var o []string
	for _, c := range cs {
		if aev(k, c).alertSend(l) {
			o = append(o, c)
		}
	}
	return strings.Join(o, ",")
}

// ② 주인 없는 받은편지 레인 사건 → 그 저장소 대표 한 명(가장 먼저 뜬 지휘). ⑨ all(기본)이면 오늘처럼 셋 다. ⑤ 대표가 끝나면 다음 지휘
func TestAlertRepoRep(t *testing.T) {
	k := alertKit(t)
	k.acoord("ca", 300, cwdA)
	k.acoord("cb", 100, cwdA)
	k.acoord("cc", 200, cwdA)
	l := aline(time.Now(), "inbox-x", "막힘", cwdA)
	if got := who(k, l, "ca", "cb", "cc"); got != "ca,cb,cc" {
		t.Fatalf("⑨ all: %q, 기대 셋 다", got)
	}
	setKnob(t, "alert_route", "one")
	if got := who(k, l, "ca", "cb", "cc"); got != "cb" {
		t.Fatalf("② one: %q, 기대 가장 먼저 뜬 cb만", got)
	}
	e := aev(k, "cb")
	e.alertSend(l)
	if tag := e.rtag[lineKey(l)]; !strings.Contains(tag, "→ 대표 지휘 cb") {
		t.Errorf("② 받는 쪽 한 구절: %q", tag)
	}
	if err := os.Remove(k.d + "/get.cb"); err != nil {
		t.Fatal(err)
	}
	if got := who(k, l, "ca", "cc"); got != "cc" {
		t.Errorf("⑤ 대표 끝남 → 다음으로 먼저 뜬 cc: %q", got)
	}
}

// ④ 저장소 모르는 사건(병합 줄·도구) → 전역 대표(살아 있는 모든 지휘 가운데 가장 먼저 뜬 것)
func TestAlertGlobalRep(t *testing.T) {
	k := alertKit(t)
	setKnob(t, "alert_route", "one")
	k.acoord("ca", 300, cwdA)
	k.acoord("cb", 100, cwdB)
	if got := who(k, aline(time.Now(), "merger-a", "중간", ""), "ca", "cb"); got != "cb" {
		t.Errorf("④ 전역 대표: %q, 기대 cb", got)
	}
}

// ⑥ 그 저장소 맡은 지휘 없음 → 오늘처럼(repoSend) 모두. 대표 후보 조회 모름 → 모두
func TestAlertNoRepFallback(t *testing.T) {
	k := alertKit(t)
	setKnob(t, "alert_route", "one")
	k.acoord("ca", 300, cwdB)
	k.acoord("cb", 100, cwdB)
	if got := who(k, aline(time.Now(), "inbox-x", "막힘", cwdA), "ca", "cb"); got != "ca,cb" {
		t.Errorf("⑥ 맡은 지휘 없음: %q, 기대 모두", got)
	}
	// 대표 후보 cc 조회 모름 → 오늘(all)과 같은 판정(repoSend — 그 저장소를 맡은 cc에게 유예 동안 미룸)
	k.acoord("cc", 50, cwdA)
	k.write("get.cc", "깨진 응답")
	ly := aline(time.Now(), "inbox-y", "막힘", cwdA)
	one := who(k, ly, "ca", "cb", "cc")
	setKnob(t, "alert_route", "all")
	if all := who(k, ly, "ca", "cb", "cc"); one != all {
		t.Errorf("⑥ 후보 모름: one %q ≠ all %q(모름은 오늘처럼)", one, all)
	}
}

// ⑦ alert_coord@<프로젝트> → 그 지휘, 죽었으면 등록 차례
func TestAlertKnobCoord(t *testing.T) {
	k := alertKit(t)
	setKnob(t, "alert_route", "one")
	setKnob(t, "alert_coord@a", "ca")
	k.acoord("ca", 300, cwdA)
	k.acoord("cb", 100, cwdA)
	l := aline(time.Now(), "inbox-x", "막힘", cwdA)
	if got := who(k, l, "ca", "cb"); got != "ca" {
		t.Fatalf("⑦ 손잡이 지휘: %q, 기대 ca", got)
	}
	if got := who(k, aline(time.Now(), "inbox-z", "막힘", cwdB), "ca", "cb"); got != "ca,cb" {
		t.Errorf("⑦ 다른 프로젝트는 손잡이 몫이 아님(B 맡은 지휘 없음 → 모두): %q", got)
	}
	if err := os.Remove(k.d + "/get.ca"); err != nil {
		t.Fatal(err)
	}
	if got := who(k, l, "cb"); got != "cb" {
		t.Errorf("⑦ 손잡이 지휘가 죽음 → 등록 차례 cb: %q", got)
	}
}

// aturn: 지휘 a(acoord로 만든 것)의 hc get에 에이전트 상태 state와 그 상태로 바뀐 때(status_ms)를 싣는다
func (k *cwdKit) aturn(a string, created int, state string, ms time.Time) {
	pid := strconv.Itoa(1000 + created)
	k.write("get."+a, `{"address":"`+a+`","cwd":"/","created_ms":`+strconv.Itoa(created)+`,"foreground":{"name":"node","pid":7},"name":"`+a+
		`","pid":`+pid+`,"state":"live","status":{"app":"claude-code","state":"`+state+`"},"status_ms":`+strconv.FormatInt(ms.UnixMilli(), 10)+`}`)
}

// ⑧ 막힘이 대표에게 간 뒤 alert_escalate_min 동안 대표가 못 봄(턴이 안 돎)·풀림 없음 → 대표 아닌 지휘도 한 번.
// 「봤다」 = 사건 뒤 대표의 hc get status가 바뀜(status_ms)이고 지금 막힘·오류가 아님 — 손 안 대고 넘긴 것도 봄(ht-alert-stale-escalate)
func TestAlertEscalate(t *testing.T) {
	k := alertKit(t)
	setKnob(t, "alert_route", "one")
	k.acoord("ca", 300, cwdA)
	k.acoord("cb", 100, cwdA)
	at := time.Now().Add(-31 * time.Minute)
	k.aturn("cb", 100, "working", at.Add(-time.Minute)) // 사건 앞부터 한 턴이 이어짐 — 아직 못 봄
	l := aline(at, "inbox-x", "막힘", cwdA)
	if got := who(k, l, "ca"); got != "" {
		t.Fatalf("⑧ 대표가 아직 못 봄(유예 안) — 대표 아닌 지휘는 안 보냄: %q", got)
	}
	l10 := aline(time.Now().Add(-10*time.Minute), "inbox-x", "막힘", cwdA)
	k.write("st/hc-lane/seen.cb", lineKey(l)+"\n"+lineKey(l10)+"\n") // 대표 cb의 watch가 보냄
	e := aev(k, "ca")
	if !e.alertSend(l) {
		t.Fatal("⑧ 31분 대표 턴 안 돎 → ca도 보내야 한다")
	}
	if tag := e.rtag[lineKey(l)]; !strings.Contains(tag, "모든 지휘") || !strings.Contains(tag, "cb") {
		t.Errorf("⑧ 받는 쪽 한 구절: %q", tag)
	}
	if got := who(k, l10, "ca"); got != "" {
		t.Errorf("⑧ 10분은 아직: %q", got)
	}
	if aev(k, "ca").alertSend(aline(at, "inbox-x", "중간", cwdA)) {
		t.Error("⑧ 막힘이 아닌 사건은 올려 보내지 않는다")
	}
	k.aturn("cb", 100, "blocked", at.Add(time.Minute))
	if !aev(k, "ca").alertSend(l) {
		t.Error("⑧ 대표가 사건 뒤 막힘(질문 창 등) → 올려 보냄")
	}
	k.aturn("cb", 100, "idle", at.Add(time.Minute))
	if aev(k, "ca").alertSend(l) {
		t.Error("⑧ 대표가 사건 뒤 턴을 돌림(손 안 댔어도) → 안 보냄")
	}
	// 대표 끝남 → 다음 대표 cc: 커서에 있고 사건 뒤 턴 없음 → 올려 보냄
	if err := os.Remove(k.d + "/get.cb"); err != nil {
		t.Fatal(err)
	}
	k.acoord("cc", 200, cwdA)
	k.aturn("cc", 200, "idle", at.Add(-time.Minute))
	k.write("st/hc-lane/seen.cc", lineKey(l)+"\n")
	if !aev(k, "ca").alertSend(l) {
		t.Error("⑧ 다음 대표 cc가 사건 뒤 턴 없음 → 올려 보냄")
	}
	// 풀림: 같은 레인의 뒤 사건이 막힘 아님
	l2 := aline(at, "inbox-q", "막힘", cwdA)
	k.write("st/hc-lane/seen.cc", lineKey(l)+"\n"+lineKey(l2)+"\n")
	e = aev(k, "ca")
	e.win = []string{l2, aline(at.Add(5*time.Minute), "inbox-q", "중간", cwdA)}
	if e.alertSend(l2) {
		t.Error("⑧ 풀림(같은 레인 뒤 사건) → 안 보냄")
	}
}

// 2026-11-27 13:00 재현(ht-alert-stale-escalate): 기능 착지(12:14) 뒤 지휘 t34x에 이미 처리된 사건 다섯 — ht-selfcheck 결정 필요 넷(전날
// 19:20~21:49, mini-m1 known.tsv에 표시)과 on-land-deploy 막힘 하나(12:14:50, 다시 돌려 「같음」) — 가 「모든 지휘(대표 t3yx 30분 조치 없음)」로 왔다.
// 대표가 그 뒤 턴을 돌렸으면 하나도 밀지 않는다. 대표가 턴을 안 돌렸어도 기능이 켜지기(이 watch가 one을 처음 본 때) 전 넷은 세지 않는다
func TestAlertEscalateStale(t *testing.T) {
	k := alertKit(t)
	now := time.Now()
	t.Setenv("HC_LANE_ALERT_ONE_SINCE", strconv.FormatInt(now.Add(-46*time.Minute).Unix(), 10))
	setKnob(t, "alert_route", "one")
	k.acoord("t34x", 300, cwdA)
	k.acoord("t3yx", 100, cwdA)
	sc := func(at time.Time, msg string) string {
		return strings.Replace(aline(at, "ht-selfcheck", "막힘", ""), "요약", msg, 1)
	}
	ls := []string{
		sc(now.Add(-17*time.Hour-40*time.Minute), "결정 필요 새 1건: [ht:dabaed] hc-lane"),
		sc(now.Add(-17*time.Hour), "결정 필요 새 1건: [ht:f83742] hc-merger"),
		sc(now.Add(-17*time.Hour), "결정 필요 새 1건: [ht:b0c19d] hc-merger"),
		sc(now.Add(-15*time.Hour-10*time.Minute), "결정 필요 새 1건: [ht:05a46a] hc-merger"),
		strings.Replace(aline(now.Add(-45*time.Minute), "on-land-deploy", "막힘", ""), "요약", "착지 뒤 deploy 실패(종료 1)", 1),
	}
	var seen strings.Builder
	for _, l := range ls {
		seen.WriteString(lineKey(l) + "\n")
	}
	k.write("st/hc-lane/seen.t3yx", seen.String()) // 대표 t3yx의 watch가 보냄
	push := func() int {
		n := 0
		for _, l := range ls {
			if aev(k, "t34x").alertSend(l) {
				n++
			}
		}
		return n
	}
	k.aturn("t3yx", 100, "idle", now.Add(-5*time.Minute))
	if n := push(); n != 0 {
		t.Errorf("대표가 사건 뒤 턴을 돌림 → 밀림 0, 실제 %d", n)
	}
	k.aturn("t3yx", 100, "working", now.Add(-18*time.Hour))
	if n := push(); n != 1 {
		t.Errorf("대표 턴 안 돎 → 기능 켠 뒤 사건(on-land-deploy) 하나만, 실제 %d", n)
	}
}
