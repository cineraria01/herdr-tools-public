//go:build !windows

package main

// 맨 앞 후보도 검증 기계에(merger-verify-dispatch): 검증 기계 목록이 있고 닿는 기계가 있으면 맨 앞 무리도 뒤쪽 추측과 같은 꼴로 띄우고,
// 병합기 기계는 모으기·차례·rebase·기록 확인·넣기만 한다. 맡길 기계가 없거나 기다림 상한을 넘기면 손잡이 HC_LANE_FRONT_LOCAL로 정한 길
// (기본 1 = 지금처럼 직접 검증, 0 = 직접 검증하지 않고 기다리며 주인 지휘에 사건 한 번). 틀은 specrun_test.go(specFx)·
// specfrontwait_test.go(age)·culprit_test.go(culpritFx — 일꾼 실패 기록에 시험 실패 근거 test=1).

import (
	"os"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

// oldFront: 옛 길(맨 앞은 병합기가 직접 검증 — 손잡이 front_dispatch 0)에 묶인 시험. 그 시험들은 뒤쪽 추측·범인 가리기·일지를 보며
// 「맨 앞은 병합기가 직접」을 전제로 띄운 차례·착지 줄을 센다 — 손잡이 0도 지원하는 길(실행 중 되돌리기)이라 기대값을 그대로 두고,
// 기본 길은 이 파일이 본다
func oldFront(t *testing.T) { t.Setenv("HC_LANE_FRONT_DISPATCH", "0") }

// evs: 가짜 report가 적은 사건 줄(글에 빈칸이 있어 lines로 세면 안 된다)
func (s *sfx) evs() []string {
	b, _ := os.ReadFile(s.events)
	var o []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			o = append(o, l)
		}
	}
	return o
}

// ① 맨 앞 혼자 제출·기록 없음 → 병합기 기계 검증 0번, 검증 기계에 띄움 → 통과 뒤 기록으로 착지
func TestFrontDispatchedToVerifier(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	m0 := s.mainSHA()
	s.round(1)
	d := s.dispatched()
	if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("① 맨 앞도 검증 기계에 — 병합기 검증 0번: 착지 %v 띄움 %v", s.lines(s.landlog), d)
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" || !s.onMain("a.txt") {
		t.Fatalf("① 통과 기록으로 착지(검증 건너뜀): %q", got)
	}
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "추측 검증 기다림 1바퀴, 검증 맡김(v1)") {
		t.Fatalf("① 일지 「검증 맡김(v1)」:\n%s", jdump())
	}
}

// ② 검증 기계 목록 없음 → 지금처럼 병합기가 직접
func TestFrontNoVerifiersVerifiesLocally(t *testing.T) {
	s := specFx(t)
	s.git(s.seed, "push", "-q", "origin", ":"+verified.VerifiersRef)
	s.sub(1000000001, "a", false)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 0 {
		t.Fatalf("② 기계 없으면 직접: %q 띄움 %v", got, s.dispatched())
	}
}

// ③ 「직접 검증 안 함」 손잡이 + 맡길 기계 없음(못 띄움) → 착지하지 않고 기다림, 주인 지휘에 사건은 한 번만
func TestFrontNoLocalWaitsAndNotifiesOnce(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	t.Setenv("HC_RC", "1") // hc run --on 실패 = 닿는 기계 없음
	s.sub(1000000001, "a", false)
	m0 := s.mainSHA()
	s.round(1)
	e1 := s.evs()
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 {
		t.Fatalf("③ 직접 검증하지 않는다: %v", s.lines(s.landlog))
	}
	if e := s.evs(); len(e1) != 1 || len(e) != 1 || !strings.Contains(e[0], "막힘") {
		t.Fatalf("③ 사건 한 번: 첫 바퀴 %v 둘째 바퀴 %v", e1, e)
	}
}

// ④ 맡긴 맨 앞이 기다림 상한을 넘김 → 기본(직접 검증) 길로 / 손잡이 0이면 기다림·사건 한 번
func TestFrontOverCapDefaultVerifiesLocally(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	s.age(d[0][1], 700)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || !s.onMain("a.txt") {
		t.Fatalf("④ 상한을 넘기면 직접 검증: %q", got)
	}
}

func TestFrontOverCapNoLocalWaits(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	s.age(d[0][1], 700)
	s.round(1)
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || len(s.evs()) != 1 {
		t.Fatalf("④ 손잡이 0: 기다림·사건 한 번 — 착지 %v 사건 %v", s.lines(s.landlog), s.evs())
	}
}

// ⑤ 검증 기계에서 시험 실패 → 첫 실패만으로는 반송하지 않고 다른 기계에서 한 번 더, 그래도 실패면 반송
func TestFrontTestFailRecheckedThenRejected(t *testing.T) {
	s := culpritFx(t, true)
	s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) { // 일꾼은 실패 기록을 쓰면 0
		t.Fatalf("v1 시험 실패 기록: %v", d)
	}
	s.round(1)
	d = s.dispatched()
	if s.has("refs/merge-failed/1000000001-a") || len(d) != 2 || d[1][0] == d[0][0] {
		t.Fatalf("⑤ 첫 실패만으로 반송하지 않고 다른 기계에서 다시: 반송 %v 띄움 %v", s.has("refs/merge-failed/1000000001-a"), d)
	}
	s.verifyOn(d[1])
	s.round(1)
	if !s.has("refs/merge-failed/1000000001-a") || s.onMain("bad.txt") {
		t.Fatal("⑤ 두 번째도 시험 실패면 반송")
	}
}

// ⑥ 환경 실패(시험 실패 근거 없음) → 모름: 반송하지 않는다(기본 손잡이 — 직접 검증해 통과하면 착지)
func TestFrontEnvFailIsUnknown(t *testing.T) {
	s := culpritOut(t, "준비 실패")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	fc := s.git(s.seed, "commit-tree", et, "-m", "hc-merger-spec-fail id="+d[0][1]+" machine=v1 at=1 log=- exit=1 test=0")
	s.git(s.seed, "push", "-q", "origin", fc+":"+specFails+d[0][1])
	s.round(1)
	if s.has("refs/merge-failed/1000000001-a") {
		t.Fatal("⑥ 환경 실패로 반송하지 않는다")
	}
}

// drive: 바퀴를 돌리며 새로 띄운 일꾼을 그 기계에서 부른다(n바퀴). 기본 길의 끝 상태만 본다
func (s *sfx) drive(batch, n int) {
	s.t.Helper()
	done := 0
	for i := 0; i < n; i++ {
		s.round(batch)
		d := s.dispatched()
		for _, x := range d[done:] {
			s.verifyOn(x)
		}
		done = len(d)
	}
}

// 기본 길의 사슬 A·B·C: 병합기 기계는 한 번도 검증하지 않고(착지 줄이 모두 fast) 큐 차례대로 넣는다. 끝나면 추측 참조를 모두 치운다
func TestFrontChainNoLocalVerify(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.sub(1000000003, "c", false)
	s.round(1)
	if d := s.dispatched(); len(d) != 2 || d[0][0] != "v1" || d[1][0] != "v2" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("맨 앞 A가 v1에서 도는 동안 뒤 B 추측도 v2에(착지 없음): 띄움 %v 착지 %v", d, s.lines(s.landlog))
	}
	for _, x := range s.dispatched() {
		s.verifyOn(x)
	}
	s.drive(1, 8)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a fast|b fast|c" || !s.onMain("c.txt") {
		t.Fatalf("A·B·C 모두 검증 기계의 기록으로 차례대로: %q", got)
	}
	if r := append(append(append(s.refs(specRefs), s.refs(specFails)...), s.refs(specStarts)...), s.refs(verified.Prefix)...); len(r) != 0 {
		t.Fatalf("끝난 추측 참조가 남음: %v", r)
	}
}

// ⑧ 묶음 실패 뒤의 길(범인이 맨 앞 [a(범인),b,c]): 묶음 시험 실패 → 마지막을 빼고 다시 → 맨 앞 혼자 추측의 시험 실패로 a 반송(지금의
// 범인 반송 꼴 그대로), b·c 착지 — 병합기 기계 검증은 0번(옛 길은 2번)
func TestFrontCulpritPathSame(t *testing.T) {
	s := culpritFx(t, true)
	s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.submit(1000000002, "b", "보통")
	s.submit(1000000003, "c", "보통")
	s.drive(3, 10)
	if !s.has("refs/merge-failed/1000000001-a") || s.onMain("bad.txt") || !s.onMain("b.txt") || !s.onMain("c.txt") {
		t.Fatalf("⑧ a 반송·b·c 착지:\n%s", jdump())
	}
	for _, l := range s.lines(s.landlog) {
		if strings.HasPrefix(l, "verify|") {
			t.Fatalf("⑧ 병합기 기계 검증 0번: %v", s.lines(s.landlog))
		}
	}
	if len(jrows(t, "묶음 실패")) == 0 || len(jrows(t, "맨 앞 혼자 추측")) != 1 {
		t.Fatalf("⑧ 묶음 실패 줄·맨 앞 혼자 추측 한 줄:\n%s", jdump())
	}
	back := jrows(t, "돌려보냄")
	if len(back) != 1 || back[0][2] != "a" || !strings.Contains(back[0][11], "착지 실패(종료 1") || !strings.Contains(back[0][11], "--- FAIL: TestBad") {
		t.Fatalf("⑧ 반송 줄 = 범인 반송 꼴:\n%s", jdump())
	}
}

// 범인이 마지막 [a,b,c(범인)]: 묶음 시험 실패로 줄여 [a,b] 착지, 혼자 남은 c는 두 검증 기계에서 시험 실패해야 반송(까닭에 두 기계)
func TestFrontCulpritLastRechecked(t *testing.T) {
	s := culpritFx(t, true)
	s.submit(1000000001, "a", "보통")
	s.submit(1000000002, "b", "보통")
	s.sub2(1000000003, "c", "", "", "bad.txt", "c\n", false)
	s.drive(3, 10)
	if !s.onMain("b.txt") || !s.has("refs/merge-failed/1000000003-c") || s.onMain("bad.txt") {
		t.Fatalf("a·b 착지·c 반송:\n%s", jdump())
	}
	back := jrows(t, "돌려보냄")
	if len(back) != 1 || !strings.Contains(back[0][11], "v1·v2") && !strings.Contains(back[0][11], "v2·v1") {
		t.Fatalf("c 반송 까닭에 두 검증 기계:\n%s", jdump())
	}
	for _, l := range s.lines(s.landlog) {
		if strings.HasPrefix(l, "verify|") {
			t.Fatalf("병합기 기계 검증 0번: %v", s.lines(s.landlog))
		}
	}
}

// 검증 기계가 하나뿐(감사 atozd0z-zklwye 중간): cap 바퀴에 그 기계를 맨 앞 k−1이 먼저 쓴다 — 혼자 추측이 가져가 병합기가 직접 검증하지
// 않는다. [a,b,c(범인)] → a·b 착지, c는 같은 기계에서 두 번 시험 실패 뒤 반송, 병합기 기계 검증 0번
func TestFrontOneVerifierCapFrontFirst(t *testing.T) {
	s := culpritFx(t, true)
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := s.git(s.seed, "commit-tree", et, "-m", verified.FormatVerifiers([]verified.Verifier{{Machine: "v1", Path: s.dir + "/v1"}}))
	s.git(s.seed, "push", "-q", "-f", "origin", vc+":"+verified.VerifiersRef)
	s.submit(1000000001, "a", "보통")
	s.submit(1000000002, "b", "보통")
	s.sub2(1000000003, "c", "", "", "bad.txt", "c\n", false)
	s.drive(3, 12)
	if !s.onMain("b.txt") || !s.has("refs/merge-failed/1000000003-c") || s.onMain("bad.txt") {
		t.Fatalf("a·b 착지·c 반송:\n%s", jdump())
	}
	for _, l := range s.lines(s.landlog) {
		if strings.HasPrefix(l, "verify|") {
			t.Fatalf("병합기 기계 검증 0번: %v\n%s", s.lines(s.landlog), jdump())
		}
	}
}

// 기다림 일지: 옛 세 칸 기록도 읽고(기계 없음 — 덧붙이지 않음), 손잡이 0으로 맡기지 못해 기다린 줄은 「맡길 기계를 기다림」
func TestFrontSpecWaitRecord(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	m := s.merger()
	os.MkdirAll(m.mj, 0o777)
	q := "refs/merge-queue/1000000001-a"
	os.WriteFile(m.mj+"/specwait", []byte("100 200 3\n"+q+" "+strings.Repeat("a", 40)+" a -\n"), 0o666)
	f, l, n, b, mach, ok := m.specWaitRead()
	if !ok || f != 100 || l != 200 || n != 3 || len(b) != 1 || mach != "" {
		t.Fatalf("옛 세 칸 기록: %v %d %d %d %v %q", ok, f, l, n, b, mach)
	}
	m.specWaitLine(f, l, n, b, "-")
	if w := jrows(t, "추측 기다림"); len(w) != 1 || !strings.Contains(w[0][11], "추측 검증 기다림 3바퀴, 맡길 기계를 기다림(직접 검증 안 함)") {
		t.Fatalf("손잡이 0 기다림 줄:\n%s", jdump())
	}
}

// 손잡이 파일(<일지 폴더>/front_dispatch 0)은 환경 변수보다 앞서 바퀴마다 먹는다 — 0이면 옛 길(맨 앞 직접 검증)
func TestFrontDispatchKnobFile(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	os.MkdirAll(s.clone+"/.git/hc-merger", 0o777)
	os.WriteFile(s.clone+"/.git/hc-merger/front_dispatch", []byte("0\n"), 0o666)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 0 {
		t.Fatalf("손잡이 파일 0이면 옛 길: %q 띄움 %v", got, s.dispatched())
	}
}

// ⑦ 미리 검증 기록이 이미 있음 → 지금처럼 바로 착지(새로 띄우지 않음)
func TestFrontPreRecordLandsFast(t *testing.T) {
	s, d, _ := frontFx(t, 0)
	if rc := s.verifyOn(d); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" || len(s.dispatched()) != 1 {
		t.Fatalf("⑦ 기록으로 바로 착지: %q 띄움 %v", got, s.dispatched())
	}
}
