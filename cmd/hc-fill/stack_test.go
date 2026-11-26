package main

// 쌓아 띄움(stack.go·restack.go, docs/hc-fill/stack.md) — 진짜 git(원격은 맨 저장소)에 선행 제출(lane/<선행>·큐 표식·제출 기록)을 만들고
// 판정·다시 쌓기를 직접 부른다. 레인 쪽 hc-lane(HC_LANE_STACK → 범위 P..HEAD)과 병합기 차례는 tests/merger가 본다

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/gitx"
)

type skScene struct {
	t      *testing.T
	fr, or string // 판정 클론 · 원격
	base   string // 처음 main
	st     int64
	q      []Row
}

func (s *skScene) git(dir string, a ...string) string {
	s.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, a...)...).CombinedOutput()
	if err != nil {
		s.t.Fatalf("git %v: %v\n%s", a, err, out)
	}
	return strings.TrimSpace(string(out))
}

func skSetup(t *testing.T, on bool) *skScene {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	setData(t)
	t.Setenv("HOME", t.TempDir())
	fake := t.TempDir() // 다시 쌓기의 중간 알림(hc-lane report)이 진짜 사건에 닿지 않게
	if err := os.WriteFile(fake+"/hc-lane", []byte("#!/bin/sh\necho \"$*\" >>\""+fake+"/report.log\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fake+":"+os.Getenv("PATH"))
	s := &skScene{t: t, st: time.Now().Unix() - 600}
	d := t.TempDir()
	s.or, s.fr = d+"/origin.git", d+"/fr"
	s.git(d, "init", "-q", "--bare", "-b", "main", s.or)
	s.git(d, "clone", "-q", s.or, s.fr)
	if err := os.WriteFile(s.fr+"/f.txt", []byte("1\n2\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.git(s.fr, "add", "f.txt")
	s.git(s.fr, "commit", "-q", "-m", "처음")
	s.git(s.fr, "push", "-q", "origin", "HEAD:main")
	s.git(s.fr, "remote", "set-head", "origin", "main")
	s.base = s.git(s.fr, "rev-parse", "HEAD")
	LOCAL, CONN, TERMS, EV, EVBAD, OREC, OSEED, THENMAP = "here", nil, nil, nil, false, nil, false, nil
	FETCHED, FETCHBAD, dry = "", "", false
	pcRef = map[string]pcRes{}
	c := fmt.Sprintf("local here\nrepo app here %s\nrepo other here %s\nmachine here 3 - -\n", s.fr, s.fr)
	if on {
		c += "stack app\n"
	}
	confLines = lines(c)
	return s
}

func (s *skScene) row(name, after, state, early string) {
	s.q = append(s.q, pad16(name, "app", "b.md", after, "-", state, "here", "here/"+name, "-", "t", "-", "-", "normal", early, "-", "-").
		Set(qRID, fmt.Sprintf("r%d-1-%d", s.st, len(s.q))))
	if err := os.WriteFile(Q, []byte(rowsText(s.q)), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// commit: from 위에 파일 하나를 고친 커밋(줄 번호 line을 val로)
func (s *skScene) commit(from, subj string, line int, val string) string {
	s.git(s.fr, "checkout", "-q", "--detach", from)
	b, _ := os.ReadFile(s.fr + "/f.txt")
	ls := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for len(ls) < line {
		ls = append(ls, "")
	}
	ls[line-1] = val
	if err := os.WriteFile(s.fr+"/f.txt", []byte(strings.Join(ls, "\n")+"\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
	s.git(s.fr, "commit", "-q", "-am", subj)
	return s.git(s.fr, "rev-parse", "HEAD")
}

func (s *skScene) patch(a, b string) string {
	p, rc := gitx.PatchIDIn(s.fr, io.Discard, a, b)
	if rc != 0 {
		s.t.Fatalf("patch-id %s..%s", a, b)
	}
	return p
}

// submit: 원격에 병합기 제출(lane/<레인>·큐 표식·제출 기록)을 pre(merge-queue·merge-parked) 자리로. extra는 기록 끝 칸
func (s *skScene) submit(pre, lane, sha, fork, extra string, sec int64) string {
	n := fmt.Sprintf("%010d-%s", sec, lane)
	et := s.git(s.fr, "hash-object", "-t", "tree", "-w", "/dev/null")
	msg := fmt.Sprintf("merge-queue lane=%s id=w1-x-%d orig=%s fork=%s base=main host=w1 term=- owner=- job=- created_ms=- priority=보통%s", lane, sec, sha, fork, extra)
	mc := s.git(s.fr, "commit-tree", et, "-p", sha, "-m", msg)
	s.git(s.fr, "push", "-q", "-f", "origin", sha+":refs/heads/lane/"+lane, sha+":refs/"+pre+"/"+n, mc+":refs/"+pre+"-meta/"+n)
	return n
}

func (s *skScene) remote(pat string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(s.git(s.fr, "ls-remote", "origin", pat), "\n") {
		if sh, r, ok := strings.Cut(l, "\t"); ok {
			m[r] = sh
		}
	}
	return m
}

func (s *skScene) pick(name string) bool {
	FETCHED, FETCHBAD = "", ""
	r := qrow(name)
	if r == nil {
		s.t.Fatalf("줄 없음 %s", name)
	}
	if afterOK(name, r.F(qProj), r.F(qAfter)) != 1 {
		s.t.Fatalf("%s 선행이 「아직」이 아님", name)
	}
	return stackPick(r, dash(r.F(qEarly)))
}

// ① 선행이 감사 통과로 제출되면 뒤 레인이 그 커밋 위에서 뜬다(착지를 안 기다림) — 지시문 절·환경·감사 범위
func TestStackPickSubmitted(t *testing.T) {
	s := skSetup(t, true)
	a := s.commit(s.base, "A: 앞 일", 1, "a")
	s.submit("merge-queue", "A", a, s.base, " verdict=aud1 patch="+s.patch(s.base, a), s.st)
	s.row("A", "-", "up", "-")
	s.row("B", "A", "wait", "-")
	if !s.pick("B") || STACK != "A@"+a {
		t.Fatalf("쌓지 않음: STACK=%q", STACK)
	}
	if e := stackEnvs(); len(e) != 1 || e[0] != "HC_LANE_STACK=A@"+a {
		t.Errorf("레인 환경: %v", e)
	}
	sec := stackSec("B", "-", "origin/main")
	for _, w := range []string{"## 쌓아 띄움 — 기준은 선행 A " + a[:8], "범위는 " + a[:8] + "..HEAD", "`git rebase origin/main` 하지 않는다"} {
		if !strings.Contains(sec, w) {
			t.Errorf("절에 %q 없음:\n%s", w, sec)
		}
	}
	if p := stackSec("B", "parked", "origin/main"); !strings.Contains(p, "`git fetch origin && git rebase "+a+"`") {
		t.Errorf("주차 이어 띄움 절: %s", p)
	}
	// ③ 감사 범위: 떠 있음(stacked)으로 기록되면 origin/{base} 자리가 P
	stackSave("B", s.q[1].F(qRID))
	s.q[1] = s.q[1].Set(qState, "up").Set(qEarly, "stacked")
	os.WriteFile(Q, []byte(rowsText(s.q)), 0o644)
	if got := stackRef("B", "origin/main"); got != a {
		t.Errorf("감사 범위 시작 = %q, 바람 P %s", got, a)
	}
	if got := stackRef("A", "origin/main"); got != "origin/main" {
		t.Errorf("쌓지 않은 레인의 감사 범위가 바뀜: %q", got)
	}
	if n := stackNote("B"); n != "쌓아 띄움(선행 A "+a[:8]+" 제출됨)" {
		t.Errorf("ls 덧말: %q", n)
	}
	// 쌓지 않고 다시 띄우면 기록을 지운다
	STACK = ""
	stackSave("B", s.q[1].F(qRID))
	if stackRec("B") != nil {
		t.Error("쌓지 않은 띄움 뒤에도 기록이 남음")
	}
}

// ② 선행이 감사 전(판정 없는 제출)·아직 제출 전이면 지금의 먼저 띄움 그대로 · ⑧ 손잡이 끔이면 git을 보지도 않고 그대로
func TestStackPickNotYet(t *testing.T) {
	s := skSetup(t, true)
	a := s.commit(s.base, "A: 앞 일", 1, "a")
	s.row("A", "-", "up", "-")
	s.row("B", "A", "wait", "-")
	if s.pick("B") || STACK != "" {
		t.Fatal("제출 전 선행 위에 쌓음")
	}
	s.submit("merge-queue", "A", a, s.base, "", s.st) // 판정 없음(감사 전·손 제출)
	if s.pick("B") {
		t.Fatal("감사 통과 판정 없는 제출 위에 쌓음")
	}
	s2 := skSetup(t, false)
	a2 := s2.commit(s2.base, "A: 앞 일", 1, "a")
	s2.submit("merge-queue", "A", a2, s2.base, " verdict=aud1 patch="+s2.patch(s2.base, a2), s2.st)
	s2.row("A", "-", "up", "-")
	s2.row("B", "A", "wait", "-")
	if s2.pick("B") || STACK != "" {
		t.Fatal("손잡이 끔인데 쌓음")
	}
	s2.row("C", "A", "wait", "no-early")
	confLines = append(confLines, "stack app")
	if s2.pick("C") {
		t.Fatal("--no-early 줄을 쌓음(줄마다 끔)")
	}
}

// ⑨ 세 겹 사슬·깊이 상한 · ⑩ 잇지 않은 선행 둘·다른 프로젝트 선행은 안 쌓음
func TestStackPickChainAndShape(t *testing.T) {
	s := skSetup(t, true)
	z := s.commit(s.base, "Z: 맨 앞", 1, "z")
	s.submit("merge-queue", "Z", z, s.base, " verdict=a0 patch="+s.patch(s.base, z), s.st)
	a := s.commit(z, "A: 가운데", 2, "a")
	s.submit("merge-queue", "A", a, z, " verdict=a1 patch="+s.patch(z, a)+" stack=Z@"+z, s.st+1)
	b := s.commit(a, "B: 셋째", 3, "b")
	s.submit("merge-queue", "B", b, a, " verdict=a2 patch="+s.patch(a, b)+" stack=A@"+a, s.st+2)
	s.row("Z", "-", "up", "-")
	s.row("A", "Z", "up", "stacked")
	s.row("B", "A", "up", "stacked")
	s.row("C", "B", "wait", "-")
	s.row("D", "A", "wait", "-")
	if !s.pick("D") || STACK != "A@"+a {
		t.Fatalf("두 겹(D ← A ← Z)을 쌓지 않음: %q", STACK)
	}
	if s.pick("C") {
		t.Fatal("세 겹(C ← B ← A ← Z)이 깊이 상한 2를 넘었는데 쌓음")
	}
	confLines = append(confLines, "stack_depth 3")
	if !s.pick("C") || STACK != "B@"+b {
		t.Fatalf("깊이 3이면 쌓아야 함: %q", STACK)
	}
	s.row("E", "A,B", "wait", "-")
	if s.pick("E") {
		t.Fatal("아직 안 든 선행 둘(이어진 사슬이어도 — 설계는 선행 하나만)에 쌓음")
	}
	s.row("F", "A@other", "wait", "-")
	FETCHED, FETCHBAD = "", ""
	if r := qrow("F"); stackPick(r, "-") {
		t.Fatal("다른 프로젝트 선행에 쌓음")
	}
}

// ⑦ 다시 쌓기 — 선행이 고쳐져 새 커밋 P'로 다시 서면: 깨끗하면 자기 커밋만 옮겨 승인 그대로 새 큐 이름, 충돌이면 반송 다시 띄움 길
func TestStackRestack(t *testing.T) {
	s := skSetup(t, true)
	a := s.commit(s.base, "A: 앞 일", 1, "a")
	b := s.commit(a, "B: 뒤 일", 3, "b")
	pb := s.patch(a, b)
	pn := s.submit("merge-parked", "B", b, a, " verdict=a2 patch="+pb+" stack=A@"+a, s.st)
	s.row("A", "-", "up", "-")
	s.row("B", "A", "up", "stacked")
	stackPut("B", Row{"B", s.q[1].F(qRID), "A", a, s.st2q(), "up", utcnow()})
	stackTick()
	if r := stackRec("B"); r == nil || r.F(skState) != "parked" {
		t.Fatalf("선행이 아직 고치는 중인데 주차 기록이 아님: %v", r)
	}
	if m := s.remote("refs/merge-queue/*"); len(m) != 0 {
		t.Fatalf("선행 전에 다시 넣음: %v", m)
	}
	a2 := s.commit(s.base, "A: 앞 일 고침", 1, "a2")
	s.submit("merge-queue", "A", a2, s.base, " verdict=aud9 patch="+s.patch(s.base, a2), s.st+5)
	stackTick()
	q := s.remote("refs/merge-queue/*-B")
	var n2, b2 string
	for r, sh := range q {
		n2, b2 = strings.TrimPrefix(r, "refs/merge-queue/"), sh
	}
	if b2 == "" || s.remote("refs/merge-parked/" + pn)["refs/merge-parked/"+pn] != "" {
		t.Fatalf("깨끗한 다시 쌓기가 줄에 다시 세우지 않음: 큐 %v", q)
	}
	if s.git(s.fr, "rev-parse", b2+"^") != a2 || s.patch(a2, b2) != pb {
		t.Errorf("옮긴 제출이 P' 위 자기 커밋 하나가 아니거나 patch-id가 다름")
	}
	s.git(s.fr, "fetch", "-q", "origin", "refs/merge-queue-meta/"+n2)
	meta := s.git(s.fr, "log", "-1", "--format=%s", "FETCH_HEAD")
	for _, w := range []string{"orig=" + b2, "fork=" + a2, "verdict=a2", "patch=" + pb, "stack=A@" + a2} {
		if !strings.Contains(meta, w) {
			t.Errorf("다시 쌓은 기록에 %q 없음: %s", w, meta)
		}
	}
	if r := stackRec("B"); r == nil || r.F(skState) != "restacked" || r.F(skP) != a2 {
		t.Errorf("기록: %v", r)
	}

	// 충돌: 선행이 뒤가 고친 줄을 함께 고쳐 다시 섬 → 반송 다시 띄움 대기·주차 참조는 refs/hc-restack으로
	s.git(s.fr, "push", "-q", "origin", ":refs/merge-queue/"+n2, ":refs/merge-queue-meta/"+n2)
	pn = s.submit("merge-parked", "B", b2, a2, " verdict=a2 patch="+pb+" stack=A@"+a2, s.st+6)
	a3 := s.commit(s.base, "A: 또 고침", 3, "충돌")
	s.git(s.fr, "push", "-q", "origin", ":refs/merge-queue/"+fmt.Sprintf("%010d-A", s.st+5), ":refs/merge-queue-meta/"+fmt.Sprintf("%010d-A", s.st+5))
	s.submit("merge-queue", "A", a3, s.base, " verdict=aud10 patch="+s.patch(s.base, a3), s.st+7)
	stackTick()
	if r := qrow("B"); r.F(qState) != "wait" || r.F(qEarly) != "bounced" {
		t.Fatalf("충돌 다시 쌓기가 반송 다시 띄움 대기가 아님: %v", r)
	}
	if s.remote("refs/hc-restack/" + pn)["refs/hc-restack/"+pn] != b2 || len(s.remote("refs/merge-parked/*")) != 0 {
		t.Error("주차 참조를 refs/hc-restack으로 옮기지 않음")
	}
	if sec := bounceSec("B"); !strings.Contains(sec, "## 다시 쌓기") || !strings.Contains(sec, "git fetch origin refs/hc-restack/"+pn) {
		t.Errorf("다시 띄움 절: %s", sec)
	}
}

// 선행이 대기열에서 빠지면(지휘가 rm) 기본 가지 위로 옮겨 다시 넣고 주인 지휘에 중간 한 줄
func TestStackRestackPredGone(t *testing.T) {
	s := skSetup(t, true)
	a := s.commit(s.base, "A: 앞 일", 1, "a")
	b := s.commit(a, "B: 뒤 일", 3, "b")
	pb := s.patch(a, b)
	s.submit("merge-parked", "B", b, a, " verdict=a2 patch="+pb+" stack=A@"+a, s.st)
	s.row("B", "A", "up", "stacked")
	stackPut("B", Row{"B", s.q[0].F(qRID), "A", a, s.st2q(), "up", utcnow()})
	stackTick()
	var b2, n2 string
	for r, sh := range s.remote("refs/merge-queue/*-B") {
		n2, b2 = strings.TrimPrefix(r, "refs/merge-queue/"), sh
	}
	if b2 == "" || s.git(s.fr, "rev-parse", b2+"^") != s.base || s.patch(s.base, b2) != pb {
		t.Fatalf("기본 가지 위로 다시 넣지 않음: %s", b2)
	}
	s.git(s.fr, "fetch", "-q", "origin", "refs/merge-queue-meta/"+n2)
	if meta := s.git(s.fr, "log", "-1", "--format=%s", "FETCH_HEAD"); strings.Contains(meta, "stack=") {
		t.Errorf("기본 가지 위로 옮겼는데 stack 칸이 남음: %s", meta)
	}
	if r := stackRec("B"); r.F(skState) != "rebased-main" || stackRef("B", "origin/main") != "origin/main" {
		t.Errorf("기록: %v", r)
	}
	if lg, _ := os.ReadFile(strings.Split(os.Getenv("PATH"), ":")[0] + "/report.log"); !strings.Contains(string(lg), "report 중간 - 쌓아 띄움 B: 선행 A 이 대기열에서 빠짐") {
		t.Errorf("주인 지휘에 중간 알림 없음: %s", lg)
	}
}

func (s *skScene) st2q() string { return fmt.Sprintf("%010d-A", s.st) }
