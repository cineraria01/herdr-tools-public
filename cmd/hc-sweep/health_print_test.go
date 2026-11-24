package main

import (
	"path/filepath"
	"testing"
	"time"
)

// hc-sweep --health: 기록만 읽는다 — 기록 없음 3, 모두 좋음 0, 나쁨 1, 모름만 남은 점검 3
func TestHealthPrintRC(t *testing.T) {
	st := filepath.Join(t.TempDir(), "health")
	now := time.Now()
	if rc := healthPrint(st, now); rc != 3 {
		t.Fatalf("기록 없음 rc %d", rc)
	}
	f := &fakeHealth{res: map[string]result{"pmset": {out: pmOn}, "security": kcOK()}}
	hcCheck(t, f, st)
	if rc := healthPrint(st, now); rc != 0 {
		t.Fatalf("모두 좋음 rc %d", rc)
	}
	f.res["security"] = kcLocked()
	hcCheck(t, f, st)
	if rc := healthPrint(st, now); rc != 1 {
		t.Fatalf("나쁨 rc %d", rc)
	}
	st2 := filepath.Join(t.TempDir(), "health")
	hcCheck(t, &fakeHealth{res: map[string]result{"pmset": {rc: 1}, "security": {rc: 51}}}, st2)
	if rc := healthPrint(st2, now); rc != 3 {
		t.Fatalf("모름만 rc %d", rc)
	}
}
