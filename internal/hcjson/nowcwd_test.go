package hcjson

import (
	"os"
	"path/filepath"
	"testing"
)

// cwd-now-inuse A3(2026-11-23): cwd 원문의 `..`는 링크를 푼 뒤에 올라간다 — Clean으로 글자로 접으면 다른 폴더가 된다.
// G/lsub → W/sub/deep 링크일 때 G/lsub/.. 은 디스크로 W/sub(그 작업 트리 아래)인데 글자로 접으면 G(밖)다
func TestUsesDotDotDisk(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	W := G + "/repo.wt/a"
	os.MkdirAll(W+"/sub/deep", 0o755)
	os.Symlink(W+"/sub/deep", G+"/lsub")
	if !Uses(G+"/lsub/..", W) {
		t.Errorf("Uses(%q, %q): 디스크로 푼 W/sub를 못 봄", G+"/lsub/..", W)
	}
	if !Uses(G+"/lsub/../..", W) {
		t.Errorf("Uses: 디스크로 푼 W를 못 봄")
	}
	if Uses(G+"/lsub/../../..", W) { // W의 부모(repo.wt) — 밖
		t.Errorf("Uses: 디스크로 W 밖인데 쓰는 중으로 봄")
	}
}

// cwd-now-inuse: hc get의 foreground.cwd → NowCwd(없음·null·꼴 틀림·상대 경로는 "" = 모름 — get 자체는 오류가 아니다), cwd 원문 → CwdText
func TestNowCwd(t *testing.T) {
	get := func(fg string) string {
		return `{"address":"t1","cwd":"/w/x/../a","state":"live","status":{"state":"working"}` + fg + `}`
	}
	for fg, want := range map[string]string{
		`,"foreground":{"cwd":"/w/b","pid":7}`: "/w/b",
		`,"foreground":{"cwd":"C:\\w"}`:        `C:\w`,
		`,"foreground":{"name":"node"}`:        "",
		`,"foreground":{"cwd":null}`:           "",
		`,"foreground":{"cwd":7}`:              "",
		`,"foreground":{"cwd":"rel"}`:          "",
		`,"foreground":"x"`:                    "",
		`,"foreground":null`:                   "",
		``:                                     "",
	} {
		tm, found, err := ParseGet([]byte(get(fg)), 0, "t1")
		if err != nil || !found || tm.NowCwd != want || tm.Cwd != "/w/a" || tm.CwdText != "/w/x/../a" {
			t.Errorf("%s: NowCwd %q CwdText %q Cwd %q %v %v(기대 NowCwd %q)", fg, tm.NowCwd, tm.CwdText, tm.Cwd, found, err, want)
		}
	}
}
