//go:build !windows

package proc

// gofix4-proc(Go 전환 재감사 바퀴 3): 남은 고아 기록이 바퀴를 끝없이 75로 쉬게 하지 않는다 · 띄우지 않음은 모름이다.
// 줄마다 지적 ID를 단다.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// putRecLine: 기록 폴더 d에 무리 pg의 기록 한 줄(부모 1999999 — 없는 pid라 「죽은 부모」). tail은 넷째 칸 뒤에 붙일 칸들
func putRecLine(t *testing.T, d string, pg int, st string, tail ...string) string {
	t.Helper()
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	p := fmt.Sprintf("%s/%d", d, pg)
	f := append([]string{fmt.Sprint(pg), st, "1999999", "x"}, tail...)
	if err := os.WriteFile(p, []byte(strings.Join(f, "\t")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// base/R-3M03 · ops/R-3M01 · merger/R-3M01(나): keep 기록의 무리 번호를 다른 무리(우두머리 시작 시각이 다름)가 쓰면 옛 무리는 이미 없다 —
// 기록을 지우고 세지 않는다(옛 판: groupGone만 봐 Orphans가 내내 1, 손으로 지울 때까지 75)
func TestGofix4KeepReusedGroup(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	c, bg := startGroup(t, "wait")
	pg := c.Process.Pid
	defer func() { syscall.Kill(-pg, syscall.SIGKILL); c.Wait() }()
	p := putRecLine(t, d, pg, "Mon Jan  1 00:00:00 2001", "keep")
	var got []int
	for i := 0; i < 3; i++ {
		got = append(got, Orphans())
	}
	if fmt.Sprint(got) != "[0 0 0]" || exists(p) {
		t.Fatalf("번호가 다시 쓰인 keep 기록: Orphans 세 번=%v 기록 남음=%v, 기대 [0 0 0]·지움", got, exists(p))
	}
	if !Alive(bg) || !Alive(pg) {
		t.Fatal("번호를 다시 쓴 남의 무리에 신호를 보냈다")
	}
	// 대조: 시작 시각이 같으면 그 일이 아직 돈다 — 세고 기록을 둔다
	st, err := StartTime(pg)
	if err != nil {
		t.Fatal(err)
	}
	p = putRecLine(t, d, pg, st, "keep")
	if n := Orphans(); n != 1 || !exists(p) || !Alive(bg) {
		t.Fatalf("살아 있는 keep 일은 세고 기록을 둬야: Orphans=%d 기록=%v", n, exists(p))
	}
}

// merger/R-3M01(나): 부팅 전에 쓰인 기록(재부팅으로 남음)은 그 무리가 이미 없다 — 우두머리가 끝나 소유를 못 가려도 기록을 지운다
func TestGofix4StaleBootRecord(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	pg1, st1, bg1 := leaderGone(t)
	pg2, st2, bg2 := leaderGone(t)
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	var ps []string
	for _, r := range []struct {
		pg   int
		st   string
		tail []string
	}{{pg1, st1, nil}, {pg2, st2, []string{"keep"}}} {
		p := putRecLine(t, d, r.pg, r.st, r.tail...)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		ps = append(ps, p)
	}
	if n := Orphans(); n != 0 || exists(ps[0]) || exists(ps[1]) {
		t.Fatalf("부팅 전 기록: Orphans=%d 기록 남음=%v·%v, 기대 0·지움", n, exists(ps[0]), exists(ps[1]))
	}
	if !Alive(bg1) || !Alive(bg2) {
		t.Fatal("부팅 전 기록의 번호를 쓴 무리에 신호를 보냈다")
	}
	// 대조: 지금 쓰인 같은 꼴의 기록은 그대로 센다(소유 모름 = 미룸)
	p := putRecLine(t, d, pg1, st1)
	if n := Orphans(); n != 1 || !exists(p) {
		t.Fatalf("부팅 뒤 기록은 세야: Orphans=%d 기록=%v", n, exists(p))
	}
}

// fill/R-3M02 · merger/R-3M01: 기록 폴더는 도구가 함께 쓴다 — 다른 도구가 남긴 무리는 이 도구의 바퀴를 막지 않는다(그 도구의 바퀴가 센다).
// 옛 판: hc-fill run이 herdr-sync·병합기가 남긴 무리가 사라질 때까지 매 바퀴 75, 사건 없음
func TestGofix4OtherToolRecord(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	old := Who
	defer func() { Who = old }()
	pg, st, bg := leaderGone(t)
	p := putRecLine(t, d, pg, st, "-", "herdr-sync")
	Who = "hc-fill"
	if n := Orphans(); n != 0 || !exists(p) || !Alive(bg) {
		t.Fatalf("다른 도구의 기록: Orphans=%d 기록=%v, 기대 0·그대로(신호 없음)", n, exists(p))
	}
	Who = "herdr-sync"
	if n := Orphans(); n != 1 {
		t.Fatalf("제 도구의 기록은 세야: %d", n)
	}
	// 도구 이름이 없는 옛 꼴 기록은 누구의 것인지 모른다 — 모두가 센다
	Who = "hc-fill"
	os.Remove(p)
	putRecLine(t, d, pg, st)
	if n := Orphans(); n != 1 {
		t.Fatalf("옛 꼴 기록은 모두가 세야: %d", n)
	}
}

// record는 도구 이름 칸을 적는다 — 다른 도구의 바퀴가 가려 읽게
func TestGofix4RecordWritesWho(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	old := Who
	defer func() { Who = old }()
	Who = "hc-lane merger"
	for _, keep := range []bool{false, true} {
		if err := record(4242, "S", "P", keep); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(d + "/4242")
		f := strings.Split(strings.TrimRight(string(b), "\n"), "\t")
		mark := "-"
		if keep {
			mark = keepMark
		}
		if len(f) != 6 || f[4] != mark || f[5] != Who || !recShape(f) {
			t.Fatalf("기록 꼴: %q", b)
		}
	}
}

// merger/R-3M02: 띄우지 않음(고아 기록을 못 씀 — 명령은 한 줄도 안 돎)은 모름(TimeoutCode)이다. 옛 판: 127 = 「명령 실패」로 읽혀
// 병합기가 멀쩡한 제출을 착지 실패로 반송하고, 추측 일꾼이 spec-fail을 올렸다
func TestGofix4RefusedIsUnknown(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(d+"/file", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PROC_DIR", d+"/file/rec") // 파일 아래라 기록 폴더를 못 만든다
	for _, max := range []time.Duration{0, time.Minute} {
		rc := WaitMax(exec.Command("sh", "-c", "touch \"$0\"", d+"/ran"), "", max)
		if rc != TimeoutCode || exists(d+"/ran") {
			t.Fatalf("상한 %v: 띄우지 않음 rc=%d 돎=%v, 기대 %d·안 돎", max, rc, exists(d+"/ran"), TimeoutCode)
		}
		rc, ran := WaitFor(exec.Command("sh", "-c", "touch \"$0\"", d+"/ran"), "", max, true)
		if rc != TimeoutCode || ran || exists(d+"/ran") {
			t.Fatalf("WaitFor 상한 %v: rc=%d ran=%v", max, rc, ran)
		}
	}
	// 대조: 띄울 수 있으면 ran 참, 종료 코드 그대로
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	if rc, ran := WaitFor(exec.Command("sh", "-c", "exit 3"), "", time.Minute, true); rc != 3 || !ran {
		t.Fatalf("띄운 명령: rc=%d ran=%v", rc, ran)
	}
}

// merger/R-3M07: 상한이 있어도 keep으로 띄운 일(착지)은 이 실행이 죽어도 다음 실행이 거두지 않고 센다 — 상한은 살아 있는 부모가 건다
func TestGofix4WaitForKeepRecord(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	c := exec.Command("sh", "-c", "cat \"$0\"/rec/* >\"$0\"/seen", d)
	if rc, _ := WaitFor(c, "", time.Minute, true); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	b, _ := os.ReadFile(d + "/seen")
	if f := strings.Split(strings.TrimRight(string(b), "\n"), "\t"); len(f) != 6 || f[4] != keepMark {
		t.Fatalf("상한 있는 keep 기록: %q", b)
	}
	// 상한을 넘기면 무리째 거두고 모름
	t0 := time.Now()
	rc, ran := WaitFor(exec.Command("sh", "-c", "sleep 60 & wait"), "", 300*time.Millisecond, true)
	if rc != TimeoutCode || !ran || time.Since(t0) > 30*time.Second {
		t.Fatalf("keep 상한: rc=%d ran=%v %v", rc, ran, time.Since(t0))
	}
}

// merger/R-3M01(가): ChildCheck는 「돈다 / 우두머리는 끝났는데 무리가 남음 / 모름 / 없음」을 가른다 — 병합기가 남은 무리는 세어 사건으로 넘긴다
func TestGofix4ChildCheck(t *testing.T) {
	rec := t.TempDir() + "/child"
	if s, _ := ChildCheck(rec); s != ChildNone {
		t.Fatalf("기록 없음: %v", s)
	}
	c, _ := startGroup(t, "wait")
	defer func() { syscall.Kill(-c.Process.Pid, syscall.SIGKILL); c.Wait() }()
	st, _ := StartTime(c.Process.Pid)
	os.WriteFile(rec, []byte(fmt.Sprintf("%d\t%s\n", c.Process.Pid, st)), 0o666)
	if s, pid := ChildCheck(rec); s != ChildRunning || pid != c.Process.Pid {
		t.Fatalf("도는 자식: %v %d", s, pid)
	}
	pg, st2, bg := leaderGone(t)
	os.WriteFile(rec, []byte(fmt.Sprintf("%d\t%s\n", pg, st2)), 0o666)
	if s, pid := ChildCheck(rec); s != ChildLeft || pid != pg || !Alive(bg) || !exists(rec) {
		t.Fatalf("우두머리가 끝나고 무리가 남음: %v %d", s, pid)
	}
	os.WriteFile(rec, []byte("반쪽"), 0o666)
	if s, _ := ChildCheck(rec); s != ChildUnknown || !ChildBusy(rec, time.Second) {
		t.Fatalf("꼴이 틀린 기록: %v", s)
	}
}

// Left: 남은 기록의 무리 번호·기록 경로(병합기 막힘 사건 글) — 같은 기록이면 같은 글(쉼 세기가 같은 까닭으로 이어지게)
func TestGofix4Left(t *testing.T) {
	d := t.TempDir() + "/rec"
	t.Setenv("HERDR_PROC_DIR", d)
	pg, st, _ := leaderGone(t)
	p := putRecLine(t, d, pg, st)
	a, b := Left(), Left()
	if len(a) != 1 || !strings.Contains(a[0], fmt.Sprint(pg)) || !strings.Contains(a[0], p) || fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("Left: %q %q", a, b)
	}
}
