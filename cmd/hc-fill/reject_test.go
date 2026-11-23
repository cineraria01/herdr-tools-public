package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fill-reject-hold 시험 차림: 기계 둘(here 로컬·far 원격, 차례 here → far), 대기열에 레인 줄 하나(줄 ID r1), 가짜 hc-lane — 띄우기는
// 기계마다 HC_T_RC_<기계>로 끝나고(기본 1) 인자를 calls에, report(사건)는 reports에 적는다. 가짜 hc는 원격 run 지시문 자리만 답한다
type rejT struct {
	t    *testing.T
	d    string
	rows func() []Row
}

func rejSetup(t *testing.T) *rejT {
	DATA = t.TempDir()
	d := DATA
	HOME = d
	Q, OWNF, OWND = d+"/queue.tsv", d+"/own.tsv", d+"/own"
	LOCAL, dry, P, CONN, AGUNK, SEATS, NOW, NOWL, ROWS, TERMS, OREC = "here", false, "", []string{"far"}, nil, nil, nil, nil, nil, nil, nil
	AK, AUD, NEED, RESUME, EARLYF, COORD, TASK, LAUNCH = "-", "", "", false, "", "", "{run}", "agent {task}"
	confLines = []string{"machine here 5 -", "machine far 5 -", "repo p here " + d + "/r", "repo p far /far/r"}
	RID, proj, stuck, SB = "r1", "p", "-", d+"/brief.md"
	rejW(t, d+"/hc-lane", `#!/bin/sh
case "$1" in
report) printf '%s\n' "$*" >>"$HC_T/reports"; exit 0 ;;
coord) echo t0; exit 0 ;;
--print-steps) echo steps; exit 0 ;;
esac
m=here; [ "$1" = --on ] && m=$2
echo "$m" >>"$HC_T/calls"
eval "rc=\${HC_T_RC_$m:-1}"
[ "$rc" = 11 ] && echo "hc-lane: 원격 브랜치 origin/l1 가 이미 있다. 다른 이름은 --branch <이름>" >&2
[ "$rc" = 1 ] && echo "hc-lane: git fetch 실패" >&2
exit "$rc"
`)
	rejW(t, d+"/hc", "#!/bin/sh\ncat >/dev/null; echo /far/runs/l1-run.md\n")
	rejW(t, d+"/brief.md", "# 일\n본문\n")
	rejW(t, Q, strings.Join([]string{"l1", "p", d + "/brief.md", "-", "-", "wait", "-", "-", "-", "t", "-", "t9", "normal", "-", "-", "-", "r1"}, "\t")+"\n")
	for _, f := range []string{"hc-lane", "hc"} {
		if err := os.Chmod(d+"/"+f, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HC_T", d)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	return &rejT{t: t, d: d}
}

func rejW(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (x *rejT) read(f string) string {
	b, _ := os.ReadFile(x.d + "/" + f)
	return string(b)
}

func (x *rejT) place() {
	NOW, OFF = nil, ""
	place("l1", "p", x.d+"/brief.md", "-", "-", "t9", "")
}

// ①⑤ 확정 거절(종료 11): 한 번만 시도(다른 기계로 넘기지 않음), 막힘 사건 1번, 다음 바퀴에 다시 안 띄움
func TestRejectHolds(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	x.place()
	if got := x.read("calls"); got != "here\n" {
		t.Fatalf("시도 %q, 바람 here 한 번(far로 넘기지 않음)", got)
	}
	if !bad || placed {
		t.Fatalf("bad %v placed %v", bad, placed)
	}
	if w := rejWhy("l1", "r1"); !strings.Contains(w, "원격 브랜치 origin/l1 가 이미 있다") || !strings.Contains(w, "@here") {
		t.Fatalf("까닭 %q", w)
	}
	for i := 0; i < 3; i++ {
		x.place()
	}
	if got := x.read("calls"); got != "here\n" {
		t.Fatalf("멈춘 뒤에도 띄움 %q", got)
	}
	rep := x.read("reports")
	if strings.Count(rep, "report 막힘") != 1 || !strings.Contains(rep, "l1 띄우기 거절 — 원격 브랜치") || !strings.Contains(rep, "--to t9") || !strings.Contains(rep, "hc-fill retry l1") {
		t.Fatalf("사건 %q, 바람 주인에게 한 번", rep)
	}
}

// 사건 넘기기가 실패하면 다음 바퀴에 다시(같은 키)
func TestRejectTellRetriesOnReportFail(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	orig := x.read("hc-lane")
	rejW(t, x.d+"/hc-lane", strings.Replace(orig, "report) printf '%s\\n' \"$*\" >>\"$HC_T/reports\"; exit 0", "report) printf '%s\\n' \"$*\" >>\"$HC_T/reports\"; exit 1", 1))
	x.place()
	if r := rejRow("l1", "r1", "reject"); r == nil || r.F(6) != "-" {
		t.Fatalf("못 알린 기록 %q", r)
	}
	rejW(t, x.d+"/hc-lane", orig)
	x.place()
	r := rejRow("l1", "r1", "reject")
	if r == nil || !strings.HasPrefix(r.F(6), "hf:l1:reject:r1:") {
		t.Fatalf("다시 알린 키 %q", r)
	}
	if n := strings.Count(x.read("reports"), "report 막힘"); n != 2 {
		t.Fatalf("report %d번, 바람 2(실패 1 + 다시 1)", n)
	}
	if got := x.read("calls"); got != "here\n" {
		t.Fatalf("시도 %q", got)
	}
}

// ② retry: 기록을 지워 다음 바퀴에 다시 시도. edit도 푼다(rejDrop)
func TestRejectRetry(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	x.place()
	if rejDrop("l1") != 1 {
		t.Fatal("retry가 기록을 못 지움")
	}
	t.Setenv("HC_T_RC_here", "1")
	x.place()
	if got := x.read("calls"); got != "here\nhere\nfar\n" {
		t.Fatalf("retry 뒤 시도 %q, 바람 here 다시(그리고 일시 실패라 far)", got)
	}
	if rejWhy("l1", "r1") != "" {
		t.Fatal("retry 뒤에도 멈춤")
	}
}

// 명령 수준: retry(대기열에 없으면 종료 2, 멈춘 줄을 풂)·edit(고치면 함께 풂) — 감사 낮음-3
func TestRejectCmdRetryEdit(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	if rc := withQ(func() { retryCmd([]string{"nope"}) }); rc != 2 {
		t.Fatalf("없는 레인 retry 종료 %d, 바람 2", rc)
	}
	x.place()
	if rc := withQ(func() { retryCmd([]string{"l1"}) }); rc != 0 || rejWhy("l1", "r1") != "" {
		t.Fatalf("retry 종료 %d, 남은 거절 %q", rc, rejWhy("l1", "r1"))
	}
	x.place()
	if rejWhy("l1", "r1") == "" {
		t.Fatal("다시 거절됐는데 안 멈춤")
	}
	if rc := withQ(func() { editCmd([]string{"l1", "--priority", "high"}) }); rc != 0 || rejWhy("l1", "r1") != "" {
		t.Fatalf("edit 종료 %d, 남은 거절 %q", rc, rejWhy("l1", "r1"))
	}
	if got := x.read("calls"); got != "here\nhere\n" {
		t.Fatalf("시도 %q", got)
	}
	if rep := x.read("reports"); strings.Count(rep, "report 막힘") != 2 || !strings.Contains(rep, "@here 한 곳의 것") {
		t.Fatalf("사건 %q, 바람 거절마다 한 번(기계 한정 안내 함께)", rep)
	}
}

// rm 뒤 다시 add한 줄(새 줄 ID)은 옛 거절에 묶이지 않는다
func TestRejectNewRowID(t *testing.T) {
	x := rejSetup(t)
	t.Setenv("HC_T_RC_here", "11")
	x.place()
	RID = "r2"
	rejW(t, Q, strings.Replace(x.read("queue.tsv"), "\tr1\n", "\tr2\n", 1))
	t.Setenv("HC_T_RC_here", "1")
	x.place()
	if got := x.read("calls"); got != "here\nhere\nfar\n" {
		t.Fatalf("새 줄 시도 %q", got)
	}
}

// ③⑥ 일시 실패(1)·모르는 종료(42 — 미확정)는 멈추지 않고 다음 바퀴에 다시
func TestRejectTransientRetries(t *testing.T) {
	x := rejSetup(t)
	x.place() // here 1 → far 1
	t.Setenv("HC_T_RC_here", "42")
	x.place() // here 42: 미확정 — 이 바퀴는 그만(지금처럼)
	if got := x.read("calls"); got != "here\nfar\nhere\n" {
		t.Fatalf("시도 %q", got)
	}
	if rejWhy("l1", "r1") != "" {
		t.Fatal("일시 실패·모르는 종료로 멈춤")
	}
	if r := rejRow("l1", "r1", "fail"); r == nil || !strings.Contains(r.F(8), "종료 42") {
		t.Fatalf("일시 실패 기록 %q", r)
	}
	if strings.Contains(x.read("reports"), "report") {
		t.Fatalf("상한 전에 알림 %q", x.read("reports"))
	}
}

// ④ 일시 실패가 상한(launch_fail_warn분)을 넘김 → 사건 1번(되풀이 안 함). 0이면 끔
func TestRejectTransientWarnOnce(t *testing.T) {
	x := rejSetup(t)
	confLines = append(confLines, "launch_fail_warn 5")
	old := strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)
	recent := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	rejW(t, x.d+"/reject.tsv", strings.Join([]string{"r1", "l1", "fail", old, recent, "-", "-", "x"}, "\t")+"\n")
	for i := 0; i < 3; i++ {
		x.place()
	}
	rep := x.read("reports")
	if strings.Count(rep, "report 막힘") != 1 || !strings.Contains(rep, "l1 띄우기가 6분째 실패") || !strings.Contains(rep, "--key hf:l1:fail:r1:"+old) {
		t.Fatalf("사건 %q, 바람 한 번", rep)
	}
	if got := strings.Count(x.read("calls"), "\n"); got != 6 {
		t.Fatalf("시도 %d번, 바람 6(바퀴마다 계속)", got)
	}
	// 성공하면 지운다
	rejOK("l1")
	if rejRow("l1", "r1", "fail") != nil {
		t.Fatal("성공 뒤에도 일시 실패 기록")
	}
}

// 상한 0: 세지도 알리지도 않음 · 마지막 실패 뒤 상한보다 오래 쉬었으면 새로 센다
func TestRejectTransientOffAndReset(t *testing.T) {
	x := rejSetup(t)
	confLines = append(confLines, "launch_fail_warn 0")
	x.place()
	if rejRow("l1", "r1", "fail") != nil || x.read("reports") != "" {
		t.Fatal("끔인데 셈")
	}
	confLines[len(confLines)-1] = "launch_fail_warn 5"
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	rejW(t, x.d+"/reject.tsv", strings.Join([]string{"r1", "l1", "fail", stale, stale, "-", "-", "x"}, "\t")+"\n")
	x.place()
	if r := rejRow("l1", "r1", "fail"); r == nil || r.F(4) == stale || x.read("reports") != "" {
		t.Fatalf("오래 쉰 뒤 새로 세지 않음 %q · %q", r, x.read("reports"))
	}
}
