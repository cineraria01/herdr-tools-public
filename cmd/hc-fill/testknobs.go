package main

// 시험 손잡이 자리(2026-11-24 test-knobs-buildtag). 보통 빌드는 아무것도 하지 않고 그 환경 변수 이름조차 바이너리에 없다 —
// 상주하는 대기열 바이너리가 환경 변수 하나로 셸 명령을 돌리거나 스스로 죽지 않게. 시험 빌드(-tags herdrtest,
// testknobs_herdrtest.go — clitest.Bin이 그렇게 짓는다)와 이 꾸러미 단위 시험(testknobs_unit_test.go)만 채운다.
var (
	testCrashAt = func() string { return "" }                   // crash()가 멈출 지점
	testQsetCmd = func(name, state string) string { return "" } // qset이 잠금 직전에 돌릴 셸 명령(빈 값이면 안 돌림)
)
