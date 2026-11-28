package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ht-retry-class: 결정적 실패는 hc-lane이 종료 11 하나로 돌려주고(임대 「이미 돎」 포함), 그 앞 「풀리면 다시: 임대 <값>」이 있으면
// hc-fill이 바퀴마다 그 임대를 보아 바뀌거나 없어지면 스스로 다시 띄운다. 차림은 reject_test.go rejSetup

const lsA = "aaaa1111 hc-lane lease l1 w1 2026-11-28T03:54:00Z"

func (x *rejT) held() {
	x.t.Setenv("HC_T_HELD", "1")
	x.t.Setenv("HC_T_RC_here", "11")
	rejW(x.t, x.d+"/lease", lsA+"\n")
}

// ①② 임대를 남이 쥠 → 첫 바퀴에 알림 한 번(쥔 곳·놓는 명령·스스로 다시 띄움), 열 바퀴 동안 다시 안 띄움 → 임대가 없어지면 다음 바퀴에 다시 뜸·풀림 한 번
func TestRejectLeaseWatch(t *testing.T) {
	x := rejSetup(t)
	x.held()
	for i := 0; i < 11; i++ { // 첫 바퀴 + 열 바퀴 (같은 바퀴 두 번 부른 꼴도 된다)
		x.place()
	}
	if got := x.read("calls"); got != "here\n" {
		t.Fatalf("시도 %q, 바람 첫 바퀴 here 한 번(far로도, 다음 바퀴에도 안 띄움)", got)
	}
	r := rejRow("l1", "r1", "reject")
	if r == nil || r.F(rjLease) != "aaaa1111" || !strings.Contains(r.F(rjWhy), "w1의 실행이") {
		t.Fatalf("기록 %q, 바람 임대 값 aaaa1111", r)
	}
	rep := x.read("reports")
	if strings.Count(rep, "report 막힘") != 1 || !strings.Contains(rep, "hc-lane lease drop --repo "+x.d+"/r --name l1") ||
		!strings.Contains(rep, "임대가 놓이거나 바뀌면 hc-fill이 스스로 다시 띄운다") || !strings.Contains(rep, "w1의 실행이 2026-11-28T03:54:00Z부터") {
		t.Fatalf("사건 %q, 바람 한 번(쥔 곳·놓는 명령·스스로 다시)", rep)
	}
	if n := strings.Count(x.read("lease-calls"), "lease show"); n != 10 {
		t.Fatalf("임대 조회 %d번, 바람 멈춘 뒤 바퀴마다 한 번(10)", n)
	}
	// 그 실행이 끝나 임대가 놓였다(병합기 착지·clean·손 drop — 차례와 상관없이 값만 본다)
	rejW(t, x.d+"/lease", "-\n")
	t.Setenv("HC_T_HELD", "")
	t.Setenv("HC_T_RC_here", "1")
	x.place()
	if got := x.read("calls"); got != "here\nhere\nfar\n" {
		t.Fatalf("풀린 뒤 시도 %q, 바람 다시 띄움", got)
	}
	if rejWhy("l1", "r1") != "" {
		t.Fatal("풀린 뒤에도 멈춤")
	}
	up := "--key " + r.F(rjTold) + ":up"
	if rep := x.read("reports"); strings.Count(rep, up) != 1 || !strings.Contains(rep, "report 중간") || !strings.Contains(rep, "임대가 놓였다") {
		t.Fatalf("풀림 사건 %q, 바람 %s 한 번", rep, up)
	}
}

// 바퀴 순서 반대(hc-fill이 한참 안 돎 — 그 사이 임대가 다른 실행으로 넘어감) · 이미 쌓인 9칸 기록: 값이 바뀌면 풀고 다시 띄워,
// 새 주인이면 새 기록·새 알림 한 번
func TestRejectLeaseChanged(t *testing.T) {
	x := rejSetup(t)
	x.held()
	old := strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)
	rejW(t, x.d+"/reject.tsv", strings.Join([]string{"r1", "l1", "reject", old, old, "hf:l1:reject:r1:" + old, "here", "이미 돎 (@here)", "9999ffff"}, "\t")+"\n")
	x.place()
	if got := x.read("calls"); got != "here\n" {
		t.Fatalf("시도 %q, 바람 값이 바뀌어 한 번 다시", got)
	}
	r := rejRow("l1", "r1", "reject")
	if r == nil || r.F(rjLease) != "aaaa1111" || r.F(rjFirst) == old {
		t.Fatalf("새 기록 %q", r)
	}
	rep := x.read("reports")
	if strings.Count(rep, "hf:l1:reject:r1:"+old+":up") != 1 || strings.Count(rep, "report 막힘") != 1 || !strings.Contains(rep, "임대가 바뀌었다") {
		t.Fatalf("사건 %q, 바람 옛 막힘 풀림 한 번 + 새 막힘 한 번", rep)
	}
	for i := 0; i < 3; i++ {
		x.place()
	}
	if got := x.read("calls"); got != "here\n" || strings.Count(x.read("reports"), "report 막힘") != 1 {
		t.Fatalf("새 주인 뒤 되풀이 %q · %q", got, x.read("reports"))
	}
}

// 외부 가정: 임대를 못 읽음(모름)은 풂의 근거가 아니다 — 멈춤 그대로, 알림 되풀이 없음
func TestRejectLeaseUnknownStays(t *testing.T) {
	x := rejSetup(t)
	x.held()
	x.place()
	t.Setenv("HC_T_LEASE_RC", "1")
	for i := 0; i < 5; i++ {
		x.place()
	}
	t.Setenv("HC_T_LEASE_RC", "")
	rejW(t, x.d+"/lease", "fatal: 판마다 다른 원격 오류 글\n") // 값 꼴이 아닌 답도 모름
	x.place()
	if got := x.read("calls"); got != "here\n" || rejWhy("l1", "r1") == "" || strings.Count(x.read("reports"), "report") != 1 {
		t.Fatalf("모름에 풂 %q · %q", got, x.read("reports"))
	}
}

// 거절이 난 기계(far)가 임대를 못 읽으면 이 기계 클론으로 한 번 더 읽는다(임대는 원격 참조 — 감사 atp7x2b-3xl3b2 낮음 1)
func TestRejectLeaseFallbackLocal(t *testing.T) {
	x := rejSetup(t)
	old := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	rejW(t, x.d+"/reject.tsv", strings.Join([]string{"r1", "l1", "reject", old, old, "hf:l1:reject:r1:" + old, "far", "이미 돎 (@far)", "aaaa1111"}, "\t")+"\n")
	rejW(t, x.d+"/lease", "-\n")
	t.Setenv("HC_T_LEASE_ON_FAIL", "1")
	x.place()
	if lc := x.read("lease-calls"); lc != "lease show --on far --repo /far/r --name l1\nlease show --repo "+x.d+"/r --name l1\n" || rejWhy("l1", "r1") != "" || x.read("calls") == "" {
		t.Fatalf("이 기계로 다시 읽지 않음 %q · 멈춤 %q", lc, rejWhy("l1", "r1"))
	}
}

// 이미 쌓인 상태: 옛 꼴 거절 기록(8칸 — 임대 값 없음)은 지금처럼 retry를 기다린다(임대를 보지 않음)
func TestRejectOldRowNoWatch(t *testing.T) {
	x := rejSetup(t)
	old := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	rejW(t, x.d+"/reject.tsv", strings.Join([]string{"r1", "l1", "reject", old, old, "hf:l1:reject:r1:" + old, "here", "이미 돎 (@here)"}, "\t")+"\n")
	for i := 0; i < 3; i++ {
		x.place()
	}
	if x.read("calls") != "" || x.read("lease-calls") != "" || x.read("reports") != "" {
		t.Fatalf("옛 기록에 움직임 %q · %q · %q", x.read("calls"), x.read("lease-calls"), x.read("reports"))
	}
}

// 구분은 종료 코드만 — 같은 글(「이미 돎」·「풀리면 다시」)이라도 종료 1이면 일시(지금 그대로), 옛 판 hc-lane의 10도 일시(안전 쪽)
func TestRejectClassByCodeOnly(t *testing.T) {
	for _, rc := range []string{"1", "10"} {
		x := rejSetup(t)
		x.held()
		t.Setenv("HC_T_RC_here", rc)
		t.Setenv("HC_T_RC_far", rc)
		x.place()
		if got := x.read("calls"); got != "here\nfar\n" || rejWhy("l1", "r1") != "" || x.read("lease-calls") != "" {
			t.Fatalf("종료 %s: 시도 %q · 멈춤 %q", rc, got, rejWhy("l1", "r1"))
		}
		if r := rejRow("l1", "r1", "fail"); r == nil {
			t.Fatalf("종료 %s: 일시 실패로 안 셈", rc)
		}
	}
}

// retry로 풀면 알린 막힘의 풀림(:up) 한 번
func TestRejectRetryUp(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	x.place()
	key := rejRow("l1", "r1", "reject").F(rjTold)
	if rc := withQ(func() { retryCmd([]string{"l1"}) }); rc != 0 {
		t.Fatalf("retry 종료 %d", rc)
	}
	if rep := x.read("reports"); strings.Count(rep, "--key "+key+":up") != 1 || !strings.Contains(rep, "지휘가 풀었다") {
		t.Fatalf("풀림 사건 %q", rep)
	}
}

// ⑥ rm: 그 줄 실행의 터미널이 끝났음이 확인되고, 지금 임대가 그 줄 기계의 것이며 그 기계에 같은 이름의 살아 있는 터미널이 없을 때만
// 본 값으로(--expect) 놓는다 — 살아 있음·모름·다른 기계의 임대·같은 이름이 살아 있음(손으로 이어 띄운 실행)·그 사이 바뀜·없음은 그대로
func TestRmLease(t *testing.T) {
	const farL = "aaaa1111 hc-lane lease l1 far 2026-11-28T03:54:00Z"
	for _, c := range []struct {
		name, lease, live, getrc, say string
		drop                          bool
	}{
		{"끝남", farL, "", "", "임대 놓음 refs/hc-lanes/l1", true},
		{"줄 터미널 살아 있음", farL, "far/t5", "", "임대 그대로", false},
		{"모름", farL, "", "1", "임대 그대로", false},
		{"다른 기계의 임대", lsA, "", "", "다른 실행", false},
		{"같은 기계 같은 이름 살아 있음", farL, "far/l1", "", "살아 있다", false},
		{"그 사이 바뀜", farL, "", "", "그 사이", false},
		{"임대 없음", "-", "", "", "임대 없음", false},
	} {
		x := rejSetup(t)
		rejW(t, Q, strings.Join([]string{"l1", "p", x.d + "/brief.md", "-", "-", "up", "far", "far/t5", "-", "t", "-", "t9", "normal", "-", "-", "-", "r1"}, "\t")+"\n")
		rejW(t, x.d+"/lease", c.lease+"\n")
		t.Setenv("HC_T_LIVE", c.live)
		t.Setenv("HC_T_GET_RC", c.getrc)
		if c.name == "그 사이 바뀜" { // show 뒤 drop 전에 다른 실행이 잡은 꼴 — 가짜 drop이 기대값과 다르면 3
			t.Setenv("HC_T_DROP_EXPECT_MISS", "1")
		}
		var out string
		if rc := withQ(func() { out = captured(t, func() { rmCmd([]string{"l1"}) }) }); rc != 0 {
			t.Fatalf("%s: rm 종료 %d", c.name, rc)
		}
		_, err := os.Stat(x.d + "/lease")
		dropped := err != nil
		if dropped != c.drop || !strings.Contains(out, c.say) {
			t.Fatalf("%s: 놓음 %v(바람 %v) · 출력 %q · 임대 호출 %q", c.name, dropped, c.drop, out, x.read("lease-calls"))
		}
		if c.drop && !strings.Contains(x.read("lease-calls"), "lease drop --on far --repo /far/r --name l1 --expect aaaa1111\n") {
			t.Fatalf("%s: 본 값으로 놓지 않음 %q", c.name, x.read("lease-calls"))
		}
		if qHasName("l1") {
			t.Fatalf("%s: 줄이 남음", c.name)
		}
	}
}
