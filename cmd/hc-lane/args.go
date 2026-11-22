package main

// 인자 읽기: bin/hc-lane의 공통 인자 줄(셸 판 while 루프와 그 뒤 검사)을 land·guard·lease·fetch 몫만 그대로.
// 도움말(-h)이나 셸 판이 usage(사용법 전문)로 끝날 인자는 Go 판이 맡지 않는다 — can이 거짓이라 포장이 셸 판으로 부른다.

import (
	"fmt"
	"os"
	"strings"
)

type opts struct {
	cmd                                     string
	on, branch, name, repo, lbase, act, lvd string
	integ, minage, brief, swarg             string
	read, direct, resume, archive, dry      bool
	steps, rb, forceOld                     bool
	rest                                    []string // -- 뒤(또는 fetch 참조)
}

// parseErr: 셸 판의 끝 — usage(사용법, 셸 판에 맡김) · die(「hc-lane: <글>」 종료 1) · exit2(글 그대로 종료 2)
type parseErr struct {
	usage bool
	msg   string
	rc    int
}

func parseArgs(cmd string, a []string) (o opts, pe *parseErr) {
	o.cmd = cmd
	die1 := func(f string, x ...any) *parseErr { return &parseErr{msg: "hc-lane: " + fmt.Sprintf(f, x...), rc: 1} }
	switch cmd {
	case "land":
		if len(a) > 0 && a[0] != "" && !strings.HasPrefix(a[0], "-") {
			o.lbase, a = a[0], a[1:]
		}
	case "guard", "lease":
		if len(a) > 0 {
			o.act, a = a[0], a[1:]
		}
	}
	for len(a) > 0 {
		val := func() *parseErr {
			if len(a) < 2 {
				return die1("%s 값이 없다", a[0])
			}
			return nil
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
		case x == "--force-old":
			if cmd != "land" {
				return o, die1("--force-old는 land에서만 쓴다")
			}
			o.forceOld, a = true, a[1:]
		case x == "--verdict":
			if e := val(); e != nil {
				return o, e
			}
			if cmd != "land" {
				return o, die1("--verdict는 land에서만 쓴다")
			}
			if a[1] == "" || strings.Trim(a[1], alnum+"_-") != "" {
				return o, die1("--verdict 판정 ID 글자: %s", a[1])
			}
			o.lvd, a = a[1], a[2:]
		case x == "-h" || x == "--help":
			return o, &parseErr{usage: true}
		case x == "--":
			o.rest, a = a[1:], nil
		case strings.HasPrefix(x, "-"):
			return o, die1("알 수 없는 인자: %s", x)
		default:
			if cmd == "fetch" {
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
	rds := !o.read && !o.dry && !o.steps
	switch cmd {
	case "fetch":
		if o.repo == "" {
			return o, &parseErr{usage: true}
		}
		if o.name+o.branch != "" || !rds {
			return o, die1("fetch에는 --on·--repo와 참조만 쓴다")
		}
		for _, r := range o.rest {
			if strings.HasPrefix(r, "-") {
				return o, die1("fetch 참조는 - 로 시작하지 않는다(git 옵션으로 읽힌다): %s", r)
			}
		}
	case "land":
		if o.on+o.name+o.branch+o.repo != "" || !rds || len(o.rest) > 0 {
			return o, die1("land는 레인 작업 폴더 안에서 인자 없이(또는 기본 브랜치 하나만) 부른다: hc-lane land [<기본 브랜치>]")
		}
	case "lease":
		if o.act != "show" && o.act != "drop" {
			return o, &parseErr{usage: true}
		}
		if o.repo == "" || o.name == "" || o.branch != "" || !rds || len(o.rest) > 0 {
			return o, die1("lease show|drop에는 --on·--repo·--name만 쓴다")
		}
	case "guard":
		if o.act != "install" && o.act != "status" && o.act != "remove" {
			return o, &parseErr{usage: true}
		}
		if o.repo == "" {
			return o, &parseErr{usage: true}
		}
		if o.on+o.name != "" || !rds || len(o.rest) > 0 {
			return o, die1("guard에는 --repo(와 --branch <기본 브랜치>)만 쓴다. 다른 기계는 hc exec --on <기계> -- hc-lane guard …")
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

// can: Go 판이 이 인자를 맡나(usage로 끝나면 셸 판)
func can(cmd string) func([]string) bool {
	return func(a []string) bool {
		_, pe := parseArgs(cmd, a)
		return pe == nil || !pe.usage
	}
}

// mustParse: 읽고, 틀리면 셸 판과 같은 글·종료 코드로 끝낸다
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
