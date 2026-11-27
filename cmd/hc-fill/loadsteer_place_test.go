//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
	"herdr-tools/internal/shx"
)

// fill-load-steer 이음 시험: place가 부하·손잡이를 보고 고르는지(dry — 실제 띄우기 없이 NOW에 고른 기계), 바퀴 끝 치우침 사건

// lpFix: 기계 a(이 기계)·b(연결) 둘, 자리 3씩, 프로젝트 p는 두 기계 모두. 손잡이 저장은 시험 HOME(knobFix)
func lpFix(t *testing.T) (reround func()) {
	t.Helper()
	reround = knobFix(t)
	oc, ocn, ost, oag, orows := CONN, NOW, stuck, AGUNK, ROWS
	t.Cleanup(func() {
		CONN, NOW, stuck, AGUNK, ROWS = oc, ocn, ost, oag, orows
		LOADS, MWHY, PADDED, SEATS, NOWL, OREC = nil, nil, "", nil, nil, nil
	})
	Q, AT = DATA+"/queue.tsv", DATA+"/audit.tsv"
	LOCAL, CONN, stuck, NEED, AK, OFF = "a", []string{"b"}, "-", "", "-", ""
	SEATS, NOW, NOWL, AGUNK, ROWS, OREC = nil, nil, nil, nil, nil, nil
	confLines = []string{"local a", "machine a 3 -", "machine b 3 -", "repo p a /r/a", "repo p b /r/b"}
	MWHY, PADDED = map[string]string{}, time.Now().Format("2006-01-02T15:04")
	return reround
}

func lpPlace(t *testing.T, ld map[string]mLoad) string {
	t.Helper()
	LOADS, NOW, dry = ld, nil, true
	defer func() { dry = false }()
	place("l1", "p", DATA+"/brief.md", "-", "-", "-", "")
	return strings.Join(NOW, ",")
}

// ① 자리가 남는 두 기계 — 부하 낮은 b(고치기 전: 설정 차례 앞 a) ② 모름이면 설정 차례
func TestLoadPlaceLowFirst(t *testing.T) {
	lpFix(t)
	if got := lpPlace(t, map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 1.2, Cores: 8, OK: true}}); got != "b" {
		t.Fatalf("부하 낮은 b에 떠야 한다: %q", got)
	}
	if got := lpPlace(t, map[string]mLoad{"b": {L1: 1.2, Cores: 8, OK: true}}); got != "a" {
		t.Fatalf("a 모름 → 설정 차례 a: %q", got)
	}
}

// ③ 모두 몹시 바쁨 → 이번 바퀴 안 띄움(까닭), 오래 기다린 줄은 가장 덜 바쁜 곳
func TestLoadPlaceAllBusy(t *testing.T) {
	lpFix(t)
	ld := map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 35, Cores: 10, OK: true}} // 3.33 · 3.5, 기본 load_busy 300%
	if got := lpPlace(t, ld); got != "" || MWHY["a"] != "모두 바쁨" {
		t.Fatalf("막 넣은 줄: %q 까닭 %q", got, MWHY["a"])
	}
	PADDED = time.Now().Add(-40 * time.Minute).Format("2006-01-02T15:04") // 기본 load_starve 30분
	if got := lpPlace(t, ld); got != "a" {
		t.Fatalf("40분 기다린 줄은 가장 덜 바쁜 a: %q", got)
	}
}

// 후보가 하나뿐인 줄(--only 한 기계)은 모두 바쁨 관문으로 미루지 않는다(감사 atozgd9 낮음 1) — 자리 상한은 그대로 지킨다
func TestLoadPlaceSingleCandNoGate(t *testing.T) {
	lpFix(t)
	ld := map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 35, Cores: 10, OK: true}} // 둘 다 load_busy 초과
	LOADS, NOW, dry = ld, nil, true
	defer func() { dry = false }()
	place("l1", "p", DATA+"/brief.md", "a", "-", "-", "")
	if got := strings.Join(NOW, ","); got != "a" {
		t.Fatalf("--only a 한 기계: 막 넣은 줄도 바로 a에 떠야 한다: %q", got)
	}
	NOW, SEATS = nil, []string{"a", "a", "a"} // a 자리 3 다 참
	place("l2", "p", DATA+"/brief.md", "a", "-", "-", "")
	if len(NOW) != 0 || MWHY["a"] != "자리 참" {
		t.Fatalf("자리 상한은 지킨다: %q 까닭 %q", NOW, MWHY["a"])
	}
}

// 정적 후보가 둘인 줄은 다른 기계가 자리 참이라 하나만 남아도 관문을 탄다(감사 atozgyf 중간 1 재현 — 바쁜 b에 바로 얹지 않음)
func TestLoadPlaceDynamicSingleKeepsGate(t *testing.T) {
	lpFix(t)
	SEATS = []string{"a", "a", "a"} // a 자리 3 다 참
	if got := lpPlace(t, map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 35, Cores: 10, OK: true}}); got != "" || MWHY["b"] != "모두 바쁨" || MWHY["a"] != "자리 참" {
		t.Fatalf("a 자리 참·b 몹시 바쁨 → 안 띄움: %q 까닭 %v", got, MWHY)
	}
}

// ④ --pause 기계에는 안 뜨고 기한(--for)이 지나면 돌아온다 ⑤ --seats 조정이 다음 바퀴에 먹는다
func TestLoadPlacePauseSeats(t *testing.T) {
	reround := lpFix(t)
	ld := map[string]mLoad{"a": {L1: 20, Cores: 12, OK: true}, "b": {L1: 1.2, Cores: 8, OK: true}} // a 1.67 — 바쁘지만 load_busy 아래
	now := time.Now()
	if err := knobSet([]string{"pause@b", "1", "--why", "점검", "--for", "30"}, now); err != nil {
		t.Fatal(err)
	}
	reround()
	if got := lpPlace(t, ld); got != "a" || MWHY["b"] != "멈춤(조정)" || !strings.Contains(machAdj("b"), "멈춤") {
		t.Fatalf("멈춘 b: %q 까닭 %q 조정 %q", got, MWHY["b"], machAdj("b"))
	}
	if err := knob.Set(knob.Path(HOME), knob.Entry{Name: "pause@b", Value: "1", By: "x", Why: "y", At: now.Unix(), Until: now.Unix() - 1}, true, now); err != nil {
		t.Fatal(err)
	}
	reround()
	if got := lpPlace(t, ld); got != "b" {
		t.Fatalf("기한 지난 멈춤 → b로 돌아옴: %q", got)
	}
	SEATS = []string{"b"} // b에 하나 떠 있음
	if err := knobSet([]string{"seats@b", "1", "--why", "덜 쓰기"}, now); err != nil {
		t.Fatal(err)
	}
	if got := lpPlace(t, ld); got != "b" {
		t.Fatalf("같은 바퀴(새로 안 읽음)는 설정 자리 3: %q", got)
	}
	reround()
	if got := lpPlace(t, ld); got != "a" || MWHY["b"] != "자리 참" {
		t.Fatalf("다음 바퀴 seats@b 1 → b 참, a에: %q 까닭 %q", got, MWHY["b"])
	}
	if err := knobSet([]string{"seats@zz", "1", "--why", "x"}, now); err == nil {
		t.Fatal("설정에 없는 기계의 자리 조정을 받았다")
	}
}

// ⑥ 치우침 사건: skew_min 이어진 뒤 한 번, 되풀이 간격 안에는 다시 안 감. ⑦ 기계 줄은 load.tsv에서
func TestLoadSkewEvent(t *testing.T) {
	lpFix(t)
	ev := DATA + "/ev.log"
	if err := os.WriteFile(DATA+"/hc-lane", []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>'"+ev+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", DATA+":"+os.Getenv("PATH"))
	ROWS = []Row{{"w1", "p", "/b", "-", "a", "wait"}} // --only a: 바쁜 a로만 갈 줄(옮길 일 있음 — fill-polish-1124)
	LOADS = map[string]mLoad{"a": {L1: 40, Cores: 12, OK: true}, "b": {L1: 1, Cores: 8, OK: true}}
	t0 := time.Now()
	evs := func() string { b, _ := os.ReadFile(ev); return string(b) }
	loadEnd(t0)
	loadEnd(t0.Add(5 * time.Minute))
	if evs() != "" {
		t.Fatalf("skew_min(10분) 전에 보냄: %q", evs())
	}
	loadEnd(t0.Add(11 * time.Minute))
	if e := evs(); strings.Count(e, "\n") != 1 || !strings.Contains(e, "a 코어당 부하 3.33 11분째 · 대기 레인 1 · 한가한 기계 b") || !strings.Contains(e, "hc-fill-skew") {
		t.Fatalf("치우침 사건: %q", e)
	}
	loadEnd(t0.Add(40 * time.Minute))
	if strings.Count(evs(), "\n") != 1 {
		t.Fatalf("되풀이 간격(60분) 안에 다시 보냄: %q", evs())
	}
	loadEnd(t0.Add(72 * time.Minute))
	if strings.Count(evs(), "\n") != 2 {
		t.Fatalf("간격 지나면 다시: %q", evs())
	}
	ls := machineLines(false)
	if len(ls) != 2 || !strings.Contains(ls[0], "a  부하 40.0/12(코어당 3.33)") || !strings.Contains(ls[1], "자리 0/3") {
		t.Fatalf("기계 줄: %q", ls)
	}
	// 바쁨이 풀리면 바쁨 시작을 지운다 — 다시 바빠지면 skew_min부터
	LOADS["a"] = mLoad{L1: 2, Cores: 12, OK: true}
	loadEnd(t0.Add(80 * time.Minute))
	if loadRecs()["a"].hotSince != 0 {
		t.Fatal("바쁨이 풀렸는데 바쁨 시작이 남음")
	}
}

// 부하 조각을 이 기계의 sh로 — 혼자(한 줄, 종료 0)와 실제 변경 조각 앞에 실린 꼴(끝 표식) 둘 다 읽힌다
func TestLoadSnipHere(t *testing.T) {
	confLines = []string{"repo p x /r/with space", "repo q x /r/it's"} // 폴더 인용(빈칸·따옴표)도 한 줄 그대로
	defer func() { confLines = nil }()
	if strings.Contains(loadSnip("x"), "\n") || !strings.Contains(loadSnip("x"), `--repo-read '/r/with space' '/r/it'\''s' ||`) {
		t.Fatalf("조각은 한 줄이어야 한다(가짜 hc 기록 한 줄 = exec 한 번): %s", loadSnip("x"))
	}
	out, err := exec.Command("sh", "-c", X+loadSnip("x"), "_", "-").Output()
	if l := loadParse(string(out)); err != nil || !l.OK {
		t.Fatalf("이 기계 부하를 못 읽음: %q %v", out, err)
	}
	// 원격 셸이 zsh여도 같은 조각(ht-worker-cordon 외부 가정) — 인용한 폴더가 한 인자로 간다(가짜 hc-lane이 받은 인자를 줄마다)
	if zsh, err := exec.LookPath("zsh"); err == nil {
		d := t.TempDir()
		os.WriteFile(d+"/hc-lane", []byte("#!/bin/sh\nfor a in \"$@\"; do echo \"arg $a\"; done\n"), 0o755)
		c := exec.Command(zsh, "-c", X+loadSnip("x"), "_", "-")
		c.Env = append(os.Environ(), "PATH="+d+":"+os.Getenv("PATH"))
		o, err := c.Output()
		if err != nil || !strings.Contains(string(o), "arg /r/with space\narg /r/it's\n") || !loadParse(string(o)).OK {
			t.Fatalf("zsh: %q %v", o, err)
		}
	}
	out, err = exec.Command("sh", "-c", X+actScript("x"), "_", "-").Output()
	b, ok := shx.Cut(string(out), 0)
	if l := loadParse(b); err != nil || !ok || !l.OK || len(actParse(b)) != 0 {
		t.Fatalf("실제 변경 조각에 실린 부하: %q %v", out, err)
	}
}
