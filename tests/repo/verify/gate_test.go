//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ht-merger-gate-slot ⑥(계약 docs/hc-lane/merger-spec-verify.md 「병합 검증 자리」): HERDR_VERIFY_GATE=1이면 레인 검증 잠금(verify.lock)이
// 아니라 같은 폴더의 병합 검증 잠금(verify-gate.lock — 기계마다 한 번에 하나)을 쥔다. 레인이 쥔 잠금을 기다리지 않고, 다른 병합 검증이 쥔
// 잠금은 기다린다. 값이 없거나 1이 아니면 지금 그대로
func TestGateLock(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	lane := lockPath()
	gate := filepath.Join(filepath.Dir(lane), "verify-gate.lock")
	for v, want := range map[string]string{"": lane, "0": lane, "1": gate} {
		t.Setenv("HERDR_VERIFY_GATE", v)
		if got := lockFor(); got != want {
			t.Errorf("HERDR_VERIFY_GATE=%q: 잠금 %s, want %s", v, got, want)
		}
	}
	t.Setenv("HERDR_VERIFY_GATE", "1")
	held := hold(t, lane) // 레인 검증이 쥠 → 병합 검증은 기다리지 않음
	defer held.Close()
	f, read := outFile(t)
	t0 := time.Now()
	release, rc := verifyLock(lockFor(), time.Minute, make(chan os.Signal), f)
	if rc != 0 || time.Since(t0) > 5*time.Second || waitedRe.MatchString(read()) {
		t.Fatalf("레인 잠금을 기다리지 않아야 rc=%d %v\n%s", rc, time.Since(t0), read())
	}
	release()
	gh := hold(t, gate) // 다른 병합 검증이 쥠 → 기다림
	unhook := releaseOnBlock(t, gh)
	f2, read2 := outFile(t)
	release, rc = verifyLock(lockFor(), time.Minute, make(chan os.Signal), f2)
	unhook()
	if rc != 0 || !waitedRe.MatchString(read2()) {
		t.Fatalf("병합 검증 잠금은 기다려야 rc=%d\n%s", rc, read2())
	}
	release()
}

// 감사 atp3qsv 중간 2: 병합 검증 자리의 무거운 조각은 레인 검증과 함께 돌지만, 판정을 정하는 시간 시험(홀로 단계)과 혼자 다시 돌기는
// 레인 잠금을 기다려 쥐고 돈다 — 레인 검증의 조각과 겹친 채 판정이 나지 않게
func TestGateTimedStepsWaitLaneLock(t *testing.T) {
	H := t.TempDir()
	t.Setenv("HOME", H)
	t.Setenv("XDG_CACHE_HOME", H+"/.cache")
	held := hold(t, lockPath()) // 레인 검증이 도는 중
	releaseOnBlock(t, held)
	rc, out, calls, again := fakeVerify(t, map[string]string{"HOME": H, "XDG_CACHE_HOME": H + "/.cache", "HERDR_VERIFY_NO_LOCK": "",
		"HERDR_VERIFY_GATE": "1", "FAKE_FAIL": "run \\^TestCotenantGuard", "FAKE_MODE": "flaky"})
	if rc != 0 || strings.Count(again, "\n") != 1 {
		t.Fatalf("rc=%d again=%q\n%s", rc, again, out)
	}
	w, a, sh := waitedRe.FindStringIndex(out), strings.Index(out, "-- [홀로]"), strings.Index(out, "-- [")
	if w == nil || sh < 0 || a < 0 || !(sh < w[0] && w[0] < a) {
		t.Fatalf("조각은 레인 잠금 없이 먼저, 홀로 단계는 레인 잠금을 기다린 뒤(조각 %d 기다림 %v 홀로 %d)\n%s\n%s", sh, w, a, out, calls)
	}
}

// 병합 검증 자리의 홀로 단계(cotenant — 1분 부하를 기다리는 시험)는 조각 뒤에: 함께 도는 레인 검증의 부하로 먼저 오래 기다리지 않게.
// 값이 없으면 지금처럼 맨 앞, 실패하면 다시 돌기는 그대로
func TestGateAloneAfterShards(t *testing.T) {
	order := func(calls string) (alone, firstShard int) {
		alone, firstShard = -1, -1
		for i, l := range strings.Split(strings.TrimSpace(calls), "\n") {
			switch {
			case strings.Contains(l, "TestCotenantGuard") && strings.Contains(l, "-run"):
				if alone < 0 {
					alone = i
				}
			case strings.HasPrefix(l, "test ") && !strings.Contains(l, "./tests/repo/") && firstShard < 0:
				firstShard = i
			}
		}
		return
	}
	_, _, calls, _ := fakeVerify(t, nil)
	if a, s := order(calls); a < 0 || s < 0 || a > s {
		t.Fatalf("값 없음: 홀로 단계가 조각 앞(홀로 %d 조각 %d)\n%s", a, s, calls)
	}
	rc, out, calls, again := fakeVerify(t, map[string]string{"HERDR_VERIFY_GATE": "1", "FAKE_FAIL": "run \\^TestCotenantGuard", "FAKE_MODE": "flaky"})
	if a, s := order(calls); rc != 0 || a < 0 || s < 0 || a < s {
		t.Fatalf("병합 검증 자리: 홀로 단계가 조각 뒤 rc=%d(홀로 %d 조각 %d)\n%s\n%s", rc, a, s, calls, out)
	}
	if strings.Count(again, "\n") != 1 || !strings.Contains(out, "혼자 다시 돌려 통과") {
		t.Fatalf("병합 검증 자리: 홀로 단계 흔들림은 그대로 한 번 다시 again=%q\n%s", again, out)
	}
}
