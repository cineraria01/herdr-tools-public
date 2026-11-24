// herdr-tools-config: 개인 설정 폴더 읽기·개인 값 검사(Go 판, 2026-11-21 사용자 「모두 고로 옮기고 싶어」 — ops-go).
// 명령·인자·출력·종료 코드는 옛 셸 판 그대로다(셸 판은 2026-11-22 go-shell-remove로 걷었다).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"herdr-tools/internal/oplog"
	"herdr-tools/internal/toolcfg"
	"herdr-tools/internal/version"
)

const usage = `herdr-tools-config: 개인 설정 폴더($HERDR_TOOLS_CONFIG, 기본 ~/.config/herdr-tools)를 읽는 도우미.
  get <키> [<기본값>]  config의 값을 찍는다. 키가 없으면 기본값, 기본값도 없으면 종료 3
  file <이름>          그 폴더의 <이름> 파일 절대 경로. 없으면 종료 3
  lint [--strict] [<파일>…]  개인 값 패턴(private-patterns.txt, 한 줄에 ERE 하나)에 걸린 줄을 「파일:줄: 패턴」으로. 걸리면 종료 1
                       패턴 파일이 없으면 경고 뒤 건너뜀(종료 0), --strict면 패턴 파일·패턴 줄이 없을 때 종료 3
                       이진·UTF-8 아닌 파일도 바이트로 본다. 상대 경로 파일은 경로 글자도(「파일:0: 경로 패턴」). 못 읽으면 종료 2. 패턴은 바이트 단위(LC_ALL=C), 대소문자 무시(ASCII)
  sync-lint <범용 저장소> [<sync.list>]  바이너리 줄(<뿌리>bin/<도구>  tools:cmd/<도구>)마다 그 대상(또는 덮는 all·macos)의 Go 소스 나무 줄
                       (go.mod·cmd/**·internal/**)과 곁 도구(cmd/<곁>/OWNER에 그 도구 이름 — 예: hc-merger → hc-lane) 바이너리 줄이 없으면
                       더할 줄을 찍고 종료 1. 걷힌 셸 판 줄(저장소에 없는 tools:bin/<도구>·tools:lib/…·*.sh·*.py)이 남아도 종료 1
                       (지울 줄은 오류 출력, 바꿀 바이너리 줄은 표준 출력). 줄 끝 CR·끝 줄바꿈 없음도 종료 1, sync.list 없음은 종료 2
`

const me = "herdr-tools-config"

func die2(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	oplog.Log(me, "", oplog.Die, 2, fmt.Sprintf(f, a...))
	os.Exit(2)
}

// usage2: 인자 오류(개수·꼴) — 글·종료 2는 die2 그대로, 운영 기록은 usage(사람·에이전트가 잘못 부름, cli-usage-version)
func usage2(f string, a ...any) {
	fmt.Fprintf(os.Stderr, me+": "+f+"\n", a...)
	oplog.Log(me, "", oplog.BadArg, 2, fmt.Sprintf(f, a...))
	os.Exit(2)
}

func main() {
	a := os.Args[1:]
	if len(a) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	dir := toolcfg.Dir()
	switch a[0] {
	case "--version":
		fmt.Println(version.String(me))
	case "-h", "--help":
		fmt.Print(usage)
	case "get":
		get(dir, a[1:])
	case "file":
		file(dir, a[1:])
	case "lint":
		strict := len(a) > 1 && a[1] == "--strict"
		files := a[1:]
		if strict {
			files = a[2:]
		}
		os.Exit(toolcfg.Lint(dir, strict, files, os.Stdout, os.Stderr))
	case "sync-lint":
		if len(a) < 2 || len(a) > 3 {
			usage2("sync-lint <범용 저장소> [<sync.list>]")
		}
		list := dir + "/sync.list"
		if len(a) == 3 && a[2] != "" { // 빈 인자도 기본 경로(셸 판 ${2:-…})
			list = a[2]
		}
		bad := listForm(list)
		if rc := toolcfg.SyncLint(a[1], list, os.Stdout, os.Stderr); rc != 0 || !bad {
			os.Exit(rc)
		}
		os.Exit(1)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

// listForm: sync.list의 줄 끝 CR·끝 줄바꿈 없음을 줄 번호로 알린다(있으면 참). 옛 herdr-sync 셸 판은 CR 줄을 어느 기계에도 못 맞추고
// 줄바꿈 없는 마지막 줄을 버렸다 — 그런 목록이 다시 생기지 않게 막는다. 못 읽으면 거짓(SyncLint가 종료 2)
func listForm(list string) bool {
	data, err := os.ReadFile(list)
	if err != nil || len(data) == 0 {
		return false
	}
	bad := false
	ls := strings.Split(string(data), "\n")
	for i, l := range ls {
		if strings.HasSuffix(l, "\r") {
			fmt.Fprintf(os.Stderr, "%s: %s:%d: 줄 끝 CR — 줄바꿈은 LF만\n", me, list, i+1)
			bad = true
		}
	}
	if data[len(data)-1] != '\n' {
		fmt.Fprintf(os.Stderr, "%s: %s:%d: 끝 줄바꿈 없음 — 마지막 줄 뒤에 줄바꿈을 붙인다\n", me, list, len(ls))
		bad = true
	}
	return bad
}

func get(dir string, a []string) {
	if len(a) < 1 || len(a) > 2 {
		usage2("get <키> [<기본값>]")
	}
	if !toolcfg.KeyRe.MatchString(a[0]) {
		usage2("키 이름 꼴이 틀림: %s (<영역>.<이름>, 소문자·숫자·_)", a[0])
	}
	v, ok, err := toolcfg.Get(dir, a[0])
	if d, isDup := err.(*toolcfg.ErrDup); isDup {
		fmt.Fprintln(os.Stderr, d.Msg)
		os.Exit(2)
	} else if err != nil {
		die2("읽기 실패 %s/config", dir)
	}
	switch {
	case ok:
		fmt.Println(v)
	case len(a) == 2:
		fmt.Println(a[1])
	default:
		fmt.Fprintf(os.Stderr, "%s: 설정 없음 %s (%s/config — 예: config.example/config)\n", me, a[0], dir)
		os.Exit(3)
	}
}

func file(dir string, a []string) {
	if len(a) != 1 || a[0] == "" {
		usage2("file <이름>")
	}
	n := a[0]
	if w := "/" + n + "/"; strings.HasPrefix(w, "//") || strings.Contains(w, "/../") || strings.Contains(w, "/./") {
		usage2("이름은 설정 폴더 안의 상대 경로: %s", n)
	}
	if st, err := os.Stat(dir + "/" + n); err == nil && st.Mode().IsRegular() {
		abs, err := filepath.Abs(dir)
		if err != nil {
			die2("설정 폴더 경로를 풀지 못했다: %s", dir)
		}
		fmt.Printf("%s/%s\n", abs, n)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: 설정 파일 없음 %s/%s (예: config.example/%s)\n", me, dir, n, n)
	os.Exit(3)
}
