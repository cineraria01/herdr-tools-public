package main

// deploy-hold-landtime(2026-11-24 지휘): 착지 뒤 배포가 「다른 herdr-sync가 돌고 있다」(종료 75)로 물러나 막힘 사건이 모든 지휘에
// 갔다(손 배포가 같은 판을 이미 깔았다). 종료 75 = 일시 실패 — 상한(손잡이 onland_retry_wait) 안에서 기다렸다 다시 돌린다.
// ④는 고치기 전 코드로 실패한다(첫 75에서 바로 막힘)

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func lrFx(t *testing.T) {
	t.Helper()
	pu, pp := onlandRetryUnit, onlandRetryPoll
	onlandRetryUnit, onlandRetryPoll = 20*time.Millisecond, 5*time.Millisecond // 상한 10단위 = 200ms
	t.Cleanup(func() { onlandRetryUnit, onlandRetryPoll = pu, pp })
}

// ④ 첫 실행 75 → 기다렸다 다시, 그 사이 다른 배포가 깔아 「배포할 것 없음」 → 막힘 사건 없음, 성공 한 줄
func TestOnlandRetryAfterLockBusy(t *testing.T) {
	lrFx(t)
	d := t.TempDir()
	n := d + "/n"
	rec := onlandEnv(t, d, fmt.Sprintf(`if [ ! -e %s ]; then touch %s; echo 다른 herdr-sync가 돌고 있다; exit 75; fi; echo 배포할 것 없음`, n, n))
	if err := onlandPend("dev", "a", "-", "s1"); err != nil {
		t.Fatal(err)
	}
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	reps, _ := readFile(rec)
	if strings.Contains(reps, "막힘") || !strings.HasPrefix(reps, "report|중간|-|착지 뒤 dev 성공: 배포할 것 없음") || !strings.Contains(reps, "종료 75 뒤 다시") {
		t.Fatalf("75 뒤 다시 돌려 성공이어야: %s", reps)
	}
}

// ⑤ 상한까지 75 → 지금처럼 막힘
func TestOnlandRetryGivesUp(t *testing.T) {
	lrFx(t)
	d := t.TempDir()
	rec := onlandEnv(t, d, "echo 다른 herdr-sync가 돌고 있다; exit 75")
	if err := onlandPend("dev", "a", "-", "s1"); err != nil {
		t.Fatal(err)
	}
	if rc := onlandRun("dev"); rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	reps, _ := readFile(rec)
	if !strings.HasPrefix(reps, "report|막힘|-|착지 뒤 dev 실패(종료 75") {
		t.Fatalf("상한까지 75면 막힘: %s", reps)
	}
	if strings.Count(reps, "report|") != 1 {
		t.Fatalf("사건은 한 번: %s", reps)
	}
}

// 손잡이 0 → 다시 안 돌림(전과 같음)
func TestOnlandRetryOff(t *testing.T) {
	lrFx(t)
	d := t.TempDir()
	n := d + "/n"
	rec := onlandEnv(t, d, fmt.Sprintf(`if [ ! -e %s ]; then touch %s; exit 75; fi; echo 둘째`, n, n))
	confLines = append(confLines, "onland_retry_wait 0")
	if err := onlandPend("dev", "a", "-", "s1"); err != nil {
		t.Fatal(err)
	}
	onlandRun("dev")
	if reps, _ := readFile(rec); !strings.HasPrefix(reps, "report|막힘|-|착지 뒤 dev 실패(종료 75)") {
		t.Fatalf("손잡이 0이면 다시 안 돌림: %s", reps)
	}
}
