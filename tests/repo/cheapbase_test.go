package repo_test

// 싼 검사 파일 빠짐의 기준 = 이 가지가 기본 가지에서 갈라진 자리(2026-11-24 cheapkept-base). 갈라진 뒤 다른 레인이 기본 가지에 더한
// 싼 검사 파일은 아직 받지 않은 것이지 지운 것이 아니다 — 옛 기준(기본 가지 끝)은 그것을 「없음」으로 읽어 떠 있는 레인을 모두 막았다.

import (
	"reflect"
	"strings"
	"testing"

	"herdr-tools/internal/clitest"
)

func TestCheapKeptBase(t *testing.T) {
	T := clitest.TempDir(t)
	clitest.Isolate(t, T)
	for _, kv := range []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"} {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	R := T + "/r"
	clitest.Put(t, R+"/tests/repo/a_test.go", "package a\n")
	clitest.Put(t, R+"/tests/repo/b_test.go", "package a\n")
	clitest.MustGit(t, T, "init", "-q", "--bare", "-b", "main", T+"/o.git")
	clitest.MustGit(t, R, "init", "-q", "-b", "main")
	clitest.MustGit(t, R, "add", ".")
	clitest.MustGit(t, R, "commit", "-qm", "base")
	clitest.MustGit(t, R, "remote", "add", "origin", T+"/o.git")
	clitest.MustGit(t, R, "push", "-q", "origin", "HEAD:main")
	clitest.MustGit(t, R, "fetch", "-q", "origin")
	base, _, _ := git(R, "rev-parse", "HEAD")
	clitest.MustGit(t, R, "branch", "lane")
	// 레인이 갈라진 뒤 다른 레인이 싼 검사 파일 c를 더해 착지
	clitest.Put(t, R+"/tests/repo/c_test.go", "package a\n")
	clitest.MustGit(t, R, "add", ".")
	clitest.MustGit(t, R, "commit", "-qm", "c")
	clitest.MustGit(t, R, "push", "-q", "origin", "HEAD:main")
	clitest.MustGit(t, R, "fetch", "-q", "origin")
	tip, _, _ := git(R, "rev-parse", "HEAD")
	clitest.MustGit(t, R, "checkout", "-q", "lane")

	want := func(scene, at string, miss ...string) {
		t.Helper()
		gotAt, m, err := missingCheap(R)
		if err != nil || gotAt != at || !reflect.DeepEqual(m, miss) {
			t.Errorf("%s: 기준 %q 빠짐 %v 이어야 — 기준 %q 빠짐 %v 오류 %v", scene, at, miss, gotAt, m, err)
		}
	}
	// ① 갈라진 뒤 기본 가지에 새로 생긴 파일은 보지 않는다(고치기 전: c_test.go 빠짐으로 실패)
	want("① 갈라진 뒤 새 파일", base[:8])
	// ② 갈라진 자리에 있던 파일을 이 가지가 지움 → 실패
	clitest.MustGit(t, R, "rm", "-q", "--", "tests/repo/b_test.go")
	clitest.MustGit(t, R, "commit", "-qm", "rm b")
	want("② 지움", base[:8], "tests/repo/b_test.go")
	// ③ rebase 뒤(병합기·미리 검증의 트리): 갈라진 자리 = 기본 가지 끝, 새 파일은 rebase가 가져오고 지운 것은 여전히 걸린다
	clitest.MustGit(t, R, "rebase", "-q", "origin/main")
	want("③ rebase 뒤", tip[:8], "tests/repo/b_test.go")
	// ④ 갈라진 자리를 못 구함(공통 조상 없음) → 기본 가지 끝과 견줌(엄한 쪽)
	clitest.MustGit(t, R, "checkout", "-q", "--orphan", "solo")
	clitest.MustGit(t, R, "rm", "-q", "-f", "--", "tests/repo/c_test.go")
	clitest.MustGit(t, R, "commit", "-qm", "solo")
	want("④ 공통 조상 없음", "", "tests/repo/b_test.go", "tests/repo/c_test.go")
	// ④ 얕은 클론 — 갈라진 자리가 잘린 역사 너머일 수 있어 믿지 않고 기본 가지 끝과 견줌
	clitest.MustGit(t, T, "clone", "-q", "--depth", "1", "file://"+T+"/o.git", T+"/sh")
	if at, m, err := missingCheap(T + "/sh"); err != nil || at != "" || len(m) != 0 {
		t.Errorf("④ 얕은 클론은 기본 가지 끝과 견줘야: 기준 %q 빠짐 %v 오류 %v", at, m, err)
	}
	// ⑤ 원격 없음 → 건너뜀은 TestCheapKeptSelf가 본다
}
