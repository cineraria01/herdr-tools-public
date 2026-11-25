package main

// hc-fill machine — 기계 줄 보기와 실행 중 조정(자리·멈춤 손잡이). 고르기는 loadsteer.go(줄 수 예산으로 2026-11-26 나눔)

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// machine [<기계> (--seats <n>|- | --pause | --resume) [--for <분>] [--why <글>] [--force]] — 인자 없으면 기계 줄
func machineCmd(args []string) {
	if len(args) == 1 && args[0] == "--json" {
		machineJSON()
		exit(0)
	}
	if len(args) > 0 && args[0] == "--rename" { // 기계 이름 바꾸기(rename.go)
		machineRename(args[1:])
	}
	args, dryM := takeDry(args)
	if len(args) == 0 {
		for _, l := range machineLines(true) {
			fmt.Println(l)
		}
		exit(0)
	}
	m := args[0]
	if strings.HasPrefix(m, "-") {
		usage()
	}
	if !machineKnown(m) {
		die("설정에 없는 기계: %s — machine 줄의 이름만", m)
	}
	act, val, why, forM, force := "", "", "", "", false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--seats" && i+1 < len(args) && act == "":
			act, val = "seats", args[i+1]
			i++
		case (a == "--pause" || a == "--resume") && act == "":
			act = a[2:]
		case (a == "--why" || a == "--for") && i+1 < len(args):
			if a == "--why" {
				why = args[i+1]
			} else {
				forM = args[i+1]
			}
			i++
		case a == "--force":
			force = true
		default:
			usage()
		}
	}
	if act == "" || (forM != "" && (act == "resume" || val == "-")) {
		usage()
	}
	now := time.Now()
	var err error
	k := map[string]string{"seats": "seats@" + m, "pause": "pause@" + m, "resume": "pause@" + m}[act]
	if act == "resume" || val == "-" {
		var a []string
		if a = []string{k}; force {
			a = append(a, "--force")
		}
		if dryM {
			a = append(a, "--dry-run")
		}
		err = knobUnset(a, now)
	} else {
		v := val
		if act == "pause" {
			v = "1"
		}
		a := []string{k, v, "--why", why}
		if forM != "" {
			a = append(a, "--for", forM)
		}
		if force {
			a = append(a, "--force")
		}
		if dryM {
			a = append(a, "--dry-run")
		}
		err = knobSet(a, now)
	}
	if err != nil {
		die("%v", err)
	}
	if dryM { // 미리 보기는 조정 기록도 안 남긴다
		exit(0)
	}
	what := act
	if act == "seats" {
		what = "seats " + val
	}
	if e := appendLine(DATA+"/machine.log", strings.Join([]string{stampLog(), knobBy(), m, what, dash(flat1(forM)), dash(flat1(why))}, "\t")); e != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: 조정 기록을 남기지 못함: %v\n", e)
	}
	exit(0)
}
