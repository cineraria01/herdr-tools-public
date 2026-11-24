package main

// watch-idle-open(2026-11-24): 지휘 레인의 마지막 상태가 제출·주차(결과가 아직 안 옴)인 동안은 watch의 쉬는 시간으로 세지 않는다 —
// 레인이 제출하고 닫힌 뒤 병합기 결과를 기다리는 사이에 watch가 lane.watch_idle분 만에 끝나 착지·막힘 알림이 늦거나 다른 지휘에게 갔다.
// 끝날 때는 까닭을 watch.log에 한 줄. hc-lane coord --watch <지휘>: hc-fill 바퀴가 주인 지휘의 watch를 다시 띄우는 길

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// idleFake: get은 견본, ls는 빈 목록(바퀴마다 calls에 한 줄), send는 send 파일에, run은 run 파일에
const idleFake = `#!/bin/sh
echo "$1" >>"$D/calls"
case $1 in
get) cat "$D/body"; exit 0 ;;
ls) echo '{"machines":[],"terminals":[]}'; exit 0 ;;
send) printf '%s\t%s\n' "$2" "$3" >>"$D/send"; exit 0 ;;
run) echo "$*" >>"$D/run"; echo '{"address":"t9"}'; exit 0 ;;
esac
exit 0
`

// idleEnv: 가짜 hc와 지휘 정보(견본의 pid·이름)를 둔 폴더 — 지휘 주소를 돌려준다
func idleEnv(t *testing.T, d string, body []byte) string {
	t.Helper()
	tm, _, err := parseSample(t, body)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{d + "/bin", d + "/st/hc-lane"} {
		if err := os.MkdirAll(p, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(d+"/bin/hc", []byte(idleFake), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d+"/body", body, 0o644); err != nil {
		t.Fatal(err)
	}
	nm := tm.Name
	if nm == "" {
		nm = "-"
	}
	a := tm.Address
	if err := os.WriteFile(d+"/st/hc-lane/coord."+ckey(a), []byte(a+tab+strconv.Itoa(tm.PID)+tab+nm+tab+"0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return a
}

func idleCmdEnv(d string, extra ...string) []string {
	return append([]string{"PATH=" + d + "/bin:/usr/bin:/bin", "HOME=" + d, "XDG_STATE_HOME=" + d + "/st", "D=" + d,
		"HC_FILL_DATA=" + d + "/fill", "HC_LANE_WAIT_TICK=1", "HC_LANE_WATCH_HOLD_MAX=2"}, extra...)
}

// openLine: 그 지휘가 주인(11칸)인 레인 사건 한 줄 — 레인 터미널은 이미 닫힘(hc ls에 없음)
func openLine(at int64, lane, st, owner string) string {
	return strings.Join([]string{utcAt(at), "m", "t5", lane, st, "-", "요약", "1", "-", "-", owner}, tab) + "\n"
}

// startIdleWatch: 사건 파일을 두고 watch --to <지휘>를 돌린다
func startIdleWatch(t *testing.T, bin, d, co, events string, extra ...string) *watchRun {
	t.Helper()
	ed := d + "/st/hc-lane"
	if err := os.WriteFile(ed+"/events.tsv", []byte(events), 0o644); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bin, "watch", "--to", co)
	ef, err := os.Create(d + "/stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ef.Close() })
	c.Stderr = ef
	c.Env = idleCmdEnv(d, extra...)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	w := &watchRun{cmd: c, send: d + "/send", wlog: ed + "/watch.log", ed: ed, errf: d + "/stderr", done: make(chan error, 1)}
	go func() { w.done <- c.Wait() }()
	t.Cleanup(func() { c.Process.Kill(); <-w.done })
	return w
}

// lsTicks: watch가 hc ls를 부른 바퀴 수
func lsTicks(d string) int {
	b, _ := os.ReadFile(d + "/calls")
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if l == "ls" {
			n++
		}
	}
	return n
}

func exited(w *watchRun) bool {
	select {
	case err := <-w.done:
		w.done <- err
		return true
	default:
		return false
	}
}

func TestWatchIdleOpenSubmit(t *testing.T) {
	if testing.Short() {
		t.Skip("바이너리를 빌드해 watch를 돌린다")
	}
	bin := buildLane(t, t.TempDir())
	body := agentSample(t, "4-shell-agent-idle")

	// ① 제출만 남은 지휘: 쉬는 시간 상한(2초)을 몇 바퀴 넘겨도 끝나지 않고, 착지 사건을 받은 뒤에야 한가로 끝난다
	d := t.TempDir()
	co := idleEnv(t, d, body)
	w := startIdleWatch(t, bin, d, co, openLine(time.Now().Unix(), "lane-y", "제출", co), "HC_LANE_WATCH_IDLE=2")
	waitFor(t, "제출 알림", func() bool { return w.sendCount("lane-y 제출") > 0 })
	n0 := lsTicks(d)
	waitFor(t, "쉬는 시간 상한을 넘는 바퀴", func() bool { return lsTicks(d) >= n0+5 || exited(w) })
	if exited(w) {
		b, _ := os.ReadFile(w.errf)
		t.Fatalf("① 제출이 열린 동안 watch가 한가로 끝났다: %s", b)
	}
	f, err := os.OpenFile(w.ed+"/events.tsv", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(openLine(time.Now().Unix(), "lane-y", "착지", co))
	f.Close()
	waitFor(t, "착지 알림", func() bool { return w.sendCount("lane-y 착지") > 0 })
	waitFor(t, "착지 뒤 한가로 끝남", func() bool { return exited(w) })
	if b, _ := os.ReadFile(w.wlog); !strings.Contains(string(b), "끝냄(한가)") {
		t.Errorf("③ 끝날 때 watch.log에 까닭이 없다: %q", b)
	}

	// ② 제출이 상한(HC_LANE_WATCH_OPEN_MAX초)보다 오래됨 → 세고 끝남, ③ watch.log에 한 줄
	d2 := t.TempDir()
	co2 := idleEnv(t, d2, body)
	w2 := startIdleWatch(t, bin, d2, co2, openLine(time.Now().Unix()-3600, "lane-z", "주차", co2),
		"HC_LANE_WATCH_IDLE=2", "HC_LANE_WATCH_OPEN_MAX=60")
	waitFor(t, "오래된 제출은 세고 끝남", func() bool { return exited(w2) })
	if b, _ := os.ReadFile(w2.wlog); !strings.Contains(string(b), "끝냄(한가)") {
		t.Errorf("③ 끝날 때 watch.log에 까닭이 없다: %q", b)
	}

	// 다른 지휘의 열린 제출은 이 지휘를 붙잡지 않는다
	d3 := t.TempDir()
	co3 := idleEnv(t, d3, body)
	w3 := startIdleWatch(t, bin, d3, co3, openLine(time.Now().Unix(), "lane-o", "제출", "t-other"), "HC_LANE_WATCH_IDLE=2")
	waitFor(t, "남의 제출은 세지 않고 끝남", func() bool { return exited(w3) })
}

// coord --watch: 지휘 확인을 통과하고 watch가 돌지 않으면 띄운다(④의 hc-lane 쪽), 에이전트가 끝난 지휘에는 안 띄운다(⑤)
func TestCoordWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("바이너리를 빌드해 돌린다")
	}
	bin := buildLane(t, t.TempDir())
	run := func(sample string) (string, int, string) {
		d := t.TempDir()
		co := idleEnv(t, d, agentSample(t, sample))
		c := exec.Command(bin, "coord", "--watch", co)
		c.Env = idleCmdEnv(d)
		out, err := c.CombinedOutput()
		rc := 0
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		r, _ := os.ReadFile(d + "/run")
		return string(r), rc, string(out) + " / " + co
	}
	if r, rc, out := run("4-shell-agent-idle"); rc != 0 || !strings.Contains(r, "watch --all --to ") || !strings.Contains(r, "--label hc-lane-watch=1") {
		t.Errorf("④ 에이전트 있는 지휘에 watch를 띄우지 않았다: 종료 %d, run %q, 출력 %s", rc, r, out)
	}
	if r, rc, out := run("2-shell-agent-exited"); rc != 1 || r != "" {
		t.Errorf("⑤ 셸만 남은 지휘: 종료 %d(바람 1), run %q(바람 없음), 출력 %s", rc, r, out)
	}
}
