//go:build !windows

package main

// 통과 조각 이어 쓰기·직접 검증 자리(ht-merger-spec-reuse, specparts.go·specfront.go workerRedo).
// 틀은 specFx + 조각을 아는 검증 스크립트: 조각 p1·p2를 차례로 돌고(HERDR_VERIFY_PARTS의 같은 트리 조각은 건너뜀), 통과할 때마다 그 파일을
// 새로 쓴다. $PARTDIR/fail-<기계>가 있으면 그 기계에서 p2가 실패(시험 실패 줄 없음 = 가를 수 없음), $PARTDIR/linger가 있으면 p1 뒤에
// 잠깐 서서 원격에 조각이 올라왔는지 본다. 돈 것은 $HOOKLOG에 「run|skip <조각>@<기계>」.

import (
	"os"
	"strings"
	"testing"
	"time"
)

const partsVerify = `#!/bin/bash
t=$(git rev-parse 'HEAD^{tree}')
done=" "
if [ -n "$HERDR_VERIFY_PARTS" ] && [ -f "$HERDR_VERIFY_PARTS" ]; then
  read -r pt rest <"$HERDR_VERIFY_PARTS"
  [ "$pt" = "$t" ] && done=" $rest "
fi
passed=
for p in p1 p2; do
  case "$done" in *" $p "*) echo "skip $p@$HC_LANE_MACHINE" >>"$HOOKLOG"; passed="$passed $p"; continue;; esac
  echo "run $p@$HC_LANE_MACHINE" >>"$HOOKLOG"
  if [ "$p" = p2 ] && [ -e "$PARTDIR/fail-$HC_LANE_MACHINE" ]; then
    [ ! -e "$PARTDIR/testfail" ] || printf -- '--- FAIL: TestFlaky (0.01s)\nFAIL\texample/p2\t0.1s\n'
    exit 1
  fi
  passed="$passed $p"
  [ -z "$HERDR_VERIFY_PARTS" ] || echo "$t$passed" >"$HERDR_VERIFY_PARTS"
  if [ "$p" = p1 ] && [ -e "$PARTDIR/linger" ]; then
    sleep 1
    [ -z "$(git ls-remote origin 'refs/hc-merger/spec-part/*')" ] || echo seen >>"$HOOKLOG"
  fi
done
exit 0
`

// partsFx: 조각을 아는 검증 스크립트(script면 기본 브랜치에 착지 스크립트도). 돈 기록·표식 폴더
func partsFx(t *testing.T, script bool) (*sfx, string, string) {
	s := specFx(t)
	hl, pd := s.dir+"/hook.log", s.dir+"/parts"
	os.MkdirAll(pd, 0o777)
	t.Setenv("HOOKLOG", hl)
	t.Setenv("PARTDIR", pd)
	old := partTick
	partTick = 50 * time.Millisecond
	t.Cleanup(func() { partTick = old })
	os.WriteFile(s.seed+"/.claude/herdr-verify.sh", []byte(partsVerify), 0o777)
	if script {
		os.WriteFile(s.seed+"/.claude/herdr-land.sh", []byte("#!/bin/bash\nexit 0\n"), 0o777)
	}
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "parts verify")
	s.git(s.seed, "push", "-q", "origin", "main")
	return s, hl, pd
}

// ran: 돈 기록(줄마다)을 비운다 — 다음 실행만 보게
func ran(t *testing.T, hl string) string {
	t.Helper()
	b, _ := os.ReadFile(hl)
	os.Remove(hl)
	return strings.TrimSpace(string(b))
}

// partOf: 원격 spec-part 참조의 본문 첫 줄(없으면 "")
func (s *sfx) partOf(id string) string {
	if !s.has(specParts + id) {
		return ""
	}
	return partLine(s.git(s.dir, "--git-dir="+s.origin, "log", "-1", "--format=%b", specParts+id))
}

// specTree: 원격 spec 참조의 트리
func (s *sfx) specTree(id string) string {
	return s.git(s.dir, "--git-dir="+s.origin, "rev-parse", specRefs+id+"^{tree}")
}

// 가 검증하는 동안 통과 조각을 원격에 올린다(끝을 기다리지 않음 — 상한에 걸려 버려지는 실행의 조각도 남는다)
func TestPartsPublishedWhileRunning(t *testing.T) {
	s, hl, pd := partsFx(t, false)
	os.WriteFile(pd+"/linger", nil, 0o666)
	os.WriteFile(pd+"/fail-v1", nil, 0o666)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	if got := ran(t, hl); got != "run p1@v1\nseen\nrun p2@v1" {
		t.Fatalf("가 p1 통과 뒤 도는 중에 원격에 올라감: %q", got)
	}
	if got, want := s.partOf(d[0][1]), s.specTree(d[0][1])+" p1"; got != want {
		t.Fatalf("가 원격 조각 %q, 바람 %q", got, want)
	}
}

// 나 상한 넘김 · 한가한 다른 기계 있음(손잡이 1 기본) → 맥에서 직접 검증하지 않고 그 기계에, 앞 실행이 통과한 조각은 건너뛰고 남은 것만
func TestOverCapRedoResumesParts(t *testing.T) {
	s, hl, _ := partsFx(t, false)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	// v1 일꾼이 도는 중 — p1 통과까지 올림(그 일꾼의 partPub가 올린 꼴)
	og := "--git-dir=" + s.origin
	et := s.git(s.dir, og, "hash-object", "-t", "tree", "-w", "/dev/null")
	pc := s.git(s.dir, og, "commit-tree", et, "-m", "hc-merger-spec-part id="+d[0][1]+" machine=v1 at=1", "-m", s.specTree(d[0][1])+" p1")
	s.git(s.dir, og, "update-ref", specParts+d[0][1], pc)
	s.age(d[0][1], 700)
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][0] != "v2" || d[1][1] != d[0][1] || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("나 상한 넘김 → 한가한 v2에 같은 추측(직접 검증 안 함): 띄움 %v 착지 %v", d, s.lines(s.landlog))
	}
	if rc := s.verifyOn(d[1]); rc != 0 {
		t.Fatalf("v2 일꾼 종료 %d", rc)
	}
	if got := ran(t, hl); got != "skip p1@v2\nrun p2@v2" {
		t.Fatalf("나 p1은 건너뛰고 p2만: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" || !s.onMain("a.txt") {
		t.Fatalf("나 v2 통과 기록으로 착지(직접 검증 없음): %q", got)
	}
	s.round(1) // 큐가 빈 바퀴가 치운다(spec·spec-run과 함께)
	if r := append(s.refs(specParts), s.refs(specRefs)...); len(r) != 0 {
		t.Fatalf("나 착지 뒤 조각 참조를 치운다: %v", r)
	}
}

// 다 착지 스크립트 저장소 · 맨 앞 추측 실패(한 기계에서 p2 흔들림) · 한가한 다른 기계 있음 → 착지 스크립트(병합기 기계 전체 검증)
// 전에 그 기계에서 p2만 다시, 통과하면 기록으로 착지 — 다른 기계도 바쁘면 지금처럼 착지 스크립트로(대조는 specscript_test.go ①·⑤)
func TestScriptRepoFailRedoOnWorker(t *testing.T) {
	s, hl, pd := partsFx(t, true)
	os.WriteFile(pd+"/fail-v1", nil, 0o666)
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("v1 추측 실패 기록: %v", d)
	}
	ran(t, hl)
	s.round(1)
	d = s.dispatched()
	if len(d) != 2 || d[1][0] != "v2" || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("다 착지 스크립트 전에 v2에: 띄움 %v 착지 %v", d, s.lines(s.landlog))
	}
	s.verifyOn(d[1])
	if got := ran(t, hl); got != "skip p1@v2\nrun p2@v2" {
		t.Fatalf("다 p2만: %q", got)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("다 통과 기록으로 착지: %q", got)
	}
}

// 라 한 번만 — 다시 맡긴 기계에서도 실패면 또 옮기지 않고 손잡이대로(기본 = 병합기 기계 직접 검증, 반송 없음 — 가를 수 없는 실패)
func TestFailRedoOncePerFront(t *testing.T) {
	s, _, pd := partsFx(t, false)
	os.WriteFile(pd+"/fail-v1", nil, 0o666)
	os.WriteFile(pd+"/fail-v2", nil, 0o666)
	s.sub(1000000001, "a", false)
	s.round(1)
	s.verifyOn(s.dispatched()[0])
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 || d[1][0] != "v2" {
		t.Fatalf("라 v2에 한 번: %v", d)
	}
	s.verifyOn(d[1])
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); len(s.dispatched()) != 2 || got != "verify|a" || s.has("refs/merge-failed/1000000001-a") {
		t.Fatalf("라 두 번째는 직접 검증: 띄움 %v 착지 %q", s.dispatched(), got)
	}
}

// 마 트리가 바뀌면 이어 쓰지 않는다 — 기준이 움직여 새 추측(아이디가 다름)은 앞 실행의 조각을 안 읽고 다 돌고, 옛 조각 참조는 치운다.
// 같은 아이디라도 조각 줄의 트리가 이 추측 트리가 아니면 깔지 않는다
func TestPartsNotReusedWhenTreeChanges(t *testing.T) {
	s, hl, pd := partsFx(t, false)
	os.WriteFile(pd+"/fail-v1", nil, 0o666)
	s.sub(1000000001, "a", false)
	s.round(1)
	d0 := s.dispatched()[0]
	s.verifyOn(d0)
	if s.partOf(d0[1]) == "" {
		t.Fatal("마 v1의 p1 조각이 원격에")
	}
	ran(t, hl)
	os.Remove(pd + "/fail-v1")
	s.moveMain("m2")
	s.round(1)
	d := s.dispatched()
	if len(d) != 2 || d[1][1] == d0[1] {
		t.Fatalf("마 기준이 움직여 새 추측: %v", d)
	}
	s.verifyOn(d[1])
	if got := ran(t, hl); got != "run p1@"+d[1][0]+"\nrun p2@"+d[1][0] {
		t.Fatalf("마 새 트리는 처음부터: %q", got)
	}
	if s.has(specParts + d0[1]) {
		t.Fatal("마 옛 추측의 조각 참조를 치운다")
	}
	// 같은 아이디 · 다른 트리의 조각 줄 → 깔지 않음
	lf, _ := os.CreateTemp(t.TempDir(), "log")
	defer lf.Close()
	t.Chdir(s.dir + "/v1")
	og := "--git-dir=" + s.origin
	et := s.git(s.dir, og, "hash-object", "-t", "tree", "-w", "/dev/null")
	pc := s.git(s.dir, og, "commit-tree", et, "-m", "hc-merger-spec-part id=x", "-m", et+" p1 p2")
	s.git(s.dir, og, "update-ref", specParts+d[1][1], pc)
	f := t.TempDir() + "/parts"
	if p := specPartsSeed(d[1][1], d[1][1], s.specTree(d[1][1]), f, lf); p.line != "" {
		t.Fatalf("마 다른 트리의 조각을 깔았다: %q", p.line)
	}
	if _, err := os.Stat(f); err == nil {
		t.Fatal("마 다른 트리면 조각 파일을 만들지 않는다")
	}
}

// 바 맨 앞 추측이 시험 실패(test=1)로 끝나 병합기 기계가 직접 검증(착지 스크립트 저장소 — 다시 맡긴 기계에서도 실패)하면, 그 추측의
// 통과 조각 줄을 HERDR_VERIFY_PARTS_FROM 파일로 넘긴다(쓸지는 검증 스크립트가). 조각이 없으면 넘기지 않는다
func TestDirectVerifyGetsSpecParts(t *testing.T) {
	for _, drop := range []bool{false, true} {
		t.Run(map[bool]string{false: "조각있음", true: "조각없음"}[drop], func(t *testing.T) {
			s, _, pd := partsFx(t, true)
			for _, f := range []string{"fail-v1", "fail-v2", "testfail"} {
				os.WriteFile(pd+"/"+f, nil, 0o666)
			}
			s.sub(1000000001, "a", false)
			s.round(1)
			d := s.dispatched()
			if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
				t.Fatalf("v1 추측 실패 기록: %v", d)
			}
			og := "--git-dir=" + s.origin
			if subj := s.git(s.dir, og, "log", "-1", "--format=%s", specFails+d[0][1]); !strings.Contains(subj, " test=1") {
				t.Fatalf("v1 실패는 시험 실패(test=1): %q", subj)
			}
			s.round(1)
			d = s.dispatched()
			if len(d) != 2 || d[1][0] != "v2" {
				t.Fatalf("한가한 v2에 한 번 더: %v", d)
			}
			s.verifyOn(d[1])
			want := s.specTree(d[0][1]) + " p1"
			if got := s.partOf(d[0][1]); got != want {
				t.Fatalf("원격 조각 %q, 바람 %q", got, want)
			}
			if drop {
				s.git(s.dir, og, "update-ref", "-d", specParts+d[0][1])
			}
			s.round(1)
			if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" {
				t.Fatalf("직접 검증(착지 스크립트): %q", got)
			}
			b, _ := os.ReadFile(s.landlog + ".parts")
			if got := strings.TrimSpace(string(b)); drop && got != "" || !drop && got != "a|"+want {
				t.Fatalf("직접 검증에 넘긴 조각 %q (조각 없음 %v)", got, drop)
			}
		})
	}
}

// 사 맨 앞 추측이 없으면(손잡이 front_dispatch 0 — 병합기가 직접 검증, 짓기만) 조각 파일을 넘기지 않는다
func TestDirectVerifyNoSpecNoParts(t *testing.T) {
	oldFront(t)
	s, _, _ := partsFx(t, false)
	s.sub(1000000001, "a", false)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" {
		t.Fatalf("직접 검증: %q", got)
	}
	if _, err := os.Stat(s.landlog + ".parts"); err == nil {
		t.Fatal("추측이 없으면 조각 파일을 넘기지 않는다")
	}
}
