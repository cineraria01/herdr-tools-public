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
			(str(t, "status") == "idle" || str(t, "status") == "done" || (t["status"] == nil && shellName(str(t, "agent")))) {
			// 상태 없는 셸 칸도 본다 — hc keep이 셸로 바꾼 끝난 레인(judge가 hc get의 keep·착지 증거로 가른다)
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
		var dtRepo, dtTop string
		if f.worktree == yes && now-orNow(t, now) >= 30*minute {
			// 분리·깨끗한 읽기 전용 작업 트리(감사 스크립트의 worktree add --detach): 닫고 지운다(detachedAfterClose — =0이면 닫기만).
			// 다른 live 터미널이 그 폴더를 쓰거나 모르면 닫지도 지우지도 않는다(worktreeRemove가 지우기 직전 한 번 더 본다)
			var why string
			if dtRepo, dtTop, why = detachedClean(str(t, "cwd"), run); dtRepo != "" {
				if f.wtwhy = dirInUse(dtTop, str(t, "address"), run); f.wtwhy != "" {
					dtRepo = ""
				} else {
					f.worktree, f.unlanded, f.detached = no, no, true
				}
			} else if why != "" {
				f.wtwhy = why
			}
		}
		if (isWorker(t) || f.detached) && f.worktree == no { // R2 보호에만 쓴다
			if wtRepo == "" && !f.detached {
				f.unlanded = triOf(unlanded(str(t, "cwd"), run))
			}
			tail := screenTail(str(t, "address"), run)
			f.tail, f.fill, f.submitted = &tail, fillState(str(t, "name"), str(t, "address"), ""), sub
		}
		why, kept, q := settle(t, f, now, keep, run)
		if why == "" {
			// 쉼 판정으로 남긴 것은 실제 쓸기에서도 한 줄 — 그 터미널·세션·까닭 종류마다 한 번(closed.log)
			if dry || q.sig != "" && keptOnce(t, q, kept) {
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
			if dtRepo != "" && detachedRemoveOn() {
				line += " (닫은 뒤 git worktree remove)"
			} else if dtRepo != "" {
				line += " (작업 트리 남김 — 분리 지우기 꺼짐)"
			}
			fmt.Println("닫을 것:", line)
			continue
		}
		res := closeTerm(t, why, q)
		switch {
		case strings.HasPrefix(res, "닫힘"):
			closed++
		case !strings.HasPrefix(res, "남김"):
			failed++
		}
		fmt.Println(res+":", line)
		if strings.HasPrefix(res, "닫힘") {
			jobDrop(str(t, "address"), str(t, "name"))
		}
		if wtRepo != "" && strings.HasPrefix(res, "닫힘") {
			ok, msg := laneClean(wtRepo, wtBranch, run)
			fail := ""
			if !ok {
				fail = " 실패"
			}
			record(str(t, "address"), str(t, "name"), "작업 트리 정리(hc-lane clean "+wtBranch+")"+fail, wtRepo, "-", msg)
			fmt.Printf("작업 트리 정리%s: %s %s — %s\n", fail, wtRepo, wtBranch, msg)
		}
		if dtRepo != "" && strings.HasPrefix(res, "닫힘") {
			line, why, msg, bad := detachedAfterClose(dtRepo, dtTop, run)
			if why != "" {
				record(str(t, "address"), str(t, "name"), why, dtRepo, "-", msg)
			}
			if bad {
				failed++
			}
			fmt.Println(line)
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
