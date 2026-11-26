//go:build !windows

package main

import (
	"strings"
	"testing"
	"time"
)

// ht-advise-slot-cap(2026-11-26): 빈 자리는 machRoom 하나로 센다 — 띄우기가 「레인상한 참」으로 못 띄운 기계를 바퀴 기록(load.tsv)도
// 빈 자리 0으로 적고, 상태 한 장 free·advise는 그 값만 읽는다. 고치기 전: advise가 레인·자리 칸으로 따로 세 「빈 자리 1」 → 거짓 free-wait
func TestSlotCapOneCount(t *testing.T) {
	lpFix(t)
	confLines = []string{"local a", "machine a 3 -", "machine b 5 2", "repo p a /r/a", "repo p b /r/b"}
	SEATS, NOWL = []string{"b", "b"}, []string{"b", "b"} // b: 에이전트 2/5, 레인 2/2
	LOADS, NOW, dry = map[string]mLoad{}, nil, true
	place("l1", "p", DATA+"/brief.md", "b", "-", "-", "")
	dry = false
	if len(NOW) != 0 || MWHY["b"] != "레인상한 참" {
		t.Fatalf("띄우기: NOW %q 까닭 %q", NOW, MWHY["b"])
	}
	if n, why := machRoom(Machine{Name: "b", Cap: "5", LCap: "2"}, false, 0); n != 0 || why != "레인상한 참" {
		t.Fatalf("machRoom: %d %q", n, why)
	}
	loadEnd(time.Now())
	if r := loadRecs()["b"]; !r.freeOK || r.free != 0 || r.why != "레인상한 참" {
		t.Fatalf("바퀴 기록 b: %+v", r)
	}
	if r := loadRecs()["a"]; !r.freeOK || r.free != 3 {
		t.Fatalf("바퀴 기록 a(자리 3·레인상한 없음): %+v", r)
	}

	// 레인 하나 빠지면 빈 자리 1(자리 5−2=3, 레인상한 2−1=1 가운데 작은 것)
	NOWL = []string{"b"}
	if n, why := machRoom(Machine{Name: "b", Cap: "5", LCap: "2"}, false, 0); n != 1 || why != "" {
		t.Fatalf("machRoom 레인 1/2: %d %q", n, why)
	}
	// 자동 감사 줄은 레인상한을 안 본다
	NOWL = []string{"b", "b"}
	if n, _ := machRoom(Machine{Name: "b", Cap: "5", LCap: "2"}, true, 0); n != 3 {
		t.Fatalf("자동 감사: %d", n)
	}
}

// 옛 판 load.tsv(빈 자리 칸 없음)는 모름 — 상태 한 장 free null, advise는 세지 않는다
func TestSlotCapOldLoadUnknown(t *testing.T) {
	advFx(t)
	wr := func(s string) {
		t.Helper()
		if err := writeAtomic(DATA+"/load.tsv", s); err != nil {
			t.Fatal(err)
		}
	}
	wr("m\t1.20\t12\t" + itoa(time.Now().Unix()-30) + "\t1\t3\t-\t0\t0\n")
	if r := loadRecs()["m"]; r.freeOK {
		t.Fatalf("옛 판인데 빈 자리 앎: %+v", r)
	}
	a := adviseNow(time.Now())
	for _, it := range a.Items {
		if it.Kind == "free-wait" {
			t.Errorf("빈 자리 모름인데 %s", it.Key)
		}
	}
	// 바퀴가 레인상한 참(빈 자리 0)으로 적었으면 free-wait 없음 — 레인 칸으로는 남아 보여도
	wr("m\t1.20\t12\t" + itoa(time.Now().Unix()-30) + "\t1\t3\t레인상한 참\t0\t0\t1\t0\n")
	for _, it := range adviseNow(time.Now()).Items {
		if strings.HasPrefix(it.Key, "free-wait:") || it.Kind == "chain" {
			t.Errorf("바퀴 판정 빈 자리 0인데 %s", it.Key)
		}
	}
}
