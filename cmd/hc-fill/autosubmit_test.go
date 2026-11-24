//go:build !windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 대기열이 제출(fill-auto-submit): 가짜 hc-lane은 진짜 표시 규칙을 흉내 낸다 — land는 표시가 없으면 시작 표시를 만들고(대기열이면
// 다섯째 칸 fill), 에이전트 표시(다섯째 칸 없음)가 있으면 손대지 않고 종료 2. 가짜 hc는 send를 적고, exec는 그대로 돌린다
// (ASUB 줄이 든 exec는 HC_T_EXEC_RC가 있으면 그 종료로 — 원격 모름)
const asTLane = `#!/bin/sh
echo "$1 $* JOB=$HC_LANE_JOB TERM=$HC_TERMINAL_ID BY=$HC_LANE_LAND_BY PRIO=$HC_LANE_PRIORITY OWNER=$HC_LANE_OWNER ORUN=$HC_LANE_OWNER_RUN HLP=$HERDR_LAND_PRIORITY BR=$LANE_BRANCH X=$HC_T_NOTLANE LEASE=$HC_LANE_LEASE LANE=$HC_LANE PWD=$(pwd -P)" >>"$HC_T_LOG"
d=$HC_FILL_VERDICTS/$HC_LANE
case $1 in
verdict) echo "감사 통과 1번째 $2 — sha. 착지: 이 작업 트리에서 hc-lane land main --verdict $2"; exit 0 ;;
land)
	x=$d/$4.ack
	[ ! -f "$HC_T_DIR/agent-first" ] || printf 'landing\tT\tw1@1700000000000\tpid\n' >"$x"
	if [ ! -f "$x" ]; then
		j=${HC_LANE_JOB:-w1@1700000000000} # 없으면 레인처럼 <터미널>@<hc get created_ms>
		if [ "$HC_LANE_LAND_BY" = fill ]; then printf 'landing\tT\t%s\tpid\tfill\n' "$j" >"$x"; else printf 'landing\tT\t%s\tpid\n' "$j" >"$x"; fi
	elif [ "$(head -n 1 "$x" | cut -s -f 5)" != fill ]; then echo "hc-lane land: 판정 $4 착지를 레인 에이전트가 시작했다" >&2; exit 2
	fi
	if [ -f "$HC_T_DIR/land-rc" ]; then echo "hc-lane land: rebase 충돌 — x.go" >&2; exit "$(cat "$HC_T_DIR/land-rc")"; fi
	echo "hc-lane land: 제출 abc → refs/merge-queue/1-w1 — 이 레인은 여기서 끝"; exit 0 ;;
report) exit 0 ;;
esac
exit 9
`

const asTHc = `#!/bin/sh
case $1 in
send) echo "send $2 $3" >>"$HC_T_LOG"; exit 0 ;;
get)
	[ -z "$HC_T_GETFAIL" ] || exit 1
	printf '{"address":"%s","state":"live","status":"idle","cwd":"/","name":"w1","created_ms":%s,"argv":%s}\n' "$2" "${HC_T_CREATED:-1700000000000}" "$HC_T_ARGV"; exit 0 ;;
exec)
	case "$*" in *ASUB*) [ -z "$HC_T_EXEC_RC" ] || { echo "exec-asub" >>"$HC_T_LOG"; exit "$HC_T_EXEC_RC"; } ;; esac
	while [ "$1" != -- ]; do shift; done; shift; exec "$@" ;;
esac
exit 9
`

type subT struct {
	t              *testing.T
	dir, log, vdir string
}

// asTArgv: 레인 터미널 argv(hc-lane 띄우기 꼴 — env 앞머리의 레인 값 뒤 명령). HC_T_NOTLANE은 레인 값이 아니라 넘기지 않는다
const asTArgv = `["env","HC_LANE_PRIORITY=긴급","HERDR_LAND_PRIORITY=3","HC_LANE_OWNER=c/coord","HC_LANE_OWNER_RUN=1699","HC_LANE=w1","LANE_BRANCH=w1","HC_T_NOTLANE=x","claude","HC_LANE_X=after-cmd"]`

// asTSetup: 레인 w1(기계 m 또는 원격 r, 주소 <기계>/w1, 실행 w1@1700000000000), 통과 판정 a1, 손잡이 auto_submit = on
func asTSetup(t *testing.T, machine, status, on string) *subT {
	t.Helper()
	knobFix(t)
	t.Setenv("HOME", HOME)
	k := &subT{t: t, dir: t.TempDir()}
	k.log = k.dir + "/log"
	addr := machine + "/w1"
	Q, AT, LOCAL, TERMS, AUNK = DATA+"/queue.tsv", DATA+"/audit.tsv", "m", []Term{{M: machine, Addr: addr, Name: "w1", Status: status}}, nil
	repo := k.dir + "/app"
	if err := os.MkdirAll(repo+".wt/w1", 0o755); err != nil {
		t.Fatal(err)
	}
	confLines = []string{"repo app " + machine + " " + repo}
	if on != "" {
		confLines = append(confLines, "auto_submit "+on)
	}
	k.vdir = HOME + "/.local/share/hc-fill/verdicts/w1"
	if err := os.MkdirAll(k.vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, s := range map[string]string{"hc-lane": asTLane, "hc": asTHc} {
		if err := os.WriteFile(k.dir+"/"+n, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", k.dir+":"+os.Getenv("PATH"))
	t.Setenv("HC_T_LOG", k.log)
	t.Setenv("HC_T_DIR", k.dir)
	t.Setenv("HC_T_EXEC_RC", "")
	t.Setenv("HC_T_GETFAIL", "")
	t.Setenv("HC_T_CREATED", "")
	t.Setenv("HC_T_ARGV", asTArgv)
	t.Setenv("HC_T_NOTLANE", "")
	q := []string{"w1", "app", "b", "-", machine, "up", machine, addr, "-", "t", "-", "c/coord", "urgent", "-", "auto", "-", "r1"}
	at := []string{"a1", "w1", "1", "w1@1700000000000", strings.Repeat("ab", 20), machine, addr, "deliver", "0", "pass", "0", "0", "-", "-", "0", "-", "-"}
	wr(t, Q, strings.Join(q, "\t")+"\n")
	wr(t, AT, strings.Join(at, "\t")+"\n")
	if err := os.WriteFile(DATA+"/audit-proj.tsv", []byte("a1\tapp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := arow("a1")
	auAid, auLn, auRnd, auPrun, auSha, auAm, auPa, auAst = r.F(1), r.F(2), r.F(3), r.F(4), r.F(5), r.F(6), r.F(7), r.F(8)
	auAfx, auAv, auAwk, auAwt, auAlc, auAlw, auAnv, auAfl = r.F(9), r.F(10), r.F(11), r.F(12), r.F(13), r.F(14), r.F(15), r.F(16)
	return k
}

func wr(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (k *subT) logged() string {
	b, _ := os.ReadFile(k.log)
	return string(b)
}

func (k *subT) ack() string {
	b, _ := os.ReadFile(k.vdir + "/a1.ack")
	return string(b)
}

// ① 통과 + 쉬는 레인 → 대기열이 레인 환경으로 verdict·land·report를 돌려 제출, 판정 깨우기 0번(정리 알림 한 번), status 표시
func TestAutoSubmitIdleLane(t *testing.T) {
	k := asTSetup(t, "m", "idle", "other,app")
	t.Setenv("HC_LANE_LEASE", "hc-fill-env") // hc-fill 환경에 남은 레인 값은 레인 것이 아니다 — 지운다
	if !aAutoSubmit("main") {
		t.Fatalf("제출했으면 이 바퀴는 끝(참): %s", k.logged())
	}
	l := k.logged()
	for _, w := range []string{"verdict verdict a1 ", "land land main --verdict a1 ", "report report 착지 abababab ",
		"JOB= TERM=w1 BY=fill PRIO=긴급 OWNER=c/coord ORUN=1699 HLP=3 BR=w1 X= LEASE= LANE=w1 PWD=" + realpath(t, k.dir+"/app.wt/w1")} {
		if !strings.Contains(l, w) {
			t.Fatalf("레인이 부를 줄을 레인 환경으로 돌리지 않았다(%q 없음):\n%s", w, l)
		}
	}
	if strings.Contains(l, "감사 판정이 왔다") || strings.Count(l, "send ") != 1 || !strings.Contains(l, "대신 제출했다") {
		t.Fatalf("판정 깨우기 없이 정리 알림 한 번이어야 한다:\n%s", l)
	}
	r := arow("a1")
	if r.F(8) != "taken" || r.F(11) != "0" || r.F(16) != "asub,asubok" {
		t.Fatalf("감사 기록: 상태 %s 깨움 %s 표시 %s", r.F(8), r.F(11), r.F(16))
	}
	if !strings.HasSuffix(strings.TrimSpace(k.ack()), "\tfill") {
		t.Fatalf("시작 표시는 대기열 것: %q", k.ack())
	}
	if a := anote("w1"); a != asubAud {
		t.Fatalf("status 비고: %q", a)
	}
	if s := stStage(stIn{State: "up", Pos: 2, Aud: asubAud}); s != "제출됨(병합기 줄 2번째) · 대기열이 제출함" {
		t.Fatalf("status 단계: %q", s)
	}
}

// ② 에이전트가 턴 중 → 손대지 않는다(지금처럼 깨우기만 — 넘김 글 없음)
func TestAutoSubmitBusyLane(t *testing.T) {
	for _, st := range []string{"working", "blocked"} {
		k := asTSetup(t, "m", st, "app")
		if aAutoSubmit("main") || asNote != "" || k.logged() != "" || arow("a1").F(16) != "-" {
			t.Fatalf("%s: 손댔다 — 글 %q 기록 %q 표시 %s", st, asNote, k.logged(), arow("a1").F(16))
		}
	}
}

// ③ 충돌(land 종료 3) → 대기열 시작 표시를 에이전트에 넘기고(fill 칸 지움) 무엇을 했고 어디서 멈췄는지 한 줄과 함께 깨운다
func TestAutoSubmitConflictHandsOff(t *testing.T) {
	k := asTSetup(t, "m", "done", "app")
	wr(t, k.dir+"/land-rc", "3")
	if aAutoSubmit("main") {
		t.Fatal("넘겼으면 거짓(지금처럼 깨운다)")
	}
	if !strings.Contains(asNote, "land 종료 3") || !strings.Contains(asNote, "rebase 충돌") {
		t.Fatalf("멈춘 자리 한 줄: %q", asNote)
	}
	if a := k.ack(); strings.Contains(a, "fill") || !strings.HasPrefix(a, "landing\tT\tw1@1700000000000\tpid\n") {
		t.Fatalf("표시를 에이전트 것으로 넘기지 않았다: %q", a)
	}
	if r := arow("a1"); r.F(8) != "deliver" || r.F(16) != "asub" {
		t.Fatalf("넘긴 차수는 그대로 수거 기다림 + 다시 시도 안 함 표시: %s %s", r.F(8), r.F(16))
	}
	if aAutoSubmit("main") || strings.Count(k.logged(), "land land") != 1 {
		t.Fatalf("한 차수에 대기열 시도는 한 번: %s", k.logged())
	}
}

// ④ 에이전트가 먼저 수거(시작 표시를 먼저 만듦) → 대기열은 제출하지 않고 깨우지도 넘기지도 않는다(이중 제출 0)
func TestAutoSubmitAgentFirst(t *testing.T) {
	k := asTSetup(t, "m", "idle", "app")
	wr(t, k.dir+"/agent-first", "")
	if !aAutoSubmit("main") || asNote != "" {
		t.Fatalf("에이전트 것이면 손대지 않고 끝(참): %q", asNote)
	}
	if a := k.ack(); a != "landing\tT\tw1@1700000000000\tpid\n" {
		t.Fatalf("에이전트 표시를 건드렸다: %q", a)
	}
	if l := k.logged(); strings.Contains(l, "제출") || strings.Contains(l, "send ") || strings.Contains(l, "report ") {
		t.Fatalf("이중 제출·알림: %s", l)
	}
	if r := arow("a1"); r.F(8) != "deliver" {
		t.Fatalf("수거는 다음 바퀴 표시 읽기(landing → taken)가 닫는다: %s", r.F(8))
	}
}

// ⑤ 원격 실행 모름(hc exec 종료 124) → 성공으로 치지 않고 넘긴다
func TestAutoSubmitRemoteUnknown(t *testing.T) {
	k := asTSetup(t, "r", "idle", "app")
	CONN = []string{"r"}
	t.Setenv("HC_T_EXEC_RC", "124")
	if aAutoSubmit("main") {
		t.Fatal("모름은 넘김(거짓)")
	}
	if !strings.Contains(asNote, "결과 모름") || arow("a1").F(8) != "deliver" || strings.Contains(arow("a1").F(16), "asubok") {
		t.Fatalf("모름을 성공으로 쳤다: %q %v", asNote, arow("a1"))
	}
	if !strings.Contains(k.logged(), "exec-asub") {
		t.Fatalf("원격으로 부르지 않았다: %s", k.logged())
	}
}

// ⑥ 끔(기본, 또는 다른 프로젝트만) → 지금 그대로
func TestAutoSubmitOff(t *testing.T) {
	for _, on := range []string{"", "-", "other"} {
		k := asTSetup(t, "m", "idle", on)
		if aAutoSubmit("main") || asNote != "" || k.logged() != "" || arow("a1").F(16) != "-" {
			t.Fatalf("끔(%q)인데 손댔다: %q %q", on, asNote, k.logged())
		}
	}
}

// 판정 파일 두기(s_vput)가 대기열 시작 표시를 「landing+fill」로, 에이전트 표시를 「landing」으로 알리고, s_ahand가 fill 칸만 지운다
func TestAutoSubmitMarkScripts(t *testing.T) {
	k := asTSetup(t, "m", "idle", "app")
	vb := "aid\ta1\n"
	sum := sha256.Sum256([]byte(vb))
	put := func() string {
		o, rc := onm(strings.NewReader(vb), "m", sVput, "~/.local/share/hc-fill/verdicts/w1", "a1", hex.EncodeToString(sum[:]), strconv.Itoa(len(vb)))
		if rc != 0 {
			t.Fatalf("s_vput 종료 %d", rc)
		}
		return sub(o)
	}
	for _, c := range []struct{ ack, want, hand string }{
		{"", "none", "NONE"},
		{"landing\tT\tw1@1\tpid\tfill\n", "landing+fill", "HANDED"},
		{"landing\tT\tw1@1\tpid\n", "landing", "KEEP"},
		{"taken\tT\tw1@1\n", "taken", "KEEP"},
	} {
		os.Remove(k.vdir + "/a1.ack")
		if c.ack != "" {
			wr(t, k.vdir+"/a1.ack", c.ack)
		}
		if o := put(); !strings.HasPrefix(o, "WROTE "+c.want+" ") && !strings.HasPrefix(o, "ALREADY "+c.want+" ") {
			t.Fatalf("표시 %q → %q, 기대 %s", c.ack, o, c.want)
		}
		o, rc := onm(nil, "m", sAhand, "~/.local/share/hc-fill/verdicts/w1", "a1")
		if rc != 0 || sub(o) != c.hand {
			t.Fatalf("넘기기 %q → %q(%d), 기대 %s", c.ack, o, rc, c.hand)
		}
	}
	if a := k.ack(); a != "taken\tT\tw1@1\n" {
		t.Fatalf("에이전트 표시를 바꿨다: %q", a)
	}
	wr(t, k.vdir+"/a1.ack", "landing\tT\tw1@1\tpid\tfill\n")
	onm(nil, "m", sAhand, "~/.local/share/hc-fill/verdicts/w1", "a1")
	if a := k.ack(); a != "landing\tT\tw1@1\tpid\n" {
		t.Fatalf("넘긴 표시: %q", a)
	}
	if es, _ := filepath.Glob(k.vdir + "/.hand.*"); len(es) != 0 {
		t.Fatalf("임시 파일이 남았다: %v", es)
	}
}

// 감사 지적 1(atozg4m-aooh8y): 원격 레인의 성공 길 — 레인 기계에서 HC_TERMINAL_ID는 레인 자신의 값(기계 접두 없음).
// 고치기 전에는 hc-fill이 본 주소 「r/w1」를 그대로 넘겨, 워커 기계의 hc get이 「unknown machine」으로 실행을 못 읽고 verdict가 종료 75였다
func TestAutoSubmitRemoteTerm(t *testing.T) {
	k := asTSetup(t, "r", "idle", "app")
	CONN = []string{"r"}
	if !aAutoSubmit("main") {
		t.Fatalf("원격 레인도 제출: %q\n%s", asNote, k.logged())
	}
	if l := k.logged(); !strings.Contains(l, " TERM=w1 ") || strings.Contains(l, "TERM=r/") {
		t.Fatalf("레인 기계에 기계 접두 있는 주소를 넘겼다:\n%s", l)
	}
}

// 감사 지적 2: 레인 환경은 레인 터미널 argv의 env 앞머리 그대로 — 레인 값만, 명령 뒤는 안 봄, HC_LANE_JOB은 있었을 때만
func TestAutoSubmitEnvOf(t *testing.T) {
	got := strings.Join(asEnvOf([]string{"env", "HC_LANE_JOB=j-1", "HC_LANE_LAND_BY=fill", "LANE_BASE=main", "PATH=/x", "HERDR_LAND_PRIORITY=2", "-u", "HC_LANE_Y=1", "claude"}), " ")
	if got != "HC_LANE_JOB=j-1 LANE_BASE=main HERDR_LAND_PRIORITY=2" {
		t.Fatalf("asEnvOf = %q", got)
	}
	if e := asEnvOf([]string{"claude", "HC_LANE=x"}); e != nil {
		t.Fatalf("env 앞머리가 없으면 넘길 것 없음: %q", e)
	}
	// 레인에 HC_LANE_JOB이 있었으면 그 값이 실행이고 그대로 넘긴다
	k := asTSetup(t, "m", "idle", "app")
	t.Setenv("HC_T_ARGV", `["env","HC_LANE_JOB=job-7","HC_LANE=w1","claude"]`)
	auPrun = "job-7"
	if !aAutoSubmit("main") || !strings.Contains(k.logged(), " JOB=job-7 ") {
		t.Fatalf("레인의 HC_LANE_JOB을 넘기지 않았다: %q\n%s", asNote, k.logged())
	}
}

// 레인 터미널을 못 읽거나 실행이 다르면(주소 재사용) 모르는 것 — 돌리지 않고 시도 표시도 남기지 않는다(지금처럼 깨운다)
func TestAutoSubmitEnvUnknown(t *testing.T) {
	for _, c := range []struct{ name, key, val string }{
		{"hc get 실패", "HC_T_GETFAIL", "1"},
		{"다른 실행", "HC_T_CREATED", "1800000000000"},
		{"argv 없음", "HC_T_ARGV", "null"},
	} {
		k := asTSetup(t, "m", "idle", "app")
		t.Setenv(c.key, c.val)
		if aAutoSubmit("main") || asNote != "" || k.logged() != "" || arow("a1").F(16) != "-" {
			t.Fatalf("%s: 돌렸다 — %q %q %s", c.name, asNote, k.logged(), arow("a1").F(16))
		}
	}
}

// 감사 지적 3: 그 판정의 hc-lane land가 아직 돌면(시간 넘김·hc-fill이 죽은 뒤 남은 land) 넘기지 않고 다음 바퀴에 — 에이전트 land와 겹치지 않게
func TestAutoSubmitHandBusy(t *testing.T) {
	k := asTSetup(t, "m", "idle", "app")
	bd := k.dir + "/busy"
	if err := os.MkdirAll(bd, 0o755); err != nil {
		t.Fatal(err)
	}
	wr(t, bd+"/hc-lane", "#!/bin/sh\nsleep 30\n")
	if err := os.Chmod(bd+"/hc-lane", 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bd+"/hc-lane", "land", "main", "--verdict", "a1-busy")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill(); c.Wait() }) // 띄운 그 pid만
	auAid = "a1-busy"
	wr(t, k.vdir+"/a1-busy.ack", "landing\tT\tw1@1\tpid\tfill\n")
	if asHand() {
		t.Fatal("돌고 있는 land가 있는데 넘겼다")
	}
	if b, _ := os.ReadFile(k.vdir + "/a1-busy.ack"); !strings.HasSuffix(string(b), "\tfill\n") {
		t.Fatalf("표시를 바꿨다: %q", b)
	}
	c.Process.Kill()
	c.Wait()
	if !asHand() {
		t.Fatal("land가 끝났으면 넘긴다")
	}
}

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
