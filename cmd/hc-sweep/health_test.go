package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 워커 건강 점검(잠자기 끔·키체인) — 가짜 pmset·security. 상태가 바뀔 때만 사건, 점검 실패는 모름(사건 없음)

const (
	pmOn    = " SleepDisabled\t\t1\n sleep                0\n"
	pmOff   = " SleepDisabled\t\t0\n sleep                1\n"
	pmSleep = " sleep                0\n" // 잠자기 끔은 아니어도 sleep 0이면 통과(worker-mac-check와 같게)
)

// fakeHealth: 명령 이름 → 결과
type fakeHealth struct{ res map[string]result }

func (f *fakeHealth) run(_ time.Duration, _ []string, name string, _ ...string) result {
	return f.res[filepath.Base(name)]
}

func hcCheck(t *testing.T, f *fakeHealth, state string) []string {
	t.Helper()
	var sent []string
	healthCheck("worker-a", state, false, f.run, func(msg, key string) bool {
		sent = append(sent, msg+"|"+key)
		return true
	})
	return sent
}

func kcOK() result     { return result{} }
func kcLocked() result { return result{rc: 36} }

// ① 잠자기 끔이 꺼짐으로 바뀜 → 사건 1번, 다음 바퀴에 되풀이 안 함 ② 회복 → 회복 사건
func TestHealthSleepFlipOnceAndRecover(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOn}, "security": kcOK()}}
	if s := hcCheck(t, f, st); len(s) != 0 {
		t.Fatalf("처음 좋음인데 사건: %v", s)
	}
	f.res["pmset"] = result{out: pmOff}
	s := hcCheck(t, f, st)
	if len(s) != 1 || !strings.Contains(s[0], "worker-a") || !strings.Contains(s[0], "잠자기") {
		t.Fatalf("나빠짐 사건 1번이어야: %v", s)
	}
	if s := hcCheck(t, f, st); len(s) != 0 {
		t.Fatalf("같은 나쁨을 되풀이: %v", s)
	}
	f.res["pmset"] = result{out: pmSleep}
	s = hcCheck(t, f, st)
	if len(s) != 1 || !strings.Contains(s[0], "회복") {
		t.Fatalf("회복 사건이어야: %v", s)
	}
	if s := hcCheck(t, f, st); len(s) != 0 {
		t.Fatalf("회복을 되풀이: %v", s)
	}
}

// ③ 키체인 막힘 → 사건
func TestHealthKeychainLocked(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOn}, "security": kcOK()}}
	hcCheck(t, f, st)
	f.res["security"] = kcLocked()
	s := hcCheck(t, f, st)
	if len(s) != 1 || !strings.Contains(s[0], "키체인") {
		t.Fatalf("키체인 막힘 사건 1번이어야: %v", s)
	}
}

// ④ 점검 명령 실패(못 띄움·시간 초과·모르는 종료 코드·읽을 수 없는 출력) → 모름, 사건 없음, 앞 상태를 지우지 않는다
func TestHealthProbeFailUnknown(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOn}, "security": kcOK()}}
	hcCheck(t, f, st)
	for _, bad := range []map[string]result{
		{"pmset": {rc: -1, err: errors.New("pmset: 시간 초과")}, "security": {rc: -1, err: errors.New("없음")}},
		{"pmset": {rc: 1}, "security": {rc: 51}},
		{"pmset": {out: "알 수 없는 출력\n"}, "security": kcOK()},
	} {
		f.res = bad
		if s := hcCheck(t, f, st); len(s) != 0 {
			t.Fatalf("모름을 나쁨으로 알림: %v (%v)", s, bad)
		}
	}
	// 모름 사이에 앞 상태(좋음)가 남아 있어야 다음 나빠짐을 알린다
	f.res = map[string]result{"pmset": {out: pmOff}, "security": kcOK()}
	if s := hcCheck(t, f, st); len(s) != 1 {
		t.Fatalf("모름 뒤 나빠짐 사건 1번이어야: %v", s)
	}
}

// 처음 본 상태가 나쁨이면 알린다(이미 나빠진 기계에 점검이 처음 깔린 때를 놓치지 않는다)
func TestHealthFirstSeenBad(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOff}, "security": kcLocked()}}
	if s := hcCheck(t, f, st); len(s) != 2 {
		t.Fatalf("처음부터 나쁜 둘을 알려야: %v", s)
	}
}

// 알림이 실패하면 상태를 안 바꿔 다음 바퀴에 같은 키로 다시 보낸다(hc-lane이 같은 키는 한 번만 적는다)
func TestHealthReportFailRetrySameKey(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOn}, "security": kcOK()}}
	hcCheck(t, f, st)
	f.res["pmset"] = result{out: pmOff}
	var keys []string
	healthCheck("worker-a", st, false, f.run, func(_, key string) bool { keys = append(keys, key); return false })
	s := hcCheck(t, f, st)
	if len(keys) != 1 || len(s) != 1 || !strings.HasSuffix(s[0], "|"+keys[0]) || keys[0] == "" {
		t.Fatalf("실패 뒤 같은 키로 다시 보내야: %v %v", keys, s)
	}
}
