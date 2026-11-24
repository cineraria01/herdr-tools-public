package main

// 공통 손잡이(knobs-migrate, docs/ops/knobs.md): 지휘 기계의 hc-fill이 올린 원격 참조 refs/herdr/knobs를 바퀴의 ls-remote 한 번에 함께
// 읽는다(조회를 늘리지 않는다 — 그 SHA가 바뀐 때만 cat-file, 없으면 fetch 한 번). 우선순위: 손잡이 > 일지 폴더 파일(옛 길) > 환경
// 변수(·--tick) > 기본. 참조가 없거나 못 읽으면 손잡이가 없는 것(지금 값 그대로). 바퀴 일꾼은 바퀴마다 새 프로세스라 다음 바퀴에 먹고,
// 상주 부모의 빈 바퀴 쉼(merger_tick)은 일꾼이 남긴 <일지 폴더>/knobs.remote(SHA 한 줄 + 메시지)를 쉬기 전마다 읽는다.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/gitx"
	"herdr-tools/internal/knob"
)

// knobRef: 바퀴의 ls-remote에 함께 싣는 손잡이 참조(round.go)
const knobRef = knob.Ref

// rk: 이 바퀴가 읽은 공통 손잡이(바퀴 밖 — 추측 일꾼 등 — 에서는 비어 지금 그대로)
var rk knob.Remote

func knobCache(mj string) string { return mj + "/knobs.remote" }

// loadKnobs: m.qs(바퀴의 ls-remote)에서 손잡이 참조를 읽어 rk를 채우고 묶음 크기에 바로 먹인다. 바뀌면 한 줄 알린다
func (m *M) loadKnobs() {
	now := time.Now()
	sha := refsha(m.qs, knob.Ref)
	old, _ := os.ReadFile(knobCache(m.mj))
	osha, msg, _ := strings.Cut(string(old), "\n")
	if sha != osha {
		msg = ""
		if sha != "" {
			var st gitx.State
			if msg, st = knob.ReadMsg(m.top, sha, os.Stderr); st != gitx.Yes {
				fmt.Fprintf(os.Stderr, "hc-lane merger: 손잡이 참조 %s 를 못 읽었다 — 이번 바퀴는 손잡이 없이\n", knob.Ref)
				rk = knob.Remote{}
				return
			}
		}
		if err := atomic.WriteFile(knobCache(m.mj), []byte(sha+"\n"+msg), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 손잡이 사본을 못 썼다: %v\n", err)
		}
	}
	rk = knob.ParseRemote(msg, now)
	if sha != osha {
		fmt.Printf("hc-lane merger: 손잡이 %s\n", knobLine(rk))
	}
	if v, ok := rk["merger_batch"]; ok {
		m.batch, _ = strconv.Atoi(v)
	}
}

func knobLine(r knob.Remote) string {
	var o []string
	for _, d := range knob.Defs {
		if v, ok := r[d.Name]; ok && d.Tool == "hc-merger" {
			o = append(o, d.Name+"="+v)
		}
	}
	if len(o) == 0 {
		return "없음(지금 값 그대로)"
	}
	return strings.Join(o, " ")
}

// kenv: 손잡이 <이름>이 있으면 그 값, 없으면 환경 변수 <env>
func kenv(name, env string) string {
	if v, ok := rk[name]; ok {
		return v
	}
	return os.Getenv(env)
}

// tickKnob: 상주 부모의 빈 바퀴 쉼 — 일꾼이 남긴 손잡이 사본의 merger_tick(기한을 지금 다시 본다), 없으면 def(--tick·기본)
func tickKnob(mj string, def int) int {
	b, err := os.ReadFile(knobCache(mj))
	if err != nil {
		return def
	}
	_, msg, _ := strings.Cut(string(b), "\n")
	if v, ok := knob.ParseRemote(msg, time.Now())["merger_tick"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
