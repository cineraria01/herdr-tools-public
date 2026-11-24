// digest.go: 지휘 알림 묶기(coord-digest, 2026-11-24) — watch가 지휘에게 보내는 사건을 「즉시」와 「묶음」으로 가른다.
// 지휘 세션이 하루 사건 200여 건을 하나씩 받아(대부분 손댈 것 없는 완료 → 판정 → 제출 → 착지) 사용자 대화와 서로 막혔다.
// 손잡이 coord_digest(분, 기본 0 = 지금과 똑같이 하나씩)가 0보다 크면 묶음 사건은 커서에 안 적고 모아 두었다가 그 주기마다 한 메시지로.
// 사건 원본(events.tsv)·커서 꼴은 그대로 — 「보냄」 표시만 늦춘다(watch가 다시 떠도 커서에 없는 사건은 다시 보이니 잃지 않는다)
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/knob"
)

// digestMax: 모인 것이 이만큼을 넘으면 주기 전이라도 보낸다
const digestMax = 20

// digestState: 이 watch의 묶음 상태(메모리 — 다시 뜨면 처음부터 세어 늦을 뿐 놓치지 않는다)
type digestState struct {
	seen map[string]time.Time // 묶음 사건 열쇠 → 이 watch가 처음 본 때(보내면 지움). 주기는 지금 모인 것 가운데 가장 이른 때부터
	kbad bool                 // 손잡이 파일을 못 읽었다고 한 번 알렸음
}

// sent: 보낸 묶음의 열쇠를 지운다
func (d *digestState) sent(dig []string) {
	for _, l := range dig {
		delete(d.seen, lineKey(l))
	}
}

// digestable: 묶음 갈래인가 — 가르는 표는 여기 한 곳(사건 종류가 늘면 이 표만 고친다). 모르는 것은 즉시(놓치지 않기)
//
//	즉시: 막힘 · 멈춤 blocked·error·뜻밖의 종료(정리됨이 아닌 것)·연결 안 됨 · 「사람이 정한다」 · 실패·반송·못 … 이 든 중간 ·
//	      운영 사건(아래 둘 말고) · 주차·모르는 상태
//	묶음: 완료 · 중간 · 판정(감사 통과·지적) · 제출 · 착지 · 멈춤 done·idle·뜻밖의 종료(정리됨) ·
//	      운영 중간 가운데 착지 뒤 명령 성공(on-land-<이름>)·운영 에이전트가 맡은 손댈 것(hc-fill-advise, 키 hf:advise:agent:)
func digestable(x events.Event) bool {
	if strings.Contains(x.Msg, "사람이 정한다") {
		return false
	}
	if x.KindOf() == events.KindOps {
		if x.State != "중간" {
			return false
		}
		return strings.HasPrefix(x.Lane, events.OpsOnLand) || x.Lane == events.OpsAdvise && strings.HasPrefix(x.Key, events.AdviseAgentKey)
	}
	switch x.State {
	case "완료", "판정", "제출", "착지":
		return true
	case "중간":
		for _, w := range []string{"실패", "반송", "못 ", "손으로"} {
			if strings.Contains(x.Msg, w) {
				return false
			}
		}
		return true
	case "멈춤":
		for _, p := range []string{"done ", "idle ", "뜻밖의 종료(정리됨)"} {
			if strings.HasPrefix(x.Msg, p) {
				return true
			}
		}
	}
	return false
}

// digestPeriod: 손잡이 coord_digest(분). 시험은 HC_LANE_COORD_DIGEST_SEC(초)로 짧게. 못 읽으면 0(지금과 같이 — 한 번 알림)
func (e *ev) digestPeriod() time.Duration {
	if s := os.Getenv("HC_LANE_COORD_DIGEST_SEC"); s != "" && digits(s) {
		return time.Duration(atoi(s)) * time.Second
	}
	d, _ := knob.Find("coord_digest")
	now := time.Now()
	es, err := knob.Load(knob.Path(os.Getenv("HOME")))
	if err != nil {
		if !e.dg.kbad {
			e.dg.kbad = true
			fmt.Fprintf(os.Stderr, "hc-lane watch: 손잡이를 못 읽어 묶지 않고 하나씩 보낸다: %v\n", err)
		}
		return 0
	}
	v := d.Default
	if k, ok := es[d.Name]; ok && k.Live(now) {
		v = k.Value
	}
	n, _ := strconv.Atoi(v)
	return time.Duration(n) * time.Minute
}

// digest: 보낼 줄을 묶음(dig)·즉시(imm)로 가른다. hold면 이번 바퀴는 아무것도 안 보낸다(모아 둠 — 커서에 안 적음).
// 즉시가 있거나, 모인 것이 digestMax를 넘거나, 주기가 차면 모인 묶음을 즉시 앞에 붙여 같은 차례에 보낸다(since = 모인 것 가운데
// 가장 이른 것을 처음 본 뒤). 손잡이 0이면 모두 즉시
func (e *ev) digest(lines []string) (dig, imm []string, since time.Duration, hold bool) {
	p := e.digestPeriod()
	if p <= 0 {
		e.dg.seen = nil
		return nil, lines, 0, false
	}
	for _, l := range lines {
		if digestable(events.Cols(l)) {
			dig = append(dig, l)
		} else {
			imm = append(imm, l)
		}
	}
	now := time.Now()
	if e.dg.seen == nil {
		e.dg.seen = map[string]time.Time{}
	}
	for k, t := range e.dg.seen { // 남이 받아(wait 등) 다시 안 보이는 열쇠 — 주기 상한의 두 배 넘게 묵으면 걷는다
		if now.Sub(t) > 240*time.Minute {
			delete(e.dg.seen, k)
		}
	}
	oldest := now
	for _, l := range dig {
		k := lineKey(l)
		if t, ok := e.dg.seen[k]; ok {
			if t.Before(oldest) {
				oldest = t
			}
		} else {
			e.dg.seen[k] = now
		}
	}
	since = now.Sub(oldest)
	if len(dig) > 0 && len(imm) == 0 && len(dig) <= digestMax && since < p {
		return nil, nil, since, true
	}
	return dig, imm, since, false
}

// digestText: 묶음 한 메시지 — 머리 「hc-lane 묶음 — 최근 <n>분 <k>건」, 레인마다 한 줄로 접고(시간 순으로 「 → 」), 레인에 안 묶이는
// 운영 사건은 끝에 한 줄씩, 끝 줄 「→ hc-fill status」. watch는 제어 문자(줄바꿈 포함)를 지우니 줄은 「 ‖ 」로 잇는다
func digestText(dig []string, since time.Duration) string {
	n := int((since + time.Minute - 1) / time.Minute)
	if n < 1 {
		n = 1
	}
	var order []string
	chain := map[string][]string{}
	var ops []string
	for _, l := range dig {
		x := events.Cols(l)
		if x.KindOf() == events.KindOps {
			ops = append(ops, wpart(l))
			continue
		}
		// 감사 레인(hc-fill이 띄우는 audit-<레인>)의 판정은 그 레인 줄에 「감사 …」로 접는다
		lane, it := x.Lane, x.State
		if b, ok := strings.CutPrefix(lane, "audit-"); ok && b != "" {
			lane, it = b, "감사 "+x.State
		}
		if x.SHA != "-" && x.SHA != "" {
			it += " " + short8(x.SHA)
		}
		if m := strings.TrimSpace(x.Msg); m != "" && m != "-" {
			it += " " + digestCut(m, 80)
		}
		if _, ok := chain[lane]; !ok {
			order = append(order, lane)
		}
		chain[lane] = append(chain[lane], it)
	}
	parts := []string{fmt.Sprintf("hc-lane 묶음 — 최근 %d분 %d건", n, len(dig))}
	for _, lane := range order {
		parts = append(parts, lane+": "+strings.Join(chain[lane], " → "))
	}
	parts = append(parts, ops...)
	return strings.Join(append(parts, "→ hc-fill status"), " ‖ ")
}

// digestCut: 앞 n글자(룬), 잘랐으면 「…」
func digestCut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
