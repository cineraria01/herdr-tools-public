// Package opsstate: 상태 한 장(2026-11-24 ops-state, docs/ops/state.md) — 기계·대기열·레인·병합 줄·배포·감시·손잡이를 한 꼴에 담고
// 사람용 표·json·상태 지문·눈에 띄는 것을 만든다. 모으기(파일·원격 호출)는 hc-fill state가 하고, 여기는 순수 함수만.
//
// 모름은 nil(json null)과 까닭(why)이고 0·정상으로 적지 않는다. 판단은 하지 않는다 — 눈에 띄는 것은 지금 손댈 것(advise.go)의 요약이다.
package opsstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Schema: 꼴 판. 칸 뜻이 바뀌면 올린다
const Schema = 1

type State struct {
	Schema      int         `json:"schema"`
	At          time.Time   `json:"at"`
	Fingerprint string      `json:"fingerprint"`
	Sources     []Source    `json:"sources"`
	Machines    []Machine   `json:"machines"`
	Queue       Queue       `json:"queue"`
	Lanes       []Lane      `json:"lanes"`
	Merge       []MergeLine `json:"merge"`
	Deploy      []Deploy    `json:"deploy"`
	Watch       []Watch     `json:"watch"`
	Knobs       []Knob      `json:"knobs"`
	Journal     []string    `json:"journal"`
	Events      []string    `json:"events"`
	Notable     []string    `json:"notable"`
	Hidden      *Hidden     `json:"hidden"`
	goneWatch   []Watch     // Trim이 숨긴 안 도는 감시 — Advise가 본다(숨기기 전 상태로 판정)
}

// Source: 재료 하나 — 어디서 언제 읽었나, 이번 호출에서 든 원격 호출 수·초
type Source struct {
	Name  string  `json:"name"`
	AgeS  *int    `json:"age_s"`
	Calls int     `json:"calls"`
	Secs  float64 `json:"secs"`
	Why   string  `json:"why"`
}

type Machine struct {
	Name        string   `json:"name"`
	Reach       string   `json:"reach"` // ok | unknown
	Why         string   `json:"why"`
	Load1       *float64 `json:"load1"`
	Cores       *int     `json:"cores"`
	LoadPerCore *float64 `json:"load_per_core"`
	Live        *int     `json:"live"`
	Seats       *int     `json:"seats"`
	Lanes       *int     `json:"lanes"`
	Verifies    *int     `json:"verifies"` // 도는 저장소 검증 수(레인·병합기 합침, 부하와 같은 때 읽음) — 모름 nil
	LaneCap     *int     `json:"lane_cap"`
	Free        *int     `json:"free"` // 마지막 바퀴 띄우기가 판정한 빈 자리(hc-fill machRoom) — 모름 nil. advise는 따로 세지 않고 이것만 읽는다(ht-advise-slot-cap)
	Health      Health   `json:"health"`
	Steer       string   `json:"steer"`
	SkipWhy     string   `json:"skip_why"`

	// MergeVerifies: Verifies 가운데 병합기 검증(추측 일꾼·착지 실행) 수 — 자리·레인상한에는 안 든다(ht-fill-gate-seat). 모름 nil
	MergeVerifies *int `json:"merge_verifies"`
}

// Health: 빈 글 = 모름(json null)
type Health struct {
	Sleep      string
	Keychain   string
	DiskFreeGB *float64
	Tailscale  string
}

func nstr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h Health) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Sleep      *string  `json:"sleep"`
		Keychain   *string  `json:"keychain"`
		DiskFreeGB *float64 `json:"disk_free_gb"`
		Tailscale  *string  `json:"tailscale"`
	}{nstr(h.Sleep), nstr(h.Keychain), h.DiskFreeGB, nstr(h.Tailscale)})
}

func (h *Health) UnmarshalJSON(b []byte) error {
	var x struct {
		Sleep      string   `json:"sleep"`
		Keychain   string   `json:"keychain"`
		DiskFreeGB *float64 `json:"disk_free_gb"`
		Tailscale  string   `json:"tailscale"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*h = Health{x.Sleep, x.Keychain, x.DiskFreeGB, x.Tailscale}
	return nil
}

// known: 건강 넷 가운데 하나라도 값이 있나
func (h Health) known() bool {
	return h.Sleep != "" || h.Keychain != "" || h.DiskFreeGB != nil || h.Tailscale != ""
}

// MachineUnknown: 닿지 못한 기계 — 잰 칸은 모두 모름
func MachineUnknown(name, why string) Machine { return Machine{Name: name, Reach: "unknown", Why: why} }

type Queue struct {
	Counts map[string]int `json:"counts"`
	Oldest *Waiting       `json:"oldest"`
}

type Waiting struct {
	Lane string `json:"lane"`
	AgeS int    `json:"age_s"`
	Why  string `json:"why"`
}

type Lane struct {
	Name      string `json:"name"`
	Machine   string `json:"machine"`
	Stage     string `json:"stage"`
	StageAgeS *int   `json:"stage_age_s"`
	LastEvent string `json:"last_event"`
	Done      bool   `json:"done"`         // 더 할 일 없음: 마지막 사건이 정리됨·착지이고 터미널이 살아 있지 않다(모으는 쪽이 정한다 — 단계 글과 무관)
	Open      bool   `json:"open"`         // 터미널이 살아 있다(또는 살아 있는지 모른다) — 단계 글이 착지·끝남이어도 끝난 레인이 아니다
	PR        string `json:"pr,omitempty"` // PR 거울이 연 PR 「PR #n <주소>」(없으면 칸 없음 — docs/hc-fill/pr.md)
	// Queued: 병합 줄에서 정상으로 기다림 — 병합기가 살아 있고 그 제출이 줄에 있거나 도는 묶음의 구성원(모으는 쪽이 정한다).
	// 「같은 단계 오래」를 올리지 않는다(줄이 안 움직이는 것은 병합 줄 항목이 본다)
	Queued bool `json:"queued"`
}

type MergeLine struct {
	Project       string   `json:"project"`
	Length        *int     `json:"length"`
	Merger        string   `json:"merger"` // alive | gone | unknown
	LastRoundAgeS *int     `json:"last_round_age_s"`
	RecentVerifyS *float64 `json:"recent_verify_s"`
	SkipRatio     *float64 `json:"skip_ratio"`
	Lands         *int     `json:"lands"`
	Skipped       *int     `json:"skipped"`
	Delegated     *int     `json:"delegated"`
	// 줄에 선 가장 오래된 제출(병합기 살아 있음 기록 — 큐 참조 이름의 초)의 나이와 레인. 모르면 nil·빈 값
	OldestAgeS *int   `json:"oldest_age_s"`
	OldestLane string `json:"oldest_lane"`
	// 병합기 기록(internal/mergerstate — 병합기가 쓴 상태 한 장, docs/hc-lane/merger-state.md)에서: 착지 단계(병합기 기계·워커)면 맨 앞
	// 레인과 그 단계 나이. 착지 단계가 아니거나 기록 모름이면 빈 값·nil
	Landing     string `json:"landing"`
	LandingAgeS *int   `json:"landing_age_s"`
	// 단계(idle·round·spec-wait·land·land-on·prev-land)·까닭 한 줄·단계 나이·기록 나이. 기록 모름이면 빈 값·nil(까닭은 Why).
	// LastRoundAgeS(json 이름은 그대로 — 글은 「마지막 진행 N분 전」)는 기록의 마지막 진행 시각부터(바퀴가 도는 시각도 일지 줄 시각도 아님)
	Phase     string `json:"phase"`
	PhaseWhy  string `json:"phase_why"`
	PhaseAgeS *int   `json:"phase_age_s"`
	RecAgeS   *int   `json:"rec_age_s"`
	// 기다리는 대상(기록의 주소 — 살아 있는지는 이번 hc ls로), 착지 안쪽 단계 한 줄과 그 나이(착지 스크립트가 안 쓰면 빈 값)
	Waits     []MergeWait `json:"waits"`
	Inner     string      `json:"inner"`
	InnerAgeS *int        `json:"inner_age_s"`
	// 워커 착지 단계면 그 워커(기다리는 대상의 기계), 아니면 빈 값
	LandOn string `json:"land_on"`
	Why    string `json:"why"`
}

// MergeWait: 병합기가 기다리는 대상 — 기계·주소(병합기가 못 찾았으면 빈 값)·역할(병합기가 쓴 글)·띄운 나이, 살아 있나(live·gone·?
// — 이번 hc ls에서 그 주소, 못 읽으면 ?)
type MergeWait struct {
	Machine string `json:"machine"`
	Addr    string `json:"addr"`
	Role    string `json:"role"`
	AgeS    *int   `json:"age_s"`
	Live    string `json:"live"`
}

// Deploy: Version 빈 글 = 모름(json null)
type Deploy struct {
	Machine string `json:"machine"`
	Version string `json:"version"`
	Behind  *int   `json:"behind"`
	Why     string `json:"why"`
}

func (d Deploy) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Machine string  `json:"machine"`
		Version *string `json:"version"`
		Behind  *int    `json:"behind"`
		Why     string  `json:"why"`
	}{d.Machine, nstr(d.Version), d.Behind, d.Why})
}

func (d *Deploy) UnmarshalJSON(b []byte) error {
	var x struct {
		Machine string `json:"machine"`
		Version string `json:"version"`
		Behind  *int   `json:"behind"`
		Why     string `json:"why"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*d = Deploy{x.Machine, x.Version, x.Behind, x.Why}
	return nil
}

type Watch struct {
	Owner string `json:"owner"`
	Alive *bool  `json:"alive"`
	// Need: 감시가 있어야 한다 — 지휘 터미널이 hc ls에 있고 착지·판정 전 열린 줄이 있다(손댈 것 「감시 안 돎」만 쓴다, 출력 칸 아님).
	// 지휘가 사라진 흔적은 거짓: 그 지휘의 사건은 다른 살아 있는 지휘의 감시가 받는다(hc-lane watch wfilter)
	Need bool `json:"-"`
}

type Knob struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	By    string `json:"by"`
	Why   string `json:"why"`
	Until string `json:"until"`
}

// Thresholds: 손댈 것의 문턱(손잡이 state_stage·state_hot·state_cool·idle·advise_wait). 0이면 그 본보기는 끈다
type Thresholds struct {
	// StageMin: 같은 단계에 이만큼(분) 머문 레인. 터미널이 살아 있는(또는 모르는, Lane.Open) 줄만 — 끝난 줄은 자리를 잡지 않고 hc read할
	// 터미널도 없다(2026-11-24 지휘: 스스로 착지·정리하고 완료로 보고한 레인·착지 없는 검토 레인이 거짓 양성이었다. 완료 뒤 멈춤은 done_stall 사건이 따로 본다)
	StageMin       int
	SkewHi, SkewLo float64 // 코어당 부하가 SkewHi 이상인 기계와 SkewLo 이하인 기계가 함께 있으면
	IdleMin        int     // 빈 자리가 있는데 대기 줄이 이만큼(분) 넘게 기다리면(손잡이 idle)
	WaitMin        int     // 감사 자리 없음·병합 줄 안 움직임이 이만큼(분) 넘으면(손잡이 advise_wait)
	SlowMin        int     // 병합기의 지금 착지가 이만큼(분) 넘으면 병합 줄이 느림(손잡이 advise_merge_slow)
	PinMin         int     // --only로 묶인 기계가 모두 상한이라 이만큼(분) 못 뜬 대기 줄(손잡이 advise_pinned, 0이면 끔)
}

// minutesRe: 단계 글의 경과 분(「감사 대기(자리 없음 N분)」) — 차수(「N번째」)·sha는 판단 칸이라 남긴다(감사 atozipa 중간 1)
var minutesRe = regexp.MustCompile(`[0-9]+분`)

// durRe: 나이 글(dur — 「45초」「12분」「2시간05분」). 눈에 띄는 것은 손댈 것의 근거를 담아, 숫자만 빼면 나이가 1분·1시간을 넘을 때
// 단위 글이 바뀌어 지문이 바뀐다(감사 atozr8z 낮음 1) — 나이 글을 통째로 뺀다
var durRe = regexp.MustCompile(`[0-9]+시간[0-9]+분|[0-9]+분|[0-9]+초`)

// noDigits: 지문에 넣을 글에서 숫자를 뺀다(나이·부하 숫자가 바뀌어도 같은 사실이면 같게)
func noDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, s)
}

func ival(p *int) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprint(*p)
}

func bval(p *bool) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprint(*p)
}

// Fingerprint: 판단 칸만으로 만든 12자. 시각·나이·잰 값(부하·검증 초·건너뜀 비율)은 넣지 않고, 사실 글(눈에 띄는 것·안 띄운 까닭)은 숫자를 빼고 넣는다
func Fingerprint(s State) string {
	h := sha256.New()
	w := func(a ...any) { fmt.Fprintln(h, a...) }
	for _, m := range s.Machines {
		w("m", m.Name, m.Reach, m.Why, ival(m.Live), ival(m.Seats), ival(m.Lanes), ival(m.LaneCap), ival(m.Free),
			m.Health.Sleep, m.Health.Keychain, m.Health.Tailscale, m.Steer, noDigits(m.SkipWhy))
	}
	var ks []string
	for k := range s.Queue.Counts {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		w("q", k, s.Queue.Counts[k])
	}
	if o := s.Queue.Oldest; o != nil {
		w("qo", o.Lane, o.Why)
	}
	for _, l := range s.Lanes {
		if Finished(l) { // 끝난 줄이 창 밖으로 나가도(기본 출력에서 숨겨져도) 지문은 그대로 — 착지는 사건·대기열 수에 남는다
			continue
		}
		w("l", l.Name, l.Machine, minutesRe.ReplaceAllString(l.Stage, "분"), l.LastEvent)
	}
	for _, m := range s.Merge {
		w("g", m.Project, ival(m.Length), m.Merger)
	}
	for _, d := range s.Deploy {
		w("d", d.Machine, d.Version, ival(d.Behind), d.Why)
	}
	for _, x := range s.Watch {
		w("w", x.Owner, bval(x.Alive))
	}
	for _, k := range s.Knobs {
		w("k", k.Name, k.Value, k.By, k.Until)
	}
	for _, j := range s.Journal {
		w("j", j)
	}
	for _, e := range s.Events {
		w("e", e)
	}
	for _, n := range s.Notable {
		w("n", noDigits(durRe.ReplaceAllString(n, "")))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// JSON: 기계가 읽는 꼴(압축 json 한 줄). 칸 이름은 docs/ops/state.md 「json 칸」
func JSON(w io.Writer, s State) error {
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	return e.Encode(s)
}

func fval(p *float64, f string) string {
	if p == nil {
		return "모름"
	}
	return fmt.Sprintf(f, *p)
}

func age(p *int) string {
	if p == nil {
		return "모름"
	}
	return dur(*p)
}

// dur: 초 → 「45초」「12분」「2시간05분」
func dur(s int) string {
	switch {
	case s < 0:
		return "0초"
	case s < 60:
		return fmt.Sprintf("%d초", s)
	case s < 3600:
		return fmt.Sprintf("%d분", s/60)
	}
	return fmt.Sprintf("%d시간%02d분", s/3600, s%3600/60)
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// Human: 사람용 표. json과 같은 사실을 담는다
func Human(w io.Writer, s State) error {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	p("상태 %s · 지문 %s", s.At.Local().Format("2006-01-02 15:04:05"), or(s.Fingerprint, "-"))
	var src []string
	for _, x := range s.Sources {
		t := x.Name + " 나이 모름"
		if x.AgeS != nil {
			t = fmt.Sprintf("%s %s 전", x.Name, dur(*x.AgeS))
		}
		if x.Calls > 0 || x.Secs > 0 {
			t += fmt.Sprintf("(호출 %d번 %.1f초)", x.Calls, x.Secs)
		}
		if x.Why != "" {
			t += " 못 읽음: " + x.Why
		}
		src = append(src, t)
	}
	p("재료: %s", or(strings.Join(src, " · "), "-"))

	p("\n기계")
	// 추린 출력(Hidden 있음)은 건강 값이 있는 기계 줄에만 건강을 적고, 모두 모름이면 절 끝에 한 줄
	trim, anyH := s.Hidden != nil, false
	for _, m := range s.Machines {
		anyH = anyH || m.Reach == "ok" && m.Health.known()
	}
	for _, m := range s.Machines {
		if m.Reach != "ok" {
			p("  %s  모름(%s)%s", m.Name, or(m.Why, "까닭 모름"), steer(m))
			continue
		}
		ld := "부하 모름"
		if m.LoadPerCore != nil {
			ld = fmt.Sprintf("부하 %s/%s(코어당 %.2f)", fval(m.Load1, "%.1f"), ival(m.Cores), *m.LoadPerCore)
		}
		hl, ht := m.Health, ""
		if !trim || hl.known() {
			ht = fmt.Sprintf("  건강 잠자기 %s·키체인 %s·디스크 %s·tailscale %s",
				or(hl.Sleep, "모름"), or(hl.Keychain, "모름"), fval(hl.DiskFreeGB, "%.0fGB"), or(hl.Tailscale, "모름"))
		}
		p("  %s  %s  에이전트 %s/%s  레인 %s/%s%s  빈 자리 %s%s%s",
			m.Name, ld, ival(m.Live), ival(m.Seats), ival(m.Lanes), ival(m.LaneCap), VerNote(m.Verifies, m.MergeVerifies), ival(m.Free), ht, steer(m))
	}
	if trim && !anyH && len(s.Machines) > 0 {
		p("  건강 값 없음 — 점검이 아직 안 남김")
	}

	var ks []string
	for k := range s.Queue.Counts {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var qc []string
	for _, k := range ks {
		qc = append(qc, fmt.Sprintf("%s %d", k, s.Queue.Counts[k]))
	}
	old := "없음"
	if o := s.Queue.Oldest; o != nil {
		old = fmt.Sprintf("%s %s(%s)", o.Lane, dur(o.AgeS), o.Why)
	}
	p("\n대기열 %s · 가장 오래 기다린 줄 %s", or(strings.Join(qc, " · "), "없음"), old)

	sec := func(title string, n int) {
		p("\n%s", title)
		if n == 0 {
			p("  없음")
		}
	}
	hid := s.Hidden
	if hid == nil {
		hid = &Hidden{}
	}
	sec("레인", len(s.Lanes)+hid.Lanes)
	for _, l := range s.Lanes {
		pr := ""
		if l.PR != "" {
			pr = "  " + l.PR
		}
		p("  %s  %s  %s  %s째  마지막 %s%s", l.Name, or(l.Machine, "-"), l.Stage, age(l.StageAgeS), or(l.LastEvent, "없음"), pr)
	}
	if hid.Lanes > 0 {
		p("  끝난 레인 %d줄 숨김(--all)", hid.Lanes)
	}
	sec("병합 줄", len(s.Merge))
	for _, m := range s.Merge {
		last := "마지막 진행 모름"
		if m.LastRoundAgeS != nil {
			last = "마지막 진행 " + age(m.LastRoundAgeS) + " 전"
		}
		t := fmt.Sprintf("  %s  줄 %s · 병합기 %s · %s · 최근 검증 %s · 건너뜀 %s",
			m.Project, ival(m.Length), m.Merger, last, fval(m.RecentVerifyS, "%.0f초"), fval(pct(m.SkipRatio), "%.0f%%"))
		if m.Phase != "" {
			t += " · " + or(m.PhaseWhy, m.Phase) + " " + age(m.PhaseAgeS) + "째"
		}
		if m.Inner != "" {
			t += " · 안쪽 단계 " + m.Inner + " " + age(m.InnerAgeS) + "째"
		}
		for _, w := range m.Waits {
			t += " · 기다림 " + w.Role + " " + or(w.Addr, w.Machine) + "(" + w.Live + ")"
		}
		if m.Lands != nil {
			t += fmt.Sprintf("(최근 착지 %d번 중 %s번) · 검증 맡김 %s번", *m.Lands, ival(m.Skipped), ival(m.Delegated))
		}
		if m.Why != "" {
			t += " (" + m.Why + ")"
		}
		p("%s", t)
	}
	sec("배포", len(s.Deploy)+hid.Deploy)
	for _, d := range s.Deploy {
		if d.Version == "" {
			p("  %s  모름(%s)", d.Machine, or(d.Why, "까닭 모름"))
			continue
		}
		t := fmt.Sprintf("  %s  %s · 밀림 %s커밋", d.Machine, d.Version, ival(d.Behind))
		if d.Why != "" {
			t += " (" + d.Why + ")"
		}
		p("%s", t)
	}
	if hid.Deploy > 0 {
		p("  대상 아님 %d대(--all)", hid.Deploy)
	}
	sec("감시", len(s.Watch)+hid.Watch)
	for _, x := range s.Watch {
		st := "모름"
		if x.Alive != nil {
			st = map[bool]string{true: "돎", false: "안 돎"}[*x.Alive]
		}
		p("  %s  %s", x.Owner, st)
	}
	if hid.Watch > 0 {
		p("  안 도는 지휘 흔적 %d개(--all)", hid.Watch)
	}
	sec("손잡이", len(s.Knobs))
	for _, k := range s.Knobs {
		p("  %s=%s (%s · %s · %s)", k.Name, k.Value, or(k.By, "-"), or(k.Why, "-"), or(k.Until, "계속"))
	}
	sec("최근 조치", len(s.Journal))
	for _, j := range s.Journal {
		p("  %s", j)
	}
	sec("최근 사건", len(s.Events))
	for _, e := range s.Events {
		p("  %s", e)
	}
	sec("눈에 띄는 것", len(s.Notable))
	for _, n := range s.Notable {
		p("  - %s", n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func steer(m Machine) string {
	t := ""
	if m.Steer != "" {
		t += "  조정 " + m.Steer
	}
	if m.SkipWhy != "" {
		t += "  안 띄움 " + m.SkipWhy
	}
	return t
}

func pct(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p * 100
	return &v
}
