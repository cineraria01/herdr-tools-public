package main

// --skill: 에이전트가 읽는 안내(skill.md, 2026-11-24 agent-skill-guides). 안내의 인자는 tests/repo/skill_test.go가 코드와 맞춘다

import _ "embed"

//go:embed skill.md
var skillText string
