//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// ht-config-knobs: 개인 설정 폴더의 손잡이 설정 파일 — 설정 파일 > hc-fill.tsv 줄, 손잡이 > 설정 파일, ls 출처 「설정 파일 <경로>:<줄>」,
// save가 손잡이 값을 파일에 쓰고 손잡이를 지움, 원격 참조에 설정 파일 값(프로젝트 몫)도 실림
func TestKnobConfFile(t *testing.T) {
	reround := knobFix(t)
	cd := t.TempDir()
	t.Setenv("HERDR_TOOLS_CONFIG", cd)
	cp := cd + "/" + knob.ConfName
	confLines = []string{"local here", "idle 15", "hold 25", "machine box-1 3 - -"}
	if err := os.WriteFile(cp, []byte("# 정해 둔 값\nidle 20 # 지휘가 보기에 10분은 잦다\nseats@box-1 2\nno_such 1\ngc 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reround()
	rows := func() map[string]knobRow {
		m := map[string]knobRow{}
		for _, r := range knobRows(time.Now()) {
			m[r.Name] = r
		}
		return m
	}
	if conf("idle") != "20" || conf("hold") != "25" || conf("gc") != "" || knobInt("gc") != 60 {
		t.Fatalf("층: idle %q(설정 파일 20) hold %q(옛 설정 25) gc %q·%d(범위 밖 → 기본 60)", conf("idle"), conf("hold"), conf("gc"), knobInt("gc"))
	}
	if ms := machines(); len(ms) != 1 || ms[0].Cap != "2" {
		t.Fatalf("묶음 손잡이 seats@box-1이 설정 파일에서 안 먹음: %+v", ms)
	}
	r := rows()
	if x := r["idle"]; x.Source != string(knob.FromFile) || x.Where != cp+":2" || x.Kind != kindFixed || x.Why != "지휘가 보기에 10분은 잦다" {
		t.Fatalf("ls idle = %+v", x)
	}
	if x := r["seats@box-1"]; x.Value != "2" || x.Source != string(knob.FromFile) {
		t.Fatalf("ls seats@box-1 = %+v", x)
	}
	if x := r["hold"]; x.Source != string(knob.FromConf) {
		t.Fatalf("ls hold = %+v (옛 설정)", x)
	}

	// 손잡이(잠깐 바꾼 것)가 설정 파일을 덮는다
	now := time.Now()
	if err := knobSet([]string{"idle", "40", "--why", "밤사이 조용히", "--for", "30"}, now); err != nil {
		t.Fatal(err)
	}
	reround()
	if conf("idle") != "40" || rows()["idle"].Kind != kindTemp {
		t.Fatalf("손잡이가 안 앞섬: %q %+v", conf("idle"), rows()["idle"])
	}

	// save: 설정 파일의 같은 이름 줄 자리를 바꾸고 손잡이는 지운다 — 먹는 값은 그대로
	if err := knobSave([]string{"idle", "--why", "밤에도 40"}, now); err != nil {
		t.Fatal(err)
	}
	reround()
	b, err := os.ReadFile(cp)
	if err != nil {
		t.Fatal(err)
	}
	if ls := strings.Split(string(b), "\n"); ls[1] != "idle\t40\t# 밤에도 40" || ls[0] != "# 정해 둔 값" {
		t.Fatalf("설정 파일 = %q", b)
	}
	if _, ok := knobEntries()["idle"]; ok {
		t.Fatal("save 뒤 손잡이가 남음")
	}
	if x := rows()["idle"]; x.Value != "40" || x.Source != string(knob.FromFile) {
		t.Fatalf("save 뒤 = %+v", x)
	}
	if err := knobSave([]string{"hold"}, now); err == nil {
		t.Fatal("실행 중 손잡이 없이 save가 됨")
	}

	// 원격 참조 몫: 설정 파일(프로젝트별 줄 포함) 위에 손잡이
	if err := os.WriteFile(cp, []byte("merger_front_wait@pa 1500\nmerger_batch 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reround()
	es := map[string]knob.Entry{"merger_batch": {Name: "merger_batch", Value: "6", At: now.Unix()}}
	msg := knob.FormatRemote(knobRemoteOf(es, "pa", now), now)
	if want := "hc-knobs\n\nmerger_batch\t6\t-\nmerger_front_wait\t1500\t-\n"; msg != want {
		t.Fatalf("pa 몫 = %q, 바람 %q", msg, want)
	}
	if msg := knob.FormatRemote(knobRemoteOf(nil, "pb", now), now); msg != "hc-knobs\n\nmerger_batch\t4\t-\n" {
		t.Fatalf("pb 몫(다른 프로젝트 줄은 안 실림) = %q", msg)
	}
}
