// digest.go: 지휘 알림 묶기(coord-digest, 2026-11-24) — watch가 지휘에게 보내는 사건을 「즉시」와 「묶음」으로 가른다.
// 지휘 세션이 하루 사건 200여 건을 하나씩 받아(대부분 손댈 것 없는 완료 → 판정 → 제출 → 착지) 사용자 대화와 서로 막혔다.
// 손잡이 coord_digest(분, 기본 0 = 지금과 똑같이 하나씩)가 0보다 크면 묶음 사건은 커서에 안 적고 모아 두었다가 그 주기마다 한 메시지로.
// 사건 원본(events.tsv)·커서 꼴은 그대로 — 「보냄」 표시만 늦춘다(watch가 다시 떠도 커서에 없는 사건은 다시 보이니 잃지 않는다)
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/knob"
)

// digestState: 이 watch의 묶음 상태(메모리 — 다시 뜨면 처음부터 세어 늦을 뿐 놓치지 않는다)
type digestState struct {
	seen  map[string]time.Time // 묶음 사건 열쇠 → 이 watch가 처음 본 때(보내면 지움). 주기는 지금 모인 것 가운데 가장 이른 때부터
	kbad  bool                 // 손잡이 파일을 못 읽었다고 한 번 알렸음
	cwarn bool                 // 손잡이 설정 파일의 거절한 줄을 한 번 알렸음
}

// sent: 보낸 묶음의 열쇠를 지운다
func (d *digestState) sent(dig []string) {
	for _, l := range dig {
		delete(d.seen, lineKey(l))
	}
}

// digestPeriod: 손잡이 coord_digest(분). 시험은 HC_LANE_COORD_DIGEST_SEC(초)로 짧게. 못 읽으면 0(지금과 같이 — 한 번 알림)
func (e *ev) digestPeriod() time.Duration {
	if s := os.Getenv("HC_LANE_COORD_DIGEST_SEC"); s != "" && digits(s) {
		return time.Duration(atoi(s)) * time.Second
	}
	v, err := e.knob("coord_digest")
	if err != nil {
		if !e.dg.kbad {
			e.dg.kbad = true
			fmt.Fprintf(os.Stderr, "hc-lane watch: 손잡이를 못 읽어 묶지 않고 하나씩 보낸다: %v\n", err)
		}
		return 0
	}
	n, _ := strconv.Atoi(v)
	return time.Duration(n) * time.Minute
}

// knob: 이 기계 손잡이 값(저장 파일 > 설정 파일 > 기본). 설정 파일의 거절한 줄은 이 watch에서 처음 한 번만 알린다
func (e *ev) knob(name string) (string, error) {
	var w io.Writer
	if !e.dg.cwarn {
		e.dg.cwarn, w = true, os.Stderr
	}
	return knob.Local(os.Getenv("HOME"), name, w, time.Now())
}

// digestMax: 모인 것이 손잡이 coord_digest_max(개)를 넘으면 주기 전이라도 보낸다(저장 파일이 깨졌으면 설정 파일·기본)
func (e *ev) digestMax() int {
	v, _ := e.knob("coord_digest_max")
	n, _ := strconv.Atoi(v)
	return n
}

// digest: 보낼 줄을 묶음(dig — 한 문의 갈래 소식)·즉시(imm)로 가른다. hold면 이번 바퀴는 아무것도 안 보낸다(모아 둠 — 커서에 안 적음).
// 즉시가 있거나, 모인 것이 digestMax를 넘거나, 주기가 차면(모인 것 가운데 가장 이른 것을 처음 본 뒤) 모인 묶음을 즉시 앞에 붙여 같은
// 차례에 보낸다. 손잡이 0이면 모두 즉시
func (e *ev) digest(lines []string) (dig, imm []string, hold bool) {
	p := e.digestPeriod()
	if p <= 0 {
		e.dg.seen = nil
		return nil, lines, false
	}
	for _, l := range lines {
		if v, ok := e.vd[lineKey(l)]; ok && v.br == brNews {
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
	if len(dig) > 0 && len(imm) == 0 && len(dig) <= e.digestMax() && now.Sub(oldest) < p {
		return nil, nil, true
	}
	return dig, imm, false
}

// digestText: 묶음 한 메시지 — 머리 「hc-lane 묶음 — <k>건(<첫 사건 시각>~<끝 사건 시각>)」(사건 시각, 이 기계 시간대), 레인마다 한 줄로
// 접고(시간 순으로 「 → 」), 레인에 안 묶이는 운영 사건은 끝에 한 줄씩, 끝 줄 「→ hc-fill status」. 끝 사건보다 30분 넘게 이른 항목은 앞에
// 자기 시각, 대표라서 받은 항목은 뒤에 그 한 구절(e.rtag). 항목은 상태·sha 앞, 자유 글은 뒤에서 자른다. 줄은 「 ‖ 」로 잇는다(제어 문자를 지우니)
func (e *ev) digestText(dig []string) string {
	var t0, t1 time.Time
	for _, l := range dig {
		if t := evTime(events.Cols(l)); !t.IsZero() {
			if t0.IsZero() || t.Before(t0) {
				t0 = t
			}
			if t.After(t1) {
				t1 = t
			}
		}
	}
	var order []string
	chain := map[string][]string{}
	var ops []string
	for _, l := range dig {
		x := events.Cols(l)
		pre, tag := "", e.rtag[lineKey(l)]
		if t := evTime(x); !t.IsZero() && t1.Sub(t) > 30*time.Minute {
			pre = "[" + clock(t) + "] "
		}
		if x.KindOf() == events.KindOps { // 끝 줄과 같은 안내는 떼고 끝에 한 번(착지 뒤 명령 성공이 「→ hc-fill status ‖ → hc-fill status」로 갔다)
			ops = append(ops, pre+strings.TrimSuffix(wpart(l), " "+digestTail)+tag)
			continue
		}
		// 감사 레인(hc-fill이 띄우는 audit-<레인>)의 판정은 그 레인 줄에 「감사 …」로 접는다
		lane, it := x.Lane, pre+x.State
		if b, ok := strings.CutPrefix(lane, "audit-"); ok && b != "" {
			lane, it = b, pre+"감사 "+x.State
		}
		if x.SHA != "-" && x.SHA != "" {
			it += " " + short8(x.SHA)
		}
		if m := strings.TrimSpace(x.Msg); m != "" && m != "-" {
			it += " " + digestCut(m, 120) // 80자는 항목 절반 넘게 잘렸다(되감기 2,401/3,870) — 120자면 1,125
		}
		if _, ok := chain[lane]; !ok {
			order = append(order, lane)
		}
		chain[lane] = append(chain[lane], it+tag)
	}
	span := clock(t0)
	if c1 := clock(t1); c1 != span {
		span += "~" + c1
	}
	parts := []string{fmt.Sprintf("hc-lane 묶음 — %d건(%s)", len(dig), span)}
	for _, lane := range order {
		parts = append(parts, lane+": "+strings.Join(chain[lane], " → "))
	}
	parts = append(parts, ops...)
	return strings.Join(append(parts, digestTail), " ‖ ")
}

// digestTail: 묶음 끝 줄
const digestTail = "→ hc-fill status"

// digestCut: 앞 n글자(룬), 잘랐으면 「…」
func digestCut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
