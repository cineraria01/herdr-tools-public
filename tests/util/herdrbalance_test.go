package util_test

// 옛 tests/herdr-balance.test.sh: balance.remote 읽기(설정 있음·파일 없음·키 없음·도우미 없음), 가짜 herdr-app(PATH 앞, 상태는 임시 JSON)로
// 한 탭 맞춤(--add·--drop·HUD 안 건드림·포커스 되돌림·상태 파일)·오류 꼴·--watch 한 바퀴. 실제 herdr-app·설정은 쓰지 않는다.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 가짜 herdr-app: ls·status·do(terminal.park·pane.focus·terminal.keep). keep은 포커스된 칸 바로 뒤에 새 칸으로 넣는다.
// 부른 인자는 한 줄씩 ha.log. FAKE_HA_FAIL=1이면 종료 3, do 인자 JSON의 terminal이 err_on과 같으면 {"error": "nope"}
const fakeHerdrApp = `#!/usr/bin/env python3
import json, os, sys
a = sys.argv[1:]
with open(os.environ["FAKE_HA_LOG"], "a") as f:
    f.write(" ".join(a) + "\n")
if os.environ.get("FAKE_HA_FAIL"):
    print("boom", file=sys.stderr); sys.exit(3)
p = os.environ["FAKE_HA_STATE"]; st = json.load(open(p))
def find(key, val):
    for tab in st["tabs"]:
        for i, x in enumerate(tab["terminals"]):
            if x[key] == val:
                return tab, i
    return None, None
if a[0] == "ls":
    print(json.dumps({"spaces": [{"name": "s", "tabs": st["tabs"]}]}))
elif a[0] == "status":
    print(json.dumps({"focused_pane": st["focused"]}))
elif a[0] == "do":
    kw = json.loads(a[2])
    if kw.get("terminal") and kw.get("terminal") == st.get("err_on"):
        print(json.dumps({"error": "nope"})); sys.exit(0)
    if a[1] == "terminal.park":
        tab, i = find("terminal", kw["terminal"]); st["parked"][kw["terminal"]] = tab["terminals"].pop(i)
    elif a[1] == "pane.focus":
        st["focused"] = kw["pane"] if "pane" in kw else find("terminal", kw["terminal"])[0]["terminals"][find("terminal", kw["terminal"])[1]]["pane"]
    elif a[1] == "terminal.keep":
        x = st["parked"].pop(kw["terminal"], None) or {"terminal": kw["terminal"], "name": None, "app": None}
        st["n"] += 1; x["pane"] = "n%d" % st["n"]
        tab, i = find("pane", st["focused"]); tab["terminals"].insert(i + 1, x)
    print("{}")
    json.dump(st, open(p, "w"))
`

func TestHerdrBalance(t *testing.T) { eachImpl(t, "herdr-balance", testHerdrBalance) }

func testHerdrBalance(t *testing.T, T string, e Env) {
	mkdirs(t, T+"/bin", T+"/home", T+"/conf", T+"/py", T+"/fake")
	os.Symlink(filepath.Join(repo, "bin", "herdr-tools-config"), T+"/bin/herdr-tools-config")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/py/python3") // 셸 판이 부를 python3(PATH를 좁혀도 — 도우미를 찾는 길은 그대로 시험)
	HB := filepath.Join(repo, "bin", "herdr-balance")
	iv := implVar("herdr-balance")
	impl := e[iv]
	// hbEnv: env -i 꼴 — 바깥 환경 없이 HOME·PATH·설정·가짜 상태와 판 스위치만
	hbEnv := func(path, conf string, kv ...string) Env {
		return Env{"HOME": T + "/home", "PATH": path, "HERDR_TOOLS_CONFIG": conf, "FAKE_HA_STATE": T + "/state.json",
			"FAKE_HA_LOG": T + "/ha.log", iv: impl}.With(kv...)
	}
	var errOut string
	hb := func(path, conf string, kv []string, args ...string) (string, int) {
		out, se, rc := run2(T, hbEnv(path, conf, kv...), HB, args...)
		errOut = se
		return out, rc
	}
	// remote <PATH> <설정 폴더>: remote_prefixes()의 결과를 「a|b」로, 경고는 errOut
	remote := func(path, conf string) string { out, _ := hb(T+"/py:"+path, conf, nil, "--print-remote"); return out }
	eq := func(name, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("FAIL - %s: [%s] != [%s] err=[%s]", name, got, want, errOut)
		}
	}
	ex := filepath.Join(repo, "config.example")
	bin := T + "/bin:/usr/bin:/bin"

	eq("예시 설정: 목록", remote(bin, ex), "worker-a/|worker-b/")
	eq("예시 설정: 경고 없음", errOut, "")
	eq("설정 파일 없음: 빈 목록", remote(bin, T+"/conf"), "")
	eq("설정 파일 없음: 경고 한 줄", strconv.Itoa(grepc(errOut, "설정 파일 없음")), "1")
	write(t, T+"/conf/config", "sync.prebuild true\n")
	eq("키만 없음: 빈 목록", remote(bin, T+"/conf"), "")
	eq("키만 없음: 경고 없음", errOut, "")
	write(t, T+"/conf/config", "balance.remote a/\nbalance.remote b/\n")
	eq("같은 키 두 번: 빈 목록", remote(bin, T+"/conf"), "")
	eq("같은 키 두 번: 경고 한 줄", strconv.Itoa(grepc(errOut, "balance.remote 못 읽음")), "1")
	eq("도우미가 PATH·~/.local/bin에 없음: 빈 목록", remote("/usr/bin:/bin", ex), "")
	eq("도우미 없음: 경고 한 줄", strconv.Itoa(grepc(errOut, "herdr-tools-config 못 부름")), "1")
	mkdirs(t, T+"/home/.local/bin")
	os.Symlink(filepath.Join(repo, "bin", "herdr-tools-config"), T+"/home/.local/bin/herdr-tools-config")
	eq("PATH에 없어도 ~/.local/bin의 도우미(launchd)", remote("/usr/bin:/bin", ex), "worker-a/|worker-b/")

	script(t, T+"/fake/herdr-app", T, fakeHerdrApp)
	P := T + "/fake:" + T + "/py:" + T + "/bin:/usr/bin:/bin"
	term := func(tm, pane, name, app string) string {
		return fmt.Sprintf(`{"terminal": "%s", "pane": "%s", "name": %s, "app": %s}`, tm, pane, name, app)
	}
	state := func(focused, errOn, tab, terms string) {
		write(t, T+"/state.json", fmt.Sprintf(`{"n": 0, "focused": "%s", "err_on": "%s", "parked": {"t9": %s}, "tabs": [{"id": "%s", "terminals": [%s]}]}`+"\n",
			focused, errOn, term("t9", "-", `"lane-d"`, "null"), tab, terms))
	}
	TABS := strings.Join([]string{term("t0", "p0", `"lead"`, `"claude"`), term("t1", "p1", `"lane-a"`, `"claude"`), term("t2", "p2", "null", "null"),
		term("t3", "p3", `"lane-b"`, "null"), term("t4", "p4", `"lane-c"`, "null")}, ", ")
	state("p2", "", "w9/tab1", TABS)
	write(t, T+"/ha.log", "")
	out, rc := hb(P, ex, nil, "--main", "t0", "--add", "t9", "--drop", "t3")
	eq("맞춤: 출력(한글 그대로·순서)", fmt.Sprintf("%d %s", rc, out), `0 {"tab": "w9/tab1", "main": "t0", "workers": ["t1", "t4", "t9"]}`)
	eq("맞춤: 상태 파일(/는 __)", strings.TrimRight(read(T+"/home/.local/state/herdr-balance/w9__tab1.json"), "\n"), `["t1", "t4", "t9"]`)
	var st struct {
		Tabs []struct{ Terminals []struct{ Terminal string } }
	}
	json.Unmarshal([]byte(read(T+"/state.json")), &st)
	var order []string
	if len(st.Tabs) > 0 {
		for _, x := range st.Tabs[0].Terminals {
			order = append(order, x.Terminal)
		}
	}
	eq("맞춤: 칸 순서(가짜는 포커스 칸 뒤에 넣음 — t0 다음 t1·t4·t9, HUD t2는 안 뗌)", strings.Join(order, " "), "t0 t1 t4 t9 t2")
	var dos []string
	for _, l := range strings.Split(read(T+"/ha.log"), "\n") {
		if strings.HasPrefix(l, "do") {
			dos = append(dos, l)
		}
	}
	eq("맞춤: herdr-app 부른 순서(park 다음 반씩 keep, 끝에 보던 칸으로)", strings.Join(dos, "\n"), `do terminal.park {"terminal": "t1"}
do terminal.park {"terminal": "t4"}
do terminal.park {"terminal": "t3"}
do pane.focus {"pane": "p0"}
do terminal.keep {"terminal": "t1", "place": "right"}
do pane.focus {"pane": "n1"}
do terminal.keep {"terminal": "t4", "place": "down"}
do pane.focus {"pane": "n2"}
do terminal.keep {"terminal": "t9", "place": "down"}
do pane.focus {"terminal": "t2"}`)
	eq("맞춤: stderr 없음", errOut, "")
	state("p0", "", "tab1", TABS)
	out, rc = hb(P, ex, nil, "--main", "t404")
	eq("화면에 없는 지휘: 종료 1·문구", fmt.Sprintf("%d %s|%s", rc, out, strings.TrimRight(errOut, "\n")), "1 |t404이(가) 화면의 어느 탭에도 없음")
	_, rc = hb(P, ex, []string{"FAKE_HA_FAIL=1"}, "--main", "t0")
	eq("herdr-app 실패: 종료 1·출력을 붙인 문구(그 stderr 줄바꿈까지)", fmt.Sprintf("%d|%s.", rc, errOut), "1|herdr-app ls 실패: boom\n\n.")
	state("p0", "t4", "tab1", TABS)
	_, rc = hb(P, ex, nil, "--main", "t0")
	eq("do 오류: 종료 1·문구", fmt.Sprintf("%d|%s", rc, strings.TrimRight(errOut, "\n")), "1|terminal.park 실패: nope")
	_, rc = hb(P, ex, nil)
	eq("인자 없음: usage·종료 2", fmt.Sprintf("%d|%s", rc, strings.TrimRight(errOut, "\n")), `2|usage: herdr-balance [-h] [--main MAIN] [--add [ADD ...]] [--drop [DROP ...]]
                     [--watch]
herdr-balance: error: --main 또는 --watch가 필요`)

	// --watch: 처음 보는 탭은 지금 배치를 적고, 이름 붙은 worker가 늘어 4초 그대로면 맞춘다. TERM이면 끝난다
	os.RemoveAll(T + "/home/.local/state")
	state("p0", "", "tab1", term("t0", "p0", `"lead"`, `"claude"`)+", "+term("t1", "p1", `"lane-a"`, "null"))
	wout, werr := T+"/watch.out", T+"/watch.err"
	wo, _ := os.Create(wout)
	we, _ := os.Create(werr)
	w := exec.Command(HB, "--watch") // 포장 → 판이 모두 exec라 w.Process가 watch 그 프로세스
	w.Dir, w.Env, w.Stdout, w.Stderr = T, hbEnv(P, ex).list(), wo, we
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	wo.Close()
	we.Close()
	done := make(chan error, 1)
	go func() { done <- w.Wait() }()
	t.Cleanup(func() {
		w.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	until := func(n int, f func() bool) {
		for i := 0; i < n && !f(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	TAB1 := T + "/home/.local/state/herdr-balance/tab1.json"
	until(100, func() bool { return exists(TAB1) })
	errOut = read(werr)
	eq("watch: 처음 본 탭은 지금 배치를 기준으로", strings.TrimRight(read(TAB1), "\n"), `["t1"]`)
	if _, rc := run(T, e, PY, "-c", `import json,sys; p=sys.argv[1]; s=json.load(open(p)); s["tabs"][0]["terminals"].append({"terminal": "t5", "pane": "p5", "name": "lane-e", "app": None}); json.dump(s, open(p + ".new", "w"))`, T+"/state.json"); rc == 0 {
		os.Rename(T+"/state.json.new", T+"/state.json")
	}
	until(200, func() bool { return strings.Contains(read(wout), "맞춤") })
	errOut = read(werr)
	ts := regexp.MustCompile(`(?m)^[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} `)
	eq("watch: 바뀐 목록이 그대로면 맞춤 한 줄", strings.TrimRight(ts.ReplaceAllString(read(wout), ""), "\n"), `맞춤 {"tab": "tab1", "main": "t0", "workers": ["t1", "t5"]}`)
	w.Process.Signal(syscall.SIGTERM)
	var werrv error
	ended := "돈다"
	select {
	case werrv = <-done:
		ended = "끝"
		done <- werrv // Cleanup이 다시 받게
	case <-time.After(5 * time.Second):
	}
	eq("watch: TERM이면 곧 끝남", ended, "끝")
	wrc := 0
	var ee *exec.ExitError
	if errors.As(werrv, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			wrc = 128 + int(ws.Signal())
		} else {
			wrc = ee.ExitCode()
		}
	}
	// Go 판은 회차를 마치고 0, 셸 판은 TERM으로 죽는다(143) — 의도된 차이
	if impl == "bash" {
		eq("watch: 셸 판 TERM 종료 코드", strconv.Itoa(wrc), "143")
	} else {
		eq("watch: Go 판은 TERM에 0으로 끝남", strconv.Itoa(wrc), "0")
	}
}
