package opsstate

import "testing"

// 감사 atozipa 중간 1: 단계 글의 경과 분(「감사 대기(자리 없음 N분)」)만 달라서는 지문이 안 바뀐다. 차수·sha는 판단 칸이라 남는다
func TestFingerprintStageMinutes(t *testing.T) {
	a, b := fake(), fake()
	a.Lanes[0].Stage = "감사 대기(자리 없음 3분)"
	b.Lanes[0].Stage = "감사 대기(자리 없음 4분)"
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatalf("기다린 분만 다른데 지문이 바뀜: %s %s", Fingerprint(a), Fingerprint(b))
	}
	b.Lanes[0].Stage = "감사 대기(2번째)"
	c := fake()
	c.Lanes[0].Stage = "감사 대기(3번째)"
	if Fingerprint(b) == Fingerprint(c) {
		t.Error("감사 차수가 다른데 지문이 같음")
	}
}
