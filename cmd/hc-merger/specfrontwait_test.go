//go:build !windows

package main

// 맨 앞 기다림 상한(preverify-wait-cap): 맨 앞 무리의 추측이 「도는 중」이어도 띄운 때(at)부터 HC_LANE_SPEC_FRONT_WAIT(기본 600초)를
// 넘기면 맨 앞 판정은 「모름」 — 병합기 기계가 직접 검증한다. 시험은 실제로 기다리지 않고 추측 참조의 at을 옛 값으로 고쳐 쓴다
// (일꾼이 떴다는 표식도 올려 HC_LANE_SPEC_START의 「안 뜬 일꾼」 길과 가른다). 틀은 specpre_test.go(preFx).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/verified"
)

// age: 원격 추측 참조의 at을 sec초 앞으로 고쳐 쓰고(아이디·트리·부모 그대로) 일꾼이 떴다는 표식을 올린다
func (s *sfx) age(id string, sec int64) {
	s.t.Helper()
	ref := specRefs + id
	s.git(s.seed, "fetch", "-q", "origin", "+"+ref+":"+ref)
	subj := s.git(s.seed, "log", "-1", "--format=%s", ref)
	var out []string
	for _, kv := range strings.Fields(subj) {
		if v, ok := strings.CutPrefix(kv, "at="); ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			kv = fmt.Sprintf("at=%d", n-sec)
		}
		out = append(out, kv)
	}
	c := s.git(s.seed, "commit-tree", ref+"^{tree}", "-p", s.git(s.seed, "rev-parse", ref+"^"), "-m", strings.Join(out, " "))
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	sc := s.git(s.seed, "commit-tree", et, "-m", "hc-merger-spec-run machine=v1")
	s.git(s.seed, "push", "-q", "-f", "origin", c+":"+ref, sc+":"+specStarts+id)
	s.git(s.seed, "update-ref", "-d", ref)
}

// frontFx: 표식 p → 미리 줄 추측(v1, 일꾼은 아직) → 같은 sha 제출 → 그 추측을 sec초 묵힘. 추측 아이디·p SHA·바퀴 전 main
func frontFx(t *testing.T, sec int64) (*sfx, [2]string, string) {
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("띄움: %v", d)
	}
	s.enqueue(1000000001, "p", sha)
	s.age(d[0][1], sec)
	return s, d[0], s.mainSHA()
}

// ① 맨 앞 추측이 도는 중 · 상한 안 → 기다림(착지·새 추측 없음)
func TestFrontWaitUnderCapWaits(t *testing.T) {
	s, _, m0 := frontFx(t, 100)
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(s.dispatched()) != 1 {
		t.Fatalf("① 상한 안이면 기다린다: 착지 %v 띄움 %v", s.lines(s.landlog), s.dispatched())
	}
}

// 추측을 기다린 바퀴(묶음 2 — 후보를 쌓느라 착지 로그를 연다)는 빈 착지 로그를 남기지 않는다(2026-11-25 한 저장소의 병합기: 맨 앞
// 묶음 2가 추측 기다림 74바퀴 동안 빈 <레인>-<시각>-<pid>.log 74개 — 재시도가 아니라 기다림 바퀴)
func TestSpecWaitNoEmptyLog(t *testing.T) {
	s := specFx(t)
	for i, l := range []string{"a", "b", "c", "d"} {
		s.sub(1000000001+i, l, false)
	}
	for i := 0; i < 3; i++ {
		s.round(2)
		time.Sleep(1100 * time.Millisecond) // 로그 이름이 초 단위 — 바퀴마다 다른 이름이 되게
	}
	if s.onMain("a.txt") || len(s.dispatched()) == 0 {
		t.Fatalf("추측을 기다려야(착지 없음): 띄움 %v", s.dispatched())
	}
	lg, _ := filepath.Glob(s.dir + "/a-*.log")
	for _, f := range lg {
		if st, err := os.Stat(f); err == nil && st.Size() == 0 {
			t.Fatalf("기다린 바퀴가 빈 착지 로그를 남겼다: %s", f)
		}
	}
}

// ② 도는 중 · 상한 넘김 · 한가한 다른 기계 없음 → 「모름」: 병합기가 직접 검증(통과로 치지 않음 — HERDR_SPEC_TREE 빈 값)해 착지,
// 추측을 새로 띄우지 않는다(한가한 기계가 있으면 먼저 그 기계에 — specreuse_test.go)
func overCap(t *testing.T) (*sfx, [2]string) {
	s, d, _ := frontFx(t, 700)
	s.idle("v2", verifyBusy)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || !s.onMain("p.txt") || s.treeLog("p") != "" {
		t.Fatalf("② 상한을 넘기면 병합기가 직접 검증: %q 트리 %q", got, s.treeLog("p"))
	}
	if len(s.dispatched()) != 1 {
		t.Fatalf("② 추측을 새로 띄우지 않는다: %v", s.dispatched())
	}
	return s, d
}

func TestFrontWaitOverCapVerifies(t *testing.T) { overCap(t) }

// 뒤늦은 결과 뒤 바퀴들: 착지·반송·새 사건 없음, 추측·결과·표식 참조는 모두 치움
func (s *sfx) afterLate(what string) {
	s.t.Helper()
	m0, e0 := s.mainSHA(), strings.Join(s.lines(s.events), " ")
	s.round(1)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || s.mainSHA() != m0 || !s.onMain("p.txt") {
		s.t.Fatalf("%s 다시 착지하지 않는다: %q", what, got)
	}
	if e := strings.Join(s.lines(s.events), " "); len(s.refs("refs/merge-failed/")) != 0 || e != e0 {
		s.t.Fatalf("%s 반송·새 사건 없음: %v %q", what, s.refs("refs/merge-failed/"), strings.TrimPrefix(e, e0))
	}
	if r := append(append(append(append(s.refs(specRefs), s.refs(specFails)...), s.refs(specStarts)...), s.refs(verified.Prefix)...), s.refs(verified.PreRefs)...); len(r) != 0 {
		s.t.Fatalf("%s 짝 없는 추측·결과를 치운다: %v", what, r)
	}
}

// ③ ② 뒤에 일꾼의 통과 기록이 뒤늦게 생김 → 이중 착지·오류 없음, 치움
func TestFrontWaitLatePass(t *testing.T) {
	s, d := overCap(t)
	if rc := s.verifyOn(d); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	if len(s.refs(verified.Prefix)) != 1 {
		t.Fatalf("뒤늦은 통과 기록: %v", s.refs(verified.Prefix))
	}
	s.afterLate("③")
}

// ④ ② 뒤에 실패 참조가 뒤늦게 생김 → 이미 착지한 레인을 반송하지 않는다
func TestFrontWaitLateFail(t *testing.T) {
	s, d := overCap(t)
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	fc := s.git(s.seed, "commit-tree", et, "-m", fmt.Sprintf("hc-merger-spec-fail id=%s machine=v1 at=%d log=- exit=1 test=1", d[1], time.Now().Unix()))
	s.git(s.seed, "push", "-q", "origin", fc+":"+specFails+d[1])
	s.afterLate("④")
}

// ⑤ 뒤쪽 무리의 도는 중 추측은 상한과 무관하게 지금 그대로(통과로 보고 더 깊이 — 다시 짓지 않는다)
func TestFrontWaitBackUnaffected(t *testing.T) {
	oldFront(t)
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.sub(1000000003, "c", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 {
		t.Fatalf("B·C 띄움: %v", d)
	}
	s.age(d[1][1], 700) // 뒤쪽 C(main+A+B+C)만 묵힘, 맨 앞 B는 새것
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || len(s.dispatched()) != 2 || !s.has(specRefs+d[1][1]) {
		t.Fatalf("⑤ 맨 앞 B는 기다리고 뒤 C 추측은 그대로: 착지 %q 띄움 %v", got, s.dispatched())
	}
	for _, x := range d {
		if rc := s.verifyOn(x); rc != 0 {
			t.Fatalf("일꾼 %v 종료 %d", x, rc)
		}
	}
	s.round(1)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a fast|b fast|c" {
		t.Fatalf("⑤ B·C는 기록으로 차례로: %q", got)
	}
}

// moveMain: 원격 main에 커밋 하나(기준이 움직임)
func (s *sfx) moveMain(name string) {
	s.t.Helper()
	s.git(s.seed, "fetch", "-q", "origin", "main")
	s.git(s.seed, "checkout", "-q", "-B", "main", "origin/main")
	os.WriteFile(s.seed+"/"+name+".txt", []byte(name+"\n"), 0o666)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", name)
	s.git(s.seed, "push", "-q", "origin", "main")
}

// 가-1(preverify 2차 감사 낮음 2): 제출하지 않는 표식은 기준이 두 번 움직여도 처음 + HC_LANE_PRE_REBUILD(기본 1)번까지만 미리 검증한다
func TestPreRebuildCapped(t *testing.T) {
	s := preFx(t)
	s.pre("p", time.Now().Unix(), false)
	for i, want := range []int{1, 2, 2, 2} {
		if i > 0 {
			s.moveMain(fmt.Sprintf("m%d", i))
		}
		s.round(1)
		d := s.dispatched()
		if len(d) != want {
			t.Fatalf("기준 이동 %d번 뒤 띄움 %d, 기대 %d: %v", i, len(d), want, d)
		}
		if rc := s.verifyOn(d[len(d)-1]); rc != 0 && i < 2 {
			t.Fatalf("일꾼 종료 %d", rc)
		}
	}
	if !s.has(verified.PreRefs + "p") {
		t.Fatal("상한을 넘겨도 표식은 나이 상한까지 둔다(제출되면 병합기가 직접 검증)")
	}
	// 다시 완료 보고(새 표식 커밋)는 처음부터 센다
	s.mark("p", s.git(s.seed, "rev-parse", "p"), time.Now().Unix()+1)
	s.round(1)
	if len(s.dispatched()) != 3 {
		t.Fatalf("다시 보고하면 새로 짓는다: %v", s.dispatched())
	}
}

// 가-2: 나이 상한 기본은 45분 — 6시간이던 때 남던 표식(50분)과 그 추측·기록을 치운다
func TestPreMaxDefault(t *testing.T) {
	if got := specPreMax(); got != 2700 {
		t.Fatalf("HC_LANE_PRE_MAX 기본 %d, 기대 2700", got)
	}
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	s.mark("p", sha, time.Now().Unix()-3000)
	s.round(1)
	if r := append(append(s.refs(specRefs), s.refs(verified.Prefix)...), s.refs(verified.PreRefs)...); len(r) != 0 || len(s.dispatched()) != 1 {
		t.Fatalf("나이 상한 뒤 표식·추측·기록을 치운다: %v 띄움 %v", r, s.dispatched())
	}
}

// 나(preverify 2차 감사 낮음 2): cap이 끝나면 solo도 지운다 — 같은 sha 재제출 + 기준 그대로 + 묶음 실패가 다시 겹쳐도 옛 solo가
// 같은 아이디의 미리 줄 추측 실패를 반송 근거로 믿게 하지 않는다(병합기가 직접 검증한 뒤 반송)
func TestSoloClearedWithCap(t *testing.T) {
	oldFront(t)
	s := culpritFx(t, true)
	_, sa := s.sub2(1000000001, "a", "", "", "bad.txt", "a\n", false)
	s.submit(1000000002, "b", "보통")
	s.submit(1000000003, "c", "보통")
	s.round(3)
	s.round(3)          // [a,b] 직접 + a 혼자 추측(solo)
	d := s.dispatched() // a 혼자(v1)와 그 뒤 [c](v2 — 끝내지 않고 둠)
	if len(d) != 2 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("a 혼자 추측 실패: %v", d)
	}
	x := d[0][1]
	s.round(3) // 범인 a 반송 — cap 끝
	solo := s.clone + "/.git/hc-merger/solo"
	if _, err := os.Stat(solo); err == nil || !s.has("refs/merge-failed/1000000001-a") {
		t.Fatalf("cap이 끝나면 solo도 지운다(반송 %v)", s.has("refs/merge-failed/1000000001-a"))
	}
	// b·c는 거둬 기준을 그대로 두고, 같은 sha a를 미리 줄로 다시 띄워 묶음 [a,d]가 다시 실패하게 한다
	s.git(s.seed, "push", "-q", "origin", ":refs/merge-queue/1000000002-b", ":refs/merge-queue/1000000003-c",
		":refs/merge-queue-meta/1000000002-b", ":refs/merge-queue-meta/1000000003-c")
	s.round(3)
	s.mark("a", sa, time.Now().Unix())
	s.round(3)
	d = s.dispatched()
	if len(d) != 3 || d[2][1] != x {
		t.Fatalf("기준 그대로 — 미리 줄 추측 아이디가 옛 혼자 추측과 같다: %v", d)
	}
	s.enqueue(1000000004, "a", sa)
	s.submit(1000000005, "d", "보통")
	s.round(3) // [a,d] 직접 검증 실패 → cap 1
	s.verifyOn(d[2])
	if !s.has(specFails + x) {
		t.Fatal("전제: 미리 줄 추측 시험 실패 기록")
	}
	s.round(3)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|a verify|a verify|a" || !s.has("refs/merge-failed/1000000004-a") {
		t.Fatalf("미리 줄 실패만으로 반송하지 않고 직접 검증 뒤 반송: %q", got)
	}
}

// ⑥ 상한 설정값: 비었거나 0·음수·글자·최소(60) 미만이면 기본 600
func TestSpecFrontWaitSetting(t *testing.T) {
	for v, want := range map[string]int64{"": 600, "0": 600, "-5": 600, "abc": 600, "59": 600, "60": 60, "900": 900} {
		t.Setenv("HC_LANE_SPEC_FRONT_WAIT", v)
		if got := specFrontWait(); got != want {
			t.Errorf("HC_LANE_SPEC_FRONT_WAIT=%q: %d, 기대 %d", v, got, want)
		}
	}
}
