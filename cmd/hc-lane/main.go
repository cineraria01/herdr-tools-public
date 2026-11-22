// hc-lane: 커밋하는 레인을 자기 git 워크트리에서 띄우고(hc run), 착지(land)·정리(clean)한다. Go 바이너리 하나다
// (2026-11-21 사용자 「모두 고로 옮기고 싶어」, 셸 판은 2026-11-22 go-shell-remove로 걷었다). 계약 — 인자·출력·종료 코드·상태 파일·
// 사건 꼴 — 은 옛 셸 판 그대로다.
//
// 하위 명령은 cmd/hc-lane/<명령>.go가 init()에서 register로 넣는다. can이 거짓인 인자(도움말 등)는 usage(오류 출력, 종료 2).
// 첫 인자가 하위 명령 이름이 아니면 레인 띄우기(launch)이고 인자 전부가 그 몫이다.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"herdr-tools/internal/proc"
)

type sub struct {
	run func(args []string) int
	can func(args []string) bool // nil이면 늘 — 거짓이면 그 인자는 usage(도움말 등)
}

var subs = map[string]sub{}

func register(name string, run func([]string) int, can func([]string) bool) {
	subs[name] = sub{run, can}
}

//go:embed usage.txt
var usageText string

// 첫 인자 분기 이름들. 그 밖의 첫 인자는 레인 띄우기(launch)
var named = map[string]bool{
	"verdict": true, "early-wait": true, "report": true, "send": true, "broadcast": true, "events": true, "wait": true,
	"watch": true, "coord": true, "clean": true, "fetch": true, "gc": true, "land": true, "merger": true, "guard": true,
	"lease": true, "park": true,
}

// pick: 그 인자의 하위 명령과 그 명령 몫 인자. 맡을 명령이 없거나 can이 거르면 ok=false(→ usage)
func pick(args []string) (s sub, rest []string, ok bool) {
	c := "launch"
	rest = args
	if len(args) > 0 && named[args[0]] {
		c, rest = args[0], args[1:]
	}
	s, ok = subs[c]
	if ok && s.can != nil && !s.can(rest) {
		ok = false
	}
	return s, rest, ok
}

func main() {
	proc.Who = "hc-lane"
	// 물려받은 fd(바깥 병합기 잠금 fd 18·19 등)를 자식에게 넘기지 않는다. Go가 여는 fd는 기본 O_CLOEXEC
	closeInherited()
	a := os.Args[1:]
	if len(a) > 0 && a[0] == "--version" {
		fmt.Println("hc-lane 1")
		return
	}
	s, rest, ok := pick(a)
	if !ok {
		usage()
	}
	os.Exit(s.run(rest))
}

// usage: 사용법 글을 오류 출력에, 종료 2
func usage() {
	fmt.Fprint(os.Stderr, usageText)
	exit(2)
}
