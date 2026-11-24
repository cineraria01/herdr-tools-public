//go:build !windows

package main

import (
	"path/filepath"
	"testing"
	"time"
)

// 이 저장소: cotenant 단계와 조각마다 지문이 나오고(꾸러미를 모르는 조각 없음), 조각끼리 지문이 다르다
func TestScopeRepo(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	x, err := scopeIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := goList(root)
	if err != nil {
		t.Fatal(err)
	}
	_, alone, base := plan("")
	sh := append([]shard{{argv: alone}}, shards(root, pkgs, base, splits, 10)...)
	seen := map[string]bool{}
	for _, s := range sh {
		k, err := x.key(s.argv, s.env)
		if err != nil {
			t.Errorf("%v: %v", s.argv, err)
		}
		if seen[k] {
			t.Errorf("%v %v: 다른 조각과 같은 지문", s.argv, s.env)
		}
		seen[k] = true
	}
	// 시간은 단언하지 않는다(조각과 함께 돌면 부하로 흔들린다 — docs/repo/tests.md 「시험의 기다림」). 실측은 검증의 「범위 지문 …초」 줄
	t.Logf("조각 %d개 지문 %v", len(sh), time.Since(t0))
}
