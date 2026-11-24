package opsstate

// 감사 atozuyd-gi866o [중간]: 보류(DeployHold)는 새 밀림의 첫 알림만 미룬다 — 이미 알린 열쇠(Known)면 보류 동안에도 항목이 남는다
// (빠지면 사건 기록·운영 에이전트 시도 기록이 열쇠를 잊는다). 고치기 전 코드로는 Known이 없어 컴파일부터 실패한다

import "testing"

func TestAdviseDeployHoldKnown(t *testing.T) {
	s := aBase()
	s.Deploy = []Deploy{{Machine: "a", Version: "abc123456789", Behind: ip(5)}, {Machine: "b", Version: "abc123456789", Behind: ip(5)}}
	it := kinds(Advise(s, Facts{DeployHold: "배포 도는 중", Known: map[string]bool{"deploy:a": true}}, ath), "deploy")
	if len(it) != 1 || it[0].Key != "deploy:a" {
		t.Fatalf("알린 a는 남고 새 b는 미룸: %+v", it)
	}
}
