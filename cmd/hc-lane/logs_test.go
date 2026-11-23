package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// hc-lane logs(2026-11-23 hc-lane-logs): 가짜 hc의 exec --on <기계>는 수집 글을 그 기계의 가짜 집(<d>/home-<기계>)에서 그대로 돌린다.
// cut 기계는 출력을 앞 200바이트로 자른다(출력 상한에 잘린 것). 이 기계는 HC_LANE_MACHINE=me, 집 <d>/home-me
func logsSetup(t *testing.T, machines ...string) string {
	t.Helper()
	d := t.TempDir()
	js := []string{}
	for _, m := range machines {
		js = append(js, `{"name":"`+m+`","state":"connected"}`)
	}
	clitestPut(t, d+"/bin/hc", `#!/bin/sh
case "$1" in
machine) echo '{"machines":[`+strings.Join(js, ",")+`]}';;
exec) m=$3; shift 6
  if [ "$m" = cut ]; then HOME=`+d+`/home-$m "$@" | head -c 200; else HOME=`+d+`/home-$m exec "$@"; fi;;
*) exit 4;;
esac
`, 0o755)
	t.Setenv("PATH", d+"/bin:"+os.Getenv("PATH"))
	t.Setenv("HOME", d+"/home-me")
	t.Setenv("XDG_STATE_HOME", d+"/st")
	t.Setenv("HC_LANE_MACHINE", "me")
	return d
}

func logsRunT(t *testing.T, a ...string) (string, int) {
	t.Helper()
	var b bytes.Buffer
	rc := logsRun(&b, a)
	return b.String(), rc
}

// 상태 줄(launchd·herdr-sync/bak — 시험 기계의 진짜 launchd가 보인다)은 빼고 그 기계의 기록 줄만
func logsOf(out, m string) []string {
	var r []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, m+tab) && !strings.HasPrefix(l, m+tab+"(상태)"+tab) {
			r = append(r, l)
		}
	}
	return r
}

func logsEq(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s\n--- 받음\n%s\n--- 바람\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func appendTo(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

const lgEv = ".local/state/hc-lane/events.tsv"

// 두 번 부르면 둘째는 새 줄만 · 끝 줄바꿈 없는 반쪽 줄은 다음에 · 다른 기계(--all)도 같은 길 · 상태 줄도 온다
func TestLogsNewLinesOnly(t *testing.T) {
	d := logsSetup(t, "box")
	clitestPut(t, d+"/home-me/"+lgEv, "e1\te\ne2\n", 0o644)
	clitestPut(t, d+"/home-box/Library/Logs/hc-sweep.log", "s1\ns2-반쪽", 0o644)
	out, rc := logsRunT(t, "--all")
	if rc != 0 || !strings.HasPrefix(out, "수집: 2/2대\n") {
		t.Fatalf("첫 줄·종료 %d:\n%s", rc, out)
	}
	logsEq(t, "이 기계 첫 번", logsOf(out, "me"), "me\t"+lgEv+"\te1\te", "me\t"+lgEv+"\te2")
	logsEq(t, "box 첫 번(반쪽 줄은 아직)", logsOf(out, "box"), "box\tLibrary/Logs/hc-sweep.log\ts1")
	if !strings.Contains(out, "box\t(상태)\therdr-sync/bak 없음\n") {
		t.Errorf("상태 줄 없음:\n%s", out)
	}
	appendTo(t, d+"/home-me/"+lgEv, "e3\n")
	appendTo(t, d+"/home-box/Library/Logs/hc-sweep.log", "-끝\ns3\n")
	out, rc = logsRunT(t, "--all")
	if rc != 0 {
		t.Fatalf("둘째 종료 %d:\n%s", rc, out)
	}
	logsEq(t, "이 기계 둘째", logsOf(out, "me"), "me\t"+lgEv+"\te3")
	logsEq(t, "box 둘째", logsOf(out, "box"), "box\tLibrary/Logs/hc-sweep.log\ts2-반쪽-끝", "box\tLibrary/Logs/hc-sweep.log\ts3")
	out, _ = logsRunT(t, "--all")
	logsEq(t, "셋째(새 것 없음)", append(logsOf(out, "me"), logsOf(out, "box")...))
}

// 파일이 줄면(돌림) 처음부터
func TestLogsRotated(t *testing.T) {
	d := logsSetup(t)
	clitestPut(t, d+"/home-me/"+lgEv, "old1\nold2\nold3\n", 0o644)
	logsRunT(t)
	clitestPut(t, d+"/home-me/"+lgEv, "new1\n", 0o644)
	out, rc := logsRunT(t)
	if rc != 0 {
		t.Fatalf("종료 %d", rc)
	}
	logsEq(t, "돌린 뒤", logsOf(out, "me"), "me\t"+lgEv+"\tnew1")
}

// 상한을 넘으면 뒤쪽만 + 잘림 표시 — 파일당, 그리고 기계당(앞 파일이 다 쓰면 뒤 파일은 0바이트 + 잘림)
func TestLogsMaxTail(t *testing.T) {
	d := logsSetup(t)
	clitestPut(t, d+"/home-me/.local/state/herdr-tools/ops.tsv", "aaaa\nbbbb\ncccc\n", 0o644)
	clitestPut(t, d+"/home-me/"+lgEv, "x\n", 0o644)
	out, rc := logsRunT(t, "--max", "10")
	if rc != 0 {
		t.Fatalf("종료 %d:\n%s", rc, out)
	}
	logsEq(t, "뒤쪽만", logsOf(out, "me"),
		"me\t.local/state/herdr-tools/ops.tsv\t(잘림 — 앞 5바이트 건너뜀)",
		"me\t.local/state/herdr-tools/ops.tsv\tbbbb",
		"me\t.local/state/herdr-tools/ops.tsv\tcccc",
		"me\t"+lgEv+"\t(잘림 — 앞 2바이트 건너뜀)")
	appendTo(t, d+"/home-me/"+lgEv, "y\n")
	out, _ = logsRunT(t, "--max", "10")
	logsEq(t, "잘린 것은 다시 안 옴", logsOf(out, "me"), "me\t"+lgEv+"\ty")
}

// 끝 표식이 안 맞는 기계는 통째로 버리고 위치를 안 옮김 · 첫 줄에 보이고 종료 1 · 다른 기계는 그대로 앞당김
func TestLogsCutMachineDropped(t *testing.T) {
	d := logsSetup(t, "cut")
	big := strings.Repeat("z", 300) + "\n"
	clitestPut(t, d+"/home-cut/"+lgEv, big, 0o644)
	clitestPut(t, d+"/home-me/"+lgEv, "m1\n", 0o644)
	out, rc := logsRunT(t, "--all")
	if rc != 1 || !strings.HasPrefix(out, "수집: 1/2대 · 수집 못 한 기계: cut(") {
		t.Fatalf("종료 %d:\n%s", rc, out)
	}
	if len(logsOf(out, "cut")) != 0 {
		t.Errorf("잘린 기계 줄이 나왔다:\n%s", out)
	}
	cur, _ := os.ReadFile(d + "/st/herdr-tools/logs-cursor.tsv")
	if strings.Contains(string(cur), "cut\t") || !strings.Contains(string(cur), "me\t"+lgEv+"\t3\n") {
		t.Errorf("위치 파일:\n%s", cur)
	}
}

type failW struct{}

func (failW) Write([]byte) (int, error) { return 0, errors.New("닫힌 출력") }

// 출력 실패 때 위치 그대로 — 다음에 다시 받는다
func TestLogsOutputFailKeepsCursor(t *testing.T) {
	d := logsSetup(t)
	clitestPut(t, d+"/home-me/"+lgEv, "k1\n", 0o644)
	if rc := logsRun(failW{}, nil); rc != 1 {
		t.Errorf("출력 실패인데 종료 %d", rc)
	}
	if _, err := os.Stat(d + "/st/herdr-tools/logs-cursor.tsv"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("출력 실패인데 위치 파일이 생겼다: %v", err)
	}
	out, _ := logsRunT(t)
	logsEq(t, "다시 받음", logsOf(out, "me"), "me\t"+lgEv+"\tk1")
}

// 병합기 로그: 새 파일에서 실패 줄(--- FAIL·FAIL<TAB>·panic:)만, 한 번 본 파일은 다음에 안 옴
func TestLogsMergerFails(t *testing.T) {
	d := logsSetup(t)
	ml := d + "/home-me/.local/state/hc-lane/merger-logs/"
	clitestPut(t, ml+"lane-a-1.log", "ok\n--- FAIL: TestX (0.1s)\nFAIL\therdr-tools/cmd/x\t0.2s\npanic: boom\nFAIL는 이 줄 아님\n", 0o644)
	out, rc := logsRunT(t)
	if rc != 0 {
		t.Fatalf("종료 %d:\n%s", rc, out)
	}
	logsEq(t, "실패 줄", logsOf(out, "me"),
		"me\tmerger-logs/lane-a-1.log\t--- FAIL: TestX (0.1s)",
		"me\tmerger-logs/lane-a-1.log\tFAIL\therdr-tools/cmd/x\t0.2s",
		"me\tmerger-logs/lane-a-1.log\tpanic: boom")
	out, _ = logsRunT(t)
	logsEq(t, "둘째(새 파일 없음)", logsOf(out, "me"))
}

// 인자: --max는 1 이상 정수, 모르는 인자는 멈춤(die → 종료 1 — exit 대역 없이 인자 검사만 본다)
func TestLogsMaxArg(t *testing.T) {
	for _, v := range []string{"0", "-1", "1k", ""} {
		if _, ok := logsMax(v); ok {
			t.Errorf("--max %q 받음", v)
		}
	}
	if n, ok := logsMax("1048576"); !ok || n != 1048576 {
		t.Errorf("--max 1048576: %d %v", n, ok)
	}
}

// fakeMktemp: bin에 둔 가짜 mktemp는 돌려준 폴더에 빈 파일을 만든다(macOS mktemp는 인자 없이 TMPDIR을 안 따른다)
func fakeMktemp(t *testing.T, bin string) string {
	t.Helper()
	tmp := t.TempDir()
	clitestPut(t, bin+"/mktemp", "#!/bin/sh\nn=0; while [ -e "+tmp+"/t$n ]; do n=$((n+1)); done; : > "+tmp+"/t$n && echo "+tmp+"/t$n\n", 0o755)
	return tmp
}

// 수집 글이 중간에 실패해도(가짜 tr이 1로 끝남 — 첫 구획에서 exit 1) 임시 파일이 남지 않고, 이 기계의 까닭은 「sh 종료 n」
func TestLogsFailNoTempLeft(t *testing.T) {
	d := logsSetup(t)
	clitestPut(t, d+"/bin/tr", "#!/bin/sh\nexit 1\n", 0o755)
	tmp := fakeMktemp(t, d+"/bin")
	out, rc := logsRunT(t)
	if rc != 1 || !strings.HasPrefix(out, "수집: 0/1대 · 수집 못 한 기계: me(sh 종료 1)") {
		t.Errorf("종료 %d:\n%s", rc, out)
	}
	if es, _ := os.ReadDir(tmp); len(es) > 0 {
		t.Errorf("임시 파일이 남았다: %v", es)
	}
}

// 상한에 줄 중간에서 잘려도 조각 줄은 안 나온다 — 건너뛴 구획은 첫 줄바꿈까지 버리고 건너뜀 수에 더한다.
// 기계당 상한을 다 써 0바이트로 잘린 파일(끝이 반쪽 줄)도 다음 실행 첫 줄이 조각이 아니다
func TestLogsTruncNoFragment(t *testing.T) {
	d := logsSetup(t)
	clitestPut(t, d+"/home-me/.local/state/herdr-tools/ops.tsv", "aaaa\nbbbb\ncccc\n", 0o644)
	clitestPut(t, d+"/home-me/"+lgEv, "x\npart", 0o644)
	out, rc := logsRunT(t, "--max", "8")
	if rc != 0 {
		t.Fatalf("종료 %d:\n%s", rc, out)
	}
	logsEq(t, "줄 중간 잘림", logsOf(out, "me"),
		"me\t.local/state/herdr-tools/ops.tsv\t(잘림 — 앞 10바이트 건너뜀)",
		"me\t.local/state/herdr-tools/ops.tsv\tcccc",
		"me\t"+lgEv+"\t(잘림 — 앞 6바이트 건너뜀)")
	appendTo(t, d+"/home-me/"+lgEv, "ial\ny\n")
	out, _ = logsRunT(t, "--max", "8")
	logsEq(t, "0바이트로 잘린 뒤", logsOf(out, "me"),
		"me\t"+lgEv+"\t(잘림 — 앞 4바이트 건너뜀)",
		"me\t"+lgEv+"\ty")
}

// 구획 머리의 바이트 수·이름과 끝 표식이 어긋난 출력(손으로 만든 것)은 버린다 — 끝 표식 검사가 없으면 둘 다 받아 버린다
func TestLogsParseBadMarker(t *testing.T) {
	for _, o := range []string{
		"== f 0 0 9\nab\nEND f\nALL 0", // 바이트 수가 끝 표식까지 삼킴
		"== f 0 0 2\nab\nEND g\nALL 0", // 끝 표식 이름이 다름
	} {
		if secs, _, err := logsParse(o); err == nil {
			t.Errorf("받아 버렸다 %q → %+v", o, secs)
		}
	}
	if secs, wm, err := logsParse("== f 0 0 2\nab\nEND f\nALL 7"); err != nil || len(secs) != 1 || secs[0].body != "ab" || wm != 7 {
		t.Errorf("맞는 출력: %+v %d %v", secs, wm, err)
	}
}
