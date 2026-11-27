package main

// 기계 인자·--take·--pull 값 판정(ht-sync-arg-guards, 2026-11-28)

import (
	"fmt"
	"slices"
	"strings"
)

// specShape: --take·--pull 값 꼴 <기계>:<경로> (:의 앞뒤에 글자)
func specShape(s string) bool {
	i := strings.IndexByte(s, ':')
	return i > 0 && i < len(s)-1
}

func before(s string) string { i := strings.IndexByte(s, ':'); return s[:i] }
func after(s string) string  { i := strings.IndexByte(s, ':'); return s[i+1:] }

// pickMachines: 기계 자리와 --take·--pull의 <기계>:는 기계 표(hc machine ls)·이 기계(옛 이름 「맥」도)·repo(--take·--pull만)만 받는다 —
// 모르는 이름을 기계로 다뤄 배포 기록에 「<이름>=실패」를 남기지 않게. 기계 인자 없이 <기계>:를 가리켰으면 그 기계들만
// (이 기계는 늘 맞춘다), 기계 인자를 적었으면 그 목록이고 가리킨 기계가 거기 없으면 거절
func pickMachines(ms, all []string) ([]string, error) {
	known := func(m string) bool { return m == LOCAL || m == oldLocal || slices.Contains(all, m) }
	table := strings.Join(append([]string{LOCAL}, all...), " ")
	for _, m := range ms {
		if !known(m) {
			return nil, fmt.Errorf("모르는 기계 %s — 기계 표: %s", m, table)
		}
	}
	var pointed []string
	for _, s := range append(append([]string(nil), takes...), pulls...) {
		m := before(s)
		switch {
		case m == "repo":
			continue
		case !known(m):
			return nil, fmt.Errorf("모르는 기계 %s (%s) — 기계 표: %s", m, s, table)
		case m == LOCAL || m == oldLocal:
			m = LOCAL
		case len(ms) > 0 && !slices.Contains(ms, m):
			return nil, fmt.Errorf("--take·--pull이 가리킨 기계 %s — 고른 기계(%s)에 없다 (%s)", m, strings.Join(ms, " "), s)
		}
		if !slices.Contains(pointed, m) {
			pointed = append(pointed, m)
		}
	}
	if len(ms) > 0 || len(pointed) == 0 {
		return ms, nil
	}
	return pointed, nil
}
