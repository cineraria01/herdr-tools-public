package hclane_test

import (
	"testing"

	"herdr-tools/internal/clitest/sandbox"
)

// 이 시험 바이너리를 격리한다(임시 HOME·운영 값 지움 — docs/repo/tests.md 「시험의 집」). 시험이 있는 꾸러미마다 이 파일 하나(tests/repo TestTestsSandboxed)
func TestSandboxed(t *testing.T) { sandbox.Check(t) }
