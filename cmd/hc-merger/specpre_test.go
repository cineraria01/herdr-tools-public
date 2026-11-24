//go:build !windows

package main

// 미리 줄(specpre.go): 완료 보고 때 올린 refs/merge-pre/<레인> 표식으로 병합기가 빈 검증 기계에 「기준 + 그 커밋」 추측을 미리 띄우고,
// 같은 sha가 제출되면 그 추측이 곧 맨 앞 무리 추측이다. 시험 틀은 specrun_test.go(specFx — 가짜 원격·가짜 hc·가짜 land).

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/verified"
)

// preFx: specFx에 착지가 받은 HERDR_SPEC_TREE를 적는 줄을 더한다(병합기가 「통과」로 쳤는지 착지 판정과 따로 본다)
func preFx(t *testing.T) *sfx {
	s := specFx(t)
	tl := s.dir + "/tree.log"
	t.Setenv("TREELOG", tl)
	old, _ := os.ReadFile(gSelf)
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), "if [ \"$1\" = land ]; then\n",
		"if [ \"$1\" = land ]; then\n  echo \"$HC_LANE|$HERDR_SPEC_TREE\" >>\"$TREELOG\"\n", 1)), 0o777)
	return s
}

// pre: seed에서 main 위 레인 커밋 하나를 만들고 미리 줄 표식(완료 보고가 올리는 꼴)을 올린다. 그 커밋 SHA
func (s *sfx) pre(lane string, at int64, bad bool) string {
	s.t.Helper()
	s.git(s.seed, "checkout", "-q", "-B", lane, "main")
	os.WriteFile(s.seed+"/"+lane+".txt", []byte(lane+"\n"), 0o666)
	if bad {
		os.WriteFile(s.seed+"/bad.txt", []byte(lane+"\n"), 0o666)
	}
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", lane)
	sha := s.git(s.seed, "rev-parse", "HEAD")
	s.mark(lane, sha, at)
	s.git(s.seed, "checkout", "-q", "main")
	return sha
}

// mark: 표식만 다시 올린다(다시 완료 보고)
func (s *sfx) mark(lane, sha string, at int64) {
	s.t.Helper()
	c := s.git(s.seed, "commit-tree", sha+"^{tree}", "-p", sha, "-m", verified.PreMsg(lane, "main", at))
	s.git(s.seed, "push", "-q", "-f", "origin", c+":"+verified.PreRefs+lane)
}

// enqueue: 그 커밋 그대로 제출(감사 통과 뒤 hc-lane land가 내는 꼴 — 큐 표식·제출 기록·lane 가지)
func (s *sfx) enqueue(sec int, lane, sha string) {
	s.t.Helper()
	fork := s.git(s.seed, "merge-base", sha, "main")
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	meta := s.git(s.seed, "commit-tree", et, "-p", sha, "-m",
		fmt.Sprintf("merge-queue lane=%s id=box-%s orig=%s fork=%s base=main host=box term=- owner=- job=- created_ms=- priority=보통", lane, lane, sha, fork))
	name := fmt.Sprintf("%d-%s", sec, lane)
	s.git(s.seed, "push", "-q", "origin", sha+":refs/merge-queue/"+name, meta+":refs/merge-queue-meta/"+name, sha+":refs/heads/lane/"+lane)
}

func (s *sfx) mainTree() string {
	return s.git(s.seed, "--git-dir="+s.origin, "rev-parse", "main^{tree}")
}

func (s *sfx) mainSHA() string { return s.git(s.seed, "--git-dir="+s.origin, "rev-parse", "main") }

func (s *sfx) treeLog(lane string) string {
	for _, l := range s.lines(s.dir + "/tree.log") {
		if n, t, _ := strings.Cut(l, "|"); n == lane {
			return t
		}
	}
	return "(착지 안 함)"
}

// ①② 완료 보고 → 표식 → 한가한 병합기 한 바퀴에 「기준 + 후보」 추측이 뜨고, 통과 뒤 같은 sha 제출은 검증 없이 착지
func TestPreSpecLandsWithoutVerify(t *testing.T) {
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" || d[0][1] != specID(s.mainTree(), sha) {
		t.Fatalf("① 미리 줄 추측 「기준 + p」를 v1에 띄워야: %v (기대 아이디 %s)", d, specID(s.mainTree(), sha))
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" || !s.onMain("p.txt") || s.treeLog("p") == "" {
		t.Fatalf("② 같은 sha 제출은 기록으로 검증 없이 착지: %q 트리 %q", got, s.treeLog("p"))
	}
	if len(s.dispatched()) != 1 {
		t.Fatalf("제출 뒤 추측을 새로 띄우지 않는다: %v", s.dispatched())
	}
	if s.has(verified.PreRefs + "p") {
		t.Fatal("제출되면 진짜 줄이 넘겨받아 표식은 치운다")
	}
	s.round(1)
	if r := append(append(s.refs(specRefs), s.refs(verified.Prefix)...), s.refs(verified.PreRefs)...); len(r) != 0 {
		t.Fatalf("착지 뒤 추측·기록·표식이 남음: %v", r)
	}
}

// ③ 미리 줄 추측이 도는 중에 제출 → 직접 검증을 새로 시작하지 않고 기다렸다가 착지
func TestPreSpecRunningWaits(t *testing.T) {
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("띄움: %v", d)
	}
	s.enqueue(1000000001, "p", sha)
	m0 := s.mainSHA()
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || s.mainSHA() != m0 || len(s.dispatched()) != 1 {
		t.Fatalf("③ 도는 중이면 기다린다: 착지 %v 띄움 %v", s.lines(s.landlog), s.dispatched())
	}
	if rc := s.verifyOn(d[0]); rc != 0 {
		t.Fatalf("일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|p" || !s.onMain("p.txt") {
		t.Fatalf("③ 통과 뒤 검증 없이 착지: %q", got)
	}
}

// ④ 그 사이 기준이 움직임 → 아이디가 달라 기록이 안 맞고 지금처럼 직접 검증(병합기가 통과로 치지 않음 — HERDR_SPEC_TREE 빈 값)
func TestPreSpecBaseMovedVerifies(t *testing.T) {
	oldFront(t)
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	s.git(s.seed, "fetch", "-q", "origin", "main")
	s.git(s.seed, "checkout", "-q", "-B", "main", "origin/main")
	os.WriteFile(s.seed+"/moved.txt", []byte("x\n"), 0o666)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "moved")
	s.git(s.seed, "push", "-q", "origin", "main")
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || !s.onMain("p.txt") || s.treeLog("p") != "" {
		t.Fatalf("④ 기준이 움직였으면 직접 검증: %q 트리 %q", got, s.treeLog("p"))
	}
}

// ⑤ 미리 검증 실패 → 반송 없음(사건·반송 표식 없음), 제출되면 지금처럼 병합기가 직접 검증해 지금 규칙대로 반송
func TestPreSpecFailNoReject(t *testing.T) {
	s := preFx(t)
	sha := s.pre("p", time.Now().Unix(), true)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("띄움: %v", d)
	}
	s.verifyOn(d[0]) // 실패는 spec-fail 참조로 남는다(일꾼 종료 코드는 기록 여부)
	s.round(1)
	if s.has("refs/merge-failed/1000000001-p") || len(s.refs("refs/merge-failed/")) != 0 || len(s.lines(s.events)) != 0 {
		t.Fatalf("⑤ 제출 전 미리 검증 실패로 반송·사건을 내지 않는다: %v %v", s.refs("refs/merge-failed/"), s.lines(s.events))
	}
	if !s.has(specFails+d[0][1]) || !s.has(verified.PreRefs+"p") {
		t.Fatal("⑤ 실패 기록과 표식은 남는다")
	}
	s.enqueue(1000000001, "p", sha)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" || !s.has("refs/merge-failed/1000000001-p") || s.onMain("bad.txt") {
		t.Fatalf("⑤ 제출되면 직접 검증해 반송: %q", got)
	}
}

// ⑥ 새 sha로 다시 완료 보고 → 옛 추측·기록 치움, 새 추측
func TestPreSpecReReport(t *testing.T) {
	s := preFx(t)
	at := time.Now().Unix()
	old := s.pre("p", at, false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	vold := s.refs(verified.Prefix)
	s.git(s.seed, "checkout", "-q", "p")
	os.WriteFile(s.seed+"/p2.txt", []byte("2\n"), 0o666)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "p2")
	nsha := s.git(s.seed, "rev-parse", "HEAD")
	s.git(s.seed, "checkout", "-q", "main")
	s.mark("p", nsha, at+1)
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][1] != specID(s.mainTree(), nsha) {
		t.Fatalf("⑥ 새 sha 추측: %v", d)
	}
	if s.has(specRefs+specID(s.mainTree(), old)) || len(vold) != 1 || s.has(vold[0]) {
		t.Fatal("⑥ 옛 추측과 그 기록은 치운다")
	}
}

// ⑦ 미리 줄만 있고 제출 없음 → main·큐 그대로, 상한(HC_LANE_PRE_MAX) 뒤 표식·추측·기록 치움
func TestPreSpecOnlyNeverLands(t *testing.T) {
	s := preFx(t)
	s.pre("p", time.Now().Unix()-1000, false)
	m0 := s.mainSHA()
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("띄움·통과: %v", d)
	}
	for range 2 {
		s.round(1)
	}
	if s.mainSHA() != m0 || len(s.lines(s.landlog)) != 0 || len(s.refs("refs/merge-queue/")) != 0 || len(s.refs("refs/heads/lane/")) != 0 {
		t.Fatalf("⑦ 표식만으로 착지·제출하지 않는다: 착지 %v", s.lines(s.landlog))
	}
	t.Setenv("HC_LANE_PRE_MAX", "600")
	s.round(1)
	if r := append(append(s.refs(specRefs), s.refs(verified.Prefix)...), s.refs(verified.PreRefs)...); len(r) != 0 {
		t.Fatalf("⑦ 상한 뒤 치움: %v", r)
	}
}

// ⑧ 진짜 줄 추측이 먼저 기계를 받는다: 큐 A·B와 표식 P·Q, 검증 기계 둘 → v1은 B(진짜 줄), v2는 P, Q는 기다림
func TestPreSpecQueueFirst(t *testing.T) {
	oldFront(t)
	s := preFx(t)
	now := time.Now().Unix()
	p := s.pre("p", now, false)
	s.pre("q", now, false)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	base := s.mainTree()
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 || d[0][0] != "v1" || d[1][0] != "v2" || d[1][1] != specID(base, p) {
		t.Fatalf("⑧ v1 = B(진짜 줄), v2 = P: %v", d)
	}
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" {
		t.Fatalf("A는 지금처럼 직접: %q", got)
	}
}

// ⑨(감사 atoz3el-ese56i 중간): 미리 줄 추측이 도는 중에 묶음 [p,q]로 제출돼 묶음이 실패하고, 그 뒤 미리 검증이 시험 실패로 끝나도
// cap 바퀴의 맨 앞 [p]는 그 기록만으로 반송하지 않고 지금처럼 병합기가 직접 검증한다(맨 앞 혼자 추측으로 믿는 것은 cap 바퀴에 지은 것뿐)
func TestPreSpecFailNotCulprit(t *testing.T) {
	oldFront(t)
	s := culpritFx(t, true)
	p := s.pre("p", time.Now().Unix(), true)
	s.round(2)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("미리 줄 띄움: %v", d)
	}
	s.enqueue(1000000001, "p", p)
	s.submit(1000000002, "q", "보통")
	s.round(2)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p" {
		t.Fatalf("묶음 [p,q] 직접 검증·실패: %q", got)
	}
	s.verifyOn(d[0])
	if !s.has(specFails + d[0][1]) {
		t.Fatal("미리 검증 시험 실패 기록")
	}
	s.round(2)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|p verify|p" || !s.has("refs/merge-failed/1000000001-p") {
		t.Fatalf("⑨ 미리 검증 실패 기록만으로 반송하지 않고 직접 검증 뒤 반송: %q", got)
	}
}

// 끄기: 검증 기계 목록에 #preverify=off 줄 → 띄우지 않고 표식을 치운다
func TestPreSpecOff(t *testing.T) {
	s := preFx(t)
	vs := []verified.Verifier{{Machine: "v1", Path: s.dir + "/v1"}, {Machine: "v2", Path: s.dir + "/v2"}}
	et := s.git(s.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := s.git(s.seed, "commit-tree", et, "-m", verified.FormatVerifiers(vs)+verified.PreOffLine+"\n")
	s.git(s.seed, "push", "-q", "-f", "origin", vc+":"+verified.VerifiersRef)
	s.pre("p", time.Now().Unix(), false)
	s.round(1)
	if len(s.dispatched()) != 0 || s.has(verified.PreRefs+"p") {
		t.Fatalf("끔이면 띄우지 않고 표식을 치운다: %v", s.dispatched())
	}
}
