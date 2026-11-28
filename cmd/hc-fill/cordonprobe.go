package main

// 격리 점검 항목 더하기(ht-probe-cordon, 2026-11-28, docs/hc-fill/cordon.md 「점검 항목」 — 사용자 「다섯 묶음 모두 (추천)」. 업계의 합성 점검
// synthetic monitoring: 레인이 하는 일을 작게 끝까지 해 보면 원인별 점검이 모르는 고장도 잡는다). 11-27 워커 하나가 OS 권한 창에 멈춘 동안
// 「실제 변경 못 읽음 @<기계>」 208줄이 기록만 되고 사건은 0건이었고, 같은 날 토큰 폐기는 착지 push에서야 드러났다.
// 틀은 cordon.go 그대로(cordonCks에 항목 — 제 기록·문턱·알림, 기계는 어느 항목이든 격리면 격리). 모두 지금 조각과 같은 원격 호출에 얹는다:
//   - 원격 쓰기(cdWrite): --remote-auth 호출의 remote-write 줄(push --dry-run — 원격에 아무것도 안 씀). 원격 로그인이 거절이면 모름(같은 까닭 두 번 안 알림)
//   - 에이전트 풀(cdPool): 설정 worker.proxy(worker-mac-check와 같은 키)가 있으면 차례인 기계에 env로 실어 「쓸 계정 0」만 나쁨
//   - 키체인·임시 폴더(cdKeychain·cdTmp): --repo-read 호출마다(2분 조각) 「probe …」 줄
//   - 시계(cdClock): 조각 셸의 date +%s와 부른 쪽 시각 — 오가는 시간 밖으로 120초 넘게 어긋나면 알리기만(격리 안 함)
// 시킨 일(실제 변경 조회) 실패는 격리 재료로 넣지 않았다(지휘 결정 — docs/hc-fill/cordon.md). 판정 낱말·모름 규칙은 그대로(모름은 격리하지도 풀지도 않음). 풀 출력(계정 이름)은 수 말고 받지 않는다

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/opsstate"
	"herdr-tools/internal/toolcfg"
)

var (
	// cdWrite: 원격 쓰기 — 원격 로그인과 같은 호출·같은 간격(7분)
	cdWrite = &cordonCk{key: "-write", name: "원격 쓰기", bad: "거절", pre: opsstate.CordonWrite.Pre, look: opsstate.CordonWrite.Look, every: 420,
		verdict: writeVerdict}
	// cdPool: 에이전트 풀 — 7분(원격 로그인과 같은 간격이라 처음부터 같은 조각에 함께 실린다)
	cdPool = &cordonCk{key: "-pool", name: "에이전트 풀", bad: "빔", pre: opsstate.CordonPool.Pre, look: opsstate.CordonPool.Look, every: 420,
		verdict: probeVerdict("pool")}
	// cdKeychain·cdTmp: 저장소 읽기 조각(2분)에 함께 — 따로 부를 차례는 저장소 읽기가 정한다
	cdKeychain = &cordonCk{key: "-keychain", name: "키체인", bad: "막힘", pre: opsstate.CordonKeychain.Pre, look: opsstate.CordonKeychain.Look,
		every: 120, verdict: probeVerdict("keychain")}
	cdTmp = &cordonCk{key: "-tmp", name: "임시 폴더", bad: "못 씀", pre: opsstate.CordonTmp.Pre, look: opsstate.CordonTmp.Look, every: 120,
		verdict: probeVerdict("tmp")}
	cdClock = &cordonCk{key: "-clock", name: "시계", bad: "틀림", look: "그 기계 시간 동기화(날짜와 시간 자동 설정·NTP)", every: 420,
		verdict: clockVerdict, alertOnly: true}
)

// clockMax: 시계 틀림 문턱(초) — 오가는 시간을 뺀 차가 이보다 크면 나쁨
const clockMax = 120

// poolAsked: 풀을 물은 조각이 먼저 내는 표식 줄(물었는데 줄이 없으면 옛 판 — 모름)
const poolAsked = "probe-pool-asked"

// PROBE: 이번 바퀴에 받은 점검 줄(기계 → 항목 → 값). keychain·tmp·pool은 「ok|bad|na|unknown <글>」, write는 remote-write 줄(물었는데
// 없으면 빈 값), clock은 그 기계 시각 − 이 기계 시각(초, 오가는 시간 안이면 0)
var PROBE map[string]map[string]string

// probeTake: repoTake가 잠금 안에서 부른다(온전한 출력만). asked = 원격 로그인을 물은 조각(쓰기 자격도 그 호출)
func probeTake(m, out string, asked bool) {
	now := nowUnix()
	p := map[string]string{}
	pool := false
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == poolAsked:
			pool = true
		case strings.HasPrefix(l, "probe "):
			if k, v, ok := strings.Cut(l[len("probe "):], " "); ok && p[k] == "" {
				p[k] = v
			}
		case strings.HasPrefix(l, "remote-write ") && asked && p["write"] == "":
			p["write"] = l
		case strings.HasPrefix(l, "clock "):
			f := strings.Fields(l)
			if len(f) != 3 {
				continue
			}
			t0, e1 := strconv.ParseInt(f[1], 10, 64)
			r, e2 := strconv.ParseInt(f[2], 10, 64)
			if e1 != nil || e2 != nil {
				continue
			}
			skew := int64(0) // 부른 때 t0와 받은 때 now(초를 버린 몫 1초) 사이면 오가는 시간 안 — 차 0
			switch {
			case r < t0:
				skew = r - t0
			case r > now+1:
				skew = r - now - 1
			}
			p["clock"] = fmt.Sprint(skew)
		}
	}
	if _, ok := p["write"]; asked && !ok {
		p["write"] = ""
	}
	for _, k := range []string{"keychain", "tmp"} { // --repo-read에 답한 조각이면 늘 물은 것 — 줄이 없으면 옛 판(물었는데 모름, 다음 확인은 간격 뒤로)
		if _, ok := p[k]; !ok {
			p[k] = "unknown 옛 판(줄 없음)"
		}
	}
	if _, ok := p["pool"]; !pool {
		delete(p, "pool")
	} else if !ok {
		p["pool"] = "unknown 옛 판(풀 줄 없음)"
	}
	if PROBE == nil {
		PROBE = map[string]map[string]string{}
	}
	PROBE[m] = p
}

// probeVerdict: 「probe <항목>」 줄 판정 — ok 좋음 · bad 나쁨(글) · na·unknown·꼴이 다름 = 물었는데 모름 · 줄 없음(옛 판) = 못 물음
func probeVerdict(item string) func(m string) (int, string) {
	return func(m string) (int, string) {
		v, ok := PROBE[m][item]
		if !ok {
			return 0, ""
		}
		st, msg, _ := strings.Cut(v, " ")
		switch st {
		case "ok":
			return 1, ""
		case "bad":
			return 2, bcutS(authRedact(msg), 200)
		}
		return 3, ""
	}
}

// writeVerdict: 쓰기 자격 줄 — 원격 로그인과 같은 판정(remoteVerdict). 원격 로그인이 이미 거절이면 모름(읽기가 막히면 쓰기도 막힌다 — 같은 까닭 두 번 안 알림)
func writeVerdict(m string) (int, string) {
	l, asked := PROBE[m]["write"]
	if !asked {
		return 0, ""
	}
	if v, _ := authVerdict(m); v == 2 {
		return 3, ""
	}
	return remoteVerdict(m, l, "remote-write")
}

// clockVerdict: 시계 — 차가 clockMax를 넘으면 나쁨(알리기만)
func clockVerdict(m string) (int, string) {
	v, ok := PROBE[m]["clock"]
	if !ok {
		return 0, ""
	}
	d, _ := strconv.ParseInt(v, 10, 64)
	switch {
	case d > clockMax:
		return 2, fmt.Sprintf("그 기계 시각이 %d초 빠름(오가는 시간 뺌, 문턱 %d초)", d, clockMax)
	case d < -clockMax:
		return 2, fmt.Sprintf("그 기계 시각이 %d초 느림(오가는 시간 뺌, 문턱 %d초)", -d, clockMax)
	}
	return 1, ""
}

// poolCmd: 풀 상태 명령(설정 worker.proxy — worker-mac-check와 같은 키, 없으면 빈 값 = 풀 점검 안 함). 이번 실행에 한 번 읽는다(시험이 바꾼다)
var poolCmd = sync.OnceValue(func() string { // loadSnip이 기계마다 고루틴에서 부른다
	s, _, _ := toolcfg.Get(toolcfg.Dir(), "worker.proxy")
	return strings.TrimSpace(s)
})

// snipDue: 이 바퀴에 조각을 안 부른 기계를 따로 부를 차례(cordonTick) — 조각에 실린 항목 가운데 하나라도 차례면
func snipDue(prev map[*cordonCk]map[string]cordonRow, m string, now int64) bool {
	for _, c := range []*cordonCk{cdRepo, cdKeychain, cdTmp, cdClock} {
		if c.due(prev[c], m, now) {
			return true
		}
	}
	return authDue(m) || poolDue(m)
}

// poolDue: 그 기계에 풀을 물을 차례
func poolDue(m string) bool {
	return poolCmd() != "" && len(repoPaths(m)) > 0 && cdPool.due(cdPool.load(), m, nowUnix())
}

// DEPLOYAT: 기계 → 배포 기록(herdr-sync deploy.tsv, statedeploy.go와 같은 파일)상 마지막으로 새 판을 깐(씀) 때 — 이번 실행에 한 번 읽는다
var (
	DEPLOYAT map[string]int64
	deployMu sync.Mutex // loadSnip(→ due)이 기계마다 고루틴에서 부른다
)

// deployAt: 그 기계에 도구 새 판이 깔린 마지막 때(유닉스 초, 모르면 0) — 점검 차례(due)가 배포 직후 한 번 더 돌게
func deployAt(m string) int64 {
	deployMu.Lock()
	defer deployMu.Unlock()
	if DEPLOYAT == nil {
		DEPLOYAT = map[string]int64{}
		b, _ := readFile(HOME + "/.local/state/herdr-sync/deploy.tsv")
		for _, l := range lines(b) {
			f := strings.Split(l, "\t")
			if len(f) < 8 {
				continue
			}
			t, err := time.Parse(time.RFC3339, f[0])
			if err != nil {
				continue
			}
			for _, mr := range strings.Fields(f[7]) {
				if mm, r, ok := strings.Cut(mr, "="); ok && r == "씀" {
					DEPLOYAT[deployMachine(mm)] = t.Unix()
				}
			}
		}
	}
	return DEPLOYAT[m]
}
