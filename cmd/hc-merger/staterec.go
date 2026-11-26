package main

// 병합기 상태 한 장 쓰기(ht-merger-state-record, 2026-11-26 사용자 「구조를 다시 짜기」 — docs/hc-lane/merger-state.md). hc-fill은 이
// 기록만 읽는다(hc-merger --state). 바퀴 끝에 한 번(stEnd), 단계가 바뀔 때(stPhase), 착지 중에는 stBeatEvery마다(stBeat — 바퀴 일꾼이
// 착지를 기다리며 막혀 있어도). 쓰기 실패는 경고만 — 차례·검증·착지 동작은 바꾸지 않는다(적는 것만 더한다).
//
// 「진행」(progress_at을 지금으로): 단계·맨 앞이 바뀜, 바퀴가 일을 함(종료 76), 기다리는 일꾼이 진행 중(probeAt — spec_fresh분 안 갱신,
// 나눔이면 몫 하나라도), 병합기 기계 착지 자식이 살아 있음, 착지 안쪽 단계 줄이 바뀜. 같은 단계로 바퀴만 돈 것·기록만 다시 쓴 것은 아니다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/mergerstate"
)

var (
	stBeatEvery          = 30 * time.Second // 착지 중 기록 다시 쓰기 간격(시험이 줄인다)
	stProbeEvery   int64 = 60               // 기다리는 일꾼의 진행을 재는 간격(초) — 바퀴(약 10초)마다 원격을 재지 않게
	landPhaseLocal       = "land-phase"     // 병합기 기계 착지의 안쪽 단계 파일(<기록 폴더>/…) — HERDR_LAND_PHASE
)

// recM: M에 묻는 것 — 이 바퀴가 기다리는 맨 앞 추측(specDecide가 채움 — 기다리는 대상), 기록 상태
type recM struct {
	wrow specRow
	wpre []specRow // 범인 가르기로 기다리는 앞 무리 추측들(prefixWait)
	rs   recState
}

// recState: 기록 상태
type recState struct {
	mu     sync.Mutex
	prev   mergerstate.Rec // 마지막으로 쓴 것(앞 바퀴·앞 병합기 — stLoad)
	cur    mergerstate.Rec
	set    bool     // 이 바퀴에 단계를 정함
	queue  []string // 이 바퀴의 줄 차례(pickBatch — 우선 반영), nil이면 앞 기록 것
	beat   int      // 도는 stBeat의 번호 — 멈추면 늘려 멈춘 뒤의 쓰기를 버린다
	warned bool
	loaded bool // 앞 기록을 읽음(stLoad)
}

// stLoad: 앞 기록(앞 바퀴·다시 띄우기 전 병합기)을 처음 한 번 읽어 둔다 — 같은 단계·같은 맨 앞이면 이어 쓴다(잠금을 쥔 채로 부른다)
func (m *M) stLoad() {
	if m.rs.loaded {
		return
	}
	m.rs.loaded = true
	if r, err := mergerstate.Read(m.mj); err == nil {
		m.rs.prev = r
	}
}

// stQueue: 이 바퀴의 줄 차례(queue()가 돌려준 차례 — 레인 이름)
func (m *M) stQueue(q [][2]string) {
	m.rs.mu.Lock()
	defer m.rs.mu.Unlock()
	m.rs.queue = []string{}
	for _, e := range q {
		n := strings.TrimPrefix(e[0], "refs/merge-queue/")
		if len(n) > 11 && n[10] == '-' {
			n = n[11:]
		}
		m.rs.queue = append(m.rs.queue, n)
	}
}

// stLandOn: 착지 실행 결정(「<기계>(<까닭>)」) — 이력으로 다음 기록들에 이어 싣는다
func (m *M) stLandOn(s string) {
	m.rs.mu.Lock()
	m.stLoad()
	m.rs.prev.LandOn = s
	m.rs.mu.Unlock()
}

// stPhase: 단계를 정하고 쓴다. 같은 단계·같은 맨 앞이면 단계 시작·마지막 진행·안쪽 단계·기다리는 대상의 주소와 띄운 때를 앞 기록에서
// 잇는다(다시 띄운 병합기도). adv = 이번에 나아감, probed = 이번에 일꾼을 잰 안쪽 단계 줄(nil이면 이번에 안 잼 — 앞 것을 잇는다)
func (m *M) stPhase(phase, why string, waits []mergerstate.Wait, adv bool, probed *string) {
	m.rs.mu.Lock()
	defer m.rs.mu.Unlock()
	m.stLoad()
	now := time.Now().Unix()
	p := m.rs.prev
	r := mergerstate.Rec{Phase: phase, Why: why, Queue: m.rs.queue, Waits: waits, LandOn: p.LandOn, Since: now, Progress: now, Probe: p.Probe}
	if r.Queue == nil {
		r.Queue = p.Queue
	}
	for _, e := range m.b {
		r.Members = append(r.Members, e.l)
	}
	if len(m.b) > 0 {
		r.Front = m.b[0].s
	}
	if p.Phase == phase && p.Front == r.Front {
		r.Since, r.Inner = p.Since, p.Inner
		if !adv && p.Progress > 0 {
			r.Progress = p.Progress
		}
		for i := range r.Waits {
			for _, w := range p.Waits {
				if w.Machine == r.Waits[i].Machine && w.Role == r.Waits[i].Role {
					r.Waits[i].Addr = cmpOr(r.Waits[i].Addr, w.Addr)
					if r.Waits[i].At == 0 {
						r.Waits[i].At = w.At
					}
				}
			}
		}
	}
	if probed != nil {
		r.Probe = now
		if stInner(&r, *probed) && r.Progress != now {
			r.Progress = now // 안쪽 단계 줄이 바뀜 = 진행
		}
	}
	m.rs.cur, m.rs.set = r, true
	m.stWrite()
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// stWrite: 지금 기록을 쓴다(잠금을 쥔 채로 부른다)
func (m *M) stWrite() {
	r := m.rs.cur
	r.At, r.Machine, r.Term, r.Base = time.Now().Unix(), m.mme, os.Getenv("HC_TERMINAL_ID"), m.base
	if err := mergerstate.Write(m.mj, r); err != nil {
		if !m.rs.warned {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 상태 기록을 쓰지 못했다(hc-fill이 모름으로 본다 — 병합 동작은 그대로): %v\n", err)
			m.rs.warned = true
		}
		return
	}
	m.rs.prev = r
}

// stEnd: 바퀴 끝(roundMain) — 이 바퀴가 기다림 단계를 정했으면 그대로 두고, 아니면(착지가 끝났거나 일찍 끝난 바퀴) 줄·결과로
func (m *M) stEnd(rc int) {
	m.rs.mu.Lock()
	set, ph, q := m.rs.set, m.rs.cur.Phase, m.rs.queue
	m.rs.mu.Unlock()
	landed := ph == mergerstate.Land || ph == mergerstate.LandOn
	if set && !landed {
		return
	}
	adv := rc == 76 || landed // 착지가 끝남 = 나아감
	switch {
	case q != nil && len(q) == 0:
		m.stPhase(mergerstate.Idle, "줄 빔", nil, adv, nil)
	case m.deferred != "":
		m.stPhase(mergerstate.Round, "쉼 — "+m.deferred, nil, adv, nil)
	case rc == 76:
		m.stPhase(mergerstate.Round, "일을 함 — 바로 다음 바퀴", nil, adv, nil)
	case m.mer:
		m.stPhase(mergerstate.Round, "쉼 — 원격을 못 읽었거나 꺼짐·남은 일(다음 바퀴에)", nil, adv, nil)
	default:
		m.stPhase(mergerstate.Round, "바퀴 — 할 일 없음", nil, adv, nil)
	}
}

// stSpecWait: 맨 앞 추측 검증을 기다리는 바퀴(round — specWaitNote 다음). 기다리는 일꾼(추측·몫 i/K·마무리)과, stProbeEvery마다 그
// 일꾼들을 재어 주소·진행·부하 대기를 적는다. 일꾼이 부하가 기준 아래로 내려가길 기다리면 단계 load-wait(정상 대기 — 까닭에 기계·부하·기준)
func (m *M) stSpecWait() {
	r := m.wrow
	labs := make([]string, len(m.b))
	for i, e := range m.b {
		labs[i] = e.l
	}
	lanes := strings.Join(labs, "+")
	why := "추측 검증 기다림 — " + lanes
	type at struct{ mach, term, key string }
	var ws []mergerstate.Wait
	var ps []at
	s, sh := m.shardRead()
	switch {
	case len(m.wpre) > 0: // 묶음 실패 뒤 범인 가르기 — 앞 무리 추측들
		why = "범인 가르기 — 앞 무리 추측 기다림 — " + lanes
		for _, p := range m.wpre {
			if len(p.id) >= 8 {
				ws = append(ws, mergerstate.Wait{Machine: p.machine, Role: "앞 무리 추측", At: p.at})
				ps = append(ps, at{p.machine, "spec-" + p.id[:8], p.id[:8]})
			}
		}
	case r.machine == "" || r.machine == "-" || len(r.id) < 8:
		why += " · 맡길 기계를 기다림"
	case sh && s.id == r.id && s.fin == "":
		why += fmt.Sprintf(" · 나눔 %d", s.k)
		for _, p := range s.parts {
			ws = append(ws, mergerstate.Wait{Machine: p[1], Role: "몫 " + p[0] + "/" + strconv.Itoa(s.k), At: s.t0})
			ps = append(ps, at{p[1], "spec-" + r.id[:8] + "-" + p[0], r.id[:8] + "-s" + p[0]})
		}
	default:
		role := "추측"
		if sh && s.id == r.id {
			role = "마무리"
		}
		ws = append(ws, mergerstate.Wait{Machine: r.machine, Role: role, At: r.at})
		ps = append(ps, at{r.machine, "spec-" + r.id[:8], r.id[:8]})
	}
	front := ""
	if len(m.b) > 0 {
		front = m.b[0].s
	}
	m.rs.mu.Lock()
	m.stLoad()
	p := m.rs.prev
	m.rs.mu.Unlock()
	cont := (p.Phase == mergerstate.SpecWait || p.Phase == mergerstate.LoadWait) && p.Front == front
	phase, adv := mergerstate.SpecWait, false
	var probed *string
	if len(ps) > 0 && m.specFreshMin() > 0 && (!cont || time.Now().Unix()-p.Probe >= stProbeEvery) {
		inner := ""
		for i, x := range ps {
			pr := m.probeAt("spec", x.mach, x.term, x.key)
			ws[i].Addr = pr.addr
			adv = adv || pr.fresh
			if pr.wait != "" {
				phase, why = mergerstate.LoadWait, "추측 검증 "+pr.wait+" — "+lanes
			}
			inner = cmpOr(inner, pr.inner) // 검증 스크립트가 알린 안쪽 단계(HERDR_LAND_PHASE — 첫 몫의 것)
		}
		probed = &inner
	} else if cont && p.Phase == mergerstate.LoadWait {
		phase, why = p.Phase, p.Why // 재지 않은 바퀴 — 앞에 잰 부하 대기를 잇는다
	}
	if cont && phase != p.Phase { // 추측 기다림 ↔ 부하 대기는 같은 기다림 — 단계 시작은 잇는다
		m.rs.mu.Lock()
		m.rs.prev.Phase = phase
		m.rs.mu.Unlock()
		adv = true
	}
	m.stPhase(phase, why, ws, adv, probed)
}

// stInner: 착지 안쪽 단계 줄(첫 줄)을 기록에 — 바뀌었으면 참(진행)
func stInner(r *mergerstate.Rec, line string) bool {
	line, _, _ = strings.Cut(strings.TrimSpace(line), "\n")
	switch {
	case line == "" && r.Inner == nil:
		return false
	case line == "":
		r.Inner = nil
		return true
	case r.Inner != nil && r.Inner.Line == line:
		return false
	}
	r.Inner = &mergerstate.Inner{Line: line, At: time.Now().Unix()}
	return true
}

// stBeat: 착지를 기다리는 동안 stBeatEvery마다 기록을 다시 쓴다 — step(기록 사본)이 참이면 진행. 돌려준 함수로 멈춘다(멈춘 뒤의 쓰기는
// 버린다 — 기다리지 않는다)
func (m *M) stBeat(step func(r *mergerstate.Rec) bool) func() {
	m.rs.mu.Lock()
	m.rs.beat++
	id := m.rs.beat
	m.rs.mu.Unlock()
	done := make(chan struct{})
	every := stBeatEvery // 띄울 때 한 번 — 멈춘 뒤에도 도는 고루틴이 전역을 읽지 않게(시험이 되돌릴 때 경합)
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			m.rs.mu.Lock()
			r := m.rs.cur
			r.Waits = append([]mergerstate.Wait(nil), r.Waits...)
			m.rs.mu.Unlock()
			adv := step(&r)
			m.rs.mu.Lock()
			if m.rs.beat == id {
				if adv {
					r.Progress = time.Now().Unix()
				}
				m.rs.cur = r
				m.stWrite()
			}
			m.rs.mu.Unlock()
		}
	}()
	return func() {
		m.rs.mu.Lock()
		m.rs.beat++
		m.rs.mu.Unlock()
		close(done)
	}
}

// stLandLocal: 병합기 기계에서 착지(runCandidate) — 단계 land, 착지 자식이 살아 있는 동안 진행, 안쪽 단계 파일을 읽는다. 돌려준 함수로 멈춘다
func (m *M) stLandLocal() (phaseFile string, stop func()) {
	pf := filepath.Join(m.mj, landPhaseLocal)
	os.Remove(pf)
	m.stPhase(mergerstate.Land, "착지 — "+m.mlab+"(병합기 기계)", nil, true, nil)
	return pf, m.stBeat(func(r *mergerstate.Rec) bool {
		b, _ := os.ReadFile(pf) // 없으면 안쪽 단계 모름
		stInner(r, string(b))
		return true // 이 맥박은 착지 자식을 기다리는 동안만 돈다 — 살아 있음 = 진행
	})
}

// stLandRemote: 워커에서 착지(runRemote) — 단계 land-on, stProbeEvery마다 그 워커의 착지 터미널·로그 갱신(probeAt land)과 안쪽 단계
// 파일(<클론>.land-wt/<차례 12>.phase)을 읽는다
func (m *M) stLandRemote(mach, cm string) func() {
	m.stPhase(mergerstate.LandOn, "워커 착지 실행 — "+m.mlab+"("+mach+")",
		[]mergerstate.Wait{{Machine: mach, Role: "착지 실행", At: time.Now().Unix()}}, true, nil)
	every := stProbeEvery // 맥박 고루틴은 전역을 읽지 않는다(stBeat)
	return m.stBeat(func(r *mergerstate.Rec) bool {
		now := time.Now().Unix()
		if now-r.Probe < every {
			return false
		}
		r.Probe = now
		pr := m.probeAt("land", mach, "land-"+cm[:8], cm[:12]) // 안쪽 단계 줄도 같은 한 번에(<클론>.land-wt/<차례 12>.phase)
		if pr.addr != "" && len(r.Waits) > 0 {
			r.Waits[0].Addr = pr.addr
		}
		return stInner(r, pr.inner) || pr.fresh
	})
}

// stateMain: hc-merger --state <클론> — 그 클론 병합기의 기록 한 장을 그대로 찍는다(종료 0). 기록 없음(옛 판 병합기) 3, 못 읽음 1.
// hc-fill이 이 기계·다른 기계 모두 이 한 명령으로 읽는다(hc exec --on)
func stateMain(a []string) int {
	if len(a) != 1 {
		fmt.Fprintln(os.Stderr, "hc-lane merger --state <클론>")
		return 1
	}
	gp, err := exec.Command("git", "-C", a[0], "rev-parse", "--git-path", "hc-merger").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger --state: 클론 %s 의 기록 폴더를 모른다: %v\n", a[0], err)
		return 1
	}
	d := strings.TrimSpace(string(gp))
	if !filepath.IsAbs(d) {
		d = filepath.Join(a[0], d)
	}
	b, err := os.ReadFile(filepath.Join(d, mergerstate.File))
	switch {
	case os.IsNotExist(err):
		return 3
	case err != nil:
		fmt.Fprintf(os.Stderr, "hc-lane merger --state: %v\n", err)
		return 1
	}
	if _, err := mergerstate.Parse(b); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger --state: %v\n", err)
		return 1
	}
	os.Stdout.Write(b)
	return 0
}
