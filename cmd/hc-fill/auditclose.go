package main

import (
	"strconv"
	"strings"
)

// fill-audit-close(2026-11-24): 판정을 레인에 넘긴(판정 파일을 둔) 바퀴부터, 대기열이 띄운 그 감사 터미널이 끝남(done·idle)이면 닫는다 —
// 레인이 판정을 받자마자 착지·hc-lane clean으로 가도 끝난 감사 터미널이 작업 트리를 쥐고 있지 않게(하루 네 번 「clean 멈춤」).
// 닫는 것은 aCloseTerm과 같다: try.tsv에 확정한 실행·지금 이름이 둘 다 맞는 그 정확한 주소만. 일하는 중·닫기 실패·안 닿음이면
// 다음 바퀴에 다시, audit_close바퀴(기본 5)를 넘으면 그만두고 지금처럼 수거 뒤 정리(aCleanup)·정리 도구에 맡긴다. audit_close 0이면 끔.
// 진행은 차수 16칸 표시에: aclose=<센 바퀴>, 끝나면(닫음·그만둠) aclosed.
func aCloseDelivered() {
	lim := 5
	if v := conf("audit_close"); v != "" {
		if !isDigits(v) {
			say("알림: 설정 audit_close는 바퀴 수(숫자, 0이면 끔)다, 5로: %s", v)
		} else {
			lim = atoi10(v)
		}
	}
	if lim == 0 || dry || inlist("aclosed", dash(auAfl)) {
		return
	}
	qr := qrow(auAn)
	if qr == nil || qr.F(6) != "up" || qr.F(15) != "audit="+auLn {
		return // 안 뜬 줄·빠진 줄은 닫을 터미널이 없다(대기열 줄 정리는 aCleanup)
	}
	qm, qa := qr.F(7), qr.F(8)
	n := 0
	var rest []string
	for _, f := range strings.Split(dash(auAfl), ",") {
		if strings.HasPrefix(f, "aclose=") {
			n = atoi10(f[7:])
		} else if f != "" && f != "-" {
			rest = append(rest, f)
		}
	}
	done := false
	if auOK(qm) {
		switch alive(qa) {
		case "done", "idle":
			done = aCloseTerm(auAn, qa)
		}
	}
	if n++; done || n > lim {
		rest = append(rest, "aclosed")
	} else {
		rest = append(rest, "aclose="+strconv.Itoa(n))
	}
	auAfl = strings.Join(rest, ",")
	aupd(auAid, "16", auAfl)
}
