package opsstate

// 2026-11-24 지휘: 「배포 밀림」 거짓 양성 — 레인이 착지하고 착지 뒤 배포가 도는 1~2분 사이에 「N커밋 밀림」 사건이 나갔고 1분 뒤 배포는
// 종료 0으로 끝났다. 배포가 도는 중·착지 뒤 명령 차례가 남음·마지막 착지 뒤 deploy_grace분 안이면(모으는 쪽이 Facts.DeployHold에 까닭)
// 밀림 항목을 내지 않고, 그 뒤에도 밀려 있으면 낸다. 고치기 전 코드로는 DeployHold가 없어 컴파일부터 실패한다

import "testing"

func TestAdviseDeployHold(t *testing.T) {
	s := aBase()
	s.Deploy = []Deploy{{Machine: "a", Version: "abc123456789", Behind: ip(5), Why: "배포 기록상 판"}}
	if it := kinds(Advise(s, Facts{DeployHold: "배포 도는 중(landed pid 42)"}, ath), "deploy"); len(it) != 0 {
		t.Fatalf("배포 도는 중이면 밀림 아님: %+v", it)
	}
	it := kinds(Advise(s, Facts{}, ath), "deploy")
	if len(it) != 1 || it[0].Key != "deploy:a" {
		t.Fatalf("그 뒤에도 밀려 있으면 항목 하나: %+v", it)
	}
}
