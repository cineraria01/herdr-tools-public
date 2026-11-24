//go:build !windows

package clitest

// ②-F 계약 시험(gofix3-testkit): 시험 바이너리를 -test.run·-test.timeout으로 다시 띄워(안쪽 시험 TestContractInner) 시험 마감·회수·
// 표식 판독의 계약을 본다. 표의 줄마다 한 경우 — 지적 base/R-213·R-223·R-224·R-225, hooks/R-M207·R-M208.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"herdr-tools/internal/proc"
)

func TestMain(m *testing.M) { Main(m) }

// TestContractInner: 바깥 계약 시험이 CLITEST_CASE로 고른 안쪽 경우. 바깥 없이 돌면 건너뛴다
func TestContractInner(t *testing.T) {
	S, M := os.Getenv("CLITEST_S"), os.Getenv("CLITEST_M")
	switch os.Getenv("CLITEST_CASE") {
	case "":
		t.Skip("바깥 계약 시험이 띄울 때만")
	case "term-ignored": // TERM을 무시하는 자식 무리(손자 sleep 30) — 마감 전에 무리째 KILL·회수하고 124
		r := Cmd{}.Run("sh", "-c", stubborn, "sh", S)
		if !r.Timeout || r.RC != 124 {
			t.Fatalf("마감 전에 시간 넘김(124)이어야: %+v", r)
		}
	case "refuse": // 남은 마감이 회수 여유보다 짧으면 띄우지 않는다
		dl, ok := t.Deadline()
		if !ok {
			t.Fatal("마감 없음")
		}
		// minRemain(200ms)보다 덜 남게. Run이 보는 바깥 마감(binStart 기준)은 t.Deadline보다 이르니 190ms면 늘 덜 남고,
		// 부하로 잠이 늦게 깨도 마감 panic까지 190ms 여유
		time.Sleep(time.Until(dl) - 190*time.Millisecond)
		r := Cmd{}.Run("sh", "-c", `: >"$1"`, "sh", S)
		if r.RC == 0 || r.Timeout || !strings.Contains(r.Err, "띄우지 않음") {
			t.Fatalf("띄우지 않고 실패여야: %+v", r)
		}
	case "bin", "main": // Bin — 가짜 go(PATH 앞)로 짓는다
		Bin(t, "x")
	case "absent":
		Absent(t, M)
		t.Fatal("Absent가 EACCES를 부재로 읽었다")
	case "space": // 공백 든 임시 폴더에서도 표식은 예상 경로에만
		d := TempDir(t)
		m := filepath.Join(d, "with space", "m")
		os.MkdirAll(filepath.Dir(m), 0o755)
		ch := Spawn(t, exec.Command("sh", "-c", `: >"$1"`, "sh", m))
		ch.Wait()
		WaitFile(t, m, 5*time.Second)
		if !Absent(t, filepath.Join(d, "with")) {
			t.Fatal("잘린 경로에 표식이 생겼다")
		}
		os.WriteFile(S, []byte(d), 0o644)
	default:
		t.Fatal("모르는 경우")
	}
}

// stubborn: TERM을 무시하는 무리 — 손자 sleep 30(무시를 물려받음)의 pid를 $1에 쓰고 기다린다. 손자가 마감보다 훨씬 오래 살아
// 마감을 넉넉히 잡아도(부하에서 늦게 떠도) 「거두지 않았으면 아직 살아 있다」가 성립하고, 우두머리만 죽이면 손자가 남아 무리 회수까지 본다
const stubborn = `trap '' TERM; sleep 30 & echo $! >"$1.tmp"; mv "$1.tmp" "$1"; wait`

// inner: 안쪽 경우 하나를 timeout 마감으로 돌려 (종료 코드, 출력, 걸린 시간)
func inner(t *testing.T, c string, timeout string, env ...string) (int, string, time.Duration) {
	t.Helper()
	t0 := time.Now()
	r := Cmd{Env: append([]string{"CLITEST_CASE=" + c}, env...), Combined: true, Max: time.Minute}.
		Run(os.Args[0], "-test.run=^TestContractInner$", "-test.timeout="+timeout, "-test.count=1", "-test.v")
	return r.RC, r.Out, time.Since(t0)
}

// fakeGo: PATH 앞에 둘 가짜 go
func fakeGo(t *testing.T, body string) string {
	d := TempDir(t)
	Put(t, filepath.Join(d, "go"), "#!/bin/sh\n"+body)
	Chmod(t, filepath.Join(d, "go"), 0o755)
	return d
}

func TestContract(t *testing.T) {
	type tc struct {
		name, kase, timeout string
		env                 func(t *testing.T, d string) []string // 하위 시험의 t(Cleanup 차례가 그 TempDir보다 뒤)
		check               func(t *testing.T, d string, rc int, out string, took time.Duration)
	}
	S := func(d string) string { return filepath.Join(d, "start") }
	Mk := func(d string) string { return filepath.Join(d, "mark") }
	SM := func(_ *testing.T, d string) []string { return []string{"CLITEST_S=" + S(d), "CLITEST_M=" + Mk(d)} }
	// noLater: 시작 표식(손자 pid)을 먼저 본 뒤(원인이 일어남), 안쪽 시험이 끝난 뒤 그 손자가 사라졌는지. 죽은 손자는 고아라 init이
	// 거둘 때까지 잠깐 좀비로 보일 수 있어 5초까지 기다린다 — 거두지 않은 손자는 30초 사니 기다림이 판별력을 잃지 않는다
	noLater := func(t *testing.T, d string) {
		t.Helper()
		pid, err := strconv.Atoi(strings.TrimSpace(Read(S(d))))
		if err != nil || pid <= 1 {
			t.Fatalf("시작 표식 없음 — 자식이 돌지 않아 시험이 되지 않는다 (%q)", Read(S(d)))
		}
		for end := time.Now().Add(5 * time.Second); proc.Alive(pid) && time.Now().Before(end); {
			time.Sleep(50 * time.Millisecond)
		}
		if proc.Alive(pid) {
			syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // 살아남은 손자 정리
			t.Errorf("시험이 끝난 뒤 자식 무리(손자 %d)가 살아남았다", pid)
		}
	}
	for _, c := range []tc{
		// 마감 4s: 회수 여유(유예 rem/5 + KILL 기다림 rem/10×2)를 빼고도 자식이 뜰 창이 약 2.4s. 손자가 30초 살아 마감을 키워도 판별력은 그대로
		{"R-223 TERM 무시 자식 — 마감 전 KILL·회수", "term-ignored", "4s", SM,
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if rc != 0 {
					t.Errorf("안쪽 시험이 마감 전에 124로 끝나야(종료 0): rc %d (%v)\n%s", rc, took, out)
				}
				noLater(t, d)
			}},
		{"R-223 남은 마감 < 회수 여유 — 띄우지 않음", "refuse", "2s", SM,
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if rc != 0 {
					t.Errorf("안쪽 시험이 띄우지 않고 끝나야(종료 0): rc %d\n%s", rc, out)
				}
				time.Sleep(300 * time.Millisecond)
				if !Absent(t, S(d)) {
					t.Errorf("남은 시간이 여유보다 짧은데 자식을 띄웠다")
				}
			}},
		{"R-224 Bin go build 30s · 마감 4s — 빌드 무리 회수", "bin", "4s",
			func(t *testing.T, d string) []string {
				return append(SM(t, d), "PATH="+fakeGo(t, `set -- "$CLITEST_S"; `+stubborn+"\n")+":"+os.Getenv("PATH"))
			},
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if rc != 1 || strings.Contains(out, "panic: test timed out") {
					t.Errorf("빌드가 마감 안에서 끊겨 Bin이 실패(종료 1)해야 — 마감 panic이 아니라: rc %d\n%s", rc, out)
				}
				noLater(t, d)
			}},
		{"R-225 Main 뒤 herdr-test-bin.* 없음", "main", "60s",
			func(t *testing.T, d string) []string {
				Put(t, filepath.Join(d, "tmp", ".keep"), "")
				return []string{"TMPDIR=" + filepath.Join(d, "tmp"), "PATH=" + fakeGo(t, `while [ "$1" != -o ]; do shift; done; : >"$2"; : >"$CLITEST_S"`+"\n") + ":" + os.Getenv("PATH"),
					"CLITEST_S=" + S(d)}
			},
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if rc != 0 || !Exists(S(d)) {
					t.Fatalf("안쪽 Bin이 (가짜) 빌드를 해야 시험이 된다: rc %d\n%s", rc, out)
				}
				if m, _ := filepath.Glob(filepath.Join(d, "tmp", "herdr-test-bin.*")); len(m) != 0 {
					t.Errorf("시험 프로세스가 끝난 뒤 빌드 폴더가 남았다: %v", m)
				}
			}},
		{"R-M207 Absent: EACCES는 부재가 아니다", "absent", "60s",
			func(t *testing.T, d string) []string {
				Put(t, filepath.Join(d, "locked", "f"), "")
				Chmod(t, filepath.Join(d, "locked"), 0)
				t.Cleanup(func() { os.Chmod(filepath.Join(d, "locked"), 0o755) })
				return []string{"CLITEST_M=" + filepath.Join(d, "locked", "f")}
			},
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if os.Geteuid() == 0 {
					t.Skip("root는 EACCES가 안 난다")
				}
				if rc != 1 || strings.Contains(out, "Absent가 EACCES를 부재로") || !strings.Contains(out, "permission denied") {
					t.Errorf("Absent는 EACCES에서 시험을 멈춰야(모름): rc %d\n%s", rc, out)
				}
			}},
		{"R-M207 공백 든 임시 폴더 — 표식은 예상 경로에만", "space", "60s",
			func(t *testing.T, d string) []string {
				Put(t, filepath.Join(d, "t mp", ".keep"), "")
				return []string{"TMPDIR=" + filepath.Join(d, "t mp"), "GOTMPDIR=" + filepath.Join(d, "t mp"), "CLITEST_S=" + S(d)}
			},
			func(t *testing.T, d string, rc int, out string, took time.Duration) {
				if rc != 0 || !strings.Contains(Read(S(d)), " ") {
					t.Errorf("공백 든 경로에서 표식 판독이 맞아야: rc %d 폴더 %q\n%s", rc, Read(S(d)), out)
				}
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := TempDir(t)
			rc, out, took := inner(t, c.kase, c.timeout, c.env(t, d)...)
			c.check(t, d, rc, out, took)
		})
	}
}

// R-213 · R-M208: Spawn한 자식이 먼저 끝났으면 Cleanup(Stop)은 신호를 하나도 보내지 않는다(번호가 다른 프로세스에 다시 쓰였을 수 있다)
func TestSpawnFinishedNoSignal(t *testing.T) {
	for _, wait := range []bool{true, false} {
		before := sigSent.Load()
		t.Run(map[bool]string{true: "Wait 뒤", false: "Wait 없이"}[wait], func(t *testing.T) {
			m := filepath.Join(TempDir(t), "done")
			ch := Spawn(t, exec.Command("sh", "-c", `: >"$1"`, "sh", m))
			if wait {
				if err := ch.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			WaitFile(t, m, 10*time.Second)
			time.Sleep(200 * time.Millisecond) // 자식이 끝날 틈
		})
		if n := sigSent.Load() - before; n != 0 {
			t.Errorf("먼저 끝난 자식에 신호 %d번 — 0이어야", n)
		}
	}
	// 대조: 살아 있는 자식은 Cleanup이 무리째 끈다(신호가 실제로 나간다 — 위 0이 「세지 않음」이 아님을)
	before := sigSent.Load()
	var pid int
	t.Run("살아 있음", func(t *testing.T) {
		m := filepath.Join(TempDir(t), "up")
		ch := Spawn(t, exec.Command("sh", "-c", `: >"$1"; exec sleep 30`, "sh", m))
		pid = ch.Pid
		WaitFile(t, m, 10*time.Second)
	})
	if sigSent.Load() == before || syscall.Kill(pid, 0) == nil {
		t.Errorf("살아 있는 자식은 Cleanup이 끄고 거둬야 (신호 %d)", sigSent.Load()-before)
	}
}
