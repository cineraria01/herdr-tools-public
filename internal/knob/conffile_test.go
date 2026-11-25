//go:build !windows

package knob

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ht-config-knobs: 개인 설정 폴더의 손잡이 설정 파일(ConfName) — 「정해 둔 것」 층. 우선순위 손잡이 > 설정 파일 > 도구별 옛 설정 > 기본

func TestConfParse(t *testing.T) {
	src := "# 주석\n\nidle  20  # 지휘가 보기에 10분은 잦다\nseats@w1\t1\nmerger_front_wait@proj 1500 # 검증이 긴 저장소\n" +
		"no_such 1\nidle 30\nhold 99999\nhold\n"
	es, errs := ParseConf(src, "/c/knobs.tsv")
	if e := es["idle"]; e.Value != "20" || e.Why != "지휘가 보기에 10분은 잦다" || e.Line != 3 {
		t.Fatalf("idle = %+v", e)
	}
	if es["seats@w1"].Value != "1" || es["merger_front_wait@proj"].Value != "1500" {
		t.Fatalf("묶음·프로젝트별 줄 = %+v", es)
	}
	// 표에 없는 이름·같은 이름 두 번(뒤 줄 버림)·범위 밖·값 없음은 줄마다 하나씩 거절, 먹지 않는다
	if len(errs) != 4 {
		t.Fatalf("거절 %d개: %v", len(errs), errs)
	}
	for _, n := range []string{"no_such", "hold"} {
		if _, ok := es[n]; ok {
			t.Errorf("%s가 먹었다", n)
		}
	}
	if !strings.Contains(errs[0].Error(), "/c/knobs.tsv:6") {
		t.Errorf("거절 글에 경로:줄이 없다: %v", errs[0])
	}
}

func TestConfLayer(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cf, _ := ParseConf("idle 20 # 정함\nmerger_front_wait 900\n", "/c/knobs.tsv")
	d, _ := Find("idle")
	if v, src, _ := Resolve(d, nil, cf, "15", now); v != "20" || src != FromFile {
		t.Fatalf("설정 파일이 옛 설정·기본을 덮지 않음: %s %s", v, src)
	}
	es := map[string]Entry{"idle": {Name: "idle", Value: "40", At: now.Unix()}}
	if v, src, _ := Resolve(d, es, cf, "15", now); v != "40" || src != FromKnob {
		t.Fatalf("손잡이가 설정 파일을 덮지 않음: %s %s", v, src)
	}
	h, _ := Find("hold")
	if v, src, _ := Resolve(h, nil, cf, "15", now); v != "15" || src != FromConf {
		t.Fatalf("옛 설정(hc-fill.tsv 키·환경 변수) 호환: %s %s", v, src)
	}
	if v, src, _ := Resolve(h, nil, cf, "", now); v != h.Default || src != FromDefault {
		t.Fatalf("기본: %s %s", v, src)
	}
	// 프로젝트별은 「이름@프로젝트」 → 공통 손잡이 → 설정 파일의 공통 줄
	p, _ := Find("merger_front_wait@proj")
	if v, src, _ := Resolve(p, nil, cf, "", now); v != "900" || src != FromFile {
		t.Fatalf("프로젝트별이 설정 파일 공통 줄로 안 내려감: %s %s", v, src)
	}
	if where := cf["idle"].Where("/c/knobs.tsv"); where != "/c/knobs.tsv:1" {
		t.Fatalf("출처 = %q", where)
	}
}

// 다른 기계(원격 참조로 읽는 도구)도 같은 설정 파일 값 — 참조에 없는 손잡이는 설정 파일, 그다음 기본
func TestConfRemote(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cf, _ := ParseConf("sweep_r1 45\nsweep_r2 50\n", "/c/knobs.tsv")
	r := ParseRemote(remoteHead+"\n\nsweep_r2\t60\t-\n", now)
	if got := r.With(cf).Get("sweep_r1"); got != "45" {
		t.Fatalf("원격에 없는 손잡이 = %s, 설정 파일 45여야", got)
	}
	if got := r.With(cf).Get("sweep_r2"); got != "60" {
		t.Fatalf("원격 손잡이가 설정 파일을 못 덮음: %s", got)
	}
	if got := r.With(cf).Get("sweep_r3"); got != "360" {
		t.Fatalf("둘 다 없으면 기본: %s", got)
	}
}

// 범용 틀 config.example/knobs.tsv는 등록 표에서 만든 것과 바이트까지 같다(손으로 고치지 않는다 — 고치면 이 시험이 잡는다)
func TestConfTemplate(t *testing.T) {
	b, err := os.ReadFile("../../config.example/" + ConfName)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Template() {
		t.Fatalf("config.example/%s가 등록 표와 다르다 — go run ./cmd/herdr-tools-config knobs-template > config.example/%s", ConfName, ConfName)
	}
	for _, d := range Defs {
		if !strings.Contains(string(b), "# "+d.Name) {
			t.Errorf("틀에 %s 줄이 없다", d.Name)
		}
	}
	es, errs := ParseConf(string(b), "틀")
	if len(es) != 0 || len(errs) != 0 {
		t.Fatalf("틀은 모두 주석이어야: %v %v", es, errs)
	}
}
