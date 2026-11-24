package opsstate

// ops-state 준비 시험(2026-11-24, 선행 fill-load-steer 착지 전 — 구현 전이라 컴파일부터 실패한다).
// 순수 부분만: 가짜 State → 사람용 표·json·지문·눈에 띄는 것. 모으기(원격 호출·스냅숏)는 cmd/hc-fill 쪽 시험에서.

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func fp(f float64) *float64 { return &f }
func ip(i int) *int         { return &i }
func bp(b bool) *bool       { return &b }

var t0 = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

// fake: 모든 칸이 찬 가짜 상태(기계 이름은 시험용 a·b·u)
func fake() State {
	return State{
		Schema: 1,
		At:     t0,
		Sources: []Source{
			{Name: "tick", AgeS: ip(40), Calls: 0},
			{Name: "remote", AgeS: ip(0), Calls: 3, Secs: 1.5},
		},
		Machines: []Machine{
			{Name: "a", Reach: "ok", Load1: fp(37.2), Cores: ip(12), LoadPerCore: fp(3.1), Live: ip(3), Seats: ip(3), Lanes: ip(2), LaneCap: ip(3),
				Health: Health{Sleep: "off", Keychain: "ok", DiskFreeGB: fp(120), Tailscale: "ok"}, Steer: "자리 2(지휘, 30분)", SkipWhy: ""},
			{Name: "b", Reach: "ok", Load1: fp(0.8), Cores: ip(8), LoadPerCore: fp(0.1), Live: ip(0), Seats: ip(3), Lanes: ip(0), LaneCap: ip(3),
				Health: Health{Sleep: "off", Keychain: "ok", DiskFreeGB: fp(80), Tailscale: "ok"}},
			MachineUnknown("u", "연결 안 됨"),
		},
		Queue: Queue{Counts: map[string]int{"wait": 4, "up": 2, "sent": 0},
			Oldest: &Waiting{Lane: "lane-old", AgeS: 7200, Why: "선행 lane-x"}},
		Lanes: []Lane{
			{Name: "lane-stuck", Machine: "a", Stage: "작업 중(working)", StageAgeS: ip(95 * 60), LastEvent: "중간 시험 통과", Open: true},
			{Name: "lane-ok", Machine: "b", Stage: "완료 abcd1234", StageAgeS: ip(5 * 60), LastEvent: "완료", PR: "PR #7 https://example.org/pull/7"},
		},
		Merge: []MergeLine{{Project: "p", Length: ip(2), Merger: "alive", LastRoundAgeS: ip(30), RecentVerifyS: fp(410), SkipRatio: fp(0.25)}},
		Deploy: []Deploy{
			{Machine: "a", Version: "abc1234", Behind: ip(4)},
			{Machine: "u", Why: "연결 안 됨"},
		},
		Watch:   []Watch{{Owner: "t1", Alive: bp(true)}, {Owner: "t2"}},
		Knobs:   []Knob{{Name: "idle", Value: "20", By: "t1", Why: "밤", Until: "2026-11-24 18:00"}},
		Journal: []string{"12:00 knob set idle 20"},
		Events:  []string{"11:58 lane-ok 완료"},
		Hidden:  &Hidden{Lanes: 3, Watch: 1},
	}
}

var th = Thresholds{StageMin: 90, SkewHi: 2.0, SkewLo: 0.5}

// ① 사람용·json이 같은 사실을 담는다
func TestHumanJSONSameFacts(t *testing.T) {
	s := fake()
	s.Notable = Notable(s, th)
	var h, j bytes.Buffer
	if err := Human(&h, s); err != nil {
		t.Fatal(err)
	}
	if err := JSON(&j, s); err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(j.Bytes(), &back); err != nil {
		t.Fatalf("json을 다시 못 읽음: %v", err)
	}
	for _, w := range []string{"a", "b", "u", "3.10", "lane-stuck", "lane-old", "선행 lane-x", "abc1234", "idle", "밤", s.Fingerprint} {
		if !strings.Contains(h.String(), w) {
			t.Errorf("사람용에 %q 없음", w)
		}
		if !strings.Contains(j.String(), w) {
			t.Errorf("json에 %q 없음", w)
		}
	}
	for _, n := range s.Notable {
		if !strings.Contains(h.String(), n) {
			t.Errorf("사람용에 눈에 띄는 것 %q 없음", n)
		}
	}
	if len(back.Notable) != len(s.Notable) || back.Fingerprint != s.Fingerprint {
		t.Errorf("json 왕복: notable %d→%d fp %s→%s", len(s.Notable), len(back.Notable), s.Fingerprint, back.Fingerprint)
	}
}

// ② 닿지 못한 기계 → 모름(0·정상으로 적지 않는다)
func TestUnknownMachine(t *testing.T) {
	m := MachineUnknown("u", "연결 안 됨")
	if m.Reach != "unknown" || m.Why != "연결 안 됨" || m.LoadPerCore != nil || m.Live != nil || m.Seats != nil {
		t.Fatalf("모름 기계: %+v", m)
	}
	var j bytes.Buffer
	if err := JSON(&j, State{Schema: 1, At: t0, Machines: []Machine{m}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.String(), `"load_per_core":null`) || !strings.Contains(j.String(), `"reach":"unknown"`) {
		t.Errorf("json 모름 꼴: %s", j.String())
	}
	var h bytes.Buffer
	if err := Human(&h, State{Schema: 1, At: t0, Machines: []Machine{m}}); err != nil {
		t.Fatal(err)
	}
	line := ""
	for _, l := range strings.Split(h.String(), "\n") {
		if strings.Contains(l, "u") && strings.Contains(l, "연결 안 됨") {
			line = l
		}
	}
	if !strings.Contains(line, "모름") || strings.Contains(line, "0.00") || strings.Contains(line, "0/") {
		t.Errorf("사람용 모름 줄: %q\n%s", line, h.String())
	}
}

// ③ 지문: 판단 칸이 같으면 같고, 다르면 다르다. 나이·시각·잰 값(부하·검증 초)만 달라서는 안 바뀐다
func TestFingerprint(t *testing.T) {
	a := fake()
	b := fake()
	b.At = t0.Add(7 * time.Minute)
	b.Sources[0].AgeS = ip(100)
	b.Lanes[0].StageAgeS = ip(96 * 60)
	b.Queue.Oldest.AgeS = 9000
	b.Merge[0].LastRoundAgeS = ip(5)
	b.Merge[0].RecentVerifyS = fp(380)
	b.Machines[0].Load1 = fp(36.0)
	b.Machines[0].LoadPerCore = fp(3.0)
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatalf("나이·시각만 다른데 지문이 바뀜: %s %s", Fingerprint(a), Fingerprint(b))
	}
	if len(Fingerprint(a)) < 8 || len(Fingerprint(a)) > 16 {
		t.Errorf("지문 길이 %d (짧은 값이어야)", len(Fingerprint(a)))
	}
	for name, f := range map[string]func(*State){
		"레인 단계":   func(s *State) { s.Lanes[0].Stage = "완료 ffff0000" },
		"대기 수":    func(s *State) { s.Queue.Counts["wait"] = 5 },
		"기계 닿음":   func(s *State) { s.Machines[1] = MachineUnknown("b", "연결 안 됨") },
		"살아 있는 수": func(s *State) { s.Machines[0].Live = ip(2) },
		"병합기":     func(s *State) { s.Merge[0].Merger = "gone" },
		"배포 밀림":   func(s *State) { s.Deploy[0].Behind = ip(5) },
		"감시":      func(s *State) { s.Watch[1].Alive = bp(false) },
		"손잡이":     func(s *State) { s.Knobs = nil },
		"눈에 띄는 것": func(s *State) { s.Notable = []string{"새 사실"} },
	} {
		c := fake()
		f(&c)
		if Fingerprint(a) == Fingerprint(c) {
			t.Errorf("%s가 바뀌었는데 지문이 같음", name)
		}
	}
}

// ④ 눈에 띄는 것 — 본보기 셋(같은 단계에 오래 · 코어당 부하 치우침 · 배포 밀림)과 모름. 조치는 하지 않고 사실만
func TestNotable(t *testing.T) {
	n := Notable(fake(), th)
	all := strings.Join(n, "\n")
	for _, w := range []string{
		"lane-stuck", "95분", // 같은 단계에 오래
		"3.10", "0.10", // 치우침: 바쁜 a, 한가한 b
		"4커밋", // 배포 밀림
		"u",   // 모름 기계
	} {
		if !strings.Contains(all, w) {
			t.Errorf("눈에 띄는 것에 %q 없음:\n%s", w, all)
		}
	}
	if strings.Contains(all, "lane-ok") {
		t.Errorf("5분 머문 레인은 띄지 않는다:\n%s", all)
	}
	// 모두 한가·밀림 없음·짧게 머묾이면 비어 있다
	s := fake()
	s.Machines = s.Machines[1:2]
	s.Lanes = s.Lanes[1:]
	s.Deploy = []Deploy{{Machine: "b", Version: "abc1234", Behind: ip(0)}}
	s.Watch = []Watch{{Owner: "t1", Alive: bp(true)}}
	if got := Notable(s, th); len(got) != 0 {
		t.Errorf("조용한 상태에 눈에 띄는 것: %q", got)
	}
	// 문턱 0이면 그 본보기는 끈다(손잡이로 끌 수 있게)
	if got := strings.Join(Notable(fake(), Thresholds{}), "\n"); strings.Contains(got, "lane-stuck") || strings.Contains(got, "3.10") {
		t.Errorf("문턱 0인데 띄움: %s", got)
	}
}

// ⑤ json 칸 이름이 문서(docs/ops/state.md 「json 칸」 표)와 맞다 — 빠진 것도, 남는 것도 없이
func TestJSONKeysMatchDoc(t *testing.T) {
	s := fake()
	s.Notable = Notable(s, th)
	var j bytes.Buffer
	if err := JSON(&j, s); err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(j.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	var walk func(p string, v any)
	walk = func(p string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				q := k
				if p != "" {
					q = p + "." + k
				}
				if p == "queue.counts" { // 상태 이름은 값이라 칸 이름이 아니다
					continue
				}
				got[q] = true
				walk(q, c)
			}
		case []any:
			for _, c := range x {
				walk(p+"[]", c)
			}
		}
	}
	walk("", v)
	doc, err := os.ReadFile("../../docs/ops/state.md")
	if err != nil {
		t.Fatal(err)
	}
	sec := string(doc)
	i := strings.Index(sec, "## json 칸")
	if i < 0 {
		t.Fatal("문서에 「## json 칸」 절이 없음")
	}
	sec = sec[i:]
	if k := strings.Index(sec[3:], "\n## "); k >= 0 {
		sec = sec[:k+3]
	}
	want := map[string]bool{}
	re := regexp.MustCompile("(?m)^\\| `([a-z0-9_.\\[\\]]+)` \\|")
	for _, m := range re.FindAllStringSubmatch(sec, -1) {
		want[m[1]] = true
	}
	var miss, extra []string
	for k := range got {
		if !want[k] {
			miss = append(miss, k)
		}
	}
	for k := range want {
		if !got[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(miss)
	sort.Strings(extra)
	if len(miss)+len(extra) > 0 {
		t.Errorf("문서에 없는 json 칸: %v\njson에 없는 문서 칸: %v", miss, extra)
	}
}
