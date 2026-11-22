package main

// 리눅스로 갈 바이너리 고르기 — 2026-11-21 go-linux-dist(사용자 「난 다 고로 변경 하고 싶어」), 2026-11-22 바이너리 줄로(go-shell-remove).
// 리눅스 기계의 CPU 가드는 TestInstallShBuildFallback(다른 CPU면 받지 않음)

import (
	"reflect"
	"testing"
)

// 리눅스 대상 기계가 있는 도구만: 바이너리 줄 대상이 all이거나 리눅스 기계 이름을 담을 때. macos·local·macOS 기계 이름만인 줄은 뺀다
func TestLinuxTools(t *testing.T) {
	bins := []binUnit{{"merge", "all"}, {"lane", "macos"}, {"fill", "local"}, {"foo", "box-m,box-l"}, {"bar", "box-m"}}
	tools := []string{"bar", "fill", "foo", "lane", "merge"}
	osOf := map[string]string{"box-m": "Darwin", "box-l": "Linux"}
	if got, want := linuxTools(tools, bins, osOf), []string{"foo", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("linuxTools = %v, 바람 %v", got, want)
	}
	if got := linuxTools(tools, bins[1:3], osOf); len(got) != 0 {
		t.Errorf("리눅스 대상이 없는데 %v", got)
	}
	// all 줄은 리눅스 기계가 이번에 안 닿아도 짓는다(SHA256SUMS가 닿은 기계에 따라 바뀌지 않게)
	if got, want := linuxTools(tools, bins, map[string]string{"box-m": "Darwin"}), []string{"merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("리눅스 기계를 모를 때 %v, 바람 %v", got, want)
	}
}

// 기계에 갈 도구: 대상이 덮는 바이너리 줄의 도구, 이름 순·겹침 없이
func TestBinTools(t *testing.T) {
	defer func(b []binUnit) { BINS = b }(BINS)
	BINS = []binUnit{{"merge", "all"}, {"lane", "macos"}, {"fill", "local"}, {"lane", "box-l"}}
	if got, want := binTools("맥", "Darwin"), []string{"fill", "lane", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("맥: %v", got)
	}
	if got, want := binTools("box-l", "Linux"), []string{"lane", "merge"}; !reflect.DeepEqual(got, want) {
		t.Errorf("box-l: %v", got)
	}
}
