// herdr-balance: Herdr 탭의 오른쪽 worker 열을 고른 높이로 다시 붙인다(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」).
// 계약은 셸 판(lib/herdr-balance/herdr-balance.py)과 같다 — 인자·출력 JSON 꼴(한글 그대로)·종료 코드·상태 파일
// ~/.local/state/herdr-balance/<탭>.json·herdr-app에 넘기는 인자 글자. 바뀐 것: herdr-app 응답이 JSON이 아니거나 상태 파일을
// 못 쓰면 셸 판은 예외로 죽었고(--watch도 멈춤) Go 판은 그 회차 오류 한 줄로 친다, --watch는 TERM·INT를 받으면 그 회차를 마치고 0으로
// 끝난다(맞추던 중에 죽어 worker가 떼인 채 남지 않게), 상태 파일은 반쪽 없이 바꿔 넣는다.
//
//	herdr-balance --main <지휘 터미널> [--add tX ...] [--drop tX ...]   한 탭을 지금 맞춤
//	herdr-balance --watch                                              자동 맞춤(launchd)
//
// worker = 지휘 터미널과 같은 탭에서 이름(hc --name)이 붙은 다른 터미널 + --add. 이름 없는 터미널(하단 HUD 등)은 건드리지 않는다.
// worker를 terminal.park로 화면에서만 뗐다가(프로세스 유지) 반씩 나누는 순서로 terminal.keep 해서, 높이가 최대 2배 차이 안으로
// 고르게 된다(2^k개면 똑같음). ponytail: Herdr에 분할 비율 API가 없어 이분 분할로 근사, 비율 API가 생기면 그걸로.
// 위→아래 순서는 지금 순서를 지키고 --add는 맨 아래에 붙는다. 끝나면 맞추기 전에 보던 터미널로 포커스를 되돌린다.
// --watch: 2초마다 화면을 보고, 이 기계 탭과 설정 balance.remote(빈칸으로 나눈 원격 탭 이름 앞부분, 예 `w1/ w2/`, 없으면 원격 탭은
// 맞추지 않음 — herdr-tools-config) 탭 가운데 첫 터미널이 에이전트(지휘)이고 이름 붙은 worker 목록이 바뀌어 4초 동안 그대로면
// 그 탭을 맞춘다. 마지막으로 맞춘 목록은 ~/.local/state/herdr-balance/<탭>.json에 남겨 같은 목록이면 다시 하지 않는다.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"herdr-tools/internal/argp"
	"herdr-tools/internal/atomic"
	"herdr-tools/internal/pyjson"
)

var parser = &argp.Parser{
	Prog: "herdr-balance",
	Usage: "usage: herdr-balance [-h] [--main MAIN] [--add [ADD ...]] [--drop [DROP ...]]\n" +
		"                     [--watch]\n",
	Help: "usage: herdr-balance [-h] [--main MAIN] [--add [ADD ...]] [--drop [DROP ...]]\n" +
		"                     [--watch]\n\noptions:\n" +
		"  -h, --help         show this help message and exit\n  --main MAIN\n  --add [ADD ...]\n  --drop [DROP ...]\n  --watch\n",
	Opts: []argp.Opt{{Name: "main", Kind: argp.One}, {Name: "add", Kind: argp.Star}, {Name: "drop", Kind: argp.Star},
		{Name: "watch", Kind: argp.Flag}, {Name: "print-remote", Kind: argp.Flag}}, // --print-remote: 시험용(숨김)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("herdr-balance 1")
		return
	}
	a, _ := parser.Parse(os.Args[1:])
	if a.Has["print-remote"] {
		fmt.Println(strings.Join(remotePrefixes(), "|"))
		return
	}
	if a.Has["watch"] {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		watch(ctx)
		return
	}
	if a.Get("main") == "" {
		parser.Error("--main 또는 --watch가 필요")
	}
	r, err := balance(a.Get("main"), a.Vals["add"], a.Vals["drop"])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(pyjson.Dumps(r, false))
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// remotePrefixes: balance.remote — 이 창에서 맞춰 줄 다른 기계 탭 이름 앞부분. 나머지 원격 탭은 건드리지 않는다.
// launchd PATH에는 ~/.local/bin이 없을 수 있어 거기도 본다. 못 읽거나 설정 파일이 없으면 빈 목록 + 경고 한 줄(키만 없으면 조용히)
func remotePrefixes() []string {
	cmd, err := exec.LookPath("herdr-tools-config")
	if err != nil {
		cmd = filepath.Join(home(), ".local/bin/herdr-tools-config")
	}
	out, errs, rc, err := run10(cmd, "get", "balance.remote", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-balance: herdr-tools-config 못 부름(%v) — 원격 탭은 맞추지 않음\n", err)
		return nil
	}
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "herdr-balance: balance.remote 못 읽음(%s) — 원격 탭은 맞추지 않음\n", strings.TrimSpace(errs))
		return nil
	}
	remote := strings.Fields(out)
	if len(remote) == 0 {
		if _, _, rc, err := run10(cmd, "file", "config"); err != nil || rc != 0 {
			fmt.Fprintln(os.Stderr, "herdr-balance: 설정 파일 없음(herdr-tools-config file config) — 원격 탭은 맞추지 않음")
		}
	}
	return remote
}

// run10: 10초 안에 끝나야 하는 도우미. err는 못 띄움·시간 초과, rc는 종료 코드
func run10(name string, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var o, e bytes.Buffer
	c := exec.CommandContext(ctx, name, args...) // Stdin nil = /dev/null
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	if ctx.Err() != nil {
		return "", "", 0, fmt.Errorf("시간 초과 10초: %s", name)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return o.String(), e.String(), ee.ExitCode(), nil
	}
	if err != nil {
		return "", "", 0, err
	}
	return o.String(), e.String(), 0, nil
}

type term struct {
	Terminal string `json:"terminal"`
	Pane     any    `json:"pane"`
	Name     any    `json:"name"`
	App      any    `json:"app"`
}

type tab struct {
	ID        string `json:"id"`
	Terminals []term `json:"terminals"`
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return fmt.Sprint(v) != "0"
}

// appOut: herdr-app 부르기. 종료 코드가 0이 아니면 셸 판과 같은 문구로 실패(출력을 그대로 붙임)
func appOut(args ...string) ([]byte, error) {
	var o, e bytes.Buffer
	c := exec.Command("herdr-app", args...)
	c.Stdout, c.Stderr = &o, &e
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil, fmt.Errorf("herdr-app %s 실패: %v", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("herdr-app %s 실패: %s%s", strings.Join(args, " "), o.String(), e.String())
	}
	return o.Bytes(), nil
}

// appJSON: herdr-app 응답을 v로. 셸 판은 「{」로 시작할 때만 JSON으로 읽었다 — 아니면 obj=false
func appJSON(v any, args ...string) (obj bool, err error) {
	out, err := appOut(args...)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "{") {
		return false, nil
	}
	if err := pyjson.Decode(out, v); err != nil {
		return false, fmt.Errorf("herdr-app %s 응답을 못 읽음: %v", strings.Join(args, " "), err)
	}
	return true, nil
}

func tabs() ([]tab, error) {
	var ls struct {
		Spaces []struct {
			Tabs []tab `json:"tabs"`
		} `json:"spaces"`
	}
	obj, err := appJSON(&ls, "ls")
	if err != nil {
		return nil, err
	}
	if !obj {
		return nil, errors.New("herdr-app ls 응답이 JSON 객체가 아님")
	}
	var out []tab
	for _, s := range ls.Spaces {
		out = append(out, s.Tabs...)
	}
	return out, nil
}

// where: term이 있는 탭과 그 터미널(없으면 nil)
func where(t string) (*tab, *term, error) {
	ts, err := tabs()
	if err != nil {
		return nil, nil, err
	}
	for i := range ts {
		for j := range ts[i].Terminals {
			if ts[i].Terminals[j].Terminal == t {
				return &ts[i], &ts[i].Terminals[j], nil
			}
		}
	}
	return nil, nil, nil
}

const pollEvery, waitLimit = 200 * time.Millisecond, 10 * time.Second

func waitFor(cond func() (bool, error), what string) error {
	end := time.Now().Add(waitLimit)
	for time.Now().Before(end) {
		ok, err := cond()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(pollEvery)
	}
	return fmt.Errorf("시간 초과: %s", what)
}

// do: herdr-app do <명령> <JSON> — 인자 JSON은 파이썬 json.dumps 기본 꼴(셸 판과 같은 글자)
func do(cmd string, kw pyjson.Obj) error {
	var r map[string]any
	obj, err := appJSON(&r, "do", cmd, pyjson.Dumps(kw, true))
	if err != nil {
		return err
	}
	if obj && truthy(r["error"]) {
		e, ok := r["error"].(string)
		if !ok {
			e = pyjson.Dumps(r["error"], false)
		}
		return fmt.Errorf("%s 실패: %s", cmd, e)
	}
	return nil
}

func workersOf(t tab, main string) []string {
	var out []string
	for _, x := range t.Terminals {
		if x.Terminal != main && truthy(x.Name) {
			out = append(out, x.Terminal)
		}
	}
	return out
}

func statePath(tabID string) string {
	return filepath.Join(home(), ".local/state/herdr-balance", strings.ReplaceAll(tabID, "/", "__")+".json")
}

func save(tabID string, workers []string) error {
	p := statePath(tabID)
	if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
		return fmt.Errorf("상태 폴더 만들기: %w", err)
	}
	if workers == nil {
		workers = []string{}
	}
	if err := atomic.WriteFile(p, []byte(pyjson.Dumps(workers, true)), 0o644); err != nil {
		return fmt.Errorf("상태 저장: %w", err)
	}
	return nil
}

// saved: 마지막으로 맞춘 목록. 없거나 못 읽으면 ok=false
func saved(tabID string) ([]string, bool) {
	b, err := os.ReadFile(statePath(tabID))
	if err != nil {
		return nil, false
	}
	var ws []string
	if err := pyjson.Decode(b, &ws); err != nil {
		return nil, false
	}
	return ws, true
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// build: group[0]은 이미 pane에 있다. 아래 절반을 group[0] 바로 밑에 붙이고 양쪽을 다시 나눈다. keep(터미널, 기준 칸, 자리) → 새 칸
func build(group []string, pane any, keep func(string, any, string) (any, error)) error {
	if len(group) < 2 {
		return nil
	}
	mid := len(group) / 2
	lower, err := keep(group[mid], pane, "down")
	if err != nil {
		return err
	}
	if err := build(group[:mid], pane, keep); err != nil {
		return err
	}
	return build(group[mid:], lower, keep)
}

func balance(main string, add, drop []string) (pyjson.Obj, error) {
	t, m, err := where(main)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("%s이(가) 화면의 어느 탭에도 없음", main)
	}
	tabID, mainPane := t.ID, m.Pane
	var workers []string
	for _, w := range workersOf(*t, main) {
		if !contains(drop, w) {
			workers = append(workers, w)
		}
	}
	base := append([]string(nil), workers...)
	for _, a := range add { // 셸 판처럼 맞춤 전 목록에 없는 것만(--add 안의 겹침은 그대로)
		if !contains(base, a) {
			workers = append(workers, a)
		}
	}

	// 맞춘 뒤 돌아갈 곳: 지금 포커스된 pane의 터미널(pane id는 park/keep 뒤 바뀐다)
	var st map[string]any
	if obj, err := appJSON(&st, "status"); err != nil {
		return nil, err
	} else if !obj {
		return nil, errors.New("herdr-app status 응답이 JSON 객체가 아님")
	}
	ts, err := tabs()
	if err != nil {
		return nil, err
	}
	back := ""
	for _, x := range ts {
		for _, y := range x.Terminals {
			if back == "" && reflect.DeepEqual(y.Pane, st["focused_pane"]) {
				back = y.Terminal
			}
		}
	}

	for _, w := range append(append([]string(nil), workers...), drop...) {
		wt, _, err := where(w)
		if err != nil {
			return nil, err
		}
		if wt == nil {
			continue
		}
		if err := do("terminal.park", pyjson.Obj{{K: "terminal", V: w}}); err != nil {
			return nil, err
		}
		if err := waitFor(func() (bool, error) { x, _, err := where(w); return x == nil, err }, w+" park"); err != nil {
			return nil, err
		}
	}

	keep := func(tm string, anchor any, place string) (any, error) {
		if err := do("pane.focus", pyjson.Obj{{K: "pane", V: anchor}}); err != nil {
			return nil, err
		}
		if err := do("terminal.keep", pyjson.Obj{{K: "terminal", V: tm}, {K: "place", V: place}}); err != nil {
			return nil, err
		}
		var pane any
		err := waitFor(func() (bool, error) {
			x, y, err := where(tm)
			if x != nil && x.ID == tabID {
				pane = y.Pane
				return true, err
			}
			return false, err
		}, tm+" keep")
		return pane, err
	}

	if len(workers) > 0 {
		var first any
		if first, err = keep(workers[0], mainPane, "right"); err == nil {
			err = build(workers, first, keep)
		}
	}
	// 셸 판의 finally: 맞추다 실패해도 보던 칸으로 돌아간다(여기서 난 오류가 앞 오류를 덮는다)
	if back != "" {
		bt, _, ferr := where(back)
		if ferr == nil && bt != nil {
			ferr = do("pane.focus", pyjson.Obj{{K: "terminal", V: back}})
		}
		if ferr != nil {
			return nil, ferr
		}
	}
	if err != nil {
		return nil, err
	}
	if err := save(tabID, workers); err != nil {
		return nil, err
	}
	if workers == nil {
		workers = []string{}
	}
	return pyjson.Obj{{K: "tab", V: tabID}, {K: "main", V: main}, {K: "workers", V: workers}}, nil
}
