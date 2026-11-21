package cksum

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestMatchesCksumCommand(t *testing.T) {
	if _, err := exec.LookPath("cksum"); err != nil {
		t.Skip("cksum 명령 없음")
	}
	for _, s := range []string{"", "a", "\"한국어\"\n", strings.Repeat("xyz\n", 1000)} {
		c := exec.Command("cksum")
		c.Stdin = strings.NewReader(s)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("cksum: %v", err)
		}
		f := strings.Fields(string(out))
		if got, want := Space([]byte(s)), f[0]+" "+f[1]; got != want {
			t.Errorf("%q: Space 얻음 %s, 바람 %s", s, got, want)
		}
		if got, want := Dot([]byte(s)), f[0]+"."+f[1]; got != want {
			t.Errorf("%q: Dot 얻음 %s, 바람 %s", s, got, want)
		}
		crc, n, err := Reader(bytes.NewReader([]byte(s)))
		if c2, n2 := Bytes([]byte(s)); err != nil || crc != c2 || n != n2 {
			t.Errorf("%q: Reader가 Bytes와 다름", s)
		}
	}
}
