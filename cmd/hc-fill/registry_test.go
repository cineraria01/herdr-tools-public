package main

// 하위 명령 등록표(2026-11-22 go-cmd-registry): 도움말 조각과 등록의 짝. 조각 잇는 차례는 internal/helptext 시험

import (
	"strings"
	"testing"

	"herdr-tools/internal/helptext"
)

// 조각마다 등록된 명령이 있고, 등록된 명령(속 명령 _… 빼고)마다 조각이 있거나 요약 줄 「hc-fill <이름> 」이 있다
func TestHelpMatchesRegistry(t *testing.T) {
	if !strings.HasPrefix(usageText, "사용법: hc-fill ") || !strings.HasSuffix(usageText, usageTail) {
		t.Fatalf("머리·꼬리가 어긋났다: %.40q", usageText)
	}
	has := map[string]bool{}
	for _, n := range helptext.Names(helpFS, "help") {
		has[n] = true
		if _, ok := cmds[n]; !ok {
			t.Errorf("help/%s.txt에 등록된 명령이 없다", n)
		}
	}
	for n := range cmds {
		if strings.HasPrefix(n, "_") || has[n] {
			continue
		}
		if !strings.Contains(usageText, "hc-fill "+n+" ") && !strings.Contains(usageText, "| "+n+" ") {
			t.Errorf("명령 %s의 도움말 조각 help/%s.txt가 없다", n, n)
		}
	}
}
