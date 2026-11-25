//go:build !windows

package main

// merger-journal-why(2026-11-24): 병합기 일지 착지·반송 줄의 까닭 칸에 검증 길(「검증 건너뜀(추측 <아이디 8> · <기계>)」 또는
// 「직접 검증(<까닭>)」)을 붙이고, 일시 실패 판독은 착지 자신의 마지막 멈춤 줄만 본다(검증 출력 속 같은 글을 일시 실패로 읽지 않는다).
// 틀은 specrun_test.go(specFx)·specpre_test.go(preFx)·merger_test.go(newFx).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// jwhy: 병합기 일지에서 그 레인·결과 줄의 까닭 칸(없으면 빈 값)
func (f *fx) jwhy(lane, res string) string {
	b, _ := os.ReadFile(gEvdir + "/merger-journal.tsv")
	for _, l := range strings.Split(string(b), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 && c[2] == lane && c[9] == res {
			return c[11]
		}
	}
	return ""
}

// skipFx: preFx에 진짜 검증 스크립트처럼 건너뛸 때 「건너뜀 검증」 한 줄을 찍게 한다
func skipFx(t *testing.T) *sfx {
	s := preFx(t)
	old, _ := os.ReadFile(gSelf)
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), "echo \"fast|$HC_LANE\" >>\"$LANDLOG\"\n",
		"echo \"fast|$HC_LANE\" >>\"$LANDLOG\"\n    echo \"건너뜀 검증 — 이 트리($t)는 이미 통과(refs/herdr/verified)\"\n", 1)), 0o777)
	return s
}

// ① 미리 검증이 통과한 같은 sha의 빠른 착지 → 일지에 「검증 건너뜀(추측 <아이디 8> · v1)」
func TestJournalWhyFastSkip(t *testing.T) {
	s := skipFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" {
		t.Fatalf("빠른 착지여야: %q", got)
	}
	if w, want := s.jwhy("p", "착지"), "병합기 착지, 검증 건너뜀(추측 "+d[0][1][:8]+" · v1)"; w != want {
		t.Fatalf("① 일지 까닭 %q, 바람 %q", w, want)
	}
}

// ② 직접 검증 까닭: 추측 없음
func TestJournalWhyNoSpec(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.round(1)
	if w := s.jwhy("a", "착지"); w != "병합기 착지, 직접 검증(추측 없음)" {
		t.Fatalf("② 추측 없음: %q", w)
	}
	// 추측을 아예 켜지 않은 저장소(검증 기계 목록·추측 참조 없음)도 같다
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	whyLand(t, "ok", 0)
	f.submit(1000000001, "b", "보통")
	f.merger().round()
	if w := f.jwhy("b", "착지"); w != "병합기 착지, 직접 검증(추측 없음)" {
		t.Fatalf("② 추측을 안 켠 저장소: %q", w)
	}
}

// ② 직접 검증 까닭: 묶음
func TestJournalWhyBatch(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(2)
	for _, l := range []string{"a", "b"} {
		if w := s.jwhy(l, "착지"); !strings.HasPrefix(w, "병합기 착지(묶음 2: a+b), 직접 검증(묶음)") {
			t.Fatalf("② 묶음 %s: %q", l, w)
		}
	}
}

// ② 직접 검증 까닭: 기준 움직임(미리 검증은 통과했지만 제출 전에 기준이 움직임)
func TestJournalWhyBaseMoved(t *testing.T) {
	oldFront(t)
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	s.moveMain("moved")
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if w := s.jwhy("p", "착지"); w != "병합기 착지, 직접 검증(기준 움직임)" {
		t.Fatalf("② 기준 움직임: %q", w)
	}
}

// ② 직접 검증 까닭: 추측 실패 — 반송 줄도 같은 꼴
func TestJournalWhySpecFailReject(t *testing.T) {
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), true)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("띄움: %v", d)
	}
	s.verifyOn(d[0])
	s.idle("v2", verifyBusy) // 한가한 다른 기계가 없어야 직접 검증(있으면 먼저 그 기계에 — specreuse_test.go)
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if w := s.jwhy("p", "돌려보냄"); w != "착지 실패(종료 1), 직접 검증(추측 실패)" {
		t.Fatalf("② 추측 실패 반송: %q", w)
	}
}

// ② 직접 검증 까닭: 맨 앞 기다림 상한
func TestJournalWhyFrontWaitCap(t *testing.T) {
	s, _ := overCap(t)
	if w := s.jwhy("p", "착지"); w != "병합기 착지, 직접 검증(맨 앞 기다림 상한)" {
		t.Fatalf("② 맨 앞 기다림 상한: %q", w)
	}
}

// ② 맨 앞 추측 상태 → 까닭(모름은 검증 기계에 띄운 추측이 시간 넘김·안 뜸 — 병합기가 직접 지은 맨 앞(기계 -)은 추측 없음).
// 맨 앞도 맡기면(disp — merger-verify-dispatch) 줄이 없는 것은 빈 검증 기계가 없었다는 뜻, 기계 - 줄은 못 띄움·준비 실패(모름)
func TestFrontWhyTable(t *testing.T) {
	g := newGroup([]entry{{s: "s1", l: "a"}})
	g2 := newGroup([]entry{{s: "s1", l: "a"}, {s: "s2", l: "b"}})
	row := func(st specState, mach, parent, group string) *specRow {
		return &specRow{id: "0123456789abcdef", tree: "t", parent: parent, group: group, machine: mach, state: st}
	}
	for _, c := range []struct {
		name string
		st   []specStep
		rows []specRow
		over bool
		disp bool
		want string
	}{
		{"줄 없음", []specStep{{g: g, parent: "B"}}, nil, false, false, "추측 없음"},
		{"병합기가 지은 맨 앞", []specStep{{g: g, parent: "B", row: row(specLost, "-", "B", "s1")}}, nil, false, false, "추측 없음"},
		{"검증 기계 추측 시간 넘김", []specStep{{g: g, parent: "B", row: row(specLost, "v1", "B", "s1")}}, nil, false, false, "모름"},
		{"충돌", []specStep{{g: g, parent: "B", row: row(specConflict, "-", "B", "s1")}}, nil, false, false, "모름"},
		{"실패", []specStep{{g: g, parent: "B", row: row(specFail, "v1", "B", "s1")}}, nil, false, false, "추측 실패"},
		{"도는 중 상한 넘김", []specStep{{g: g, parent: "B", row: row(specRun, "v1", "B", "s1")}}, nil, true, false, "맨 앞 기다림 상한"},
		{"묶음", []specStep{{g: g2, parent: "B"}}, nil, false, false, "묶음"},
		{"옛 기준의 추측", []specStep{{g: g, parent: "B"}}, []specRow{*row(specPass, "v1", "A", "s1")}, false, false, "기준 움직임"},
		{"다른 레인의 옛 추측", []specStep{{g: g, parent: "B"}}, []specRow{*row(specPass, "v1", "A", "s9")}, false, false, "추측 없음"},
		{"맡김 · 줄 없음", []specStep{{g: g, parent: "B"}}, nil, false, true, "맡길 한가한 기계 없음"},
		{"맡김 · 묶음 줄 없음", []specStep{{g: g2, parent: "B"}}, []specRow{*row(specPass, "v1", "A", "s1")}, false, true, "맡길 한가한 기계 없음"},
		{"맡김 · 바쁨(다시 고를 기계 없음)", []specStep{{g: g, parent: "B", row: row(specBusyX, "v2", "B", "s1")}}, nil, false, true, "맡길 한가한 기계 없음"},
		{"맡김 · 못 띄움(기계 -)", []specStep{{g: g, parent: "B", row: row(specLost, "-", "B", "s1")}}, nil, false, true, "모름"},
		{"맡김 · 실패", []specStep{{g: g, parent: "B", row: row(specFail, "v1", "B", "s1")}}, nil, false, true, "추측 실패"},
	} {
		if got := frontWhy(c.st, c.rows, c.over, c.disp); got != c.want {
			t.Errorf("%s: %q, 바람 %q", c.name, got, c.want)
		}
	}
}

// ② 빠른 착지를 받은 뒤 착지 로그로 마저 가름: 건너뜀 · 기준 움직임(rebase한 트리가 다름) · 모름(기록 없음·건너뜀 줄 없음)
func TestVtagFromLandLog(t *testing.T) {
	d := t.TempDir()
	m := &M{vw: vwhy{why: "통과", id: "0123abcd", mach: "v1"}, lgf: filepath.Join(d, "l.log")}
	for _, c := range []struct{ log, want string }{
		{"hc-lane land: 이 트리는 이미 검증 통과(x) — HERDR_VERIFIED_TREE를 검증에 넘긴다\n건너뜀 검증 — 이 트리(t)는 이미 통과(refs/herdr/verified)\n", ", 검증 건너뜀(추측 0123abcd · v1)"},
		{"hc-lane land: 이 트리는 이미 검증 통과(x) — HERDR_VERIFIED_TREE를 검증에 넘긴다\n▸ 건너뜀 — 검증 기록 x (병합기: 같은 트리·같은 검증 스크립트의 원격 통과 기록)\n", ", 검증 건너뜀(추측 0123abcd · v1)"},
		{"hc-lane land: 추측 트리(abc)와 rebase한 트리가 다르다 — 검증을 그대로 돈다\nok\n", ", 직접 검증(기준 움직임)"},
		{"hc-lane land: 추측 트리 abc 의 검증 기록이 없다(또는 검증 스크립트가 다름) — 검증을 그대로 돈다\n", ", 직접 검증(모름)"},
		{"    x_test.go:3: 건너뜀 검증 — 시험 출력\n", ", 직접 검증(모름)"},
	} {
		os.WriteFile(m.lgf, []byte(c.log), 0o666)
		if got := m.vtag(); got != c.want {
			t.Errorf("%q → %q, 바람 %q", c.log, got, c.want)
		}
	}
	if got := (&M{}).vtag(); got != "" {
		t.Errorf("검증 길을 모르는 착지(끊긴 앞 실행 정리 등)는 까닭을 붙이지 않는다: %q", got)
	}
}

// whyLand: newFx의 가짜 hc-lane에서 land는 out을 찍고 rc로 끝난다(rc 0이면 진짜처럼 기본 브랜치에 push)
func whyLand(t *testing.T, out string, rc int) {
	t.Helper()
	body := "if [ \"$1\" = land ]; then\n  printf '%s\\n' \"$FAKE_LAND_OUT\"\n"
	if rc == 0 {
		body += "  git fetch -q origin && git rebase -q origin/main && git push -q --atomic origin HEAD:refs/heads/main $HERDR_LAND_PUSH_EXTRA || exit 1\n"
	}
	body += "  exit " + strconv.Itoa(rc) + "\nfi\n"
	t.Setenv("FAKE_LAND_OUT", out)
	old, _ := os.ReadFile(gSelf)
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), "#!/bin/sh\n", "#!/bin/sh\n"+body, 1)), 0o777)
}

// ③ 검증 출력(시험 실패가 찍은 대상 출력)에 「git fetch 실패」·「push 결과 모름」 글이 있어도 착지 자신의 멈춤 줄이 검증 실패면
// 일시 실패가 아니다 — 큐에 두고 세 번 다시 하지 않고 이 바퀴에 바로 반송(고치기 전 코드는 일시 실패로 읽어 큐에 남겼다)
func TestTransIgnoresVerifyOutput(t *testing.T) {
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	whyLand(t, strings.Join([]string{
		"--- FAIL: TestLandFetch (0.01s)",
		"    land_test.go:12: 출력: hc-lane: land: git fetch 실패",
		"hc-lane: land: git fetch 실패",
		"hc-lane: land: push 결과 모름 — 원격 main 와 견주지 못했다",
		"hc-lane land: 실패 줄(로그에서, 최대 40줄):",
		"--- FAIL: TestLandFetch (0.01s)",
		"hc-lane: land: 검증 실패, push하지 않았다. 고쳐 커밋한 뒤 같은 줄을 다시",
	}, "\n"), 1)
	q, _, _, _ := f.submit(1000000001, "a", "보통")
	f.merger().round()
	if f.has(q) || !f.has("refs/merge-failed/1000000001-a") {
		t.Fatal("③ 검증 실패는 이 바퀴에 바로 반송해야 한다(일시 실패로 큐에 남김)")
	}
	if w := f.jwhy("a", "돌려보냄"); w != "착지 실패(종료 1), 직접 검증(추측 없음)" {
		t.Fatalf("③ 반송 까닭: %q", w)
	}
}

// ④ 진짜 원격 받기 실패(착지의 마지막 멈춤 줄)는 지금처럼 일시 실패 — 큐에 둔다
func TestTransRealFetchFail(t *testing.T) {
	f := newFx(t)
	f.git(f.seed, "push", "-q", "origin", "main:refs/herdr/merger/box")
	whyLand(t, "fatal: unable to access 'origin'\nhc-lane: land: git fetch 실패", 1)
	q, _, _, _ := f.submit(1000000001, "a", "보통")
	f.merger().round()
	if !f.has(q) || f.has("refs/merge-failed/1000000001-a") {
		t.Fatal("④ 진짜 fetch 실패는 일시 실패로 큐에 둬야 한다")
	}
}

// ④ 착지 멈춤 줄의 꼴별 — 내장 길(hc-lane: …)과 저장소 착지 스크립트(▸ 착지 멈춤: …), 그 뒤에 붙는 hc-lane land 안내 줄
func TestTransLastStopLine(t *testing.T) {
	d := t.TempDir()
	m := &M{lr: "1", lgf: filepath.Join(d, "l.log")}
	for _, c := range []struct {
		log  string
		want bool
	}{
		{"hc-lane: land: git fetch 실패\n", true},
		{"hc-lane: land: push 결과 모름 — git ls-remote에 refs/heads/main 줄이 하나가 아니다. 들어갔는지 확인한 뒤 같은 줄을 다시\n", true},
		{"hc-lane: land: push 실패(다른 쪽 push가 앞선 것이 아니다 — 위 git 출력을 본다)\n", true},
		{"▸ 착지 멈춤: push 실패 뒤 git fetch origin 실패(위 git 오류)\nhc-lane land: 실패 줄(로그에서, 최대 40줄):\nhc-lane land: 전체 로그 x\n", true},
		{"hc-lane: land: git fetch 실패\n▸ 착지 멈춤: 검증 실패\n", false},
		{"git fetch 실패\nhc-lane: land: 검증 실패, push하지 않았다\n", false},
		{"  hc-lane: land: git fetch 실패\n", false},
		{"", false},
	} {
		os.WriteFile(m.lgf, []byte(c.log), 0o666)
		if got := m.trans(); got != c.want {
			t.Errorf("%q → %v, 바람 %v", c.log, got, c.want)
		}
	}
	m.lgf = filepath.Join(d, "없음.log")
	if m.trans() {
		t.Error("로그를 못 읽으면 일시 실패로 보지 않는다(지금과 같음)")
	}
}

// ⑤ 일지 요약(hc-lane merger --why): 후보 SHA로 묶은 최근 N번의 착지 — 건너뜀·직접 검증 까닭별·까닭 없음(옛 줄), 착지 시간 중앙값
func TestWhySummary(t *testing.T) {
	at := func(s string) string {
		if s == "-" {
			return s
		}
		return "2026-11-24T" + s + "Z"
	}
	j := func(lane, t0, t1, cand, why string) string {
		return strings.Join([]string{"2026-11-24T10:00:00Z", "1795339000-" + lane, lane, "-", "-", "-", "1", at(t0), at(t1), "착지", cand, why}, "\t")
	}
	A, B, C, D, E := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40)
	lines := []string{
		j("old", "09:00:00", "09:05:00", E, "병합기 착지"), // 옛 줄 — N=4 밖
		j("a", "10:00:00", "10:00:40", A, "병합기 착지, 검증 건너뜀(추측 0123abcd · v1)"),
		j("b", "10:10:00", "10:14:00", B, "병합기 착지(묶음 2: b+c), 직접 검증(묶음), 승인 확인 판정 x patch-id y"),
		j("c", "10:10:00", "10:14:00", B, "병합기 착지(묶음 2: b+c), 직접 검증(묶음), 승인 확인 판정 x patch-id y"),
		strings.Replace(j("r", "10:20:00", "10:21:00", "-", "착지 실패(종료 1), 직접 검증(추측 없음)"), "\t착지\t", "\t돌려보냄\t", 1),
		j("d", "10:30:00", "10:36:00", C, "병합기 착지, 직접 검증(추측 없음)"),
		j("x", "-", "-", D, "제출이 이미 main 에 있음"),
		j("e", "11:00:00", "11:02:00", D, "앞 병합기 실행이 넣었음"),
	}
	want := "최근 착지 4번: 검증 건너뜀 1 · 직접 검증 2(묶음 1 · 추측 없음 1) · 까닭 없음 1 · 착지 시간 중앙 — 건너뜀 40s · 직접 4m0s"
	if got := whySummary(lines, 4); got != want {
		t.Fatalf("⑤ 요약\n받음 %s\n바람 %s", got, want)
	}
	if got := whySummary(nil, 20); got != "최근 착지 0번" {
		t.Fatalf("빈 일지: %q", got)
	}
}
