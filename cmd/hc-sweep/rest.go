package main

// 쉼 판정(ht-sweep-busy-idle → ht-rest-one-gate, docs/hc-sweep/busy-idle.md 「정말 쉬는가」): 판정 자체는 공용 꾸러미 internal/rest 한 곳
// (hc-fill·hc-lane·hc-job-server도 그것을 거친다 — 다른 기계는 이 도구의 --rest로 묻는다). 이 파일은 hc-sweep의 닫기 판정(settle = judge
// 규칙 + 쉼)과 남김 기록. 터미널을 닫는 길은 모두 settle을 거친다

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"herdr-tools/internal/rest"
)

// rested: 쉼 판정(rest.Verdict). 0값은 모름이다(확인을 거치지 않은 닫기를 closeTerm이 거절한다)
type rested = rest.Verdict

const (
	restUnknown = rest.Unknown
	restQuiet   = rest.Quiet
	restBusy    = rest.Busy
)

// quiet: rest.Of에 이 쓸기의 실행기로 ps와 열린 경로(openPaths — 디스크 쓸기와 같은 lsof 읽기)를 준다. lsof 상한은 ps와 같은 30초:
// --rest 전체가 묻는 쪽 상한(rest.AskMax 60초) 안에 끝나게(sweep.lsof_secs는 디스크 쓸기 몫 — 느린 기계에서 넘기면 모름)
func quiet(t map[string]any, now, r3 float64, r runner) rested {
	return rest.Of(t, now, r3, func() (string, bool) {
		res := r(30*time.Second, nil, "ps", "-A", "-o", "pid=,ppid=")
		return res.out, res.err == nil && res.rc == 0
	}, func() map[string]bool { return openPaths(r, 30*time.Second) })
}

// restMain: hc-sweep --rest <주소> — 이 기계 터미널 하나의 쉼 판정을 한 줄(rest.Line)로. 다른 도구·다른 기계가 닫거나 제출하기 직전에
// 묻는다(rest.Ask). r3은 이 기계의 쓸기 문턱(sweep_r3) 그대로. hc get 실패는 모름 줄(종료 0 — 답의 꼴은 늘 한 줄). 다른 기계 주소
// (「기계/tN」)는 이 기계 자료(ps·세션 파일)로 판정할 수 없어 모름(감사 atp7a3w L2 — rest.Ask는 접두를 떼어 그 기계에서 묻는다)
func restMain(addr string) int {
	if strings.Contains(addr, "/") {
		fmt.Println(rest.Line(rested{Sig: "다른 기계 주소 — 그 기계에서 hc-sweep --rest <tN>으로 물어라"}))
		return 0
	}
	lim = sweepKnobs(time.Now())
	v := rested{Sig: "hc get 실패"}
	if t, err := hc("get", addr); err == nil {
		v = quiet(t, float64(time.Now().UnixNano())/1e6, float64(lim.r3)*minute, run)
	}
	fmt.Println(rest.Line(v))
	return 0
}

// settle: 닫기 판정 = judge(규칙) + quiet(정말 쉬는가). 규칙이 나온 터미널에만 기록을 본다(바퀴 시간). 턴이 끝나고 1분 넘어 기록이 바뀌었으면
// 쉰 시간은 마지막 활동부터 다시 센다(제출 뒤 빠른 닫기도 R2 문턱부터). 모름은 R3(상태가 r3만큼 그대로)일 때만 닫는다
func settle(t map[string]any, f facts, now float64, keep map[string]bool, r runner) (string, string, rested) {
	why, kept := judge(t, f, now, keep)
	if why == "" {
		return "", kept, rested{}
	}
	q := quiet(t, now, float64(lim.r3)*minute, r)
	switch {
	case q.State == restBusy:
		return "", rest.Why(q), q
	case q.State == restUnknown && why != "R3":
		return "", rest.Why(q), q
	case q.State == restUnknown:
		q.State = restQuiet
	case q.Last > orNow(t, now)+minute:
		moved := map[string]any{}
		for k, v := range t {
			moved[k] = v
		}
		moved["status_ms"] = q.Last
		g := f
		g.submitted = false
		if why, _ = judge(moved, g, now, keep); why == "" {
			q.State, q.Sig = restBusy, fmt.Sprintf("대화 기록이 %d분 전에 바뀜", int((now-q.Last)/minute))
			return "", rest.Why(q), q
		}
	}
	return why, "", q
}

// keptOnce: 쉼 판정으로 남긴 터미널을 closed.log에 그 주소·이름·세션·까닭 종류마다 한 번만 적는다(바퀴마다 되풀이하지 않게). 새로 적었나
func keptOnce(t map[string]any, q rested, kept string) bool {
	kind := "남김(쉼 모름)"
	if q.State == restBusy {
		kind = "남김(안에서 일이 도는 중)"
	}
	key := []string{str(t, "address"), dash(str(t, "name")), kind}
	b, _ := os.ReadFile(filepath.Join(tasksDir, "closed.log"))
	for _, l := range strings.Split(string(b), "\n") {
		if x := strings.Split(l, "\t"); len(x) > 5 && x[1] == key[0] && x[2] == key[1] && x[3] == kind && x[5] == dash(q.Sid) {
			return false
		}
	}
	record(key[0], str(t, "name"), kind, str(t, "cwd"), dash(q.Sid), kept)
	return true
}
