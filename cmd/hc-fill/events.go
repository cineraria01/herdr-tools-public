package main

// 레인 사건 — 셸 판 lib/hc-fill/audit.sh의 mgres·ev_tick·park_tick·wcnt

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/tri"
)

// evUnk: 사건을 못 읽은 기계(EV의 「모름 <기계>」 줄)
func evUnk() []string {
	var o []string
	for _, e := range EV {
		if strings.HasPrefix(e, "모름 ") {
			o = append(o, strings.TrimPrefix(e, "모름 "))
		}
	}
	return o
}

// mgres <레인> <제출 ID>: 병합기 기계(MGM)가 그 레인 이름·그 제출 ID로 적은 마지막 결과(착지·막힘) 사건 줄, 없으면 빈 값
func mgres(lane, sid string) string {
	if sid == "" {
		return ""
	}
	mg := mgSet() // 이름 바꾼 기계의 옛 이름도(rename.go)
	l := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(evLane) != lane || (r.F(evState) != "착지" && r.F(evState) != "막힘") || !mg[r.F(evMach)] {
			continue
		}
		if strings.Contains(" "+strings.ReplaceAll(r.F(evMsg), ",", " ")+" ", " 제출 ID "+sid+" ") {
			l = e
		}
	}
	return l
}

// evSid <제출 사건 줄>: 그 제출 ID(events.SubmitID — 12칸 submit:<ID>, 옛 줄은 ID 꼴 첫 낱말). 손으로 적은 제출이면 ""
func evSid(l string) string {
	e, ok := events.Parse(l)
	if !ok {
		return ""
	}
	return e.SubmitID()
}

// sublast <레인> <주소>: joblast(…, "제출")와 같되 제출 ID가 있는 진짜 제출만 — 손으로 적은 report 제출은 건너뛰고 그 앞의 제출을
func sublast(n, a string) string {
	saved := EV
	defer func() { EV = saved }()
	var f []string
	for _, e := range EV {
		if evF(e, evState) != "제출" || evSid(e) != "" {
			f = append(f, e)
		}
	}
	EV = f
	return joblast(n, a, "제출")
}

// evSortU: awk 'NF' | sort -u (바이트 차례)
func evSortU(xs []string) []string {
	var o []string
	seen := map[string]bool{}
	for _, x := range xs {
		if strings.TrimSpace(x) == "" || seen[x] {
			continue
		}
		seen[x] = true
		o = append(o, x)
	}
	sort.Strings(o)
	return o
}

// ev_tick: 바퀴에 한 번 레인 사건을 읽어 EV에 두고, 마지막 보고 last.tsv를 쓴다(dry-run은 안 씀).
// up 줄의 기계·쓰기 소유 실행 기록(OREC)의 기계·병합기 기계를 hc-lane events --on 하나로 묶어 읽는다
func evTick() {
	var eups []Row
	var projs []string
	for _, r := range qRows() {
		if r.F(qState) == "up" {
			eups = append(eups, Row{r.F(qName), r.F(qMach), r.F(qAddr), r.F(qRID)})
			projs = append(projs, r.F(qProj))
		}
	}
	var eoms []string
	for _, r := range OREC {
		eoms = append(eoms, r.F(owMach))
		projs = append(projs, r.F(owProj))
	}
	// 병합기를 켠 프로젝트는 떠 있는 줄이 없어도 병합기 사건을 읽는다(반송 → 자동 다시 맞춤, reland.go)
	mgp := relandOn() || len(prProjects()) > 0 // PR 거울도 줄이 빠진 뒤의 착지 사건을 본다(pr.go)
	for _, f := range confRows("merger") {
		if len(f) >= 3 && mgp {
			projs = append(projs, f[1])
		}
	}
	if len(eups) == 0 && strings.TrimSpace(strings.Join(eoms, "")) == "" && !mgp {
		return
	}
	MGM = ""
	for _, p := range evSortU(projs) {
		MGM += " " + strings.Join(mgmach(p), " ")
	}
	ms := strings.Fields(MGM)
	for _, r := range eups {
		ms = append(ms, r.F(upMach))
	}
	ms = append(ms, eoms...)
	args := []string{"events"}
	for _, m := range evSortU(ms) {
		if m != LOCAL && connHas(m) {
			args = append(args, "--on", m)
		}
	}
	out, rc := Cmd{Name: "hc-lane", Args: args}.Run()
	if rc != 0 {
		EVBAD = true
		fmt.Println("레인 사건 읽기 실패(hc-lane events) — 다음 바퀴에")
		return
	}
	EV = lines(sub(out))
	if dry {
		return
	}
	eunk, enow, lrt := evUnk(), time.Now().Format("15:04"), DATA+"/last.tsv"
	old := readRows(lrt)
	var b strings.Builder
	for _, u := range eups {
		n, m, a, rid := u.F(upName), u.F(upMach), u.F(upAddr), u.F(upRID)
		if n == "" {
			continue
		}
		// 끊긴 기계의 지난 값은 줄 ID가 같을 때만 남긴다
		if m != LOCAL && (!connHas(m) || has(eunk, m)) {
			for _, o := range old {
				if o.F(lsName) == n && o.F(lsRID) == rid {
					b.WriteString(o.Set(lsFresh, "old").Line() + "\n")
					break
				}
			}
			continue
		}
		l := joblast(n, a, "")
		if l == "" {
			b.WriteString(strings.Join([]string{n, "-", "-", "-", "-", enow, "fresh", rid}, "\t") + "\n")
			continue
		}
		// 제출 뒤 정리 중간 보고가 끼어도 같은 실행의 병합 결과를 보인다
		subm := ""
		switch evF(l, evState) {
		case "제출", "중간":
			subm = sublast(n, a)
		}
		if subm != "" {
			if r := mgres(n, evSid(subm)); r != "" {
				l = r
			}
		}
		eh := "?"
		if t, err := time.Parse("2006-01-02T15:04:05Z", evF(l, evTime)); err == nil {
			eh = t.Local().Format("15:04")
		}
		b.WriteString(strings.Join([]string{n, dash(evF(l, evState)), dash(head(evF(l, evSHA), 8)), eh, dash(runeCut(evF(l, evMsg), 40)), enow, "fresh", rid}, "\t") + "\n")
	}
	if err := writeAtomic(lrt, b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// park_tick: 먼저 띄운(early)·다시 잡힌(parked) up 줄의 실행이 마지막 report로 「주차」를 적었으면 같은 줄 ID로 대기·parked로 되돌린다
func parkTick() {
	if EVBAD || !nonEmpty(Q) {
		return
	}
	for _, q := range qRows() {
		n, m, a, st, rid, ef := q.F(qName), q.F(qMach), q.F(qAddr), q.F(qStuck), q.F(qRID), q.F(qEarly)
		if q.F(qState) != "up" || (ef != "early" && ef != "parked") || n == "" {
			continue
		}
		if parkev(n, a, m) != tri.Yes {
			continue
		}
		w := ""
		if ef != "early" {
			w = " (주차 뒤 다시 잡힌 줄 복구)"
		}
		if dry {
			fmt.Println(P + "주차로 되돌릴 " + n + " (" + a + ")" + w)
			continue
		}
		if qset(true, rid, n, "wait", "-", "-", st, "parked", "") == 0 {
			fmt.Println("주차 " + n + ": " + a + " 가 lane/" + n + " (" + head(evF(PKL, evSHA), 8) + ")에 두고 끝냄 — 자리 비움, 선행 착지 뒤 그 가지에서 이어 띄움" + w)
		}
	}
}

// wcnt <레인>: 그 줄을 --after로 기다리는 줄 수
func wcnt(lane string) int { return WC[lane] }
