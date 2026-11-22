package main

import "testing"

// 착지 직전 묶음 제출마다 원격 refs/herdr/landing/<큐 이름> = 제출 SHA, 다음 바퀴 시작에 남은 표식을 지운다
func TestLandMarkUnmark(t *testing.T) {
	f := newFx(t)
	q1, s1, _, _ := f.submit(1000000001, "a", "")
	q2, s2, _, _ := f.submit(1000000002, "b", "")
	f.git(f.clone, "fetch", "-q", "origin", "+refs/merge-queue/*:refs/hc-merger/q/*")
	m := f.merger()
	m.b = []entry{{q: q1, s: s1, l: "a"}, {q: q2, s: s2, l: "b"}}
	m.landMark()
	for name, sha := range map[string]string{"refs/herdr/landing/1000000001-a": s1, "refs/herdr/landing/1000000002-b": s2} {
		if got := f.git(f.clone, "--git-dir="+f.origin, "rev-parse", name); got != sha {
			t.Fatalf("표식 %s = %q, 바람 %s", name, got, sha)
		}
	}
	m.qs = f.git(f.clone, "ls-remote", "origin", "refs/merge-queue/*", "refs/herdr/landing/*")
	m.landUnmark()
	if f.has("refs/herdr/landing/1000000001-a") || f.has("refs/herdr/landing/1000000002-b") {
		t.Fatal("남은 표식을 지우지 않았다")
	}
	if !f.has(q1) || !f.has(q2) {
		t.Fatal("큐 표식은 그대로여야")
	}
	m.qs = ""
	m.landUnmark() // 표식이 없으면 push하지 않는다(실패 글 없음)
}
