package main

// 병합기 일지의 검증 길(merger-journal-why, 2026-11-24): 착지·반송 줄의 까닭 칸 끝에 「, 검증 건너뜀(추측 <아이디 8> · <기계>)」 또는
// 「, 직접 검증(<까닭>)」을 붙인다(칸 수는 그대로). 까닭은 spec()이 맨 앞 무리를 보고 정하고(추측 없음 · 맡길 기계 없음 · 묶음 · 기준
// 움직임 · 추측 실패 · 맨 앞 기다림 상한 · 모름 — 묶음·기준 움직임은 맨 앞을 맡기지 않을 때만), 맨 앞을 맡겨 기다린 바퀴는 「추측 기다림」 줄에 「검증 맡김(<기계>)」
// (jround.go specWaitLine), 맨 앞 추측이 통과했으면 착지 로그로 마저 가른다(검증 스크립트의 「건너뜀 검증」 줄 → 건너뜀, 착지의 「rebase한
// 트리가 다르다」 → 기준 움직임, 그 밖 → 모름). 요약은 hc-lane merger --why [N].

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// vwhy: 이번 착지의 검증 길. why가 비면 모름(일지에 붙이지 않는다). id가 있으면 맨 앞 추측이 통과(착지 로그로 마저 가른다)
type vwhy struct{ why, id, mach string }

// frontWhy: 맨 앞 무리를 병합기가 직접 검증하는(손잡이 front_local 0이면 기다리는) 까닭. rows = 이번 바퀴에 읽은 추측 전부(옛 기준의 같은
// 무리를 찾는다), over = 맨 앞 기다림 상한을 넘김, disp = 맨 앞도 검증 기계에 맡김(merger-verify-dispatch — 그러면 줄이 없다는 것은 빈
// 검증 기계가 없었다는 뜻이다)
func frontWhy(st []specStep, rows []specRow, over, disp bool) string {
	if over {
		return "맨 앞 기다림 상한"
	}
	if len(st) == 0 {
		return "모름"
	}
	f := st[0]
	if f.row == nil && disp {
		return "맡길 기계 없음"
	}
	if r := f.row; r != nil && r.machine != "-" { // 검증 기계에 띄운 추측
		if r.state == specFail {
			return "추측 실패"
		}
		return "모름" // 시간 넘김·안 뜸(통과·도는 중은 여기 안 온다)
	}
	if r := f.row; r != nil && (r.state == specConflict || disp) {
		return "모름" // 충돌 · 맡겼는데 못 띄움·준비 실패(state=lost, 기계 -)
	}
	if len(f.g.shas) > 1 {
		return "묶음"
	}
	for _, r := range rows {
		if r.group == f.g.key && r.parent != f.parent && r.machine != "-" {
			return "기준 움직임"
		}
	}
	return "추측 없음"
}

// vtag: 일지 까닭 칸에 붙일 글(앞 「, 」 포함). 검증 길을 모르면 빈 값
func (m *M) vtag() string {
	switch {
	case m.vw.why == "":
		return ""
	case m.vw.id == "":
		return ", 직접 검증(" + m.vw.why + ")"
	}
	b, err := os.ReadFile(m.lgf)
	if err != nil {
		return ", 직접 검증(모름)"
	}
	for _, l := range strings.Split(string(b), "\n") {
		switch {
		// 검증 스크립트가 건너뛴 표시 줄 — 계약 꼴 「건너뜀 검증」과 Go 검증 판의 「▸ 건너뜀 — 검증 기록」(docs/hc-lane/merger-spec-verify.md)
		case strings.HasPrefix(l, "건너뜀 검증"), strings.HasPrefix(l, "▸ 건너뜀 — 검증 기록"):
			return fmt.Sprintf(", 검증 건너뜀(추측 %s · %s)", m.vw.id, m.vw.mach)
		case strings.HasPrefix(l, "hc-lane land: 추측 트리(") && strings.Contains(l, "rebase한 트리가 다르다"):
			return ", 직접 검증(기준 움직임)"
		}
	}
	return ", 직접 검증(모름)"
}

// whyMain: hc-lane merger --why [N] — 이 기계 병합기 일지의 최근 N번(기본 20) 착지 한 줄 요약
func whyMain(a []string) int {
	n := 20
	if len(a) > 1 {
		fmt.Fprintln(os.Stderr, "사용법: hc-lane merger --why [<최근 착지 수>]")
		return 2
	}
	if len(a) == 1 {
		v, err := strconv.Atoi(a[0])
		if err != nil || v < 1 {
			fmt.Fprintf(os.Stderr, "hc-lane merger: --why 의 수가 이상하다: %s\n", a[0])
			return 2
		}
		n = v
	}
	b, err := os.ReadFile(gEvdir + "/merger-journal.tsv")
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "hc-lane merger: 병합기 일지를 읽지 못했다: %v\n", err)
		return 1
	}
	fmt.Println(whySummary(strings.Split(string(b), "\n"), n))
	return 0
}

// whySummary: 일지 줄 → 「최근 착지 N번: 검증 건너뜀 k · 직접 검증 j(까닭별) · 까닭 없음 x · 착지 시간 중앙 — 건너뜀 · 직접」.
// 착지 한 번 = 「착지」 줄을 후보 SHA로 묶은 것(hc-lane stats와 같음, 「이미 … 에 있음」 정리는 뺌). 착지 시간 = 착지 시작 → 끝(검증 포함)
func whySummary(lines []string, n int) string {
	type land struct {
		why string
		sec int64
	}
	var ls []land
	seen := map[string]bool{}
	for _, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) < 12 || f[9] != "착지" || seen[f[10]] || strings.Contains(f[11], "이미 ") && strings.Contains(f[11], " 에 있음") {
			continue
		}
		seen[f[10]] = true
		sec := int64(-1)
		t0, e0 := time.Parse(time.RFC3339, f[7])
		t1, e1 := time.Parse(time.RFC3339, f[8])
		if e0 == nil && e1 == nil {
			sec = int64(t1.Sub(t0).Seconds())
		}
		ls = append(ls, land{f[11], sec})
	}
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	out := fmt.Sprintf("최근 착지 %d번", len(ls))
	if len(ls) == 0 {
		return out
	}
	skip, none := 0, 0
	direct, order := map[string]int{}, []string{}
	var ts, td []int64
	for _, x := range ls {
		if strings.Contains(x.why, ", 검증 건너뜀(") {
			skip++
			if x.sec >= 0 {
				ts = append(ts, x.sec)
			}
			continue
		}
		_, r, ok := strings.Cut(x.why, ", 직접 검증(")
		if !ok {
			none++
			continue
		}
		r, _, _ = strings.Cut(r, ")")
		if direct[r] == 0 {
			order = append(order, r)
		}
		direct[r]++
		if x.sec >= 0 {
			td = append(td, x.sec)
		}
	}
	var parts []string
	for _, r := range order {
		parts = append(parts, fmt.Sprintf("%s %d", r, direct[r]))
	}
	out += fmt.Sprintf(": 검증 건너뜀 %d · 직접 검증 %d", skip, len(ls)-skip-none)
	if len(parts) > 0 {
		out += "(" + strings.Join(parts, " · ") + ")"
	}
	out += fmt.Sprintf(" · 까닭 없음 %d · 착지 시간 중앙 — 건너뜀 %s · 직접 %s", none, med(ts), med(td))
	return out
}

// med: 아래 중앙값(짝수면 가운데 둘 중 작은 쪽), 없으면 -
func med(v []int64) string {
	if len(v) == 0 {
		return "-"
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return (time.Duration(v[(len(v)-1)/2]) * time.Second).String()
}
