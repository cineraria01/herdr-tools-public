//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ht-fill-smalls-1128 ②: 레인을 멈춘(pause@) 기계 — 부하는 loadTick이 안 읽어 「부하 모름」·며칠 전 값이었다. 격리 점검(cordonTick)이
// 2분마다 같은 조각(부하 한 줄 포함)을 읽으니 그 출력의 부하도 같은 자리(LOADS → load.tsv)에 적는다. 새 원격 호출·배정 판단은 그대로

func TestCordonTickRecordsPausedLoad(t *testing.T) {
	c := ctFix(t)
	if err := knobSet([]string{"pause@b", "1", "--why", "병합 검증 전용"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	c.re()
	LOADS = nil
	loadTick() // 기다리는 줄 없음·멈춤 — 부하를 안 읽는다
	c.tick(0)
	calls, _ := os.ReadFile(c.cd + "/exec.calls")
	if got := strings.Count(string(calls), "b\n"); got != 1 {
		t.Fatalf("원격 호출은 격리 점검 한 번 그대로: %q", calls)
	}
	if l := LOADS["b"]; !l.OK || !l.VerOK || l.Cores <= 0 {
		t.Fatalf("점검 한 번 뒤 b의 부하·검증 수가 보임: %+v", LOADS["b"])
	}
	loadEnd(time.Now())
	line := func() string {
		for _, l := range machineLines(true) {
			if strings.HasPrefix(l, "b  ") {
				return l
			}
		}
		return ""
	}
	if l := line(); strings.Contains(l, "부하 모름") || !strings.Contains(l, "0m 전") || !strings.Contains(l, "멈춤") {
		t.Fatalf("멈춘 b의 기계 줄: %q", l)
	}
	// 띄울 기계 고르기 그대로 — 멈춘 b는 부하가 보여도 안 고른다
	MWHY = map[string]string{}
	LOADS["a"] = mLoad{L1: 9, Cores: 8, OK: true}
	LOADS["b"] = mLoad{L1: 0.1, Cores: 8, OK: true}
	NOW, dry = nil, true
	place("l1", "p", DATA+"/brief.md", "-", "-", "-", "")
	dry = false
	if got := strings.Join(NOW, ","); got != "a" || MWHY["b"] != "멈춤(조정)" {
		t.Fatalf("멈춘 b에 안 띄움: %q %q", got, MWHY["b"])
	}
	// 다음 점검의 호출이 실패(모름) — 앞 값 그대로(읽은 때와 함께), 모름으로 덮지 않음
	prev := loadRecs()["b"].l
	os.WriteFile(c.cd+"/b.rc", []byte("1"), 0o644)
	LOADS = nil
	c.tick(10 * 60)
	if _, ok := LOADS["b"]; ok {
		t.Fatalf("실패한 호출은 부하를 안 적음: %+v", LOADS["b"])
	}
	loadEnd(time.Now())
	if got := loadRecs()["b"].l; !got.OK || got.At != prev.At || got.L1 != prev.L1 {
		t.Fatalf("모름이면 앞 값 그대로: 전 %+v 뒤 %+v", prev, got)
	}
}
