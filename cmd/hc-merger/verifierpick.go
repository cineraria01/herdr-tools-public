package main

// 맡길 검증 기계 고르기(merger-verifier-pick, 2026-11-24 — 「맨 앞도 검증 기계에」 첫 실사용에서 병합기가 레인 검증이 도는 첫 기계에 맨 앞을
// 맡겨 600초를 기다렸다). 「빈 검증 기계」는 이제 병합기의 추측이 안 도는 기계가 아니라 **실제로 한가한** 기계다:
//
//   - 맡기기 전에 후보 기계마다 싼 확인 한 번(hc exec --on <기계> --json --timeout <상한> -- hc-lane merger --spec-idle — 그 기계에서 도는
//     저장소 검증 수·1분 부하·코어 수, 읽기만). 바퀴에 처음 고를 때 후보 전부를 함께 부르고 그 바퀴 동안 그 값을 쓴다(후보 수만큼만).
//     한가함 = 도는 검증 0 이고 부하가 코어 수의 손잡이 idle_load %(기본 50) 밑. 확인을 못 한 기계(안 닿음·시간 넘김·꼴이 다름)는
//     모름 — 고르지 않는다. 목록 차례는 한가한 기계끼리의 차례다.
//   - 일꾼(--spec-verify)은 검증 전에 같은 확인을 손잡이 busy_wait 초(기본 30) 동안 본다. 그새 바빠져 한가해지지 않으면 검증하지 않고
//     spec 참조를 state=busy(그 기계, 본문에 기다린 것 한 줄)로 고쳐 쓰고 끝난다. 「바쁨」은 통과도 실패도 아니다(반송·범인 판정에 안 쓴다).
//   - 병합기는 맨 앞이 「바쁨」이면 기다림 상한을 기다리지 않고 그 바퀴에 바쁨을 낸 기계들을 뺀 한가한 기계로 다시 띄운다. 같은 제출의
//     바쁨이 손잡이 busy_max(기본 2)번을 넘거나 남은 한가한 기계가 없으면 손잡이 front_local대로.
//
// 손잡이는 모두 병합기 일지 폴더의 파일(바퀴마다 읽어 바로 먹는다) > 환경 변수 > 기본. 일꾼 쪽 값(busy_wait·idle_load)은 병합기가 띄울 때 넘긴다.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"herdr-tools/internal/hcjson"
	"herdr-tools/internal/verified"
)

// idleInfo: 한 기계의 한가함 재기
type idleInfo struct {
	load     float64
	cpus     int
	verifies int // 도는 저장소 검증(.claude/herdr-verify.sh를 돌리는 프로세스) 수
}

func (i idleInfo) String() string {
	return fmt.Sprintf("load=%.2f cpus=%d verifies=%d", i.load, i.cpus, i.verifies)
}

// idle: 도는 검증이 없고 부하가 코어 수의 pct% 밑
func (i idleInfo) idle(pct int) bool {
	return i.verifies == 0 && i.load*100 < float64(pct*i.cpus)
}

// parseIdle: --spec-idle의 출력 한 줄. 꼴이 다르면 ok=false(모름)
func parseIdle(s string) (idleInfo, bool) {
	var i idleInfo
	f := strings.Fields(s)
	if len(f) != 3 {
		return i, false
	}
	seen := 0
	for _, kv := range f {
		k, v, _ := strings.Cut(kv, "=")
		var err error
		switch k {
		case "load":
			i.load, err = strconv.ParseFloat(v, 64)
		case "cpus":
			i.cpus, err = strconv.Atoi(v)
		case "verifies":
			i.verifies, err = strconv.Atoi(v)
		default:
			return i, false
		}
		if err != nil {
			return i, false
		}
		seen++
	}
	return i, seen == 3 && i.cpus > 0 && i.load >= 0 && i.verifies >= 0
}

// idleProbe: 이 기계를 잰다(시험이 바꾼다)
var idleProbe = localIdle

// localIdle: 1분 부하·코어 수·도는 저장소 검증 수(ps). 못 재면 ok=false
func localIdle() (idleInfo, bool) {
	l, ok := load1()
	if !ok {
		return idleInfo{}, false
	}
	o, err := exec.Command("ps", "-A", "-o", "args=").Output()
	if err != nil {
		return idleInfo{}, false
	}
	n := 0
	for _, line := range strings.Split(string(o), "\n") {
		if strings.Contains(line, verified.Script) {
			n++
		}
	}
	return idleInfo{load: l, cpus: runtime.NumCPU(), verifies: n}, true
}

// specIdleMain: hc-lane merger --spec-idle — 이 기계의 한가함 한 줄(병합기가 hc exec --on으로 부른다). 못 재면 종료 1
func specIdleMain(a []string) int {
	if len(a) != 0 {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-idle 은 인자를 받지 않는다")
		return 2
	}
	i, ok := idleProbe()
	if !ok {
		fmt.Fprintln(os.Stderr, "hc-lane merger: --spec-idle — 부하·도는 검증을 재지 못했다")
		return 1
	}
	fmt.Println(i)
	return 0
}

// knobInt: 정수 손잡이 — 공통 손잡이 merger_<파일>(knobs.go) > <병합기 일지 폴더>/<파일>(옛 길, 바퀴마다 읽어 바로 먹는다) > 환경 변수 > 기본
func (m *M) knobInt(file, env string, def, min int) int {
	return intSetting(env, m.knobStr(file, env), def, min)
}

// idleLoad: 손잡이 idle_load(HC_LANE_SPEC_IDLE_LOAD, %, 기본 50) — 부하가 코어 수의 이만큼 밑이어야 한가함
func (m *M) idleLoad() int { return m.knobInt("idle_load", "HC_LANE_SPEC_IDLE_LOAD", 50, 1) }

// busyWait: 손잡이 busy_wait(HC_LANE_SPEC_BUSY_WAIT, 초, 기본 30) — 일꾼이 한가해지길 기다리는 상한, 넘으면 「바쁨」
func (m *M) busyWait() int { return m.knobInt("busy_wait", "HC_LANE_SPEC_BUSY_WAIT", 30, 0) }

// busyMax: 손잡이 busy_max(HC_LANE_SPEC_BUSY_MAX, 번, 기본 2) — 같은 맨 앞 제출의 「바쁨」이 이만큼을 넘으면 다시 고르지 않고 front_local
func (m *M) busyMax() int { return m.knobInt("busy_max", "HC_LANE_SPEC_BUSY_MAX", 2, 0) }

// idleMax: 한가함 확인 한 번의 상한(HC_LANE_SPEC_IDLE_MAX, 초, 기본 15)
func idleMax() int {
	return intSetting("HC_LANE_SPEC_IDLE_MAX", os.Getenv("HC_LANE_SPEC_IDLE_MAX"), 15, 1)
}

// 한가함 확인 결과
const (
	idUnknown = iota
	idIdle
	idBusy
)

// probeRemote: 그 기계의 한가함(hc exec --on). hc 실패·시간 넘김·0이 아닌 종료·꼴이 다르면 ok=false(모름)와 그 까닭
func probeRemote(mach string) (idleInfo, bool, string) {
	lim := idleMax()
	o, rc, e := boundedRun(time.Duration(lim+10)*time.Second, "hc", nil, "exec", "--on", mach, "--json", "--timeout", strconv.Itoa(lim),
		"--", "hc-lane", "merger", "--spec-idle")
	if rc == 142 {
		return idleInfo{}, false, "시간 넘김"
	}
	r, err := hcjson.ParseExec([]byte(o), []byte(e), rc)
	switch {
	case err != nil:
		return idleInfo{}, false, bcut(80, flat(err.Error()))
	case r.Code != 0 || r.TimedOut:
		return idleInfo{}, false, fmt.Sprintf("종료 %d", r.Code)
	}
	i, ok := parseIdle(string(r.Stdout))
	if !ok {
		return idleInfo{}, false, "꼴이 다름"
	}
	return i, true, ""
}

// idleProbeAll: 이 바퀴 처음 고를 때 — 후보(병합기 기계·도는 추측이 있는 기계 뺌) 전부를 함께 재어 c.idle에 둔다
func (m *M) idleProbeAll(c *specCtx, busy map[string]bool) {
	if c.idle != nil {
		return
	}
	c.idle = map[string]int{}
	pct := m.idleLoad()
	var mu sync.Mutex
	var wg sync.WaitGroup
	var notes []string
	for _, v := range c.vers {
		if busy[v.Machine] || v.Machine == m.mme {
			continue
		}
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			i, ok, txt := probeRemote(mach)
			st := idUnknown
			switch {
			case !ok:
			case i.idle(pct):
				st, txt = idIdle, i.String()
			default:
				st, txt = idBusy, i.String()
			}
			mu.Lock()
			c.idle[mach] = st
			notes = append(notes, mach+" "+[...]string{"모름", "한가함", "바쁨"}[st]+"("+txt+")")
			mu.Unlock()
		}(v.Machine)
	}
	wg.Wait()
	if len(notes) > 0 {
		fmt.Printf("hc-lane merger: 검증 기계 한가함 — %s\n", strings.Join(notes, " · "))
	}
}

// specPick: 맡길 한가한 검증 기계 — 목록 차례로, skip(이 제출에 바쁨을 낸 기계)은 빼고, avoid가 아닌 것을 먼저(없으면 avoid).
// 병합기 기계·이 바퀴에 맡긴(또는 추측이 도는) 기계·바쁨·모름인 기계는 고르지 않는다. 없으면 빈 값
func (m *M) specPick(c *specCtx, busy, skip map[string]bool, avoid string) verified.Verifier {
	var cand []verified.Verifier
	for _, v := range c.vers {
		if !busy[v.Machine] && v.Machine != m.mme && !skip[v.Machine] {
			cand = append(cand, v)
		}
	}
	if len(cand) == 0 {
		return verified.Verifier{}
	}
	m.idleProbeAll(c, busy)
	var same verified.Verifier
	for _, v := range cand {
		switch {
		case c.idle[v.Machine] != idIdle:
		case v.Machine != avoid:
			return v
		default:
			same = v
		}
	}
	return same
}

// specFree: 맡길 한가한 검증 기계(목록 차례)
func (m *M) specFree(c *specCtx, busy map[string]bool) verified.Verifier {
	return m.specPick(c, busy, nil, "")
}

// busySeen: 이 맨 앞 추측에 바쁨을 낸 기계들(<일지 폴더>/specbusy — 「<아이디> <기계>…」, 다른 아이디면 빈 값)
func (m *M) busySeen(id string) []string {
	b, err := os.ReadFile(m.mj + "/specbusy")
	if err != nil {
		return nil
	}
	f := strings.Fields(string(b))
	if len(f) < 2 || f[0] != id {
		return nil
	}
	return f[1:]
}

// specBusyRedo: 맨 앞 추측 r이 「바쁨」 — 바쁨을 낸 기계를 적고, busy_max를 넘지 않았으면 그 기계들을 뺀 한가한 기계로 곧바로 다시 띄운다.
// 다시 띄웠으면 true(m.wre에 「<바쁨 기계>→<새 기계>」)
func (m *M) specBusyRedo(c *specCtx, r *specRow, busy map[string]bool, lanes string, now int64) bool {
	seen := m.busySeen(r.id)
	from := r.machine
	if !strings.Contains(" "+strings.Join(seen, " ")+" ", " "+from+" ") {
		seen = append(seen, from)
		if err := writeVia(m.mj+"/specbusy", r.id+" "+strings.Join(seen, " ")+"\n"); err != nil {
			fmt.Fprintf(os.Stderr, "hc-lane merger: 바쁨 기계를 적지 못했다(다음 바퀴에 그 기계를 다시 고를 수 있다): %v\n", err)
		}
	}
	if len(seen) > m.busyMax() {
		fmt.Printf("hc-lane merger: 맨 앞 %s 검증 기계가 %d번 바쁨(%s) — 손잡이 busy_max %d를 넘어 다시 고르지 않는다\n", lanes, len(seen), strings.Join(seen, "·"), m.busyMax())
		return false
	}
	skip := map[string]bool{}
	for _, s := range seen {
		skip[s] = true
	}
	note := ""
	if first, again := specRecheckOf(r); again {
		note = first // 한 번 더 확인 중이던 추측이면 첫 실패 메모를 이어 준다
	}
	if !m.specRedo(c, r, busy, skip, "", note, lanes, now) {
		fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 바쁨 — 맡길 한가한 기계 없음\n", lanes, from)
		return false
	}
	fmt.Printf("hc-lane merger: 맨 앞 %s 가 %s 에서 바쁨 — %s 로 다시 고름\n", lanes, from, r.machine)
	m.wre = from + "→" + r.machine
	return true
}

// specBusyWait: 일꾼 — 검증 전에 이 기계가 한가해지길 HC_LANE_SPEC_BUSY_WAIT초(병합기 손잡이 busy_wait가 넘긴 값, 기본 30)까지 본다.
// 한가하면 ""(로그에 기다린 초 한 줄), 끝내 바쁘거나 못 재면 기다린 것 한 줄(「바쁨」 결과의 본문)
func specBusyWait(lf *os.File) string {
	max := intSetting("HC_LANE_SPEC_BUSY_WAIT", os.Getenv("HC_LANE_SPEC_BUSY_WAIT"), 30, 0)
	pct := intSetting("HC_LANE_SPEC_IDLE_LOAD", os.Getenv("HC_LANE_SPEC_IDLE_LOAD"), 50, 1)
	t0 := time.Now()
	for {
		i, ok := idleProbe()
		waited := int(time.Since(t0).Seconds())
		if ok && i.idle(pct) {
			fmt.Fprintf(lf, "hc-lane merger: 검증 자리 기다림 %d초(%s, 기준 부하 %d%%)\n", waited, i, pct)
			return ""
		}
		left := time.Duration(max)*time.Second - time.Since(t0)
		if left <= 0 {
			what := i.String()
			if !ok {
				what = "잴 수 없음"
			}
			s := fmt.Sprintf("waited %s %s %ds (busy_wait %ds, idle_load %d%%)", machine(), what, waited, max, pct)
			fmt.Fprintf(lf, "hc-lane merger: 검증 자리를 %d초 안에 못 얻었다(%s) — 바쁨\n", max, what)
			return s
		}
		time.Sleep(min(5*time.Second, left))
	}
}
