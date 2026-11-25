package main

import (
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/opsstate"
)

// 바퀴 차례: hang.tsv(배경 일 기다림 b-live)를 적는 hangTick이 그것을 읽는 adviseTick·opsAgentTick보다 먼저 —
// 거꾸로면 턴이 끝난 첫 바퀴에 b-live가 아직 없어 lane-done이 한 번 나갔다(감사 atp2gk8-mt7ny8 중간)
func TestStallTickHangBeforeAdvise(t *testing.T) {
	b, err := os.ReadFile("stall.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, "func stallTick() {")
	if i < 0 {
		t.Fatal("stallTick 없음")
	}
	body := src[i:]
	h, a, o := strings.Index(body, "\thangTick("), strings.Index(body, "\tadviseTick("), strings.Index(body, "\topsAgentTick(")
	if h < 0 || a < 0 || o < 0 || h > a || h > o {
		t.Fatalf("stallTick 차례: hangTick %d · adviseTick %d · opsAgentTick %d — hangTick이 먼저여야", h, a, o)
	}
}

// 「끝났는데 보고 없음」 오경보 두 꼴(2026-11-25~26 ht-advise-done-noreport):
// d1a-r1430-frame-memory — 병합기에 제출한 뒤 「착지 안 됨」 중간 메모를 하나 더 남겨 마지막 보고가 중간이었다(착지·반송 결과를 기다리는 중).
// d1a-r1434-tiktok-cdn-refused — 턴은 끝났지만(done) 「1 shell still running」, 배경 셸로 마지막 검증을 기다리는 중이었다.
// 배경 셸이 죽었는데 보고가 없거나, 그냥 끝나고 보고가 없으면 지금처럼 알린다
func TestAdviseDoneNoReportWaits(t *testing.T) {
	sid := "box-20261125T231400-4242"
	sub := isEv("2026-11-25T14:14:00Z", "box", "제출", "990dea01aaaabbbb", sid+" → refs/merge-queue/1-l-x (병합기 mg 가 main 에 넣는다)", "submit:"+sid)
	memo := isEv("2026-11-25T14:15:00Z", "box", "중간", "990dea01aaaabbbb", "착지 안 됨 — 병합기 결과 기다림", "-")
	memoNoSHA := isEv("2026-11-25T14:15:00Z", "box", "중간", "-", "착지 안 됨", "-")
	newWork := isEv("2026-11-25T14:20:00Z", "box", "중간", "1234567800000000", "다른 커밋으로 이어 감", "-")
	work := isEv("2026-11-25T14:10:00Z", "box", "중간", "abcdef12", "검증 돌림", "-")
	land := isEv("2026-11-25T14:30:00Z", "mg", "착지", "0123456789abcdef", "병합기: 착지 — 제출 ID "+sid+", 묶음 1", "-")
	for _, c := range []struct {
		name  string
		ev    []string
		hang  string // hang.tsv 종류(빈 값 = 줄 없음)
		stage string // 단계 머리
		alert bool   // lane-done 알림
	}{
		{"제출 뒤 중간 메모 — 착지 기다림", []string{sub, memo}, "", "제출됨", false},
		{"제출 뒤 sha 없는 중간 메모 — 착지 기다림", []string{sub, memoNoSHA}, "", "제출됨", false},
		{"제출 뒤 결과가 옴 — 그 결과", []string{sub, memo, land}, "", "착지", false},
		{"제출 뒤 다른 커밋으로 중간 — 제출을 기다리는 것이 아니다", []string{sub, newWork}, "", "작업 중(done", true},
		{"배경 셸 살아 있음 — 배경 일 기다림", []string{work}, hangBgLive, "배경 일 기다림", false},
		{"배경 셸 죽음 + 보고 없음 — 알림", []string{work}, "b", "작업 중(done", true},
		{"그냥 끝나고 보고 없음 — 알림", []string{work}, "", "작업 중(done", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			setData(t)
			oMGM, oEV := MGM, EV
			t.Cleanup(func() { MGM, EV = oMGM, oEV })
			MGM, EV = " mg", c.ev
			if err := os.WriteFile(DATA+"/try.tsv", []byte("l-x\t-\t-\t-\t-\tt1\tt1@1000\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.hang != "" {
				if err := os.WriteFile(DATA+"/hang.tsv", []byte("l-x\tt1\t"+c.hang+"\t1\t-\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			l := evLast("l-x", "t1")
			last := Row{"l-x", evF(l, evState), head(evF(l, evSHA), 8), "14:15", "-", "14:40", "fresh", "r1"}
			stage := stStage(stIn{State: "up", Live: "done", Last: last, Pos: -1, Bg: hangBgWait("l-x", "t1")})
			if !strings.HasPrefix(stage, c.stage) {
				t.Fatalf("단계 %q — 바람 머리 %q", stage, c.stage)
			}
			s := opsstate.State{Lanes: []opsstate.Lane{{Name: "l-x", Machine: "box", Stage: stage, Open: true}}}
			it := opsstate.Advise(s, opsstate.Facts{Addr: map[string]string{"l-x": "t1"}}, opsstate.Thresholds{StageMin: 90})
			got := false
			for _, x := range it {
				got = got || x.Kind == "lane-done"
			}
			if got != c.alert {
				t.Fatalf("lane-done %v — 바람 %v (%v)", got, c.alert, opsstate.Summaries(it))
			}
		})
	}
}
