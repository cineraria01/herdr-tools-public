//go:build !windows

package main

import (
	"os"
	"testing"
	"time"
)

// knobs-migrate ⑤: 앞 레인들이 따로 만든 스위치가 knob ls 한 화면에 보이고, 공통 저장 값이 옛 자리보다 앞선다.
// 병합기 살아 있음(옛 표식 파일 DATA/merger_*), 기계 자리·멈춤(seats@·pause@), 대기열 제출(auto_submit), 옮긴 설정 키(gc 등)
func TestKnobMigratedSwitchesListed(t *testing.T) {
	reround := knobFix(t)
	confLines = []string{"local here", "gc 45", "machine box-1 3 - -"}
	rows := func() map[string]knobRow {
		m := map[string]knobRow{}
		for _, r := range knobRows(time.Now()) {
			m[r.Name] = r
		}
		return m
	}
	// 옛 표식 파일만 있을 때: 값이 그대로 먹고 ls에 「설정」으로 보인다
	if err := os.WriteFile(DATA+"/merger_restart", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DATA+"/merger_dead", []byte("7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := rows()
	for _, c := range []struct{ name, val, src string }{
		{"merger_restart", "1", "설정"}, {"merger_dead", "7", "설정"}, {"merger_unknown", "30", "기본"},
		{"gc", "45", "설정"}, {"idle_empty", "0", "기본"}, {"reland_wait", "30", "기본"}, {"auto_submit", "-", "기본"},
	} {
		if r[c.name].Value != c.val || r[c.name].Source != c.src {
			t.Errorf("ls %s = %+v — 바람 %s(%s)", c.name, r[c.name], c.val, c.src)
		}
	}
	if !maRestart() || maKnob("merger_dead", 5) != 7 {
		t.Fatalf("옛 표식 파일: restart %v dead %d", maRestart(), maKnob("merger_dead", 5))
	}
	for _, name := range []string{"merger_batch", "merger_conflict", "merger_busy_max", "sweep_r1", "sweep_r3"} {
		if r[name].Tool == "" || !r[name].Remote {
			t.Errorf("ls에 원격 손잡이 %s가 없다: %+v", name, r[name])
		}
	}
	// 공통 저장 값이 옛 자리·설정보다 앞선다(다음 바퀴에 먹음)
	now := time.Now()
	for _, a := range [][]string{
		{"merger_restart", "0", "--why", "끔"}, {"merger_dead", "12", "--why", "느슨히"}, {"gc", "90", "--why", "덜 자주"},
		{"seats@box-1", "1", "--why", "줄임"},
	} {
		if err := knobSet(a, now); err != nil {
			t.Fatal(err)
		}
	}
	reround()
	r = rows()
	if maRestart() || maKnob("merger_dead", 5) != 12 || conf("gc") != "90" {
		t.Fatalf("손잡이가 안 앞섬: restart %v dead %d gc %q", maRestart(), maKnob("merger_dead", 5), conf("gc"))
	}
	if r["merger_restart"].Source != "손잡이" || r["seats@box-1"].Value != "1" || r["seats@box-1"].Source != "손잡이" {
		t.Fatalf("ls: restart %+v seats %+v", r["merger_restart"], r["seats@box-1"])
	}
	// 범위 밖은 거절(도구를 망가뜨리는 값)
	for _, a := range [][]string{{"merger_tick", "0", "--why", "x"}, {"merger_batch", "0", "--why", "x"}, {"sweep_r1", "0", "--why", "x"}, {"merger_restart", "2", "--why", "x"}} {
		if err := knobSet(a, now); err == nil {
			t.Errorf("범위 밖 %v를 받았다", a)
		}
	}
}
