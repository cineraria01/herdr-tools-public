//go:build !windows

package main

// gofix3-shx fill/R-216: 추적 파일만 고친(새 미추적 파일이 없는) 작업 트리 — actScript의 마지막 빈 줄 건너뛰기가 1로 끝나
// 조각 전체가 실패(ACT 없음)로 읽혀 같은 파일을 예약한 다음 레인과의 겹침을 놓쳤다. 실제 git 작업 트리로 본다

import (
	"os"
	"reflect"
	"testing"
)

func TestActTrackedOnly(t *testing.T) {
	actSave(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	d := t.TempDir()
	org, repo := d+"/o.git", d+"/app"
	actGitT(t, d, "init", "-q", "--bare", "-b", "main", org)
	actGitT(t, d, "clone", "-q", org, repo)
	writeT(t, repo+"/기존 자료.md", "a\n")
	actGitT(t, repo, "add", "기존 자료.md")
	actGitT(t, repo, "commit", "-q", "-m", "base")
	actGitT(t, repo, "push", "-q", "origin", "HEAD:main")
	actGitT(t, repo, "remote", "set-head", "origin", "main")
	wt := repo + ".wt/live"
	actGitT(t, repo, "worktree", "add", "-q", "-b", "live", wt, "origin/main")
	writeT(t, wt+"/기존 자료.md", "a 고침\n") // 추적 파일만 고침 — 미추적 새 파일 없음

	confLines = lines("local here\nrepo app here " + repo + "\n")
	LOCAL, dry, FETCHED, FETCHBAD = "here", false, "", ""
	OREC = []Row{{"live", "app", "here", "t1", "s", "b", ",-,", "-"}}
	TERMS = []Term{{M: "here", Addr: "t1", Name: "live"}}
	actTick(nil)
	want := map[string][]string{"app\tlive": {"기존 자료.md"}}
	if !reflect.DeepEqual(ACT, want) {
		t.Fatalf("추적 파일만 고친 작업 트리\n got %#v\nwant %#v", ACT, want)
	}
	if got := rowsBar(ovcmp(actKeysOwn("live", "app"), []Row{{"R", "기존 자료.md"}})); got != "live\t실제\t기존 자료.md|" {
		t.Fatalf("같은 파일을 예약한 다음 레인과 겹침: %q", got)
	}
}

// actKeysOwn: 앞 레인의 키 A를 ovcmp의 앞 레인 줄 꼴(레인·종류·파일)로
func actKeysOwn(n, p string) []Row {
	var o []Row
	for _, k := range actKeys(p, n) {
		o = append(o, Row{n, k.F(1), k.F(2)})
	}
	return o
}
