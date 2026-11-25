package main

import (
	"os"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// ht-config-knobs: 손잡이 저장 파일·원격 참조가 없는 기계(워커)도 herdr-sync로 맞춘 설정 파일 값을 쓰고, 지휘 기계의 손잡이는 그 위에
func TestSweepKnobsConfFile(t *testing.T) {
	oh, ob := home, binDir
	t.Cleanup(func() { home, binDir = oh, ob })
	home, binDir = t.TempDir(), t.TempDir()
	cd := t.TempDir()
	t.Setenv("HERDR_TOOLS_CONFIG", cd)
	if err := os.WriteFile(cd+"/"+knob.ConfName, []byte("sweep_r1 45 # 워커는 느슨히\nsweep_r3 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if l := sweepKnobs(now); l != (limits{45, 30, 360}) { // r3는 범위 밖이라 버리고 기본
		t.Fatalf("설정 파일 = %+v", l)
	}
	if err := knob.Set(knob.Path(home), knob.Entry{Name: "sweep_r1", Value: "5", By: "t", Why: "t", At: now.Unix()}, false, now); err != nil {
		t.Fatal(err)
	}
	if l := sweepKnobs(now); l != (limits{5, 30, 360}) {
		t.Fatalf("손잡이가 설정 파일을 덮지 않음 = %+v", l)
	}
}
