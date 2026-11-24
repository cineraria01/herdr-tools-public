package main

// 끝난 레인 정리(gc_tick·gc_all)·놀고 있음 알림(idle_tick) — 셸 판 lib/hc-fill/tick.sh

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"herdr-tools/internal/lockx"
	"herdr-tools/internal/proc"
)

func init() { register("_gc", gcMain) }

// gcMain: run이 배경으로 띄운 정리(_gc <잠금 경로>). 잠금은 이 프로세스가 쥔다(끝나면 풀림)
func gcMain(args []string) {
	gcLock = DATA + "/gc.lock"
	if len(args) > 0 {
		gcLock = args[0]
	}
	f, busy, err := lockx.TryLock(gcLock)
	if err != nil || busy {
		fmt.Fprintf(os.Stderr, "lockf: %s: already locked\n", gcLock)
		busyExit()
	}
	heldLocks = append(heldLocks, f)
	dry = false
	say("--- %s", stampLog())
	gcAll()
	exit(0)
}

// gc_tick: 설정 gc 분마다 한 번, 바퀴 밖 배경(<SELF> _gc <잠금>)으로. 잠금이 쥐여 있으면 앞 정리가 아직 도는 것
func gcTick() {
	g := conf("gc")
	if g == "" {
		g = "60"
	}
	if !isDigits(g) {
		say("알림: 설정 gc는 분(숫자)이다, 정리 건너뜀: %s", g)
		return
	}
	gm := atoi10(g)
	if gm == 0 {
		return
	}
	s, _ := readFile(DATA + "/gc.last")
	gl := int64(0)
	if v := sub(s); isDigits(v) {
		gl, _ = strconv.ParseInt(v, 10, 64)
	}
	gn := nowUnix()
	if gn-gl < int64(gm*60) {
		return
	}
	if dry {
		gcAll()
		return
	}
	lock := DATA + "/gc.lock"
	f, busy, err := lockx.TryLock(lock)
	if err != nil {
		say("gc 잠금 실패(종료 1, %s/gc.lock) — 이번 바퀴는 안 띄움", DATA)
		return
	}
	if busy {
		say("gc: 앞 정리가 아직 돈다(%s/gc.lock) — 이번 바퀴는 안 띄움", DATA)
		return
	}
	f.Close()
	if err := writeAtomic(DATA+"/gc.last", strconv.FormatInt(gn, 10)+"\n"); err != nil {
		say("gc 잠금 실패(종료 1, %s/gc.lock) — 이번 바퀴는 안 띄움", DATA)
		return
	}
	log, err := os.OpenFile(DATA+"/gc.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		say("gc 잠금 실패(종료 1, %s/gc.lock) — 이번 바퀴는 안 띄움", DATA)
		return
	}
	defer log.Close()
	c := exec.Command(SELF, "_gc", lock)
	c.Stdout, c.Stderr = log, log
	proc.Group(c) // 바퀴와 따로(바퀴가 끝나도 정리는 이어 감)
	if err := c.Start(); err != nil {
		say("gc 잠금 실패(종료 1, %s/gc.lock) — 이번 바퀴는 안 띄움", DATA)
		return
	}
	pid := c.Process.Pid
	c.Process.Release()
	say("gc: 배경으로 띄움(pid %d) — 출력 %s/gc.log", pid, DATA)
}

// gc_all: repo 줄마다 hc-lane gc(출력은 표준 출력). dry면 --dry-run
func gcAll() {
	gcon := connected()
	for _, f := range confRows("repo") {
		gp, gmach := F1(f, 1), F1(f, 2)
		gr := repoOf(gp, gmach)
		if gr == "" {
			continue
		}
		var a []string
		switch {
		case gmach == LOCAL:
			a = []string{"--integ"}
		case has(gcon, gmach):
			a = []string{"--on", gmach}
		default:
			say("gc 건너뜀 %s @%s: 연결 안 됨", gp, gmach)
			continue
		}
		if dry {
			a = append(a, "--dry-run")
		}
		out, rc := Cmd{Name: "hc-lane", Args: append(append([]string{"gc"}, a...), "--repo", gr), Err: errMerge}.Run()
		ls := lines(sub(out))
		if len(ls) == 0 {
			ls = []string{""}
		}
		for _, l := range ls {
			fmt.Printf("gc %s @%s: %s\n", gp, gmach, l)
		}
		if rc != 0 {
			say("gc 실패 %s @%s (종료 %d)", gp, gmach, rc)
		}
	}
}

// idle_tick: 바퀴 끝에도 자리가 남은 기계가 설정 idle분 넘게 이어지면 사건 한 번(사유마다 — 상한 cap·겹침 ovl·대기열 빔 none)
func idleTick() {
	if IDLE == 0 || dry {
		return
	}
	inow := nowUnix()
	var inew []Row
	type due struct{ k, m, msg string }
	var idl []due
	old := readRows(DATA + "/idle.tsv")
	for _, mc := range machines() {
		m := mc.Name
		ik, ion := "none", 0
		if ECAPHIT && (inlist("-", ECAPM) || inlist(m, ECAPM)) {
			ik = "cap"
		} else {
			for _, o := range OVWAIT {
				oo, op := o.F(wtOnly), o.F(wtProj)
				if op == "" || (oo != "-" && !inlist(m, oo)) {
					continue
				}
				if repoOf(op, m) != "" {
					ion++
				}
			}
			if ion != 0 {
				ik = "ovl"
			}
		}
		hasRepo := false
		for _, f := range confRows("repo") {
			if F1(f, 2) == m {
				hasRepo = true
			}
		}
		if !hasRepo || (m != LOCAL && !connHas(m)) || machPaused(m) { // 멈춘 기계(hc-fill machine --pause)의 빈 자리는 알리지 않는다
			continue
		}
		fr := int(num(mc.Cap)) - agentsOn(m)
		if mc.LCap != "-" {
			if l := int(num(mc.LCap)) - lanesOn(m); l < fr {
				fr = l
			}
		}
		if fr <= 0 {
			continue
		}
		var x Row
		for _, r := range old {
			if r.F(idMach) == m {
				x = r
				break
			}
		}
		// 셸 판은 「print $2, $3, $4, $5, $6」을 read로 나눴다 — 빈 칸은 뒤로 밀린다
		vals := strings.Fields(strings.Join([]string{x.F(idSince), x.F(idSent), x.F(idCount), x.F(idFree), x.F(idWhy)}, " "))
		get := func(i int) string {
			if i < len(vals) {
				return vals[i]
			}
			return ""
		}
		isinceS, isentS, icntS, ifrS, ikind := get(0), get(1), get(2), get(3), get(4)
		var isince, isent, icnt int64
		if !isDigits(isinceS) {
			isince, isent, icnt = inow, 0, 0
		} else {
			isince, _ = strconv.ParseInt(isinceS, 10, 64)
			if isDigits(isentS) {
				isent, _ = strconv.ParseInt(isentS, 10, 64)
			}
			if isDigits(icntS) {
				icnt, _ = strconv.ParseInt(icntS, 10, 64)
			}
		}
		if isDigits(ifrS) {
			if ifr, _ := strconv.Atoi(ifrS); fr < ifr {
				isince, isent, icnt = inow, 0, 0
			}
		}
		if ikind != "cap" && ikind != "ovl" {
			ikind = "none"
		}
		if isent == 0 {
			ikind = ""
		}
		inew = append(inew, Row{m, strconv.FormatInt(isince, 10), strconv.FormatInt(isent, 10), strconv.FormatInt(icnt, 10), strconv.Itoa(fr), ikind})
		if inow-isince < int64(IDLE*60) {
			continue
		}
		igap := int64(1800)
		if icnt >= 3 {
			igap = 7200
		}
		if !(isent == 0 || ikind != ik || (ik == "cap" && inow-isent >= igap)) {
			continue
		}
		imx := fmt.Sprintf("%s 자리 %d개 %d분째 빔", m, fr, (inow-isince)/60)
		if ik == "ovl" {
			imx += fmt.Sprintf("(겹침 대기 %d줄)", ion)
		}
		idl = append(idl, due{ik, m, imx})
	}
	ito := ""
	if len(idl) > 0 {
		iall := false
		var owners []string
		for _, r := range qRows() {
			if r.F(qState) == "wait" {
				owners = append(owners, dash(r.F(qOwner)))
			}
		}
		sort.Strings(owners)
		for i, o := range owners {
			if i > 0 && owners[i-1] == o {
				continue
			}
			if o == "-" {
				iall = true
				continue
			}
			if rc, _ := ownerCheck(o); rc != 1 { // 없음이 확인된 주인만 뺀다(모름을 빼면 받는 쪽이 모든 지휘로 넓어진다 — 재감사 R-09)
				if ito != "" {
					ito += ","
				}
				ito += o
			}
		}
		if iall {
			ito = ""
		}
	}
	for _, ik := range []string{"cap", "ovl", "none"} {
		var dm, msgs []string
		for _, d := range idl {
			if d.k == ik {
				dm, msgs = append(dm, d.m), append(msgs, d.msg)
			}
		}
		if len(dm) == 0 {
			continue
		}
		// 대기열에 띄울 것이 없는 빈 자리는 지휘가 할 일이 없다 — 기본은 안 보낸다(2026-11-22 go-alert-quiet: 사건 절반이 이 줄).
		// 설정 idle_empty분을 주면 그 간격에 한 번, 그동안 빈 기계를 묶어서. 간격은 idle.tsv에 남은 none 줄의 마지막 보낸 때로 잰다
		// ponytail: 자리가 찼다 비어 그 기계 줄이 지워지면 그 기계 보낸 때도 잊는다 — 다른 기계 줄이 없으면 간격보다 빨리 다시 갈 수 있다
		if ik == "none" {
			ie := int64(atoi10(conf("idle_empty")))
			last := int64(0)
			for _, r := range inew {
				if r.F(idWhy) == "none" && int64(num(r.F(idSent))) > last {
					last = int64(num(r.F(idSent)))
				}
			}
			if ie == 0 || inow-last < ie*60 {
				continue
			}
		}
		imsg := strings.Join(msgs, " · ")
		var iwhy string
		switch ik {
		case "cap":
			iwhy = "먼저 띄움 상한(early " + ECAP + ")에 걸려 남김 — 상한을 올리거나 일을 더 넣는다"
		case "ovl":
			iwhy = "겹침 대기 — 앞 레인이 착지하면 뜬다(hc-fill ls)"
		default:
			iwhy = "대기열에 띄울 것 없음"
		}
		if hcLaneReport("중간", imsg+" — "+iwhy, "hc-fill-idle", ito, "") == 0 {
			to := ito
			if to == "" {
				to = "모든 지휘"
			}
			say("놀고 있음 알림 → %s: %s — %s", to, imsg, iwhy)
			for i, r := range inew {
				if !has(dm, r.F(idMach)) {
					continue
				}
				c := "1"
				if r.F(idWhy) == ik {
					c = strconv.Itoa(int(num(r.F(idCount))) + 1)
				}
				inew[i] = r.Set(idSent, strconv.FormatInt(inow, 10)).Set(idCount, c).Set(idWhy, ik)
			}
		} else {
			say("놀고 있음 알림 실패(hc-lane report) — 다음 바퀴에 다시")
		}
	}
	if err := writeAtomic(DATA+"/idle.tsv", rowsText(inew)); err != nil {
		fmt.Fprintf(os.Stderr, "hc-fill: %v\n", err)
	}
}
