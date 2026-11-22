package main

import (
	"strings"
	"testing"
)

// 감사 보고 꼴마다 [낮음] 지적 원문 뽑기(실제 보고 꼴을 줄인 표본)
func TestLowsItems(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string // 각 지적의 첫 줄(앞뒤 공백 뗌)
		has  []string // 뽑은 글 어딘가에 있어야 할 것
		not  []string // 뽑은 글에 없어야 할 것
	}{
		{"제목 [낮음] 번호", "# 감사 x\n판정: **높음 0 · 중간 1 · 낮음 2**\n## 지적\n### [중간]1 가\n중간 본문\n### [낮음] 1. 나\n나 본문\n- 근거: 줄\n### [낮음]2 다\n다 본문\n## 확인한 것\n- 문제 없음\n",
			[]string{"### [낮음] 1. 나", "### [낮음]2 다"}, []string{"나 본문", "- 근거: 줄", "다 본문"}, []string{"중간 본문", "문제 없음"}},
		{"제목 낮음 N —", "판정: 높음 0 · 중간 1 · 낮음 2\n## 중간 1 — 가\n본문\n## 낮음 1 — 나\n나 본문\n## 낮음 2 · a.go:3 — 다\n\n다 본문\n\n## 확인한 것(문제 없음)\n- x\n",
			[]string{"## 낮음 1 — 나", "## 낮음 2 · a.go:3 — 다"}, []string{"나 본문", "다 본문"}, []string{"- x"}},
		{"제목 꼴이 있으면 목록 꼴(앞 차수 확인)은 뺌", "## 앞 지적 처리\n- [낮음] 1 가 — 고쳐짐\n- [낮음] 2 나 — 그대로\n## 지적\n### [낮음] 1 — 나 그대로\n본문\n",
			[]string{"### [낮음] 1 — 나 그대로"}, []string{"본문"}, []string{"고쳐짐"}},
		{"목록 꼴(굵은 낮음 ·)", "## 지적 (모두 낮음)\n1. **낮음 · `a.sh:2`** — 가\n2. **낮음 · 원 지시문** — 나\n   이어지는 줄\n3. 중간 아님 다른 줄\n## 기타\n- y\n",
			[]string{"1. **낮음 · `a.sh:2`** — 가", "2. **낮음 · 원 지시문** — 나"}, []string{"이어지는 줄"}, []string{"다른 줄", "- y"}},
		{"코드 울타리 안 제목은 끊지 않음", "### [낮음] 1 가\n```sh\n# 주석 줄\n### [낮음] 9 가짜\n```\n뒷 본문\n### [낮음] 2 나\n",
			[]string{"### [낮음] 1 가", "### [낮음] 2 나"}, []string{"# 주석 줄", "뒷 본문"}, nil},
		{"낮음이 등급이 아닌 제목·줄은 안 뽑음", "# 감사\n판정: 높음 0 · 중간 0 · 낮음 0\n## 지적 (모두 낮음)\n없음\n- 낮음으로 볼 것 없음\n", nil, nil, nil},
	}
	for _, c := range cases {
		got := lowsItems(c.text)
		var first []string
		for _, g := range got {
			first = append(first, strings.TrimSpace(strings.SplitN(g, "\n", 2)[0]))
		}
		if strings.Join(first, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: 첫 줄 %q, 바람 %q", c.name, first, c.want)
		}
		all := strings.Join(got, "\n")
		for _, h := range c.has {
			if !strings.Contains(all, h) {
				t.Errorf("%s: %q 없음 — %q", c.name, h, got)
			}
		}
		for _, n := range c.not {
			if strings.Contains(all, n) {
				t.Errorf("%s: %q 들어감 — %q", c.name, n, got)
			}
		}
		for _, g := range got {
			if strings.HasSuffix(g, "\n") || strings.HasSuffix(g, " ") {
				t.Errorf("%s: 끝 빈 줄·빈칸을 떼지 않음 %q", c.name, g)
			}
		}
	}
}
