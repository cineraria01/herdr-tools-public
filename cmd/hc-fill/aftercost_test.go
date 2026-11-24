package main

// fill-after-cost(2026-11-24): 대기 줄 23개쯤에서 바퀴의 띄우기(launch)·선행(after) 단계가 각각 바깥 명령 170번쯤을 불렀다 — 선행 판정이
// 줄·선행마다 기본 가지 확인(git remote get-url·symbolic-ref)·제목 이력(git log)·착지 표식(git ls-remote)을 다시 읽었다.
// 시험 틀: 진짜 git(원격은 맨 저장소), 대기 줄 n개가 선행 둘씩, 먼저 띄운 줄 n/2개가 선행 둘씩. 자리는 0이라 띄우지는 않는다

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type acScene struct {
	t      *testing.T
	fr, or string // 판정이 읽는 클론 · 원격(맨 저장소)
	st     int64  // 선행 줄 접수 초
}

func (s *acScene) git(dir string, a ...string) string {
	s.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...).CombinedOutput()
	if err != nil {
		s.t.Fatalf("git %v: %v\n%s", a, err, out)
	}
	return strings.TrimSpace(string(out))
}

// acSetup: 선행 p-0…p-(n-1) — k%3==0은 제목 머리로 들어감, 1은 착지 표식으로 들어감, 2는 아직. 대기 줄 w-k는 선행 p-k·p-(k+1),
// 먼저 띄운 줄 e-k(k<n/2, 기계 here)는 선행 p-k·p-(k+2). capN = 기계 here 자리
func acSetup(t *testing.T, n, capN int) *acScene {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	setData(t)
	t.Setenv("HOME", t.TempDir())
	s := &acScene{t: t, st: time.Now().Unix() - 600}
	d := t.TempDir()
	s.or, s.fr = d+"/origin.git", d+"/fr"
	s.git(d, "init", "-q", "--bare", "-b", "main", s.or)
	s.git(d, "clone", "-q", s.or, s.fr)
	s.git(s.fr, "commit", "-q", "--allow-empty", "-m", "처음")
	var marks []string
	for k := 0; k < n; k++ {
		switch k % 3 {
		case 0:
			s.git(s.fr, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("p-%d: 본 작업", k))
		case 1:
			s.git(s.fr, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("feat(x): 병합기가 넣은 %d", k))
			marks = append(marks, fmt.Sprintf("%s:refs/hc-landed/p-%d/%d", s.git(s.fr, "rev-parse", "HEAD"), k, s.st+5))
		}
	}
	s.git(s.fr, "push", "-q", "origin", "HEAD:main")
	if len(marks) > 0 {
		s.git(s.fr, append([]string{"push", "-q", "origin"}, marks...)...)
	}
	s.git(s.fr, "remote", "set-head", "origin", "main")

	LOCAL, CONN, TERMS, EV, EVBAD, OREC, OSEED = "here", nil, nil, nil, false, nil, false
	OWNF, OWND, SB = DATA+"/own.tsv", DATA+"/own", DATA+"/sb.md"
	MQP, MQBAD, MQS = "", "", nil
	confLines = lines(fmt.Sprintf("local here\nrepo app here %s\nmachine here %d - -\noverlap off\ngc 0\nearly 0", s.fr, capN))
	brief := d + "/b.md"
	if err := os.WriteFile(brief, []byte("# 일\n파일: x.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var q strings.Builder
	row := func(r Row) { q.WriteString(r.Line() + "\n") }
	for k := 0; k < n; k++ {
		row(pad16(fmt.Sprintf("p-%d", k), "app", brief, "-", "-", "up", "here", "here/p-"+fmt.Sprint(k), "-", "t", "-", "-", "normal", "-", "-", "-").
			Set(17, fmt.Sprintf("r%d-1-%d", s.st, k)))
	}
	for k := 0; k < n; k++ {
		row(pad16(fmt.Sprintf("w-%d", k), "app", brief, fmt.Sprintf("p-%d,p-%d", k, (k+1)%n), "-", "wait", "-", "-", "-", "t", "-", "-", "normal", "-", "-", "-"))
	}
	for k := 0; k < n/2; k++ {
		row(pad16(fmt.Sprintf("e-%d", k), "app", brief, fmt.Sprintf("p-%d,p-%d", k, (k+2)%n), "-", "up", "here", "here/e-"+fmt.Sprint(k), "-", "t", "-", "-", "normal", "early", "-", "-"))
	}
	if err := os.WriteFile(Q, []byte(q.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

// acTick: 바퀴의 띄우기~선행 단계를 한 번 돌려 단계별 바깥 명령 수와 찍은 줄
func acTick(t *testing.T) (st map[string]int, calls map[string]int, out string) {
	t.Helper()
	oe := exitHooks
	defer func() { exitHooks = oe; tk.on = false }()
	ROWS = qRows()
	NOW, NOWL, OFF, FETCHED, FETCHBAD = nil, nil, "", "", ""
	tickReset()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	tickStart()
	launchRows()
	tickMark("끝")
	os.Stdout = old
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	tk.mu.Lock()
	defer tk.mu.Unlock()
	st = map[string]int{}
	for _, x := range tk.stages {
		st[x.name] += x.calls
	}
	calls = map[string]int{}
	for k, v := range tk.calls {
		calls[k] = v
	}
	return st, calls, string(b)
}

// ①② 선행 판정의 바깥 명령 수는 대기 줄 수에 비례하지 않는다 — 줄 30(선행 60+30)과 60(선행 120+60)이 같다.
// 고치기 전: 줄 30에서 launch 182번·after 181번(줄·선행마다 git remote get-url·symbolic-ref·log·ls-remote)
func TestAfterCostFlat(t *testing.T) {
	var got [2]int
	for i, n := range []int{30, 60} {
		acSetup(t, n, 0)
		st, calls, _ := acTick(t)
		t.Logf("줄 %d: launch %d · early %d · after %d · 명령 %v", n, st["launch"], st["early"], st["after"], calls)
		got[i] = st["launch"] + st["early"] + st["after"]
		// 프로젝트 하나·기계 하나: 기본 가지(2)·fetch(1)·제목 이력(1)·표식 목록(1)·표식 조상 확인(표식 든 선행 수와 무관하게 sha·가지마다 1)·after.tsv 쓰기(1)
		if got[i] > 10 {
			t.Errorf("줄 %d: 바깥 명령 %d번(launch %d · early %d · after %d), 바람 10 이하", n, got[i], st["launch"], st["early"], st["after"])
		}
	}
	if got[0] != got[1] {
		t.Errorf("줄 30 → 60에 바깥 명령 %d → %d번, 바람 같음", got[0], got[1])
	}
}

// acLaunched: 찍은 줄에서 띄운 레인 차례(dry-run 「띄울」·「먼저 띄울」)
func acLaunched(out string) []string {
	var o []string
	for _, l := range lines(out) {
		if i := strings.Index(l, "띄울 "); i >= 0 && strings.HasPrefix(l, "[dry-run] ") {
			o = append(o, strings.TrimSpace(strings.TrimSpace(l[len("[dry-run] "):i])+" "+strings.Fields(l[i+len("띄울 "):])[0]))
		}
	}
	return o
}

// ⑤ 줄이 많은 장면에서 뜨는 차례 — 선행이 다 들어간 줄(w-k, k%3==0)이 대기열 차례대로 먼저, 남은 자리는 선행만 남은 줄이 먼저 띄움으로.
// 고치기 전 코드에서도 같은 차례다(판정 결과는 전후가 같다)
func TestAfterCostLaunchOrder(t *testing.T) {
	acSetup(t, 30, 13)
	confLines = lines(strings.Replace(strings.Join(confLines, "\n"), "\nearly 0", "", 1)) // 먼저 띄움 켬
	od, op := dry, P
	defer func() { dry, P = od, op }()
	dry, P = true, "[dry-run] "
	_, _, out := acTick(t)
	got := strings.Join(acLaunched(out), " | ")
	want := "w-0 | w-3 | w-6 | w-9 | w-12 | w-15 | w-18 | w-21 | w-24 | w-27 | 먼저 w-1 | 먼저 w-2 | 먼저 w-4"
	if got != want {
		t.Fatalf("뜬 차례\n%s\n바람\n%s\n%s", got, want, out)
	}
	for _, w := range []string{"선행 w-1 ← p-1: 들어감 (", "선행 w-1 ← p-2: 아직 (origin/main)", "선행 w-0 ← p-0: 들어감 ("} {
		if !strings.Contains(out, w) {
			t.Errorf("판정 줄 없음: %s", w)
		}
	}
}

// ③ 선행이 바퀴 중간에 들어가도: 띄우기 단계가 fetch로 새로 받으면 그 바퀴에서 들어감으로 보고 뜬다(앞서 읽은 「아직」을 들고 가지 않는다).
// fetch 없이 같은 바퀴에서 다시 물으면 한 번 읽은 것을 쓴다(명령 없음)
func TestAfterCostLandsMidTick(t *testing.T) {
	s := acSetup(t, 3, 0)
	od := dry
	defer func() { dry = od }()
	dry = false
	FETCHED, FETCHBAD = "", ""
	tickReset()
	if r := afterOK("w-1", "app", "p-1,p-2"); r != 1 {
		t.Fatalf("p-2 안 들어감: %d, 바람 1(아직)", r)
	}
	// 다른 클론이 p-2를 넣는다 — 이 클론의 origin/main은 fetch 전까지 옛것
	o2 := t.TempDir() + "/o2"
	s.git(".", "clone", "-q", s.or, o2)
	s.git(o2, "commit", "-q", "--allow-empty", "-m", "p-2: 본 작업")
	s.git(o2, "push", "-q", "origin", "HEAD:main")
	oe := exitHooks
	defer func() { exitHooks = oe; tk.on = false }()
	tickStart()
	if r := afterOK("w-1", "app", "p-1,p-2"); r != 1 {
		t.Fatalf("fetch 전 같은 바퀴: %d, 바람 1", r)
	}
	tk.mu.Lock()
	n := len(tk.calls)
	tk.mu.Unlock()
	if n != 0 {
		t.Fatalf("같은 바퀴 다시 물음에 바깥 명령 %v, 바람 없음", tk.calls)
	}
	FETCHED = "" // 띄우기 단계 앞(run.go)처럼 fetch를 다시
	if r := afterOK("w-1", "app", "p-1,p-2"); r != 0 {
		t.Fatalf("fetch 뒤: %d, 바람 0(들어감)", r)
	}
}

// ④ 읽기 실패한 바퀴: 이력(git log)·표식(ls-remote)을 못 읽으면 그 선행은 모름 — 들어감·아직으로 굳히지 않고, 이 바퀴 안에서는 한 번만
// 읽는다. 다음 바퀴(새로 읽기)는 제대로 본다
func TestAfterCostReadFailNotKept(t *testing.T) {
	acSetup(t, 30, 0)
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git 없음")
	}
	bin, flag := t.TempDir(), t.TempDir()+"/fail"
	sc := "#!/bin/sh\nif [ -e " + flag + " ]; then for a; do case $a in log|ls-remote) exit 128;; esac; done; fi\nexec " + real + " \"$@\"\n"
	if err := os.WriteFile(bin+"/git", []byte(sc), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, calls, out := acTick(t)
	if strings.Contains(out, ": 들어감 (") || strings.Contains(out, ": 아직 (") {
		t.Errorf("읽기 실패 바퀴에 들어감·아직 판정:\n%s", out)
	}
	if !strings.Contains(out, "선행 w-0 ← p-0: 판정 못 함(커밋·착지 표식 조회 실패)") {
		t.Errorf("모름 줄 없음:\n%s", out)
	}
	if calls["git log"] != 1 {
		t.Errorf("실패한 이력 읽기 %d번, 바람 바퀴에 1번", calls["git log"])
	}
	at, _ := readFile(os.Getenv("HOME") + "/.local/share/hc-fill/after.tsv")
	if strings.Contains(at, "들어감") || !strings.Contains(at, "모름(착지 조회 실패)") {
		t.Errorf("먼저 띄운 줄 선행 상태: %q, 바람 모름만", at)
	}
	if lm, _ := readFile(DATA + "/after-landed.tsv"); lm != "" {
		t.Errorf("확인한 착지 표식을 남김: %q", lm)
	}
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	_, _, out = acTick(t)
	if !strings.Contains(out, "선행 w-0 ← p-0: 들어감 (") || !strings.Contains(out, "선행 w-1 ← p-1: 들어감 (") || !strings.Contains(out, "선행 w-1 ← p-2: 아직 (") {
		t.Errorf("다음 바퀴 판정:\n%s", out)
	}
}
