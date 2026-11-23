package hcjson

// gofix5-sweep S5-01·S5-03(2026-11-23 착지 전 확인): Uses(cwd, dir)의 경계를 실제 폴더로. want는 「그 cwd의 터미널이 dir을 쓰는가」의
// 참값이다. 심볼릭 링크가 작업 트리 맨 위가 아니라 그 아래 폴더를 가리키면 글자로 올라간 조상 어디도 맨 위와 같지 않았다(S5-01 회귀)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsesTable(t *testing.T) {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wt := d + "/repo.wt/한aud"
	for _, p := range []string{wt + "/sub/deep", d + "/repo.wt/한aud2", d + "/other", d + "/locked/in"} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][2]string{{wt, d + "/l-root"}, {wt + "/sub", d + "/l-sub"}, {d + "/other", wt + "/l-out"}} {
		if err := os.Symlink(l[0], l[1]); err != nil {
			t.Fatal(err)
		}
	}
	type row struct {
		name, cwd string
		want      bool
	}
	rows := []row{
		{"같은 폴더", wt, true},
		{"아래 폴더", wt + "/sub/deep", true},
		{"형제(이름 접두)", d + "/repo.wt/한aud2", false},
		{"다른 폴더", d + "/other", false},
		{"링크 → 맨 위", d + "/l-root", true},
		{"링크 → 맨 위, 그 아래", d + "/l-root/sub", true},
		{"링크 → 아래 폴더(S5-01)", d + "/l-sub", true},
		{"링크 → 아래 폴더, 그 아래(S5-01)", d + "/l-sub/deep", true},
		{"링크 → 아래 폴더, 그 아래의 없는 꼬리", d + "/l-sub/gone/x", true},
		{"작업 트리 안 링크(밖을 가리킴)로 띄움 — 글자로 안", wt + "/l-out", true},
		{"없는 경로", "/nonexistent-s5/x", false},
		{"빈 글", "", false},
	}
	// 대소문자·정규화를 가리지 않는 볼륨(macOS 기본)에서만 다른 철자 줄
	fold := d + "/REPO.WT/한AUD"
	if a, e1 := os.Stat(wt); e1 == nil {
		if b, e2 := os.Stat(fold); e2 == nil && os.SameFile(a, b) {
			nfd := strings.ReplaceAll(wt, "한", "한")
			rows = append(rows, row{"대소문자 다른 철자", fold, true}, row{"대소문자 다른 철자의 아래", fold + "/SUB/deep", true},
				row{"한글 NFD 철자", nfd, true}, row{"한글 NFD 철자의 아래", nfd + "/sub", true},
				row{"다른 철자 아래의 없는 폴더", fold + "/gone/x", true})
		}
	}
	// 읽을 수 없는 폴더 아래는 모름 — 쓰는 중(권한을 넘는 root·Windows에서는 빼다)
	if os.Geteuid() > 0 {
		if err := os.Chmod(d+"/locked", 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(d+"/locked", 0o755)
		rows = append(rows, row{"읽을 수 없는 폴더 아래(모름)", d + "/locked/in", true})
	}
	for _, c := range rows {
		if got := Uses(c.cwd, wt); got != c.want {
			t.Errorf("%s: Uses(%q)=%v, 참값 %v", c.name, c.cwd, got, c.want)
		}
	}
	if Uses(wt, d+"/nope") || !Uses(d+"/nope/sub", d+"/nope") {
		t.Error("없는 dir: 글자로만 견줘야 한다")
	}
}
