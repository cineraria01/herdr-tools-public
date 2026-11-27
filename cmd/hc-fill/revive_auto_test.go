package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// ht-revive-auto(2026-11-28): 기계가 다시 닿은 바퀴(unreach.go 「다시 닿음」)에 그 기계의 꺼져 죽은 줄마다 revive와 같은 판정으로 스스로
// 이어 띄운다 — 손잡이 revive_max(줄당 횟수, 기본 2, 0이면 스스로 안 함). 가짜 hc·hc-lane은 revive_test의 것

// raFx: 줄 w1(·w2)이 꺼져 죽은 기계 m이 이 바퀴에 다시 닿았다(앞 바퀴 seen.tsv에 그 터미널)
func raFx(t *testing.T, two bool) *rvT {
	k := rvFx(t)
	orc := REACHED
	t.Cleanup(func() { REACHED = orc })
	REACHED = map[string]bool{"m": true}
	seen := "m\tw1\tm/w1\n"
	if two {
		wt2 := DATA + "/repo.wt/w2"
		if err := os.MkdirAll(wt2, 0o755); err != nil {
			t.Fatal(err)
		}
		k.git(wt2, "init", "-q", "-b", "w2")
		k.git(wt2, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", "w2: 반쯤")
		k.wr("queue.tsv", k.read("queue.tsv")+strings.Join([]string{"w2", "app", DATA + "/b.md", "-", "-", "up", "m", "m/w2", "-", "t", "-", "t0", "normal", "-", "auto", "-", "r2"}, "\t")+"\n")
		EV = append(EV, auTEv("2026-11-28T01:00:00Z", "m/w2", "w2", "중간", "-", "일하는 중", "w2@1700000000000"))
		seen += "m\tw2\tm/w2\n"
	}
	k.wr("seen.tsv", seen)
	return k
}

// tick: 바퀴 하나의 두 자리 — 멈춤 알림 자리(deadTick)와 띄우기 앞(reviveAutoTick). 표(TERMS)는 가짜 hc 목록 그대로
func (k *rvT) tick() {
	deadTick()
	reviveAutoTick()
	// 다음 바퀴의 readTables — 가짜 hc 목록 「주소 이름 created_ms 상태 cwd」
	TERMS, LSSNAP = nil, true
	for _, l := range lines(k.read("terms.tsv")) {
		if f := strings.Split(l, "\t"); len(f) == 5 {
			TERMS = append(TERMS, Term{"m", f[0], "claude-code", f[1], f[3], f[4]})
		}
	}
}

func (k *rvT) reports(st string) int { return strings.Count(k.read("lane.log"), "report "+st+" ") }

// ① 다시 닿음 + 죽은 줄 둘 → 둘 다 이어 띄움·중간 알림 둘(꺼져 죽음 막힘은 내지 않는다 — 결과가 알림)
func TestReviveAutoTwo(t *testing.T) {
	k := raFx(t, true)
	k.tick()
	if k.launches() != 2 || k.reports("중간") != 2 || k.reports("막힘") != 0 {
		t.Fatalf("둘 다 이어 띄움·중간 둘이어야:\n%s", k.read("lane.log"))
	}
	e := k.read("lane.log")
	if !strings.Contains(e, "기계 m 재시동 — 레인 w1 같은 작업 트리에서 이어 띄움(m/w1 → m/w1b)") || !strings.Contains(e, "--to t0") {
		t.Fatalf("알림 글: %s", e)
	}
	if qrow("w1").F(qAddr) != "m/w1b" || qrow("w2").F(qAddr) != "m/w2bb" || !strings.Contains(k.read("revive.tsv"), "\t이어 띄움\tm/w1b\t") {
		t.Fatalf("줄이 새 실행에: %v %v\n%s", qrow("w1"), qrow("w2"), k.read("revive.tsv"))
	}
	// ⑤ 같은 다시 닿음을 다음 바퀴도 봄(알림이 실패해 reach.tsv가 down으로 남은 경우) → 한 번만
	k.tick()
	if k.launches() != 2 {
		t.Fatalf("두 번째 바퀴는 띄우지 않는다: %d", k.launches())
	}
}

// ② 다시 닿음 + 터미널 살아 있음(끊김만) → 새 세션 0, 알림 0
func TestReviveAutoAliveNothing(t *testing.T) {
	k := raFx(t, false)
	k.wr("terms.tsv", "m/w1\tw1\t1700000000000\tworking\t"+k.wt+"\n")
	TERMS = []Term{{"m", "m/w1", "claude-code", "w1", "working", k.wt}}
	k.tick()
	if k.launches() != 0 || strings.Contains(k.read("lane.log"), "report") || exists(rvFile()) {
		t.Fatalf("끊김만이면 아무것도 안 함:\n%s", k.read("lane.log"))
	}
}

// ③ revive_max 넘김 → 막힘 한 번(되풀이 없음), 띄우지 않음 ④ 손잡이 0 → 스스로 안 함(꺼져 죽음 막힘은 그대로)
func TestReviveAutoMaxAndOff(t *testing.T) {
	k := raFx(t, false)
	confLines = append(confLines, "revive_max 1")
	k.wr("revive.tsv", "w1\tr1\tm/w0\t이어 띄움\tm/w1\t1700000000\t스스로\n")
	k.tick()
	k.tick()
	if k.launches() != 0 || k.reports("막힘") != 1 || !strings.Contains(k.read("lane.log"), "스스로 이어 띄우기 상한 revive_max 1번") ||
		!strings.Contains(k.read("lane.log"), "hc-fill revive w1") {
		t.Fatalf("상한이면 막힘 한 번:\n%s", k.read("lane.log"))
	}
	k = raFx(t, false)
	confLines = append(confLines, "revive_max 0")
	k.tick()
	k.tick()
	if k.launches() != 0 || k.reports("막힘") != 1 || !strings.Contains(k.read("lane.log"), "꺼져 죽음") {
		t.Fatalf("0이면 스스로 안 하고 꺼져 죽음 막힘 한 번:\n%s", k.read("lane.log"))
	}
}

// ⑥ 이어 띄운 직후 또 끊겼다 다시 닿음 → 두 번째 이어 띄움, 셋째는 상한(기본 2)
func TestReviveAutoAgain(t *testing.T) {
	k := raFx(t, false)
	for i, want := range []string{"m/w1b", "m/w1bb"} {
		k.tick()
		if a := qrow("w1").F(qAddr); a != want {
			t.Fatalf("%d번째: %s(바람 %s)\n%s", i+1, a, want, k.read("lane.log"))
		}
		// 또 끊겼다 다시 닿음: 앞 바퀴에 본 터미널이 없어졌다
		k.wr("seen.tsv", fmt.Sprintf("m\tw1\t%s\n", want))
		k.wr("terms.tsv", "")
		TERMS = nil
	}
	k.tick()
	if k.launches() != 2 || k.reports("막힘") != 1 || !strings.Contains(k.read("lane.log"), "상한 revive_max 2번") {
		t.Fatalf("셋째는 상한:\n%s", k.read("lane.log"))
	}
}

// 운영 장면: 이미 쌓인 상태에 처음 돌 때 — 다시 닿음을 못 본 옛 죽음(기록 「죽음」)은 스스로 띄우지 않고 표시만 · 외부 가정(재시동 뒤 코어가 덜
// 뜸 — hc get 모름)은 기다렸다가 다음 바퀴에 · 못 띄움(작업 트리 없음)은 막힘 한 번과 손으로 칠 명령
func TestReviveAutoOldWaitFail(t *testing.T) {
	k := raFx(t, false)
	REACHED = map[string]bool{}
	k.tick()
	if k.launches() != 0 || !strings.Contains(k.read("revive.tsv"), "\t죽음\t") {
		t.Fatalf("다시 닿음을 못 본 죽음은 표시만:\n%s", k.read("lane.log"))
	}
	k = raFx(t, false)
	hc := k.read("hc")
	k.wr("hc", strings.Replace(hc, "case \"$1\" in\n", "[ \"$1\" != get ] || exit 9\ncase \"$1\" in\n", 1))
	k.tick()
	if k.launches() != 0 || strings.Contains(k.read("lane.log"), "report") {
		t.Fatalf("모름이면 기다림:\n%s", k.read("lane.log"))
	}
	k.wr("hc", hc)
	k.wr("seen.tsv", "") // 다음 바퀴 — 앞 바퀴 seen에는 이제 없어도 기록 「죽음(재시동)」으로 잇는다
	k.tick()
	if k.launches() != 1 || k.reports("중간") != 1 {
		t.Fatalf("다음 바퀴에 이어 띄움:\n%s", k.read("lane.log"))
	}
	k = raFx(t, false)
	if err := os.RemoveAll(k.wt); err != nil {
		t.Fatal(err)
	}
	k.tick()
	k.tick()
	if k.launches() != 0 || k.reports("막힘") != 1 || !strings.Contains(k.read("lane.log"), "이어 띄우지 못함") || !strings.Contains(k.read("lane.log"), "작업 트리가 없다") {
		t.Fatalf("못 띄움은 막힘 한 번:\n%s", k.read("lane.log"))
	}
}

// unreachTick: 앞 바퀴 끊김(reach.tsv down)이던 기계가 이 바퀴에 연결이면 REACHED — 레인이 없어 알림이 안 갔어도
func TestUnreachMarksReached(t *testing.T) {
	urFx(t)
	CONN, TERMS = nil, nil
	if err := os.WriteFile(reachFile(), []byte("m2\tdown\t1\t0\t-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	CONN = []string{"m2"}
	unreachTick()
	if !REACHED["m2"] {
		t.Fatal("다시 닿은 기계로 적어야")
	}
	unreachTick()
	if REACHED["m2"] {
		t.Fatal("다음 바퀴는 다시 닿음이 아니다")
	}
}
