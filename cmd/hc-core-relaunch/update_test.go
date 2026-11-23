package main

// 판 맞추기(--update·--all)와 점검 판정 시험 — 가짜 hc·ssh·worker-mac-check(PATH 앞)만 쓴다. 실제 기계에 업데이트·다시 띄우기를 돌리지 않는다.
// 기계 상태는 $FAKE/m/<기계>/ 파일: ver(판) state(연결) os(uname) terms·spec(「주소 이름」 줄) check(점검 출력)
// slow(업데이트가 멈춤) vanish(업데이트가 터미널을 지움) finish(업데이트가 터미널을 finished로 — 목록에는 남음)
// drop(업데이트 뒤 연결 상태를 그 값으로) reconn(machine ls를 그 횟수만큼 읽으면 다시 connected) stuck(업데이트해도 판 그대로) sshrc(ssh 종료 코드) specn(검증 터미널이 보이는 횟수).

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeHC = `#!/bin/bash
S=$FAKE
echo "hc $*" >>"$S/log"
case "$1" in
  -V) echo "hc $(cat "$S/base")" ;;
  machine)
    case "$2" in
      ls) printf '{"machines":['; sep=
        for d in "$S"/m/*; do n=${d##*/}
          if [ -f "$d/reconn" ]; then k=$(($(cat "$d/reconn") - 1)); echo $k >"$d/reconn"; [ $k -le 0 ] && { echo connected >"$d/state"; rm -f "$d/reconn"; }; fi
          printf '%s{"name":"%s","state":"%s","version":"%s"}' "$sep" "$n" "$(cat "$d/state" 2>/dev/null || echo connected)" "$(cat "$d/ver")"; sep=,
        done; echo ']}' ;;
      update) d=$S/m/$3
        [ -f "$d/slow" ] && sleep 30
        [ -f "$d/stuck" ] || cp "$S/base" "$d/ver"
        [ -f "$d/vanish" ] && : >"$d/terms"
        [ -f "$d/finish" ] && { while read -r ad nm st; do echo "$ad $nm finished"; done <"$d/terms" >"$d/terms.new"; mv "$d/terms.new" "$d/terms"; }
        [ -f "$d/drop" ] && cp "$d/drop" "$d/state"
        echo "updated $3" ;;
    esac ;;
  ls) f=terms; for a; do [ "$a" = --label ] && f=spec; done
    printf '{"terminals":['; sep=
    for d in "$S"/m/*; do n=${d##*/}; [ -f "$d/$f" ] || continue
      while read -r ad nm st; do printf '%s{"address":"%s/%s","name":"%s","state":"%s"}' "$sep" "$n" "$ad" "$nm" "${st:-live}"; sep=,; done <"$d/$f"
      if [ "$f" = spec ] && [ -f "$d/specn" ]; then n=$(($(cat "$d/specn") - 1)); echo $n >"$d/specn"; [ $n -le 0 ] && rm -f "$d/spec" "$d/specn"; fi
    done; echo ']}' ;;
  exec) cat "$S/m/$3/os" ;;
esac
`

const fakeSSH = `#!/bin/bash
cat >/dev/null
m=${@: -2:1}
echo "ssh $m" >>"$FAKE/log"
exit "$(cat "$FAKE/m/$m/sshrc" 2>/dev/null || echo 0)"
`

const fakeCheck = `#!/bin/bash
echo "worker-mac-check $1" >>"$FAKE/log"
f=$FAKE/m/$1/check
[ -f "$f" ] && { cat "$f"; exit 1; }
printf '통과\tSSH 연결\t키 로그인\n통과\t코어 문맥(Aqua)\tlaunchd 작업\n통과\t키체인(hc 문맥)\t열림\n통과\tbrew PATH(hc 문맥)\t있음\n'
`

type fake struct {
	t   *testing.T
	dir string
}

func newFake(t *testing.T) *fake {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	for n, s := range map[string]string{"hc": fakeHC, "ssh": fakeSSH, "worker-mac-check": fakeCheck} {
		put(t, filepath.Join(bin, n), s, 0o755)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("FAKE", d)
	t.Setenv("HC_CORE_RELAUNCH_POLL", "0")
	put(t, d+"/base", "0.2.0\n", 0o644)
	put(t, d+"/log", "", 0o644)
	return &fake{t, d}
}

func put(t *testing.T, p, s string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), mode); err != nil {
		t.Fatal(err)
	}
}

// mach: 기계 하나(판 ver, OS os, 터미널 「주소 이름」 줄들)
func (f *fake) mach(m, ver, osName string, terms ...string) {
	f.set(m, "ver", ver)
	f.set(m, "os", osName)
	f.set(m, "terms", strings.Join(terms, "\n")+"\n")
}

func (f *fake) set(m, k, v string) { put(f.t, filepath.Join(f.dir, "m", m, k), v+"\n", 0o644) }

func (f *fake) log() string {
	b, err := os.ReadFile(f.dir + "/log")
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *fake) run(args ...string) (string, int) {
	var b bytes.Buffer
	rc := run(context.Background(), args, &b)
	return b.String(), rc
}

func TestUpdateSameVersionDoesNothing(t *testing.T) { // ①
	f := newFake(t)
	f.mach("w1", "0.2.0", "Darwin", "t1 lane-a")
	out, rc := f.run("--update", "w1")
	if rc != 0 || !strings.Contains(out, "w1\t같음\t") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	if l := f.log(); strings.Contains(l, "machine update") || strings.Contains(l, "ssh ") || strings.Contains(l, "worker-mac-check") {
		t.Fatalf("판이 같은데 손댐:\n%s", l)
	}
}

func TestUpdateMacUpdatesRelaunchesChecks(t *testing.T) { // ②
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a", "t2 merger-x")
	out, rc := f.run("--update", "w1")
	l := f.log()
	if rc != 0 || !strings.Contains(out, "w1\t성공\t0.1.0 → 0.2.0, 다시 띄움") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	iu, is, ic := strings.Index(l, "hc machine update w1"), strings.Index(l, "ssh w1"), strings.Index(l, "worker-mac-check w1")
	if iu < 0 || is < iu || ic < is {
		t.Fatalf("차례가 업데이트 → 다시 띄우기 → 점검이 아님:\n%s", l)
	}
	// 하기 전에 상태(판·터미널·병합 줄)를 먼저 보인다
	if i := strings.Index(out, "-- 상태"); i < 0 || i > strings.Index(out, "업데이트:") || !strings.Contains(out, "병합기: merger-x") {
		t.Fatalf("상태를 먼저 보이지 않음:\n%s", out)
	}
}

func TestUpdateCheckFailIsFailure(t *testing.T) { // ③
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.set("w1", "check", "통과\t코어 문맥(Aqua)\tok\n실패\t키체인(hc 문맥)\tUser interaction is not allowed (코어가 SSH 문맥)")
	out, rc := f.run("--update", "w1")
	if rc != 1 || !strings.Contains(out, "w1\t실패\t점검 키체인(hc 문맥) 실패") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	// 줄이 없거나 모름도 통과가 아니다
	f.set("w1", "ver", "0.1.0")
	f.set("w1", "check", "모름\t키체인(hc 문맥)\trc 시간 초과")
	out, rc = f.run("--update", "w1")
	if rc != 1 || !strings.Contains(out, "코어 문맥 줄 없음") || !strings.Contains(out, "키체인(hc 문맥) 모름") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
}

func TestUpdateVanishedTerminalIsFailure(t *testing.T) { // ④
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a", "t2 lane-b")
	f.set("w1", "vanish", "1")
	out, rc := f.run("--update", "w1")
	if rc != 1 || !strings.Contains(out, "사라지거나 끝난 터미널: lane-a (w1/t1) 없어짐, lane-b (w1/t2) 없어짐") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
}

// 재시작에 죽은 터미널은 hc ls --all에 finished로 남는다 — 「있음」으로 세면 놓친다(감사 atoz9yh 중간, 고치기 전 코드로 rc 0)
func TestUpdateFinishedTerminalIsFailure(t *testing.T) {
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.set("w1", "finish", "1")
	out, rc := f.run("--update", "w1")
	if rc != 1 || !strings.Contains(out, "사라지거나 끝난 터미널: lane-a (w1/t1) finished — 재시작에 죽었는지 확인") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
}

// 다시 붙기 전에 목록을 읽으면 모두 「사라짐」으로 잘못 보인다 — 붙을 때까지 보고, 끝내 안 붙으면 그 까닭으로 실패(감사 atoz9yh 낮음)
func TestUpdateWaitsForReconnect(t *testing.T) {
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.set("w1", "drop", "disconnected")
	f.set("w1", "reconn", "3") // 처음 상태 읽기·업데이트 뒤 첫 읽기에는 끊김, 셋째에 붙음
	out, rc := f.run("--update", "w1")
	if rc != 0 || !strings.Contains(out, "w1\t성공") {
		t.Fatalf("다시 붙었는데 rc=%d\n%s", rc, out)
	}
	t.Setenv("HC_CORE_RELAUNCH_POLL", "1")
	f.set("w1", "ver", "0.1.0")
	out, rc = f.run("--update", "--timeout", "2", "w1") // 끝내 안 붙음 — 손잡이 --timeout으로 짧게
	if rc != 1 || !strings.Contains(out, "w1\t실패\t업데이트 뒤 다시 연결되지 않음(disconnected)") || strings.Contains(out, "사라지거나") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
}

func TestUpdateLinuxUpdateOnly(t *testing.T) { // ⑤
	f := newFake(t)
	f.mach("s1", "0.1.0", "Linux", "t1 job")
	out, rc := f.run("--update", "s1")
	l := f.log()
	if rc != 0 || !strings.Contains(out, "s1\t성공\t0.1.0 → 0.2.0 (리눅스 — 업데이트만)") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	if !strings.Contains(l, "hc machine update s1") || strings.Contains(l, "ssh ") || strings.Contains(l, "worker-mac-check") {
		t.Fatalf("리눅스에 다시 띄우기·점검을 함:\n%s", l)
	}
}

func TestUpdateTimeoutIsFailure(t *testing.T) { // ⑥ 손잡이 --timeout 1로 짧게(가짜 업데이트는 30초 멈춤 — 상한이 무리째 거둔다)
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.mach("w2", "0.1.0", "Darwin", "t1 lane-b")
	f.set("w1", "slow", "1")
	out, rc := f.run("--update", "--timeout", "1", "w1", "w2")
	if rc != 1 || !strings.Contains(out, "w1\t실패\thc machine update 전체 상한(1초) 넘김") || !strings.Contains(out, "w2\t실패\t전체 상한(1초) 넘김 — 손대지 않음") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	if strings.Contains(f.log(), "ssh ") {
		t.Fatalf("상한을 넘겼는데 다시 띄움:\n%s", f.log())
	}
}

func TestUpdateDryRun(t *testing.T) { // ⑦
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.mach("w2", "0.2.0", "Darwin")
	f.mach("s1", "0.1.0", "Linux")
	out, rc := f.run("--all", "--dry-run")
	if rc != 0 || !strings.Contains(out, "w1\t할 일\thc machine update w1 (0.1.0 → 0.2.0) · 다시 띄우기") ||
		!strings.Contains(out, "s1\t할 일\thc machine update s1 (0.1.0 → 0.2.0)\n") || !strings.Contains(out, "w2\t같음") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	if l := f.log(); strings.Contains(l, "machine update") || strings.Contains(l, "ssh ") {
		t.Fatalf("dry-run인데 손댐:\n%s", l)
	}
}

func TestUpdateBusyKnob(t *testing.T) {
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin", "t1 lane-a")
	f.set("w1", "spec", "t9 spec-abc")
	out, rc := f.run("--update", "w1") // 기본 skip — 손대지 않고 3
	if rc != 3 || !strings.Contains(out, "w1\t건너뜀\t병합기 검증 중(spec-abc)") || !strings.Contains(out, "검증 중: spec-abc") {
		t.Fatalf("skip rc=%d\n%s", rc, out)
	}
	if strings.Contains(f.log(), "machine update") {
		t.Fatalf("건너뛸 기계를 업데이트:\n%s", f.log())
	}
	f.set("w1", "specn", "2") // wait — 상태·첫 확인에 보이고 그다음 사라지면 한다
	out, rc = f.run("--update", "--busy", "wait", "w1")
	if rc != 0 || !strings.Contains(out, "기다림: 검증 중 spec-abc") || !strings.Contains(out, "w1\t성공") {
		t.Fatalf("wait rc=%d\n%s", rc, out)
	}
	f.set("w1", "ver", "0.1.0")
	f.set("w1", "spec", "t9 spec-abc")
	out, rc = f.run("--update", "--busy", "go", "w1") // go — 그대로 한다
	if rc != 0 || !strings.Contains(out, "w1\t성공") {
		t.Fatalf("go rc=%d\n%s", rc, out)
	}
}

func TestUpdateUnknownIsFailure(t *testing.T) {
	f := newFake(t)
	f.mach("w1", "0.1.0", "Darwin")
	f.set("w1", "state", "disconnected")
	f.mach("w2", "0.1.0", "")
	out, rc := f.run("--update", "w1", "w2", "nope")
	if rc != 1 || !strings.Contains(out, "w1\t실패\t연결 안 됨(disconnected)") || !strings.Contains(out, "w2\t실패\tOS를 모른다") ||
		!strings.Contains(out, "nope\t실패\thc 기계 목록에 없음") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	if strings.Contains(f.log(), "machine update") {
		t.Fatalf("모르는데 손댐:\n%s", f.log())
	}
}

// 판 맞추기 없이 부르는 예전 꼴도 점검 줄을 판독한다(고치기 전: 키체인 실패여도 종료 0)
func TestRelaunchCheckFailIsFailure(t *testing.T) {
	f := newFake(t)
	f.mach("w1", "0.2.0", "Darwin")
	f.set("w1", "check", "실패\t코어 문맥(Aqua)\tserve asid 1 ≠ GUI 2\n실패\t키체인(hc 문맥)\tUser interaction is not allowed")
	out, rc := f.run("w1")
	if rc != 1 || !strings.Contains(out, "✗ w1: 점검 코어 문맥 실패 · 키체인(hc 문맥) 실패") || !strings.Contains(out, "실패\t키체인(hc 문맥)\t") {
		t.Fatalf("rc=%d\n%s", rc, out)
	}
	os.Remove(filepath.Join(f.dir, "m", "w1", "check")) // 지우면 기본(통과)
	if out, rc := f.run("w1"); rc != 0 {
		t.Fatalf("통과인데 rc=%d\n%s", rc, out)
	}
}

func TestParse(t *testing.T) {
	for _, a := range [][]string{{}, {"-x", "w1"}, {"w1", "-oProxyCommand=x"}, {"--all", "w1"}, {"--dry-run", "w1"},
		{"--update", "--busy", "maybe", "w1"}, {"--update", "--timeout", "0", "w1"}, {"--update"}, {"--busy", "go", "w1"}} {
		if _, ok := parse(a); ok {
			t.Errorf("%q 를 받음", a)
		}
	}
	if o, ok := parse([]string{"--all", "--busy", "wait", "--timeout", "60"}); !ok || !o.update || o.busy != "wait" || o.timeout != 60 {
		t.Errorf("%+v %v", o, ok)
	}
}
