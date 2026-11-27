//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/knob"
)

// ht-worker-cordon: 저장소 폴더를 못 읽는 기계를 스스로 격리하고 돌아오면 스스로 푼다(docs/hc-fill/cordon.md).
// 틀: 기계 a(이 기계)·b(연결), 프로젝트 p·q. 가짜 hc exec는 그 기계 이름(M)을 env로 주고 sh를 이 기계에서 돌리고, 가짜 hc-lane은
// merger --spec-idle --repo-read면 $CD/<기계> 글(없으면 옛 판처럼 종료 2), 인자 없는 --spec-idle은 한가함 한 줄, report는 ev.log에 한 줄.
// $CD/<기계>.rc가 있으면 hc exec가 그 종료로 실패한다(원격 호출 실패)

const ctFakeHC = `#!/bin/sh
[ "$1" = exec ] || exit 0
while [ "$1" != -- ]; do [ "$1" = --on ] && M=$2; shift; done; shift
echo "$M" >>"$CD/exec.calls"
[ -f "$CD/$M.rc" ] && exit "$(cat "$CD/$M.rc")"
export M; exec "$@"
`

const ctFakeLane = `#!/bin/sh
if [ "$1" = report ]; then printf '%s\n' "$*" >>"$CD/ev.log"; [ -f "$CD/report.rc" ] && exit 1; exit 0; fi
if [ "$1 $2" = "merger --spec-idle" ]; then
  if [ "$3" = --remote-auth ]; then
    echo "${M:-a}" >>"$CD/auth.calls"; a="$CD/${M:-a}.auth"; [ -f "$a" ] || exit 2; f="$CD/${M:-a}"; [ -f "$f" ] || exit 2
    head -1 "$f"; cat "$a"; tail -n +2 "$f"; exit 0
  fi
  if [ "$3" = --repo-read ]; then f="$CD/${M:-a}"; [ -f "$f" ] || exit 2; cat "$f"; exit 0; fi
  echo "load=0.10 cpus=8 verifies=0 gates=0"; exit 0
fi
exit 0
`

const (
	ctGood = "load=0.10 cpus=8 verifies=0 gates=0\nrepo-read 1 ok\nrepo-read 2 ok\nrepo-read end 2"
	ctBad  = "load=0.10 cpus=8 verifies=0 gates=0\nrepo-read 1 bad open /r/b: interrupted system call\nrepo-read 2 ok\nrepo-read end 2"
	ctHang = "load=0.10 cpus=8 verifies=0 gates=0\nrepo-read 1 ok\nrepo-read 2 bad 읽기가 5s 안에 안 끝남\nrepo-read end 2"
)

// ctFix: lpFix(배정 틀) 위에 가짜 hc·hc-lane과 프로젝트 둘. b의 점검 결과는 ct.set으로
type ctx struct {
	t  *testing.T
	cd string
	re func() // 손잡이 다시 읽기(다음 바퀴)
}

func ctFix(t *testing.T) *ctx {
	t.Helper()
	re := lpFix(t)
	confLines = append(confLines, "repo q a /r/qa", "repo q b /r/q b", "cordon 5")
	cd := t.TempDir()
	for n, s := range map[string]string{"hc": ctFakeHC, "hc-lane": ctFakeLane} {
		if err := os.WriteFile(cd+"/"+n, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CD", cd)
	t.Setenv("PATH", cd+":"+os.Getenv("PATH"))
	CORDON, REPOCK, AUTHCK, ROWS, TERMS = nil, nil, nil, nil, nil
	t.Cleanup(func() { CORDON, REPOCK, AUTHCK, TERMS = nil, nil, nil, nil })
	c := &ctx{t, cd, re}
	c.set("a", ctGood)
	c.set("b", ctGood)
	return c
}

func (c *ctx) set(m, out string) {
	c.t.Helper()
	if err := os.WriteFile(c.cd+"/"+m, []byte(out+"\n"), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

func (c *ctx) ev() string { b, _ := os.ReadFile(c.cd + "/ev.log"); return string(b) }

// tick: 바퀴 하나(이 바퀴 점검을 비우고 cordonTick) — 앞 기록의 때를 back초 앞으로 당겨 문턱·간격을 기다리지 않는다
func (c *ctx) tick(back int64) {
	c.t.Helper()
	for _, ck := range cordonCks {
		var rs []Row
		for _, r := range readRows(ck.file()) {
			rs = append(rs, r.Set(cdSince, fmt.Sprint(int64(atoi10(r.F(cdSince)))-back)).Set(cdAt, fmt.Sprint(int64(atoi10(r.F(cdAt)))-back)))
		}
		if len(rs) > 0 {
			if err := writeAtomic(ck.file(), rowsText(rs)); err != nil {
				c.t.Fatal(err)
			}
		}
	}
	REPOCK, AUTHCK = map[string]repoCk{}, map[string]string{}
	cordonTick()
}

func (c *ctx) place() (string, string) {
	c.t.Helper()
	MWHY = map[string]string{}
	got := lpPlace(c.t, map[string]mLoad{"a": {L1: 9, Cores: 8, OK: true}, "b": {L1: 0.1, Cores: 8, OK: true}}) // 한가한 b가 먼저
	return got, MWHY["b"]
}

// ① 읽기 오류가 문턱을 넘김 → 격리·알림 한 번·새 레인은 a로 ② 문턱 전에는 격리 없음(b에 뜸)
func TestCordonAfterThreshold(t *testing.T) {
	c := ctFix(t)
	if got, _ := c.place(); got != "b" {
		t.Fatalf("격리 전: 한가한 b에: %q", got)
	}
	c.set("b", ctBad)
	c.tick(0)
	c.tick(3 * 60)
	if got, _ := c.place(); got != "b" || c.ev() != "" || cordonOn("b") {
		t.Fatalf("② 문턱(5분) 전: b에 뜨고 알림 없음: %q %q", got, c.ev())
	}
	if !strings.Contains(strings.Join(machineLines(true), "\n"), "저장소 읽기 막힘") {
		t.Fatalf("격리 전 기계 줄: %v", machineLines(true))
	}
	c.tick(3 * 60)
	got, why := c.place()
	if got != "a" || why != cordonWhy || !cordonOn("b") {
		t.Fatalf("① 격리된 b를 건너 a에: %q 까닭 %q", got, why)
	}
	e := c.ev()
	if strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "--name hc-fill-cordon") || !strings.Contains(e, "--key hf:cordon:b:") ||
		!strings.Contains(e, "interrupted system call") || !strings.Contains(e, "worker-mac-check") || !strings.Contains(e, "스스로 풂") {
		t.Fatalf("① 막힘 한 번(기계·오류 원문·사람이 볼 것): %q", e)
	}
	if l := strings.Join(machineLines(true), "\n"); !strings.Contains(l, "b  ") || !strings.Contains(l, "격리 6m째(p /r/b: open /r/b: interrupted system call)") {
		t.Fatalf("① 기계 줄 끝 격리: %s", l)
	}
	// ⑥ 같은 격리 동안 되풀이 없음 · 같은 바퀴 두 번(두 번째 실행)도 겹치지 않음
	c.tick(0)
	c.tick(30 * 60)
	if strings.Count(c.ev(), "report 막힘") != 1 {
		t.Fatalf("⑥ 되풀이: %q", c.ev())
	}
	// 감사 줄(자동 감사)도 그 기계에 안 뜬다 — 그 기계 작업 트리라
	if n, why := machRoom(Machine{Name: "b", Cap: "3", LCap: "-"}, true); n != 0 || why != cordonWhy {
		t.Fatalf("감사도 막힘: %d %q", n, why)
	}
}

// ③ 읽기가 상한 안에 안 끝남(점검 조각이 bad … 안 끝남) → 나쁨으로 셈
func TestCordonHangCountsBad(t *testing.T) {
	c := ctFix(t)
	c.set("b", ctHang)
	c.tick(0)
	c.tick(6 * 60)
	if !cordonOn("b") || !strings.Contains(cordonLoad()["b"].Why, "q /r/q b: 읽기가 5s 안에 안 끝남") {
		t.Fatalf("③ 안 끝남은 나쁨(빈칸 든 폴더 q): %+v", cordonLoad()["b"])
	}
}

// ④ 원격 호출 실패·잘린 출력·옛 판 워커(줄 없음)·폴더 수가 다름 → 모름: 격리 없음, 앞 상태 유지(풀지도 않음)
func TestCordonUnknownKeepsState(t *testing.T) {
	c := ctFix(t)
	unknowns := map[string]func(){
		"호출 실패":  func() { os.WriteFile(c.cd+"/b.rc", []byte("1"), 0o644) },
		"잘린 출력":  func() { c.set("b", "load=0.10 cpus=8 verifies=0\nrepo-read 1 bad open: x\nrepo-read 2 o") },
		"옛 판 워커": func() { os.Remove(c.cd + "/b") },
		"폴더 수":   func() { c.set("b", "repo-read 1 bad x\nrepo-read end 1") },
	}
	for name, f := range unknowns {
		os.Remove(c.cd + "/b.rc")
		c.set("b", ctGood)
		os.Remove(cordonFile())
		f()
		c.tick(0)
		c.tick(30 * 60)
		if cordonOn("b") || c.ev() != "" {
			t.Fatalf("④ %s: 모름으로 격리: %+v %q", name, cordonLoad()["b"], c.ev())
		}
	}
	// 격리 중 모름 → 그대로(풀지 않음)
	os.Remove(c.cd + "/b.rc")
	c.set("b", ctBad)
	os.Remove(cordonFile())
	c.tick(0)
	c.tick(6 * 60)
	os.WriteFile(c.cd+"/b.rc", []byte("1"), 0o644)
	c.tick(60)
	if !cordonOn("b") || strings.Count(c.ev(), "report 중간") != 0 {
		t.Fatalf("④ 격리 중 모름은 그대로: %+v %q", cordonLoad()["b"], c.ev())
	}
}

// ⑤ 격리 뒤 좋음 → 다음 바퀴에 풀림·회복 한 번·다시 b에 뜸 ⑥ 회복 알림이 실패하면 같은 키로 다음 바퀴에(격리는 이미 풂)
func TestCordonRecovers(t *testing.T) {
	c := ctFix(t)
	if err := os.WriteFile(Q, []byte(urQ("w1", "b", "t0")), 0o644); err != nil {
		t.Fatal(err)
	}
	ROWS = qRows()
	TERMS = []Term{{M: "b", Addr: "b/t5", Agent: "claude-code", Name: "w1", Status: "working", Cwd: "/w"}}
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	if e := c.ev(); !strings.Contains(e, "--to t0") || !strings.Contains(e, "떠 있는 레인 w1") {
		t.Fatalf("레인 주인에게(레인 이름과): %q", e)
	}
	c.set("b", ctGood)
	os.WriteFile(c.cd+"/report.rc", []byte("1"), 0o644)
	c.tick(60)
	if got, _ := c.place(); got != "b" || cordonOn("b") {
		t.Fatalf("⑤ 좋음 다음 바퀴에 풀림·b에 뜸(알림 실패와 상관없이): %q", got)
	}
	os.Remove(c.cd + "/report.rc")
	c.tick(0) // 회복 알림이 남은 기계는 간격 없이 다시 본다(때를 당기지 않는다 — 키의 나쁨 시작이 그대로여야)
	c.tick(0)
	e := c.ev()
	ups := strings.Count(e, "report 중간")
	if ups != 2 || !strings.Contains(e, "저장소 읽기 돌아옴") || strings.Count(e, "--key hf:cordon:b:") != 3 {
		t.Fatalf("⑤⑥ 회복: 실패 한 번 + 같은 키로 한 번 = 중간 2줄(같은 키), 그 뒤 없음: %q", e)
	}
	k := strings.Split(strings.Split(e, "report 중간")[1], "--key ")[1]
	k2 := strings.Split(strings.Split(e, "report 중간")[2], "--key ")[1]
	if k != k2 || !strings.Contains(k, ":up:t0") {
		t.Fatalf("⑥ 같은 키로 다시: %q %q", k, k2)
	}
}

// ⑦ 사람의 pause@와 따로: 격리가 풀려도 멈춤 그대로 · 격리 중 멈춤을 풀어도 격리 그대로
func TestCordonSeparateFromPause(t *testing.T) {
	c := ctFix(t)
	now := time.Now()
	if err := knobSet([]string{"pause@b", "1", "--why", "점검"}, now); err != nil {
		t.Fatal(err)
	}
	c.re()
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	if err := knobUnset([]string{"pause@b", "--why", "풂"}, now); err != nil {
		t.Fatal(err)
	}
	c.re()
	if got, why := c.place(); got != "a" || why != cordonWhy {
		t.Fatalf("⑦ 멈춤을 풀어도 격리 그대로: %q %q", got, why)
	}
	if err := knobSet([]string{"pause@b", "1", "--why", "다시"}, now); err != nil {
		t.Fatal(err)
	}
	c.re()
	c.set("b", ctGood)
	c.tick(60)
	if got, why := c.place(); got != "a" || why != "멈춤(조정)" || cordonOn("b") {
		t.Fatalf("⑦ 격리가 풀려도 멈춤 그대로: %q %q", got, why)
	}
}

// ⑨ 손잡이 0 → 격리하지 않고 알리기만(기본 문턱 5분에 한 번)
func TestCordonKnobZeroAlertsOnly(t *testing.T) {
	c := ctFix(t)
	confLines = append(confLines[:len(confLines)-1], "cordon 0")
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	c.tick(60)
	if got, _ := c.place(); got != "b" || cordonOn("b") {
		t.Fatalf("⑨ 0이면 격리 안 함: %q", got)
	}
	if e := c.ev(); strings.Count(e, "report 막힘") != 1 || !strings.Contains(e, "격리 안 함(손잡이 cordon 0") {
		t.Fatalf("⑨ 알리기만 한 번: %q", e)
	}
}

// ⑩ 격리 단위는 기계 — 한 프로젝트 폴더만 나빠도 그 기계의 다른 프로젝트 레인도 안 뜬다(문서의 결정 그대로)
func TestCordonMachineUnit(t *testing.T) {
	c := ctFix(t)
	c.set("b", ctHang) // q 폴더만 나쁨
	c.tick(0)
	c.tick(6 * 60)
	MWHY = map[string]string{}
	LOADS, NOW, dry = map[string]mLoad{"a": {L1: 9, Cores: 8, OK: true}, "b": {L1: 0.1, Cores: 8, OK: true}}, nil, true
	place("l1", "p", DATA+"/brief.md", "-", "-", "-", "")
	dry = false
	if got := strings.Join(NOW, ","); got != "a" || MWHY["b"] != cordonWhy {
		t.Fatalf("⑩ p 레인도 b에 안 뜸: %q %q", got, MWHY["b"])
	}
}

// 운영 장면 — 처음 돌 때: 기록 없음·옛 꼴 기록(칸 수 다름)·이미 나쁜 기계를 처음 봄 → 그 때부터 문턱을 센다(바로 격리하지 않음)
func TestCordonFirstRunAndOldRecord(t *testing.T) {
	c := ctFix(t)
	if err := os.WriteFile(cordonFile(), []byte("b\tbad\t1\t1\n"), 0o644); err != nil { // 옛 꼴(4칸) — 오래전 나쁨처럼 보여도 버린다
		t.Fatal(err)
	}
	c.set("b", ctBad)
	c.tick(0)
	if cordonOn("b") || c.ev() != "" {
		t.Fatalf("처음 본 나쁜 기계를 바로 격리: %+v", cordonLoad()["b"])
	}
	if x := cordonLoad()["b"]; x.St != "bad" || time.Now().Unix()-x.Since > 5 {
		t.Fatalf("처음 본 때부터 센다: %+v", x)
	}
}

// 좋은 기계는 따로 부르는 점검을 cordonEvery마다만 — 같은 바퀴에 이미 읽었으면(부하 조각) 부르지 않는다
func TestCordonProbeEvery(t *testing.T) {
	c := ctFix(t)
	c.tick(0)
	os.WriteFile(c.cd+"/b.rc", []byte("1"), 0o644) // 다시 부르면 실패 — 부르지 않았으면 기록이 그대로
	c.tick(30)
	if x := cordonLoad()["b"]; x.St != "ok" || time.Now().Unix()-x.At < 25 {
		t.Fatalf("간격 안에는 다시 부르지 않는다(앞 판정 때 그대로): %+v", x)
	}
	os.Remove(c.cd + "/b.rc")
	c.set("b", ctBad)
	REPOCK = map[string]repoCk{}
	loadTake("b", ctBad) // 이 바퀴 부하 조각(actTick·loadTick)이 이미 읽음
	cordonTick()
	if x := cordonLoad()["b"]; x.St != "bad" {
		t.Fatalf("같은 바퀴에 읽은 점검을 쓴다: %+v", x)
	}
}

// 병합기에: 격리 줄은 원격 손잡이 참조에 기한과 함께, 사람의 pause@는 싣지 않는다(병합 검증과 상관없음)
func TestCordonRemoteLine(t *testing.T) {
	c := ctFix(t)
	now := time.Now()
	if err := knobSet([]string{"pause@a", "1", "--why", "점검"}, now); err != nil {
		t.Fatal(err)
	}
	c.re()
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	msg := knob.FormatRemote(knobRemoteOf(knobEntries(), "p", now), now)
	r := knob.ParseRemote(msg, now)
	if r["cordon@b"] != "1" || strings.Contains(msg, "pause@") || r["cordon@a"] != "" {
		t.Fatalf("원격 줄: %q", msg)
	}
	if r := knob.ParseRemote(msg, now.Add(25*time.Minute)); r["cordon@b"] != "" {
		t.Fatalf("hc-fill이 안 돌면 기한이 지나 모름: %v", r)
	}
	if again := knob.FormatRemote(knobRemoteOf(knobEntries(), "p", now.Add(time.Minute)), now.Add(time.Minute)); again != msg {
		t.Fatal("바퀴마다 내용이 같아야(push가 늘지 않게)")
	}
	c.set("b", ctGood)
	c.tick(60)
	if msg := knob.FormatRemote(knobRemoteOf(knobEntries(), "p", now), now); strings.Contains(msg, "cordon@") {
		t.Fatalf("풀리면 원격 줄도 없음: %q", msg)
	}
}

// 상태 한 장·advise: 격리 칸과 항목 하나(사람이 정한다)
func TestCordonStateAdvise(t *testing.T) {
	c := ctFix(t)
	c.set("b", ctBad)
	c.tick(0)
	c.tick(6 * 60)
	s, _ := stateCollectF(time.Now())
	var got string
	for _, m := range s.Machines {
		if m.Name == "b" && m.Cordon != nil {
			got = m.Cordon.Why
		}
	}
	if !strings.Contains(got, "interrupted system call") {
		t.Fatalf("상태 한 장 cordon 칸: %+v", s.Machines)
	}
	var it []string
	for _, i := range adviseNow(time.Now()).Items {
		if i.Kind == "cordon" {
			it = append(it, i.Key+" "+i.What+" "+i.Sure)
		}
	}
	if len(it) != 1 || !strings.Contains(it[0], "cordon:b b 격리 6분째") || !strings.Contains(it[0], "사람이 정한다") {
		t.Fatalf("advise 항목 하나: %v", it)
	}
}
