package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ---- 상태 파일 <DATA>/load.tsv: 기계 · 1분 부하 · 코어 · 읽은 때 · 쓴 자리 · 상한 · 안 띄운 까닭 · 바쁨 시작 · 치우침 보낸 때 · 도는 검증 수 ·
// 빈 자리(machRoom) · 그 가운데 병합기 검증 수 · 쉬는 대기열 터미널 수 · 대기열 밖 에이전트 수(옛 판은 끝 칸들 없음 — 모름). 쓰기는 loadEnd ----

type loadRec struct {
	m              string
	l              mLoad
	used, cp       int
	why            string
	hotSince, sent int64
	free           int  // 그 바퀴 띄우기가 판정한 보통 레인 빈 자리(machRoom)
	freeOK         bool // 거짓 = 칸 없음(옛 판) — 모름
	rest, out      int  // 자리에 안 드는 쉬는 대기열 터미널·대기열 밖 에이전트 수(countSeats) — 칸 없음(옛 판)이면 -1
}

// ptrN: 0 이상이면 그 값, 음수(모름)면 nil — JSON null
func ptrN(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

// whoNote: 기계 줄에 붙이는 자리 밖 수 「 · 쉼 n · 대기열 밖 m」(0·모름은 안 적음) — 자리에는 안 들지만 그 기계에 떠 있는 것
func (x loadRec) whoNote() string {
	s := ""
	if x.rest > 0 {
		s += fmt.Sprintf(" · 쉼 %d", x.rest)
	}
	if x.out > 0 {
		s += fmt.Sprintf(" · 대기열 밖 %d", x.out)
	}
	return s
}

// freeN: 빈 자리, 모르면 -1
func (x loadRec) freeN() int {
	if !x.freeOK {
		return -1
	}
	return x.free
}

func loadRecs() map[string]loadRec {
	o := map[string]loadRec{}
	for _, r := range readRows(DATA + "/load.tsv") {
		if r.NF() < loNF {
			continue
		}
		x := loadRec{m: r.F(loMach), why: strings.TrimPrefix(r.F(loWhy), "-")}
		x.l = loadParse("load " + r.F(loL1) + " " + r.F(loCores))
		x.l.At, _ = strconv.ParseInt(r.F(loAt), 10, 64)
		x.used, _ = strconv.Atoi(r.F(loUsed))
		x.cp, _ = strconv.Atoi(r.F(loCap))
		x.hotSince, _ = strconv.ParseInt(r.F(loHot), 10, 64)
		x.sent, _ = strconv.ParseInt(r.F(loSent), 10, 64)
		if v, err := strconv.Atoi(r.F(loVer)); err == nil && v >= 0 && x.l.OK { // 칸 없음(옛 판)·「-」은 모름
			x.l.Ver, x.l.VerOK = v, true
			if g, err := strconv.Atoi(r.F(loGates)); err == nil && g >= 0 && g <= v {
				x.l.Gates, x.l.GatesOK = g, true
			}
		}
		if v, err := strconv.Atoi(r.F(loFree)); err == nil && v >= 0 {
			x.free, x.freeOK = v, true
		}
		x.rest, x.out = -1, -1
		if v, err := strconv.Atoi(r.F(loRest)); err == nil && v >= 0 {
			x.rest = v
		}
		if v, err := strconv.Atoi(r.F(loOut)); err == nil && v >= 0 {
			x.out = v
		}
		o[x.m] = x
	}
	return o
}
