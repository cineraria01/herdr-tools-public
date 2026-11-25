package main

// 맨 앞 후보도 검증 기계에(merger-verify-dispatch, 2026-11-24 사용자 「병합은 지휘에서 모와서 하는건데 기본적으로」 「워커들은 열심히
// 일해야지」): 병합기 기계는 모으기·차례·rebase·기록 확인·넣기만 하고, 맨 앞 무리의 검증도 뒤 무리와 같은 꼴(spec 참조·같은 일꾼·같은
// 통과 기록)로 빈 검증 기계에 맡긴다(specrun.go spec의 짓기 고리). 착지 판정은 그대로다 — rebase한 트리 = 통과 기록의 트리일 때만 건너뜀.
//
//   - 맡길 기계가 없거나(모두 바쁨)·못 띄웠거나·기다림 상한(HC_LANE_SPEC_FRONT_WAIT)을 넘기거나·결과가 모름이면 손잡이 frontLocal로:
//     1(기본) = 지금처럼 병합기 기계가 직접 검증(줄이 멈추지 않게) · 0 = 직접 검증하지 않고 기다리며 주인 지휘에 막힘 사건 한 번(맨 앞
//     제출마다). 0이면 못 띄운·모름인 맨 앞은 바퀴마다 빈 검증 기계에 다시 띄운다. 검증 기계 목록에 병합기 기계 말고 기계가 없으면
//     손잡이와 상관없이 직접 검증한다(기다려도 맡을 기계가 없다).
//   - 맨 앞 혼자가 검증 기계에서 시험 실패(test=1)면 한 기계의 흔들림으로 반송하지 않는다: 같은 추측을 다른 빈 검증 기계(없으면 같은
//     기계)에 한 번 더 띄우고 — spec 참조를 그 기계·지금 시각으로 고쳐 쓰고(본문에 첫 실패) 실패 기록·떴다는 표식을 같은 push로 지운다 —
//     그것도 시험 실패면 지금 반송 규칙대로 반송한다(까닭에 두 기계). 다시 띄울 빈 기계가 없으면 모름과 같다(손잡이).
//   - 묶음 맨 앞의 시험 실패는 묶음 착지 실패와 같은 길로 합친다(반송 없이 마지막을 빼고 다시 — cap). 그 뒤 「맨 앞 혼자 추측」·범인
//     반송(specSolo·specCulpritWhy)은 지금 그대로다.
//   - 환경 실패(시험 실패 근거 없음 — 준비 실패·시간 넘김·가를 수 없음)는 모름: 반송하지 않는다.
//   - 착지 스크립트 저장소(m.hb — 기본 브랜치에 .claude/herdr-land.sh, spec-script-land): 그 스크립트가 착지 중에 스스로 검증하니 맨 앞
//     추측의 실패·모름은 손잡이와 상관없이 착지 스크립트로 넘기고, 추측 실패를 반송·한 번 더 확인·줄이기·범인 반송 근거로 쓰지 않는다.
//   - 맨 앞이 기다림 상한을 넘기거나 실패·모름이면, 서서 기다리거나 병합기 기계에서 직접 검증하기(착지 스크립트 포함) 전에 한가한
//     다른 기계에 맨 앞 제출마다 한 번 더 맡긴다(workerRedo — 통과한 조각은 이어 쓴다, specparts.go). 맡길 기계가 없을 때만 손잡이대로.

//   - 맨 앞 추측이 기다림 상한을 넘겨도 진행 중이면(specFresh — spec-<아이디 8> 터미널 살아 있음 + 로그·작업 트리가 손잡이 spec_fresh분
//     안에 갱신, hc-fill advise_spec_fresh와 같은 verified.FreshSh) 직접 검증을 새로 시작하지 않고 merger_spec_timeout까지 더 기다린다
//     (ht-spec-overrun — 1471초째 도는 추측을 두고 맥에서 처음부터 전체 직접 검증을 시작해 23~34분 걸렸다). 멈췄으면 지금처럼.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/verified"
)

// jOverWait: 맨 앞 추측이 기다림 상한을 넘겼는데 진행 중이라 더 기다리기 시작함(맨 앞 제출마다 한 줄)
const jOverWait = "추측 상한 넘김"

// frontOver: 맨 앞 판정 d가 기다림인데 기다림 상한(merger_front_wait)을 넘겼나. 넘겼으면 over와 판정 — 진행 중(specFresh)이면 더
// 기다림, 아니면 직접 검증(stall = 상한에서 직접 검증한 까닭)이나 손잡이 front_local 0의 다시 맡김(overRedo)
func (m *M) frontOver(c *specCtx, st []specStep, d specDecision, busy map[string]bool, now int64, disp bool) (specDecision, bool, string) {
	w := specFrontWait()
	if d != specWaitFront || now-st[0].row.at <= w {
		return d, false, ""
	}
	// 기다릴 값어치는 남은 시간이 직접 검증보다 짧을 때뿐 — 검증 기계가 죽었거나 바쁘거나 느리면 그 뒤 큐 전체가 멈춘다.
	// 그 추측은 통과도 실패도 아니다(반송·범인 판정에 안 씀): 지금 「모름」 길 그대로, 뒤늦은 결과는 specClean이 거둔다.
	// 다만 살아서 진행 중이면 처음부터 직접 검증하지 않고 merger_spec_timeout까지 더 기다린다
	fmt.Printf("hc-lane merger: 맨 앞 검증이 %d초 넘게 도는 중(%s)\n", w, st[0].row.machine)
	fresh, stall := m.specFresh(st[0].row)
	switch {
	case fresh:
		m.overWait(st[0], now)
		return specWaitFront, false, ""
	case disp && m.overRedo(c, st[0], busy, now): // 기다리거나 직접 검증하기 전에 한가한 다른 기계에 한 번(통과 조각 이어 씀)
		return specWaitFront, false, ""
	}
	return specLand, true, stall
}

// specFreshMin: 손잡이 spec_fresh(분, 기본 10, 0이면 끔 — 상한에서 지금처럼 직접 검증)
func (m *M) specFreshMin() int { return m.knobInt("spec_fresh", "HC_LANE_SPEC_FRESH", 10, 0) }

// specFresh: 맨 앞 추측 r이 그 기계에서 진행 중인가 — spec-<아이디 8> 터미널이 살아 있고(hc get) 그 로그·작업 트리가 spec_fresh분 안에
// 갱신됐다(hc exec로 verified.FreshSh). 아니면 까닭(갱신 없음·터미널 없음·못 읽음), 손잡이 0이면 빈 까닭
// ponytail: 상한을 넘긴 바퀴마다 hc get·exec 한 번씩 — 바퀴가 잦아 무거우면 표식 파일에 마지막 확인 시각을 두고 1분에 한 번으로
func (m *M) specFresh(r *specRow) (bool, string) {
	min := m.specFreshMin()
	if min == 0 {
		return false, ""
	}
	if !addrOK(r.machine) || len(r.id) < 8 {
		return false, "터미널 없음"
	}
	id8 := r.id[:8]
	o, rc, _ := hcRun(hcLook, "get", r.machine+"/spec-"+id8, "--json")
	t, found, err := hcjson.ParseGet([]byte(o), rc, "")
	switch {
	case err != nil:
		return false, "터미널 못 읽음"
	case !found || t.State != "live" || t.Cwd == "":
		return false, "터미널 없음"
	}
	const lim = 30
	o, rc, e := boundedRun((lim+10)*time.Second, "hc", nil, "exec", "--on", r.machine, "--json", "--timeout", strconv.Itoa(lim),
		"--", "sh", "-c", "n=$1; shift; "+verified.FreshSh, "_", strconv.Itoa(min), t.Address, t.Cwd, id8)
	x, err := hcjson.ParseExec([]byte(o), []byte(e), rc)
	switch {
	case err != nil || x.Code != 0 || x.TimedOut:
		return false, "갱신 못 읽음"
	case strings.TrimSpace(string(x.Stdout)) == "":
		return false, "갱신 없음"
	}
	return true, ""
}

// overWait: 상한을 넘긴 맨 앞이 진행 중이라 더 기다린다 — 화면에 바퀴마다, 일지에 맨 앞 제출마다 한 줄(<일지 폴더>/overwait에 그 SHA)
func (m *M) overWait(s specStep, now int64) {
	what := fmt.Sprintf("추측 상한 넘김 — 진행 중이라 더 기다림(%d분째, %s, 끝 상한 merger_spec_timeout %d초)", (now-s.row.at)/60, s.row.machine, specTimeout())
	fmt.Printf("hc-lane merger: 맨 앞 %s %s\n", s.g.lanes, what)
	p := m.mj + "/overwait"
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == m.b[0].s {
		return
	}
	m.jmembers(m.b, jOverWait, what, "", s.row.at, now)
	if err := writeVia(p, m.b[0].s+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 상한 넘김 표식을 적지 못했다(다음 바퀴에 일지 줄이 또 나온다): %v\n", err)
	}
}

// frontDispatch: 맨 앞 무리도 검증 기계에 맡기나. 손잡이 파일 <병합기 일지 폴더>/front_dispatch가 있으면 그 값, 없으면
// HC_LANE_FRONT_DISPATCH, 둘 다 없으면 1. 0이면 옛 길 그대로(맨 앞은 병합기가 직접 검증 — 이 레인 앞의 동작으로 실행 중에 되돌리기)
func (m *M) frontDispatch() bool { return m.knob("front_dispatch", "HC_LANE_FRONT_DISPATCH") }

// knob: 켬·끔 손잡이 — <병합기 일지 폴더>/<파일>(바퀴마다 읽어 바로 먹는다)이 있으면 그 값, 없으면 환경 변수, 둘 다 없으면 켬.
// 0이 아닌 정수는 켬, 정수가 아니면 경고와 함께 켬
func (m *M) knob(file, env string) bool { return m.knobInt(file, env, 1, 0) != 0 }

// frontLocal: 맡길 수 없을 때 병합기 기계가 직접 검증하나. 손잡이 파일 <병합기 일지 폴더>/front_local(바퀴마다 읽어 바로 먹는다)이
// 있으면 그 값, 없으면 HC_LANE_FRONT_LOCAL, 둘 다 없으면 1. 0이 아닌 값은 모두 1
func (m *M) frontLocal() bool { return m.knob("front_local", "HC_LANE_FRONT_LOCAL") }

// 맨 앞 실패의 갈래
const (
	ffNone    = iota // 실패가 아니다(통과·도는 중·충돌·줄 없음) 또는 한 번 더 확인을 띄웠다
	ffUnknown        // 모름 — 손잡이로(직접 검증 또는 기다림)
	ffReject         // 혼자 · 두 기계에서 시험 실패 — 반송
	ffShrink         // 묶음 · 시험 실패 — 마지막을 빼고 다시
)

// frontFail: 맨 앞 추측 실패의 갈래와 근거(실패 기록의 칸·첫 실패 시험 줄·첫 실패 메모)
type frontFail struct {
	kind  int
	kv    map[string]string
	test  string
	first string
}

// specFrontRedo: 짓기 전에(뒤 추측보다 먼저 빈 기계를 쓴다) 맨 앞 추측이 실패·모름이면 갈래를 정하고, 한 번 더 확인 또는 손잡이 0의
// 다시 띄움을 한다
func (m *M) specFrontRedo(c *specCtx, st []specStep, busy map[string]bool, now int64) frontFail {
	if len(st) == 0 || st[0].row == nil {
		return frontFail{}
	}
	r, g := st[0].row, st[0].g
	if r.state == specBusyX { // 맡은 기계가 그새 바빠 검증하지 않음 — 기다림 상한 없이 곧바로 다른 한가한 기계로(verifierpick.go)
		if m.specBusyRedo(c, r, busy, g.lanes, now) {
			return frontFail{}
		}
		// 다시 고를 기계가 없음·바쁨 횟수 넘김 — 손잡이 front_local(0이면 아래 다시 띄움이 바쁨을 낸 기계까지 다시 본다)
	} else if r.state != specFail && r.state != specLost {
		return frontFail{} // 통과·도는 중·충돌
	}
	if m.hb != "" { // 착지 스크립트 저장소 — 추측 실패는 반송·한 번 더 확인의 근거가 아니다(착지 스크립트가 스스로 검증한다)
		if r.state == specFail {
			fmt.Printf("hc-lane merger: 맨 앞 %s 추측 실패 — 착지 스크립트 저장소라 반송 근거로 쓰지 않는다(착지 스크립트가 검증한다)\n", g.lanes)
		}
		if r.state != specBusyX && m.workerRedo(c, st[0], busy, now, r.state.String()) { // 착지 스크립트가 병합기 기계에서 검증하기 전에
			return frontFail{}
		}
		return frontFail{kind: ffUnknown}
	}
	first, again := specRecheckOf(r)
	switch r.state {
	case specFail:
		kv, test, ok := m.specFailRec(c, r.id)
		switch {
		case !ok || kv["test"] != "1" || test == "" || kv["exit"] == "":
			fmt.Printf("hc-lane merger: 맨 앞 %s 추측 실패는 시험 실패와 환경 문제를 가를 수 없다(모름)\n", g.lanes)
		case len(g.shas) > 1:
			return frontFail{kind: ffShrink, kv: kv, test: test}
		case again:
			return frontFail{kind: ffReject, kv: kv, test: test, first: first}
		default:
			note := fmt.Sprintf("recheck machine=%s exit=%s log=%s\n%s", kv["machine"], kv["exit"], kv["log"], test)
			if m.specRedo(c, r, busy, nil, kv["machine"], note, g.lanes, now) {
				fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 시험 실패 — 반송 전에 %s 에서 한 번 더 확인\n", g.lanes, kv["machine"], r.machine)
				return frontFail{}
			}
			fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 시험 실패 — 한 번 더 확인할 빈 검증 기계가 없다(모름)\n", g.lanes, kv["machine"])
		}
	}
	// 손잡이 0의 다시 띄움은 앞 시도(at)에서 HC_LANE_SPEC_START(기본 300초) 뒤에 — 닿지 않는 기계에 바퀴마다 원격 쓰기 둘·hc run을
	// 되풀이하지 않게(감사 atozd0z-zklwye 낮음)
	if !m.frontLocal() && now-r.at >= specStartMax() && m.specRedo(c, r, busy, nil, "", first, g.lanes, now) {
		return frontFail{}
	}
	// 손잡이 1 — 병합기 기계에서 직접 검증하기 전에 한가한 다른 검증 기계에 한 번(바쁨은 specBusyRedo가 이미 다시 골랐다)
	if m.frontLocal() && r.state != specBusyX && m.workerRedo(c, st[0], busy, now, r.state.String()) {
		return frontFail{}
	}
	return frontFail{kind: ffUnknown}
}

// specRecheckOf: 이 추측이 한 번 더 확인인가 — spec 커밋 본문의 첫 실패 메모(「recheck machine=… exit=… log=…」 + 첫 실패 줄)
func specRecheckOf(r *specRow) (string, bool) {
	body, rc := out("cat-file", "commit", r.commit)
	if rc != 0 {
		return "", false
	}
	_, msg, _ := strings.Cut(body, "\n\n")
	_, note, _ := strings.Cut(msg, "\n\n")
	note = strings.TrimSpace(note)
	return note, strings.HasPrefix(note, "recheck ")
}

// specRedo: 맨 앞 추측 r을 한가한 검증 기계(skip 뺌, avoid가 아닌 것 먼저, 없으면 avoid)에 다시 띄운다 — spec 참조를 같은 트리·부모로
// 그 기계·지금 시각(본문 note)으로 고쳐 쓰고 실패 기록·떴다는 표식을 같은 push로 지운다(본 값 lease — 그 사이 일꾼이 쓴 결과는 지우지 않는다).
// 띄웠으면 true(r·c·busy를 고친다)
func (m *M) specRedo(c *specCtx, r *specRow, busy, skip map[string]bool, avoid, note, lanes string, now int64) bool {
	v := m.specPick(c, busy, skip, avoid)
	if v.Machine == "" {
		return false
	}
	msg := specMsg(r.parent, r.group, v.Machine, now, "run")
	if note != "" {
		msg += "\n\n" + note
	}
	cm, ok := commitTree("", r.tree, r.commit+"^", msg)
	if !ok {
		return false
	}
	ref := specRefs + r.id
	args, specs := []string{"push", "-q", "--atomic", "--force-with-lease=" + ref + ":" + r.commit}, []string{cm + ":" + ref}
	gone := []string{specFails + r.id, specStarts + r.id}
	for _, x := range gone {
		if sha := c.refs[x]; sha != "" {
			args, specs = append(args, "--force-with-lease="+x+":"+sha), append(specs, ":"+x)
		}
	}
	if runO(append(append(args, "origin"), specs...)...) != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 맨 앞 추측 %s 를 다시 띄울 참조를 올리지 못했다 — 다음 바퀴에\n", first8(r.id))
		return false
	}
	for _, x := range gone {
		delete(c.refs, x)
	}
	c.refs[ref] = cm
	r.commit, r.machine, r.at, r.dispatchX, r.busy = cm, v.Machine, now, false, false
	m.specLaunch(r, v, lanes, now)
	busy[v.Machine] = true // 못 띄웠어도 이 바퀴에는 그 기계를 다시 고르지 않는다
	return r.state == specRun
}

// specFrontLand: 맨 앞 판정이 「지금처럼 착지」로 나왔을 때 — 반송·줄이기·직접 검증·기다림(손잡이 0) 가운데 하나
func (m *M) specFrontLand(c *specCtx, st []specStep, ff frontFail) specDecision {
	lanes := st[0].g.lanes
	switch ff.kind {
	case ffReject:
		fm := "-"
		for _, f := range strings.Fields(strings.SplitN(ff.first, "\n", 2)[0]) {
			if v, ok := strings.CutPrefix(f, "machine="); ok {
				fm = v
			}
		}
		m.clog = ff.kv["machine"] + ":" + ff.kv["log"]
		m.cwhy = fmt.Sprintf("착지 실패(종료 %s, 추측 검증 — %s·%s 에서 기준 %s 위 시험 실패 두 번, 첫 실패 「%s」)", ff.kv["exit"], fm, ff.kv["machine"], first8(m.B), ff.test)
		fmt.Printf("hc-lane merger: 맨 앞 %s 가 두 검증 기계(%s·%s)에서 시험 실패 — 병합기 검증 없이 반송\n", lanes, fm, ff.kv["machine"])
		return specReject
	case ffShrink:
		m.clog = ff.kv["machine"] + ":" + ff.kv["log"]
		m.cwhy = fmt.Sprintf("묶음 %d 추측 검증 실패(종료 %s, %s 에서 시험 실패, 첫 실패 「%s」) — 마지막 %s 빼고 다시", len(m.b), ff.kv["exit"], ff.kv["machine"], ff.test, m.b[len(m.b)-1].l)
		fmt.Printf("hc-lane merger: 묶음 %s 가 %s 에서 시험 실패 — 병합기 검증 없이 마지막을 빼고 다시\n", lanes, ff.kv["machine"])
		return specShrink
	}
	if r := st[0].row; r != nil && r.state == specConflict {
		return specLand // 부모 위에 못 얹음·병합 풀이 — 착지가 원본 그대로 판정(검증 전 rebase에서 멈춘다)
	}
	if m.frontLocal() || !m.hasVerifier(c.vers) || m.hb != "" {
		if !m.frontLocal() && m.hb != "" {
			m.scriptNote()
		}
		fmt.Printf("hc-lane merger: 맨 앞 %s 를 병합기가 직접 검증한다(%s)\n", lanes, m.vw.why)
		return specLand
	}
	m.frontHold(m.vw.why)
	m.wmach = "-"
	return specWaitFront
}

// soloKeep: cap 바퀴에 지은 맨 앞 혼자 추측(<일지 폴더>/solo의 아이디) — 치우기에서 길처럼 지킨다(cap이 끝나면 capEnd가 solo를 지운다)
func (m *M) soloKeep(c *specCtx) []specStep {
	b, err := os.ReadFile(m.mj + "/solo")
	if err != nil || m.capN() == 0 {
		return nil
	}
	id := strings.TrimSpace(string(b))
	for i := range c.rows {
		if c.rows[i].id == id {
			return []specStep{{row: &c.rows[i]}}
		}
	}
	return nil
}

// hasVerifier: 병합기 기계 말고 검증 기계가 목록에 있나
func (m *M) hasVerifier(vs []verified.Verifier) bool {
	for _, v := range vs {
		if v.Machine != m.mme {
			return true
		}
	}
	return false
}

// scriptNote: 착지 스크립트 저장소에서 손잡이 front_local 0이 안 먹는다는 한 줄 — 맨 앞 제출마다 한 번(<일지 폴더>/frontscript에 그 SHA)
func (m *M) scriptNote() {
	p := m.mj + "/frontscript"
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == m.b[0].s {
		return
	}
	fmt.Println("hc-lane merger: front_local 0은 착지 스크립트 저장소에서 안 먹는다 — 착지 스크립트가 검증한다")
	if err := writeVia(p, m.b[0].s+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 알림 표식을 적지 못했다(다음 바퀴에 같은 줄이 또 나온다): %v\n", err)
	}
}

// overRedo: 맨 앞 추측이 기다림 상한을 넘겼다 — 서서 기다리거나(손잡이 0) 병합기 기계에서 직접 검증하기(손잡이 1·착지 스크립트
// 저장소) 전에 한가한 다른 검증 기계에 한 번 더 맡긴다(workerRedo)
func (m *M) overRedo(c *specCtx, s specStep, busy map[string]bool, now int64) bool {
	return s.row != nil && s.row.state == specRun && m.workerRedo(c, s, busy, now, "기다림 상한을 넘겼다")
}

// workerRedo: 맨 앞 추측(상한 넘김·실패·모름)을 병합기 기계에서 직접 검증하기 전에 한가한 다른 검증 기계에 한 번 더 맡긴다
// (ht-merger-spec-reuse — 「직접 검증」은 맥이 아니라 한가한 워커가, 워커가 없을 때만 병합기 기계가). 같은 아이디라 새 일꾼은 앞 실행이
// 남긴 통과 조각을 이어 쓴다(specparts.go). spec 참조를 새 기계로 고쳐 쓰니 느린 기계의 뒤늦은 결과는 남지 않는다(일꾼이 참조가 바뀐
// 것을 보고 버린다 — 그새 쓴 통과 기록은 같은 트리의 기록이라 뜻이 같다). 맨 앞 제출마다 한 번(<일지 폴더>/overredo에 그 SHA) — 새
// 기계에서도 안 되면 손잡이대로(검증이 늘 상한보다 길 때 기계 사이를 끝없이 옮겨 다니며 결과를 버리지 않게, 감사 atp02sl-4fkdo9 중간).
// 맡길 한가한 기계가 없으면 false. 맡겼으면 m.wre에 「<옛 기계>→<새 기계>」
func (m *M) workerRedo(c *specCtx, s specStep, busy map[string]bool, now int64, why string) bool {
	r := s.row
	if r == nil || r.state == specConflict {
		return false
	}
	p := m.mj + "/overredo"
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == m.b[0].s {
		return false // 이 맨 앞 제출은 이미 한 번 옮겼다
	}
	from := r.machine
	note, again := specRecheckOf(r) // 한 번 더 확인 중이던 추측이면 첫 실패 메모를 이어 준다
	if !again {
		note = ""
	}
	if !m.specRedo(c, r, busy, map[string]bool{from: true}, "", note, s.g.lanes, now) {
		return false
	}
	fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 %s — 한가한 %s 에 다시 맡긴다(통과한 조각은 이어 쓴다)\n", s.g.lanes, from, why, r.machine)
	m.wre = from + "→" + r.machine
	if err := writeVia(p, m.b[0].s+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 다시 맡김 표식을 적지 못했다(다음 상한에 또 옮길 수 있다): %v\n", err)
	}
	return true
}

// frontHold: 손잡이 0으로 맨 앞이 기다린다 — 주인 지휘에 막힘 사건을 맨 앞 제출마다 한 번(<일지 폴더>/fronthold에 그 SHA, 사건 열쇠도 같음)
func (m *M) frontHold(why string) {
	e := m.b[0]
	fmt.Printf("hc-lane merger: 맨 앞 %s 검증을 맡길 수 없다(%s) — 손잡이 front_local 0: 직접 검증하지 않고 기다린다\n", e.l, why)
	p := m.mj + "/fronthold"
	if b, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(b)) == e.s {
		return
	}
	msg := fmt.Sprintf("병합기 맨 앞 %s 검증을 검증 기계에 맡기지 못해 기다린다(%s, 손잡이 front_local 0 — 직접 검증 안 함). 직접 검증으로 넘기려면 %s/front_local 에 1 (맨 앞 %s)", e.l, why, m.mj, e.s)
	owner := e.ow
	if owner == "" {
		owner = "*"
	}
	args := []string{"report", "막힘", "-", msg, "--name", "merger", "--key", "hc-merger-front:" + e.s}
	if e.ow != "" {
		args = append(args, "--to", e.ow)
	}
	if _, rc, er := boundedRun(reportMax, gSelf, []string{"HC_LANE_OWNER=" + owner, "HC_LANE_OWNER_RUN=" + e.owr}, args...); rc != 0 {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 맨 앞 기다림 사건을 적지 못했다(종료 %d) — 다음 바퀴에 다시: %s\n", rc, strings.TrimSpace(er))
		return
	}
	if err := writeVia(p, e.s+"\n"); err != nil {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 맨 앞 기다림 표식을 적지 못했다(사건 열쇠가 겹침을 막는다): %v\n", err)
	}
}
