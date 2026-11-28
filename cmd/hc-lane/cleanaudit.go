package main

// clean이 막히던 끝난 감사 터미널(2026-11-25 ht-clean-unstick): 레인이 판정을 받자마자 착지·clean으로 가면, hc-fill이 띄운 감사 터미널이
// 판정을 적고 끝난 채 작업 트리를 쥐고 있어 clean이 멈췄다(hc-fill은 다음 바퀴에야 닫는다 — 레인이 1분 안에 더 빨랐다).
// clean은 그 터미널이 이 레인 실행의 판정을 낸 감사이고 끝났으면(done·idle) 그 주소로 닫는다. 일하는 중·모름·다른 터미널은 그대로
// 두어 termGate가 지금처럼 멈춘다. 정리가 끝나면 마지막 report가 막힘일 때 「막힘 풀림」 중간을 적어 레인 단계의 막힘을 푼다.

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/rest"
)

// cleanLane: clean을 이 레인 터미널이 자기 가지에 부를 때의 레인 이름(HC_LANE이 가지·--name과 같음). 아니면 빈 값 — 다른 기계(--on)·지휘가
// 부른 clean은 감사 터미널을 닫지 않고 사건도 적지 않는다
func cleanLane() string {
	ln := os.Getenv("HC_LANE")
	if on != "" || os.Getenv("HC_TERMINAL_ID") == "" || ln == "" || (ln != branch && ln != name) {
		return ""
	}
	return ln
}

// auditCloseBusy: busy(「<주소> <폴더>」 줄) 가운데 닫은(dry-run이면 닫을) 감사 터미널을 뺀 나머지. 닫는 것은 모두 맞을 때만:
// 살아 있음·status done·idle · 이름 audit-<ID> · 판정 파일 verdicts/<레인>/<ID>.md가 꼴이 맞고 이 실행(run) 것 · 수거 표시(<ID>.ack)가 있음
func auditCloseBusy(busy []string) []string {
	ln := cleanLane()
	if ln == "" || len(busy) == 0 || !lvmyrun() {
		return busy
	}
	var left []string
	for _, b := range busy {
		a, _, _ := strings.Cut(b, " ")
		if !auditDone(ln, a) {
			left = append(left, b)
			continue
		}
		// 상태 done·idle은 턴 끝일 뿐 — 안에서 일이 돌거나 모르면 닫지 않고 termGate가 지금처럼 멈춘다(ht-rest-one-gate)
		if v := rest.Ask(restRun, a, ""); v.State != rest.Quiet {
			fmt.Printf("hc-lane clean: 끝난 감사 터미널 %s 남김 — %s\n", a, rest.Why(v))
			left = append(left, b)
			continue
		}
		if dry {
			fmt.Printf("%s끝난 감사 터미널 %s 를 닫는다(판정 %s 수거됨)\n", dryP(), a, lva)
			continue
		}
		// 실패해도 rest에 남기지 않는다 — hc-fill 바퀴가 먼저 닫았을 수 있어, termGate가 목록을 다시 보고 살아 있으면 그때 멈춘다(감사 atp1pxd 낮음 2)
		if _, r := hcRun(30, errDrop, "kill", a); r != 0 {
			errf("hc-lane clean: 끝난 감사 터미널 %s 닫기 실패(종료 %d) — 목록을 다시 본다\n", a, r)
			continue
		}
		fmt.Printf("hc-lane clean: 끝난 감사 터미널 닫음 %s (판정 %s 수거됨)\n", a, lva)
	}
	return left
}

// restRun: rest.Ask의 실행기 — 이 기계의 hc-sweep --rest(표준 오류 버림, 상한 90초 — 못 띄움·시간 넘김은 모름)
func restRun(a []string) (string, int) { return capt(90, errDrop, a[0], a[1:]...) }

// auditDone: 그 주소가 이 레인 실행의 판정을 수거한 끝난 감사 터미널인가
func auditDone(ln, a string) bool {
	t, found, err := hcGet(10, a)
	if err != nil || !found || t.State != "live" || (t.Status != "done" && t.Status != "idle") {
		return false
	}
	aid, ok := strings.CutPrefix(t.Name, "audit-")
	if !ok || lvload(ln, aid) != 0 || lvr != lvme {
		return false
	}
	st, err := os.Stat(lvx)
	return err == nil && st.Mode().IsRegular()
}

// unblockNote: 정리가 끝났는데 이 레인 터미널의 마지막 report가 막힘이면 「막힘 풀림」 중간을 적는다(hc-fill 단계·advise가 막힘을 놓게)
func unblockNote() {
	ln := cleanLane()
	if dry || ln == "" {
		return
	}
	ls, err := readLines(evdir() + "/events.tsv")
	if err != nil {
		return
	}
	self := os.Getenv("HC_TERMINAL_ID")
	if last, _, _ := lastIn(ls, self[strings.LastIndex(self, "/")+1:], ln); last != "막힘" {
		return
	}
	if reportCmd(os.Stdout, os.Stderr, "중간", "-", "막힘 풀림: hc-lane clean 성공 — "+branch+" 작업 트리·가지 정리 끝") != 0 {
		errf("hc-lane clean: 「막힘 풀림」 사건을 적지 못했다 — 레인 단계에 막힘이 남는다\n")
	}
}
