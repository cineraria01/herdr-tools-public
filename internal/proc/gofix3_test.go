//go:build !windows

package proc

// gofix3-proc(Go 전환 재감사 아스트라 2바퀴) — 계약 P1~P5의 표 주도 시험. 줄마다 지적 ID를 단다.
// 하위 프로세스 사례(첫 준비·부모 SIGKILL)는 이 시험 바이너리를 다시 띄워 돌린다(prepare 상태가 프로세스마다 하나라).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// putRec: 기록 폴더 d에 무리 pg의 고아 기록(부모 1999999 — 없는 pid라 「죽은 부모」)
func putRec(t *testing.T, d string, pg int, st string) string {
	t.Helper()
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	p := fmt.Sprintf("%s/%d", d, pg)
	if err := os.WriteFile(p, []byte(fmt.Sprintf("%d\t%s\t1999999\tx\n", pg, st)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// leaderGone: 우두머리는 끝났고 무리에 배경 sleep만 남은 무리(번호·우두머리 시작 시각·배경 pid)
func leaderGone(t *testing.T) (int, string, int) {
	t.Helper()
	c, bg := startGroup(t, "exit 0")
	pg := c.Process.Pid
	st, err := StartTime(pg)
	if err != nil {
		t.Fatal(err)
	}
	c.Wait()
	t.Cleanup(func() { syscall.Kill(-pg, syscall.SIGKILL) })
	return pg, st, bg
}

// P1 고아 셈 — fill/R-205 · ops/R-201 · base/R-227 · merger/R-208
func TestGofix3Orphans(t *testing.T) {
	cases := []struct {
		name string
		set  func(t *testing.T, d string) (check func(t *testing.T))
		min  int // Orphans 기대 하한(-1이면 정확히 0)
	}{
		{"우두머리만 끝나고 손자가 삶 — 신호 0·기록 그대로 · fill R-205 · ops R-201 · base R-227", func(t *testing.T, d string) func(*testing.T) {
			pg, st, bg := leaderGone(t)
			p := putRec(t, d, pg, st)
			return func(t *testing.T) {
				if !Alive(bg) {
					t.Fatal("우두머리가 끝난 무리(소유를 입증 못 함)에 신호를 보냈다")
				}
				if _, err := os.Stat(p); err != nil {
					t.Fatal("남은 무리의 기록을 지웠다")
				}
			}
		}, 1},
		{"무리가 다 끝남 — 0·기록 지움 · 대조", func(t *testing.T, d string) func(*testing.T) {
			c := exec.Command("sh", "-c", "exit 0")
			c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := c.Run(); err != nil {
				t.Fatal(err)
			}
			p := putRec(t, d, c.Process.Pid, "Mon Jan  1 00:00:00 2001")
			return func(t *testing.T) {
				if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("끝난 무리의 기록이 남았다: %v", err)
				}
			}
		}, -1},
		{"기록 파일 하나 0o000 · merger R-208", func(t *testing.T, d string) func(*testing.T) {
			pg, st, _ := leaderGone(t)
			p := putRec(t, d, pg, st)
			os.Chmod(p, 0)
			return func(t *testing.T) { os.Chmod(p, 0o600) }
		}, 1},
		{"기록 폴더 0o000 · merger R-208", func(t *testing.T, d string) func(*testing.T) {
			pg, st, _ := leaderGone(t)
			putRec(t, d, pg, st)
			os.Chmod(d, 0)
			return func(t *testing.T) { os.Chmod(d, 0o700) }
		}, 1},
		{"기록 꼴이 틀림", func(t *testing.T, d string) func(*testing.T) {
			os.MkdirAll(d, 0o700)
			os.WriteFile(d+"/4242", []byte("반쪽"), 0o600)
			return func(*testing.T) {}
		}, 1},
		{"기록 폴더 없음 — 0 · 대조", func(t *testing.T, d string) func(*testing.T) { return func(*testing.T) {} }, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir() + "/rec"
			t.Setenv("HERDR_PROC_DIR", d)
			check := c.set(t, d)
			n := Orphans()
			check(t)
			if c.min < 0 && n != 0 {
				t.Fatalf("Orphans = %d, 기대 0", n)
			}
			if c.min > 0 && n < c.min {
				t.Fatalf("Orphans = %d, 기대 ≥ %d(모름 = 미룸)", n, c.min)
			}
		})
	}
}

// slowPS: 시작 시각 조회를 delay만큼 늦추는 가짜 ps
func slowPS(t *testing.T, d, delay string) string {
	t.Helper()
	p := d + "/slowps"
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep "+delay+"\nexec /bin/ps \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// P2 준비도 마감 안 — ops/R-212 · merger/R-214. 새 프로세스(prepare 전)에서
func TestGofix3ColdDeadline(t *testing.T) {
	for _, mode := range []string{"cold", "warm"} {
		t.Run(mode, func(t *testing.T) {
			d := t.TempDir()
			jobGit(t, d)
			rec := d + "/rec"
			// warm: 자기 시작 시각은 이미 읽었고, Reap만 이 무리(부모가 죽은 기록, 우두머리가 삶)의 시작 시각을 늦게 읽는다
			keep, bg := startGroup(t, "wait")
			defer func() { syscall.Kill(-keep.Process.Pid, syscall.SIGKILL); keep.Wait() }()
			kst, _ := StartTime(keep.Process.Pid)
			putRec(t, rec, keep.Process.Pid, kst)
			c := helperCmd("g3"+mode, d)
			c.Env = append(c.Env, "PATH="+d+":"+os.Getenv("PATH"))
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("흉내 도구 실패: %v\n%s", err, out)
			}
			var rc int
			var el time.Duration
			if _, err := fmt.Sscanf(string(out[bytes.LastIndexByte(bytes.TrimRight(out, "\n"), '\n')+1:]), "rc=%d el=%d", &rc, &el); err != nil {
				t.Fatalf("결과 줄 없음:\n%s", out)
			}
			if rc != TimeoutCode {
				t.Fatalf("준비가 마감을 넘기면 TimeoutCode: rc=%d\n%s", rc, out)
			}
			if el > time.Second+Grace {
				t.Fatalf("상한 1초인데 %v (≤ 1s+Grace)\n%s", el, out)
			}
			if _, err := os.Stat(d + "/ran"); err == nil {
				t.Fatal("마감 뒤에 명령을 시작했다")
			}
			if !strings.Contains(string(out), "retry=ok") {
				t.Fatalf("준비가 실패한 뒤 다음 호출이 준비를 다시 하지 않았다(Once가 실패에도 닫힘)\n%s", out)
			}
			if !gone(bg) {
				t.Fatal("다시 한 준비가 고아 무리를 거두지 않았다")
			}
		})
	}
}

// P3 기록 없는 기다림 없음 — ops/R-204 · fill/R-213 · merger/R-209: WaitMax(c,"",0) 자식이 도는 중 부모가 SIGKILL이면 다음 실행이 본다
// 상한 없음(0): 그 일은 끝까지 가게 두고 끝날 때까지 센다(Orphans≥1, 신호 0) · 상한 있음: Run처럼 소유를 확인하고 거둔다
func TestGofix3WaitMaxRecorded(t *testing.T) {
	for _, max := range []string{"0", "60"} {
		t.Run("max="+max, func(t *testing.T) {
			d := t.TempDir()
			t.Setenv("HERDR_PROC_DIR", d+"/rec")
			c := helperCmd("g3waitkill", d)
			c.Env = append(c.Env, "G3_MAX="+max)
			err := c.Run()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("흉내 도구가 SIGKILL로 끝나지 않았다: %v", err)
			}
			lead := waitFile(t, d+"/lead")
			defer syscall.Kill(-lead, syscall.SIGKILL)
			if !Alive(lead) {
				t.Fatal("부모가 죽은 뒤 자식이 살아 있어야 시험이 된다")
			}
			if r := digitRecords(d + "/rec"); len(r) != 1 {
				t.Fatalf("WaitMax(c, \"\", %s)도 고아 기록에 올라야: %v", max, r)
			}
			n := Orphans()
			if max == "60" {
				if n != 0 || !gone(lead) {
					t.Fatalf("상한 있는 자식은 다음 실행이 소유를 확인하고 거둔다: Orphans=%d 살아 있음=%v", n, Alive(lead))
				}
				return
			}
			if n < 1 || !Alive(lead) {
				t.Fatalf("상한 없는 자식은 거두지 않고 센다: Orphans=%d 살아 있음=%v", n, Alive(lead))
			}
			syscall.Kill(-lead, syscall.SIGKILL)
			until(func() bool { return groupGone(lead) })
			if n := Orphans(); n != 0 || len(digitRecords(d+"/rec")) != 0 {
				t.Fatalf("끝난 뒤에는 0·기록 지움: %d %v", n, digitRecords(d+"/rec"))
			}
		})
	}
}

// P4 결과 채택 — merger/R-213: 자식 종료와 출력 끝이 둘 다 원래 마감 안일 때만
func TestGofix3LateOutput(t *testing.T) {
	cases := []struct {
		name, script string
		d            time.Duration
	}{
		{"0 종료 뒤 700ms 출력, 상한 200ms", "(sleep 0.7; echo late) & exit 0", 200 * time.Millisecond},
		{"3 종료 뒤 1.5s 출력, 상한 900ms", "(sleep 1.5; echo late) & exit 3", 900 * time.Millisecond},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HERDR_PROC_DIR", t.TempDir())
			out, err := OutputMax(exec.Command("sh", "-c", c.script), c.d)
			if Code(err) != TimeoutCode || out != nil {
				t.Fatalf("마감 뒤 출력은 버리고 TimeoutCode: rc=%d out=%q err=%v", Code(err), out, err)
			}
		})
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// blockWriter: 첫 쓰기에서 rel이 닫힐 때까지 막힌다 — 쓰기 시작 수를 센다
type blockWriter struct {
	mu  sync.Mutex
	n   int
	rel chan struct{}
}

func (w *blockWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.n++
	w.mu.Unlock()
	<-w.rel
	return len(p), nil
}

func (w *blockWriter) calls() int { w.mu.Lock(); defer w.mu.Unlock(); return w.n }

// P4 쓰개 — merger/R-220 · R-212
func TestGofix3Writers(t *testing.T) {
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	t.Run("쓰개 오류는 성공 아님 (merger/R-220)", func(t *testing.T) {
		c := exec.Command("sh", "-c", "echo hi")
		c.Stdout = errWriter{}
		if err := RunMax(c, 5*time.Second); err == nil {
			t.Fatal("io.ErrClosedPipe 쓰개인데 nil(성공)")
		}
		// io.MultiWriter 안에서도
		c = exec.Command("sh", "-c", "echo hi")
		var b bytes.Buffer
		c.Stdout = io.MultiWriter(&b, errWriter{})
		if err := RunMax(c, 5*time.Second); err == nil {
			t.Fatal("닫힌 MultiWriter인데 nil(성공)")
		}
	})
	t.Run("막힌 쓰개 — 마감+회수 유예 안 반환, 반환 뒤 새 쓰기 0 (merger/R-212)", func(t *testing.T) {
		old := Grace
		Grace = 300 * time.Millisecond
		defer func() { Grace = old }()
		w := &blockWriter{rel: make(chan struct{})}
		defer close(w.rel)
		c := exec.Command("sh", "-c", "echo a; sleep 0.1; echo b; exit 0")
		c.Stdout = w
		t0 := time.Now()
		done := make(chan error, 1)
		go func() { done <- RunMax(c, 500*time.Millisecond) }()
		var err error
		select {
		case err = <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("막힌 쓰개에 묶여 돌아오지 않았다")
		}
		if el := time.Since(t0); el > 500*time.Millisecond+3*Grace+time.Second {
			t.Fatalf("마감 500ms+회수 유예인데 %v", el)
		}
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("출력을 다 옮기지 못했으니 모름: %v", err)
		}
		n := w.calls()
		w.rel <- struct{}{} // 막힌 쓰기 하나를 풀어 준다 — 그 뒤 새 쓰기가 시작되면 안 된다
		time.Sleep(300 * time.Millisecond)
		if m := w.calls(); m != n {
			t.Fatalf("반환 뒤 쓰개에 새로 썼다: %d → %d", n, m)
		}
	})
}

// openFDs: 이 프로세스의 열린 fd 수
func openFDs(t *testing.T) int {
	t.Helper()
	es, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skip("/dev/fd 없음")
	}
	return len(es)
}

// P4 fd — merger/R-219: 정상·시작 실패·시간 넘김 모두에서 읽기 끝을 닫는다(GC 없이)
func TestGofix3ReadFDsClosed(t *testing.T) {
	t.Setenv("HERDR_PROC_DIR", t.TempDir())
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	run := func() {
		var b bytes.Buffer
		c := exec.Command("sh", "-c", "echo ok")
		c.Stdout = &b
		if err := RunMax(c, 5*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	run() // 준비(신호 처리기 등)는 한 번 먼저
	n0 := openFDs(t)
	for i := 0; i < 3; i++ {
		run()
	}
	c := exec.Command("/없는/명령")
	c.Stdout = &bytes.Buffer{}
	RunMax(c, 5*time.Second)
	c = exec.Command("sh", "-c", "sleep 5")
	c.Stdout = &bytes.Buffer{}
	RunMax(c, 200*time.Millisecond)
	if n := openFDs(t); n != n0 {
		t.Fatalf("열린 fd %d → %d(정상 3회·시작 실패·시간 넘김 뒤)", n0, n)
	}
}

// P5 소유 — merger/R-207 · R-221: 우두머리 종료와 출력 끝 사이(동기화 지점 leaderdone)에서 소유 판정
func TestGofix3LeaderDoneOwnership(t *testing.T) {
	cases := []struct {
		name   string
		inside bool // 관을 쥔 손자가 무리 안(남음)인가 밖(무리 사라짐)인가
	}{{"무리 사라짐 — bounded에 없음·신호 0", false}, {"무리 남음 — 그 무리만", true}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			t.Setenv("HERDR_PROC_DIR", d+"/rec")
			old := Grace
			Grace = 300 * time.Millisecond
			defer func() { Grace = old }()
			other, otherBg := startGroup(t, "wait") // 남의 무리 — 신호가 가면 안 된다
			defer func() { syscall.Kill(-other.Process.Pid, syscall.SIGKILL); other.Wait() }()
			// 무리 밖에서 관을 쥔다 — 무리를 떠난 뒤에야 우두머리가 끝난다(표식 out)
			hold := "perl -e 'setpgrp(0,0); open(F, \">\", $ARGV[0]); close F; sleep 3' \"$0/out\" & while [ ! -e \"$0/out\" ]; do sleep 0.01; done;"
			if c.inside {
				hold = "sleep 3 &"
			}
			cmd := exec.Command("sh", "-c", "echo $$ >\"$0/lead\"; "+hold+" echo $! >\"$0/holder\"; exit 0", d)
			var b bytes.Buffer
			cmd.Stdout = &b
			seen, inBounded := false, false
			testHook = func(s string) {
				if s != "leaderdone" {
					return
				}
				seen = true
				lead := waitFile(t, d+"/lead")
				curMu.Lock()
				_, inBounded = bounded[lead]
				curMu.Unlock()
				if !c.inside {
					until(func() bool { return groupGone(lead) }) // 무리가 사라졌음을 먼저 확인
				}
			}
			defer func() { testHook = nil }()
			err := RunMax(cmd, 2*time.Second)
			holder := waitFile(t, d+"/holder")
			defer syscall.Kill(holder, syscall.SIGKILL)
			if !seen {
				t.Fatal("우두머리 종료와 출력 끝 사이 동기화 지점(leaderdone)이 없다")
			}
			if !errors.Is(err, ErrTimeout) {
				t.Fatalf("관이 마감 안에 안 닫히면 모름: %v", err)
			}
			if !c.inside && inBounded {
				t.Fatal("무리가 사라졌는데 번호가 bounded에 남았다(숫자만으로 소유)")
			}
			if c.inside {
				if !inBounded {
					t.Fatal("무리가 남았는데 bounded에서 내렸다")
				}
				if !gone(holder) {
					t.Fatal("남은 그 무리를 거두지 않았다")
				}
			}
			if !Alive(otherBg) {
				t.Fatal("남의 무리에 신호를 보냈다")
			}
			curMu.Lock()
			n := len(bounded)
			curMu.Unlock()
			if n != 0 {
				t.Fatalf("끝난 뒤 bounded가 비지 않았다: %d", n)
			}
		})
	}
}

// P5 ChildBusy — merger/R-207: 기록 pid가 끝났는데 무리가 남았으면(시작 시각 다른 기록) 신호 없이 busy, 기록 그대로
func TestGofix3ChildBusyLeaderGone(t *testing.T) {
	rec := t.TempDir() + "/child"
	pg, _, bg := leaderGone(t)
	os.WriteFile(rec, []byte(strconv.Itoa(pg)+"\tMon Jan  1 00:00:00 2001\n"), 0o666)
	if !ChildBusy(rec, time.Second) {
		t.Fatal("우두머리가 끝나고 무리가 남았으면 도는 것으로 쳐야")
	}
	if !Alive(bg) {
		t.Fatal("소유를 입증할 수 없는 무리에 신호를 보냈다")
	}
	if _, err := os.Stat(rec); err != nil {
		t.Fatal("기록을 지웠다")
	}
}

// 새 API — RunCtx: ctx가 끝나면 마감과 같게(무리째 거둠, ErrTimeout·ctx 오류 둘 다)
func TestGofix3RunCtx(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HERDR_PROC_DIR", d+"/rec")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	c := exec.Command("sh", "-c", "sleep 30 & echo $! >"+d+"/bg; wait")
	err := RunCtx(ctx, c, time.Minute)
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx 취소는 모름(ErrTimeout)·context.Canceled: %v", err)
	}
	if bg := waitFile(t, d+"/bg"); !gone(bg) {
		t.Fatal("ctx 취소 뒤 무리가 남았다")
	}
	b, err := CombinedMax(exec.Command("sh", "-c", "echo o; echo e >&2"), 5*time.Second)
	if err != nil || string(b) != "o\ne\n" {
		t.Fatalf("CombinedMax: %q %v", b, err)
	}
	b, err = OutputMax(exec.Command("sh", "-c", "echo o; echo e >&2; exit 3"), 5*time.Second)
	if Code(err) != 3 || string(b) != "o\n" {
		t.Fatalf("OutputMax: %q %v", b, err)
	}
}

// gofix3Helper: 하위 프로세스 사례
func gofix3Helper(h, d string) int {
	switch h {
	case "g3cold", "g3warm":
		Grace = 300 * time.Millisecond
		if h == "g3warm" {
			if _, err := self(time.Time{}); err != nil {
				fmt.Println("self:", err)
				return 1
			}
		}
		real := psPath
		psPath = d + "/slowps"
		os.WriteFile(psPath, []byte("#!/bin/sh\nsleep 3\nexec /bin/ps \"$@\"\n"), 0o755)
		t0 := time.Now()
		err := RunMax(exec.Command("git", "status"), time.Second)
		el := time.Since(t0)
		psPath = real
		// 다음 호출은 준비를 다시 한다 — 고아 무리(부모가 죽은 기록)를 거두고 명령을 돌린다
		if err := RunMax(exec.Command("sh", "-c", "exit 0"), 20*time.Second); err == nil {
			fmt.Println("retry=ok")
		} else {
			fmt.Println("retry:", err)
		}
		fmt.Printf("rc=%d el=%d\n", Code(err), el)
		return 0
	case "g3waitkill":
		go func() {
			until(func() bool { b, _ := os.ReadFile(d + "/lead"); return strings.HasSuffix(string(b), "\n") })
			syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}()
		max, _ := strconv.Atoi(os.Getenv("G3_MAX"))
		WaitMax(exec.Command("sh", "-c", "echo $$ >\"$0/lead.tmp\"; mv \"$0/lead.tmp\" \"$0/lead\"; exec sleep 60", d), "", time.Duration(max)*time.Second)
		time.Sleep(time.Minute)
		return 0
	}
	return 2
}
