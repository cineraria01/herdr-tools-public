//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// knobFix: 손잡이 저장을 시험 HOME에. reround는 「다음 바퀴」(새 프로세스처럼 저장 파일을 다시 읽음)
func knobFix(t *testing.T) (reround func()) {
	t.Helper()
	oh, od, ol, oc, odr := HOME, DATA, LOCAL, confLines, dry
	HOME, DATA, LOCAL, dry = t.TempDir(), t.TempDir(), "here", false // 앞 시험이 남긴 dry에 기대지 않는다(dry면 원격 싣기가 push하지 않는다)
	t.Setenv("HC_TERMINAL_ID", "w-test")
	reround = func() { knobOnce, knobES = sync.Once{}, nil }
	reround()
	t.Cleanup(func() { HOME, DATA, LOCAL, confLines, dry = oh, od, ol, oc, odr; reround() })
	return reround
}

func knobLsOut(t *testing.T) []knobRow {
	t.Helper()
	m := map[string]knobRow{}
	for _, r := range knobRows(time.Now()) {
		m[r.Name] = r
	}
	return []knobRow{m["idle"], m["hold"]}
}

// ① set → 다음 바퀴에 그 값, ls에 누가·왜·언제까지 ② 기한이 지나면 설정 값으로
func TestKnobSetNextRound(t *testing.T) {
	reround := knobFix(t)
	confLines = []string{"local here", "idle 15"}
	now := time.Now()
	if err := knobSet([]string{"idle", "40", "--why", "밤사이 조용히", "--for", "30"}, now); err != nil {
		t.Fatal(err)
	}
	reround()
	if conf("idle") != "40" || confRaw("idle") != "15" {
		t.Fatalf("다음 바퀴 conf(idle) = %q, 설정 = %q", conf("idle"), confRaw("idle"))
	}
	r := knobLsOut(t)[0]
	if r.Source != "손잡이" || r.By != "w-test" || r.Why != "밤사이 조용히" || r.Until == "" || r.Range != "0~1440" {
		t.Fatalf("ls 줄 = %+v", r)
	}
	if h := knobLsOut(t)[1]; h.Source != "기본" || h.Value != "60" {
		t.Fatalf("손대지 않은 hold = %+v", h)
	}
	// ② 기한을 지난 것으로 고쳐 쓴다(파일을 직접 — 시계 손잡이 없이)
	es, err := knob.Load(knob.Path(HOME))
	if err != nil {
		t.Fatal(err)
	}
	e := es["idle"]
	e.Until = now.Unix() - 1
	if err := knob.Set(knob.Path(HOME), e, true, time.Unix(e.At, 0)); err != nil {
		t.Fatal(err)
	}
	reround()
	if conf("idle") != "15" {
		t.Fatalf("기한 뒤 conf(idle) = %q (설정 15여야)", conf("idle"))
	}
	if err := knobUnset([]string{"idle"}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// ③ 고정 ④ 모르는 이름 ⑤ 범위 밖 ⑦ --why 없음 ⑧ 안전 값은 바꿀 길이 없다
func TestKnobRejects(t *testing.T) {
	reround := knobFix(t)
	confLines = []string{"local here", "overlap block"}
	now := time.Now()
	if err := knobSet([]string{"hold", "90", "--why", "사람이 못 박음", "--pin"}, now); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"hold", "30", "--why", "에이전트"}, "고정된 값"},
		{[]string{"idel", "5", "--why", "x"}, "이것인가: idle"},
		{[]string{"wake_every", "0", "--why", "x"}, "1~240"},
		{[]string{"idle", "abc", "--why", "x"}, "정수"},
		{[]string{"idle", "5"}, "--why"},
		{[]string{"idle", "5", "--why", "  "}, "--why"},
		{[]string{"idle", "5", "--why", "x", "--until", "2000-01-01 00:00"}, "지났다"},
		{[]string{"overlap", "off", "--why", "x"}, "모르는 손잡이: overlap"},
		{[]string{"audit", "-", "--why", "x"}, "모르는 손잡이"},
	}
	for _, c := range cases {
		err := knobSet(c.args, now)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("set %v = %v (%q여야)", c.args, err, c.want)
		}
	}
	if err := knobUnset([]string{"hold"}, now); err == nil {
		t.Error("고정 값을 --force 없이 지움")
	}
	if err := knobSet([]string{"hold", "30", "--why", "사람이 풂", "--force"}, now); err != nil {
		t.Fatalf("--force: %v", err)
	}
	// 저장 파일에 손으로 쓴 안전 값 줄도 먹지 않는다
	f, err := os.OpenFile(knob.Path(HOME), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("overlap\toff\t사람\t손으로\t1\t-\t0\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	reround()
	if conf("overlap") != "block" || conf("hold") != "30" {
		t.Fatalf("overlap = %q, hold = %q", conf("overlap"), conf("hold"))
	}
}

// ⑥ 깨진 저장 파일 → 설정 값으로, 알림 한 줄
func TestKnobBrokenFile(t *testing.T) {
	reround := knobFix(t)
	confLines = []string{"local here", "idle 15"}
	p := knob.Path(HOME)
	if err := os.MkdirAll(p[:strings.LastIndex(p, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("idle\t40\t깨짐\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reround()
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	v1, v2 := conf("idle"), conf("idle")
	os.Stderr = old
	w.Close()
	b, _ := io.ReadAll(r)
	if v1 != "15" || v2 != "15" {
		t.Fatalf("깨진 파일인데 conf(idle) = %q %q", v1, v2)
	}
	if n := strings.Count(string(b), "손잡이를 못 읽어"); n != 1 {
		t.Fatalf("알림 %d줄: %q", n, b)
	}
}

// ⑨ Remote 손잡이는 바퀴마다 원격 참조로(바뀐 때만), 공용 함수가 다른 클론에서 읽는다. 지우면 참조도 지운다
func TestKnobRemoteTick(t *testing.T) {
	reround := knobFix(t)
	defer func(o []knob.Def) { knob.Defs = o }(knob.Defs)
	knob.Defs = append(append([]knob.Def(nil), knob.Defs...), knob.Def{Name: "t_remote", Tool: "hc-merger", Default: "3", Min: 1, Max: 9, Remote: true})
	d := t.TempDir()
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"} {
		t.Setenv(k, v)
	}
	g := func(args ...string) string {
		o, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	g("init", "-q", "--bare", d+"/o.git")
	g("clone", "-q", d+"/o.git", d+"/r")
	g("clone", "-q", d+"/o.git", d+"/far")
	confLines = []string{"local here", "repo app here " + d + "/r", "repo app box-1 /srv/app"}
	knobsRemoteTick() // 실을 것 없음 — 참조 없음
	if r, _ := knob.ReadRemote(d+"/far", io.Discard, time.Now()); r.Get("t_remote") != "3" {
		t.Fatal("참조가 없으면 기본값")
	}
	if err := knobSet([]string{"t_remote", "7", "--why", "병합 묶음 늘림", "--for", "60"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reround()
	knobsRemoteTick()
	if r, _ := knob.ReadRemote(d+"/far", io.Discard, time.Now()); r.Get("t_remote") != "7" {
		t.Fatalf("원격 값 = %v", r)
	}
	sha := g("--git-dir="+d+"/o.git", "rev-parse", knob.Ref)
	g("--git-dir="+d+"/o.git", "update-ref", knob.Ref, sha) // 그대로
	knobsRemoteTick()
	if g("--git-dir="+d+"/o.git", "rev-parse", knob.Ref) != sha {
		t.Fatal("같은 값이면 같은 SHA")
	}
	if err := knobUnset([]string{"t_remote"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	reround()
	knobsRemoteTick()
	if exec.Command("git", "--git-dir="+d+"/o.git", "rev-parse", "-q", "--verify", knob.Ref).Run() == nil {
		t.Fatal("값을 지우면 참조도 지운다")
	}
	if b, _ := os.ReadFile(DATA + "/knobs-remote.tsv"); len(b) != 0 {
		t.Fatalf("기록은 비어야: %q", b)
	}
}
