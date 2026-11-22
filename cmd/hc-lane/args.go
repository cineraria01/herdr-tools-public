package main

// 인자 읽기: 옛 셸 판 공통 인자 줄(while 루프와 그 뒤 검사)을 land·guard·lease·fetch 몫만 그대로.
// 이 파일에는 공용 옵션만 둔다. 한 명령만의 앞 자리 인자·옵션·검사는 그 명령 파일이 init()에서 argSpecs[<명령>]에 넣는다
// (새 하위 명령·옵션을 더하는 레인이 이 파일을 고치지 않게 — 2026-11-22 go-cmd-registry).
// 도움말(-h)이나 usage(사용법 전문)로 끝날 인자는 can이 거짓이라 usage로 간다.

import (
	"fmt"
	"os"
	"strings"
)

type opts struct {
	cmd                                string
	on, branch, name, repo             string
	head                               string // 그 명령의 앞 자리 인자(argSpec.head — land <기본>, guard·lease <동작>)
	integ, minage, brief, swarg        string
	read, direct, resume, archive, dry bool
	steps, rb                          bool
	rest                               []string          // -- 뒤(또는 argSpec.bare 명령의 맨 인자)
	own                                map[string]string // 그 명령만의 옵션(argSpec.flags): 옵션 이름 → 값(값 없는 옵션은 "1")
}

// argSpec: 한 하위 명령만의 인자 규칙. 그 명령 파일이 init()에서 argSpecs[<명령>]에 넣는다
type argSpec struct {
	head  func(a []string) bool             // 옵션 앞 첫 인자를 o.head로 받나(nil이면 안 받음)
	bare  bool                              // 옵션이 아닌 맨 인자를 o.rest로(fetch 참조)
	flags map[string]ownFlag                // 이 명령에서만 쓰는 옵션 — 다른 명령에 주면 「<옵션>는 <명령>에서만 쓴다」
	check func(o *opts, rds bool) *parseErr // 옵션을 다 읽은 뒤 이 명령만의 검사(rds: --read·--dry-run·--print-steps 없음)
}

type ownFlag struct {
	val bool                  // 값을 받나
	bad func(v string) string // 값 검사: 빈 글이면 통과, 아니면 그 글로 멈춤(nil이면 검사 없음)
}

var argSpecs = map[string]argSpec{}

// ownerOf: 그 옵션을 제 것으로 둔 명령(두 명령이 같은 옵션을 두지 않는다 — TestArgSpecsOwnFlagsUnique)
func ownerOf(flag string) (string, ownFlag, bool) {
	for c, sp := range argSpecs {
		if f, ok := sp.flags[flag]; ok {
			return c, f, true
		}
	}
	return "", ownFlag{}, false
}

// parseErr: 끝 — usage(사용법) · die(「hc-lane: <글>」 종료 1) · exit2(글 그대로 종료 2)
type parseErr struct {
	usage bool
	msg   string
	rc    int
}

func die1(f string, x ...any) *parseErr {
	return &parseErr{msg: "hc-lane: " + fmt.Sprintf(f, x...), rc: 1}
}

func parseArgs(cmd string, a []string) (o opts, pe *parseErr) {
	o.cmd = cmd
	o.own = map[string]string{}
	sp := argSpecs[cmd]
	if sp.head != nil && sp.head(a) {
		o.head, a = a[0], a[1:]
	}
	for len(a) > 0 {
		val := func() *parseErr {
			if len(a) < 2 {
				return die1("%s 값이 없다", a[0])
			}
			return nil
		}
		if oc, f, ok := ownerOf(a[0]); ok {
			if f.val {
				if e := val(); e != nil {
					return o, e
				}
			}
			if oc != cmd {
				return o, die1("%s는 %s에서만 쓴다", a[0], oc)
			}
			v, n := "1", 1
			if f.val {
				v, n = a[1], 2
			}
			if f.bad != nil {
				if m := f.bad(v); m != "" {
					return o, die1("%s", m)
				}
			}
			o.own[a[0]], a = v, a[n:]
			continue
		}
		switch x := a[0]; {
		case x == "--on" || x == "--branch" || x == "--name" || x == "--repo" || x == "--brief" || x == "--start-wait" || x == "--min-age":
			if e := val(); e != nil {
				return o, e
			}
			v := a[1]
			switch x {
			case "--on":
				o.on = v
			case "--branch":
				o.branch = v
			case "--name":
				o.name = v
			case "--repo":
				o.repo = v
			case "--brief":
				o.brief = v
			case "--start-wait":
				o.swarg = v
			case "--min-age":
				o.minage = v
			}
			a = a[2:]
		case x == "--read":
			o.read, a = true, a[1:]
		case x == "--direct":
			o.direct, a = true, a[1:]
		case x == "--resume":
			o.resume, a = true, a[1:]
		case x == "--dry-run":
			o.dry, a = true, a[1:]
		case x == "--print-steps":
			o.steps, a = true, a[1:]
		case x == "--integ":
			o.integ, a = "refs/integ/", a[1:]
		case strings.HasPrefix(x, "--integ="):
			o.integ = strings.TrimPrefix(x, "--integ=")
			if o.integ == "" {
				return o, die1("--integ= 뒤에 접두가 없다")
			}
			a = a[1:]
		case x == "--remote-branches":
			o.rb, a = true, a[1:]
		case x == "--archive":
			o.archive, a = true, a[1:]
		case x == "-h" || x == "--help":
			return o, &parseErr{usage: true}
		case x == "--":
			o.rest, a = a[1:], nil
		case strings.HasPrefix(x, "-"):
			return o, die1("알 수 없는 인자: %s", x)
		default:
			if sp.bare {
				o.rest, a = a, nil
				break
			}
			return o, die1("알 수 없는 인자: %s (실행 명령 앞에는 -- 를 둔다)", x)
		}
	}
	if o.brief+o.swarg != "" {
		return o, die1("--brief·--start-wait는 레인을 띄울 때만 쓴다")
	}
	if o.direct {
		return o, die1("--direct는 레인을 띄울 때만 쓴다")
	}
	if o.resume {
		return o, die1("--resume은 쓰기 레인을 띄울 때만 쓴다")
	}
	if sp.check != nil {
		if e := sp.check(&o, !o.read && !o.dry && !o.steps); e != nil {
			return o, e
		}
	}
	if o.archive {
		return o, die1("--archive는 clean에서만 쓴다")
	}
	if o.integ+o.minage != "" || o.rb {
		return o, die1("--integ·--min-age·--remote-branches는 gc에서만 쓴다")
	}
	if !laneChars(o.name) {
		return o, die1("--name은 a-z 0-9 - _ 만 쓴다(hc 규칙): %s", o.name)
	}
	if len(o.name) > 32 {
		return o, die1("--name은 32자까지: %s", o.name)
	}
	return o, nil
}

// can: 이 인자를 맡나(usage로 끝나면 거짓)
func can(cmd string) func([]string) bool {
	return func(a []string) bool {
		_, pe := parseArgs(cmd, a)
		return pe == nil || !pe.usage
	}
}

// mustParse: 읽고, 틀리면 같은 글·종료 코드로 끝낸다
func mustParse(cmd string, a []string) opts {
	o, pe := parseArgs(cmd, a)
	if pe != nil {
		if pe.usage { // 보통은 can이 먼저 거른다
			usage()
		}
		fmt.Fprintln(os.Stderr, pe.msg)
		os.Exit(pe.rc)
	}
	on, repo = o.on, o.repo
	return o
}
