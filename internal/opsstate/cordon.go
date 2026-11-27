package opsstate

// 격리(ht-worker-cordon·ht-cordon-remote-auth, docs/hc-fill/cordon.md): 상태 한 장의 기계 칸 cordon과 advise 항목 하나.
// 고치는 것은 사람이다(그 기계 권한 창·코어·원격 로그인) — 도구는 새 일만 막고 돌아오면 스스로 푼다

import (
	"fmt"
	"strings"
)

// Cordon: 격리 중인 기계(아니면 Machine.Cordon nil)
type Cordon struct {
	SinceS int64  `json:"since_s"` // 가장 이른 격리 까닭이 시작된 지 초
	Why    string `json:"why"`     // 까닭들(「; 」로 — 저장소 읽기 「프로젝트 폴더: 오류」, 원격 로그인 CordonAuthPre + 「프로젝트 호스트: 오류」)
}

// CordonAuthPre: 원격 로그인 거절 까닭의 머리(hc-fill cordonauth.go가 붙인다)
const CordonAuthPre = "원격 로그인 거절 — "

func cordonItem(m Machine) Item {
	repo, auth := false, false
	for _, w := range strings.Split(m.Cordon.Why, "; ") {
		if strings.HasPrefix(w, CordonAuthPre) {
			auth = true
		} else if !auth { // 원격 까닭 뒤의 조각은 그 오류 글 속 「; 」
			repo = true
		}
	}
	var what, look []string
	if repo || !auth {
		what, look = append(what, "저장소 폴더를 못 읽음"), append(look, "그 기계에서 worker-mac-check(코어가 띄운 프로그램의 폴더 권한 — OS 권한 창)")
	}
	if auth {
		what, look = append(what, "원격 로그인 거절"), append(look, "그 기계에서 원격 로그인 다시(`gh auth status`·다시 로그인)")
	}
	return Item{Key: "cordon:" + m.Name, Kind: "cordon",
		What:     fmt.Sprintf("%s 격리 %d분째 — %s", m.Name, m.Cordon.SinceS/60, strings.Join(what, "·")),
		Evidence: m.Cordon.Why + " · 사람이 볼 것: " + strings.Join(look, " · "),
		Cost:     "그 기계에 새 레인·감사·병합 검증이 안 간다(떠 있는 레인은 그대로 — 멈췄을 수 있다). 까닭이 돌아오면 다음 바퀴에 스스로 풂",
		Sure:     SureHuman}
}
