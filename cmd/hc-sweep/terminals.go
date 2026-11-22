package main

// 터미널 쓸기 한 바퀴: 이 기계 idle·done 터미널마다 판정(judge.go)하고 닫는다(closeTerm)

import (
	"fmt"
	"strings"
	"time"
)

// sweepTerminals: 터미널 쓸기 한 바퀴(요약 한 줄까지). 닫기가 실패·모름으로 끝난 수를 돌려준다(R-36 — 종료 코드에)
func sweepTerminals(terms any, dry bool) (failed int) {
	keep, keepOK := keepList()
	if !keepOK {
		fmt.Println("터미널 쓸기 건너뜀: ~/.claude-tasks/keep을 못 읽음(보호 목록을 모름)")
		return 0
	}
	now := float64(time.Now().UnixNano()) / 1e6
	list, _ := terms.([]any)
	var listed []map[string]any
	for _, x := range list {
		t, ok := x.(map[string]any)
		if ok && str(t, "state") == "live" && str(t, "address") != "" && !strings.Contains(str(t, "address"), "/") &&
			(str(t, "status") == "idle" || str(t, "status") == "done") {
			listed = append(listed, t)
		}
	}
	closed, targets := 0, 0
	for _, brief := range listed {
		t, err := hc("get", str(brief, "address"))
		if err != nil {
			if dry {
				fmt.Printf("남김: %s %s — hc get 실패\n", str(brief, "address"), dash(str(brief, "name")))
			}
			continue
		}
		f := facts{worktree: triOf(linkedWorktree(str(t, "cwd")))}
		var wtRepo, wtBranch string
		sub := submitted(t, "")
		if isWorker(t) && f.worktree == yes && (sub || now-orNow(t, now) >= 30*minute) {
			// 착지한 깨끗한 작업 트리: 닫고 hc-lane clean(교착 풀기)
			if wtRepo, wtBranch, f.wtwhy = landedClean(t, run, ""); wtRepo != "" {
				f.worktree, f.unlanded = no, no
			}
		}
		if isWorker(t) && f.worktree == no { // R2 보호에만 쓴다
			if wtRepo == "" {
				f.unlanded = triOf(unlanded(str(t, "cwd"), run))
			}
			tail := screenTail(str(t, "address"), run)
			f.tail, f.fill, f.submitted = &tail, fillState(str(t, "name"), str(t, "address"), ""), sub
		}
		why, kept := judge(t, f, now, keep)
		if why == "" {
			if dry {
				fmt.Printf("남김: %s %s — %s\n", str(t, "address"), dash(str(t, "name")), kept)
			}
			continue
		}
		created, changed := now, orNow(t, now)
		if v, ok := num(t, "created_ms"); ok && v != 0 {
			created = v
		}
		line := fmt.Sprintf("%s %s %s (만든 지 %d분·상태 그대로 %d분, turn %s) %s", why, str(t, "address"), dash(str(t, "name")),
			int((now-created)/minute), int((now-changed)/minute), pyStr(t["turn"]), pyStr(t["cwd"]))
		targets++
		if dry {
			if wtRepo != "" {
				line += " (닫은 뒤 hc-lane clean " + wtBranch + ")"
			}
			fmt.Println("닫을 것:", line)
			continue
		}
		res := closeTerm(t, why)
		switch {
		case strings.HasPrefix(res, "닫힘"):
			closed++
		case !strings.HasPrefix(res, "남김"):
			failed++
		}
		fmt.Println(res+":", line)
		if wtRepo != "" && strings.HasPrefix(res, "닫힘") {
			ok, msg := laneClean(wtRepo, wtBranch, run)
			fail := ""
			if !ok {
				fail = " 실패"
			}
			record(str(t, "address"), str(t, "name"), "작업 트리 정리(hc-lane clean "+wtBranch+")"+fail, wtRepo, "-", msg)
			fmt.Printf("작업 트리 정리%s: %s %s — %s\n", fail, wtRepo, wtBranch, msg)
		}
	}
	word, n := "닫음", closed
	if dry {
		word, n = "닫을 것", targets
	}
	fmt.Printf("%s 이 기계 idle·done %d개 확인, %s %d개\n", time.Now().Format("2006-01-02 15:04:05"), len(listed), word, n)
	return failed
}

func orNow(t map[string]any, now float64) float64 {
	if v, ok := num(t, "status_ms"); ok && v != 0 {
		return v
	}
	return now
}
