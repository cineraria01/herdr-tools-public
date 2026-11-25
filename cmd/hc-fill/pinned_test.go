//go:build !windows

package main

// ht-advise-only-stuck(2026-11-25 지휘 t30x — --only 한 기계로 묶은 줄이 선행이 들어간 뒤에도 그 기계 레인상한으로 「대기(주차 — 선행 기다림)」에
// 남고 다른 기계는 놀았다): 바퀴가 「묶인 기계 상한」을 pinned.tsv에 적고, status는 그 까닭을, advise는 advise_pinned분 넘으면 주인 지휘에 한 번.
// 고치기 전 코드: ONLYFULL·pinSave·stIn.Pin이 없어 컴파일부터 실패

import (
	"os"
	"strings"
	"testing"
	"time"
)

// 묶인 기계(a)가 자리 참 → 안 뜨고 까닭 「a 상한 — 부하」. 대조: 묶음 없음 → b에 뜨고 까닭 없음, 묶인 기계에 자리 있음 → 까닭 없음
func TestPinnedPlace(t *testing.T) {
	lpFix(t)
	dry = true
	defer func() { dry = false }()
	LOADS = map[string]mLoad{"a": {L1: 18.9, Cores: 10, OK: true}, "b": {L1: 1, Cores: 8, OK: true}}
	SEATS = []string{"a", "a", "a"}
	place("l1", "p", DATA+"/brief.md", "a", "-", "-", "")
	if len(NOW) != 0 || ONLYFULL != "a 상한 — 부하 18.9/10" {
		t.Fatalf("묶인 기계 자리 참: 띄움 %q 까닭 %q", NOW, ONLYFULL)
	}
	NOW = nil
	place("l1", "p", DATA+"/brief.md", "-", "-", "-", "")
	if strings.Join(NOW, ",") != "b" || ONLYFULL != "" {
		t.Fatalf("묶음 없음: 띄움 %q 까닭 %q", NOW, ONLYFULL)
	}
	NOW, SEATS = nil, nil
	place("l1", "p", DATA+"/brief.md", "a", "-", "-", "")
	if strings.Join(NOW, ",") != "a" || ONLYFULL != "" {
		t.Fatalf("묶인 기계 자리 있음: 띄움 %q 까닭 %q", NOW, ONLYFULL)
	}
}

// 한 바퀴 통째로(감사 atp22es 중간 1 — 조각 시험만 있어 waitRow의 PINNOW 줄을 지워도 통과했다): 선행 p-0이 들어간 주차 줄 pin을
// --only here(자리 0)로 묶으면 launchRows가 pinned.tsv에 적고, 상태 단계·state 대기 까닭이 진짜 까닭. 다음 바퀴에 처음 본 초 그대로,
// 선행이 아직인 줄로 바뀌면(선행 기다림이 진짜 까닭) 빠진다
func TestPinnedRound(t *testing.T) {
	s := acSetup(t, 3, 0)
	brief := s.fr + "/../b.md"
	q, _ := os.ReadFile(Q)
	pin := func(after string) {
		r := pad16("pin", "app", brief, after, "here", "wait", "-", "-", "-", "t", "-", "-", "normal", "parked", "-", "-").Set(qRID, "rp")
		if err := os.WriteFile(Q, append(append([]byte(nil), q...), r.Line()+"\n"...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pin("p-0")
	acTick(t)
	why, since := pinOf("pin", "rp")
	if why != "here 상한" || since == 0 {
		t.Fatalf("첫 바퀴 pinned.tsv: %q %d\n%s", why, since, readLinesT(DATA+"/pinned.tsv"))
	}
	if st := stStage(stIn{State: "wait", Early: "parked", Pin: why}); st != "대기(묶인 기계 here 상한)" {
		t.Errorf("상태 단계: %q", st)
	}
	for _, r := range qRows() {
		if r.F(qName) == "pin" && stateWaitWhy(r) != "묶인 기계 here 상한" {
			t.Errorf("state 대기 까닭: %q", stateWaitWhy(r))
		}
	}
	old := itoa(since - 300)
	if err := os.WriteFile(DATA+"/pinned.tsv", []byte("pin\trp\t"+old+"\there 상한\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	acTick(t)
	if _, s2 := pinOf("pin", "rp"); itoa(s2) != old {
		t.Errorf("둘째 바퀴에 처음 본 초가 바뀜: %d, 바람 %s", s2, old)
	}
	pin("p-2") // 아직 안 들어간 선행
	acTick(t)
	if why, _ := pinOf("pin", "rp"); why != "" {
		t.Errorf("선행 기다림인데 묶인 기계 까닭: %q", why)
	}
}

func readLinesT(p string) string { b, _ := os.ReadFile(p); return string(b) }

// 감사 atp22es 낮음 1: 지시문 「필요:」 능력이 없는 기계로 넓히라고 하지 않는다(그 기계엔 어차피 안 뜬다). 띄울 수 있는 기계(Can)도 같다
func TestPinnedAdviseNeeds(t *testing.T) {
	advFx(t)
	brief := DATA + "/briefs/pin.md"
	if err := os.MkdirAll(DATA+"/briefs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brief, []byte("# 일\n필요: gpu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	confLines = append(confLines, "repo app x /r", "machine-has w gpu", "machine-has x gpu") // m엔 gpu 없음
	r := pad16("lane-p", "app", brief, "-", "w", "wait", "-", "-", "-", "t", "-", "m/t9", "normal", "-", "-", "-").Set(qRID, "r9")
	w := adviseWait(r, time.Now())
	if strings.Join(w.Can, ",") != "w" || strings.Join(w.Wider, ",") != "x" {
		t.Fatalf("Can %q Wider %q — 바람 w · x(m은 gpu 없음)", w.Can, w.Wider)
	}
	r = r.Set(qOnly, "-")
	if w := adviseWait(r, time.Now()); strings.Join(w.Can, ",") != "w,x" {
		t.Errorf("묶음 없음 Can %q — 바람 w,x", w.Can)
	}
}

// 감사 atp22es 낮음 2: 바퀴가 pinStale초 넘게 pinned.tsv를 다시 쓰지 않았으면(launchd 꺼짐·잠금 실패) 옛 까닭으로 보이지 않는다
func TestPinnedStale(t *testing.T) {
	DATA = t.TempDir()
	if err := os.WriteFile(DATA+"/pinned.tsv", []byte("l1\tr1\t1795000000\ta 상한\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if why, _ := pinOf("l1", "r1"); why != "a 상한" {
		t.Fatalf("갓 쓴 기록: %q", why)
	}
	old := time.Now().Add(-time.Duration(pinStale+60) * time.Second)
	if err := os.Chtimes(DATA+"/pinned.tsv", old, old); err != nil {
		t.Fatal(err)
	}
	if why, _ := pinOf("l1", "r1"); why != "" {
		t.Errorf("바퀴가 멈춘 옛 기록인데 까닭: %q", why)
	}
}

// 바퀴마다 다시 적되 처음 본 때는 그대로(같은 줄 ID), 이번 바퀴에 없으면 빠진다
func TestPinnedSaveSince(t *testing.T) {
	DATA = t.TempDir()
	t0 := time.Unix(1795000000, 0)
	PINNOW = []Row{{"l1", "r1", "", "a 상한"}}
	pinSave(t0)
	PINNOW = []Row{{"l1", "r1", "", "a 상한 — 부하 2.0/10"}}
	pinSave(t0.Add(5 * time.Minute))
	if why, since := pinOf("l1", "r1"); why != "a 상한 — 부하 2.0/10" || since != t0.Unix() {
		t.Fatalf("까닭 %q 처음 본 때 %d", why, since)
	}
	if why, _ := pinOf("l1", "r2"); why != "" {
		t.Errorf("다른 줄 ID인데 까닭: %q", why)
	}
	PINNOW = nil
	pinSave(t0.Add(10 * time.Minute))
	if why, _ := pinOf("l1", "r1"); why != "" {
		t.Errorf("풀린 뒤에도 까닭: %q", why)
	}
}

func TestPinnedStage(t *testing.T) {
	for _, in := range []stIn{{State: "wait", Early: "parked", Pin: "a 상한 — 부하 18.9/10"}, {State: "wait", Pin: "a 상한 — 부하 18.9/10"}} {
		if got := stStage(in); got != "대기(묶인 기계 a 상한 — 부하 18.9/10)" {
			t.Errorf("%+v: %q", in, got)
		}
	}
}

// advise: 묶인 기계(w — 빈 자리 없음)로 11분 못 뜬 줄, 다른 기계 m에 빈 자리 → 주인 지휘(--to)에 한 번, 되풀이 없음
func TestPinnedAdviseTick(t *testing.T) {
	advFx(t)
	now := time.Now()
	q, _ := os.ReadFile(DATA + "/queue.tsv")
	add := now.Add(-3 * time.Hour).Format("2006-01-02T15:04")
	row := strings.Join([]string{"lane-p", "app", "b", "-", "w", "wait", "-", "-", "-", add, "-", "m/t9", "normal", "parked", "-", "-", "r9"}, "\t")
	akWrite(t, "queue.tsv", string(q)+row+"\n")
	akWrite(t, "pinned.tsv", "lane-p\tr9\t"+itoa(now.Add(-11*time.Minute).Unix())+"\tw 상한 — 부하 30.0/10\n")
	adviseTick(now)
	var got []string
	for _, r := range reports(t) {
		if strings.Contains(r, "묶인 기계") {
			got = append(got, r)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "--to m/t9") || !strings.Contains(got[0], "hc-fill edit lane-p --only w,m") {
		t.Fatalf("묶인 기계 사건: %q\n전체 %q", got, reports(t))
	}
	n := len(reports(t))
	adviseTick(now.Add(10 * time.Minute))
	for _, r := range reports(t)[n:] {
		if strings.Contains(r, "묶인 기계") {
			t.Fatalf("되풀이: %q", r)
		}
	}
	why, _ := pinOf("lane-p", "r9")
	if st := stStage(stIn{State: "wait", Early: "parked", Pin: why}); st != "대기(묶인 기계 w 상한 — 부하 30.0/10)" {
		t.Errorf("상태 줄: %q", st)
	}
}
