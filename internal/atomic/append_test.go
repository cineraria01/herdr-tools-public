package atomic

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// dedupe-helpers ③: 옛 줄 덧붙이기 네 벌(hc-sweep·hc-fill appendLine 0644+줄바꿈, hc-lane appendLines 0666 여러 줄,
// hc-lane launch appendLine 0666 그대로)의 꼴을 덮는 표 — 새 파일은 perm 안(마스크 뒤), 있는 파일은 권한 그대로 뒤에 붙임, 못 열면 open 오류
func TestAppendTable(t *testing.T) {
	d := t.TempDir()
	cases := []struct {
		name string
		perm os.FileMode
		pre  string // 미리 있는 내용(0600, 빈 값이면 파일 없음)
		data string
		want string
	}{
		{"새 파일 0644 한 줄", 0o644, "", "a\tb\n", "a\tb\n"},
		{"새 파일 0666 여러 줄", 0o666, "", "x\ny\n", "x\ny\n"},
		{"있는 파일 뒤에", 0o666, "old\n", "new\n", "old\nnew\n"},
		{"줄바꿈 없는 글 그대로", 0o666, "", "raw", "raw"},
	}
	for i, c := range cases {
		p := filepath.Join(d, string(rune('a'+i)))
		if c.pre != "" {
			if err := os.WriteFile(p, []byte(c.pre), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := Append(p, c.perm, c.data); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		b, _ := os.ReadFile(p)
		st, _ := os.Stat(p)
		m := st.Mode().Perm()
		if string(b) != c.want || (c.pre == "" && m&^c.perm != 0) || (c.pre != "" && m != 0o600) {
			t.Fatalf("%s: %q %v", c.name, b, m)
		}
	}
	var pe *fs.PathError
	if err := Append(filepath.Join(d, "없는 폴더", "f"), 0o644, "x\n"); !errors.As(err, &pe) || pe.Op != "open" {
		t.Fatalf("폴더가 없으면 open 오류: %v", err)
	}
}
