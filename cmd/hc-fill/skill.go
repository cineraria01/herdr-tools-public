package main

// --skill: 에이전트가 읽는 안내(skill.md — 하는 일·흔한 흐름·상태 보기·조정과 되돌리기·사건·사람에게 물을 것·하지 않을 것, 2026-11-24
// agent-skill-guides). 설정 없이도 나온다(설정 읽기 전에 main이 부른다). 안내와 등록된 하위 명령·인자는 tests/repo/skill_test.go가 맞춘다

import _ "embed"

//go:embed skill.md
var skillText string
