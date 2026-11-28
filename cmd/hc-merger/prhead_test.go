package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// mir <sha> <레인>: 거울이 만든 머리 가지 — 가지와 거울 표식(refs/herdr/pr-src/<레인>, pr-mirror-gaps)을 함께
func mir(sha, lane string) string {
	return sha + "\trefs/heads/pr/" + lane + "\n" + sha + "\trefs/herdr/pr-src/" + lane
}

// pr-reland-head: 다시 맞춤 레인이 짝 참조보다 먼저 제출해도 원래 PR 머리 가지(꼬리를 뗀 이름)를 찾는다
func TestPRLaneRelandTail(t *testing.T) {
	a, b, c := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	cases := []struct{ name, list, lane, want string }{
		{"① 짝 참조 없이 pr/X만", mir(a, "x"), "x-reland", "x"},
		{"① 둘째 다시 맞춤", mir(a, "x"), "x-reland2", "x"},
		{"② 짝 참조가 먼저", mir(a, "x") + "\n" + mir(b, "y") + "\n" + c + "\trefs/herdr/pr-of/x-reland/y", "x-reland", "y"},
		{"③ 제 머리 가지가 먼저", mir(a, "x") + "\n" + mir(b, "x-reland"), "x-reland", "x-reland"},
		{"④ pr/X 없음", mir(b, "other"), "x-reland", "x-reland"},
		{"⑤ -reland 꼴 아님", mir(a, "x"), "x-relandy", "x-relandy"},
		{"⑤ 꼬리만인 이름", mir(a, ""), "-reland", "-reland"},
	}
	for _, k := range cases {
		if got := prLane(k.list, k.lane); got != k.want {
			t.Errorf("%s: prLane(%s) = %s, 바람 %s", k.name, k.lane, got, k.want)
		}
	}
	m := &M{qs: mir(a, "x"), b: []entry{{l: "x-reland", s: b}}}
	if got := m.prHeads(); got != "x="+b+"="+a {
		t.Errorf("① prHeads = %q", got)
	}
}

// ③ 같은 머리 가지를 두 구성원이 가리키면 뒤 차례 구성원 것만 — 한 push에 같은 가지를 두 번 올리지 않게
func TestPRHeadsSameHeadLast(t *testing.T) {
	a, b, c := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	m := &M{qs: mir(a, "x"), b: []entry{{l: "x", s: b}, {l: "o", s: a}, {l: "x-reland", s: c}}}
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
	rr := h + "\trefs/heads/pr/x" // done의 다시 읽은 목록(머리 가지만), 머리 레인은 바퀴 목록(m.qs — 거울 표식 포함)으로
	if got := prNote(rr, mir(h, "x"), prLane(mir(h, "x"), "x-reland"), h, h); got != " · PR 머리 "+first8(h)+" 들어감" {
		t.Errorf("prNote = %q", got)
	}
}

// ht-smalls-1129: 「안 들어감」의 까닭을 사실대로 — 레인 커밋이 이미 기본 가지에 있어 rebase가 떨어뜨림 / 머리 가지가 바퀴가 본 값에서 움직임
// (lease 거절 — 경합) / 그 밖(지금 글). 고치기 전은 셋 다 「가지 갱신 실패」
func TestPRNoteWhy(t *testing.T) {
	d := t.TempDir()
	t.Chdir(d)
	g := func(a ...string) string {
		o, err := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, a...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", a, err, o)
		}
		return strings.TrimSpace(string(o))
	}
	write := func(f, s string) {
		if err := os.WriteFile(f, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g("init", "-q", "-b", "main")
	g("commit", "-q", "--allow-empty", "-m", "base")
	g("checkout", "-q", "-b", "lane")
	write("a", "1")
	g("add", "a")
	g("commit", "-q", "-m", "lane a")
	L := g("rev-parse", "HEAD")
	g("checkout", "-q", "main")
	// main을 한 칸 움직여 착지 커밋이 레인 커밋과 다른 SHA가 되게, 신원(작성자·시각·메시지)이 같은 착지 — 보통 착지
	g("commit", "-q", "--allow-empty", "-m", "main moved")
	g("cherry-pick", L)
	landed := g("rev-parse", "HEAD")
	g("reset", "-q", "--hard", "HEAD~1")
	write("a", "1")
	g("add", "a")
	g("commit", "-q", "-m", "fix lane a") // 같은 패치가 다른 커밋으로 먼저 들어감 — rebase가 레인 커밋을 떨어뜨림
	dropped := g("rev-parse", "HEAD")
	other := g("commit-tree", "-m", "other", "-p", dropped, dropped+"^{tree}")
	rr := func(h string) string { return h + "\trefs/heads/pr/x" }
	for _, c := range []struct{ name, rr, seen, C, want string }{
		{"이미 들어 있음", rr(L), mir(L, "x"), dropped, " · PR 머리 안 들어감(이미 들어 있어 올릴 머리 없음 — Merged 표시 없음)"},
		{"경합", rr(other), mir(L, "x"), landed, " · PR 머리 안 들어감(머리 가지 갱신 거절(경합) — Merged 표시 없음)"},
		{"그 밖", rr(L), mir(L, "x"), landed, " · PR 머리 안 들어감(가지 갱신 실패 — Merged 표시 없음)"},
		{"들어감", rr(landed), mir(L, "x"), landed, " · PR 머리 " + first8(landed) + " 들어감"},
	} {
		if got := prNote(c.rr, c.seen, "x", L, c.C); got != c.want {
			t.Errorf("%s: prNote = %q, 바람 %q", c.name, got, c.want)
		}
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

// pr-mirror-gaps ⑤: 머리 가지는 거울이 만든 것(숨은 참조 refs/herdr/pr-src/<레인>이 있음)만 — 사람이 만든 같은 이름 가지는 건드리지 않는다
// (고치기 전: pr/x만 있어도 넘겨 착지가 사람 가지를 착지 커밋으로 덮었다)
func TestPRHeadsMirrorOnly(t *testing.T) {
	a, b, c := strings.Repeat("1", 40), strings.Repeat("2", 40), strings.Repeat("3", 40)
	cases := []struct{ name, list, lane, want string }{
		{"사람 가지(표식 없음)", a + "\trefs/heads/pr/x", "x", ""},
		{"거울 가지(대조)", a + "\trefs/heads/pr/x\n" + c + "\trefs/herdr/pr-src/x", "x", "x=" + b + "=" + a},
		{"짝 참조 — 원래 가지가 사람 것", a + "\trefs/heads/pr/o\n" + c + "\trefs/herdr/pr-of/x/o", "x", ""},
		{"짝 참조 — 원래 가지가 거울 것", a + "\trefs/heads/pr/o\n" + c + "\trefs/herdr/pr-src/o\n" + c + "\trefs/herdr/pr-of/x/o", "x", "o=" + b + "=" + a},
		{"꼬리 뗀 이름 — 사람 가지", a + "\trefs/heads/pr/x", "x-reland", ""},
		{"꼬리 뗀 이름 — 거울 가지", a + "\trefs/heads/pr/x\n" + c + "\trefs/herdr/pr-src/x", "x-reland2", "x=" + b + "=" + a},
		{"제 이름은 사람 가지, 원래는 거울", a + "\trefs/heads/pr/x\n" + c + "\trefs/herdr/pr-src/x\n" + b + "\trefs/heads/pr/x-reland", "x-reland", "x=" + b + "=" + a},
		{"표식만 남고 가지 없음", c + "\trefs/herdr/pr-src/x", "x", ""},
	}
	for _, k := range cases {
		m := &M{qs: k.list, b: []entry{{l: k.lane, s: b}}}
		if got := m.prHeads(); got != k.want {
			t.Errorf("%s: prHeads = %q, 바람 %q", k.name, got, k.want)
		}
	}
}

// ⑤ 바퀴의 ls-remote 무늬(머리 가지·짝 참조·거울 표식)가 실제 git 응답을 온전히(요청 밖 이름 없이) 받고, 그 목록으로 머리를 고른다
func TestPRSrcPatLsRemote(t *testing.T) {
	d := t.TempDir()
	t.Chdir(d)
	for _, a := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"},
		{"update-ref", "refs/herdr/pr-src/x", "HEAD"}, {"update-ref", "refs/herdr/pr-of/y/x", "HEAD"}, {"update-ref", "refs/herdr/merger/m", "HEAD"},
		{"update-ref", "refs/heads/pr/x", "HEAD"}} {
		if o, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, o)
		}
	}
	s, rc := lsRemote(".", prHeadPat, prOfPat, prSrcPat)
	if rc != 0 || !strings.Contains(s, "\trefs/herdr/pr-src/x") || !strings.Contains(s, "\trefs/herdr/pr-of/y/x") || strings.Contains(s, "merger") {
		t.Fatalf("ls-remote(종료 %d):\n%s", rc, s)
	}
	h, _ := outQ("rev-parse", "HEAD")
	if got := (&M{qs: s, b: []entry{{l: "y", s: h}}}).prHeads(); got != "x="+h+"="+h {
		t.Fatalf("prHeads = %q", got)
	}
}
