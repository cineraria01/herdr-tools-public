package main

// gofix3-gitref(재감사 아스트라 2바퀴 ②-C): 병합기의 원격 목록 판독 — gitx.LsRemote 계약

import (
	"strings"
	"testing"
)

// merger/R-201: 정확한 이름을 요청한 조회의 응답이 요청 끝 한 글자와 줄바꿈만 빠진 이름이면 모름 — 「표식 없음」(proof 1)으로 읽지 않는다
func TestGitrefProofRefTail(t *testing.T) {
	f := newFx(t)
	m := f.merger()
	a := strings.Repeat("a", 40)
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf '%s\trefs/hc-landed/box-' `+a+`; exit 0; }`)
	if m.proofload([]string{"box-x"}) {
		t.Fatalf("이름이 잘린 응답을 받아들였다: lk2=%q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf '%s\trefs/hc-landed/box-y\n' `+a+`; exit 0; }`)
	if m.proofload([]string{"box-x"}) {
		t.Fatalf("요청 밖 이름을 받아들였다: lk2=%q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && [ "$2" = --exit-code ] && exit 2`)
	if !m.proofload([]string{"box-x"}) || m.lk2 != "" {
		t.Fatalf("종료 2(맞는 참조 없음)는 표식 없음: %q", m.lk2)
	}
	fakeGitIn(t, `[ "$1" = ls-remote ] && { printf '%s\trefs/hc-landed/box-x\n' `+a+`; exit 0; }`)
	if !m.proofload([]string{"box-x"}) || refsha(m.lk2, "refs/hc-landed/box-x") != a {
		t.Fatalf("온전한 줄은 받아야: %q", m.lk2)
	}
}

// 꼴이 틀린 목록(m.qs)이면 남은 착지 중 표식을 지우지 않는다(무엇이 남았는지 모름)
func TestGitrefUnmarkBadList(t *testing.T) {
	f := newFx(t)
	q1, s1, _, _ := f.submit(1000000001, "a", "")
	f.git(f.clone, "--git-dir="+f.origin, "update-ref", "refs/herdr/landing/1000000001-a", s1)
	m := f.merger()
	m.qs = s1 + "\t" + q1 + "\n" + s1 + "\trefs/herdr/landing/1000000001-a\n" + "abcdef012345\trefs/herdr/landing/x\n"
	m.landUnmark()
	if !f.has("refs/herdr/landing/1000000001-a") {
		t.Fatal("꼴이 틀린 목록으로 표식을 지웠다")
	}
}
