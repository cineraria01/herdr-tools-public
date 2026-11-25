package main

// nudge(2026-11-24 fill-edit-hands): 레인 터미널이 일시 오류로 턴을 끝내고 멈춘 것(에이전트 CLI가 「다시 보내라」로 끝냄, hc 상태
// error)을 다시 굴리는 정해진 한 줄 — 지휘가 손으로 hc send를 썼다. 터미널이 살아 있고 상태가 error·done인데 그 실행의 마지막 보고가
// 없거나 「중간」일 때만 글 하나를 보낸다. 레인 일을 대신하지 않고 프로세스를 끄지 않는다. 한 레인에 손잡이 nudge_every분 안에 한 번

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"herdr-tools/internal/hcjson"
)

func init() { register("nudge", nudgeCmd) }

const nudgeMsg = "일시 오류로 턴이 끊겼다 — 작업 트리 상태를 확인하고 지시문의 남은 단계부터 이어서"

// nudge <레인> [<글>] [--why <까닭>]. 0 보냄 · 1 안 보냄(까닭)·모름·보내기 실패 · 2 잘못 부름
func nudgeCmd(args []string) {
	args, why := takeWhy(args)
	if len(args) < 1 || len(args) > 2 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	name, msg := args[0], nudgeMsg
	if len(args) == 2 {
		if msg = strings.TrimSpace(args[1]); msg == "" {
			usage()
		}
	}
	r := qrowW(name)
	if r == nil {
		noLane(name)
	}
	addr, m := r.F(qAddr), r.F(qMach)
	t, found, err := laneTerm(r)
	switch {
	case t.Status != "error" && t.Status != "done":
		die("%s: 터미널 %s 상태가 %s 다 — error·done일 때만 보낸다(일하는 중·입력 기다림이면 그대로 둔다)", name, addr, dash(t.Status))
	case t.Created <= 0:
		die("%s: 터미널 %s 의 시작 시각을 모른다 — 이 실행의 보고를 가를 수 없어 보내지 않음", name, addr)
	}
	if st, ok := nudgeLast(name, addr, m, t.Created); !ok {
		die("%s: 레인 사건을 읽지 못했다(모름) — 보내지 않음", name)
	} else if st != "" && st != "중간" {
		die("%s: 이 실행이 이미 「%s」를 보고했다 — 멈춘 것이 아니라 끝난 것이라 보내지 않음", name, st)
	}
	every := knobInt("nudge_every")
	lf := lockWait(DATA+"/nudge.lock", 30*time.Second)
	if lf == nil {
		die("다른 hc-fill nudge가 끝나지 않았다 — 다시 부른다")
	}
	defer lf.Close()
	now := time.Now().Unix()
	if at := nudgeAt(name); every > 0 && at > 0 && now-at < int64(every)*60 {
		die("%s: %d분 전에 이미 보냈다(손잡이 nudge_every %d분) — 그 뒤에도 멈춰 있으면 지휘가 들여다본다", name, (now-at)/60, every)
	}
	state := "상태 " + t.Status + " · 주소 " + addr
	if b := hcjson.SendBlocked(t, found, err); b != "" {
		jot("nudge", name, why, "안 보냄", "-", state+" · "+b)
		die("%s: 보내지 않음 — 받는 쪽 %s", name, b)
	}
	if !auSendTo(addr, msg, t, found, err) {
		jot("nudge", name, why, "실패", "-", state)
		die("%s: hc send %s 실패 — 보냈는지 모른다", name, addr)
	}
	if err := appendLine(DATA+"/nudge.tsv", Row{name, addr, strconv.FormatInt(now, 10)}.Line()); err != nil {
		fmt.Printf("알림: 보낸 기록을 못 남겼다(%v) — 다음 nudge가 간격을 모른다\n", err)
	}
	say("보냄 %s → %s: %s", name, addr, msg)
	jot("nudge", name, why, "보냄", "-", state)
}

// laneTerm <대기열 줄>: 그 줄이 기록한 지금 run 터미널(주소 qAddr)의 hc get 결과. 같은 이름의 앞 run 터미널은 보지 않는다(hc ls로
// 이름을 찾지 않는다). 떠 있지 않음·터미널 없음(끝남)·이름 다름(주소 재사용)·모름이면 까닭과 종료 1 — nudge·tell이 함께 쓴다
func laneTerm(r Row) (hcjson.Terminal, bool, error) {
	name, addr := r.F(qName), r.F(qAddr)
	if r.F(qState) != "up" || dash(addr) == "-" {
		die("%s: 떠 있는 레인이 아니다(대기열 상태 %s) — 보낼 터미널이 없다", name, r.F(qState))
	}
	t, found, err := hcGet(addr)
	switch {
	case err != nil:
		die("%s: 터미널 %s 상태를 모른다(%v) — 보내지 않음", name, addr, err)
	case !found || t.State == "finished":
		die("%s: 터미널 %s 가 없다(끝남) — 보내지 않음. 다시 띄울지는 지휘가 정한다", name, addr)
	case !t.HasName || t.Name != name:
		die("%s: 주소 %s 의 터미널 이름이 %q 다(주소 재사용) — 보내지 않음", name, addr, t.Name)
	}
	return t, found, err
}

// nudgeLast <레인> <주소> <기계> <created_ms>: 그 실행(주소@시작)의 마지막 보고 상태(없으면 빈 값). ok=false면 사건을 못 읽음(모름)
func nudgeLast(name, addr, m string, created int64) (string, bool) {
	ev, ok := laneEvents(m)
	if !ok {
		return "", false
	}
	saved := EV
	defer func() { EV = saved }()
	EV = ev
	run := addr[strings.LastIndexByte(addr, '/')+1:] + "@" + strconv.FormatInt(created, 10)
	return evF(runlast(name, addr, run, ""), evState), true
}

// laneEvents <기계>: 그 기계의 레인 사건 줄(hc-lane events [--on]). ok=false면 못 읽음·그 기계 모름 — nudge·edit(주차 맞추기)이 함께 쓴다
func laneEvents(m string) ([]string, bool) {
	args := []string{"events"}
	if m != LOCAL && m != "" && m != "-" {
		args = append(args, "--on", m)
	}
	out, rc := Cmd{Name: "hc-lane", Args: args}.Run()
	if rc != 0 {
		return nil, false
	}
	ev := lines(sub(out))
	return ev, !has(ev, "모름 "+m)
}

// nudgeAt <레인>: 그 레인에 마지막으로 보낸 시각(초, 없으면 0)
func nudgeAt(name string) int64 {
	var at int64
	for _, r := range readRows(DATA + "/nudge.tsv") {
		if r.F(nuLane) == name {
			if v, err := strconv.ParseInt(r.F(nuAt), 10, 64); err == nil && v > at {
				at = v
			}
		}
	}
	return at
}
