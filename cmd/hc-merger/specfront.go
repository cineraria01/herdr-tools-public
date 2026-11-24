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

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/verified"
)

// frontDispatch: 맨 앞 무리도 검증 기계에 맡기나. 손잡이 파일 <병합기 일지 폴더>/front_dispatch가 있으면 그 값, 없으면
// HC_LANE_FRONT_DISPATCH, 둘 다 없으면 1. 0이면 옛 길 그대로(맨 앞은 병합기가 직접 검증 — 이 레인 앞의 동작으로 실행 중에 되돌리기)
func (m *M) frontDispatch() bool { return m.knob("front_dispatch", "HC_LANE_FRONT_DISPATCH") }

// knob: 켬·끔 손잡이 — <병합기 일지 폴더>/<파일>(바퀴마다 읽어 바로 먹는다)이 있으면 그 값, 없으면 환경 변수, 둘 다 없으면 켬.
// 0이 아닌 정수는 켬, 정수가 아니면 경고와 함께 켬
func (m *M) knob(file, env string) bool {
	v := os.Getenv(env)
	if b, err := os.ReadFile(m.mj + "/" + file); err == nil {
		v = strings.TrimSpace(string(b))
	}
	return intSetting(env, v, 1, 0) != 0
}

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
	if r.state != specFail && r.state != specLost {
		return frontFail{} // 통과·도는 중·충돌
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
			if m.specRedo(c, r, busy, kv["machine"], note, g.lanes, now) {
				fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 시험 실패 — 반송 전에 %s 에서 한 번 더 확인\n", g.lanes, kv["machine"], r.machine)
				return frontFail{}
			}
			fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 시험 실패 — 한 번 더 확인할 빈 검증 기계가 없다(모름)\n", g.lanes, kv["machine"])
		}
	}
	// 손잡이 0의 다시 띄움은 앞 시도(at)에서 HC_LANE_SPEC_START(기본 300초) 뒤에 — 닿지 않는 기계에 바퀴마다 원격 쓰기 둘·hc run을
	// 되풀이하지 않게(감사 atozd0z-zklwye 낮음)
	if !m.frontLocal() && now-r.at >= specStartMax() && m.specRedo(c, r, busy, "", first, g.lanes, now) {
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

// specRedo: 맨 앞 추측 r을 빈 검증 기계(avoid가 아닌 것 먼저, 없으면 avoid)에 다시 띄운다 — spec 참조를 같은 트리·부모로 그 기계·지금
// 시각(본문 note)으로 고쳐 쓰고 실패 기록·떴다는 표식을 같은 push로 지운다(본 값 lease — 그 사이 일꾼이 쓴 결과는 지우지 않는다).
// 띄웠으면 true(r·c·busy를 고친다)
func (m *M) specRedo(c *specCtx, r *specRow, busy map[string]bool, avoid, note, lanes string, now int64) bool {
	v := m.specFreeBut(c.vers, busy, avoid)
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
	r.commit, r.machine, r.at, r.dispatchX = cm, v.Machine, now, false
	m.specLaunch(r, v, lanes, now)
	busy[v.Machine] = true // 못 띄웠어도 이 바퀴에는 그 기계를 다시 고르지 않는다
	return r.state == specRun
}

// specFreeBut: 빈 검증 기계 — avoid가 아닌 것을 목록 차례로 먼저, 없으면 avoid(비었으면). 병합기 기계 자신은 뺀다
func (m *M) specFreeBut(vs []verified.Verifier, busy map[string]bool, avoid string) verified.Verifier {
	var same verified.Verifier
	for _, v := range vs {
		switch {
		case busy[v.Machine] || v.Machine == m.mme:
		case v.Machine != avoid:
			return v
		default:
			same = v
		}
	}
	return same
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
	if m.frontLocal() || !m.hasVerifier(c.vers) {
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
