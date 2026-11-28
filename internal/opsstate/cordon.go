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

// CordonKind: 격리 까닭 한 갈래 — 머리(hc-fill 점검이 까닭 앞에 붙인다)·advise에 보일 것·사람이 볼 것(hc-fill 알림도 이 글)
type CordonKind struct{ Pre, What, Look string }

// 격리 까닭 갈래(ht-probe-cordon — 점검 항목을 더할 때 이 표에 한 줄). 첫 줄 저장소 읽기는 머리가 없다(옛 기록 글 그대로)
var (
	CordonRepo     = CordonKind{"", "저장소 폴더를 못 읽음", "그 기계에서 worker-mac-check(코어가 띄운 프로그램의 폴더 권한 — OS 권한 창)"}
	CordonAuth     = CordonKind{CordonAuthPre, "원격 로그인 거절", "그 기계에서 원격 로그인 다시(`gh auth status`·다시 로그인)"}
	CordonWrite    = CordonKind{"원격 쓰기 거절 — ", "원격 쓰기 거절", "그 기계에서 원격 쓰기 권한·로그인(`gh auth status`의 권한 범위·다시 로그인) — 착지 push가 막힌다"}
	CordonPool     = CordonKind{"에이전트 풀 빔 — ", "에이전트 풀에 쓸 계정 없음", "그 기계의 풀 프록시 상태(설정 worker.proxy의 status — 계정 한도·로그인)"}
	CordonKeychain = CordonKind{"키체인 막힘 — ", "키체인 막힘", "그 기계 hc-sweep --health — 코어가 SSH 문맥으로 떴을 수 있다(hc-core-relaunch)"}
	CordonTmp      = CordonKind{"임시 폴더 못 씀 — ", "작업 임시 폴더를 못 씀", "그 기계 작업 임시 뿌리(설정 jobtmp.root)의 권한·디스크"}
	CordonKinds    = []CordonKind{CordonRepo, CordonAuth, CordonWrite, CordonPool, CordonKeychain, CordonTmp}
)

func cordonItem(m Machine) Item {
	has := make([]bool, len(CordonKinds))
	cur := -1
	for _, w := range strings.Split(m.Cordon.Why, "; ") {
		k := -1
		for i, c := range CordonKinds {
			if c.Pre != "" && strings.HasPrefix(w, c.Pre) {
				k = i
			}
		}
		if k < 0 && cur >= 0 { // 머리 있는 까닭 뒤의 조각은 그 오류 글 속 「; 」
			continue
		}
		if k < 0 {
			k = 0
		}
		has[k], cur = true, k
	}
	var what, look []string
	for i, c := range CordonKinds {
		if has[i] {
			what, look = append(what, c.What), append(look, c.Look)
		}
	}
	return Item{Key: "cordon:" + m.Name, Kind: "cordon",
		What:     fmt.Sprintf("%s 격리 %d분째 — %s", m.Name, m.Cordon.SinceS/60, strings.Join(what, "·")),
		Evidence: m.Cordon.Why + " · 사람이 볼 것: " + strings.Join(look, " · "),
		Cost:     "그 기계에 새 레인·감사·병합 검증이 안 간다(떠 있는 레인은 그대로 — 멈췄을 수 있다). 까닭이 돌아오면 다음 바퀴에 스스로 풂",
		Sure:     SureHuman}
}
