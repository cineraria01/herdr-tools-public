//go:build !windows

package main

// merger-batch-culprit(2026-11-23 — 11-22 하루 묶음 실패 사슬 6개 가운데 4개가 범인이 맨 앞이라 [a,b,c]→[a,b]→[a] 검증 세 바퀴를 태웠다):
// 묶음이 실패하면 다음 바퀴에 앞 k−1을 검증하는 동안 「맨 앞 혼자」를 추측 검증 기계에 함께 맡기고, 그 추측이 같은 기준에서 시험 실패
// (실패한 시험 줄이 있는 실패)면 병합기 검증 한 바퀴를 더 쓰지 않고 반송한다.

import (
	"os"
	"strings"
	"testing"
)

// culpritFx: specFx + 검증 스크립트가 bad.txt면 시험 실패 한 줄을 찍는다(failLine 꼴) — failLine=false면 줄 없이 종료 1(가를 수 없음)
func culpritFx(t *testing.T, failLine bool) *sfx {
	if failLine {
		return culpritOut(t, "--- FAIL: TestBad (0.00s)")
	}
	return specFx(t)
}

// culpritOut: specFx + 검증 스크립트가 bad.txt면 out을 찍고 종료 1
func culpritOut(t *testing.T, out string) *sfx {
	s := specFx(t)
	os.WriteFile(s.seed+"/.claude/herdr-verify.sh", []byte("#!/bin/bash\nif [ -e bad.txt ]; then cat <<'EOF'\n"+out+"\nEOF\nexit 1; fi\n"), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "verify prints fail output")
	s.git(s.seed, "push", "-q", "origin", "main")
	return s
}

func (s *sfx) specGroupOf(id string) string {
	return s.git(s.seed, "--git-dir="+s.origin, "log", "-1", "--format=%s", specRefs+id)
}

// 범인 맨 앞 [a,b,c]: 지금은 병합기 검증 3번 뒤 a 반송 — 고친 뒤 2번 안에 a 반송, b·c 착지
func TestCulpritFrontRejectedEarly(t *testing.T) {
	oldFront(t)
	s := culpritFx(t, true)
	_, sa := s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.submit(1000000002, "b", "보통")
	s.submit(1000000003, "c", "보통")
	s.round(3)
	if d := s.dispatched(); len(d) != 0 {
		t.Fatalf("첫 묶음은 그대로(띄울 것 없음): %v", d)
	}
	s.round(3) // [a,b] 를 직접 검증하는 동안 a 혼자를 검증 기계에
	d := s.dispatched()
	if len(d) == 0 || d[0][0] != "v1" || !strings.Contains(s.specGroupOf(d[0][1]), "group="+sa+" ") {
		t.Fatalf("맨 앞 혼자(a)를 먼저 v1에: %v", d)
	}
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a" {
		t.Fatalf("[a,b,c]·[a,b] 직접 검증: %s", got)
	}
	if rc := s.verifyOn(d[0]); rc != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("a 혼자 추측 실패 기록(종료 %d)", rc)
	}
	solo := jrows(t, "맨 앞 혼자 추측")
	if len(solo) != 1 || solo[0][2] != "a" || !strings.Contains(solo[0][11], "v1") {
		t.Fatalf("맨 앞 혼자를 맡긴 바퀴는 일지 한 줄:\n%s", jdump())
	}
	s.round(3)
	if !s.has("refs/merge-failed/1000000001-a") || s.onMain("bad.txt") {
		t.Fatalf("a는 검증 한 바퀴 더 없이 반송:\n%s", jdump())
	}
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a" {
		t.Fatalf("병합기 검증은 2번: %s", got)
	}
	back := jrows(t, "돌려보냄")
	if len(back) != 1 || back[0][2] != "a" || !strings.Contains(back[0][11], "착지 실패(종료 1") ||
		!strings.Contains(back[0][11], "--- FAIL: TestBad") || !strings.Contains(back[0][11], "v1") {
		t.Fatalf("반송 줄 = 지금 꼴(착지 실패(종료 …)) + 추측 근거:\n%s", jdump())
	}
	if _, err := os.Stat(s.clone + "/.git/hc-merger/cap"); err == nil {
		t.Fatal("반송 뒤 묶음 크기는 원래대로")
	}
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a verify|b" || !s.onMain("b.txt") || !s.onMain("c.txt") {
		t.Fatalf("b·c는 한 묶음으로 착지: %s", got)
	}
}

// 범인이 마지막 [a,b,c(범인)]: 병합기 검증 횟수는 지금과 같다(3). 묶음 실패가 없던 추측 실패(c)는 지금처럼 직접 검증해 반송
func TestCulpritLastSameCount(t *testing.T) {
	oldFront(t)
	s := culpritFx(t, true)
	s.submit(1000000001, "a", "보통")
	s.submit(1000000002, "b", "보통")
	s.sub2(1000000003, "c", "", "", "bad.txt", "c\n", false)
	s.round(3)
	s.round(3) // [a,b] 착지 + a 혼자·[c] 추측
	if !s.onMain("b.txt") {
		t.Fatal("[a,b] 착지")
	}
	for _, x := range s.dispatched() {
		s.verifyOn(x)
	}
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a verify|c" || !s.has("refs/merge-failed/1000000003-c") {
		t.Fatalf("검증 3번(지금과 같음), c는 직접 검증해 반송: %s", got)
	}
}

// 추측이 「모름」(시험 실패와 환경 문제를 가를 수 없음 — 실패한 시험 줄 없이 종료 1)이면 반송 근거로 쓰지 않고 지금 길
func TestCulpritUnknownFallsBack(t *testing.T) {
	oldFront(t)
	s := culpritFx(t, false)
	s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.submit(1000000002, "b", "보통")
	s.submit(1000000003, "c", "보통")
	s.round(3)
	s.round(3)
	d := s.dispatched()
	if len(d) == 0 {
		t.Fatal("a 혼자 띄움")
	}
	s.verifyOn(d[0])
	if !s.has(specFails + d[0][1]) {
		t.Fatal("전제: 추측 실패 기록")
	}
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a verify|a" || !s.has("refs/merge-failed/1000000001-a") {
		t.Fatalf("가를 수 없으면 병합기가 직접 검증해 반송: %s", got)
	}
}

// 검증 기계 설정이 없는 줄: 지금과 같다(병합기 검증 3번, 추측 참조·맨 앞 혼자 줄 없음, 일지 결과 차례 그대로)
func TestCulpritNoVerifiersUnchanged(t *testing.T) {
	f := jFx(t)
	f.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	f.submit(1000000002, "b", "보통")
	f.submit(1000000003, "c", "보통")
	for i := 0; i < 4; i++ {
		f.merger().round()
	}
	b, _ := os.ReadFile(f.dir + "/land.log")
	if got := strings.Join(strings.Fields(string(b)), " "); got != "land|a land|a land|a land|b" {
		t.Fatalf("검증 차례: %s", got)
	}
	var res []string
	for _, l := range strings.Split(strings.TrimSpace(jdump()), "\n") {
		if c := strings.Split(l, "\t"); len(c) == 12 {
			res = append(res, c[2]+":"+c[9])
		}
	}
	want := "a:묶음 실패 b:묶음 실패 c:묶음 실패 a:묶음 실패 b:묶음 실패 a:돌려보냄 b:착지 c:착지"
	if strings.Join(res, " ") != want {
		t.Fatalf("일지 결과:\n%s", jdump())
	}
	o, _ := os.ReadFile(f.dir + "/o.git/packed-refs")
	if strings.Contains(string(o), specRefs) || f.has("refs/herdr/spec-verifiers") {
		t.Fatal("추측 참조 없음")
	}
}

// merger-culprit-env-gap(감사 atoy3kt-wnph24 낮음 2): 맨 앞 혼자 추측이 환경 실패(go test 「FAIL <꾸러미> [setup failed]」 — 네트워크·
// 모듈 캐시)뿐이면 test=0(모름) — 반송 근거로 쓰지 않고 병합기가 직접 검증한다(고치기 전에는 test=1로 적혀 검증 없이 반송)
func TestCulpritSetupFailedFallsBack(t *testing.T) {
	oldFront(t)
	s := culpritOut(t, "go: downloading example.invalid/x v1.0.0\n# m/a\na/a.go:2:8: module lookup disabled by GOPROXY=off\nFAIL\tm/a [setup failed]\nFAIL")
	s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.submit(1000000002, "b", "보통")
	s.submit(1000000003, "c", "보통")
	s.round(3)
	s.round(3)
	d := s.dispatched()
	if len(d) == 0 {
		t.Fatal("a 혼자 띄움")
	}
	s.verifyOn(d[0])
	if got := s.git(s.seed, "--git-dir="+s.origin, "log", "-1", "--format=%s", specFails+d[0][1]); !strings.Contains(got, " exit=1 test=0") {
		t.Fatalf("환경 실패뿐이면 test=0: %s", got)
	}
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a verify|a" || !s.has("refs/merge-failed/1000000001-a") {
		t.Fatalf("병합기가 직접 검증해 반송: %s", got)
	}
}

// firstFail(추측 검증의 시험 실패 근거): 환경 실패 줄은 세지 않고, 진짜 시험 실패·빌드 실패는 지금처럼(꼴은 go1.27 실측)
func TestFirstFailEnvLines(t *testing.T) {
	setup := "go: downloading example.invalid/x v1.0.0\n# m/a\na/a.go:2:8: module lookup disabled by GOPROXY=off\nFAIL\tm/a [setup failed]\n"
	timeout := "panic: test timed out after 1s\n\trunning tests:\n\t\tTestE (1s)\nFAIL\tm/e\t1.282s\n"
	for _, c := range []struct{ name, out, want string }{
		{"setup 뿐", setup + "FAIL\n", ""},
		{"go.sum 없음", "# m/d\nd/d.go:2:8: missing go.sum entry\nFAIL\tm/d [setup failed]\nFAIL\n", ""},
		{"시험 실패", "=== RUN   TestX\n--- FAIL: TestX (0.00s)\nFAIL\nFAIL\tm/c\t0.1s\nFAIL\n", "--- FAIL: TestX (0.00s)"},
		{"빌드 실패", "# m/b [m/b.test]\nb/b.go:2:23: cannot use\nFAIL\tm/b [build failed]\nFAIL\n", "FAIL\tm/b [build failed]"},
		{"setup + 시험 실패", setup + "--- FAIL: TestX (0.00s)\nFAIL\nFAIL\tm/c\t0.2s\nFAIL\n", "--- FAIL: TestX (0.00s)"},
		{"setup + 빌드 실패", setup + "# m/b [m/b.test]\nFAIL\tm/b [build failed]\nFAIL\n", "FAIL\tm/b [build failed]"},
		{"시간 넘김 뿐", timeout + "FAIL\n", ""},
		{"시간 넘김 + 다른 꾸러미 시험 실패", timeout + "--- FAIL: TestY (0.00s)\nFAIL\nFAIL\tm/f\t0.1s\nFAIL\n", "--- FAIL: TestY (0.00s)"},
		{"꾸러미 실패만(TestMain os.Exit 등)", "FAIL\tm/g\t0.1s\nFAIL\n", "FAIL\tm/g\t0.1s"},
		{"bun", "(fail) x > y [1.00ms]\n", "(fail) x > y [1.00ms]"},
		{"맨 FAIL 뿐", "FAIL\n", ""},
	} {
		p := t.TempDir() + "/log"
		if err := os.WriteFile(p, []byte(c.out), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := firstFail(p, 0); got != flat(c.want) {
			t.Errorf("%s: %q, 바람 %q", c.name, got, flat(c.want))
		}
	}
}
