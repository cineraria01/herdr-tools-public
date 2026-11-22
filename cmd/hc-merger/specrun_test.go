//go:build !windows

package main

// 추측 병렬 검증 바퀴(specrun.go·specverify.go): 가짜 원격(bare)·가짜 hc(run --on은 기록만 — 시험이 그 기계 클론에서 일꾼을 직접 부른다)·
// 가짜 hc-lane land(HERDR_SPEC_TREE가 rebase한 트리이고 원격 기록이 있으면 「fast」, 아니면 「verify」 — bad.txt가 있으면 실패).

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"herdr-tools/internal/verified"
)

const fakeSpecLand = `if [ "$1" = land ]; then
  git fetch -q origin || exit 1
  git rebase -q origin/main || exit 3
  t=$(git rev-parse 'HEAD^{tree}')
  if [ -n "$HERDR_SPEC_TREE" ] && [ "$t" = "$HERDR_SPEC_TREE" ] && [ -n "$(git ls-remote origin refs/herdr/verified/$t)" ]; then
    echo "fast|$HC_LANE" >>"$LANDLOG"
  else
    echo "verify|$HC_LANE" >>"$LANDLOG"
    [ ! -e bad.txt ] || exit 1
  fi
  git push -q --atomic origin HEAD:refs/heads/main $HERDR_LAND_PUSH_EXTRA || exit 1
  exit 0
fi
`

type sfx struct {
	*fx
	landlog, hclog string
}

// specFx: 켬 표식·검증 스크립트·검증 기계 둘(v1·v2 클론)·가짜 hc
func specFx(t *testing.T) *sfx {
	f := newFx(t)
	os.MkdirAll(f.seed+"/.claude", 0o777)
	os.WriteFile(f.seed+"/.claude/herdr-verify.sh", []byte("#!/bin/bash\n[ ! -e bad.txt ]\n"), 0o777)
	f.git(f.seed, "add", "-A")
	f.git(f.seed, "commit", "-qm", "verify script")
	f.git(f.seed, "push", "-q", "origin", "main", "main:refs/herdr/merger/box")
	s := &sfx{fx: f, landlog: f.dir + "/land.log", hclog: f.dir + "/hc.log"}
	t.Setenv("LANDLOG", s.landlog)
	t.Setenv("HC_LANE_SPEC_LOAD_WAIT", "0")
	old, _ := os.ReadFile(gSelf)
	os.WriteFile(gSelf, []byte(strings.Replace(string(old), "#!/bin/sh\n", "#!/bin/sh\n"+fakeSpecLand, 1)), 0o777)
	bin := f.dir + "/bin"
	os.MkdirAll(bin, 0o777)
	os.WriteFile(bin+"/hc", []byte("#!/bin/sh\n[ \"$1\" = run ] && echo \"$*\" >>"+s.hclog+"\nexit ${HC_RC:-0}\n"), 0o777)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var vs []verified.Verifier
	for _, v := range []string{"v1", "v2"} {
		f.git(f.dir, "clone", "-q", f.origin, f.dir+"/"+v)
		vs = append(vs, verified.Verifier{Machine: v, Path: f.dir + "/" + v})
	}
	et := f.git(f.seed, "hash-object", "-t", "tree", "-w", "/dev/null")
	vc := f.git(f.seed, "commit-tree", et, "-m", verified.FormatVerifiers(vs))
	f.git(f.seed, "push", "-q", "origin", vc+":"+verified.VerifiersRef)
	return s
}

func (s *sfx) lines(p string) []string {
	b, _ := os.ReadFile(p)
	return strings.Fields(string(b))
}

var dispRe = regexp.MustCompile(`--on (\S+) .*--spec-verify ([0-9a-f]{40}) main`)

// dispatched: 띄운 차례대로 「기계 아이디」
func (s *sfx) dispatched() [][2]string {
	b, _ := os.ReadFile(s.hclog)
	var o [][2]string
	for _, l := range strings.Split(string(b), "\n") {
		if m := dispRe.FindStringSubmatch(l); m != nil {
			o = append(o, [2]string{m[1], m[2]})
		}
	}
	return o
}

// verifyOn: 그 기계 클론에서 일꾼을 부른다(hc run --on이 그 기계에서 하는 일)
func (s *sfx) verifyOn(d [2]string) int {
	s.t.Helper()
	s.t.Setenv("HC_LANE_MACHINE", d[0])
	defer s.t.Setenv("HC_LANE_MACHINE", "box")
	s.t.Chdir(s.dir + "/" + d[0])
	defer s.t.Chdir(s.clone)
	return specVerifyMain([]string{d[1], "main"})
}

// round: 새 병합기 일꾼(앞 일꾼이 죽은 것과 같다 — 로컬 추측 참조도 지워 원격만으로 이어 감을 본다)
func (s *sfx) round(batch int) {
	s.t.Helper()
	for _, r := range strings.Fields(s.git(s.clone, "for-each-ref", "--format=%(refname)", specRefs)) {
		s.git(s.clone, "update-ref", "-d", r)
	}
	m := s.merger()
	m.batch = batch
	m.round()
}

func (s *sfx) refs(prefix string) []string {
	o, _ := exec.Command("git", "--git-dir="+s.origin, "for-each-ref", "--format=%(refname)", prefix).Output()
	return strings.Fields(string(o))
}

func (s *sfx) sub(sec int, lane string, bad bool) {
	if bad {
		s.sub2(sec, lane, "", "", "bad.txt", lane+"\n", false)
		return
	}
	s.submit(sec, lane, "보통")
}

// A·B·C: 첫 바퀴에 A는 직접 검증해 착지, B·C(=main+A+B+C)는 검증 기계 둘에서 함께. 통과 뒤 B·C는 검증 없이 차례로 착지,
// 병합기는 바퀴마다 새로 떠도(로컬 추측 참조 없이) 원격으로 이어 가고, 끝나면 추측 참조를 모두 치운다
func TestSpecChainLandsInOrder(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.sub(1000000003, "c", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 || d[0][0] != "v1" || d[1][0] != "v2" {
		t.Fatalf("B·C를 v1·v2에 띄워야: %v", d)
	}
	if got := s.lines(s.landlog); len(got) != 1 || got[0] != "verify|a" || !s.onMain("a.txt") {
		t.Fatalf("A는 직접 검증해 착지: %v", got)
	}
	for _, x := range d {
		if rc := s.verifyOn(x); rc != 0 {
			t.Fatalf("일꾼 %v 종료 %d", x, rc)
		}
	}
	if len(s.refs(verified.Prefix)) != 2 {
		t.Fatalf("검증 기록 둘: %v", s.refs(verified.Prefix))
	}
	s.round(1)
	s.round(1)
	got := s.lines(s.landlog)
	if strings.Join(got, " ") != "verify|a fast|b fast|c" || !s.onMain("b.txt") || !s.onMain("c.txt") {
		t.Fatalf("B·C는 기록으로 검증 없이 차례로: %v", got)
	}
	s.round(1) // 큐 빔 — 치우기
	if r := append(append(append(s.refs(specRefs), s.refs(specFails)...), s.refs(specStarts)...), s.refs(verified.Prefix)...); len(r) != 0 {
		t.Fatalf("끝난 추측 참조가 남음: %v", r)
	}
}

// B가 실패: B는 지금 규칙대로(직접 검증 → 반송), B를 품은 C 추측은 버리고 main+A+C를 다시 띄워 그 결과로 C가 착지
func TestSpecFailRespeculates(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", true)
	s.sub(1000000003, "c", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 {
		t.Fatalf("B·C 띄움: %v", d)
	}
	for _, x := range d {
		s.verifyOn(x)
	}
	if len(s.refs(specFails)) != 2 || len(s.refs(verified.Prefix)) != 0 {
		t.Fatalf("둘 다 실패 기록: %v %v", s.refs(specFails), s.refs(verified.Prefix))
	}
	s.round(1)
	if !s.has("refs/merge-failed/1000000002-b") || s.onMain("bad.txt") {
		t.Fatal("B는 반송")
	}
	old := d[1][1] // main+A+B+C
	d = s.dispatched()
	if len(d) != 3 {
		t.Fatalf("main+A+C를 다시 띄워야: %v", d)
	}
	if s.has(specRefs+old) || s.has(specFails+old) {
		t.Fatal("B를 품은 C 추측과 그 실패 기록은 치운다")
	}
	s.round(1) // C 도는 중 — 착지하지 않음
	if s.onMain("c.txt") {
		t.Fatal("C 추측이 도는 중에는 착지하지 않는다")
	}
	if rc := s.verifyOn(d[2]); rc != 0 {
		t.Fatalf("C 일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|b fast|c" || !s.onMain("c.txt") {
		t.Fatalf("착지 차례: %s", got)
	}
}

// 검증 기계에 못 띄움 → 모름, 그 제출은 병합기가 직접 검증
func TestSpecDispatchLostFallsBack(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_RC", "1")
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	rs := s.refs(specRefs)
	if len(rs) != 2 {
		t.Fatalf("A(local)·B(lost) 추측: %v", rs)
	}
	lost := 0
	for _, r := range rs {
		if strings.Contains(s.git(s.seed, "--git-dir="+s.origin, "log", "-1", "--format=%s", r), "state=lost") {
			lost++
		}
	}
	if lost != 1 {
		t.Fatal("못 띄운 B는 state=lost")
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|b" {
		t.Fatalf("B는 직접 검증: %s", got)
	}
}

// 띄웠는데 일꾼이 끝내 안 뜸(그 기계에 hc-lane이 없는 등) → 떴다는 표식 없이 HC_LANE_SPEC_START가 지나면 모름, 직접 검증
func TestSpecWorkerNeverStarted(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_SPEC_START", "0")
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	if len(s.dispatched()) != 1 {
		t.Fatal("B 띄움")
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|b" {
		t.Fatalf("안 뜬 일꾼을 기다리지 않고 직접 검증: %s", got)
	}
}

// 그 사이 main이 움직이면 앞 추측(과 그 검증 기록)은 버리고 새 기준에서 다시 — 착지는 직접 검증
func TestSpecMainMovedDiscards(t *testing.T) {
	s := specFx(t)
	s.sub(1000000001, "a", false)
	s.sub(1000000002, "b", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("B 띄움·통과: %v", d)
	}
	old := s.refs(specRefs)
	s.git(s.seed, "fetch", "-q", "origin", "main")
	s.git(s.seed, "checkout", "-q", "-B", "main", "origin/main")
	os.WriteFile(s.seed+"/moved.txt", []byte("x\n"), 0o666)
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "moved")
	s.git(s.seed, "push", "-q", "origin", "main")
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a verify|b" || !s.onMain("b.txt") {
		t.Fatalf("main이 움직였으면 B는 직접 검증: %s", got)
	}
	for _, r := range old {
		if s.has(r) {
			t.Fatalf("옛 추측 %s 가 남음", r)
		}
	}
	if len(s.refs(verified.Prefix)) != 0 {
		t.Fatal("옛 트리의 검증 기록도 치운다")
	}
}

// 묶음 둘(크기 2): 맨 앞 묶음 A+B는 직접, 뒤 묶음 C+D를 한 추측으로 띄워 통과하면 한 번에 검증 없이 착지
func TestSpecBatchGroup(t *testing.T) {
	s := specFx(t)
	for i, l := range []string{"a", "b", "c", "d"} {
		s.sub(1000000001+i, l, false)
	}
	s.round(2)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("C+D 한 추측: %v", d)
	}
	s.round(2)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a fast|c" || !s.onMain("d.txt") {
		t.Fatalf("묶음 C+D는 기록으로: %s", got)
	}
}

func TestVerifiersRoundTrip(t *testing.T) {
	vs := []verified.Verifier{{Machine: "box-1", Path: "/srv/a b"}, {Machine: "x", Path: "/y"}}
	got := verified.ParseVerifiers(verified.FormatVerifiers(vs) + "Bad\t/z\nok\trel\n")
	if fmt.Sprint(got) != fmt.Sprint(vs) {
		t.Fatalf("목록 = %v", got)
	}
}
