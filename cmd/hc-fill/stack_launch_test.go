package main

// 쌓아 띄움 — 바퀴의 띄우기 단계(launchRows → waitRow → place → launch)를 진짜 git 원격과 가짜 hc·hc-lane(tests/hcfill/testdata,
// 레인 환경 HC_LANE_STACK을 따로 기록)로 돌린다: 선행이 감사 통과로 제출되면 착지 전에 뜨고(14칸 stacked·지시문 절·환경), 손잡이를 끄면
// 오늘처럼 선행을 기다린다(먼저 띄움 후보)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skFakeBin(t *testing.T) string {
	t.Helper()
	bin, logd := t.TempDir(), t.TempDir()
	for _, n := range []string{"hc", "hc-lane"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "tests", "hcfill", "testdata", n))
		if err != nil {
			t.Fatal(err)
		}
		if n == "hc-lane" { // 띄울 때 받은 HC_LANE_STACK을 따로 남긴다
			b = []byte(strings.Replace(string(b), "\n", "\n[ -n \"${HC_LANE_STACK-}\" ] && echo \"$* :: ${HC_LANE_STACK}\" >>\"$T_LOG/stack.log\"\n", 1))
		}
		if err := os.WriteFile(filepath.Join(bin, n), b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("T_LOG", logd)
	t.Setenv("HC_FAKE_MACHINES", `{"machines":[]}`)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return logd
}

func skRound(t *testing.T) string {
	t.Helper()
	oe := exitHooks
	defer func() { exitHooks = oe }()
	ROWS = qRows()
	NOW, NOWL, OFF, FETCHED, FETCHBAD, ELIST = nil, nil, "", "", "", ""
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	launchRows()
	os.Stdout = old
	b, _ := os.ReadFile(f.Name())
	return string(b)
}

func TestStackLaunchRound(t *testing.T) {
	for _, on := range []bool{true, false} {
		skLaunch(t, on, "-")
	}
}

// ⑫ 깔리는 순간 이미 주차한 먼저 띄움 줄: 선행이 감사 통과로 제출되면 주차 가지에서(--resume) 쌓아 이어 띄운다 — 이어 가기 절
// (origin/<기본> rebase) 대신 쌓아 띄움 절(git rebase P)
func TestStackLaunchParked(t *testing.T) { skLaunch(t, true, "parked") }

func skLaunch(t *testing.T, on bool, ef string) {
	{
		s := skSetup(t, on)
		logd := skFakeBin(t)
		confLines = append(confLines, "early 0", "overlap off", "gc 0")
		oh := HOME
		HOME = os.Getenv("HOME") // run 지시문 자리(skSetup이 임시 HOME)
		t.Cleanup(func() { HOME = oh })
		OWNF, OWND, SB = DATA+"/own.tsv", DATA+"/own", DATA+"/sb.md"
		MQP, MQBAD, MQS = "", "", nil
		a := s.commit(s.base, "A: 앞 일", 1, "a")
		s.submit("merge-queue", "A", a, s.base, " verdict=aud1 patch="+s.patch(s.base, a), s.st)
		brief := t.TempDir() + "/b.md"
		os.WriteFile(brief, []byte("# 뒤 일\n"), 0o644)
		s.row("A", "-", "up", "-")
		s.q = append(s.q, s.q[0].Set(qName, "B").Set(qAfter, "A").Set(qState, "wait").Set(qMach, "-").Set(qAddr, "-").Set(qBrief, brief).Set(qRID, "r1-1-9").Set(qEarly, ef))
		os.WriteFile(Q, []byte(rowsText(s.q)), 0o644)
		out := skRound(t)
		b := qrow("B")
		stk, _ := os.ReadFile(logd + "/stack.log")
		if !on {
			if b.F(qState) != "wait" || len(stk) != 0 || strings.Contains(out, "쌓아 띄움") {
				t.Errorf("손잡이 끔: 오늘처럼 선행을 기다려야 함 — 줄 %v\n%s", b, out)
			}
			return
		}
		if b.F(qState) != "up" || b.F(qEarly) != "stacked" {
			t.Fatalf("쌓아 띄우지 않음: 줄 %v\n%s", b, out)
		}
		if !strings.Contains(string(stk), "--name B") || !strings.Contains(string(stk), ":: A@"+a) {
			t.Errorf("hc-lane에 HC_LANE_STACK을 안 넘김: %s", stk)
		}
		run, _ := os.ReadFile(HOME + "/.local/share/hc-fill/runs/B-run.md")
		if !strings.Contains(string(run), "## 쌓아 띄움 — 기준은 선행 A "+a[:8]) {
			t.Errorf("run 지시문에 쌓아 띄움 절 없음:\n%s", run)
		}
		if r := stackRec("B"); r == nil || r.F(skP) != a || r.F(skState) != "up" {
			t.Errorf("쌓아 띄움 기록: %v", r)
		}
		if ef == "parked" && (!strings.Contains(string(stk), "--resume") || strings.Contains(string(run), "## 이어 가기") ||
			!strings.Contains(string(run), "`git fetch origin && git rebase "+a+"`")) {
			t.Errorf("주차 줄: --resume·쌓아 띄움 절(이어 가기 절 대신)이 아님: %s\n%s", stk, run)
		}
		t.Logf("바퀴 출력:\n%s", out)
	}
}
