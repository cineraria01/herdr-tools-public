package trustq

import (
	"strings"
	"testing"

	"herdr-tools/internal/clitest/sandbox"
)

func TestSandboxed(t *testing.T) { sandbox.Check(t) }

const q = "Accessing workspace:\n\n /x/y\n\n Is this a project you created or one you trust?\n\n"

func TestLook(t *testing.T) {
	for _, c := range []struct {
		name, scr string
		v         int
		keys      string
	}{
		{"질문 아님", "> 일하는 중\n", None, ""},
		{"No에 커서", q + " ❯ No, exit\n   Yes, I trust this folder\n", Keys, "down enter"},
		{"Yes에 커서", q + "   No, exit\n ❯ Yes, I trust this folder\n", Keys, "enter"},
		{"커서 없음", q + "   No, exit\n   Yes, I trust this folder\n", NoCursor, ""},
		{"글자 다름", "Do you trust the files in this folder?\n ❯ 1. Yes\n", Odd, ""},
	} {
		v, k := Look(c.scr)
		if v != c.v || strings.Join(k, " ") != c.keys {
			t.Errorf("%s: %d %v, 바람 %d %q", c.name, v, k, c.v, c.keys)
		}
	}
}
