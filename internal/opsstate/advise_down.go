package opsstate

// 닿지 않는 기계 항목(machineItems의 한 단계, ht-advise-noise 2026-11-28): 그 기계의 살아 있는 레인을 레인마다 「같은 단계에 오래」로 내지
// 않고 이 항목 하나에 이름으로 묶는다 — 전원으로 몇 분 끊긴 워커의 끝난 옛 줄 175건이 레인마다 lane-long으로 나왔다

import (
	"fmt"
	"strings"
)

// downItem <기계> <살아 있는 레인>: 연결 안 됨이면 machine, 연결을 모르면(hc machine ls 못 읽음) 끊김으로 단정하지 않고 판정 못 함 —
// 둘 다 무엇 끝에 「 — 살아 있는 레인 n개(<앞 다섯> 외 k)」(없으면 안 붙임). 열쇠는 그대로 machine:<기계>(묶은 레인이 바뀌어도 새 사건 아님)
func downItem(m Machine, live []string) Item {
	var it Item
	if strings.Contains(m.Why, "모름") {
		it = unknown("machine:"+m.Name, "기계 "+m.Name, m.Why)
	} else {
		it = Item{Key: "machine:" + m.Name, Kind: "machine", What: "기계 " + m.Name + " 모름: " + or(m.Why, "까닭 모름"),
			Evidence: "그 기계 레인·자리를 잴 수 없다", Cost: "그 기계에 떠 있는 레인이 멈췄어도 모른다",
			Next: []string{"hc machine ls"}, Sure: SureJudge}
	}
	if len(live) > 0 {
		names := strings.Join(live[:min(5, len(live))], ", ")
		if len(live) > 5 {
			names += fmt.Sprintf(" 외 %d", len(live)-5)
		}
		it.What += fmt.Sprintf(" — 살아 있는 레인 %d개(%s)", len(live), names)
	}
	return it
}
