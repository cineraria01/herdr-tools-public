package main

// --skill: 에이전트가 읽는 안내(skill.md, 2026-11-24 agent-skill-guides). 안내와 등록된 하위 명령·인자는 tests/repo/skill_test.go가 맞춘다

import _ "embed"

//go:embed skill.md
var skillText string
