// 워커 건강 점검(2026-11-24 worker-health-sweep, docs/hc-sweep/health.md): 이 기계의 잠자기 끔 설정과, 이 문맥(hc-sweep이 도는 hc
// 터미널 = 코어가 띄운 프로그램)에서 로그인 키체인이 열리는가를 바퀴마다 보고 **상태가 바뀔 때만** 사건 hc-health 한 줄(나빠짐·회복).
// 보는 방식은 worker-mac-check와 같다(pmset -g의 SleepDisabled 1 또는 sleep 0이면 통과 · security show-keychain-info 종료 0 열림,
// 36 막힘). 읽기만 한다 — 설정을 바꾸거나 다른 앱·키체인 항목을 건드리지 않는다. 점검 실패(못 띄움·시간 넘김·모르는 종료 코드·읽을 수
// 없는 출력)는 모름: 사건을 내지 않고 앞 상태를 지우지 않는다. macOS만(다른 OS는 아무것도 안 한다). HC_SWEEP_HEALTH=off면 끈다(시험).
// 11/23 워커 맥이 잠들어 레인이 끊긴 일(다른 앱이 잠자기 끔을 되돌림), 코어가 SSH 문맥으로 떠 키체인이 막혀 레인이 52분 선 일 뒤.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"herdr-tools/internal/atomic"
	"herdr-tools/internal/resident"
)

// hItem: 점검 하나의 기록. State = 마지막으로 안 상태(good|bad, 빈 값 = 아직 모름), Since = 그 상태(또는 첫 기록)를 적은 때 — 사건 키에
// 쓴다(알림이 실패해 상태를 안 바꾸면 다음 바퀴가 같은 키로 다시, hc-lane이 같은 키는 한 번만 적는다). Unk = 마지막 바퀴가 모름이면 그 까닭
type hItem struct {
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Since   string `json:"since"`
	Checked string `json:"checked"` // 마지막으로 상태를 안 때
	Unk     string `json:"unknown,omitempty"`
	UnkAt   string `json:"unknown_at,omitempty"`
}

// hProbe: 점검 하나 — 이름(키)·보이는 이름·한 번 재기(good|bad|"" 모름, 자세히)·나쁨/회복 글
type hProbe struct {
	key, label string
	probe      func(r runner) (string, string)
	bad, good  string
}

var hProbes = []hProbe{
	{"sleep", "잠자기 끔", probeSleep, "잠자기 끔 설정이 꺼짐 — 이 기계가 잠들면 레인이 끊긴다(다른 앱이 되돌렸을 수 있다, 설정은 사람이)", "잠자기 끔 회복"},
	{"keychain", "키체인", probeKeychain, "이 hc 문맥에서 로그인 키체인이 막힘 — 코어가 SSH 문맥으로 떴을 수 있다(서명·키체인 쓰는 레인이 선다, hc-core-relaunch)", "키체인 회복(열림)"},
}

// probeSleep: worker-mac-check와 같게 SleepDisabled 1이면 통과(덮개 닫아도 안 잠듦), 아니어도 sleep 0이면 통과. 둘 다 못 찾으면 모름
func probeSleep(r runner) (string, string) {
	res := r(15*time.Second, nil, "/usr/bin/pmset", "-g")
	if res.err != nil || res.rc != 0 {
		return "", fmt.Sprintf("pmset -g 실패(%s)", rcText(res))
	}
	sd, sl := "", ""
	for _, l := range strings.Split(res.out, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "SleepDisabled":
			sd = f[1]
		case "sleep":
			sl = f[1]
		}
	}
	switch {
	case sd == "1":
		return "good", "SleepDisabled 1"
	case sl == "0":
		return "good", "sleep 0(덮개를 닫으면 잠듦)"
	case sd == "" && sl == "":
		return "", "pmset -g 출력에 SleepDisabled·sleep 없음"
	}
	return "bad", "SleepDisabled " + dash(sd) + ", sleep " + dash(sl)
}

// probeKeychain: 열려 있는지만 묻는다(암호·항목은 안 다룬다). 0 열림, 36 User interaction is not allowed, 그 밖은 모름
func probeKeychain(r runner) (string, string) {
	res := r(20*time.Second, nil, "/usr/bin/security", "show-keychain-info", filepath.Join(home, "Library/Keychains/login.keychain-db"))
	switch {
	case res.err != nil:
		return "", "security 실패(" + rcText(res) + ")"
	case res.rc == 0:
		return "good", "열림"
	case res.rc == 36:
		return "bad", "rc 36 User interaction is not allowed"
	}
	return "", "security 종료 " + rcText(res)
}

func rcText(r result) string {
	if r.err != nil {
		return cutRunes(r.err.Error(), 120)
	}
	return fmt.Sprint(r.rc)
}

func healthLoad(state string) map[string]*hItem {
	m := map[string]*hItem{}
	if b, err := os.ReadFile(state); err == nil {
		if json.Unmarshal(b, &m) != nil {
			m = map[string]*hItem{} // 깨진 기록은 없는 것으로(처음 본 나쁨을 다시 알린다 — 놓치지 않는 쪽)
		}
	}
	return m
}

func healthSave(state string, m map[string]*hItem) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(state), 0o755); err != nil {
		return err
	}
	return atomic.WriteFile(state, append(b, '\n'), 0o644)
}

// hStamp: 키 글자(hc-lane report --key: A-Za-z0-9_.:/-) 안의 시각 — 초 아래까지(같은 초의 회복·나빠짐이 걸러지지 않게)
func hStamp(t time.Time) string { return t.Format("2006-01-02T15:04:05.000000") }

// healthCheck: 한 바퀴. report(msg, key)는 사건 한 줄. 찍을 줄들을 돌려준다
func healthCheck(machine, state string, dry bool, r runner, report func(msg, key string) bool) []string {
	var lines []string
	m := healthLoad(state)
	now := time.Now()
	changed := false
	for _, p := range hProbes {
		st, detail := p.probe(r)
		it := m[p.key]
		if st == "" { // 모름 — 사건 없음, 앞 상태 유지
			if it == nil {
				it = &hItem{}
				m[p.key] = it
			}
			it.Unk, it.UnkAt = detail, now.Format(time.RFC3339)
			changed = true
			lines = append(lines, fmt.Sprintf("건강 점검 %s: 모름(%s) — 다음 바퀴에", p.label, detail))
			continue
		}
		if it == nil || it.Since == "" { // 첫 기록: 키 시각을 먼저 남겨 알림이 실패해도 다음 바퀴가 같은 키로
			if it == nil {
				it = &hItem{}
				m[p.key] = it
			}
			it.Since = hStamp(now)
			changed = true
		}
		it.Unk, it.UnkAt = "", ""
		send := st != it.State && (st == "bad" || it.State == "bad") // 처음 본 좋음은 조용히, 처음 본 나쁨은 알린다
		if !send {
			it.State, it.Detail, it.Checked = st, detail, now.Format(time.RFC3339)
			changed = true
			continue
		}
		msg := fmt.Sprintf("%s(%s): %s — %s", p.bad, machine, detail, "알리기만 함")
		if st == "good" {
			msg = fmt.Sprintf("%s(%s): %s", p.good, machine, detail)
		}
		if dry {
			lines = append(lines, "건강 점검 (보낼 것): "+msg)
			continue
		}
		key := "hc-health:" + machine + ":" + p.key + ":" + st + ":" + it.Since
		if !report(msg, key) {
			lines = append(lines, "건강 점검: "+msg+" — 알림 실패(hc-lane report), 다음 바퀴에 같은 키로 다시")
			continue
		}
		lines = append(lines, "건강 점검: "+msg)
		it.State, it.Detail, it.Since, it.Checked = st, detail, hStamp(now), now.Format(time.RFC3339)
		changed = true
	}
	if changed && !dry {
		if err := healthSave(state, m); err != nil {
			lines = append(lines, "건강 점검 기록을 못 씀: "+err.Error())
		}
	}
	return lines
}

func healthState() string { return filepath.Join(stateHome(), "hc-sweep", "health") }

func healthOn() bool { return runtime.GOOS == "darwin" && os.Getenv("HC_SWEEP_HEALTH") != "off" }

// healthRun: 한 바퀴(macOS만) — 잠자기 끔·키체인, 상주 판 점검(resident.go, grace = 손잡이 sweep_resident_grace),
// 시험 고아 점검(testorphan.go, orphanAge = 손잡이 sweep_test_orphan)
func healthRun(dry bool, r runner, grace, orphanAge time.Duration) {
	if !healthOn() {
		return
	}
	machine := sweepMachine()
	lane := filepath.Join(binDir, "hc-lane")
	report := func(msg, key string) bool { return laneReport(r, lane, "hc-health", msg, key) }
	for _, l := range healthCheck(machine, healthState(), dry, r, report) {
		fmt.Println(l)
	}
	stale := func(msg, key string) bool { return laneReport(r, lane, "hc-stale-resident", msg, key) }
	for _, l := range residentCheck(machine, resident.DirFor(os.Getenv("XDG_STATE_HOME"), home), dry, r, stale, time.Now(), grace) {
		fmt.Println(l)
	}
	orphan := func(msg, key string) bool { return laneReport(r, lane, "hc-test-orphan", msg, key) }
	for _, l := range testOrphanCheck(machine, filepath.Join(stateHome(), "hc-sweep", "test-orphans"), dry, r, orphan, time.Now(), orphanAge) {
		fmt.Println(l)
	}
}

// healthPrint: hc-sweep --health — 마지막 점검 결과(재지 않는다, 기록만 읽음). 0 모두 좋음 · 1 나쁨이 있음 · 3 기록 없음·모름
func healthPrint(state string, now time.Time) int {
	m := healthLoad(state)
	if len(m) == 0 {
		fmt.Println("건강 점검 기록 없음(" + state + ") — hc-sweep이 아직 이 기계에서 돌지 않았거나 macOS가 아니다")
		return 3
	}
	rc := 0
	ago := func(s string) string {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return "?"
		}
		return now.Sub(t).Round(time.Minute).String() + " 전"
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		label := k
		for _, p := range hProbes {
			if p.key == k {
				label = p.label
			}
		}
		it := m[k]
		st := map[string]string{"good": "좋음", "bad": "나쁨"}[it.State]
		switch {
		case st == "":
			st, rc = "모름", max(rc, 3)
		case it.State == "bad":
			rc = 1
		}
		l := fmt.Sprintf("%s: %s", label, st)
		if it.Detail != "" {
			l += "(" + it.Detail + ")"
		}
		if it.Checked != "" {
			l += " · 확인 " + ago(it.Checked)
		}
		if it.Unk != "" {
			l += " · 마지막 바퀴 모름(" + it.Unk + ", " + ago(it.UnkAt) + ")"
		}
		fmt.Println(l)
	}
	return rc
}
