package main

// 배포 기록(2026-11-24 deploy-log-rollback — 사용자 「자동 배포를 위해 배포 기록, 되돌리기도 만들자」): 누가·어느 커밋을·어느 기계에
// 깔았는지 남는 곳이 없었다(bin.tsv 셋째 칸은 소스 판이고 운영 기록은 실패만 적는다).
//   STATE/deploy.tsv  배포(--check 아님)마다 끝날 때 한 줄 — 실패·부분 실패·도중 멈춤(die)도. 손대기 전에 멈춘 거절(쓴 것도 기계 줄도
//                     없음)은 적지 않는다(「아무 데도 안 씀」 계약). 칸(탭):
//                     시각 · 종류(배포|되돌림) · 종료 코드 · 범용 커밋 12자 · 소스 판(bin.tsv 셋째 칸과 같은 값, 되돌림은 at:<커밋>) ·
//                     개인 커밋 12자 · 부른 곳 · 기계=결과…(빈칸 나눔, 결과 같음|씀|충돌|실패|건너뜀 — 닿기 전에 멈춘 기계는 없다)
//                     소스 판 칸이 --status가 기계의 bin.tsv 소스 판을 커밋으로 풀어 읽는 짝이다(bin.tsv 꼴은 그대로).
//   STATE/deploying   도는 배포의 「pid 시작초 종류」 — 잠금을 쥔 뒤 쓰고 끝날 때 지운다(--status가 「지금 도는 배포」로 읽는다)

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"herdr-tools/internal/proc"
)

var (
	deploying      bool   // 이번 실행을 기록한다(--check·--status·--log 아님)
	deployReached  bool   // 기계 맞추기(syncMachines)·되돌리기 깔기에 들어섰다 — 그 전 멈춤은 쓴 것이 없으면 거절
	deployKind     = "배포" // 배포 | 되돌림
	deployC12      string // 깐 바이너리 판(binsPrepare·rollback이 채움, 비면 범용 저장소 HEAD)
	deploySrc      = "-"  // 그 소스 판
	results        = map[string]string{}
	resultOrder    []string
	wrote          = map[string]bool{} // 기계마다 이번에 쓴 것이 있나(say 줄로 안다)
	writeWords     = map[string]bool{"씀": true, "받음": true, "지음": true, "지움": true, "깖": true, "가져옴": true}
	deployWhy      string   // --why (운영 일지)
	deployPicked   []string // 부를 때 고른 기계(없으면 모든 기계)
	deployLogFile  = func() string { return STATE + "/deploy.tsv" }
	deployMarkFile = func() string { return STATE + "/deploying" }
)

// track: 알림 줄 「<기계> 씀|받음|지음|지움|깖|가져옴 …」이면 그 기계를 「씀」으로 — 쓰는 자리마다 표시를 달지 않고 줄 꼴 한 곳에서
func track(s string) {
	if f := strings.Fields(s); len(f) >= 2 && writeWords[f[1]] {
		wrote[f[0]] = true
	}
}

// result: 기계 한 줄의 결과(뒤에 온 것이 이긴다 — 맥 줄은 저장소 충돌 때 한 번, 끝에 한 번)
func result(m, r string) {
	if _, ok := results[m]; !ok {
		resultOrder = append(resultOrder, m)
	}
	results[m] = r
}

// rowResult: row()의 다름 묶음·충돌 수 → 결과
func rowResult(name string, diffs bool, conf int) {
	switch {
	case diffs:
		result(name, "실패")
	case conf > 0:
		result(name, "충돌")
	case wrote[name]:
		result(name, "씀")
	default:
		result(name, "같음")
	}
}

// deployStart: 잠금을 쥔 뒤 — 도는 배포 표식
func deployStart() {
	deploying = true
	os.WriteFile(deployMarkFile(), []byte(fmt.Sprintf("%d\t%d\t%s\n", os.Getpid(), time.Now().Unix(), deployKind)), 0o644)
}

// deployBy: 부른 곳 — HERDR_SYNC_BY(스스로 배포는 「스스로」), 없으면 hc 터미널, 없으면 -
func deployBy() string {
	for _, k := range []string{"HERDR_SYNC_BY", "HC_TERMINAL_ID"} {
		if v := strings.Fields(os.Getenv(k)); len(v) > 0 {
			return v[0]
		}
	}
	return "-"
}

func head12(dir string) string {
	if h, rc := gitq("-C", dir, "rev-parse", "HEAD"); rc == 0 && len(h) >= 12 {
		return h[:12]
	}
	return "?"
}

// deployEnd: 끝날 때(exit) 한 번 — 기록 한 줄을 붙이고 표식을 지운다. 기록을 못 쓰면 경고만(종료 코드는 그대로).
// deployEndSig: 신호 처리기 쪽. 끝과 신호가 겹쳐도 한 번만 — 먼저 온 쪽이 쓰고 늦은 쪽은 기다리지 않고 돌아간다(신호 뒤에는 proc이
// 새 자식을 돌려주지 않으니, 본 줄이 git에 걸린 채 신호 처리기가 기다리면 끝나지 않는다)
var endTaken atomic.Bool

func deployEnd(code int) {
	if endTaken.CompareAndSwap(false, true) {
		deployEndOnce(code, false)
	}
}

func deployEndSig(code int) {
	if endTaken.CompareAndSwap(false, true) {
		deployEndOnce(code, true)
	}
}

// deployEndOnce: sig면 자식(git)을 띄우지 않고(신호 뒤 proc은 돌려주지 않는다) 본 줄이 쓰던 결과 표도 읽지 않는다(동시에 쓰는
// 지도를 읽으면 런타임이 멈춘다) — 아는 판과 「신호로 멈춤」만
func deployEndOnce(code int, sig bool) {
	if !deploying {
		return
	}
	deploying = false
	os.Remove(deployMarkFile())
	if !sig && !deployReached && len(wrote) == 0 {
		return // 손대기 전에 멈춘 실행(더러운 저장소·옛 묶음 이름·저장소 충돌·개인 값·없는 판 등의 거절)은 적지 않는다 — 「아무 데도 안 씀」 계약
	}
	c12, conf := deployC12, "?"
	if !sig {
		if c12 == "" {
			c12 = head12(REPO)
		}
		conf = head12(CONF)
	}
	if c12 == "" {
		c12 = "?"
	}
	rs := []string{"신호로 멈춤"}
	if !sig {
		rs = nil
		if r, ok := results[LOCAL]; ok { // 표처럼 이 기계 먼저
			rs = append(rs, LOCAL+"="+r)
		}
		for _, m := range resultOrder {
			if m != LOCAL {
				rs = append(rs, m+"="+results[m])
			}
		}
	}
	prev, _ := readLog() // 되돌리는 명령을 이번 줄을 붙이기 전에
	defer deployJournal(code, sig, c12, strings.Join(rs, " "), prev)
	line := strings.Join([]string{time.Now().Format(time.RFC3339), deployKind, strconv.Itoa(code), c12, deploySrc, conf,
		deployBy(), strings.Join(rs, " ")}, "\t") + "\n"
	f, err := os.OpenFile(deployLogFile(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err == nil {
		_, err = f.WriteString(line)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: 배포 기록을 쓰지 못했다 (%s): %v\n", deployLogFile(), err)
	}
}

// logRow: 기록 한 줄의 칸
type logRow struct{ when, kind, rc, c12, src, conf, by, res string }

func readLog() ([]logRow, error) {
	b, err := os.ReadFile(deployLogFile())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r []logRow
	for _, l := range splitLines(string(b)) {
		f := strings.Split(l, "\t")
		if len(f) < 8 {
			continue
		}
		r = append(r, logRow{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7]})
	}
	return r, nil
}

// showLog: --log [n] — 마지막 n줄(기본 20), 오래된 것부터
func showLog(args []string) int {
	args, js := takeJSON(args)
	n := 20
	if len(args) > 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if len(args) == 1 {
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		n = v
	}
	rs, err := readLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "herdr-sync: 배포 기록을 읽지 못했다 (%s): %v\n", deployLogFile(), err)
		return 1
	}
	if len(rs) > n {
		rs = rs[len(rs)-n:]
	}
	if js {
		return logJSON(rs)
	}
	if len(rs) == 0 {
		fmt.Println("배포 기록 없음")
		return 0
	}
	for _, r := range rs {
		fmt.Printf("%s %s 종료 %s · 범용 %s · 개인 %s · 부른 곳 %s · %s\n", r.when, r.kind, r.rc, r.c12, r.conf, r.by, localRes(r.res))
	}
	return 0
}

// running: 지금 도는 배포(표식의 pid가 살아 있을 때) — 「종류 pid n · m초째」, 없으면 "", 표식은 있는데 pid가 죽었으면 그렇다고
func running() string {
	b, err := os.ReadFile(deployMarkFile())
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		return "모름 (" + err.Error() + ")"
	}
	f := strings.Split(strings.TrimSpace(string(b)), "\t")
	pid, _ := strconv.Atoi(f[0])
	if len(f) < 3 || pid <= 0 {
		return "모름 (표식이 깨졌다: " + deployMarkFile() + ")"
	}
	if !proc.Alive(pid) {
		return ""
	}
	st, _ := strconv.ParseInt(f[1], 10, 64)
	return fmt.Sprintf("%s pid %d · %d초째", f[2], pid, time.Now().Unix()-st)
}
