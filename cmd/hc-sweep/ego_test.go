package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const egoUp = "/Applications/ego lite.app/Contents/MacOS/ego lite --restart\n/Applications/ego lite.app/Contents/Frameworks/x/ego Helper (Node).app/Contents/MacOS/ego Helper (Node) --type=utility\n"

// fakeEgo: ego-browser 대역 — ps 출력, 지금 공간 목록, 닫기 답(closed면 목록에서 뺌)
type fakeEgo struct {
	ps       string
	spaces   []map[string]any
	list     *result // 목록 답을 바꿔 끼움
	closeAns string  // closed(기본)·kept·changed·hang
	calls    []string
}

func egoSp(id int, name, by, own string, titles ...string) map[string]any {
	if titles == nil {
		titles = []string{"새 탭"}
	}
	return map[string]any{"createdBy": by, "id": id, "name": name, "ownership": own, "profileId": "Default", "profileName": "p", "recentTabTitles": titles, "taskId": name}
}

func (f *fakeEgo) run(_ time.Duration, _ []string, name string, args ...string) result {
	if name == "/bin/ps" {
		f.calls = append(f.calls, "ps")
		return result{out: f.ps}
	}
	js := args[len(args)-1]
	if js == egoListJS {
		f.calls = append(f.calls, "list")
		if f.list != nil {
			return *f.list
		}
		b, _ := json.Marshal(f.spaces)
		return result{out: "[ego-browser:notice] x\n" + egoMark + string(b) + "\n"}
	}
	j, _, _ := strings.Cut(strings.TrimPrefix(js, "const w = "), ";\n")
	var w struct{ ID int }
	if json.Unmarshal([]byte(j), &w) != nil {
		return result{rc: 1}
	}
	f.calls = append(f.calls, "close:"+strconv.Itoa(w.ID))
	ans := f.closeAns
	if ans == "" {
		ans = "closed"
	}
	if ans == "hang" {
		return result{rc: -1, err: errors.New("ego-browser: 시간 초과")}
	}
	if ans == "closed" {
		for i, s := range f.spaces {
			if s["id"] == w.ID {
				f.spaces = append(f.spaces[:i], f.spaces[i+1:]...)
				break
			}
		}
	}
	return result{out: egoMark + ans + "\n"}
}

type egoT struct {
	t     *testing.T
	f     *fakeEgo
	state string
	t0    time.Time
	sent  []string
}

func newEgoT(t *testing.T, spaces ...map[string]any) *egoT {
	return &egoT{t: t, f: &fakeEgo{ps: egoUp, spaces: spaces}, state: filepath.Join(t.TempDir(), "hc-sweep", "ego-spaces.json"), t0: time.Unix(1_800_000_000, 0)}
}

// at: 처음부터 m분 뒤 한 바퀴(문턱 120분)
func (e *egoT) at(m int, dry bool) []string {
	return egoCheck("ego", "box-1", 120*time.Minute, e.state, dry, e.f.run, func(msg, key string) bool {
		e.sent = append(e.sent, msg)
		return true
	}, e.t0.Add(time.Duration(m)*time.Minute))
}

func (e *egoT) closes() int {
	n := 0
	for _, c := range e.f.calls {
		if strings.HasPrefix(c, "close:") {
			n++
		}
	}
	return n
}

func TestEgoBeforeThresholdKeeps(t *testing.T) { // ① · 이미 쌓인 상태에 처음 돎
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"), egoSp(2, "b", "agent", "agent"))
	for _, m := range []int{0, 60, 119} {
		e.at(m, false)
	}
	if e.closes() != 0 || len(e.sent) != 0 {
		t.Fatalf("문턱 전인데 닫음: %v %v", e.f.calls, e.sent)
	}
}

func TestEgoPastThresholdClosesOnce(t *testing.T) { // ② · 두 번째 실행
	e := newEgoT(t, egoSp(0, "", "user", "user"), egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	e.at(61, false)
	lines := e.at(122, false)
	e.at(122, false)
	e.at(123, false)
	if e.closes() != 1 || len(e.sent) != 1 || !strings.Contains(e.sent[0], "a (#1, 122분 그대로)") || !strings.Contains(e.sent[0], "box-1") {
		t.Fatalf("닫기·사건 한 번이 아님: %v %v %v", e.f.calls, e.sent, lines)
	}
	if rec := egoLoad(e.state); len(rec) != 0 {
		t.Fatalf("닫은 것이 기록에 남음: %v", rec)
	}
	if len(e.f.spaces) != 1 || e.f.spaces[0]["id"] != 0 {
		t.Fatalf("사용자 공간이 사라짐: %v", e.f.spaces)
	}
}

func TestEgoFingerprintChangeResets(t *testing.T) { // ③ · 공간 id가 다시 쓰임
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	e.f.spaces[0] = egoSp(1, "a", "agent", "agent", "다른 탭")
	e.at(61, false)
	e.at(122, false)
	if e.closes() != 0 {
		t.Fatalf("탭이 바뀌었는데 닫음: %v", e.f.calls)
	}
	e.f.spaces[0] = egoSp(1, "b", "agent", "agent", "다른 탭") // 같은 id에 다른 공간
	e.at(181, false)
	e.at(240, false)
	if e.closes() != 0 {
		t.Fatalf("id가 다시 쓰였는데 닫음: %v", e.f.calls)
	}
	e.at(302, false)
	if e.closes() != 1 {
		t.Fatalf("바뀐 뒤 문턱을 넘겼는데 안 닫음: %v", e.f.calls)
	}
}

func TestEgoNeverTouchesOthers(t *testing.T) { // ④ · 이름이 빔
	e := newEgoT(t, egoSp(0, "", "user", "user"), egoSp(1, "a", "agent", "agentDelegatedToUser"), egoSp(2, "b", "agent", "user"),
		egoSp(3, "c", "agent", "someNewValue"), egoSp(4, "d", "someNewValue", "agent"), egoSp(5, "", "agent", "agent"))
	for m := 0; m <= 600; m += 30 {
		e.at(m, false)
	}
	if e.closes() != 0 || len(e.f.spaces) != 6 {
		t.Fatalf("남의 공간을 닫음: %v", e.f.calls)
	}
}

func TestEgoNoServiceNoCall(t *testing.T) { // ⑤
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.f.ps = "/usr/sbin/cron\n/Applications/ego lite.app/Contents/Frameworks/x/ego Helper (Node).app/Contents/MacOS/ego Helper (Node)\n"
	for _, m := range []int{0, 61, 122} {
		e.at(m, false)
	}
	if strings.Join(e.f.calls, ",") != "ps,ps,ps" {
		t.Fatalf("ego가 안 떠 있는데 불렀다(앱을 띄운다): %v", e.f.calls)
	}
	called := false
	never := func(time.Duration, []string, string, ...string) result { called = true; return result{} }
	t.Setenv("HC_SWEEP_EGO", "")
	t.Setenv("PATH", t.TempDir())
	egoRun(false, never, 120*time.Minute) // 명령 없음
	t.Setenv("HC_SWEEP_EGO", "/bin/echo")
	egoRun(false, never, 0) // ⑨ 손잡이 0 — 목록도 안 읽음
	if called {
		t.Fatal("명령 없음·손잡이 0인데 불렀다")
	}
}

func TestEgoUnknownKeepsRecord(t *testing.T) { // ⑥ · 판이 달라 칸이 빠짐
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	before, err := os.ReadFile(e.state)
	if err != nil {
		t.Fatal(err)
	}
	noTitles := `[{"createdBy":"agent","id":1,"name":"a","ownership":"agent","profileId":"Default","taskId":"a"}]`
	for _, r := range []result{{rc: -1, err: errors.New("ego-browser: 시간 초과")}, {out: "깨진 출력"}, {out: egoMark + "{not json"},
		{out: egoMark + noTitles}, {rc: 1, out: egoMark + "[]"}} {
		r := r
		e.f.list = &r
		lines := e.at(200, false)
		if len(lines) != 1 || !strings.Contains(lines[0], "모름") {
			t.Fatalf("모름이 아님 %+v: %v", r, lines)
		}
		if after, _ := os.ReadFile(e.state); string(after) != string(before) {
			t.Fatalf("모름인데 기록이 바뀜 %+v", r)
		}
	}
	if e.closes() != 0 {
		t.Fatal(e.f.calls)
	}
	e.f.ps = ""
	psFail := func(_ time.Duration, _ []string, name string, _ ...string) result { return result{rc: 1} }
	if l := egoCheck("ego", "box-1", time.Hour, e.state, false, psFail, nil, e.t0); len(l) != 1 || !strings.Contains(l[0], "모름") {
		t.Fatal(l)
	}
}

func TestEgoBusySkipsClose(t *testing.T) { // ⑦
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	e.at(61, false)
	e.f.ps = egoUp + "ego-browser nodejs -e x\n"
	lines := e.at(122, false)
	if e.closes() != 0 || len(lines) != 1 || !strings.Contains(lines[0], "도는 중") {
		t.Fatalf("ego-browser가 도는데 닫음: %v %v", e.f.calls, lines)
	}
	e.f.ps = egoUp
	e.at(130, false)
	if e.closes() != 1 {
		t.Fatalf("명령이 끝난 뒤 안 닫음: %v", e.f.calls)
	}
}

func TestEgoKeptNoRetry(t *testing.T) { // ⑧
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.f.closeAns = "kept"
	e.at(0, false)
	e.at(61, false)
	lines := e.at(122, false)
	e.at(150, false)
	e.at(180, false)
	if e.closes() != 1 || len(e.sent) != 0 || len(lines) != 1 || !strings.Contains(lines[0], "지켜짐") {
		t.Fatalf("지켜짐을 되풀이: %v %v %v", e.f.calls, e.sent, lines)
	}
}

func TestEgoChangedAndHang(t *testing.T) { // 닫기 직전 바뀜 · 닫기가 막힘(남은 것은 안 부름)
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"), egoSp(2, "b", "agent", "agent"))
	e.f.closeAns = "changed"
	e.at(0, false)
	e.at(61, false)
	e.at(122, false)
	if len(e.sent) != 0 || e.closes() != 2 {
		t.Fatalf("%v %v", e.f.calls, e.sent)
	}
	e.at(123, false) // changed 뒤는 처음부터
	if e.closes() != 2 {
		t.Fatalf("바뀐 뒤 바로 닫음: %v", e.f.calls)
	}
	e.f.closeAns = "hang"
	e.at(180, false)
	lines := e.at(244, false)
	if e.closes() != 3 || len(lines) != 1 || !strings.Contains(lines[0], "닫기 모름") {
		t.Fatalf("막힌 뒤에도 더 부름: %v %v", e.f.calls, lines)
	}
}

func TestEgoLongGapRestarts(t *testing.T) { // 바퀴 순서 반대(한참 안 돌다 돎)
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	e.at(500, false)
	e.at(501, false)
	if e.closes() != 0 {
		t.Fatalf("못 본 사이를 그대로로 셈: %v", e.f.calls)
	}
}

func TestEgoDryRunOnlySays(t *testing.T) { // ⑩
	e := newEgoT(t, egoSp(1, "a", "agent", "agent"))
	e.at(0, false)
	e.at(61, false)
	before, _ := os.ReadFile(e.state)
	lines := e.at(122, true)
	after, _ := os.ReadFile(e.state)
	if e.closes() != 0 || len(e.sent) != 0 || len(lines) != 1 || !strings.Contains(lines[0], "닫을 것") || string(before) != string(after) {
		t.Fatalf("dry-run이 바꿈: %v %v %v", e.f.calls, e.sent, lines)
	}
}

// 가짜 ego-browser 실행 파일을 진짜로 띄움(run) — 표식 줄·인자 꼴이 실제 호출에서도 맞는지
func TestEgoFakeBinary(t *testing.T) {
	d := t.TempDir()
	log := filepath.Join(d, "calls")
	bin := filepath.Join(d, "ego-browser")
	// 진짜 ego-browser(ego lite 0.6.3)는 console.log를 표준 오류로 낸다 — 표준 출력만 읽으면 늘 모름이었다
	script := "#!/bin/sh\nprintf '%s\\n' \"$1 $2\" >> '" + log + "'\ncase \"$3\" in\n*finish*) echo 'HCSWEEP closed' >&2 ;;\n" +
		"*listTaskSpaces*) echo 'HCSWEEP [{\"createdBy\":\"agent\",\"id\":3,\"name\":\"x\",\"ownership\":\"agent\",\"profileId\":\"D\",\"profileName\":\"p\",\"recentTabTitles\":[],\"taskId\":\"x\"}]' >&2 ;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := func(d time.Duration, env []string, name string, args ...string) result {
		if name == "/bin/ps" {
			return result{out: egoUp}
		}
		return run(d, env, name, args...)
	}
	state := filepath.Join(d, "s.json")
	var sent []string
	t0 := time.Unix(1_800_000_000, 0)
	for _, m := range []int{0, 1, 2} {
		egoCheck(bin, "box-1", time.Minute, state, false, r, func(msg, _ string) bool { sent = append(sent, msg); return true }, t0.Add(time.Duration(m)*time.Minute))
	}
	b, _ := os.ReadFile(log)
	if strings.Count(string(b), "nodejs -e") != 4 || len(sent) != 1 || !strings.Contains(sent[0], "x (#3") {
		t.Fatalf("호출 %q 사건 %v", b, sent)
	}
}
