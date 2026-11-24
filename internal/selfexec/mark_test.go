package selfexec

import (
	"os"
	"testing"

	"herdr-tools/internal/resident"
)

// 뜰 때 판 표시(stale-resident-check): New가 이 pid·지금 실행 파일 판을 상태 폴더에 남긴다 — 스스로 넘어간 새 판도 New에서 다시 쓴다
func TestNewLeavesMark(t *testing.T) {
	st := t.TempDir()
	t.Setenv("XDG_STATE_HOME", st)
	if New("hc-lane watch", func(string) {}) == nil {
		t.Fatal("New가 nil")
	}
	h, _ := os.UserHomeDir()
	ms, err := resident.Load(resident.DirFor(st, h))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := ms[os.Getpid()]
	p, _ := os.Executable()
	fi, _ := os.Stat(p)
	if !ok || m.Who != "hc-lane watch" || m.Path != p || m.Size != fi.Size() || !m.Mtime.Equal(fi.ModTime()) {
		t.Fatalf("표시: %+v (있음 %v)", m, ok)
	}
}
