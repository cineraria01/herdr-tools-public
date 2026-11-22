package main

// 하위 명령 등록표(2026-11-22 go-cmd-registry): 도움말 머리·등록과 조각의 짝·명령 전용 옵션. 조각 잇는 차례는 internal/helptext 시험

import (
	"strings"
	"testing"

	"herdr-tools/internal/helptext"
)

func TestUsageHead(t *testing.T) {
	if !strings.HasPrefix(usageText, "사용법: hc-lane ") {
		t.Fatalf("usageText 머리가 띄우기 조각(help/launch.txt)이 아니다: %.40q", usageText)
	}
}

// 도움말 조각마다 등록된 명령이 있고, 등록된 명령마다 조각이 있거나(별칭이면) 다른 조각이 「hc-lane <이름> 」으로 말한다
func TestHelpMatchesRegistry(t *testing.T) {
	has := map[string]bool{}
	for _, n := range helptext.Names(helpFS, "help") {
		has[n] = true
		if _, ok := subs[n]; !ok {
			t.Errorf("help/%s.txt에 등록된 명령이 없다", n)
		}
	}
	for n := range subs {
		if !has[n] && !strings.Contains(usageText, "hc-lane "+n+" ") {
			t.Errorf("명령 %s의 도움말 조각 help/%s.txt가 없다", n, n)
		}
	}
}

// argSpecs는 등록된 명령 것만, 전용 옵션은 한 명령만 두고, 다른 명령에 주면 「<옵션>는 <명령>에서만 쓴다」
func TestArgSpecsOwnFlagsUnique(t *testing.T) {
	owner := map[string]string{}
	for c, sp := range argSpecs {
		if _, ok := subs[c]; !ok {
			t.Errorf("argSpecs[%s]: 등록된 명령이 아니다", c)
		}
		for f := range sp.flags {
			if o, dup := owner[f]; dup {
				t.Errorf("옵션 %s를 %s·%s 둘이 둔다", f, o, c)
			}
			owner[f] = c
		}
	}
	for f, c := range owner {
		for other := range argSpecs {
			if other == c {
				continue
			}
			a := []string{f, "x"}
			if argSpecs[other].head != nil {
				a = append([]string{"show"}, a...) // 앞 자리 인자를 받는 명령(guard·lease)
			}
			_, pe := parseArgs(other, a)
			if want := "hc-lane: " + f + "는 " + c + "에서만 쓴다"; pe == nil || pe.msg != want {
				t.Errorf("%s %s: %q 여야 한다, %+v", other, f, want, pe)
			}
		}
	}
}
