package main

// ht-alert-sources(2026-11-28): 병합기가 내는 막힘은 그 상태가 끝날 때 같은 이름(merger)·같은 열쇠 + :up 풀림을 한 번 적는다 — 한 문이
// 「아직 안 풀렸나」를 어림하지 않게. 같은 까닭 n바퀴 쉼 ↔ 쉼 끝(또는 까닭 바뀜), 시작 단계 n번 모름 ↔ 그 단계 됨, 병합기 멈춤 ↔ 다시 돎.
// 풀림 쓰기가 실패하면 다음 바퀴에 다시(되풀이 없이)

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

var upKeyRe = regexp.MustCompile(`\|--key\|([^|]+)\|`)

func upKeys(b string) []string {
	var o []string
	for _, m := range upKeyRe.FindAllStringSubmatch(b, -1) {
		o = append(o, m[1])
	}
	return o
}

func upFx(t *testing.T) (*M, string, string) {
	f := newFx(t)
	m := f.merger()
	failf, log := f.dir+"/failreport", f.dir+"/reports"
	os.WriteFile(gSelf, []byte("#!/bin/sh\n[ -e "+failf+" ] && exit 1\nprintf '%s|' \"$@\" >>"+log+"\necho >>"+log+"\n"), 0o777)
	return m, failf, log
}

func readS2(p string) string { b, _ := os.ReadFile(p); return string(b) }

// 쉼 막힘 → 쉼이 끝난 바퀴에 같은 열쇠 + :up 중간 한 번. 풀림 쓰기 실패 → 다음 바퀴에 다시, 그 뒤 조용
func TestDeferUp(t *testing.T) {
	m, failf, log := upFx(t)
	os.WriteFile(m.mj+"/defer", []byte(fmt.Sprintf("%d\t까닭\t1700\t-\n", deferAlarm-1)), 0o666)
	m.deferred = "까닭"
	m.deferNote()
	m.deferred = ""
	os.WriteFile(failf, nil, 0o666)
	m.deferNote() // 쉼 끝 — 풀림 쓰기 실패
	os.Remove(failf)
	m.deferNote() // 다음 바퀴에 다시
	m.deferNote() // 조용
	b := readS2(log)
	ks := upKeys(b)
	if strings.Count(b, "report|막힘|") != 1 || strings.Count(b, "report|중간|") != 1 || len(ks) != 2 {
		t.Fatalf("막힘 한 번·풀림 한 번: %s", b)
	}
	if !strings.HasPrefix(ks[0], "merger:defer:1700:") || ks[1] != ks[0]+":up" {
		t.Fatalf("풀림 열쇠 = 막힘 열쇠 + :up: %q", ks)
	}
	if fileExists(m.mj + "/defer") {
		t.Fatal("풀림을 적은 뒤 쉼 기록을 지운다")
	}
}

// 까닭이 바뀜 → 옛 까닭의 풀림 한 번, 새 까닭은 처음부터. 알리지 않은(문턱 전) 쉼이 끝나면 풀림 없음(반대 장면)
func TestDeferUpReasonChange(t *testing.T) {
	m, _, log := upFx(t)
	os.WriteFile(m.mj+"/defer", []byte(fmt.Sprintf("%d\t옛 까닭\t1700\tsent\n", deferAlarm)), 0o666)
	m.deferred = "새 까닭"
	m.deferNote()
	if b := readS2(log); strings.Count(b, "report|중간|") != 1 || !strings.Contains(b, "|--key|merger:defer:1700:") {
		t.Fatalf("옛 까닭의 풀림 한 번: %s", b)
	}
	if c := readS2(m.mj + "/defer"); !strings.HasPrefix(c, "1\t새 까닭\t") {
		t.Fatalf("새 까닭은 처음부터: %q", c)
	}
	m2, _, log2 := upFx(t)
	os.WriteFile(m2.mj+"/defer", []byte("3\t까닭\t1700\t-\n"), 0o666)
	m2.deferred = ""
	m2.deferNote()
	if b := readS2(log2); b != "" {
		t.Fatalf("알리지 않은 쉼에는 풀림도 없음: %s", b)
	}
}

// 병합기 멈춤 막힘은 열쇠와 표식을 남기고, 다시 돈 상주가 같은 열쇠 + :up 한 번(표식을 지운다). 못 적으면 다음 바퀴에 다시
func TestStopUp(t *testing.T) {
	_, failf, log := upFx(t)
	top := t.TempDir()
	stopNote(top, "병합기 멈춤: 시험 (클론 x)")
	b := readS2(log)
	ks := upKeys(b)
	if strings.Count(b, "report|막힘|") != 1 || len(ks) != 1 || !strings.HasPrefix(ks[0], "hc-merger-stop:") {
		t.Fatalf("멈춤 막힘에 열쇠: %s", b)
	}
	os.WriteFile(failf, nil, 0o666)
	stopUp(top)
	os.Remove(failf)
	stopUp(top)
	stopUp(top)
	b = readS2(log)
	if ks = upKeys(b); strings.Count(b, "report|중간|") != 1 || len(ks) != 2 || ks[1] != ks[0]+":up" {
		t.Fatalf("다시 돎 풀림 한 번(실패 뒤 다시): %s", b)
	}
}

// 시작 단계 n번 모름 막힘 ↔ 그 단계가 된 뒤 같은 열쇠 + :up. 문턱 전에 됐으면 풀림 없음
func TestStartUp(t *testing.T) {
	_, _, log := upFx(t)
	n := deferAlarm
	startDone("클론 HEAD 분리", n)
	startDone("클론 HEAD 분리", deferAlarm-1)
	b := readS2(log)
	if ks := upKeys(b); strings.Count(b, "report|중간|") != 1 || len(ks) != 1 || ks[0] != startKey("클론 HEAD 분리")+":up" {
		t.Fatalf("문턱을 넘은 단계만 풀림 한 번: %s", b)
	}
}
