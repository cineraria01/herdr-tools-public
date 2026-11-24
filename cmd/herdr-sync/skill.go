package main

// --skill: 에이전트가 읽는 안내(skill.md, 2026-11-24 agent-skill-guides). main보다 먼저(init) 찍고 끝낸다 — 맥 확인·잠금 전이라
// 어느 기계에서든 읽고, main.go는 줄 수 한도에 닿아 여기 둔다. 안내의 인자는 tests/repo/skill_test.go가 코드와 맞춘다

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed skill.md
var skillText string

func init() {
	if len(os.Args) > 1 && os.Args[1] == "--skill" {
		fmt.Print(skillText)
		os.Exit(0)
	}
}
