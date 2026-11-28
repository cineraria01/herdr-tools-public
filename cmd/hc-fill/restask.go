package main

import (
	"time"

	"herdr-tools/internal/rest"
)

// restRun: rest.Ask의 실행기(ht-rest-one-gate — 터미널을 닫거나 대기열이 제출하기 직전에 그 터미널 기계의 hc-sweep --rest에 묻는다).
// 표준 오류는 버린다(답은 표준 출력의 REST 줄), 상한은 원격 hc exec --timeout보다 30초 길게. 못 띄움·시간 넘김은 0 아닌 종료 = 모름
func restRun(a []string) (string, int) {
	return Cmd{Name: a[0], Args: a[1:], Err: errDrop, Max: rest.AskMax + 30*time.Second}.Run()
}
