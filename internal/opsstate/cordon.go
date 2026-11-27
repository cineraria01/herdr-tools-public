package opsstate

// 저장소 읽기 막힘 격리(ht-worker-cordon, docs/hc-fill/cordon.md): 상태 한 장의 기계 칸 cordon과 advise 항목 하나.
// 고치는 것은 사람이다(그 기계 권한 창·코어) — 도구는 새 일만 막고 돌아오면 스스로 푼다

import "fmt"

// Cordon: 격리 중인 기계(아니면 Machine.Cordon nil)
type Cordon struct {
	SinceS int64  `json:"since_s"` // 읽기 막힘이 시작된 지 초
	Why    string `json:"why"`     // 마지막 까닭(프로젝트 폴더: 오류)
}

func cordonItem(m Machine) Item {
	return Item{Key: "cordon:" + m.Name, Kind: "cordon",
		What:     fmt.Sprintf("%s 격리 %d분째 — 저장소 폴더를 못 읽음", m.Name, m.Cordon.SinceS/60),
		Evidence: m.Cordon.Why + " · 사람이 볼 것: 그 기계에서 worker-mac-check(코어가 띄운 프로그램의 폴더 권한 — OS 권한 창)",
		Cost:     "그 기계에 새 레인·감사·병합 검증이 안 간다(떠 있는 레인은 그대로 — 읽기를 못 해 멈췄을 수 있다). 읽기가 돌아오면 다음 바퀴에 스스로 풂",
		Sure:     SureHuman}
}
