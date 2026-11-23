package main

// conflict-fast C(2026-11-23 — 다시 맞춤 한 건을 뜯으면 레인 쪽 전체 검증 5.5분 뒤 병합기가 같은 전체 검증을 또 돌았다): 다시 맞춤
// 지시문 틀과 원래 레인에 보내는 글에 전체 검증(.claude/herdr-verify.sh) 줄이 없고, 푼 파일 둘레의 빌드·시험만 돌리는 줄이 있다.

import (
	"strings"
	"testing"
)

func TestRelandNarrowCheck(t *testing.T) {
	b := relandBrief("l-reland", "l", "100-l", "box", "a.go b.go", "/s/x.log", "ev", "")
	if strings.Contains(b, "herdr-verify.sh") || strings.Contains(b, "저장소 검증") {
		t.Errorf("다시 맞춤 지시문에 전체 검증 줄이 남음:\n%s", b)
	}
	if !strings.Contains(b, "푼 파일 둘레의 빌드·시험만") || !strings.Contains(b, "전체 검증은 병합기가") {
		t.Errorf("다시 맞춤 지시문에 좁은 확인 줄이 없음:\n%s", b)
	}
	s := relandSendMsg("a.go", "100-l")
	if strings.Contains(s, "검증 뒤") || !strings.Contains(s, "푼 파일 둘레의 빌드·시험만") {
		t.Errorf("원래 레인에 보내는 글: %q", s)
	}
}
