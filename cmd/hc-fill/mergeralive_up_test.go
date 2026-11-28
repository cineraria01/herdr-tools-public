package main

// ht-alert-sources(2026-11-28): 병합기 터미널 없음 막힘과 그 「다시 돎」이 같은 열쇠(+ :up)로 짝 — 한 문(hc-lane gate.go opsKeyDone)이
// 풀림으로 본다. 주인이 여럿이어도 사건은 한 줄(--to 주인 차례 — 받는 쪽은 문이 정한다), 풀림 쓰기가 실패하면 다음 바퀴에 다시 한 번

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var maKeyRe = regexp.MustCompile(`--key (\S+)`)

func maKeys(ev string) []string {
	var o []string
	for _, m := range maKeyRe.FindAllStringSubmatch(ev, -1) {
		o = append(o, m[1])
	}
	return o
}

func maQrow(name, owner, addr string) string {
	return strings.Join([]string{name, "app", "b", "-", "-", "up", "m2", addr, "-", "t", "-", owner, "normal", "-", "auto", "-", "r-" + name}, "\t")
}

// 표 1: 막힘(dead) → 다시 돎은 같은 열쇠 + :up, 주인 둘이어도 사건 한 줄씩
func TestMergerAliveUpSameKey(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute)+maQ("w2", 19*time.Minute), "")
	os.WriteFile(Q, []byte(maQrow("w1", "t0", "m2/t5")+"\n"+maQrow("w2", "t1", "m2/t6")+"\n"), 0o644)
	TERMS = nil
	maTick(2)
	TERMS = []Term{maMerger()}
	maTick(2)
	e := ev()
	ks := maKeys(e)
	if strings.Count(e, "report 막힘") != 1 || strings.Count(e, "report 중간") != 1 || len(ks) != 2 {
		t.Fatalf("막힘 한 줄·다시 돎 한 줄(주인마다 따로 적지 않음): %q", e)
	}
	if !strings.HasPrefix(ks[0], "hf:merger:app:dead:") || ks[1] != ks[0]+":up" {
		t.Errorf("다시 돎 열쇠 = 막힘 열쇠 + :up: %q", ks)
	}
	if !strings.Contains(e, "--to t0,t1") {
		t.Errorf("주인 차례를 한 줄의 --to로: %q", e)
	}
}

// 반대 장면: 풀리지 않았으면(터미널 계속 없음) 다시 돎 없음. 재발(풀림 뒤 다시 없음) → 새 열쇠로 한 번
func TestMergerAliveUpRecur(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
	os.WriteFile(Q, []byte(maQrow("w1", "t0", "m2/t5")+"\n"), 0o644)
	TERMS = nil
	maTick(3)
	if e := ev(); strings.Contains(e, "report 중간") {
		t.Fatalf("안 풀렸는데 다시 돎: %q", e)
	}
	TERMS = []Term{maMerger()}
	maTick(1)
	TERMS = nil
	time.Sleep(1100 * time.Millisecond) // 새 상태 시작 초가 달라지게
	maTick(2)
	ks := maKeys(ev())
	if len(ks) != 3 || ks[2] == ks[0] || !strings.HasPrefix(ks[2], "hf:merger:app:dead:") {
		t.Fatalf("재발은 새 열쇠로 한 번: %q", ks)
	}
}

// ④ 풀림 사건 쓰기가 실패 → 다음 바퀴에 다시(되풀이 없이 — 그다음 바퀴는 조용)
func TestMergerAliveUpRetry(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
	os.WriteFile(Q, []byte(maQrow("w1", "t0", "m2/t5")+"\n"), 0o644)
	TERMS = nil
	maTick(2)
	TERMS = []Term{maMerger()}
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf 'FAIL %s\\n' \"$*\" >>'"+DATA+"/ev.log'\nexit 1\n"), 0o755)
	maTick(1)
	os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+DATA+"/ev.log'\n"), 0o755)
	maTick(3)
	e := ev()
	if strings.Count(e, "FAIL report 중간") != 1 || strings.Count(e, "\nreport 중간") != 1 {
		t.Fatalf("실패 한 번 뒤 다음 바퀴에 한 번만: %q", e)
	}
	ks := maKeys(e)
	if ks[len(ks)-1] != ks[0]+":up" {
		t.Errorf("다시 낸 풀림도 같은 열쇠: %q", ks)
	}
}

// 옛 기록(알림 칸 「,dead」 — 시각 없음)으로 판이 올라감 → 그 상태 시작으로 열쇠를 짓는다(옛 막힘 열쇠와 다를 수 있다 — 그 짝은 한 문의 묵음이 거른다)
func TestMergerAliveUpLegacyRecord(t *testing.T) {
	ev, _ := maFx(t, maQ("w1", 20*time.Minute), "")
	os.WriteFile(DATA+"/merger-alive.tsv", []byte(strings.Join([]string{"app", "m2", "gone", "1700000000", "-", "1", "-", "1700000000", ",dead", "-", "w1", "w1", "-", "-"}, "\t")+"\n"), 0o644)
	TERMS = []Term{maMerger()}
	maTick(1)
	if ks := maKeys(ev()); len(ks) != 1 || ks[0] != "hf:merger:app:dead:1700000000:up" {
		t.Fatalf("옛 기록의 다시 돎 열쇠: %q", ks)
	}
}
