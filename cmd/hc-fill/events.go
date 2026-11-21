package main

// 레인 사건 — 셸 판 lib/hc-fill/audit.sh의 mgres·ev_tick·park_tick·wcnt

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
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
	mg := map[string]bool{}
	for _, m := range strings.Fields(MGM) {
		mg[m] = true
	}
	l := ""
	for _, e := range EV {
		r := splitRow(e)
		if r.F(4) != lane || (r.F(5) != "착지" && r.F(5) != "막힘") || !mg[r.F(2)] {
			continue
		}
		if strings.Contains(" "+strings.ReplaceAll(r.F(7), ",", " ")+" ", " 제출 ID "+sid+" ") {
			l = e
		}
	}
	return l
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
		if r.F(6) == "up" {
			eups = append(eups, Row{r.F(1), r.F(7), r.F(8), r.F(17)})
			projs = append(projs, r.F(2))
		}
	}
	var eoms []string
	for _, r := range OREC {
		eoms = append(eoms, r.F(3))
		projs = append(projs, r.F(2))
	}
	if len(eups) == 0 && strings.TrimSpace(strings.Join(eoms, "")) == "" {
		return
	}
	MGM = ""
	for _, p := range evSortU(projs) {
		MGM += " " + strings.Join(mgmach(p), " ")
	}
	ms := strings.Fields(MGM)
	for _, r := range eups {
		ms = append(ms, r.F(2))
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
		n, m, a, rid := u.F(1), u.F(2), u.F(3), u.F(4)
		if n == "" {
			continue
		}
		// 끊긴 기계의 지난 값은 줄 ID가 같을 때만 남긴다
		if m != LOCAL && (!connHas(m) || has(eunk, m)) {
			for _, o := range old {
				if o.F(1) == n && o.F(8) == rid {
					b.WriteString(o.Set(7, "old").Line() + "\n")
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
		switch evF(l, 5) {
		case "제출":
			subm = l
		case "중간":
			subm = joblast(n, a, "제출")
		}
		if subm != "" {
			if r := mgres(n, F1(strings.Fields(evF(subm, 7)), 0)); r != "" {
				l = r
			}
		}
		eh := "?"
		if t, err := time.Parse("2006-01-02T15:04:05Z", evF(l, 1)); err == nil {
			eh = t.Local().Format("15:04")
		}
		b.WriteString(strings.Join([]string{n, dash(evF(l, 5)), dash(head(evF(l, 6), 8)), eh, dash(runeCut(evF(l, 7), 40)), enow, "fresh", rid}, "\t") + "\n")
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
		n, m, a, st, rid, ef := q.F(1), q.F(7), q.F(8), q.F(9), q.F(17), q.F(14)
		if q.F(6) != "up" || (ef != "early" && ef != "parked") || n == "" {
			continue
		}
		if parkev(n, a, m) != 0 {
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
			fmt.Println("주차 " + n + ": " + a + " 가 lane/" + n + " (" + head(evF(PKL, 6), 8) + ")에 두고 끝냄 — 자리 비움, 선행 착지 뒤 그 가지에서 이어 띄움" + w)
		}
	}
}

// wcnt <레인>: 그 줄을 --after로 기다리는 줄 수
func wcnt(lane string) int { return WC[lane] }
