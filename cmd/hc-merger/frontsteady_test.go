//go:build !windows

package main

// ht-merger-front-steady(2026-11-26 48시간 병합 줄 조사의 큰 차례 셋): A 도는 맨 앞은 얼린다(새 제출·끼어듦은 다음 무리로) ·
// B 착지 스크립트 저장소의 추측 실패는 반송 근거가 아니라 차례 근거(범인은 옆으로 빼 혼자 착지 검증) · C 맨 앞에 검증 기계 먼저
// (미리 줄 추측을 끝내고 내줌, 길에서 벗어난 추측은 그 바퀴에 끝냄). 틀은 specrun_test.go(specFx)·culpritpar_test.go(parFx)·
// specpre_test.go(preFx)·specoverrun_test.go(specTerm).

import (
	"os"
	"strings"
	"testing"
	"time"
)

// kills: 가짜 hc가 받은 kill 주소들
func (s *sfx) kills() []string {
	b, _ := os.ReadFile(s.hclog)
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if a, ok := strings.CutPrefix(l, "kill "); ok {
			o = append(o, a)
		}
	}
	return o
}

func (s *sfx) shaOf(lane string) string {
	return s.git(s.seed, "--git-dir="+s.origin, "rev-parse", "refs/heads/lane/"+lane)
}

// withdraw: 큐에서 거둠(표식·기록을 지움)
func (s *sfx) withdraw(sec int, lane string) {
	s.t.Helper()
	n := strings.TrimPrefix(s.git(s.seed, "--git-dir="+s.origin, "for-each-ref", "--format=%(refname)", "refs/merge-queue/*-"+lane), "refs/merge-queue/")
	if n == "" {
		s.t.Fatalf("큐에 %s 없음", lane)
	}
	s.git(s.seed, "push", "-q", "origin", ":refs/merge-queue/"+n, ":refs/merge-queue-meta/"+n)
}

// ① 맨 앞 추측이 도는 중 새 제출(뒤) → 그 추측은 그대로, 새 제출은 다음 무리(그 위에 추측) — 다시 짓지 않는다
func TestFreezeNewBehind(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.round(3)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 a 띄움: %v", d)
	}
	s.sub(1000000002, "b", false)
	s.round(3)
	d = s.dispatched()
	if len(d) != 2 || !strings.Contains(s.specGroupOf(d[1][1]), "group="+s.shaOf("b")+" ") || !s.has(specRefs+d[0][1]) {
		t.Fatalf("① a 추측 유지·b는 다음 무리: %v\n%s", d, jdump())
	}
	if f := jrows(t, jFreeze); len(f) != 1 || !strings.Contains(f[0][11], "얼림 — 새 제출 b 다음 무리로") {
		t.Fatalf("① 일지 「얼림 — 새 제출 b 다음 무리로」:\n%s", jdump())
	}
	s.round(3) // 같은 얼림은 일지에 한 번만
	if len(jrows(t, jFreeze)) != 1 {
		t.Fatalf("① 얼림 줄은 새 제출마다 한 번:\n%s", jdump())
	}
	for _, x := range d {
		s.verifyOn(x)
	}
	s.round(3)
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a fast|b" {
		t.Fatalf("① a·b 모두 기록으로: %q\n%s", got, jdump())
	}
	t.Log(jdump()) // -v로 일지 줄을 본다(보고용)
}

// ② 앞에 끼어드는 더 높은 우선·다시 넣음(원래 차례) → 도는 a 추측 유지, a가 끝난 뒤 줄 차례대로
func TestFreezeCutInAhead(t *testing.T) {
	s := specFx(t)
	s.sub(1000000005, "a", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 1 {
		t.Fatalf("맨 앞 a 띄움: %v", d)
	}
	s.submit(1000000006, "u", "긴급")
	s.sub(1000000001, "r", false) // 반송 뒤 다시 넣음 — 원래(더 이른) 차례
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 || !strings.Contains(s.specGroupOf(d[1][1]), "group="+s.shaOf("u")+" ") || !s.has(specRefs+d[0][1]) {
		t.Fatalf("② a 유지·u가 다음 무리: %v\n%s", d, jdump())
	}
	if f := jrows(t, jFreeze); len(f) != 1 || !strings.Contains(f[0][11], "새 제출 u") {
		t.Fatalf("② 얼림 줄:\n%s", jdump())
	}
	s.drive(1, 8)
	if got := strings.Join(s.lines(s.landlog), " "); !strings.HasPrefix(got, "fast|a fast|u") || !s.onMain("r.txt") {
		t.Fatalf("② a 먼저, 그다음 줄 차례(u → r): %q\n%s", got, jdump())
	}
}

// ③ 구성원이 빠짐(거둠) → 지금처럼 다시 짓는다 — 길에서 벗어난 옛 추측은 그 바퀴에 끝낸다
func TestFreezeMemberGoneRebuilds(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(3)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 a+b 띄움: %v", d)
	}
	s.specTerm("v1", d[0][1], 0)
	s.withdraw(1000000002, "b")
	s.round(3)
	d = s.dispatched()
	if len(d) != 2 || !strings.Contains(s.specGroupOf(d[1][1]), "group="+s.shaOf("a")+" ") {
		t.Fatalf("③ a 혼자로 다시 지음: %v\n%s", d, jdump())
	}
	if k := s.kills(); len(k) != 1 || k[0] != "v1/t9" || s.has(specRefs+d[0][1]) {
		t.Fatalf("③ 옛 a+b 추측은 정확한 주소로 끝내고 참조를 치움: kill %v\n%s", k, jdump())
	}
}

// scriptPar: parFx(검증 기계 n대) + 기본 브랜치에 착지 스크립트. 검증 스크립트는 bad.txt·flaky.txt면 시험 실패(가짜 착지는 bad.txt만 본다 —
// flaky.txt는 추측만 실패하고 착지 스크립트는 통과하는 꼴)
func scriptPar(t *testing.T, n int) *sfx {
	s := parFx(t, n)
	os.WriteFile(s.seed+"/.claude/herdr-land.sh", []byte("#!/bin/bash\nexit 0\n"), 0o777)
	os.WriteFile(s.seed+"/.claude/herdr-verify.sh", []byte("#!/bin/bash\nif [ -e bad.txt ] || [ -e flaky.txt ]; then echo '--- FAIL: TestBad (0.00s)'; exit 1; fi\n"), 0o777)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "script land")
	s.git(s.seed, "push", "-q", "origin", "main")
	return s
}

func (s *sfx) capOf() string {
	b, _ := os.ReadFile(s.clone + "/.git/hc-merger/cap")
	return strings.TrimSpace(string(b))
}

// ④ 착지 스크립트 저장소·묶음 4·범인 2번째: 추측 실패 → 직접 검증 없이 앞 무리 추측 → 앞 1 착지 → 2번째 옆으로·혼자 착지 검증 실패 → 반송,
// 3·4번째는 그동안 b 없이 추측 → 기록으로 착지
func TestSidelineScriptCulprit(t *testing.T) {
	s := scriptPar(t, 3)
	sh := s.four(2)
	s.round(4)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("묶음 4 추측 실패: %v", d)
	}
	s.round(4)
	if s.capOf() != "3" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("④ 추측 실패 → 직접 검증 없이 줄임(cap 3): cap %q 착지 %v\n%s", s.capOf(), s.lines(s.landlog), jdump())
	}
	n0 := len(s.dispatched())
	s.round(4)
	pd := s.dispatched()[n0:]
	if len(pd) != 3 {
		t.Fatalf("④ 앞 무리 3·2·1을 함께: %v\n%s", pd, jdump())
	}
	for _, x := range pd {
		s.verifyOn(x)
	}
	n1 := len(s.dispatched())
	s.round(4) // 앞 1 통과·앞 2 실패 → 범인 b, 확인 추측 없이 옆으로
	if bf := jrows(t, jBatchFail); len(bf) == 0 || !strings.Contains(bf[len(bf)-1][11], "앞 1 착지 · 범인 b") {
		t.Fatalf("④ 범인 b:\n%s", jdump())
	}
	if len(s.dispatched()) != n1 {
		t.Fatalf("④ 착지 스크립트 저장소는 범인 확인 추측을 띄우지 않는다: %v", s.dispatched()[n1:])
	}
	s.round(4)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("④ 앞 1(a) 기록으로 착지: %q\n%s", got, jdump())
	}
	n2 := len(s.dispatched())
	base := s.mainTree()
	s.round(4) // b 혼자 착지 검증(실패 → 반송), 그동안 c+d는 b 없이 추측
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a verify|b" || !s.has("refs/merge-failed/1000000002-b") {
		t.Fatalf("④ b 혼자 착지 검증 실패 → 반송: %q\n%s", got, jdump())
	}
	if sd := jrows(t, jSide); len(sd) != 1 || !strings.Contains(sd[0][11], "옆으로 뺌 b(추측 실패 --- FAIL: TestBad") || !strings.Contains(sd[0][11], "혼자 착지 검증") {
		t.Fatalf("④ 일지 「옆으로 뺌 b(추측 실패 …) — 혼자 착지 검증」:\n%s", jdump())
	}
	nd := s.dispatched()[n2:]
	if len(nd) != 1 || !s.prefixOf(nd[0][1], sh[2:]) || !strings.Contains(s.specGroupOf(nd[0][1]), "parent="+base+" ") {
		t.Fatalf("④ c+d를 b 없이(기준 %s 위) 추측: %v %s", first8(base), nd, s.specGroupOf(nd[0][1]))
	}
	s.verifyOn(nd[0])
	s.round(4)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a verify|b fast|c" || !s.onMain("d.txt") {
		t.Fatalf("④ c+d 기록으로 착지: %q\n%s", got, jdump())
	}
	if w := whySummary(strings.Split(jdump(), "\n"), 20); !strings.Contains(w, "옆으로 뺌 1") {
		t.Fatalf("④ --why에 옆으로 뺌: %s", w)
	}
	t.Log(jdump(), "\n", whySummary(strings.Split(jdump(), "\n"), 20)) // -v로 일지 줄을 본다(보고용)
}

// ⑤ 혼자 추측이 실패한 맨 앞(옆으로 뺌) → 혼자 착지 검증 통과(추측이 거짓 실패) → 착지. 묶음 a+b 실패도 직접 검증 없이 줄임
func TestSidelinePassLands(t *testing.T) {
	s := scriptPar(t, 2)
	s.submit(1000000001, "a", "보통")
	s.sub2(1000000002, "b", "", "", "flaky.txt", "b\n", false)
	s.drive(2, 8)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a verify|b" || !s.onMain("flaky.txt") || s.has("refs/merge-failed/1000000002-b") {
		t.Fatalf("⑤ a 기록으로, b는 옆으로 빼 혼자 착지 검증 → 통과해 착지: %q\n%s", got, jdump())
	}
	if len(jrows(t, jSide)) != 1 {
		t.Fatalf("⑤ 옆으로 뺌 한 줄:\n%s", jdump())
	}
}

// ⑥ 맨 앞이 맡길 기계가 없고 미리 줄 추측이 기계를 쥠 → 미리 줄 추측을 정확한 주소로 끝내고 그 기계에 맨 앞을 맡긴다
func TestFrontPreemptsPre(t *testing.T) {
	s := preFx(t)
	s.idle("v2", verifyBusy)
	s.pre("p", time.Now().Unix(), false)
	s.round(1) // 큐 빔 — 미리 줄 p를 v1에
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("미리 줄 p를 v1에: %v", d)
	}
	s.specTerm("v1", d[0][1], 0)
	s.sub(1000000001, "a", false)
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][0] != "v1" || !strings.Contains(s.specGroupOf(d[1][1]), "group="+s.shaOf("a")+" ") || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("⑥ 맨 앞 a를 v1에(직접 검증 없음): %v 착지 %v\n%s", d, s.lines(s.landlog), jdump())
	}
	if k := s.kills(); len(k) != 1 || k[0] != "v1/t9" || s.has(specRefs+d[0][1]) {
		t.Fatalf("⑥ 미리 줄 추측을 끝내고 참조를 치움: kill %v", k)
	}
	if p := jrows(t, jPreKill); len(p) != 1 || !strings.Contains(p[0][11], "미리 줄 추측 끝냄(v1) — 맨 앞에 내줌") {
		t.Fatalf("⑥ 일지 「미리 줄 추측 끝냄(v1) — 맨 앞에 내줌」:\n%s", jdump())
	}
}

// ⑦ 길에서 벗어난 추측(기준이 움직임) → 그 바퀴에 끝내고 그 기계를 맨 앞에 쓴다
func TestOffPathKilledSameRound(t *testing.T) {
	s := specFx(t)
	s.idle("v2", verifyBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("맨 앞 a를 v1에: %v", d)
	}
	s.specTerm("v1", d[0][1], 0)
	s.moveMain("x")
	s.round(1)
	d = s.dispatched()
	if k := s.kills(); len(k) != 1 || k[0] != "v1/t9" || s.has(specRefs+d[0][1]) {
		t.Fatalf("⑦ 옛 기준 추측을 끝냄: kill %v", k)
	}
	if len(d) != 2 || d[1][0] != "v1" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("⑦ 새 기준의 a를 v1에(직접 검증 없음): %v 착지 %v", d, s.lines(s.landlog))
	}
}

// 감사 atp3xrb 중간 1: 끝낸 일꾼(hc kill — defer가 안 돎)이 남긴 작업 트리는 그 기계의 다음 일꾼이 시작할 때 치운다(원격 추측 참조가 없는
// 아이디만 — 도는 다른 추측의 폴더는 그대로)
func TestKilledWorkerTreeSwept(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 {
		t.Fatalf("a·b 띄움: %v", d)
	}
	v1 := s.dir + "/v1"
	stale, live := v1+".spec-wt/deadbeef", v1+".spec-wt/"+d[1][1][:8]
	s.git(v1, "worktree", "add", "-q", "--detach", stale, "HEAD")
	os.MkdirAll(live, 0o777)
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	if _, err := os.Stat(stale); err == nil || strings.Contains(s.git(v1, "worktree", "list"), "deadbeef") {
		t.Fatalf("끝낸 일꾼의 작업 트리를 치움:\n%s", s.git(v1, "worktree", "list"))
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("도는 추측(원격 참조 있음)의 폴더는 그대로")
	}
}

// 감사 atp3xrb 낮음 3: 손잡이 front_local 0으로 맡길 기계가 없어 쉬는 바퀴(도는 추측 없음)는 얼리지 않는다
func TestFreezeNotOnHold(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.idle("v1", verifyBusy)
	s.idle("v2", verifyBusy)
	s.sub(1000000001, "a", false)
	s.round(1)
	if _, err := os.Stat(s.clone + "/.git/hc-merger/front"); err == nil || len(s.dispatched()) != 0 {
		t.Fatalf("도는 것 없는 기다림은 얼리지 않음: 띄움 %v", s.dispatched())
	}
}

// ⑧ 내장 착지 저장소 → 지금 그대로: 맨 앞 혼자 시험 실패는 다른 기계에서 한 번 더 확인(옆으로 빼지 않음)
func TestSidelineBuiltinUnchanged(t *testing.T) {
	s := parFx(t, 2)
	s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("맨 앞 a: %v", d)
	}
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][1] != d[0][1] || d[1][0] == d[0][0] || len(jrows(t, jSide)) != 0 || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("⑧ 다른 기계에서 한 번 더 확인: %v\n%s", d, jdump())
	}
	s.verifyOn(d[1])
	s.round(1)
	if !s.has("refs/merge-failed/1000000001-a") || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("⑧ 두 기계 시험 실패로 반송:\n%s", jdump())
	}
}

// ⑨ 손잡이 끔 → 지금 길: 얼림 0이면 [a,b]를 새로 지음 · 옆으로 빼기 0이면 묶음 추측 실패를 한가한 다른 기계에 한 번 더
func TestFreezeKnobOff(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_MERGER_FRONT_FREEZE", "0")
	s.sub(1000000001, "a", false)
	s.round(3)
	s.sub(1000000002, "b", false)
	s.round(3)
	d := s.dispatched()
	if len(d) != 2 || !strings.Contains(s.specGroupOf(d[1][1]), "group="+s.shaOf("a")+"+"+s.shaOf("b")+" ") || len(jrows(t, jFreeze)) != 0 {
		t.Fatalf("⑨ 얼림 끔 — a+b를 새로: %v\n%s", d, jdump())
	}
}

func TestSidelineKnobOff(t *testing.T) {
	s := scriptPar(t, 2)
	t.Setenv("HC_LANE_MERGER_ASIDE", "0")
	s.submit(1000000001, "a", "보통")
	s.sub2(1000000002, "b", "", "", "flaky.txt", "b\n", false)
	s.round(2)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("묶음 a+b: %v", d)
	}
	s.round(2)
	d = s.dispatched()
	if len(d) != 2 || d[1][1] != d[0][1] || s.capOf() != "" || len(jrows(t, jSide)) != 0 {
		t.Fatalf("⑨ 옆으로 빼기 끔 — 같은 추측을 다른 기계에 한 번 더(줄이지 않음): %v cap %q\n%s", d, s.capOf(), jdump())
	}
}
