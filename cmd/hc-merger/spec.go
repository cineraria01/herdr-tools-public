package main

// 추측 병렬 검증(merger-spec, 2026-11-22 사용자 「병합기에 추측 병렬 검증을 넣을까요?」에 「넣기 — 지금 바로」).
//
// 큐가 A·B·C면 검증 기계 여러 대에서 main+A, main+A+B, main+A+B+C를 함께 검증해 두고, 착지는 지금처럼 차례로 한다.
// 정확성은 착지 길이 지킨다: 착지(hc-lane land)는 rebase한 **바로 그 트리**의 검증 기록(refs/herdr/verified/<트리>, internal/verified)이
// 있을 때만 HERDR_VERIFIED_TREE를 검증 스크립트에 넘기고, 스크립트는 지금 HEAD^{tree}가 그 값일 때만 건너뛴다. 그 사이 main이 움직였거나
// 추측과 rebase 결과가 한 바이트라도 다르면 트리가 달라 기록이 없다 → 지금처럼 병합기 기계에서 검증(처리량만 잃고 정확성은 그대로).
//
// 단위
//   - 무리(group): 착지 한 번에 들어갈 것 — 맨 앞은 이번 바퀴의 묶음(m.b, merger-batch-audited 포함), 그 뒤는 큐 차례를 묶음 크기씩
//     자른 것(다음 바퀴의 pickBatch가 고를 꼴 — 다르게 고르면 열쇠가 달라 그 추측은 안 쓰일 뿐). 열쇠 = 제출 SHA들을 +로 이음.
//     맨 앞 무리에 줄이 없으면 그 추측은 짓기만 하고 띄우지 않는다(state=local — 병합기가 지금처럼 직접 검증하며, 뒤 추측의 부모가 된다).
//   - 추측(spec): 「부모 트리 + 무리」 하나. 아이디 = sha256(부모 트리 + " " + 열쇠) 앞 40자. 원격 참조
//     refs/hc-merger/spec/<아이디> → 커밋(트리 = 그 추측의 후보 트리, 부모 = 부모 추측 커밋 또는 origin/<기본>, 제목 specMsg).
//     충돌로 못 지은 추측도 참조를 둔다(state=conflict, 트리 = 부모 트리) — 바퀴마다 다시 짓지 않게.
//   - 결과: 통과 = refs/herdr/verified/<후보 트리>(검증 기계가 씀), 실패 = refs/hc-merger/spec-fail/<아이디>(검증 기계가 씀, 제목에 종료·기계·로그 경로),
//     둘 다 없으면 도는 중. 시간 넘김(HC_LANE_SPEC_TIMEOUT, 기본 3600초)·띄우기 실패면 모름 — 그 무리는 착지 때 병합기 기계가 검증.
//   - 일지 = 원격 참조 자체(병합기가 죽어도 다음 병합기가 ls-remote로 다시 읽는다 — 로컬 상태 파일 없음).
//
// 바퀴(round.go 갈고리 하나 — stack 뒤 runCandidate 앞, m.spec()):
//  1. 원격 spec·spec-fail·verified 목록과 검증 기계 목록(refs/herdr/spec-verifiers — hc-fill이 설정 줄 「merger_verifiers <프로젝트> <기계,…>」와
//     repo 줄의 그 기계 경로로 만든 커밋, 메시지에 「<기계>\t<클론 경로>」 줄들 — verified.ParseVerifiers)을 바퀴의 ls-remote 한 번에 함께 읽는다.
//     목록도 추측 참조도 없으면 추측 없음(지금 그대로 — 원격 호출도 늘지 않는다). 병합기 기계 자신은 검증 기계로 쓰지 않는다.
//  2. specWalk: 기준 트리에서 무리를 차례로 따라간다. 통과·도는 중이면 그 트리로 앞으로(도는 중은 통과로 보고 더 깊이 추측),
//     실패·충돌이면 그 무리를 건너뛰고 부모 그대로(뒤 무리는 그것을 빼고 추측 — 「B 실패 → main+A+C」), 모름이면 그 트리로 앞으로.
//     줄이 없는 첫 무리에서 멈춘다 → 빈 검증 기계가 있으면 그 추측을 짓고(임시 작업 트리에서 부모 커밋 위에 rebase --reapply-cherry-picks —
//     착지의 rb와 같은 명령) push한 뒤 그 기계에 띄우고 다시 따라간다.
//  3. 맨 앞 무리: 통과면 착지(착지가 기록을 보고 검증을 건너뜀) · 도는 중이면 착지하지 않고 이 바퀴를 끝냄(다음 바퀴에 다시) ·
//     그 밖(실패·충돌·모름·줄 없음)이면 지금처럼 착지(병합기 기계 검증 — 실패면 지금 규칙대로 반송, 묶음이면 줄여서 다시).
//  4. 따라간 길에 없는 추측(착지했거나 반송됐거나 main이 움직임)은 도는 중이 아니면 참조 셋을 지운다(본 값 lease). 도는 중이면 그 기계가
//     끝낼 때까지 둔다(그 기계는 그동안 바쁨 — 검증 기계 일꾼은 결과를 쓰기 전에 spec 참조가 아직 있는지 본다).
//
// 검증 기계 일꾼(hc-merger --spec-verify <아이디> <기본 브랜치>, hc run --on <기계> --bg --name spec-<아이디 8> --cwd <클론>):
// spec 참조를 받아 <클론>.spec-wt/<아이디 8>에 분리 작업 트리 → 준비 훅 → bash .claude/herdr-verify.sh(HC_LANE=spec-<아이디 8>) →
// 통과면 verified.Record, 실패면 spec-fail 참조 → 작업 트리 정리. 검증 자리는 저장소 검증 스크립트의 기계 잠금(기계별 검증 자리 수 등)이
// 레인 검증과 나눠 쓴다. 한 기계에 추측 하나씩.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type specState int

const (
	specRun      specState = iota // 검증 기계에서 도는 중(결과 참조 없음)
	specPass                      // refs/herdr/verified/<트리> 있음
	specFail                      // refs/hc-merger/spec-fail/<아이디> 있음
	specConflict                  // 부모 위에 얹으면 충돌(지을 수 없음)
	specLost                      // 시간 넘김·띄우기 실패 — 모름
)

func (s specState) String() string {
	return [...]string{"도는 중", "통과", "실패", "충돌", "모름"}[s]
}

// specRow: 원격 추측 하나
type specRow struct {
	id, commit, tree    string // 아이디 · spec 참조 커밋 · 후보 트리
	parent, group       string // 부모 트리 · 무리 열쇠
	machine             string // 띄운 검증 기계(충돌·띄우기 실패면 -)
	at                  int64  // 띄운 시각(유닉스 초)
	state               specState
	conflict, dispatchX bool // 제목의 state=conflict · state=lost(띄우기 실패)
}

// specGroup: 무리 하나
type specGroup struct {
	key   string   // 제출 SHA들을 +로
	shas  []string // 제출 SHA(큐 차례)
	lanes string   // 화면용 a+b
}

func newGroup(es []entry) specGroup {
	g := specGroup{}
	ls := make([]string, len(es))
	for i, e := range es {
		g.shas = append(g.shas, e.s)
		ls[i] = e.l
	}
	g.key, g.lanes = strings.Join(g.shas, "+"), strings.Join(ls, "+")
	return g
}

func specID(parent, key string) string {
	h := sha256.Sum256([]byte(parent + " " + key))
	return hex.EncodeToString(h[:])[:40]
}

const specHead = "hc-merger-spec"

// specMsg: spec 참조 커밋 제목
func specMsg(parent, key, machine string, at int64, state string) string {
	return fmt.Sprintf("%s parent=%s group=%s machine=%s at=%d state=%s", specHead, parent, key, machine, at, state)
}

// parseSpec: spec 참조 커밋의 제목을 읽는다(트리·커밋·아이디는 부른 쪽이 채운다)
func parseSpec(subject string) (specRow, bool) {
	f := strings.Fields(subject)
	if len(f) != 6 || f[0] != specHead {
		return specRow{}, false
	}
	var r specRow
	for _, kv := range f[1:] {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "parent":
			r.parent = v
		case "group":
			r.group = v
		case "machine":
			r.machine = v
		case "at":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return specRow{}, false
			}
			r.at = n
		case "state":
			switch v {
			case "run":
			case "conflict":
				r.conflict = true
			case "lost", "local": // 띄우기 실패 · 맨 앞이라 병합기가 직접 검증(둘 다 모름 — 길은 그 트리로 앞으로, 착지는 지금처럼)
				r.dispatchX = true
			default:
				return specRow{}, false
			}
		default:
			return specRow{}, false
		}
	}
	return r, r.parent != "" && r.group != "" && r.machine != ""
}

// specResolve: 원격 결과로 줄마다 상태를 정한다. verified = 기록이 있는 트리, failed = spec-fail이 있는 아이디
func specResolve(rows []specRow, verified, failed map[string]bool, now, timeout int64) {
	for i := range rows {
		r := &rows[i]
		switch {
		case r.conflict:
			r.state = specConflict
		case verified[r.tree]:
			r.state = specPass
		case failed[r.id]:
			r.state = specFail
		case r.dispatchX || timeout > 0 && now-r.at > timeout:
			r.state = specLost
		default:
			r.state = specRun
		}
	}
}

// specStep: 따라간 길의 한 자리
type specStep struct {
	g      specGroup
	parent string   // 그 무리를 얹는 부모 트리
	row    *specRow // 없으면 nil — 지어야 한다
}

// specWalk: 기준 트리에서 무리를 차례로 따라간다(위 2). 줄이 없는 첫 무리에서 멈추고 그 자리를 마지막 단계(row nil)로 돌려준다
func specWalk(base string, groups []specGroup, rows []specRow) []specStep {
	by := map[string]*specRow{}
	for i := range rows {
		by[rows[i].id] = &rows[i]
	}
	var st []specStep
	cur := base
	for _, g := range groups {
		r := by[specID(cur, g.key)]
		st = append(st, specStep{g: g, parent: cur, row: r})
		if r == nil {
			break
		}
		switch r.state {
		case specPass, specRun, specLost:
			cur = r.tree
		}
	}
	return st
}

// specDecision: 맨 앞 무리를 어떻게 할지(위 3)
type specDecision int

const (
	specLand      specDecision = iota // 지금처럼 착지(병합기 기계 검증)
	specLandFast                      // 착지 — 기록이 있어 착지가 검증을 건너뛴다
	specWaitFront                     // 맨 앞 추측이 도는 중 — 이 바퀴는 착지하지 않는다
)

func specFront(st []specStep) specDecision {
	if len(st) == 0 || st[0].row == nil {
		return specLand
	}
	switch st[0].row.state {
	case specPass:
		return specLandFast
	case specRun:
		return specWaitFront
	}
	return specLand
}

// specBusy: 도는 중인 추측이 있는 기계(길 위든 아니든 — 끝날 때까지 그 기계를 쓰지 않는다)
func specBusy(rows []specRow) map[string]bool {
	b := map[string]bool{}
	for _, r := range rows {
		if r.state == specRun {
			b[r.machine] = true
		}
	}
	return b
}

// specStale: 길에 없고 도는 중도 아닌 추측(지울 것, 위 4)
func specStale(st []specStep, rows []specRow) []specRow {
	on := map[string]bool{}
	for _, s := range st {
		if s.row != nil {
			on[s.row.id] = true
		}
	}
	var out []specRow
	for _, r := range rows {
		if !on[r.id] && r.state != specRun {
			out = append(out, r)
		}
	}
	return out
}
