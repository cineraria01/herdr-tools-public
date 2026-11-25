package main

// 묶인 기계(ht-advise-only-stuck, 2026-11-25 지휘 t30x — --only 한 기계로 묶은 줄이 선행이 들어간 뒤에도 그 기계 레인상한으로 「대기(주차 —
// 선행 기다림)」에 남았고 다른 기계는 놀았다): 바퀴가 선행을 지나 띄우려 했는데 묶인 기계가 모두 상한·멈춤이라 못 띄운 줄을
// <DATA>/pinned.tsv(레인 · 줄 ID · 처음 본 초 · 까닭)에 적는다. status·state는 그 까닭을, advise는 advise_pinned분 넘으면 손댈 것으로

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// pinStale: pinned.tsv를 바퀴가 이 초 넘게 다시 쓰지 않았으면(launchd 꺼짐·잠금 실패로 바퀴가 멈춤) 옛 기록이라 안 본다(감사 atp22es 낮음 2)
// ponytail: 바퀴 간격과 무관한 고정 10분 — 바퀴가 10분 넘게 걸리면 그동안 까닭이 잠깐 안 보인다. 그때는 tick.tsv 바퀴 시각으로
const pinStale = 600

var (
	ONLYFULL string // placeCands: 이 줄의 묶인 기계(--only)가 모두 상한·멈춤이면 그 까닭(「a 상한 — 부하 18.9/10 · b 멈춤」), 아니면 빈 값
	PINNOW   []Row  // 이번 바퀴에 그렇게 못 뜬 줄(pinned.tsv 꼴, 처음 본 초 칸은 비움)
)

// pinNote <기계> <까닭 한 마디>: 「a 상한 — 부하 18.9/10」(부하를 이번 바퀴에 읽었을 때만 부하)
func pinNote(m, w string) string {
	s := m + " " + w
	if l := LOADS[m]; l.OK {
		s += fmt.Sprintf(" — 부하 %.1f/%d", l.L1, l.Cores)
	}
	return s
}

// pinFinish: placeCands 끝 — 묶인 줄(쓰기 줄)의 정적 후보(--only·능력·저장소)가 모두 상한·멈춤으로 빠졌으면 ONLYFULL에 그 까닭들
func pinFinish(only string, isa bool, nstatic int, pin []string, cands []Machine) {
	if only != "-" && !isa && nstatic > 0 && len(pin) == nstatic && len(cands) == 0 {
		ONLYFULL = strings.Join(pin, " · ")
	}
}

// pinSave <지금>: 이번 바퀴의 PINNOW를 pinned.tsv에. 같은 레인·줄 ID가 지난번에도 있었으면 처음 본 초는 그대로
func pinSave(now time.Time) {
	old := map[string]string{}
	for _, r := range readRows(DATA + "/pinned.tsv") {
		old[r.F(pnLane)+"\t"+r.F(pnRID)] = r.F(pnSince)
	}
	var b strings.Builder
	for _, p := range PINNOW {
		since := old[p.F(pnLane)+"\t"+p.F(pnRID)]
		if since == "" {
			since = strconv.FormatInt(now.Unix(), 10)
		}
		b.WriteString(p.F(pnLane) + "\t" + p.F(pnRID) + "\t" + since + "\t" + flat1(p.F(pnWhy)) + "\n")
	}
	if err := writeAtomic(DATA+"/pinned.tsv", b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}

// pinOf <레인> <줄 ID>: 그 줄이 묶인 기계 때문에 못 뜬 까닭과 처음 본 초(아니면 빈 값·0)
func pinOf(name, rid string) (string, int64) {
	p := DATA + "/pinned.tsv"
	if fi, err := os.Stat(p); err != nil || time.Since(fi.ModTime()) > pinStale*time.Second {
		return "", 0
	}
	for _, r := range readRows(p) {
		if r.F(pnLane) == name && r.F(pnRID) == rid {
			s, _ := strconv.ParseInt(r.F(pnSince), 10, 64)
			return r.F(pnWhy), s
		}
	}
	return "", 0
}
