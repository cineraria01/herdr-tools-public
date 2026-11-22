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
	refs := Parse(ls)
	for _, c := range []struct{ lane, want string }{
		{"sv-x", "1795317520-sv-x"}, {"sv-y", ""}, {"sv-z", ""}, {"sv", ""}, {"", ""},
	} {
		if got := Of(refs, c.lane); got != c.want {
			t.Errorf("Of(%q) = %q, 바람 %q", c.lane, got, c.want)
		}
	}
	for qn, want := range map[string]string{"1795317520-sv-x": "sv-x", "179531752-sv": "", "17953175200sv": "", "x795317520-a": ""} {
		if got := LaneOf(qn); got != want {
			t.Errorf("LaneOf(%q) = %q, 바람 %q", qn, got, want)
		}
	}
}
