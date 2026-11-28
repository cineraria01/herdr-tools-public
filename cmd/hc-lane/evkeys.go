// evkeys.go: 사건 열쇠(셸 판 bin/hc-lane의 awk 조각 nkf·evkey·landed)
package main

import (
	"regexp"
	"strings"

	"herdr-tools/internal/events"
)

// nk: 기계 이름을 뺀 열쇠 — 보통 줄은 기계 칸을 *로·주소 칸은 끝의 t…만, 멈춤 열쇠(「멈춤 터미널 레인 turn」 4칸)는 주소 칸만.
// # 줄·7칸 못 된 줄은 그대로. 열쇠는 줄 그대로에서 두 칸만 바꾼 것(events.Raw — 한 글자도 다르지 않게)
func nk(l string) string {
	if l == "" {
		return l
	}
	if rest, ok := strings.CutPrefix(l, "멈춤"+tab); ok && strings.Count(rest, tab) == 2 {
		term, tail, _ := strings.Cut(rest, tab)
		return "멈춤" + tab + lastSeg(term) + tab + tail
	}
	x := events.Cols(l)
	if x.N < 7 || strings.HasPrefix(x.Time, "#") {
		return l
	}
	x.Machine, x.Term = "*", lastSeg(x.Term)
	return x.Raw()
}

// evKey: 사건의 열쇠 — 보통 줄은 줄 그대로, 멈춤은 「멈춤 터미널 레인 상태 turn」(done·idle은 「쉼」으로 묶음)
func evKey(x events.Event) string {
	if x.State != "멈춤" {
		return x.Raw()
	}
	w := x.Msg
	if strings.HasPrefix(w, "done ") || strings.HasPrefix(w, "idle ") {
		w = "쉼 " + w[5:]
	}
	return "멈춤" + tab + x.Term + tab + x.Lane + tab + w
}

func lineKey(l string) string { return nk(evKey(events.Cols(l))) }

func num(x string) bool { return digits(x) }

// samesha: -가 아니고, 소문자로 짧은 쪽 길이만큼 앞자리가 같다
func samesha(a, b string) bool {
	if a == "-" || b == "-" || a == "" || b == "" {
		return false
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	return strings.EqualFold(a[:n], b[:n])
}

// lastRep: 레인(열쇠)의 마지막 report(셸 판 lrep). 착지·판정 뒤 같은 턴의 같은 sha 완료는 착지 그대로 두고 turn만 그 완료의 것으로
type lastRep struct {
	lst, ltn, lsh, lt0 map[string]string
	pre                map[string]string // 넘김(제출·주차)·착지 바로 뒤 막힘이면 그 앞 상태 — 이어 오는 중간이 되돌린다
}

func newLastRep() *lastRep {
	return &lastRep{map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}}
}

func (r *lastRep) rep(s, h, t, k string) {
	if k == "" {
		k = "x"
	}
	// 제출·주차로 넘긴 뒤의 중간(「병합기에 제출함 … clean 멈춤」 같은 덧말)은 넘김을 지우지 않는다 — 넘긴 레인이 쉬거나 닫혀도 조용.
	// 완료·막힘·착지·판정은 다시 일을 맡은 것이라 그대로 덮는다. 2026-11-23 stall-after-submit-note
	if s == "중간" && handedOff(r.lst[k]) {
		return
	}
	// 넘김·착지 뒤 막힘(clean 멈춤 등)에 이은 중간(clean 성공 뒤 「막힘 풀림」)은 막힘 앞 상태로 — 넘긴 레인이 풀린 뒤 쉬어도 조용
	// (2026-11-25 ht-clean-unstick). 넘김 없는 막힘 뒤 중간은 다시 일하는 중이라 그대로 중간
	if p := r.pre[k]; s == "중간" && r.lst[k] == "막힘" && p != "" {
		r.lst[k], r.ltn[k] = p, t
		delete(r.pre, k)
		return
	}
	// 막힘이 이어지면(clean을 다시 돌려 또 멈춤) 넘김은 첫 막힘 앞 그대로(감사 atp1pxd 낮음 1)
	if s == "막힘" && (handedOff(r.lst[k]) || r.lst[k] == "착지") {
		r.pre[k] = r.lst[k]
	} else if s != "막힘" || r.lst[k] != "막힘" {
		delete(r.pre, k)
	}
	if sh, ok := r.lsh[k]; ok && s == "완료" && samesha(h, sh) && num(t) && num(r.lt0[k]) && atoi(t) <= atoi(r.lt0[k]) {
		r.ltn[k] = t
		return
	}
	r.lst[k], r.ltn[k] = s, t
	delete(r.lsh, k)
	if s == "착지" || s == "판정" {
		r.lsh[k], r.lt0[k] = h, t
	}
}

// turnOf: 사건의 turn 칸(그 칸이 없는 옛 줄이면 -)
func turnOf(x events.Event) string {
	if x.N >= 8 {
		return x.Turn
	}
	return "-"
}

// endQuiet: 터미널이 이 상태(중간을 뺀 마지막 report) 뒤에 사라지면 알릴 것이 없다 — 끝 보고·넘김이거나 막힘을 이미 알렸다(한 문)
func endQuiet(end string) bool {
	switch end {
	case "완료", "판정", "제출", "착지", "주차", "막힘":
		return true
	}
	return false
}

// closeName: 도구가 닫은 레인 터미널(정리됨)을 알릴 이름 — false면 알리지 않음: 끝 보고·넘김·알린 막힘 뒤(end, 뒤따르는 중간은 그 표시를
// 지우지 않는다)이거나 쉼을 이미 알렸다(공용 보냄 기록). 아니면 보고 없이 사라진 것 — 쉬는 터미널을 닫는 hc-sweep R2면 그 사실을 이름에
func closeName(end, msg string, rested bool) (string, bool) {
	switch {
	case endQuiet(end):
		return "끝 보고(" + end + ") 뒤 도구가 닫음", false
	case rested:
		return "쉼을 이미 알림", false
	case strings.HasPrefix(msg, "hc-sweep R2"):
		return "보고 없이 쉬다 닫힘(" + msg + ")", true
	}
	return "뜻밖의 종료(정리됨) " + msg, true
}

var reDigits = regexp.MustCompile(`[0-9]+`)

// laneField: 사건 줄의 넷째 칸(레인) — 칸 전부를 풀지 않고
func laneField(l string) string {
	for i := 0; i < 3; i++ {
		j := strings.IndexByte(l, '\t')
		if j < 0 {
			return ""
		}
		l = l[j+1:]
	}
	if j := strings.IndexByte(l, '\t'); j >= 0 {
		return l[:j]
	}
	return l
}

// semKey: 뜻으로 같은 사건의 열쇠(한 문 — 받는 쪽 칸 --to·11칸과 기계 칸이 달라도 같은 사건은 한 번): 키(12칸)가 있으면 이름·상태·키,
// 없으면 같은 실행(10칸)·상태·sha·글, 실행도 모르는 옛 줄은 시각까지. 놀고 있음은 사유(숫자 뺀 글)마다 하루 한 번. 멈춤은 멈춤 열쇠
func semKey(x events.Event) string {
	if x.State == "멈춤" || x.N < 7 || strings.HasPrefix(x.Time, "#") {
		return nk(evKey(x))
	}
	j := func(p ...string) string { return strings.Join(p, "\x1f") }
	switch {
	case x.Lane == events.OpsFillIdle:
		return j(x.Lane, x.State, reDigits.ReplaceAllString(x.Msg, "N"), strings.SplitN(x.Time, "T", 2)[0])
	case x.Key != "" && x.Key != "-":
		return j(x.Lane, x.State, x.Key)
	case x.Run != "" && x.Run != "-":
		return j(x.Lane, x.State, x.Run, x.SHA, x.Msg)
	}
	return j(x.Lane, x.State, lastSeg(x.Term), x.Time, x.SHA, x.Msg)
}

// newEvents: 커서·sent에 없는 새 사건(셸 판 ev_cmd의 큰 awk) — 정리됨 판정(closeName), 뜻이 같은 사건 한 번(semKey), 지휘 거름,
// --state·--since 거름. ctx는 창 앞의 사건 — 판정 문맥(마지막 report)으로만 쓰고 내보내지 않는다
func (e *ev) newEvents(ctx, cand []string, born string) []string {
	seen, ssem := map[string]bool{}, map[string]bool{}
	var rested map[string]bool // 공용 보냄 기록에 쉼이 있는 터미널·레인(restedOf)
	cl := map[string]bool{}    // 후보의 레인 이름 — 뜻 열쇠는 이 레인의 본 줄만 짓는다(바퀴마다 커서 전부를 풀지 않게)
	for _, l := range cand {
		cl[laneField(l)] = true
	}
	mark := func(l string) {
		seen[nk(l)] = true
		if !strings.HasPrefix(l, "멈춤"+tab) && cl[laneField(l)] { // 멈춤 열쇠는 뜻 열쇠가 곧 줄 열쇠
			ssem[semKey(events.Cols(l))] = true
		}
	}
	fl := ""
	cls, _ := readLines(e.cur)
	for _, l := range cls {
		if v, ok := strings.CutPrefix(l, "#since"+tab); ok {
			fl = v
		} else {
			mark(l)
		}
	}
	if e.wsf != "" {
		sl, _ := readLines(e.wsf)
		for _, l := range sl {
			mark(l)
		}
		rested = restedOf(sl)
	} else {
		rested = restedOf(cls) // wait는 한 문이 아니다 — 자기 커서의 쉼 알림을 본다(watch는 공용 기록만 — 어느 watch든 같은 답)
	}
	kind := e.lkinds()
	par := map[string]string{}
	for _, r := range e.pmap {
		par[r[0]] = r[1]
	}
	W := "," + e.sts + ","
	end := map[string]string{} // 터미널·레인 → 중간을 뺀 마지막 report
	for _, l := range ctx {    // 아래 cand 고리와 같은 열쇠·같은 규칙(정리됨·멈춤은 report가 아니다)
		if x := events.Cols(l); x.N >= 7 && x.State != "멈춤" && x.State != "정리됨" && x.State != "중간" {
			end[x.Term+"\x1c"+x.Lane] = x.State
		}
	}
	done, dsem := map[string]bool{}, map[string]bool{}
	var out []string
	for _, l := range cand {
		if !strings.Contains(l, tab) {
			continue
		}
		x := events.Cols(l)
		t2 := x.Term + "\x1c" + x.Lane
		if x.State == "정리됨" {
			if _, rec := kind[[2]string{lastSeg(x.Term), x.Lane}]; !rec { // 레인 기록에 없는 터미널의 정리는 이 watch의 레인이 아니다(watch.go lkinds)
				continue
			}
			name, ok := closeName(end[t2], x.Msg, rested[lastSeg(x.Term)+"\x1c"+x.Lane])
			if !ok {
				continue
			}
			x.State, x.SHA, x.Msg = "멈춤", "-", name // 창(evreadCtx)은 7칸 이상 줄만이라 세 칸이 다 있다
		}
		if x.State != "멈춤" && x.State != "중간" {
			end[t2] = x.State
		}
		if my := e.myp; my != "" {
			o := x.Owner
			if x.N >= 11 && o != "-" && o != "*" {
				ow := strings.Split(o, "@")
				addr := ow[0]
				if addr != my && !strings.HasSuffix(addr, "/"+my) && !strings.HasSuffix(my, "/"+addr) {
					continue
				}
				if len(ow) > 1 && (len(ow) != 2 || !num(ow[1]) || ow[1] != born) {
					continue
				}
			}
			if x.N < 11 || o == "-" {
				if p, ok := par[x.Term]; ok && p != "-" && p != my && !strings.HasSuffix(p, "/"+my) {
					continue
				}
			}
		}
		k, s := nk(evKey(x)), semKey(x)
		// --lane으로 거른 사건은 내보내지 않아 커서에도 안 적힌다(거름 없는 wait가 그대로 받는다)
		// 커서 바닥(#since)은 wait만 — watch는 한 문(묵음 2시간·공용 기록)이 거른다: 받는 쪽이 정해진 사건이 그 watch 바닥 앞이라 영영 안 가지 않게
		if x.Time >= e.since && (e.ec == "watch" || x.Time >= fl) && (W == ",," || strings.Contains(W, ","+x.State+",")) && e.laneOK(x) && !seen[k] && !done[k] &&
			!ssem[s] && !dsem[s] {
			done[k], dsem[s] = true, true
			out = append(out, x.Raw())
		}
	}
	sortByTime(out)
	return out
}
