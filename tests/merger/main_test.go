package merger_test

import (
	"testing"

	"herdr-tools/internal/clitest"
)

// TestMain: 끝나면 clitest.Bin 폴더를 지운다(재감사 base R-225)
func TestMain(m *testing.M) { clitest.Main(m) }
