package main

// 운영 일지(2026-11-24 ops-journal, docs/ops/journal.md): 배포·되돌림 한 번에 한 줄, 스스로 배포 켜고 끄기에 한 줄.
// 기계마다의 결과는 배포 기록(deploy.tsv, --log)이 정본이고 일지에는 그 요약만 — 두 번 적지 않는다(배포 기록 줄과 짝).
// 일지를 못 써도 배포는 그대로다(한 줄 알림).

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/journal"
)

// journalBy: 배포 기록의 부른 곳과 같은 값(「-」이면 사람)
func journalBy() string {
	if b := deployBy(); b != "-" {
		return b
	}
	return "사람"
}

// deployJournal: deployEndOnce 끝에서 — prev는 이번 줄을 붙이기 전의 배포 기록(되돌리는 명령 = 그 마지막 성공 판으로 --at)
func deployJournal(code int, sig bool, c12, res string, prev []logRow) {
	target := "모든 기계"
	if len(deployPicked) > 0 {
		target = strings.Join(deployPicked, " ")
	}
	undo := "-"
	for i := len(prev) - 1; i >= 0; i-- {
		if r := prev[i]; r.rc == "0" && r.c12 != "?" && r.c12 != "" {
			undo = strings.TrimSpace("herdr-sync --at " + r.c12 + " " + strings.Join(deployPicked, " "))
			break
		}
	}
	result := "됨"
	if code != 0 {
		result = fmt.Sprintf("실패: 종료 %d", code)
	}
	if sig {
		result = "신호로 멈춤"
	}
	e := journal.Entry{By: journalBy(), Tool: "herdr-sync", Action: deployKind, Target: target, Why: deployWhy, Result: result, Undo: undo,
		State: "판 " + c12 + " · " + res + " (자세히 herdr-sync --log)"}
	if sig { // 신호 처리기 — 읽지 않고 붙이기만
		if err := journal.Append(e); err != nil {
			fmt.Fprintf(os.Stderr, "herdr-sync: 일지를 남기지 못했다(조치는 그대로): %v\n", err)
		}
		return
	}
	journal.Record(os.Stderr, e)
}

// autoJournal: --auto on|off로 바뀐 때만
func autoJournal(on bool, why string) {
	act, undo := "auto off", "herdr-sync --auto on"
	if on {
		act, undo = "auto on", "herdr-sync --auto off"
	}
	journal.Record(os.Stderr, journal.Entry{By: journalBy(), Tool: "herdr-sync", Action: act, Target: "스스로 배포", Why: why, Result: "됨", Undo: undo})
}
