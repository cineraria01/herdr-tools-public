//go:build !windows

package main

// ht-merger-culprit-parallel: 묶음 k가 시험 실패하면 다음 바퀴(cap)에 앞 무리 k−1·k−2·…·1을 한가한 검증 기계에 함께 맡기고, 통과한 가장 긴
// 앞 무리 j를 그 기록으로 착지, 범인(앞 j+1의 j+1번째)은 다른 기계에서 한 번 더 확인한 뒤 반송한다. 틀은 specrun_test.go(specFx)·
// culprit_test.go(culpritFx — bad.txt면 시험 실패 한 줄).

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

// parFx: culpritFx + 검증 기계 n대(v1…vn). 검증 스크립트는 PASS_ALL이 있으면 bad.txt가 있어도 통과(흔들리는 시험)
func parFx(t *testing.T, n int) *sfx {
	s := culpritOut(t, "--- FAIL: TestBad (0.00s)")
	os.WriteFile(s.seed+"/.claude/herdr-verify.sh", []byte("#!/bin/bash\n[ -n \"$PASS_ALL\" ] && exit 0\nif [ -e bad.txt ]; then echo '--- FAIL: TestBad (0.00s)'; exit 1; fi\n"), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "verify flaky knob")
	s.git(s.seed, "push", "-q", "origin", "main")
	var vs []verified.Verifier
	for i := 1; i <= n; i++ {
		v := "v" + string(rune('0'+i))
		if _, err := os.Stat(s.dir + "/" + v); err != nil {
			s.git(s.dir, "clone", "-q", s.origin, s.dir+"/"+v)
		}
		vs = append(vs, verified.Verifier{Machine: v, Path: s.dir + "/" + v})
	}
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := s.git(s.seed, "commit-tree", et, "-m", verified.FormatVerifiers(vs))
	s.git(s.seed, "push", "-q", "-f", "origin", vc+":"+verified.VerifiersRef)
	return s
}

// four: a·b·<bad 자리>·d 네 제출(bad 자리는 1부터) — 제출 SHA를 차례로
func (s *sfx) four(bad int) []string {
	var o []string
	for i, l := range []string{"a", "b", "c", "d"} {
		if i+1 == bad {
			_, sha := s.sub2(1000000001+i, l, "", "", "bad.txt", l+"\n", false)
			o = append(o, sha)
			continue
		}
		_, sha, _, _ := s.submit(1000000001+i, l, "보통")
		o = append(o, sha)
	}
	return o
}

// failBatch4: 묶음 4를 검증 기계에서 시험 실패시키고 줄인 바퀴(cap 3) 앞까지 — 띄운 수를 돌려준다
func (s *sfx) failBatch4() int {
	s.t.Helper()
	s.round(4)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		s.t.Fatalf("묶음 4 추측 실패 기록: %v", d)
	}
	s.round(4) // 묶음 시험 실패 → 마지막 빼고 다시(cap 3)
	if b, _ := os.ReadFile(s.clone + "/.git/hc-merger/cap"); strings.TrimSpace(string(b)) != "3" {
		s.t.Fatalf("cap 3: %q\n%s", b, jdump())
	}
	return len(s.dispatched())
}

// prefixOf: 띄운 추측의 무리 열쇠가 shas와 같은가
func (s *sfx) prefixOf(id string, shas []string) bool {
	return strings.Contains(s.specGroupOf(id), "group="+strings.Join(shas, "+")+" ")
}

func noLocalVerify(t *testing.T, s *sfx) {
	t.Helper()
	for _, l := range s.lines(s.landlog) {
		if strings.HasPrefix(l, "verify|") {
			t.Fatalf("병합기 기계 검증 0번: %v\n%s", s.lines(s.landlog), jdump())
		}
	}
}

// ① 묶음 4·범인 3번째·한가한 기계 3 → 한 바퀴에 앞 3·2·1을 함께, 앞 2 착지, 3번째만 다른 기계에서 확인 뒤 반송, 4번째는 큐에 남음
func TestParCulpritMiddle(t *testing.T) {
	s := parFx(t, 3)
	sh := s.four(3)
	n0 := s.failBatch4()
	s.round(4)
	d := s.dispatched()[n0:]
	if len(d) != 3 {
		t.Fatalf("① 한 바퀴에 앞 무리 셋: %v\n%s", d, jdump())
	}
	for j := 1; j <= 3; j++ {
		found := false
		for _, x := range d {
			found = found || s.prefixOf(x[1], sh[:j])
		}
		if !found {
			t.Fatalf("① 앞 %d을 맡김: %v", j, d)
		}
	}
	if p := jrows(t, jPrefix); len(p) != 3 || !strings.Contains(p[0][11], "앞 2(") || !strings.Contains(p[0][11], "앞 3(") {
		t.Fatalf("① 앞 무리 추측 줄(구성원마다):\n%s", jdump())
	}
	failMach := ""
	for _, x := range d {
		s.verifyOn(x)
		if s.prefixOf(x[1], sh[:3]) {
			failMach = x[0]
		}
	}
	s.round(4) // 앞 2 통과·앞 3 시험 실패 → 앞 2로 줄이고 범인 c를 다른 기계에서 확인
	bf := jrows(t, jBatchFail)
	if len(bf) == 0 || !strings.Contains(bf[len(bf)-1][11], "앞 2 착지 · 범인 c") {
		t.Fatalf("① 묶음 실패 줄에 「앞 2 착지 · 범인 c」:\n%s", jdump())
	}
	rc := s.dispatched()
	chk := rc[len(rc)-1]
	if chk[0] == failMach || !s.prefixOf(chk[1], sh[2:3]) {
		t.Fatalf("① 범인 확인은 첫 실패 기계(%s)가 아닌 곳에 c 혼자: %v", failMach, chk)
	}
	s.round(4)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" || !s.onMain("b.txt") || s.onMain("c.txt") {
		t.Fatalf("① 앞 2(a·b)가 기록으로 착지: %q", got)
	}
	s.verifyOn(chk)
	s.round(4)
	if !s.has("refs/merge-failed/1000000003-c") || s.onMain("bad.txt") {
		t.Fatalf("① 범인 c 반송:\n%s", jdump())
	}
	back := jrows(t, "돌려보냄")
	if len(back) != 1 || !strings.Contains(back[0][11], failMach+"·"+chk[0]) {
		t.Fatalf("① 반송 까닭에 두 기계(%s·%s):\n%s", failMach, chk[0], jdump())
	}
	if !s.has("refs/merge-queue/1000000004-d") || s.has("refs/merge-failed/1000000004-d") {
		t.Fatal("① 4번째 d는 큐에 남는다")
	}
	s.drive(4, 4)
	if !s.onMain("d.txt") {
		t.Fatalf("① d 착지:\n%s", jdump())
	}
	noLocalVerify(t, s)
}

// ② 범인 마지막 → 앞 3 착지
func TestParCulpritLast(t *testing.T) {
	s := parFx(t, 3)
	s.four(4)
	n0 := s.failBatch4()
	s.round(4)
	d := s.dispatched()[n0:]
	if len(d) != 3 {
		t.Fatalf("② 앞 무리 셋: %v", d)
	}
	for _, x := range d {
		s.verifyOn(x)
	}
	s.round(4)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" || !s.onMain("c.txt") || s.onMain("d.txt") {
		t.Fatalf("② 앞 3(a·b·c) 착지: %q", got)
	}
	noLocalVerify(t, s)
}

// ③ 범인 맨 앞 → 지금 맨 앞 혼자 길과 같은 결과(a 반송, 나머지 착지)
func TestParCulpritFront(t *testing.T) {
	s := parFx(t, 3)
	s.four(1)
	n0 := s.failBatch4()
	s.round(4)
	for _, x := range s.dispatched()[n0:] {
		s.verifyOn(x)
	}
	s.drive(4, 8)
	if !s.has("refs/merge-failed/1000000001-a") || s.onMain("bad.txt") || !s.onMain("b.txt") || !s.onMain("d.txt") {
		t.Fatalf("③ a 반송·b·c·d 착지:\n%s", jdump())
	}
	back := jrows(t, "돌려보냄")
	if len(back) != 1 || !strings.Contains(back[0][11], "맨 앞 혼자") {
		t.Fatalf("③ 반송은 지금 맨 앞 혼자 꼴:\n%s", jdump())
	}
	noLocalVerify(t, s)
}

// ④ 한가한 기계 1 → 이분 차례(가운데 앞 2 먼저, 통과하면 앞 3)
func TestParCulpritOneMachineBisect(t *testing.T) {
	s := parFx(t, 2)
	sh := s.four(3)
	n0 := s.failBatch4()
	s.idle("v2", verifyBusy)
	s.round(4)
	d := s.dispatched()[n0:]
	if len(d) != 1 || !s.prefixOf(d[0][1], sh[:2]) || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("④ 가운데(앞 2) 하나만, 착지 없음: %v %v", d, s.lines(s.landlog))
	}
	s.verifyOn(d[0])
	s.round(4)
	d = s.dispatched()[n0:]
	if len(d) != 2 || !s.prefixOf(d[1][1], sh[:3]) {
		t.Fatalf("④ 앞 2 통과 → 앞 3: %v", d)
	}
	s.verifyOn(d[1])
	s.round(4) // 앞 2 통과·앞 3 실패 → 범인 c
	s.round(4)
	if !s.onMain("b.txt") || s.onMain("c.txt") {
		t.Fatalf("④ 앞 2 착지:\n%s", jdump())
	}
	noLocalVerify(t, s)
}

// ⑤ 손잡이 serial → 지금 길(맨 앞 k−1 + 맨 앞 혼자만, 앞 무리 줄 없음)
func TestParCulpritSerialKnob(t *testing.T) {
	s := parFx(t, 3)
	t.Setenv("HC_LANE_MERGER_CULPRIT", "serial")
	sh := s.four(3)
	n0 := s.failBatch4()
	s.round(4)
	d := s.dispatched()[n0:]
	if len(d) != 3 || !s.prefixOf(d[0][1], sh[:3]) || !s.prefixOf(d[1][1], sh[:1]) || len(jrows(t, jPrefix)) != 0 || len(jrows(t, jSolo)) != 1 {
		t.Fatalf("⑤ 지금 길: 맨 앞 3·혼자·뒤 무리 — %v\n%s", d, jdump())
	}
}

// ⑤ 한가한 기계 0 → 지금 길(병합기가 직접 검증)
func TestParCulpritNoIdle(t *testing.T) {
	s := parFx(t, 2)
	s.four(3)
	n0 := s.failBatch4()
	s.idle("v1", verifyBusy)
	s.idle("v2", verifyBusy)
	s.round(4)
	if d := s.dispatched()[n0:]; len(d) != 0 || len(jrows(t, jPrefix)) != 0 {
		t.Fatalf("⑤ 맡길 기계 없음: %v", d)
	}
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" {
		t.Fatalf("⑤ 지금 길 — 병합기가 맨 앞 3을 직접 검증: %q", got)
	}
}

// ⑥ 시험 실패(test=1)가 아닌 것(모름·환경 실패·바쁨·도는 중·충돌 — pOther)은 범인 근거가 아니다
func TestCulpritBounds(t *testing.T) {
	P, F, O := pPass, pFail, pOther
	for _, c := range []struct {
		st     []int
		lo, hi int
	}{
		{[]int{P, P, F}, 2, 3},
		{[]int{P, P, P}, 3, 4},
		{[]int{F, F, F}, 0, 1},
		{[]int{O, P, F}, 2, 3},
		{[]int{O, F, F}, 0, 2}, // 앞 1이 모름 — 범인 못 가름(앞 1로 줄여 지금 길)
		{[]int{P, O, F}, 1, 3}, // 앞 2가 모름 — 범인 못 가름
		{[]int{P, O, O}, 1, 4},
		{[]int{O, O, O}, 0, 4},
		{[]int{P, F, P}, 3, 4}, // 흔들림 — 통과한 가장 긴 앞 무리
	} {
		if lo, hi := culpritBounds(c.st); lo != c.lo || hi != c.hi {
			t.Errorf("%v: lo %d hi %d, 바람 %d %d", c.st, lo, hi, c.lo, c.hi)
		}
	}
	for _, c := range []struct {
		lo, hi int
		want   string
	}{{0, 4, "[2 1 3]"}, {0, 3, "[1 2]"}, {2, 4, "[3]"}, {0, 8, "[4 2 6 1 3 5 7]"}, {1, 2, "[]"}} {
		if got := fmt.Sprint(bisect(c.lo, c.hi)); got != c.want {
			t.Errorf("bisect(%d,%d) = %s, 바람 %s", c.lo, c.hi, got, c.want)
		}
	}
}

// 감사 atp3lz5-lg7y9e 중간 1: 맨 앞이 안 맡겨진 채 앞 무리가 merger_front_wait를 넘겨 도는데 진행 중(spec 터미널 살아 있음·로그 방금 갱신)이면
// 병합기가 범인을 품은 맨 앞을 직접 검증하지 않고 더 기다린다(frontOver의 specFresh와 같은 규칙). 갱신이 없으면 지금 길(직접 검증)
func overPrefix(t *testing.T, fresh bool) *sfx {
	s := parFx(t, 2)
	sh := s.four(3)
	n0 := s.failBatch4()
	s.idle("v2", verifyBusy)
	s.round(4)
	d := s.dispatched()[n0:]
	if len(d) != 1 || !s.prefixOf(d[0][1], sh[:2]) {
		t.Fatalf("앞 2만 v1에: %v", d)
	}
	s.age(d[0][1], 700)
	if fresh {
		s.specTerm("v1", d[0][1], 0)
	}
	s.round(4)
	return s
}

func TestParPrefixOverWaitFresh(t *testing.T) {
	s := overPrefix(t, true)
	if got := s.lines(s.landlog); len(got) != 0 {
		t.Fatalf("진행 중인 앞 무리를 두고 맨 앞을 직접 검증하지 않는다: %v", got)
	}
}

func TestParPrefixOverWaitStale(t *testing.T) {
	s := overPrefix(t, false)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" {
		t.Fatalf("갱신 없으면 지금 길(직접 검증): %q", got)
	}
}

// 감사 낮음 1: 가운데가 시험 실패해 hi−1로 줄이면 hi번째를 범인 후보로 표식 — 앞 hi−1 착지 뒤 혼자 묶인다(다시 묶음 사슬을 타지 않게)
// 감사 낮음 2: 시험 실패 근거 없는 실패 기록(test=0 — 준비 실패·시간 넘김)은 범인 근거가 아니다(prefixScan)
func TestParEnvFailNotCulprit(t *testing.T) {
	s := parFx(t, 3)
	sh := s.four(3)
	n0 := s.failBatch4()
	s.round(4)
	var p2 string
	for _, x := range s.dispatched()[n0:] {
		if s.prefixOf(x[1], sh[:2]) {
			p2 = x[1]
			et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
			fc := s.git(s.seed, "commit-tree", et, "-m", "hc-merger-spec-fail id="+p2+" machine="+x[0]+" at=1 log=- exit=1 test=0")
			s.git(s.seed, "push", "-q", "origin", fc+":"+specFails+p2)
			continue
		}
		s.verifyOn(x) // 앞 1 통과·앞 3 시험 실패
	}
	if p2 == "" {
		t.Fatal("앞 2 맡김")
	}
	s.round(4)
	if jcount(jBatchFail, "범인 b") != 0 || s.has("refs/merge-failed/1000000002-b") {
		t.Fatalf("환경 실패(test=0)인 앞 2를 범인 근거로 쓰지 않는다:\n%s", jdump())
	}
	if b, _ := os.ReadFile(s.clone + "/.git/hc-merger/cap"); strings.TrimSpace(string(b)) != "2" {
		t.Fatalf("앞 3 시험 실패 — 앞 2로 줄임: %q\n%s", b, jdump())
	}
	if b, _ := os.ReadFile(s.clone + "/.git/hc-merger/suspect"); strings.TrimSpace(string(b)) != sh[2] {
		t.Fatalf("낮음 1: 앞 3의 3번째(c)를 범인 후보로 표식: %q", b)
	}
}

// ⑦ 흔들리는 시험(앞 3 한 번 실패, 범인 확인은 통과) → 반송 없이 착지
func TestParCulpritFlakyNoReject(t *testing.T) {
	s := parFx(t, 3)
	s.four(3)
	n0 := s.failBatch4()
	s.round(4)
	for _, x := range s.dispatched()[n0:] {
		s.verifyOn(x)
	}
	s.round(4) // 범인 c 확인 띄움
	chk := s.dispatched()[len(s.dispatched())-1]
	s.round(4) // 앞 2 착지
	t.Setenv("PASS_ALL", "1")
	s.verifyOn(chk)
	s.round(4)
	if s.has("refs/merge-failed/1000000003-c") || !s.onMain("bad.txt") {
		t.Fatalf("⑦ 확인이 통과하면 반송 없이 착지:\n%s", jdump())
	}
}
