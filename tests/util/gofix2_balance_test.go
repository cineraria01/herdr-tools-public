package util_test

// gofix2 재감사(아스트라 1바퀴) 가운데 herdr-balance(R-07·R-26)·ovis-split(R-08) 지적의 계약 시험 — Go 판만(셸 판은 곧 걷는다).
// 이름 앞 R-ID는 보고서 지적 ID. 가짜 herdr-app(fakeHerdrApp)·가짜 hc·임시 HOME만 쓴다.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// b2Go: 시험 폴더·격리 환경
func b2Go(t *testing.T) (string, Env) {
	T := tmpdir(t)
	return T, isolated(T)
}

// b2Balance: 가짜 herdr-app 앞에 포장 — B2_LS_GONE_FROM..B2_LS_GONE_TO번째 ls는 공간이 빠진 목록({"spaces": []}),
// B2_PARK_HANG=1이면 terminal.park을 마친 뒤 park.done을 적고, 시험이 hold를 지울 때까지(최대 60초) 멈췄다가 park.exit을 적고 끝난다
// (그 사이 KILL하려고). 멈춘 herdr-app은 시험이 번호로 끄지 않고 스스로 끝난다(재감사 base R-213 — 파일의 PID에 신호 금지)
func b2Balance(t *testing.T) (T string, env func(kv ...string) Env, HB string) {
	T, _ = b2Go(t)
	mkdirs(t, T+"/bin", T+"/home", T+"/py", T+"/fake")
	HB = tool(t, "herdr-balance")
	os.Symlink(filepath.Join(filepath.Dir(HB), "herdr-tools-config"), T+"/bin/herdr-tools-config")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/py/python3")
	script(t, T+"/fake/herdr-app.py", T, fakeHerdrApp)
	script(t, T+"/fake/herdr-app", T, `#!/bin/bash
if [ "$1" = ls ]; then
  n=$(( $(cat "@T@/lsn" 2>/dev/null || echo 0) + 1 )); echo $n >"@T@/lsn"
  if [ -n "$B2_LS_GONE_FROM" ] && [ "$n" -ge "$B2_LS_GONE_FROM" ] && [ "$n" -le "${B2_LS_GONE_TO:-999999}" ]; then
    echo '{"spaces": []}'; exit 0
  fi
fi
if [ "$1 $2" = "do terminal.park" ] && [ -n "$B2_PARK_HANG" ]; then
  "@T@/fake/herdr-app.py" "$@" || exit
  : >"@T@/park.done"; i=0
  while [ -e "@T@/hold" ] && [ $i -lt 600 ]; do sleep 0.1; i=$((i+1)); done
  : >"@T@/park.exit"; exit 0
fi
exec "@T@/fake/herdr-app.py" "$@"
`)
	t.Cleanup(func() { os.Remove(T + "/hold") }) // 멈춘 가짜 herdr-app이 스스로 끝나게
	P := T + "/fake:" + T + "/py:" + T + "/bin:/usr/bin:/bin"
	env = func(kv ...string) Env {
		return Env{"HOME": T + "/home", "PATH": P, "HERDR_TOOLS_CONFIG": filepath.Join(repo, "config.example"),
			"FAKE_HA_STATE": T + "/state.json", "FAKE_HA_LOG": T + "/ha.log"}.With(kv...)
	}
	return T, env, HB
}

// b2HaState: 가짜 herdr-app 상태(탭 하나)를 새로 깔고 ls 횟수를 되돌린다
func b2HaState(t *testing.T, T, tabID, terms string) {
	write(t, T+"/state.json", `{"n": 0, "focused": "p0", "err_on": "", "parked": {}, "tabs": [{"id": "`+tabID+`", "terminals": [`+terms+`]}]}`+"\n")
	write(t, T+"/ha.log", "")
	os.Remove(T + "/lsn")
}

type b2HaSt struct {
	Parked map[string]any `json:"parked"`
	Tabs   []struct {
		ID        string `json:"id"`
		Terminals []struct {
			Terminal string `json:"terminal"`
		} `json:"terminals"`
	} `json:"tabs"`
}

func b2ReadHa(t *testing.T, T string) b2HaSt {
	t.Helper()
	var s b2HaSt
	if err := json.Unmarshal([]byte(read(T+"/state.json")), &s); err != nil {
		t.Fatalf("가짜 herdr-app 상태를 못 읽음: %v", err)
	}
	return s
}

func (s b2HaSt) inTab(tabID, term string) bool {
	for _, tb := range s.Tabs {
		if tb.ID == tabID {
			for _, x := range tb.Terminals {
				if x.Terminal == term {
					return true
				}
			}
		}
	}
	return false
}

// R-07: 원격 탭을 맞추다 세 번째 ls부터 그 공간이 목록에서 빠지면(연결 끊김) 떼야 할 worker를 「이미 없음」으로 읽고 park 0회·종료 0·
// 상태 []로 끝났다. 원래 탭·기계가 보일 때만 결과를 확정하고, 끊기면 직전 상태 파일을 그대로 두어야 한다.
func TestB2R07BalanceRemoteSpaceGone(t *testing.T) {
	T, env, HB := b2Balance(t)
	const tabID = "remote/tab"
	stf := T + "/home/.local/state/herdr-balance/remote__tab.json"
	terms := `{"terminal": "remote/t0", "pane": "p0", "name": "lead", "app": "claude"}, {"terminal": "remote/t1", "pane": "p1", "name": "lane-a", "app": "claude"}`
	const prev = `["remote/t1"]`
	for _, c := range []struct{ name, to string }{
		{"끊긴 채", ""},   // 세 번째 ls부터 계속 공간 없음 — 확정할 수 없으니 실패·상태 보존
		{"잠깐 끊김", "3"}, // 세 번째 ls만 공간 없음 — 다시 보고 떼거나(성공이면 실제로 떼임), 실패면 상태 보존
	} {
		t.Run(c.name, func(t *testing.T) {
			b2HaState(t, T, tabID, terms)
			write(t, stf, prev)
			kv := []string{"B2_LS_GONE_FROM=3"}
			if c.to != "" {
				kv = append(kv, "B2_LS_GONE_TO="+c.to)
			}
			out, se, rc := run2(T, env(kv...), HB, "--main", "remote/t0", "--drop", "remote/t1")
			st := b2ReadHa(t, T)
			_, parked := st.Parked["remote/t1"]
			got := read(stf)
			parks := grepc(read(T+"/ha.log"), "^do terminal.park ")
			if c.to == "" && rc == 0 {
				t.Errorf("FAIL - [R-07] 공간이 끊긴 채면 맞춤을 확정할 수 없다 — 실패여야 (rc=0 park %d회 상태 %q out=%q)", parks, got, out)
			}
			if rc == 0 && (!parked || st.inTab(tabID, "remote/t1") || got != "[]") {
				t.Errorf("FAIL - [R-07] 성공이면 --drop한 worker가 실제로 떼여 있어야 (park %d회 떼임=%v 상태 %q)", parks, parked, got)
			}
			if rc != 0 && got != prev {
				t.Errorf("FAIL - [R-07] 실패면 직전 상태 파일을 그대로 (rc=%d 상태 %q err=%q)", rc, got, se)
			}
		})
	}
}

// R-26: worker t1을 park한 직후 그 실행을 KILL하면 t1이 떼인 채 남았고, 다시 돌려도 종료 0·상태 []·t1은 떼인 채였다.
// park 전에 목표 배치를 적어 두고, 다음 실행이 그 실행이 끝났음을 본 뒤 미완료 배치(t1)를 되붙여야 한다.
func TestB2R26ResumesAfterKill(t *testing.T) {
	T, env, HB := b2Balance(t)
	stf := T + "/home/.local/state/herdr-balance/tab1.json"
	b2HaState(t, T, "tab1", `{"terminal": "t0", "pane": "p0", "name": "lead", "app": "claude"}, {"terminal": "t1", "pane": "p1", "name": "lane-a", "app": "claude"}`)
	write(t, T+"/hold", "")
	cmd := exec.Command(HB, "--main", "t0")
	cmd.Dir, cmd.Env = T, env("B2_PARK_HANG=1").list()
	ch := clitest.Spawn(t, cmd)                         // 시험이 소유 — Cleanup·마감에 무리째 거둔다
	clitest.WaitFile(t, T+"/park.done", 60*time.Second) // 첫 실행은 Go 판 빌드까지 — 넉넉히
	// park 직후 KILL — 그 실행의 무리(시험이 띄운 것). 멈춰 있는 herdr-app(runLim이 따로 띄운 무리)은 hold를 지워 스스로 끝낸다
	ch.Kill()
	os.Remove(T + "/hold")
	clitest.WaitFile(t, T+"/park.exit", 10*time.Second)
	select {
	case <-ch.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("KILL한 실행이 끝나지 않음")
	}
	if st := b2ReadHa(t, T); st.Parked["t1"] == nil || st.inTab("tab1", "t1") {
		t.Fatalf("t1이 떼인 채여야 시험이 된다: %s", read(T+"/state.json"))
	}

	out, se, rc := run2(T, env(), HB, "--main", "t0")
	st := b2ReadHa(t, T)
	_, still := st.Parked["t1"]
	if rc != 0 || still || !st.inTab("tab1", "t1") || read(stf) != `["t1"]` {
		t.Errorf("FAIL - [R-26] 다시 돌리면 KILL로 떼인 채 남은 t1을 되붙여야 (rc=%d 떼인 채=%v 상태 %q out=%q err=%q)",
			rc, still, read(stf), out, se)
	}
}

// b2FakeOvisHc: ovis-split용 가짜 hc — B2_LS·B2_LSL이 있으면 「ls --json」·「ls --json --label …」 응답을 그 글자로
const b2FakeOvisHc = `#!/bin/bash
printf '%s\n' "$*" >>"$FAKE_HC_LOG"
case "$*" in
  "get --json t9") echo '{"address": "t9", "cwd": "/r/proj", "labels": {}}' ;;
  "get --json t2") echo '{"address": "t2", "parent": "t9", "name": "lane-b"}' ;;
  "ls --json") if [ -n "${B2_LS+x}" ]; then printf '%s\n' "$B2_LS"; else
      echo '{"terminals": [{"address": "t9", "cwd": "/r/proj", "state": "live"}, {"address": "t2", "parent": "t9", "name": "lane-b", "state": "live"}]}'; fi ;;
  "ls --json --label ovis.workspace=proj") if [ -n "${B2_LSL+x}" ]; then printf '%s\n' "$B2_LSL"; else echo '{"terminals": []}'; fi ;;
  label\ *) ;;
  *) echo "모르는 hc 인자: $*" >&2; exit 9 ;;
esac
`

// R-08: 두 번 받는 hc ls 목록에 null 원소·배열 아님·주소·상태 없는 원소가 있어도 Go 판은 그 원소를 건너뛰고 라벨을 붙였다
// (셸 판은 1·라벨 0회). 목록의 배열·원소·주소·상태를 다 본 뒤에만 라벨을 붙이고, 아니면 종료 1·라벨 0회여야 한다.
func TestB2R08OvisSplitBadList(t *testing.T) {
	T, _ := b2Go(t)
	OS := tool(t, "ovis-split")
	mkdirs(t, T+"/bin", T+"/home")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/bin/python3")
	script(t, T+"/bin/hc", T, b2FakeOvisHc)
	LOG := T + "/hc.log"
	osp := func(kv ...string) (string, int) {
		write(t, LOG, "")
		env := Env{"HOME": T + "/home", "PATH": T + "/bin:/usr/bin:/bin", "FAKE_HC_LOG": LOG}.With(kv...)
		out, se, rc := run2(T, env, OS, "--main", "t9", "--add", "t2")
		return out + se, rc
	}
	if out, rc := osp(); rc != 0 || grepc(read(LOG), "^label") != 2 {
		t.Fatalf("정상 입력은 라벨 둘이어야 시험이 된다 rc=%d %s", rc, out)
	}
	for _, c := range []struct{ what, kv string }{
		{"ls 원소 null", `B2_LS={"terminals": [null]}`},
		{"ls 정상 원소 뒤 null", `B2_LS={"terminals": [{"address": "t9", "cwd": "/r/proj", "state": "live"}, null]}`},
		{"ls terminals 없음", `B2_LS={}`},
		{"ls terminals가 배열 아님", `B2_LS={"terminals": {}}`},
		{"ls 원소에 state 없음", `B2_LS={"terminals": [{"address": "t2", "parent": "t9", "name": "lane-b"}]}`},
		{"ls 원소 state가 문자열 아님", `B2_LS={"terminals": [{"address": "t2", "state": 1}]}`},
		{"ls 살아 있지 않은 원소에 address 없음", `B2_LS={"terminals": [{"state": "exited"}]}`},
		{"ls --label 원소 null", `B2_LSL={"terminals": [null]}`},
		{"ls --label terminals 없음", `B2_LSL={}`},
	} {
		out, rc := osp(c.kv)
		if n := grepc(read(LOG), "^label"); rc != 1 || n != 0 {
			t.Errorf("FAIL - [R-08] %s: 종료 1·라벨 0회여야 함 (rc=%d 라벨 %d)\n%s", c.what, rc, n, out)
		}
	}
}
