//go:build !windows

package main

// 메모리 디스크(ramtmp.go)의 계약 — 가짜 hdiutil·diskutil(PATH 앞)로: 포맷 전 확인 ①②③ 하나라도 아니면 포맷 명령이 한 번도 안 불림 ·
// 떼기는 메모리 디스크 이미지일 때만 · 켬이면 조각의 TMPDIR이 그 볼륨이고 끝(신호로 끝날 때도)에 뗌 · 끔·macOS 아님·너무 큼이면 아무
// 명령도 안 부름 · 만들기 실패·시간 넘김이면 한 줄 알리고 보통 임시 폴더로 끝까지 · 죽은 pid의 남은 디스크만 치움.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeDisk: 상태는 $FAKE_ST(sectors·mounted·extra), 부름은 $FAKE_ST/log 한 줄씩. 붙인 장치는 늘 /dev/disk9
const fakeDisk = `#!/bin/bash
st=$FAKE_ST
me=$(basename "$0")
echo "$me $*" >>"$st/log"
case "$me $1" in
"hdiutil attach")
  [ -n "${FAKE_ATTACH_SLEEP-}" ] && exec sleep "$FAKE_ATTACH_SLEEP"
  [ "${FAKE_ATTACH_RC:-0}" = 0 ] || exit 1
  echo "${3#ram://}" >"$st/sectors"
  if [ -n "${FAKE_ATTACH_OUT+x}" ]; then printf '%b' "$FAKE_ATTACH_OUT"; else printf '/dev/disk9          \t                               \t\n'; fi ;;
"hdiutil info")
  printf 'framework       : 1\n'
  [ -f "$st/extra" ] && cat "$st/extra"
  if [ -f "$st/sectors" ]; then
    printf '================================================\nimage-path      : %s\nblockcount      : %s\nblocksize       : 512\n' \
      "${FAKE_IMAGE_PATH:-ram://$(cat "$st/sectors")}" "${FAKE_BLOCKS:-$(cat "$st/sectors")}"
    printf '/dev/disk9\t\t\n'
    [ -n "${FAKE_PREMOUNT-}" ] && printf '/dev/disk10s1\tX\t%s\n' "$FAKE_PREMOUNT"
    [ -f "$st/mounted" ] && printf '/dev/disk10\tGUID\t\n/dev/disk10s1\tGUID\t%s\n' "$(cat "$st/mounted")"
  fi
  exit 0 ;;
"diskutil erasevolume")
  [ "${FAKE_ERASE_RC:-0}" = 0 ] || exit 1
  mkdir -p "$FAKE_VOLROOT/$3" && echo "$FAKE_VOLROOT/$3" >"$st/mounted" ;;
"hdiutil detach")
  [ "${FAKE_DETACH_RC:-0}" = 0 ] || exit 1
  [ "$2" = /dev/disk9 ] && rm -f "$st/sectors" "$st/mounted"
  exit 0 ;;
esac
`

// fakeRAM: 가짜 hdiutil·diskutil과 볼륨 뿌리를 차리고 (상태 폴더, 부름 기록 읽기)
func fakeRAM(t *testing.T) (string, func() string) {
	t.Helper()
	B, st, V := t.TempDir(), t.TempDir(), tmp(t)
	for _, n := range []string{"hdiutil", "diskutil"} {
		if err := os.WriteFile(filepath.Join(B, n), []byte(fakeDisk), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_ST", st)
	t.Setenv("FAKE_VOLROOT", V)
	t.Setenv("HERDR_VERIFY_RAMTMP", "64")
	for _, k := range []string{"FAKE_ATTACH_SLEEP", "FAKE_ATTACH_RC", "FAKE_IMAGE_PATH", "FAKE_BLOCKS", "FAKE_PREMOUNT", "FAKE_ERASE_RC", "FAKE_DETACH_RC"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	os.Unsetenv("FAKE_ATTACH_OUT")
	oGoos, oVol, oTO, oMem := goos, volRoot, ramTimeout, memBytes
	goos, volRoot, ramTimeout, memBytes = "darwin", V, 10*time.Second, func() int64 { return 32 << 30 }
	t.Cleanup(func() { goos, volRoot, ramTimeout, memBytes = oGoos, oVol, oTO, oMem })
	return st, func() string { b, _ := os.ReadFile(filepath.Join(st, "log")); return string(b) }
}

// 붙이기 출력이 이상하거나 확인 ①②③ 하나라도 아니면: 포맷 명령이 한 번도 안 불리고 nil. 떼기는 ①(메모리 디스크)일 때만
func TestRAMRefusesFormat(t *testing.T) {
	for _, c := range []struct {
		name, env, val string
		detach         bool
	}{
		{"빈 출력", "FAKE_ATTACH_OUT", "", false},
		{"여러 줄", "FAKE_ATTACH_OUT", `/dev/disk9\t\t\n/dev/disk10\t\t\n`, false},
		{"다른 꼴", "FAKE_ATTACH_OUT", `disk9\n`, false},
		{"조각 장치", "FAKE_ATTACH_OUT", `/dev/disk9s1\t\t\n`, false},
		{"마운트 칸", "FAKE_ATTACH_OUT", `/dev/disk9\tGUID\t/Volumes/x\n`, false},
		{"① 메모리 디스크 아님", "FAKE_IMAGE_PATH", "/Users/x/disk.dmg", false},
		{"② 크기 다름", "FAKE_BLOCKS", "2048", true},
		{"③ 이미 마운트됨", "FAKE_PREMOUNT", "/Volumes/other", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, calls := fakeRAM(t)
			t.Setenv(c.env, c.val)
			f, read := outFile(t)
			r := ramSetup(f)
			log := calls()
			if r != nil || strings.Contains(log, "erasevolume") || !strings.Contains(read(), "알림 메모리 디스크를 못 만들어") {
				t.Errorf("포맷 없이 nil·알림이어야 r=%v\n%s\n%s", r, log, read())
			}
			if got := strings.Contains(log, "hdiutil detach /dev/disk9"); got != c.detach {
				t.Errorf("떼기 %v여야 %v\n%s", c.detach, got, log)
			}
		})
	}
}

// verifyFake: 가짜 go(TestPlan과 같은 꼴, 부를 때마다 「TMPDIR=… 인자」 한 줄)로 verify를 돌릴 준비. 조각은 herdr-tools/a 하나.
// sleepShard면 조각이 started 파일을 쓰고 5분 잔다
func verifyFake(t *testing.T, sleepShard bool) (golog func() string, run func(stop chan os.Signal) (int, string)) {
	t.Helper()
	R, B := tmp(t), t.TempDir()
	lg := B + "/golog"
	shard := ""
	if sleepShard {
		shard = "  *' herdr-tools/a') touch '" + B + "/started'; exec sleep 300 ;;\n"
	}
	fake := "#!/bin/sh\necho \"TMPDIR=$TMPDIR BOOT=$HERDR_VERIFY_BOOT_TMPDIR $*\" >>'" + lg + "'\ncase \"$*\" in\n" + shard +
		"  'list ./...') printf 'herdr-tools/a\\n' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(B+"/go", []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", B+":"+os.Getenv("PATH"))
	t.Setenv("HERDR_VERIFY_ALL", "")
	t.Setenv("HERDR_VERIFY_JOBS", "")
	t.Setenv("HERDR_VERIFY_NO_LOCK", "1")
	golog = func() string { b, _ := os.ReadFile(lg); return string(b) }
	run = func(stop chan os.Signal) (int, string) {
		f, read := outFile(t)
		if stop == nil {
			stop = make(chan os.Signal)
		}
		return verify(R, nil, stop, f), read()
	}
	if sleepShard {
		t.Setenv("VERIFY_STARTED", B+"/started")
	}
	return golog, run
}

// shardLine: 가짜 go 기록에서 조각(herdr-tools/a) 줄
func shardLine(log string) string {
	for _, l := range strings.Split(log, "\n") {
		if strings.HasSuffix(l, " herdr-tools/a") {
			return l
		}
	}
	return ""
}

// 켬: 조각의 TMPDIR이 그 볼륨의 t/, 원래 임시 폴더는 HERDR_VERIFY_BOOT_TMPDIR, 홀로 도는 cotenant·싼 단계는 그대로, 끝에 뗌
func TestRAMOn(t *testing.T) {
	_, calls := fakeRAM(t)
	golog, run := verifyFake(t, false)
	rc, out := run(nil)
	vol := filepath.Join(volRoot, ramPrefix+strconv.Itoa(os.Getpid()))
	sl, log := shardLine(golog()), calls()
	if rc != 0 || !strings.HasPrefix(sl, "TMPDIR="+vol+"/t/ BOOT="+os.TempDir()+" ") {
		t.Errorf("조각 TMPDIR이 메모리 디스크여야 rc=%d\n%s\n%s", rc, golog(), out)
	}
	for _, l := range strings.Split(golog(), "\n") {
		if l != sl && strings.Contains(l, vol) {
			t.Errorf("조각 밖 단계는 메모리 디스크가 아니어야: %s", l)
		}
	}
	if _, err := os.Stat(vol + "/.metadata_never_index"); err != nil {
		t.Errorf(".metadata_never_index가 있어야: %v", err)
	}
	if !strings.Contains(log, "diskutil erasevolume APFS "+ramPrefix+strconv.Itoa(os.Getpid())+" /dev/disk9") ||
		!strings.HasSuffix(log, "hdiutil detach /dev/disk9\n") || strings.Count(log, "hdiutil detach") != 1 {
		t.Errorf("포맷 한 번·끝에 뗌 한 번이어야\n%s", log)
	}
	if !strings.Contains(out, "메모리 디스크 사용량 최대 ") || strings.Index(out, "메모리 디스크 사용량") > strings.Index(out, "검증 시간") {
		t.Errorf("사용량 줄이 검증 시간 줄 앞에\n%s", out)
	}
}

// 끝에 뗄 때 그 장치에 우리 볼륨이 마운트돼 있지 않으면(누가 바꿈) 떼지 않고 알림 한 줄
func TestRAMTeardownNeedsOurVolume(t *testing.T) {
	st, calls := fakeRAM(t)
	f, read := outFile(t)
	r := ramSetup(f)
	if r == nil {
		t.Fatalf("만들어야\n%s\n%s", calls(), read())
	}
	if err := os.WriteFile(filepath.Join(st, "mounted"), []byte(volRoot+"/someone-else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.teardown(f)
	if strings.Contains(calls(), "hdiutil detach") || !strings.Contains(read(), "알림 메모리 디스크를 못 뗐다") {
		t.Errorf("우리 볼륨이 아니면 안 떼야\n%s\n%s", calls(), read())
	}
}

// 끔·macOS 아님·기계 메모리의 1/4을 넘음: 아무 명령도 안 부르고 조각 TMPDIR은 그대로
func TestRAMOff(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(t *testing.T)
		note string
	}{
		{"HERDR_VERIFY_RAMTMP=0", func(t *testing.T) { t.Setenv("HERDR_VERIFY_RAMTMP", "0") }, ""},
		{"macOS 아님", func(t *testing.T) { goos = "linux" }, ""},
		{"너무 큼", func(t *testing.T) { memBytes = func() int64 { return 128 << 20 } }, "1/4을 넘는다"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, calls := fakeRAM(t)
			c.set(t)
			golog, run := verifyFake(t, false)
			rc, out := run(nil)
			if rc != 0 || calls() != "" || strings.Contains(shardLine(golog()), volRoot) || !strings.Contains(out, c.note) {
				t.Errorf("아무 명령 없이 통과여야 rc=%d\n%s\n%s", rc, calls(), out)
			}
		})
	}
}

// 만들기 실패·시간 넘김·포맷 실패: 한 줄 알리고 보통 임시 폴더로 끝까지 통과
func TestRAMFallback(t *testing.T) {
	for _, c := range []struct{ name, env, val, note string }{
		{"붙이기 실패", "FAKE_ATTACH_RC", "1", "알림 메모리 디스크를 못 만들어"},
		{"붙이기 시간 넘김", "FAKE_ATTACH_SLEEP", "30", "넘김"},
		{"포맷 실패", "FAKE_ERASE_RC", "1", "erasevolume"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, calls := fakeRAM(t)
			ramTimeout = time.Second
			t.Setenv(c.env, c.val)
			golog, run := verifyFake(t, false)
			t0 := time.Now()
			rc, out := run(nil)
			if rc != 0 || strings.Contains(shardLine(golog()), volRoot) || !strings.Contains(out, c.note) ||
				strings.Count(out, "알림 메모리 디스크") != 1 || time.Since(t0) > 20*time.Second {
				t.Errorf("알림 한 줄·보통 임시 폴더로 통과여야 rc=%d %v\n%s\n%s", rc, time.Since(t0), calls(), out)
			}
			if c.env == "FAKE_ERASE_RC" && !strings.Contains(calls(), "hdiutil detach /dev/disk9") {
				t.Errorf("포맷 실패면 붙인 장치를 뗀다\n%s", calls())
			}
		})
	}
}

// 신호로 끝날 때(조각이 도는 중 TERM)도 뗀다
func TestRAMSignal(t *testing.T) {
	_, calls := fakeRAM(t)
	_, run := verifyFake(t, true)
	stop := make(chan os.Signal, 1)
	rcc := make(chan int, 1)
	go func() { rc, _ := run(stop); rcc <- rc }()
	started := os.Getenv("VERIFY_STARTED")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop <- syscall.SIGTERM
	select {
	case rc := <-rcc:
		if rc != 143 || !strings.HasSuffix(calls(), "hdiutil detach /dev/disk9\n") {
			t.Errorf("끊김: 143·뗌이어야 rc=%d\n%s", rc, calls())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("끊김 뒤 돌아오지 않음")
	}
}

// 죽은 검증이 남긴 디스크: 우리 이름 꼴·메모리 디스크·pid가 죽은 것만 뗀다(산 pid·남의 이름·볼륨 뿌리 밖·메모리 디스크 아님은 그대로)
func TestRAMSweepStale(t *testing.T) {
	st, calls := fakeRAM(t)
	c := exec.Command("true")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	dead, live := c.Process.Pid, os.Getppid()
	sec := func(dev, path, mount string) string {
		return "================================================\nimage-path      : " + path + "\nblockcount      : 2048\n" +
			dev + "\t\t\n" + dev + "s1\tGUID\t" + mount + "\n"
	}
	extra := sec("/dev/disk20", "ram://2048", volRoot+"/"+ramPrefix+strconv.Itoa(dead)) +
		sec("/dev/disk21", "ram://2048", volRoot+"/"+ramPrefix+strconv.Itoa(live)) +
		sec("/dev/disk22", "/Users/x/disk.dmg", volRoot+"/"+ramPrefix+strconv.Itoa(dead)) +
		sec("/dev/disk23", "ram://2048", volRoot+"/other-"+strconv.Itoa(dead)) +
		sec("/dev/disk24", "ram://2048", "/elsewhere/"+ramPrefix+strconv.Itoa(dead)) +
		sec("/dev/disk25", "ram://2048", volRoot+"/"+ramPrefix+strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(filepath.Join(st, "extra"), []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	f, read := outFile(t)
	sweepStale(f)
	var got []string
	for _, l := range strings.Split(calls(), "\n") {
		if strings.HasPrefix(l, "hdiutil detach") {
			got = append(got, l)
		}
	}
	if strings.Join(got, "\n") != "hdiutil detach /dev/disk20\nhdiutil detach /dev/disk25" {
		t.Errorf("죽은 pid(disk20)·이 프로세스 이름(disk25)만 떼야\n%s\n%s", calls(), read())
	}
}
