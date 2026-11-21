// ovis-split: Ovis(herdr-core 모드)에서 worker를 지휘 탭 옆 탭에 보이게 한다(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」).
// 계약은 셸 판(lib/ovis-split/ovis-split.py)과 같다 — 인자·출력 JSON 꼴(한글 그대로)·hc label 인자·종료 코드. 바뀐 것: hc가 실패하면
// 셸 판은 예외 추적을 찍고 1로 죽었고 Go 판은 「ovis-split: hc … 실패(종료 N): …」 한 줄로 1이다. 주소(address)가 없거나 문자열이
// 아닌 hc 응답도 셸 판(KeyError)처럼 1이고, Go 판은 라벨을 하나라도 붙이기 전에 모든 대상 주소를 본다(B21 — 셸 판은 앞 대상에 붙인 뒤 죽기도 했다).
//
//	ovis-split --main <지휘> [--add tX ...]   지휘와 같은 공간에 worker마다 탭 하나. --add가 없으면 지휘의 이름 붙은 자식 전부
//	ovis-split --main <지휘> --drop tX ...     공간에서 뺀다(Ovis의 Other terminals로). 지휘 자신과 그 자식만
//	ovis-split --self-test
//
// Ovis는 herdr.space 라벨로 공간을 고르고, herdr.tab 라벨이 없으면 터미널마다 탭을 따로 준다(Ovis
// server/herdr-core/snapshot.ts groups). 한 탭에 나란히 넣지 않는다: Ovis는 칸 크기를 각 터미널의 열·행 비율로
// 정하고 그린 칸 크기로 터미널을 다시 줄여, 쓰는 칸이 계속 작아진다(2026-11-05 지휘 t1v0가 26열×1행까지).
// 지휘에 라벨이 없으면 같은 폴더의 Herdr 앱 공간, 없으면 같은 폴더의 다른 지휘가 쓰는 공간, 그것도 없으면
// 새 공간에 지휘 탭을 준다.
package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"herdr-tools/internal/argp"
	"herdr-tools/internal/proc"
	"herdr-tools/internal/pyjson"
	"herdr-tools/internal/version"
)

const (
	space, tabL, wsName, tabName = "herdr.space", "herdr.tab", "ovis.workspace", "ovis.tab"
)

// 라벨을 붙이는 순서(셸 판 dict 순서)
var labelKeys = []string{space, tabL, wsName, tabName}

var parser = &argp.Parser{
	Prog: "ovis-split",
	Usage: "usage: ovis-split [-h] [--main MAIN] [--add TERMINAL [TERMINAL ...]]\n" +
		"                  [--drop TERMINAL [TERMINAL ...]] [--self-test]\n",
	Help: "usage: ovis-split [-h] [--main MAIN] [--add TERMINAL [TERMINAL ...]]\n" +
		"                  [--drop TERMINAL [TERMINAL ...]] [--self-test]\n\n" +
		"Ovis(herdr-core 모드)에서 worker를 지휘 탭 옆 탭에 보이게 한다\n\noptions:\n" +
		"  -h, --help            show this help message and exit\n" +
		"  --main MAIN           지휘 터미널 주소(보통 $HC_TERMINAL_ID)\n" +
		"  --add TERMINAL [TERMINAL ...]\n  --drop TERMINAL [TERMINAL ...]\n  --self-test\n",
	Opts: []argp.Opt{{Name: "main", Kind: argp.One}, {Name: "add", Kind: argp.Plus}, {Name: "drop", Kind: argp.Plus},
		{Name: "self-test", Kind: argp.Flag}},
}

type obj = map[string]any

// pystr: 파이썬 f-string이 찍는 꼴(문자열은 그대로, 없음은 None)
func pystr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	}
	return fmt.Sprint(v)
}

// str: 문자열 칸(없거나 문자열이 아니면 빈 값)
func str(m obj, k string) string {
	s, _ := m[k].(string)
	return s
}

func labelsOf(m obj) obj {
	l, _ := m["labels"].(map[string]any)
	return l
}

// basename: 파이썬 os.path.basename(끝이 /면 빈 값 — filepath.Base와 다르다)
func basename(p string) string { return p[strings.LastIndex(p, "/")+1:] }

func uuid4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand는 실패하지 않는다(Go 1.24)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ovis-split: "+format+"\n", a...)
	os.Exit(1)
}

func runHC(args ...string) []byte {
	var o, e bytes.Buffer
	c := exec.Command("hc", args...) // Stdin nil = /dev/null
	c.Stdout, c.Stderr = &o, &e
	if err := proc.Run(c); err != nil { // 상한(proc.Limit) — 넘으면 아래 「못 부름」으로 죽는다
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			die("hc %s 실패(종료 %d): %s", strings.Join(args, " "), ee.ExitCode(), strings.TrimSpace(e.String()))
		}
		die("hc %s 못 부름: %v", strings.Join(args, " "), err)
	}
	return o.Bytes()
}

func hc(args ...string) obj {
	out := runHC(args...)
	m := obj{}
	if strings.TrimSpace(string(out)) == "" {
		return m
	}
	if err := pyjson.Decode(out, &m); err != nil {
		die("hc %s 응답을 못 읽음: %v", strings.Join(args, " "), err)
	}
	return m
}

// addrOf: 터미널 주소. 없거나 문자열이 아니면 종료 1 — 셸 판 KeyError와 같은 결과(B21). 라벨을 붙이기 전에 모든 대상을 이것으로 본다
// (「None」을 주소로 넘기지 않는다)
func addrOf(m obj, what string) string {
	s, ok := m["address"].(string)
	if !ok || s == "" {
		die("%s 응답에 address(문자열)가 없다", what)
	}
	return s
}

func label(address string, pairs map[string]string) {
	args := []string{"label", address}
	for _, k := range labelKeys {
		if v, ok := pairs[k]; ok {
			args = append(args, k+"="+v)
		}
	}
	runHC(args...)
}

// place: main 탭의 라벨과, 그 라벨을 main에 새로 붙여야 하는지.
// candidates: 공간을 빌려 올 후보(앞이 우선). spaceOf(주소)는 그 터미널의 herdr.space(없으면 빈 값)
func place(main obj, candidates []obj, spaceOf func(string) string) (map[string]string, bool) {
	labels := labelsOf(main)
	if str(labels, space) != "" && str(labels, tabL) != "" {
		out := map[string]string{}
		for _, k := range labelKeys {
			if v := str(labels, k); v != "" {
				out[k] = v
			}
		}
		return out, false
	}
	sp := str(labels, space)
	for _, other := range candidates {
		if sp != "" {
			break
		}
		// 같은 폴더의 터미널이 쓰는 공간에 새 탭으로. 자기 자신과 다른 기계 것은 제외.
		if str(other, "address") != str(main, "address") && pystr(other["cwd"]) == pystr(main["cwd"]) &&
			!strings.Contains(str(other, "address"), "/") {
			sp = spaceOf(str(other, "address"))
		}
	}
	if sp == "" {
		sp = "ovis-" + uuid4()
	}
	tb := str(labels, tabL)
	if tb == "" {
		tb = "ovis-" + uuid4()
	}
	ws := basename(str(main, "cwd"))
	if ws == "" {
		ws = "ovis"
	}
	return map[string]string{space: sp, tabL: tb, wsName: ws, tabName: "지휘 " + pystr(main["address"])}, true
}

// workerLabels: worker 라벨 — 지휘의 공간, 탭 라벨은 지워 worker마다 탭 하나, 탭 이름은 'worker 이름 · 지휘 주소'
func workerLabels(pairs map[string]string, worker obj, mainAddress string) map[string]string {
	n := str(worker, "name")
	if n == "" {
		n = pystr(worker["address"])
	}
	return map[string]string{space: pairs[space], tabL: "", wsName: pairs[wsName], tabName: n + " · " + mainAddress}
}

// droppable: --drop이 받는 것 — 지휘 자신이나 그 자식이고, Herdr 앱 칸(where 라벨)이 아닌 것
func droppable(target obj, mainAddress string) bool {
	if str(labelsOf(target), "where") != "" {
		return false
	}
	return str(target, "address") == mainAddress || str(target, "parent") == mainAddress
}

func terminals(m obj) []obj {
	l, _ := m["terminals"].([]any)
	var out []obj
	for _, x := range l {
		if t, ok := x.(map[string]any); ok {
			out = append(out, t)
		}
	}
	return out
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.String("ovis-split")) // 릴리스 커밋이 보이게(B25)
		return
	}
	a, _ := parser.Parse(os.Args[1:])
	if a.Has["self-test"] {
		if err := selfTest(); err != nil {
			die("self-test 실패: %v", err)
		}
		fmt.Println("ok")
		return
	}
	if a.Get("main") == "" {
		parser.Error("--main이 필요하다")
	}
	main := hc("get", "--json", a.Get("main"))
	mainAddr := addrOf(main, "hc get "+a.Get("main"))
	if len(a.Vals["drop"]) > 0 {
		dropped := []any{}
		var targets []string
		for _, name := range a.Vals["drop"] {
			target := hc("get", "--json", name)
			if !droppable(target, mainAddr) {
				fmt.Fprintf(os.Stderr, "건너뜀 %s: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다\n", name)
				continue
			}
			targets = append(targets, addrOf(target, "hc get "+name))
		}
		for _, t := range targets {
			label(t, map[string]string{space: "", tabL: "", wsName: "", tabName: ""})
			dropped = append(dropped, t)
		}
		fmt.Println(pyjson.Dumps(pyjson.Obj{{K: "main", V: main["address"]}, {K: "dropped", V: dropped}}, false))
		return
	}
	var live []obj
	for _, t := range terminals(hc("ls", "--json")) {
		if str(t, "state") == "live" {
			live = append(live, t)
		}
	}
	folder := basename(str(main, "cwd"))
	if folder == "" {
		folder = "ovis"
	}
	var candidates []obj
	for _, t := range live {
		addrOf(t, "hc ls") // 셸 판은 살아 있는 터미널 주소로 표를 만들다 KeyError — 여기서는 라벨 전에
		if str(t, "where") != "" {
			candidates = append(candidates, t)
		}
	}
	for _, t := range terminals(hc("ls", "--json", "--label", wsName+"="+folder)) {
		if str(t, "state") == "live" {
			addrOf(t, "hc ls --label")
			candidates = append(candidates, t)
		}
	}
	pairs, isNew := place(main, candidates, func(address string) string { return str(labelsOf(hc("get", "--json", address)), space) })
	if isNew {
		label(mainAddr, pairs)
	}
	named := map[string]obj{}
	for _, t := range live {
		named[pystr(t["address"])] = t
	}
	workers := []any{}
	for _, w := range a.Vals["add"] {
		workers = append(workers, w)
	}
	if len(workers) == 0 {
		// 이름 없는 자식은 HUD·배경 실행이라 건드리지 않는다(herdr-balance와 같은 기준)
		for _, t := range live {
			if str(t, "parent") == mainAddr && str(t, "name") != "" {
				workers = append(workers, t["address"])
			}
		}
	}
	for _, w := range workers {
		ws := pystr(w)
		wk := named[ws]
		if len(wk) == 0 {
			wk = obj{"address": ws}
		}
		label(ws, workerLabels(pairs, wk, mainAddr))
	}
	fmt.Println(pyjson.Dumps(pyjson.Obj{{K: "main", V: main["address"]}, {K: "space", V: pairs[space]},
		{K: "tab", V: pairs[tabL]}, {K: "added", V: workers}}, false))
}
