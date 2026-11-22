// herdr-balance: Herdr 탭의 오른쪽 worker 열을 고른 높이로 다시 붙인다(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」).
// 계약은 옛 셸 판(파이썬, 2026-11-22 go-shell-remove로 걷음)과 같다 — 인자·출력 JSON 꼴(한글 그대로)·종료 코드·상태 파일
// ~/.local/state/herdr-balance/<탭>.json·herdr-app에 넘기는 인자 글자. 바뀐 것: herdr-app 응답이 JSON이 아니거나 상태 파일을
// 못 쓰면 셸 판은 예외로 죽었고(--watch도 멈춤) Go 판은 그 회차 오류 한 줄로 친다, 상태 파일은 반쪽 없이 바꿔 넣는다,
// herdr-app 목록에 spaces·tabs·탭 id·terminals·터미널 terminal·pane·name이 없으면 셸 판(KeyError로 죽음)처럼 그 맞춤은 실패다(B12 —
// 「worker 없음」으로 읽어 상태를 저장하지 않는다. app이 없는 터미널만은 에이전트 아님으로 친다 — 셸 판은 첫 터미널이면 죽었다).
// go-final-audit B11·K2: herdr-app 한 번은 15초 상한(넘으면 그 회차 오류 — 결과 모름이라 상태를 바꾸지 않는다)이고 새 무리로 띄워 무리째
// 거둔다. TERM·INT를 받으면 돌던 호출을 끊고 128+신호(143·130 — 셸 판이 신호로 죽을 때와 같은 값)로 끝난다. 다만 worker를 떼기
// 시작한 뒤라면 그 맞춤을 마치고 끝낸다(떼인 채 남지 않게, 그 사이 호출도 상한 안 — 셸 판은 바로 죽는다). --watch는 두 판 공통 잠금
// ~/.local/state/herdr-balance/watch.lock(flock 배타)을 쥐고 돈다 — 남이 쥐면 한 줄 찍고 75(launchd가 다시 본다).
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
// gofix2: worker를 떼기 전에 목표를 <탭>.pending.json(pid·시작 시각)에 적고 저장 뒤 지운다 — 남아 있고 그 실행이 끝났으면 다음 맞춤이
// 떼인 채 남은 worker를 되붙인다(R-26, 셸 판에는 없음). 원래 탭이 빠진 목록(원격 공간 끊김)으로는 「worker 없음」을 확정하지 않는다(R-07).
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
	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/pyjson"
	"herdr-tools/internal/version"
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
		fmt.Println(version.String("herdr-balance")) // 릴리스 커밋이 보이게(B25)
		return
	}
	a, _ := parser.Parse(os.Args[1:])
	if a.Has["print-remote"] {
		fmt.Println(strings.Join(remotePrefixes(context.Background()), "|"))
		return
	}
	if a.Has["watch"] {
		os.Exit(watchMain())
	}
	if a.Get("main") == "" {
		parser.Error("--main 또는 --watch가 필요")
	}
	ctx, sig := signals()
	r, err := balance(ctx, a.Get("main"), a.Vals["add"], a.Vals["drop"])
	if ctx.Err() != nil {
		os.Exit(<-sig)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(pyjson.Dumps(r, false))
}

// signals: TERM·INT를 받으면 끝나는 ctx와, 그때 낼 종료 코드(128+신호)를 받을 곳
func signals() (context.Context, <-chan int) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	code := make(chan int, 1)
	go func() {
		s := <-ch
		n, _ := s.(syscall.Signal)
		code <- 128 + int(n)
		cancel()
	}()
	return ctx, code
}

// watchMain: 두 판 공통 잠금을 쥐고 watch. 남이 쥐면 75, 잠금을 모르면 띄우지 않고 1. 신호로 끝나면 128+신호
func watchMain() int {
	if err := os.MkdirAll(stateDir(), 0o777); err != nil {
		fmt.Fprintf(os.Stderr, "herdr-balance: 상태 폴더 만들기: %v\n", err)
		return 1
	}
	p := filepath.Join(stateDir(), "watch.lock")
	lk, busy, err := lockx.TryLock(p)
	if busy {
		fmt.Fprintf(os.Stderr, "herdr-balance: --watch가 이미 돈다(잠금 %s) — 끝냄\n", p)
		return 75
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-balance: 잠금 %s: %v\n", p, err)
		return 1
	}
	defer lk.Close()
	ctx, sig := signals()
	watch(ctx)
	return <-sig
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func stateDir() string { return filepath.Join(home(), ".local/state/herdr-balance") }

// remotePrefixes: balance.remote — 이 창에서 맞춰 줄 다른 기계 탭 이름 앞부분. 나머지 원격 탭은 건드리지 않는다.
// launchd PATH에는 ~/.local/bin이 없을 수 있어 거기도 본다. 못 읽거나 설정 파일이 없으면 빈 목록 + 경고 한 줄(키만 없으면 조용히)
func remotePrefixes(ctx context.Context) []string {
	cmd, err := exec.LookPath("herdr-tools-config")
	if err != nil {
		cmd = filepath.Join(home(), ".local/bin/herdr-tools-config")
	}
	out, errs, rc, err := runLim(ctx, 10*time.Second, cmd, "get", "balance.remote", "")
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
		if _, _, rc, err := runLim(ctx, 10*time.Second, cmd, "file", "config"); err != nil || rc != 0 {
			fmt.Fprintln(os.Stderr, "herdr-balance: 설정 파일 없음(herdr-tools-config file config) — 원격 탭은 맞추지 않음")
		}
	}
	return remote
}

// runLim: name을 새 무리로 띄워 d 안에 기다린다. d를 넘거나 ctx가 끝나면(신호) 무리째 거둔다(TERM → 2초 → KILL) — 그때는 err(결과 모름).
// err는 못 띄움·시간 넘김·멈춤, rc는 종료 코드
func runLim(ctx context.Context, d time.Duration, name string, args ...string) (string, string, int, error) {
	lctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var o, e bytes.Buffer
	c := exec.CommandContext(lctx, name, args...) // Stdin nil = /dev/null
	c.Stdout, c.Stderr = &o, &e
	proc.Group(c)
	c.Cancel = func() error { proc.StopGroup(c.Process.Pid, 2*time.Second); return nil }
	c.WaitDelay = 3 * time.Second // 끝났는데 출력 관을 쥔 손자가 남아도 이만큼만
	err := c.Run()
	if err != nil && ctx.Err() != nil {
		return "", "", 0, fmt.Errorf("멈춤(신호): %s", filepath.Base(name))
	}
	if err != nil && lctx.Err() != nil {
		return "", "", 0, fmt.Errorf("시간 넘김 %d초: %s", int(d/time.Second), filepath.Base(name))
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// 우두머리는 0으로 끝났는데 출력 관을 쥔 자손이 남았다(R-12) — 그 무리를 거둔다. 무리가 남아 있는 동안 그 번호는
		// 다른 무리에 다시 쓰이지 않으니 우리 무리다(POSIX). 출력이 덜 왔을 수 있어 결과는 모름
		proc.StopGroup(c.Process.Pid, 2*time.Second)
		return "", "", 0, fmt.Errorf("끝났는데 출력 관을 쥔 자손이 남음(거둠): %s", filepath.Base(name))
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

// appLimit: herdr-app 한 번의 상한(B11·K2 — 창이 멈추면 --watch가 끝까지 멈췄다)
var appLimit = 15 * time.Second

// appOut: herdr-app 부르기. 종료 코드가 0이 아니면 셸 판과 같은 문구로 실패(출력을 그대로 붙임)
func appOut(ctx context.Context, args ...string) ([]byte, error) {
	o, e, rc, err := runLim(ctx, appLimit, "herdr-app", args...)
	if err != nil {
		return nil, fmt.Errorf("herdr-app %s 실패: %v", strings.Join(args, " "), err)
	}
	if rc != 0 {
		return nil, fmt.Errorf("herdr-app %s 실패: %s%s", strings.Join(args, " "), o, e)
	}
	return []byte(o), nil
}

// appJSON: herdr-app 응답을 v로. 셸 판은 「{」로 시작할 때만 JSON으로 읽었다 — 아니면 obj=false
func appJSON(ctx context.Context, v any, args ...string) (obj bool, err error) {
	out, err := appOut(ctx, args...)
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

// tabs: 화면의 탭 전부. 목록이 덜 왔으면(칸이 없음) 오류 — 셸 판 KeyError와 같은 결과(B12)
func tabs(ctx context.Context) ([]tab, error) {
	var ls struct {
		Spaces *[]struct {
			Tabs *[]struct {
				ID        *string           `json:"id"`
				Terminals *[]map[string]any `json:"terminals"`
			} `json:"tabs"`
		} `json:"spaces"`
	}
	obj, err := appJSON(ctx, &ls, "ls")
	if err != nil {
		return nil, err
	}
	if !obj {
		return nil, errors.New("herdr-app ls 응답이 JSON 객체가 아님")
	}
	short := func(what string) error {
		return fmt.Errorf("herdr-app ls 응답에 %s 없음 — 목록을 다 못 받음", what)
	}
	if ls.Spaces == nil {
		return nil, short("spaces")
	}
	var out []tab
	for _, s := range *ls.Spaces {
		if s.Tabs == nil {
			return nil, short("tabs")
		}
		for _, t := range *s.Tabs {
			if t.ID == nil || t.Terminals == nil {
				return nil, short("탭 id·terminals")
			}
			tb := tab{ID: *t.ID}
			for _, x := range *t.Terminals {
				id, ok := x["terminal"].(string)
				_, hasPane := x["pane"]
				_, hasName := x["name"]
				if !ok || !hasPane || !hasName {
					return nil, short("터미널 terminal·pane·name")
				}
				tb.Terminals = append(tb.Terminals, term{Terminal: id, Pane: x["pane"], Name: x["name"], App: x["app"]})
			}
			out = append(out, tb)
		}
	}
	return out, nil
}

// where: term이 있는 탭과 그 터미널(없으면 nil)
func where(ctx context.Context, t string) (*tab, *term, error) {
	x, y, _, err := locate(ctx, t, "")
	return x, y, err
}

// locate: where에 더해 탭 tabID가 목록에 있었는가. 탭이 빠진 목록(원격 공간 연결이 끊김)에서는 터미널이 없어도 「없음」을
// 확정하지 못한다(R-07 — 떼야 할 worker를 「이미 없음」으로 읽고 맞춤을 마쳤다)
func locate(ctx context.Context, t, tabID string) (*tab, *term, bool, error) {
	ts, err := tabs(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	seen := false
	for i := range ts {
		seen = seen || ts[i].ID == tabID
	}
	for i := range ts {
		for j := range ts[i].Terminals {
			if ts[i].Terminals[j].Terminal == t {
				return &ts[i], &ts[i].Terminals[j], seen, nil
			}
		}
	}
	return nil, nil, seen, nil
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
func do(ctx context.Context, cmd string, kw pyjson.Obj) error {
	var r map[string]any
	obj, err := appJSON(ctx, &r, "do", cmd, pyjson.Dumps(kw, true))
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
	return filepath.Join(stateDir(), strings.ReplaceAll(tabID, "/", "__")+".json")
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

func balance(ctx context.Context, main string, add, drop []string) (pyjson.Obj, error) {
	t, m, err := where(ctx, main)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("%s이(가) 화면의 어느 탭에도 없음", main)
	}
	tabID, mainPane := t.ID, m.Pane
	// 앞 실행이 떼다 만 worker(화면 어느 탭에도 없는 것 — 떼인 채)는 --add처럼 다시 붙인다(R-26). 드롭한 것은 빼고
	left, err := leftover(tabID)
	if err != nil {
		return nil, err
	}
	if len(left) > 0 {
		ts, err := tabs(ctx)
		if err != nil {
			return nil, err
		}
		var shown, ids []string
		for _, x := range ts {
			ids = append(ids, x.ID)
			for _, y := range x.Terminals {
				shown = append(shown, y.Terminal)
			}
		}
		if !contains(ids, tabID) { // 탭이 빠진 목록으로는 「떼인 채」를 확정하지 않는다(R-07)
			return nil, fmt.Errorf("탭 %s이 목록에서 빠짐 — 떼다 만 worker를 확인하지 못함", tabID)
		}
		add = append([]string(nil), add...)
		for _, w := range left {
			if !contains(drop, w) && !contains(add, w) && !contains(shown, w) {
				add = append(add, w)
			}
		}
	}
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
	if obj, err := appJSON(ctx, &st, "status"); err != nil {
		return nil, err
	} else if !obj {
		return nil, errors.New("herdr-app status 응답이 JSON 객체가 아님")
	}
	ts, err := tabs(ctx)
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

	// 여기부터 worker를 뗀다 — 신호가 와도 이 맞춤은 마친다(떼인 채 남지 않게. 호출마다 상한·기다림 10초는 그대로).
	// 그 사이 KILL·기계 재시작이면 떼인 채 남으니 떼기 전에 목표를 적어 두고(pending) 다음 맞춤이 되붙인다(R-26).
	// ponytail: 떼인 채 닫힌 worker가 목표에 남으면 그 탭 맞춤은 keep 실패로 멈춘다(--watch는 다른 탭은 계속 맞춘다) —
	// <탭>.pending.json을 지우면 풀림, herdr-app이 떼인 터미널 목록을 주면 그걸로 거른다
	if err := savePending(tabID, workers); err != nil {
		return nil, err
	}
	crit := context.WithoutCancel(ctx)
	for _, w := range append(append([]string(nil), workers...), drop...) {
		// 원래 탭이 목록에 보일 때만 「없음」을 확정한다 — 탭이 빠진 목록이면 다시 본다(R-07)
		found := false
		if err := waitFor(func() (bool, error) {
			x, _, seen, err := locate(crit, w, tabID)
			found = x != nil
			return found || seen, err
		}, w+" 찾기(탭 "+tabID+"이 목록에 없음)"); err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if err := do(crit, "terminal.park", pyjson.Obj{{K: "terminal", V: w}}); err != nil {
			return nil, err
		}
		if err := waitFor(func() (bool, error) {
			x, _, seen, err := locate(crit, w, tabID)
			return x == nil && seen, err
		}, w+" park"); err != nil {
			return nil, err
		}
	}

	keep := func(tm string, anchor any, place string) (any, error) {
		if err := do(crit, "pane.focus", pyjson.Obj{{K: "pane", V: anchor}}); err != nil {
			return nil, err
		}
		if err := do(crit, "terminal.keep", pyjson.Obj{{K: "terminal", V: tm}, {K: "place", V: place}}); err != nil {
			return nil, err
		}
		var pane any
		err := waitFor(func() (bool, error) {
			x, y, err := where(crit, tm)
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
		bt, _, ferr := where(crit, back)
		if ferr == nil && bt != nil {
			ferr = do(crit, "pane.focus", pyjson.Obj{{K: "terminal", V: back}})
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
	if err := os.Remove(pendingPath(tabID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("미완료 배치 지우기: %w", err)
	}
	if workers == nil {
		workers = []string{}
	}
	return pyjson.Obj{{K: "tab", V: tabID}, {K: "main", V: main}, {K: "workers", V: workers}}, nil
}
