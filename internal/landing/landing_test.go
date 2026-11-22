package landing

import (
	"strings"
	"testing"
)

func TestOf(t *testing.T) {
	a, b := strings.Repeat("1", 40), strings.Repeat("2", 40) // 가짜 SHA는 실행 중에(추적 파일 비밀 꼴 검사)
	ls := a + "\trefs/merge-queue/1795317520-sv-x\n" +
		a + "\trefs/herdr/landing/1795317520-sv-x\n" +
		b + "\trefs/merge-queue/1795317600-sv-y\n" +
		a + "\trefs/herdr/landing/1795317600-sv-y\n" + // 바꿔 낸 제출(SHA 다름) — 남은 표식
		a + "\trefs/herdr/landing/1795317000-sv-z\n" // 큐 참조 없음(끝난 착지)
	refs, ok := Parse(ls)
	if !ok {
		t.Fatal("온전한 목록을 거절했다")
	}
	for _, c := range []struct{ lane, want string }{
		{"sv-x", "1795317520-sv-x"}, {"sv-y", ""}, {"sv-z", ""}, {"sv", ""}, {"", ""},
	} {
		if got := Of(refs, c.lane); got != c.want {
			t.Errorf("Of(%q) = %q, 바람 %q", c.lane, got, c.want)
		}
	}
	m := map[string]string{"refs/merge-queue/1795317520-sv-x": a, "refs/herdr/landing/1795317520-sv-x": a}
	if got := Of(Refs(m), "sv-x"); got != "1795317520-sv-x" {
		t.Errorf("Refs(맵) 뒤 Of = %q", got)
	}
	for qn, want := range map[string]string{"1795317520-sv-x": "sv-x", "179531752-sv": "", "17953175200sv": "", "x795317520-a": ""} {
		if got := LaneOf(qn); got != want {
			t.Errorf("LaneOf(%q) = %q, 바람 %q", qn, got, want)
		}
	}
}

// lane/R-212: 한 줄이라도 꼴이 틀리면(이름 없음·짧은 SHA·대문자·공백·같은 이름 두 번) 목록 전체가 모름 — 「표식 없음」으로 읽지 않는다
func TestParseStrict(t *testing.T) {
	a := strings.Repeat("a", 40)
	good := a + "\trefs/merge-queue/1795317520-sv-x\n"
	if r, ok := Parse(""); !ok || len(r) != 0 {
		t.Fatalf("빈 목록은 0개 ok: %v %v", r, ok)
	}
	if r, ok := Parse(strings.TrimSuffix(good, "\n")); !ok || len(r) != 1 {
		t.Fatalf("끝 줄바꿈을 뗀 목록(gitx.Out)도 받는다: %v %v", r, ok)
	}
	for _, bad := range []string{
		good + a + "\n",
		good + "abcdef012345\trefs/herdr/landing/1795317520-sv-x\n",
		good + strings.ToUpper(a) + "\trefs/herdr/landing/1795317520-sv-x\n",
		good + a + " refs/herdr/landing/1795317520-sv-x\n",
		good + good,
		good + "\n",
		good + a + "\trefs/herdr/landing/x y\n",
		good + a + "\tHEAD\n",
	} {
		if r, ok := Parse(bad); ok {
			t.Errorf("Parse(%q)는 거절해야: %v", bad, r)
		}
	}
}
