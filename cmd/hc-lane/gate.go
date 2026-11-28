// gate.go: 지휘에게 가는 알림의 한 문(업계 이름 alert manager, ht-alert-gate 2026-11-28) — watch가 보낼 사건마다 judge 한 번으로
// 받는 쪽 한 명 · 갈래(판단 필요/기다렸다 판단/소식/밀어 넣지 않음) · 까닭을 낸다. 재료는 사건 창·손잡이·지휘 살아 있음뿐이라 어느 지휘의
// watch가 불러도 같은 사건에는 같은 답이다(커서는 「이 지휘에게 이미 보냈나」에만 — gatesend.go). 갈래 표는 gateTable 한 곳이고
// docs/hc-lane/watch.md 「한 문」의 표와 같다. 받는 쪽 고르기는 gatewho.go
package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
)

// gbranch: 갈래
type gbranch int

const (
	brNow  gbranch = iota // 판단 필요: 바로, 받는 쪽 한 명(놓침 막기 있음)
	brHold                // 기다렸다 판단: 문턱 동안 보류, 그 사이 풀리면 안 보냄, 넘기면 판단 필요로 한 번
	brNews                // 소식: 묶음으로, 받는 쪽 한 명
	brMute                // 밀어 넣지 않음: 지휘 화면에 안 감(events·--explain으로 본다)
)

var brName = [...]string{"판단 필요", "기다렸다 판단", "소식", "밀어 넣지 않음"}

func (b gbranch) String() string { return brName[b] }

// staleAge: 이보다 묵은 사건은 보내지 않는다(새 커서 바닥과 같은 2시간) — 예외는 받는 쪽이 이 지휘인 안 풀린 판단 필요(시각을 붙여)
const staleAge = 2 * time.Hour

// staleMax: 묵은 예외의 나이 상한 — 안 풀린 판단 필요라도 이보다 묵으면 보내지 않는다(ht-alert-sources: 7일 창의 129건이 커서에
// 없으면 언제든 다시 나갈 수 있었다. 하루 = 밤새 끊겼던 기계의 막힘도 다음 날 받는 폭 — 새 손잡이 없이 표의 상수)
const staleMax = 24 * time.Hour

// upSuffix: 운영 풀림 열쇠 = 그 막힘 열쇠 + 이것(격리·닿지 않음이 먼저 쓰던 꼴) — 내는 쪽이 그 상태가 끝날 때 한 번 적는다
const upSuffix = ":up"

// kindRow: 갈래 표 한 줄 — is가 맞는 첫 줄이 그 사건의 종류. done(x, y)는 x 뒤 사건 y가 x를 풀었나(nil = 풀림 규칙 없음)
type kindRow struct {
	name string
	br   gbranch
	is   func(x events.Event) bool
	done func(x, y events.Event) bool
}

func isOps(x events.Event) bool { return x.KindOf() == events.KindOps }

func opsIn(st string, lanes ...string) func(events.Event) bool {
	return func(x events.Event) bool {
		if !isOps(x) || x.State != st {
			return false
		}
		for _, p := range lanes {
			if x.Lane == p || strings.HasSuffix(p, "-") && strings.HasPrefix(x.Lane, p) {
				return true
			}
		}
		return false
	}
}

func stopIs(ps ...string) func(events.Event) bool {
	return func(x events.Event) bool {
		if x.State != "멈춤" {
			return false
		}
		for _, p := range ps {
			if strings.HasPrefix(x.Msg, p) {
				return true
			}
		}
		return false
	}
}

func agentEnd(x events.Event) bool {
	return opsIn("중간", events.OpsAgent)(x) && strings.HasPrefix(x.Msg, "운영 에이전트 끝:")
}

// failWord: 중간 글에 든 실패 낱말(옛 digestable 즉시 갈래 그대로)
func failWord(m string) bool {
	for _, w := range []string{"실패", "반송", "못 ", "손으로"} {
		if strings.Contains(m, w) {
			return true
		}
	}
	return false
}

// laneMoved: y가 레인 x의 일이 다시 움직였다는 사건 — 같은 레인 이름(운영 사건 아님)의 끝 보고·넘김, 실패 낱말 없는 중간.
// 레인 이름이 도구 이름인 사건(운영)은 여기로 풀리지 않는다 — opsKeyDone
func laneMoved(x, y events.Event) bool {
	if y.Lane != x.Lane || isOps(y) {
		return false
	}
	switch y.State {
	case "완료", "판정", "제출", "착지", "주차":
		return true
	case "중간":
		return !failWord(y.Msg) && !strings.HasPrefix(y.Msg, cleanMemo) // clean 정리 메모는 일이 움직인 것이 아니다(감사 atp7hcw M1)
	}
	return false
}

// sameRunMoved: 멈춤(blocked·error·쉼)이 그 터미널의 뒤 보고로 풀림
func sameRunMoved(x, y events.Event) bool {
	return lastSeg(y.Term) == lastSeg(x.Term) && y.State != "멈춤" && y.State != "정리됨" && laneMoved(x, y)
}

// opsKeyDone: 운영 막힘은 같은 이름·같은 키(키가 있을 때만) 또는 그 키 + upSuffix의 막힘 아닌 뒤 사건으로만 풀림 — 병합기의 다른
// 사건이 그 막힘을 풀지 않는다
func opsKeyDone(x, y events.Event) bool {
	return x.Key != "" && x.Key != "-" && y.Lane == x.Lane && (y.Key == x.Key || y.Key == x.Key+upSuffix) && y.State != "막힘"
}

var reFront = regexp.MustCompile(`맨 앞 (\S+) 검증`)

// frontDone: 병합기 「맨 앞 못 맡김」은 그 맨 앞 레인의 착지·반송(막힘), 또는 같은 병합기의 맨 앞이 다른 제출로 바뀜으로 풀림
func frontDone(x, y events.Event) bool {
	if m := reFront.FindStringSubmatch(x.Msg); m != nil && y.Lane == m[1] && (y.State == "착지" || y.State == "막힘") {
		return true
	}
	return y.Lane == x.Lane && lastSeg(y.Term) == lastSeg(x.Term) && strings.HasPrefix(y.Key, "hc-merger-front:") && y.Key != x.Key
}

// gateTable: 갈래 표 — 위에서부터 첫 줄. 모르는 종류는 판단 필요(안 가는 쪽이 위험하다 — 밀어 넣지 않는 것은 표에 적힌 것만)
var gateTable = []kindRow{
	{"사람이 정한다", brNow, func(x events.Event) bool { return strings.Contains(x.Msg, "사람이 정한다") }, nil},
	{"손댈 것 참고 사본", brMute, func(x events.Event) bool {
		return x.Lane == events.OpsAdvise && strings.HasPrefix(x.Key, events.AdviseRefKey)
	}, nil},
	{"운영 에이전트 몫 손댈 것", brNews, func(x events.Event) bool {
		return x.Lane == events.OpsAdvise && strings.HasPrefix(x.Key, events.AdviseAgentKey)
	}, nil},
	{"착지 뒤 명령 성공", brMute, opsIn("중간", events.OpsOnLand), nil},
	// 운영 풀림: 내는 쪽이 막힘이 끝날 때 그 열쇠 + upSuffix로 한 번 적는 것(병합기 다시 돎·쉼 끝·격리 풀림·다시 닿음·운영 에이전트 다시 뜸)
	{"운영 풀림", brNews, func(x events.Event) bool {
		return isOps(x) && x.State != "막힘" && strings.HasSuffix(x.Key, upSuffix)
	}, nil},
	// 병합기 풀이 진행 중간(「충돌 — 병합 쪽에서 푸는 중」): 할 일 없는 진행. 풀이가 못 풂으로 끝나면 레인 반송 막힘으로 따로 온다
	{"병합기 풀이 진행", brNews, func(x events.Event) bool {
		return opsIn("중간", events.OpsMerger)(x) && strings.HasPrefix(x.Key, "hc-merger-resolve:") && !failWord(x.Msg)
	}, nil},
	// 운영 에이전트 끝: 결정은 따로 「사람이 정한다」 막힘으로 온다. 글 끝의 「(끝남, <방식>, …)」은 1000바이트에서 잘려 빠질 때가 많아
	// (되감기 55건) 머리로 가른다 — 결과 없이 끝남·시간 넘김은 판단 필요(운영 알림), 조치 방식은 소식, 그 밖(판단만·잘림)은 밀어 넣지 않음
	{"운영 에이전트 끝(조치)", brNews, func(x events.Event) bool { return agentEnd(x) && strings.Contains(x.Msg, "(끝남, 조치") }, nil},
	{"운영 에이전트 끝(판단만)", brMute, func(x events.Event) bool {
		return agentEnd(x) && !strings.Contains(x.Msg, "결과 없음") && !strings.HasPrefix(x.Msg, "운영 에이전트 끝: 시간 넘김")
	}, nil},
	{"놀고 있음", brNews, opsIn("중간", events.OpsFillIdle), nil},
	{"병합기 맨 앞 못 맡김", brHold, func(x events.Event) bool {
		return opsIn("막힘", events.OpsMerger)(x) && strings.HasPrefix(x.Key, "hc-merger-front:")
	}, frontDone},
	{"운영 돌아옴·진행", brNews, opsIn("중간", events.OpsCordon, events.OpsUnreach, events.OpsMergerP, events.OpsEgoIdle), nil},
	{"운영 막힘", brNow, func(x events.Event) bool { return isOps(x) && x.State == "막힘" }, opsKeyDone},
	{"운영 알림", brNow, isOps, nil},
	{"충돌 반송", brHold, func(x events.Event) bool {
		return x.State == "막힘" && strings.HasPrefix(x.Msg, "병합기: 착지 실패") && strings.Contains(x.Msg, "충돌")
	}, laneMoved},
	{"레인 막힘", brNow, func(x events.Event) bool { return x.State == "막힘" }, laneMoved},
	{"멈춤 blocked·error", brNow, stopIs("blocked ", "error "), sameRunMoved},
	{"연결 안 됨", brNow, stopIs("연결 안 됨"), nil},
	{"뜻밖의 종료", brNow, stopIs("뜻밖의 종료", "보고 없이 쉬다 닫힘"), laneMoved},
	{"쉼", brNews, stopIs("done ", "idle "), nil},
	{"실패가 든 중간", brNow, func(x events.Event) bool { return x.State == "중간" && failWord(x.Msg) }, laneMoved},
	{"소식", brNews, func(x events.Event) bool {
		switch x.State {
		case "완료", "판정", "제출", "착지", "주차", "중간":
			return true
		}
		return false
	}, nil},
}

var unknownKind = kindRow{"모르는 종류", brNow, nil, nil}

func classify(x events.Event) kindRow {
	for _, r := range gateTable {
		if r.is(x) {
			return r
		}
	}
	return unknownKind
}

// gverdict: 한 사건의 판정
type gverdict struct {
	kind     string
	br       gbranch   // 보류가 문턱을 넘기면 brNow
	at       time.Time // 사건 시각(지어낸 멈춤·못 읽음이면 이 watch가 처음 본 때)
	until    time.Time // 보류 끝
	open     bool      // 풀림 규칙이 있고 아직 안 풀림
	stale    bool      // staleAge보다 묵음
	to       string    // 받는 쪽 한 명(주소 끝 t…), 없으면 ""
	owner    string    // 받을 수 있는 주인(받는 쪽이 주인이면 to와 같음), 없으면 ""
	hadOwner bool      // 주인 후보(--to·11칸·레인 기록)가 하나라도 있었나 — 받을 수 있든 없든
	all      bool      // 받는 쪽을 확인 못 함 — 판단 필요는 살아 있는 모든 지휘 watch가 보낸다
	next     string    // 놓침 막기: to가 nextAt까지 안 받으면 이 지휘
	nextAt   time.Time
	why      string // 까닭 한 구절
	tag      string // 글 끝에 붙일 받는 쪽 한 구절(대표·모든 지휘)
}

// judge: 사건 x(이 바퀴 창 w.hist 안의 것)의 판정 — 순수(w의 재료만)
func (w *gworld) judge(x events.Event) gverdict {
	r := classify(x)
	v := gverdict{kind: r.name, br: r.br, at: w.timeOf(x)}
	if r.done != nil {
		if y, ok := w.doneBy(x, r.done); ok {
			v.why = "풀림(" + y.State + " " + clock(evTime(y)) + ")"
		} else {
			v.open = true
		}
	}
	switch {
	case r.br == brHold:
		v.until = v.at.Add(w.minutes("advise_wait"))
		switch {
		case !v.open:
			v.br = brMute
		case w.now.Before(v.until):
			v.why = "보류 ~" + clock(v.until) + " — 그 사이 풀리면 안 보냄"
		default:
			v.br, v.why = brNow, "보류 문턱 넘김("+w.knobv("advise_wait")+"분)"
		}
	case r.br == brNow && r.done != nil && !v.open:
		v.br = brMute // 보내기 전에 이미 풀림 — 푼 사건이 소식으로 간다
	case r.br == brMute:
		v.why = "표: " + r.name
	}
	v.stale = w.now.Sub(v.at) > staleAge
	if v.br != brMute && v.br != brHold {
		w.route(x, &v)
	}
	if v.stale && v.br != brMute && !(v.br == brNow && v.open) {
		v.why = strings.TrimPrefix(v.why+" · 묵음 "+mins(w.now.Sub(v.at)), " · ")
	}
	return v
}

// timeOf: 사건 시각 — 1칸을 못 읽거나 지어낸 멈춤(시각이 「지금」이라 바퀴마다 새로움)이면 이 watch가 그 열쇠를 처음 본 때
func (w *gworld) timeOf(x events.Event) time.Time {
	if x.State != "멈춤" || x.N >= 8 {
		if t := evTime(x); !t.IsZero() {
			return t
		}
	}
	if w.first != nil {
		return w.first(lineKey(x.Raw()))
	}
	return w.now
}

func evTime(x events.Event) time.Time {
	t, err := time.Parse(time.RFC3339, x.Time)
	if err != nil {
		return time.Time{}
	}
	return t
}

// doneBy: 창에서 x 뒤(시각이 같거나 늦고 x 자신이 아닌) 사건 가운데 done이 맞는 첫 것 — 풀림 규칙은 모두 같은 레인 이름(맨 앞 못 맡김은
// 그 맨 앞 레인도)의 사건만 보니 레인 색인(laneIdx)으로 그 레인들만 훑는다(바퀴마다 사건 수 × 창 줄 수가 되지 않게)
func (w *gworld) doneBy(x events.Event, done func(x, y events.Event) bool) (events.Event, bool) {
	t0 := evTime(x)
	lanes := []string{x.Lane}
	if m := reFront.FindStringSubmatch(x.Msg); m != nil && m[1] != x.Lane {
		lanes = append(lanes, m[1])
	}
	xi := -1 // 창에서 x의 줄(정리됨에서 지은 멈춤은 그 정리됨 줄) — 같은 초의 앞 줄은 뒤 사건이 아니다
	for _, i := range w.laneIdx()[x.Lane] {
		if y := w.hist[i]; y.Time == x.Time && lastSeg(y.Term) == lastSeg(x.Term) && (y.State == x.State && y.Msg == x.Msg || x.State == "멈춤" && y.State == "정리됨") {
			xi = i
		}
	}
	for _, ln := range lanes {
		for _, i := range w.laneIdx()[ln] {
			y := w.hist[i]
			switch {
			case i == xi || same(x, y):
				continue
			case y.Time == x.Time && !t0.IsZero():
				// 같은 초: 같은 기계 파일에서 x보다 뒤 줄일 때만(다른 기계면 차례를 모른다 — 뒤로 치지 않는다)
				if xi < 0 || i < xi || y.Machine != w.hist[xi].Machine {
					continue
				}
			case y.Time < x.Time && !t0.IsZero():
				continue
			}
			if done(x, y) {
				return y, true
			}
		}
	}
	return events.Event{}, false
}

// same: 같은 사건 줄(창에 두 번 든 것도) — 칸으로 견준다(Raw 이어 붙이기 없이)
func same(x, y events.Event) bool {
	return x.Time == y.Time && x.Lane == y.Lane && x.State == y.State && lastSeg(x.Term) == lastSeg(y.Term) && x.Msg == y.Msg
}

// laneIdx: 창(hist)의 레인 이름 → 줄 번호(시각 순 그대로), 바퀴에 한 번
func (w *gworld) laneIdx() map[string][]int {
	if w.byLane == nil {
		w.byLane = map[string][]int{}
		for i, y := range w.hist {
			w.byLane[y.Lane] = append(w.byLane[y.Lane], i)
		}
	}
	return w.byLane
}

// clock: 지휘에게 보일 시각(이 기계 시간대) — 같은 날이면 「15:04」, 아니면 「01-02 15:04」
func clock(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	l := t.Local()
	if l.Format("2006-01-02") == time.Now().Local().Format("2006-01-02") {
		return l.Format("15:04")
	}
	return l.Format("01-02 15:04")
}

func mins(d time.Duration) string { return strconv.FormatInt(int64(d/time.Minute), 10) + "분" }
