// hc-lane: Go 판(2026-11-21 사용자 「모두 고로 옮기고 싶어」). bin/hc-lane(셸)이 얇은 포장으로 이 바이너리를 부른다.
// 계약은 셸 판(bin/hc-lane·lib/hc-lane/*.sh)과 같다 — 같은 인자·출력·종료 코드·상태 파일·사건 꼴.
//
// 하위 명령은 cmd/hc-lane/<명령>.go가 init()에서 register로 넣는다. 아직 옮기지 않은 명령(표에 없음)이나 can이 거짓인 인자(도움말 등)는
// 종료 120(notYet) — 포장이 「__has <인자…>」로 먼저 물어 120이면 그 명령만 셸 판으로 돈다(셋으로 나눈 레인이 어떤 순서로 착지해도
// 멈추지 않게). 첫 인자가 하위 명령 이름이 아니면 레인 띄우기(launch)이고 인자 전부가 그 몫이다(셸 판 분기와 같다).
package main

import (
	_ "embed"
	"fmt"
	"os"
	"syscall"

	"herdr-tools/internal/proc"
)

// notYet: 「아직 셸」 — 포장(bin/hc-lane)이 셸 판으로 돈다
const notYet = 120

type sub struct {
	run func(args []string) int
	can func(args []string) bool // nil이면 늘 — 거짓이면 그 인자는 셸 판(도움말 등)
}

var subs = map[string]sub{}

func register(name string, run func([]string) int, can func([]string) bool) {
	subs[name] = sub{run, can}
}

//go:embed usage.txt
var usageText string

// 셸 판 bin/hc-lane 첫 인자 분기와 같은 이름들. 그 밖의 첫 인자는 레인 띄우기(launch)
var named = map[string]bool{
	"verdict": true, "early-wait": true, "report": true, "send": true, "broadcast": true, "events": true, "wait": true,
	"watch": true, "coord": true, "clean": true, "fetch": true, "gc": true, "land": true, "merger": true, "guard": true,
	"lease": true, "park": true,
}

// pick: 그 인자의 하위 명령과 그 명령 몫 인자. Go 판이 맡지 않으면 ok=false
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
	for fd := 3; fd < 1024; fd++ {
		syscall.CloseOnExec(fd)
	}
	// 포장이 go_run 앞에서 HC_LANE_BIN(시험이 bin/hc-lane 경로로 씀)을 바꿔 넣었다 — 자식(검증 등)에는 원래 값으로
	if v, ok := os.LookupEnv("HC_LANE_SH_BIN"); ok {
		if v == "" {
			os.Unsetenv("HC_LANE_BIN")
		} else {
			os.Setenv("HC_LANE_BIN", v)
		}
		os.Unsetenv("HC_LANE_SH_BIN")
	}
	a := os.Args[1:]
	if len(a) > 0 && a[0] == "--version" {
		fmt.Println("hc-lane 1")
		return
	}
	// __has <인자…>: 이 인자로 Go 판이 맡으면 0, 아니면 120
	if len(a) > 0 && a[0] == "__has" {
		if _, _, ok := pick(a[1:]); ok {
			return
		}
		os.Exit(notYet)
	}
	s, rest, ok := pick(a)
	if !ok {
		os.Exit(notYet)
	}
	os.Exit(s.run(rest))
}

// usage: 셸 판 usage와 같은 글을 오류 출력에, 종료 2
func usage() {
	fmt.Fprint(os.Stderr, usageText)
	exit(2)
}
