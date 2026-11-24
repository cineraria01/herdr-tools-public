// Package knob: 실행 중에 바로 먹는 공통 손잡이(2026-11-24 ops-knobs, docs/ops/knobs.md).
//
// 손잡이가 될 수 있는 값은 등록 표 Defs에 있는 것뿐이다 — 착지 판정·보호 규칙·훅·잠금 같은 안전 관문은 올리지 않는다.
// 값은 git 밖 상태 파일(Path)에 「이름·값·누가·왜·언제·언제까지·고정」으로 두고, 우선순위는 손잡이 > 설정 > 기본.
// 기한이 지난 값·범위 밖 값·표에 없는 이름은 읽을 때 버린다(설정 값으로 돌아감). 파일이 깨졌으면 손잡이가 없는 것으로 읽는다.
package knob

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/lockx"
)

// Def: 등록 표 한 줄. Enum이 있으면 그 낱말 가운데 하나, 없으면 Min~Max 정수
type Def struct {
	Name     string   // 손잡이 이름(Tool의 설정 키와 같으면 그 키를 덮는다)
	Tool     string   // 읽는 도구
	Default  string   // 설정에도 없을 때 도구가 쓰는 값(표시용 — 도구 코드의 기본과 같게)
	Min, Max int64    // 정수 범위(닫힌 구간)
	Enum     []string // 낱말 값
	List     bool     // 쉼표로 나눈 이름 목록(글자 a-zA-Z0-9_.-), 「-」 = 비움
	Unit     string   // 단위(분·시간 등, 표시용)
	Remote   bool     // 다른 기계의 도구가 읽음 — 원격 참조 Ref에 싣는다
	Desc     string
	Per      string // 묶음 손잡이: Name이 「접두@」이고 저장 이름은 「접두@<Per 값>」(예: seats@<기계>). 빈 값이면 낱개
}

// Defs: 등록 표. 새 손잡이는 여기에 한 줄 더하고, 도구는 한 함수(hc-fill은 conf, 다른 기계는 ReadRemote → Remote.Get)로 읽는다
var Defs = []Def{
	{Name: "idle", Tool: "hc-fill", Default: "10", Min: 0, Max: 1440, Unit: "분", Desc: "놀고 있음 알림(자리가 남은 채 이만큼 이어지면, 0이면 끔)"},
	{Name: "hold", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "쉰 쓰기 레인의 자리를 안 세기까지(0이면 늘 셈)"},
	{Name: "done_stall", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "완료 뒤 멈춤 알림(0이면 끔)"},
	{Name: "wake_every", Tool: "hc-fill", Default: "10", Min: 1, Max: 240, Unit: "분", Desc: "감사 판정 수거 깨우기 간격"},
	{Name: "ack_max", Tool: "hc-fill", Default: "30", Min: 1, Max: 1440, Unit: "분", Desc: "판정 수거 기다림 상한(넘으면 주인 지휘에 한 번)"},
	{Name: "early_wait", Tool: "hc-fill", Default: "5", Min: 1, Max: 48, Unit: "시간", Desc: "먼저 띄운 레인이 선행을 기다리는 기한"},
	{Name: "auto_submit", Tool: "hc-fill", Default: "-", List: true, Unit: "프로젝트", Desc: "감사 통과 뒤 쉬는 레인 대신 대기열이 제출할 프로젝트(쉼표, - = 끔)"},
	{Name: "auto_submit_max", Tool: "hc-fill", Default: "120", Min: 10, Max: 600, Unit: "초", Desc: "대기열 제출 한 번의 시간 상한(넘으면 에이전트에 넘김)"},
	{Name: "pr", Tool: "hc-fill", Default: "-", List: true, Unit: "프로젝트", Desc: "PR 거울을 켤 프로젝트(쉼표, - = 끔 — docs/hc-fill/pr.md)"},
	{Name: "pr_max", Tool: "hc-fill", Default: "20", Min: 1, Max: 200, Unit: "번", Desc: "PR 거울의 바퀴당 gh 호출 상한(넘으면 다음 바퀴에)"},
	{Name: "pr_fail", Tool: "hc-fill", Default: "3", Min: 1, Max: 100, Unit: "바퀴", Desc: "gh 실패가 이만큼 바퀴 이어지면 운영 사건 hc-fill-pr 한 번"},
	{Name: "pr_inbox", Tool: "hc-fill", Default: "1", Min: 0, Max: 1, Desc: "완료 없이 제출하는 레인(받은편지·--no-audit)도 SHA가 든 첫 보고(중간·제출)로 PR을 연다(0이면 그 레인은 완료만)"},
	{Name: "pr_timeout", Tool: "hc-fill", Default: "20", Min: 5, Max: 120, Unit: "초", Desc: "PR 거울 gh 호출 하나의 시간 상한"},
	// 부하 보고 고르기(2026-11-24 fill-load-steer, docs/hc-fill/load-steer.md). 코어당 부하는 ×100 정수(%)
	{Name: "load_band", Tool: "hc-fill", Default: "25", Min: 0, Max: 10000, Unit: "%", Desc: "코어당 부하 차이가 이 안이면 비슷함 — 설정 machine 차례"},
	{Name: "load_busy", Tool: "hc-fill", Default: "300", Min: 0, Max: 10000, Unit: "%", Desc: "후보가 모두 코어당 부하 이 값을 넘으면 그 바퀴는 안 띄움(0이면 끔)"},
	{Name: "load_starve", Tool: "hc-fill", Default: "30", Min: 0, Max: 1440, Unit: "분", Desc: "모두 바쁨이어도 이만큼 기다린 줄은 가장 덜 바쁜 기계에"},
	{Name: "skew_min", Tool: "hc-fill", Default: "10", Min: 1, Max: 1440, Unit: "분", Desc: "한 기계가 load_busy를 넘은 채 한가한 기계가 있으면 이만큼 뒤 치우침 사건"},
	{Name: "skew_every", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "치우침 사건 되풀이 간격(0이면 끔)"},
	{Name: "unreach", Tool: "hc-fill", Default: "10", Min: 0, Max: 1440, Unit: "분", Desc: "레인이 있는 기계가 이만큼 연결 안 됨이면 주인 지휘에 한 번(0이면 끔, docs/hc-fill/unreach.md)"},
	// 상태 한 장의 「눈에 띄는 것」 문턱(2026-11-24 ops-state, docs/ops/state.md) — 사실만 보이고 조치는 하지 않는다
	{Name: "state_stage", Tool: "hc-fill", Default: "90", Min: 0, Max: 10080, Unit: "분", Desc: "레인이 같은 단계에 이만큼 머물면 눈에 띄는 것(0이면 끔)"},
	{Name: "state_hot", Tool: "hc-fill", Default: "200", Min: 0, Max: 10000, Unit: "%", Desc: "코어당 부하 이 값 이상인 기계와 state_cool 이하인 기계가 함께 있으면 눈에 띄는 것(0이면 끔)"},
	{Name: "state_cool", Tool: "hc-fill", Default: "50", Min: 0, Max: 10000, Unit: "%", Desc: "치우침의 한가한 쪽 코어당 부하"},
	{Name: "nudge_every", Tool: "hc-fill", Default: "10", Min: 0, Max: 1440, Unit: "분", Desc: "hc-fill nudge가 한 레인에 다시 보내기까지(0이면 막지 않음)"},
	{Name: "state_done", Tool: "hc-fill", Default: "60", Min: 0, Max: 10080, Unit: "분", Desc: "상태 한 장 기본 출력이 끝난 레인(착지·끝남)을 남기는 시간 — 넘으면 개수 한 줄(--all은 전체)"},
	// 지금 손댈 것(2026-11-24 ops-advise, docs/ops/advise.md). 풀 수 있는 대기·사슬의 문턱은 놀고 있음 알림과 같은 idle
	{Name: "advise_wait", Tool: "hc-fill", Default: "15", Min: 0, Max: 1440, Unit: "분", Desc: "감사 자리 없음·병합 줄 안 움직임(가장 오래된 제출 뒤 병합기 일지 줄 없음)이 이만큼 넘으면 손댈 것(0이면 끔)"},
	{Name: "advise_merge_slow", Tool: "hc-fill", Default: "30", Min: 0, Max: 1440, Unit: "분", Desc: "병합기의 지금 착지가 이만큼 넘으면 병합 줄이 느림(0이면 끔)"},
	{Name: "advise_every", Tool: "hc-fill", Default: "5", Min: 0, Max: 1440, Unit: "분", Desc: "바퀴가 손댈 것을 다시 보는 간격 — 새 항목이면 사건 hc-fill-advise 한 번(0이면 사건 끔 — 켜 둔 운영 에이전트는 0이면 바퀴마다 본다)"},
	{Name: "deploy_grace", Tool: "hc-fill", Default: "5", Min: 0, Max: 1440, Unit: "분", Desc: "마지막 착지 뒤 이만큼은 배포 밀림을 손댈 것으로 안 침(배포 도는 중·착지 뒤 명령 차례 남음도 안 침, 0이면 착지 시각은 안 봄)"},
	{Name: "onland_retry_wait", Tool: "hc-fill", Default: "10", Min: 0, Max: 120, Unit: "분", Desc: "착지 뒤 명령이 종료 75(일시 실패 — 다른 herdr-sync가 돎 등)면 이만큼까지 30초마다 다시 돌림(0이면 안 다시, 바로 막힘)"},
	// 운영 에이전트(2026-11-24 ops-agent, docs/ops/agent.md). 이름이 agent가 아닌 까닭: hc-fill 설정 agent(에이전트 CLI 종류)를 덮는다.
	// 운영 에이전트 실행 안에서는 ops_agent* 손잡이를 바꾸지 못한다(자기 권한 넓히기 막기 — hc-fill opsagent.go)
	{Name: "ops_agent", Tool: "hc-fill", Default: "끔", Enum: []string{"끔", "판단만", "조치"}, Desc: "운영 에이전트 — 끔 · 판단만(hc-fill 조치 명령을 일지에 「하려던 것」으로만) · 조치"},
	{Name: "ops_agent_max_hour", Tool: "hc-fill", Default: "2", Min: 1, Max: 60, Unit: "번", Desc: "운영 에이전트를 시간당 띄우는 상한(띄우기 실패도 셈)"},
	{Name: "ops_agent_timeout", Tool: "hc-fill", Default: "20", Min: 1, Max: 240, Unit: "분", Desc: "운영 에이전트 한 바퀴 상한(넘으면 끝난 것으로 보고 조치 명령을 거절)"},
	{Name: "ops_agent_tries", Tool: "hc-fill", Default: "2", Min: 1, Max: 20, Unit: "번", Desc: "같은 손댈 것으로 띄우는 상한(떠서 시간 넘김·결과 없이 끝남) — 넘으면 「사람이 정한다」 사건 한 번. 못 띄움이 이만큼 이어지면 운영 사건 한 번"},
	{Name: "ops_agent_close", Tool: "hc-fill", Default: "1", Min: 0, Max: 1, Desc: "끝난 운영 에이전트 실행의 터미널을 그 주소로 닫는다(0이면 안 닫음 — 같은 이름으로 다시 띄우면 실패한다)"},
	{Name: "ops_agent_scope", Tool: "hc-fill", Default: "-", List: true, Unit: "프로젝트", Desc: "운영 에이전트가 볼 프로젝트(쉼표, - = 전부 — 프로젝트를 모르는 항목은 늘 본다)"},
	{Name: "seats@", Per: "기계", Tool: "hc-fill", Min: 0, Max: 99, Unit: "자리", Desc: "그 기계 자리 상한(설정 machine 줄 상한을 덮음 — hc-fill machine <기계> --seats)"},
	{Name: "pause@", Per: "기계", Tool: "hc-fill", Enum: []string{"1"}, Desc: "그 기계에 새 레인을 안 띄움(떠 있는 레인·그 레인의 감사는 그대로 — hc-fill machine <기계> --pause)"},
	// knobs-migrate(2026-11-24): 고정으로 박혔던 hc-fill 설정 키(설정 줄보다 앞선다 — conf가 먼저 본다). 기본은 지금 코드의 값 그대로
	{Name: "gc", Tool: "hc-fill", Default: "60", Min: 0, Max: 1440, Unit: "분", Desc: "끝난 레인 정리 간격(0이면 끔)"},
	{Name: "idle_empty", Tool: "hc-fill", Default: "0", Min: 0, Max: 1440, Unit: "분", Desc: "띄울 것 없는 빈 자리 묶음 알림 간격(0이면 안 보냄)"},
	{Name: "reland_max", Tool: "hc-fill", Default: "2", Min: 0, Max: 10, Unit: "번", Desc: "병합기 충돌 반송 뒤 자동 다시 맞춤 횟수(0이면 끔)"},
	{Name: "reland_wait", Tool: "hc-fill", Default: "30", Min: 0, Max: 1440, Unit: "분", Desc: "다시 맞춤 전에 원래 레인을 기다리는 시간"},
	{Name: "audit_max", Tool: "hc-fill", Default: "2", Min: 0, Max: 10, Unit: "번", Desc: "감사 지적 뒤 고침 바퀴(넘으면 주인 지휘에 막힘)"},
	{Name: "audit_close", Tool: "hc-fill", Default: "5", Min: 0, Max: 100, Unit: "바퀴", Desc: "판정을 넘긴 감사 터미널 닫기 시도 상한(0이면 끔)"},
	{Name: "audit_seat_rounds", Tool: "hc-fill", Default: "5", Min: 1, Max: 100, Unit: "바퀴", Desc: "감사 몫 자리가 이만큼 바퀴 넘게 안 나면 주인 지휘에 막힘"},
	{Name: "lows_hour", Tool: "hc-fill", Default: "3", Min: 0, Max: 100, Unit: "개", Desc: "낮음 후속 레인 시간당 상한"},
	{Name: "lows_min", Tool: "hc-fill", Default: "1", Min: 1, Max: 100, Unit: "개", Desc: "낮음 후속 레인을 넣을 최소 낮음 수"},
	// 병합기 살아 있음(merger-alive — 옛 표식 파일 <hc-fill 상태 폴더>/<이름>은 손잡이가 없을 때 그대로 먹는다)
	{Name: "merger_dead", Tool: "hc-fill", Default: "5", Min: 0, Max: 1440, Unit: "분", Desc: "제출이 기다리는데 병합기 터미널이 이만큼 없으면 주인 지휘에 막힘(0이면 끔)"},
	{Name: "merger_unknown", Tool: "hc-fill", Default: "30", Min: 0, Max: 1440, Unit: "분", Desc: "병합기 살아 있음 모름이 이만큼 이어지면 한 번(0이면 끔)"},
	{Name: "merger_restart", Tool: "hc-fill", Default: "0", Enum: []string{"0", "1"}, Desc: "병합기 터미널이 없다고 알릴 때 스스로 한 번 다시 띄움(1 켬)"},
	// 병합기(병합기 기계 — 바퀴의 ls-remote에 얹은 원격 참조로 바퀴마다 읽음). 우선순위: 손잡이 > 일지 폴더 파일(옛 길) > 환경 변수(·--tick) > 기본
	{Name: "merger_batch", Tool: "hc-merger", Default: "3", Min: 1, Max: 20, Unit: "개", Remote: true, Desc: "착지 한 번에 묶는 제출 수 상한(HC_LANE_MERGER_BATCH)"},
	{Name: "merger_tick", Tool: "hc-merger", Default: "10", Min: 1, Max: 3600, Unit: "초", Remote: true, Desc: "빈 바퀴 쉼(--tick)"},
	{Name: "merger_spec_start", Tool: "hc-merger", Default: "300", Min: 0, Max: 86400, Unit: "초", Remote: true, Desc: "추측 일꾼이 떴다는 표식을 기다리는 상한(HC_LANE_SPEC_START)"},
	{Name: "merger_spec_timeout", Tool: "hc-merger", Default: "3600", Min: 60, Max: 86400, Unit: "초", Remote: true, Desc: "추측 검증 한 번의 상한(HC_LANE_SPEC_TIMEOUT)"},
	{Name: "merger_front_wait", Tool: "hc-merger", Default: "600", Min: 60, Max: 86400, Unit: "초", Remote: true, Desc: "맨 앞 추측 기다림 상한(HC_LANE_SPEC_FRONT_WAIT)"},
	{Name: "merger_pre_max", Tool: "hc-merger", Default: "2700", Min: 60, Max: 86400, Unit: "초", Remote: true, Desc: "미리 줄 표식 수명(HC_LANE_PRE_MAX)"},
	{Name: "merger_pre_rebuild", Tool: "hc-merger", Default: "1", Min: 0, Max: 10, Unit: "번", Remote: true, Desc: "미리 줄 다시 짓기 횟수(HC_LANE_PRE_REBUILD)"},
	{Name: "merger_front_dispatch", Tool: "hc-merger", Default: "1", Enum: []string{"0", "1"}, Remote: true, Desc: "맨 앞 무리도 검증 기계에 맡김(0이면 병합기가 직접, HC_LANE_FRONT_DISPATCH)"},
	{Name: "merger_front_local", Tool: "hc-merger", Default: "1", Enum: []string{"0", "1"}, Remote: true, Desc: "맡길 수 없을 때 병합기 기계가 직접 검증(0이면 기다림, HC_LANE_FRONT_LOCAL)"},
	{Name: "merger_idle_load", Tool: "hc-merger", Default: "50", Min: 1, Max: 10000, Unit: "%", Remote: true, Desc: "검증 기계 한가함 = 1분 부하가 코어 수의 이만큼 밑(HC_LANE_SPEC_IDLE_LOAD)"},
	{Name: "merger_busy_wait", Tool: "hc-merger", Default: "30", Min: 0, Max: 3600, Unit: "초", Remote: true, Desc: "검증 일꾼이 한가해지길 기다리는 상한, 넘으면 바쁨(HC_LANE_SPEC_BUSY_WAIT)"},
	{Name: "merger_busy_max", Tool: "hc-merger", Default: "2", Min: 0, Max: 20, Unit: "번", Remote: true, Desc: "같은 맨 앞의 바쁨이 이만큼 넘으면 다시 고르지 않음(HC_LANE_SPEC_BUSY_MAX)"},
	{Name: "merger_conflict", Tool: "hc-merger", Default: "agent", Enum: []string{"agent", "event", "reject"}, Remote: true, Desc: "rebase 충돌을 병합 쪽에서 풂·지휘에 사건·반송(HC_LANE_MERGER_CONFLICT)"},
	{Name: "merger_resolve_max", Tool: "hc-merger", Default: "3600", Min: 60, Max: 86400, Unit: "초", Remote: true, Desc: "충돌 풀이·풀이 감사 한 단계의 상한(HC_LANE_MERGER_RESOLVE_MAX)"},
	{Name: "merger_resolve_stuck", Tool: "hc-merger", Default: "3", Min: 0, Max: 1440, Unit: "분", Remote: true, Desc: "풀이·풀이 감사 에이전트가 blocked·error로 이만큼 머물면 운영 사건 한 번(0이면 끔, HC_LANE_MERGER_RESOLVE_STUCK)"},
	// 정리 도구(모든 기계 — 지휘 기계는 저장 파일, 다른 기계는 개인 설정 sweep.knobs_repo 클론의 원격 참조로 읽음)
	{Name: "sweep_r1", Tool: "hc-sweep", Default: "30", Min: 1, Max: 10080, Unit: "분", Remote: true, Desc: "R1: 턴 0인 채 띄운 지 이만큼이면 닫음"},
	{Name: "sweep_r2", Tool: "hc-sweep", Default: "30", Min: 0, Max: 10080, Unit: "분", Remote: true, Desc: "R2: worker가 쉰 지 이만큼이면 닫음(제출했으면 0)"},
	{Name: "sweep_r3", Tool: "hc-sweep", Default: "360", Min: 1, Max: 43200, Unit: "분", Remote: true, Desc: "R3: 이만큼 쉬면 닫음"},
	{Name: "sweep_resident_grace", Tool: "hc-sweep", Default: "5", Min: 1, Max: 1440, Unit: "분", Remote: true, Desc: "새 판이 깔리고(상주가 뜨고) 이만큼 지나도 옛 판·판 모름인 상주에 운영 사건 hc-stale-resident"},
}

// Find: 표에서 이름으로. 묶음 손잡이(「접두@값」)는 Name을 그 저장 이름으로 바꾼 사본을 돌려준다
func Find(name string) (Def, bool) {
	for _, d := range Defs {
		if d.Name == name && d.Per == "" {
			return d, true
		}
	}
	if i := strings.IndexByte(name, '@'); i > 0 && i < len(name)-1 && !strings.ContainsAny(name[i+1:], "@\t\n ") {
		for _, d := range Defs {
			if d.Per != "" && d.Name == name[:i+1] {
				d.Name = name
				return d, true
			}
		}
	}
	return Def{}, false
}

// Of: 묶음 손잡이 저장 이름의 값 부분(seats@x → x). 낱개면 빈 값
func (d Def) Of() string {
	if d.Per == "" {
		return ""
	}
	return d.Name[strings.IndexByte(d.Name, '@')+1:]
}

// Range: 허용 범위 글
func (d Def) Range() string {
	if len(d.Enum) > 0 {
		return strings.Join(d.Enum, "|")
	}
	if d.List {
		return "-|이름,이름…"
	}
	return fmt.Sprintf("%d~%d", d.Min, d.Max)
}

// Check: 값이 허용 범위인가
func (d Def) Check(v string) error {
	if d.List {
		if v == "-" {
			return nil
		}
		for _, n := range strings.Split(v, ",") {
			if n == "" || n == "-" || strings.Trim(n, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
				return fmt.Errorf("%s 값은 「-」 또는 쉼표로 나눈 이름(a-zA-Z0-9_.-)이다: %q", d.Name, v)
			}
		}
		return nil
	}
	if len(d.Enum) > 0 {
		for _, e := range d.Enum {
			if v == e {
				return nil
			}
		}
		return fmt.Errorf("%s 값은 %s 가운데 하나다: %q", d.Name, d.Range(), v)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < d.Min || n > d.Max || strconv.FormatInt(n, 10) != v {
		return fmt.Errorf("%s 값은 %s 정수다(%s): %q", d.Name, d.Range(), d.Unit, v)
	}
	return nil
}

// Suggest: 모르는 이름에 가까운 등록 이름(편집 거리 3 이하 또는 서로 포함), 가까운 차례로 셋까지
func Suggest(name string) []string {
	type c struct {
		n string
		d int
	}
	var cs []c
	for _, d := range Defs {
		e := dist(name, d.Name)
		if name != "" && (strings.Contains(d.Name, name) || strings.Contains(name, d.Name)) {
			e = -1 // 서로 포함이 먼저
		}
		if e <= 3 {
			cs = append(cs, c{d.Name, e})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	var o []string
	for i := 0; i < len(cs) && i < 3; i++ {
		o = append(o, cs[i].n)
	}
	return o
}

func dist(a, b string) int {
	x, y := []rune(a), []rune(b)
	prev := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		cur := make([]int, len(y)+1)
		cur[0] = i
		for j := 1; j <= len(y); j++ {
			k := 1
			if x[i-1] == y[j-1] {
				k = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+k)
		}
		prev = cur
	}
	return prev[len(y)]
}

// Entry: 저장된 손잡이 하나. Until 0 = 계속
type Entry struct {
	Name, Value, By, Why string
	At, Until            int64
	Pin                  bool
}

// Live: 기한 안이고 표에 있고 값이 범위 안인가
func (e Entry) Live(now time.Time) bool {
	d, ok := Find(e.Name)
	return ok && (e.Until == 0 || now.Unix() < e.Until) && d.Check(e.Value) == nil
}

// Path: 지휘 기계의 저장 파일(git 밖)
func Path(home string) string { return home + "/.local/state/herdr-ops/knobs.tsv" }

const head = "# herdr-tools 손잡이(hc-fill knob) — 이름\t값\t누가\t왜\t언제(초)\t언제까지(초|-)\t고정(1|0)\n"

// Load: 저장 파일을 읽는다. 없으면 빈 것. 꼴이 틀린 줄이 하나라도 있으면 오류(부르는 쪽은 손잡이 없이 돌고 한 줄 알린다)
func Load(path string) (map[string]Entry, error) {
	b, _, err := atomic.Read(path)
	if err != nil {
		return nil, err
	}
	return parse(string(b), path)
}

func parse(s, path string) (map[string]Entry, error) {
	es := map[string]Entry{}
	for i, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		bad := len(f) != 7 || f[0] == "" || (f[6] != "0" && f[6] != "1")
		var at, until int64
		if !bad {
			var e1, e2 error
			at, e1 = strconv.ParseInt(f[4], 10, 64)
			if f[5] != "-" {
				until, e2 = strconv.ParseInt(f[5], 10, 64)
			}
			bad = e1 != nil || e2 != nil
		}
		if bad {
			return nil, fmt.Errorf("손잡이 파일 %s %d줄 꼴이 틀렸다", path, i+1)
		}
		es[f[0]] = Entry{f[0], f[1], f[2], f[3], at, until, f[6] == "1"}
	}
	return es, nil
}

func format(es map[string]Entry, now time.Time) []byte {
	var ns []string
	for n, e := range es {
		if e.Live(now) {
			ns = append(ns, n)
		}
	}
	sort.Strings(ns)
	var b strings.Builder
	b.WriteString(head)
	for _, n := range ns {
		e := es[n]
		u, p := "-", "0"
		if e.Until != 0 {
			u = strconv.FormatInt(e.Until, 10)
		}
		if e.Pin {
			p = "1"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", e.Name, e.Value, clean(e.By), clean(e.Why), e.At, u, p)
	}
	return []byte(b.String())
}

func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "-"
	}
	return s
}

// ErrPinned: 고정된 값을 --force 없이 바꾸려 함
var ErrPinned = errors.New("고정된 값")

// update: 잠금 안에서 읽고 고쳐 원자 쓰기. 깨진 파일은 고치지 않는다(사람이 보고 지우게)
func update(path string, now time.Time, f func(es map[string]Entry) error) error {
	dir := path[:strings.LastIndex(path, "/")]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lk, err := lockx.Lock(path + ".lock")
	if err != nil {
		return err
	}
	defer lk.Close()
	return atomic.Update(path, 0o644, func(old []byte, _ bool) ([]byte, error) {
		es, err := parse(string(old), path)
		if err != nil {
			return nil, err
		}
		if err := f(es); err != nil {
			return nil, err
		}
		return format(es, now), nil
	})
}

// Set: 값을 둔다(이름·값은 부르는 쪽이 Find·Check로 먼저 본다). 살아 있는 고정 값은 force 없이 ErrPinned
func Set(path string, e Entry, force bool, now time.Time) error {
	return update(path, now, func(es map[string]Entry) error {
		if o, ok := es[e.Name]; ok && o.Pin && o.Live(now) && !force {
			return fmt.Errorf("%w: %s(누가 %s, 왜 %s) — 바꾸려면 --force", ErrPinned, o.Name, o.By, o.Why)
		}
		es[e.Name] = e
		return nil
	})
}

// Unset: 값을 지운다(설정 값으로 돌아감). 없었으면 had=false
func Unset(path, name string, force bool, now time.Time) (had bool, err error) {
	err = update(path, now, func(es map[string]Entry) error {
		o, ok := es[name]
		if ok && o.Pin && o.Live(now) && !force {
			return fmt.Errorf("%w: %s(누가 %s, 왜 %s) — 지우려면 --force", ErrPinned, o.Name, o.By, o.Why)
		}
		had = ok && o.Live(now)
		delete(es, name)
		return nil
	})
	return had, err
}

// Source: 지금 먹는 값의 출처
type Source string

const (
	FromDefault Source = "기본"
	FromConf    Source = "설정"
	FromKnob    Source = "손잡이"
)

// Effective: 손잡이 > 설정(빈 값이 아니면) > 기본. 손잡이면 그 줄도
func Effective(d Def, es map[string]Entry, confVal string, now time.Time) (string, Source, *Entry) {
	if e, ok := es[d.Name]; ok && e.Live(now) {
		return e.Value, FromKnob, &e
	}
	if confVal != "" {
		return confVal, FromConf, nil
	}
	return d.Default, FromDefault, nil
}
