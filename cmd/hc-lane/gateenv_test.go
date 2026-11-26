package main

import (
	"os"
	"testing"
)

// ht-merger-gate-slot ⑦: 병합 검증 자리 값 HERDR_VERIFY_GATE는 병합기가 띄운 착지만 검증 스크립트에 물려준다 — 레인의 직접 착지(병합기 밖)는
// 물려받은 값을 지운다(HERDR_VERIFY_PARTS_FROM과 같은 자리). 남기면 레인 검증이 병합 검증 잠금을 쥐어 병합 검증 자리를 빼앗는다
func TestGateEnvOnlyInMerger(t *testing.T) {
	t.Setenv("HERDR_VERIFY_GATE", "1")
	specTreeTake(false)
	if _, ok := os.LookupEnv("HERDR_VERIFY_GATE"); ok {
		t.Fatal("병합기 밖 착지: HERDR_VERIFY_GATE를 지운다")
	}
	t.Setenv("HERDR_VERIFY_GATE", "1")
	specTreeTake(true)
	if os.Getenv("HERDR_VERIFY_GATE") != "1" {
		t.Fatal("병합기 안 착지: HERDR_VERIFY_GATE를 검증에 물려준다")
	}
}
