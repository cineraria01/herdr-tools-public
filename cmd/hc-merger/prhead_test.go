package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// pr-reland-head: 다시 맞춤 레인이 짝 참조보다 먼저 제출해도 원래 PR 머리 가지(꼬리를 뗀 이름)를 찾는다
func TestPRLaneRelandTail(t *testing.T) {
	a, b, c := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	cases := []struct{ name, list, lane, want string }{
		{"① 짝 참조 없이 pr/X만", a + "\trefs/heads/pr/x", "x-reland", "x"},
		{"① 둘째 다시 맞춤", a + "\trefs/heads/pr/x", "x-reland2", "x"},
		{"② 짝 참조가 먼저", a + "\trefs/heads/pr/x\n" + b + "\trefs/heads/pr/y\n" + c + "\trefs/herdr/pr-of/x-reland/y", "x-reland", "y"},
		{"③ 제 머리 가지가 먼저", a + "\trefs/heads/pr/x\n" + b + "\trefs/heads/pr/x-reland", "x-reland", "x-reland"},
		{"④ pr/X 없음", b + "\trefs/heads/pr/other", "x-reland", "x-reland"},
		{"⑤ -reland 꼴 아님", a + "\trefs/heads/pr/x", "x-relandy", "x-relandy"},
		{"⑤ 꼬리만인 이름", a + "\trefs/heads/pr/", "-reland", "-reland"},
	}
	for _, k := range cases {
		if got := prLane(k.list, k.lane); got != k.want {
			t.Errorf("%s: prLane(%s) = %s, 바람 %s", k.name, k.lane, got, k.want)
		}
	}
	m := &M{qs: a + "\trefs/heads/pr/x", b: []entry{{l: "x-reland", s: b}}}
	if got := m.prHeads(); got != "x="+b+"="+a {
		t.Errorf("① prHeads = %q", got)
	}
}

// ③ 같은 머리 가지를 두 구성원이 가리키면 뒤 차례 구성원 것만 — 한 push에 같은 가지를 두 번 올리지 않게
func TestPRHeadsSameHeadLast(t *testing.T) {
	a, b, c := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	m := &M{qs: a + "\trefs/heads/pr/x", b: []entry{{l: "x", s: b}, {l: "o", s: a}, {l: "x-reland", s: c}}}
	if got := m.prHeads(); got != "x="+c+"="+a {
		t.Errorf("prHeads = %q, 바람 뒤 구성원 하나", got)
	}
}

// ⑥ 착지 사건 글은 원래 PR 머리 기준 「들어감」(done은 prLane → prNote)
func TestPRNoteRelandTail(t *testing.T) {
	d := t.TempDir()
	t.Chdir(d)
	for _, a := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if o, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, o)
		}
	}
	h, _ := outQ("rev-parse", "HEAD")
	rr := h + "\trefs/heads/pr/x"
	if got := prNote(rr, prLane(rr, "x-reland"), h); got != " · PR 머리 "+first8(h)+" 들어감" {
		t.Errorf("prNote = %q", got)
	}
}

// 다시 맞춤 이름 규칙은 대기열(cmd/hc-fill/reland.go relandSuf)과 같아야 한다
func TestRelandTailSameAsFill(t *testing.T) {
	b, err := os.ReadFile("../hc-fill/reland.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("relandSuf = regexp.MustCompile\\(`([^`]*)`\\)").FindSubmatch(b)
	if m == nil || string(m[1]) != relandTail.String() {
		t.Errorf("대기열 규칙 %q ≠ 병합기 %q", m, relandTail.String())
	}
}
