package main

// coord-digest(2026-11-24): watch가 지휘에게 보내는 사건을 즉시·묶음으로 가른다(digest.go). 바이너리를 빌드해 가짜 hc로 돌린다
// (watchidle_test의 idleEnv·startIdleWatch). 주기는 손잡이 대신 HC_LANE_COORD_DIGEST_SEC(초)로 짧게.
// ① 손잡이 0 → 전과 바이트까지 같음 ② 주기 + 한 레인의 완료·감사 판정·제출·착지(바퀴마다 하나씩) → 한 메시지·그 레인 한 줄
// (고치기 전 코드: 네 메시지) ③ 묶음이 모인 채 막힘 → 한 메시지에 묶음 먼저·막힘 뒤 ④ 21건 → 주기 전 보냄 ⑤ watch가 다시 떠도 모인 것이
// 한 번 나감 ⑥ 모르는 종류 → 즉시 ⑦ 지휘 둘 → 각자 묶음 ⑧ 운영 에이전트가 맡은 advise → 묶음, 아니면 즉시

import (
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/events"
	"herdr-tools/internal/knob"
)

// dgLine: 그 지휘가 주인(11칸)인 사건 한 줄 — 키(12칸)가 있으면 싣는다
func dgLine(at int64, lane, st, sha, msg, owner, key string) string {
	f := []string{utcAt(at), "m", "t5", lane, st, sha, msg, "1", "-", "-", owner}
	if key != "" {
		f = append(f, key)
	}
	return strings.Join(f, tab) + "\n"
}

func appendEv(t *testing.T, w *watchRun, l string) {
	t.Helper()
	f, err := os.OpenFile(w.ed+"/events.tsv", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(l); err != nil {
		t.Fatal(err)
	}
}

// sends: 보낸 글(가짜 hc send가 「주소<탭>글」로 적은 줄)
func (w *watchRun) sends() []string {
	b, _ := os.ReadFile(w.send)
	return lines(strings.TrimRight(string(b), "\n"))
}

// ticks: 바퀴가 n번 더 돌 때까지(hc ls 부른 수)
func ticks(t *testing.T, d string, n int) {
	t.Helper()
	n0 := lsTicks(d)
	waitFor(t, "바퀴", func() bool { return lsTicks(d) >= n0+n })
}

func TestWatchDigest(t *testing.T) {
	if testing.Short() {
		t.Skip("바이너리를 빌드해 watch를 돌린다")
	}
	bin := buildLane(t, t.TempDir())
	body := agentSample(t, "4-shell-agent-idle")
	now := time.Now().Unix()
	setup := func(t *testing.T) (string, string) {
		d := t.TempDir()
		return d, idleEnv(t, d, body)
	}

	t.Run("①손잡이0은전과같음", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		l1 := dgLine(now, "lane-a", "완료", "-", "요약", co, "")
		l2 := dgLine(now+1, "audit-lane-a", "판정", "-", "통과 0·0·2", co, "")
		w := startIdleWatch(t, bin, d, co, l1+l2)
		waitFor(t, "보냄", func() bool { return len(w.sends()) > 0 })
		ticks(t, d, 2)
		want := co + tab + "hc-lane: " + wpart(strings.TrimSuffix(l1, "\n")) + " ‖ " + wpart(strings.TrimSuffix(l2, "\n"))
		if s := w.sends(); len(s) != 1 || s[0] != want {
			t.Fatalf("손잡이 0인데 전과 다름:\n%q\n바람:\n%q", s, want)
		}
	})

	t.Run("손잡이파일을읽음", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		if err := knob.Set(knob.Path(d), knob.Entry{Name: "coord_digest", Value: "5", By: "시험", Why: "시험"}, false, time.Now()); err != nil {
			t.Fatal(err)
		}
		w := startIdleWatch(t, bin, d, co, dgLine(now, "lane-a", "완료", "-", "요약", co, ""))
		ticks(t, d, 3)
		if s := w.sends(); len(s) != 0 {
			t.Fatalf("손잡이 5분인데 바로 보냄: %q", s)
		}
		appendEv(t, w, dgLine(now+1, "lane-b", "막힘", "-", "까닭", co, ""))
		waitFor(t, "막힘", func() bool { return len(w.sends()) > 0 })
		if s := w.sends(); len(s) != 1 || !strings.Contains(s[0], "hc-lane 묶음 — 최근 1분 1건 ‖ lane-a: 완료 요약") {
			t.Fatalf("묶음+막힘: %q", s)
		}
	})

	t.Run("②주기에한메시지", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		w := startIdleWatch(t, bin, d, co, dgLine(now, "lane-a", "완료", "0123456789ab", "구현 끝", co, ""), "HC_LANE_COORD_DIGEST_SEC=12")
		for i, l := range []string{
			dgLine(now+1, "audit-lane-a", "판정", "-", "통과 0·0·2", co, ""),
			dgLine(now+2, "lane-a", "제출", "-", "제출 ID x", co, ""),
			dgLine(now+3, "lane-a", "착지", "fedcba9876543210", "PR #7", co, ""),
		} {
			ticks(t, d, 1)
			if s := w.sends(); len(s) != 0 {
				t.Fatalf("%d번째 사건 전에 보냄(주기 전): %q", i+2, s)
			}
			appendEv(t, w, l)
		}
		waitFor(t, "주기 뒤 묶음", func() bool { return len(w.sends()) > 0 })
		ticks(t, d, 2)
		s := w.sends()
		want := "hc-lane 묶음 — 최근 1분 4건 ‖ lane-a: 완료 01234567 구현 끝 → 감사 판정 통과 0·0·2 → 제출 제출 ID x → 착지 fedcba98 PR #7 ‖ → hc-fill status"
		if len(s) != 1 || s[0] != co+tab+want {
			t.Fatalf("한 메시지·그 레인 한 줄이어야:\n%q\n바람:\n%q", s, want)
		}
	})

	t.Run("③묶음먼저막힘뒤", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		w := startIdleWatch(t, bin, d, co, dgLine(now, "lane-a", "완료", "-", "요약", co, ""), "HC_LANE_COORD_DIGEST_SEC=3600")
		ticks(t, d, 2)
		if s := w.sends(); len(s) != 0 {
			t.Fatalf("주기 전 보냄: %q", s)
		}
		appendEv(t, w, dgLine(now+1, "lane-b", "막힘", "-", "까닭", co, ""))
		waitFor(t, "막힘", func() bool { return len(w.sends()) > 0 })
		ticks(t, d, 2)
		want := "hc-lane 묶음 — 최근 1분 1건 ‖ lane-a: 완료 요약 ‖ → hc-fill status ‖ hc-lane: lane-b 막힘 까닭 (t5)"
		if s := w.sends(); len(s) != 1 || s[0] != co+tab+want {
			t.Fatalf("한 번에 묶음 먼저·막힘 뒤:\n%q\n바람:\n%q", s, want)
		}
	})

	t.Run("④21건이면주기전", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		var b strings.Builder
		for i := 0; i < 20; i++ {
			b.WriteString(dgLine(now+int64(i), "lane-"+string(rune('a'+i)), "완료", "-", "요약", co, ""))
		}
		w := startIdleWatch(t, bin, d, co, b.String(), "HC_LANE_COORD_DIGEST_SEC=3600")
		ticks(t, d, 3)
		if s := w.sends(); len(s) != 0 {
			t.Fatalf("20건은 모아야: %q", s)
		}
		appendEv(t, w, dgLine(now+30, "lane-z", "착지", "-", "끝", co, ""))
		waitFor(t, "21건", func() bool { return len(w.sends()) > 0 })
		if s := w.sends(); len(s) != 1 || !strings.Contains(s[0], "hc-lane 묶음 — 최근 1분 21건") || strings.Count(s[0], " ‖ ") != 22 {
			t.Fatalf("21건 한 메시지: %q", s)
		}
	})

	t.Run("⑤다시떠도한번", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		evs := dgLine(now, "lane-a", "완료", "-", "요약", co, "") + dgLine(now+1, "lane-b", "제출", "-", "제출 ID y", co, "")
		w := startIdleWatch(t, bin, d, co, evs, "HC_LANE_COORD_DIGEST_SEC=3600")
		ticks(t, d, 2)
		if s := w.sends(); len(s) != 0 {
			t.Fatalf("모아야: %q", s)
		}
		if err := w.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "끝남", func() bool { return exited(w) })
		if b, _ := os.ReadFile(w.ed + "/seen." + ckey(co)); strings.Contains(string(b), "lane-a") {
			t.Fatalf("모은 사건이 커서에 적혔다(잃음): %q", b)
		}
		w2 := startIdleWatch(t, bin, d, co, evs, "HC_LANE_COORD_DIGEST_SEC=1")
		waitFor(t, "다시 뜬 watch가 보냄", func() bool { return len(w2.sends()) > 0 })
		ticks(t, d, 3)
		if s := w2.sends(); len(s) != 1 || !strings.Contains(s[0], "2건 ‖ lane-a: 완료 요약 ‖ lane-b: 제출 제출 ID y") {
			t.Fatalf("한 번만: %q", s)
		}
	})

	t.Run("⑥모르는종류는즉시", func(t *testing.T) {
		t.Parallel()
		d, co := setup(t)
		w := startIdleWatch(t, bin, d, co, dgLine(now, "lane-u", "새상태", "-", "요약", co, ""), "HC_LANE_COORD_DIGEST_SEC=3600")
		waitFor(t, "즉시", func() bool { return len(w.sends()) > 0 })
		if s := w.sends(); len(s) != 1 || s[0] != co+tab+"hc-lane: lane-u 새상태 요약 (t5)" {
			t.Fatalf("모르는 종류: %q", s)
		}
	})
}

// ⑦ 지휘 둘 → 묶음 상태가 watch(지휘)마다 따로 — 한쪽이 보내도 다른 쪽 묶음은 그대로 모인다(주인 거름은 wfilter — 그 시험들)
func TestDigestPerCoord(t *testing.T) {
	t.Setenv("HC_LANE_COORD_DIGEST_SEC", "3600")
	a := strings.TrimSuffix(dgLine(1, "lane-a", "완료", "-", "요약", "t1", ""), "\n")
	b := strings.TrimSuffix(dgLine(2, "lane-b", "완료", "-", "요약", "t2", ""), "\n")
	blk := strings.TrimSuffix(dgLine(3, "lane-c", "막힘", "-", "까닭", "t1", ""), "\n")
	e1, e2 := &ev{}, &ev{}
	if _, _, _, h := e1.digest([]string{a}); !h {
		t.Fatal("지휘1 모아야")
	}
	if _, _, _, h := e2.digest([]string{b}); !h {
		t.Fatal("지휘2 모아야")
	}
	dig, imm, _, h := e1.digest([]string{a, blk})
	if h || len(dig) != 1 || dig[0] != a || len(imm) != 1 || imm[0] != blk {
		t.Fatalf("지휘1 막힘 → 자기 묶음만: %v %q %q", h, dig, imm)
	}
	e1.dg.sent(dig)
	if len(e1.dg.seen) != 0 || len(e2.dg.seen) != 1 {
		t.Fatalf("보낸 쪽만 비워야: %v %v", e1.dg.seen, e2.dg.seen)
	}
	if _, _, _, h := e2.digest([]string{b}); !h {
		t.Fatal("지휘2는 여전히 모아야")
	}
}

// 따로 보내는 즉시 줄(hc ls 실패 등)이 끼어도 모인 묶음의 시계는 그대로 — 주기가 차면 보낸다
func TestDigestClockKept(t *testing.T) {
	t.Setenv("HC_LANE_COORD_DIGEST_SEC", "60")
	a := strings.TrimSuffix(dgLine(1, "lane-a", "완료", "-", "요약", "t1", ""), "\n")
	blk := strings.TrimSuffix(dgLine(2, "hc-lane-wait", "멈춤", "-", "hc ls 실패 3바퀴", "-", ""), "\n")
	e := &ev{}
	if _, _, _, h := e.digest([]string{a}); !h {
		t.Fatal("모아야")
	}
	if dig, imm, _, h := e.digest([]string{blk}); h || len(dig) != 0 || len(imm) != 1 {
		t.Fatalf("즉시 줄만: %v %q %q", h, dig, imm)
	}
	e.dg.seen[lineKey(a)] = time.Now().Add(-2 * time.Minute) // 처음 본 지 2분(주기 1분)
	if dig, _, since, h := e.digest([]string{a}); h || len(dig) != 1 || since < time.Minute {
		t.Fatalf("시계가 지워졌다: %v %q %v", h, dig, since)
	}
}

// 가르는 표(⑥·⑧ 포함) — 모르는 것은 즉시
func TestDigestable(t *testing.T) {
	c := func(lane, st, msg, key string) events.Event {
		return events.Cols(strings.TrimSuffix(dgLine(1, lane, st, "-", msg, "t1", key), "\n"))
	}
	for _, x := range []struct {
		e    events.Event
		want bool
		why  string
	}{
		{c("l", "완료", "끝", ""), true, "완료"},
		{c("audit-l", "판정", "통과", ""), true, "감사 판정"},
		{c("l", "제출", "x", ""), true, "제출"},
		{c("l", "착지", "x", ""), true, "착지"},
		{c("l", "중간", "진행", ""), true, "중간"},
		{c("l", "중간", "land 실패(종료 1)", ""), false, "실패 중간"},
		{c("l", "중간", "후속 못 넣음 x — 손으로", ""), false, "못 넣음"},
		{c("l", "막힘", "까닭", ""), false, "막힘"},
		{c("l", "주차", "x", ""), false, "주차는 표에 없음"},
		{c("l", "새상태", "x", ""), false, "모르는 상태"},
		{c("l", "멈춤", "done turn 3", ""), true, "멈춤 done"},
		{c("l", "멈춤", "idle turn 3", ""), true, "멈춤 idle"},
		{c("l", "멈춤", "뜻밖의 종료(정리됨) hc-sweep R2", ""), true, "정리 알림"},
		{c("l", "멈춤", "뜻밖의 종료(gone)", ""), false, "뜻밖의 종료"},
		{c("l", "멈춤", "blocked turn 3", ""), false, "blocked"},
		{c("l", "멈춤", "error turn 3", ""), false, "error"},
		{c("l", "멈춤", "연결 안 됨 m 2026-11-24", ""), false, "연결 안 됨"},
		{c("on-land-build", "중간", "착지 뒤 build 성공", ""), true, "착지 뒤 명령 성공"},
		{c("on-land-build", "막힘", "착지 뒤 build 실패", ""), false, "착지 뒤 명령 실패"},
		{c("hc-fill-advise", "중간", "손댈 것: x", events.AdviseAgentKey+"1"), true, "⑧ 운영 에이전트가 맡은 손댈 것"},
		{c("hc-fill-advise", "중간", "손댈 것: x", ""), false, "⑧ 판단만·끔이면 즉시"},
		{c("hc-fill-agent", "중간", "운영 에이전트 끝: x", ""), false, "운영 에이전트 끝 사건"},
		{c("hc-fill-agent", "막힘", "사람이 정한다: x", ""), false, "사람이 정한다"},
		{c("l", "완료", "사람이 정한다: x", ""), false, "사람이 정한다(레인)"},
		{c("hc-fill-idle", "중간", "자리 남음", ""), false, "그 밖 운영 사건"},
	} {
		if got := digestable(x.e); got != x.want {
			t.Errorf("%s: 묶음 %v, 바람 %v", x.why, got, x.want)
		}
	}
}

// advise-fixes ⑥: 착지 뒤 명령 성공(운영, 다음 명령 hc-fill status)이 든 묶음 — 끝 안내 「→ hc-fill status」가 정확히 한 번
// (고치기 전: 「… (m) → hc-fill status ‖ → hc-fill status」). 다른 다음 명령(hc-fill advise)은 그대로
func TestDigestTailOnce(t *testing.T) {
	l := func(lane, st, msg, key string) string {
		return strings.TrimSuffix(dgLine(1, lane, st, "-", msg, "t1", key), "\n")
	}
	got := digestText([]string{l("lane-a", "완료", "끝", ""), l("on-land-build", "중간", "착지 뒤 build 성공", ""),
		l("hc-fill-advise", "중간", "손댈 것: x", events.AdviseAgentKey+"1")}, time.Minute)
	if n := strings.Count(got, "→ hc-fill status"); n != 1 || !strings.HasSuffix(got, " ‖ → hc-fill status") {
		t.Errorf("끝 안내 %d번: %s", n, got)
	}
	if !strings.Contains(got, "착지 뒤 build 성공 (t5) ‖") || !strings.Contains(got, "(t5) → hc-fill advise ‖") {
		t.Errorf("운영 줄: %s", got)
	}
}
