package main

// 닫는 문턱 손잡이(knobs-migrate, docs/ops/knobs.md): sweep_r1·sweep_r2·sweep_r3(분). 읽는 곳은 하나 — 이 기계에 공통 손잡이 저장 파일이
// 있으면(지휘 기계) 그 파일, 없으면 개인 설정 sweep.knobs_repo(이 기계의 클론 경로)가 있을 때만 그 origin의 원격 참조 refs/herdr/knobs
// (ls-remote 한 번, 바뀐 때만 fetch). 둘 다 없거나 못 읽으면 기본(30·30·360분 — 지금 값 그대로).

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/knob"
)

// limits: 닫는 문턱(분)
type limits struct{ r1, r2, r3 int }

var lim = limits{30, 30, 360}

// sweepKnobs: 이번 쓸기의 문턱을 정한다(쓸기마다 새로 — 바꾼 값은 다음 쓸기에 먹는다)
func sweepKnobs(now time.Time) limits { return limitsFrom(knobVals(now)) }

// graceFrom: 상주 판 점검 유예(sweep_resident_grace, 분 — 없거나 범위 밖이면 기본 5)
func graceFrom(vals map[string]string) time.Duration {
	d, _ := knob.Find("sweep_resident_grace")
	v, ok := vals[d.Name]
	if !ok || d.Check(v) != nil {
		v = d.Default
	}
	n, _ := strconv.Atoi(v)
	return time.Duration(n) * time.Minute
}

// knobVals: 이번 쓸기에 살아 있는 손잡이 값(이름 → 값) — 한 번 읽어 닫는 문턱과 상주 판 점검이 함께 쓴다
func knobVals(now time.Time) map[string]string {
	vals := map[string]string{}
	if _, err := os.Stat(knob.Path(home)); err == nil {
		es, err := knob.Load(knob.Path(home))
		if err != nil {
			fmt.Fprintf(os.Stderr, "hc-sweep: 손잡이를 못 읽어 기본 문턱으로: %v\n", err)
		}
		for n, e := range es {
			if e.Live(now) {
				vals[n] = e.Value
			}
		}
	} else if dir, ok := conf("sweep.knobs_repo", ""); ok && dir != "" {
		r, st := knob.ReadRemote(dir, io.Discard, now)
		if st == gitx.Unknown {
			fmt.Fprintf(os.Stderr, "hc-sweep: 손잡이 참조를 못 읽어 기본 문턱으로(%s)\n", dir)
		}
		vals = r
	}
	return vals
}

// r3Text: 남기는 까닭의 R3 문턱(시간으로 나뉘면 「6시간」, 아니면 「90분」)
func (l limits) r3Text() string {
	if l.r3%60 == 0 {
		return fmt.Sprintf("%d시간", l.r3/60)
	}
	return fmt.Sprintf("%d분", l.r3)
}

func limitsFrom(vals map[string]string) limits {
	l := limits{30, 30, 360}
	for name, p := range map[string]*int{"sweep_r1": &l.r1, "sweep_r2": &l.r2, "sweep_r3": &l.r3} {
		d, _ := knob.Find(name)
		if v, ok := vals[name]; ok && d.Check(v) == nil {
			*p, _ = strconv.Atoi(v)
		}
	}
	return l
}
