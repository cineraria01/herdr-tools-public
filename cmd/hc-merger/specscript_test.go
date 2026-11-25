//go:build !windows

package main

// spec-script-land: 착지를 저장소 착지 스크립트(.claude/herdr-land.sh)에 맡기는 저장소에서 추측 검증이 줄을 세우지 않게.
// 그 꼴: 생성 파일이 rebase·묶음에서 빠지고 착지 스크립트가 착지 중에 다시 만든다 — 다시 만드는 단계가 없는 추측 검증은 늘 실패한다.
// 틀은 specFx(specrun_test.go) + 검증 스크립트가 gen.txt에 ok가 없으면 실패(out을 찍고 종료 1) — 가짜 착지는 gen.txt를 보지 않는다
// (착지 스크립트가 다시 만들고 검증해 통과하는 것과 같다). 준비 훅·검증의 차례와 환경은 $HOOKLOG에 한 줄씩.

import (
	"os"
	"strings"
	"testing"
)

// scriptFx: script = 기본 브랜치에 착지 스크립트, hook = 준비 훅 본문(빈 값이면 훅 없음), out = 생성 파일이 낡았을 때 검증이 찍는 글
func scriptFx(t *testing.T, script bool, hook, out string) (*sfx, string) {
	s := specFx(t)
	hl := s.dir + "/hook.log"
	t.Setenv("HOOKLOG", hl)
	w := func(p, body string) { os.WriteFile(s.seed+"/"+p, []byte(body), 0o777) }
	w(".claude/herdr-verify.sh", "#!/bin/bash\necho verify >>\"$HOOKLOG\"\n[ ! -e bad.txt ] || exit 1\ngrep -q ok gen.txt && exit 0\ncat <<'EOF'\n"+out+"\nEOF\nexit 1\n")
	w(".claude/herdr-setup.sh", "#!/bin/bash\necho setup >>\"$HOOKLOG\"\n")
	w("gen.txt", "old\n")
	if script {
		w(".claude/herdr-land.sh", "#!/bin/bash\nexit 0\n")
	}
	if hook != "" {
		w(".claude/herdr-spec-prepare.sh", "#!/bin/bash\n"+hook)
	}
	s.git(s.seed, "add", "-A")
	s.git(s.seed, "commit", "-qm", "script repo")
	s.git(s.seed, "push", "-q", "origin", "main")
	return s, hl
}

const genHook = "echo \"prepare $HERDR_SPEC_BASE $HERDR_SPEC_BASE_SHA $HERDR_SPEC_MEMBERS\" >>\"$HOOKLOG\"\necho ok >gen.txt\n"

// ① 착지 스크립트 저장소 + front_local 0 + 맨 앞 추측 실패(가를 수 없음) → 기다리지 않고 착지 스크립트로(사건 없음)
func TestScriptRepoFrontLocalZeroLands(t *testing.T) {
	s, _ := scriptFx(t, true, "", "타입 검사 오류")
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("맨 앞 추측 실패 기록: %v", d)
	}
	s.idle("v2", verifyBusy) // 한가한 다른 기계가 없을 때(있으면 먼저 그 기계에 — specreuse_test.go)
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "verify|a" || !s.onMain("a.txt") || strings.Contains(strings.Join(s.evs(), "\n"), "막힘") {
		t.Fatalf("① 착지 스크립트에 넘겨 착지(기다림·막힘 사건 없음): 착지 %q 사건 %v", got, s.evs())
	}
}

// ② 같은 저장소에서 추측이 시험 실패(두 기계에서도)여도 반송 없음 — 착지 스크립트가 통과하면 착지
func TestScriptRepoSpecFailNeverRejects(t *testing.T) {
	s, _ := scriptFx(t, true, "", "--- FAIL: TestGen (0.00s)")
	s.sub(1000000001, "a", false)
	s.drive(1, 4)
	if s.has("refs/merge-failed/1000000001-a") || !s.onMain("a.txt") {
		t.Fatalf("② 추측 실패로 반송하지 않고 착지: 착지 %v 띄움 %v", s.lines(s.landlog), s.dispatched())
	}
}

// ③ 대조 — 내장 착지 저장소 + front_local 0 → 지금 그대로 기다림·사건 한 번
func TestBuiltinRepoFrontLocalZeroWaits(t *testing.T) {
	s, _ := scriptFx(t, false, "", "타입 검사 오류")
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	s.round(1)
	s.round(1)
	if len(s.lines(s.landlog)) != 0 || len(s.evs()) != 1 {
		t.Fatalf("③ 내장 착지는 그대로 기다림: 착지 %v 사건 %v", s.lines(s.landlog), s.evs())
	}
}

// ④ 착지 스크립트 저장소 + 준비 훅 → setup 뒤·verify 앞에 한 번, 기준 가지·기준 SHA·구성원 — 훅이 고친 추적 파일은 「검증이 트리를 바꿈」이 아니다
func TestSpecPrepareHookRuns(t *testing.T) {
	s, hl := scriptFx(t, true, genHook, "타입 검사 오류")
	base := s.mainSHA()
	_, sha, _, _ := s.submit(1000000001, "a", "보통")
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || s.has(specFails+d[0][1]) {
		t.Fatalf("④ 훅이 다시 만들어 추측 통과: %v", d)
	}
	b, _ := os.ReadFile(hl)
	if want := "setup\nprepare main " + base + " " + sha + "\nverify\n"; string(b) != want {
		t.Fatalf("④ 차례·환경:\n%q\n바람:\n%q", b, want)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" {
		t.Fatalf("④ 통과 기록으로 착지: %q", got)
	}
}

// ⑤ 훅 실패 → 모름(실패 참조 없음, 검증 안 돎, 반송 없음)
func TestSpecPrepareHookFailIsUnknown(t *testing.T) {
	s, hl := scriptFx(t, true, "exit 3\n", "--- FAIL: TestGen (0.00s)")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || s.has(specFails+d[0][1]) {
		t.Fatalf("⑤ 훅 실패는 실패 참조가 아니다: %v", d)
	}
	if !strings.Contains(s.specGroupOf(d[0][1]), "state=lost") {
		t.Fatalf("⑤ 모름(state=lost): %s", s.specGroupOf(d[0][1]))
	}
	if b, _ := os.ReadFile(hl); strings.Contains(string(b), "verify") {
		t.Fatalf("⑤ 훅이 실패하면 검증하지 않는다: %q", b)
	}
	s.idle("v1", verifyBusy) // 한가한 검증 기계가 없을 때 착지 스크립트로(모름 줄은 기계가 「-」라 v1도 후보다)
	s.idle("v2", verifyBusy)
	s.round(1)
	if s.has("refs/merge-failed/1000000001-a") || !s.onMain("a.txt") {
		t.Fatal("⑤ 반송 없이 착지 스크립트로 착지")
	}
}

// ⑥ 내장 착지 저장소에 훅 → 돌지 않고 로그에 알림 한 줄(검증한 트리 ≠ 넣을 트리가 되지 않게)
func TestSpecPrepareHookSkippedOnBuiltin(t *testing.T) {
	s, hl := scriptFx(t, false, genHook, "타입 검사 오류")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	if b, _ := os.ReadFile(hl); string(b) != "setup\nverify\n" {
		t.Fatalf("⑥ 훅은 돌지 않는다: %q", b)
	}
	lg, _ := os.ReadFile(s.dir + "/v1.spec-wt/" + d[0][1][:8] + ".log")
	if !strings.Contains(string(lg), "herdr-spec-prepare.sh") || !strings.Contains(string(lg), "내장 착지") {
		t.Fatalf("⑥ 알림 한 줄:\n%s", lg)
	}
}

// ⑦ 훅 없음 → 지금과 같다(setup → verify, 실패면 실패 참조)
func TestSpecNoHookSame(t *testing.T) {
	s, hl := scriptFx(t, true, "", "--- FAIL: TestGen (0.00s)")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || s.verifyOn(d[0]) != 0 || !s.has(specFails+d[0][1]) {
		t.Fatalf("⑦ 실패 참조: %v", d)
	}
	if b, _ := os.ReadFile(hl); string(b) != "setup\nverify\n" {
		t.Fatalf("⑦ 차례: %q", b)
	}
}

// 라 front_local 0 + 맨 앞 추측이 기다림 상한을 넘김 + 다른 기계 한가 → 그 기계에 한 번 더 맡겨 그 결과로 착지(사건 없음)
func TestOverCapNoLocalRedispatches(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 || d[0][0] != "v1" {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	s.age(d[0][1], 700)
	out := stdoutOf(t, func() { s.round(1) })
	d = s.dispatched()
	if len(d) != 2 || d[1][0] != "v2" || len(s.evs()) != 0 || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("라 한가한 v2에 다시: 띄움 %v 사건 %v 착지 %v", d, s.evs(), s.lines(s.landlog))
	}
	// 다: 재지 않은 기계(추측이 도는 v1)도 한가함 줄에 까닭과 함께
	if !strings.Contains(out, "검증 기계 한가함 — v2 한가함(") || !strings.Contains(out, " · v1 추측 도는 중") {
		t.Fatalf("다 한가함 줄에 v1 추측 도는 중:\n%s", out)
	}
	if rc := s.verifyOn(d[0]); rc != 0 { // 느린 v1이 뒤늦게 끝남 — 참조가 바뀌어 결과를 남기지 않는다
		t.Fatalf("옛 일꾼 종료 %d", rc)
	}
	if rc := s.verifyOn(d[1]); rc != 0 {
		t.Fatalf("새 일꾼 종료 %d", rc)
	}
	s.round(1)
	if got := strings.Join(s.lines(s.landlog), " "); got != "fast|a" || !s.onMain("a.txt") {
		t.Fatalf("라 다시 맡긴 결과로 한 번 착지: %q", got)
	}
}

// 라 맨 앞 제출마다 한 번 — 옮긴 기계에서도 상한을 넘기면 또 옮기지 않고 기다림·막힘 사건 한 번(감사 atp02sl-4fkdo9 중간: 끝없이 옮겨 다님)
func TestOverCapRedoOncePerFront(t *testing.T) {
	s := specFx(t)
	t.Setenv("HC_LANE_FRONT_LOCAL", "0")
	s.sub(1000000001, "a", false)
	s.round(1)
	d := s.dispatched()
	if len(d) != 1 {
		t.Fatalf("맨 앞 띄움: %v", d)
	}
	for i := 0; i < 4; i++ {
		s.age(d[0][1], 700)
		s.round(1)
	}
	if got := s.dispatched(); len(got) != 2 || len(s.evs()) != 1 || !strings.Contains(s.evs()[0], "막힘") || len(s.lines(s.landlog)) != 0 {
		t.Fatalf("라 한 번만 옮기고 그 뒤는 기다림·사건 한 번: 띄움 %v 사건 %v 착지 %v", got, s.evs(), s.lines(s.landlog))
	}
}

// stdoutOf: f가 찍은 표준 출력
func stdoutOf(t *testing.T, f func()) string {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = tmp
	f()
	os.Stdout = old
	tmp.Close()
	b, _ := os.ReadFile(tmp.Name())
	return string(b)
}

// 라 대조 — 다른 기계가 다 바쁘면 지금처럼 기다림·사건 한 번: frontdispatch_test.go TestFrontOverCapNoLocalWaits(v2 바쁨)
