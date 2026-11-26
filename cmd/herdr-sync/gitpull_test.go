//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// cloneOf: up의 클론. 훅은 그 클론의 .git/hooks(전역 core.hooksPath를 덮는다)
func cloneOf(t *testing.T, up string) string {
	t.Helper()
	w := t.TempDir() + "/w"
	gitT(t, up, "clone", "-q", up, w)
	gitT(t, w, "config", "core.hooksPath", w+"/.git/hooks")
	return w
}

func commitUp(t *testing.T, up, body string) {
	t.Helper()
	os.WriteFile(up+"/asset", []byte(body+"\n"), 0o644)
	gitT(t, up, "commit", "-qam", body)
}

// racingFetch: origin/main 갱신이 committed 되는 순간 다른 fetch처럼 FETCH_HEAD를 for-merge 두 줄로 고쳐 쓰는 훅(결정적 재현)
const racingFetch = "#!/bin/sh\n[ \"$1\" = committed ] || exit 0\ngrep -q refs/remotes/origin/main || exit 0\n" +
	"printf '%s\\t\\tbranch main of x\\n%s\\t\\tbranch other of x\\n' $(git rev-parse refs/remotes/origin/main) $(git rev-parse refs/remotes/origin/other) > .git/FETCH_HEAD\n"

// ① 2026-11-26 17:15: 앞당김 도중 같은 체크아웃의 다른 fetch가 FETCH_HEAD를 여러 줄로 고쳐 쓴다 — 옛 길 git pull은
// 「Cannot fast-forward to multiple branches」로 실패, 새 길은 FETCH_HEAD를 읽지 않아 앞당긴다
func TestFFMainIgnoresRacingFetchHead(t *testing.T) {
	up := newRepo(t)
	gitT(t, up, "branch", "other")
	old, w := cloneOf(t, up), cloneOf(t, up)
	commitUp(t, up, "next")
	for _, d := range []string{old, w} {
		if err := os.WriteFile(d+"/.git/hooks/reference-transaction", []byte(racingFetch), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", old, "pull", "-q", "--ff-only").CombinedOutput(); err == nil || !strings.Contains(string(out), "multiple branches") {
		t.Fatalf("재현이 안 됨 — 옛 길 git pull이 실패해야: err=%v %s", err, out)
	}
	if why, div := ffMain(w); why != "" || div {
		t.Fatalf("새 길은 앞당겨야: why=%q 갈라짐=%v", why, div)
	}
	if h, u := gitT(t, w, "rev-parse", "HEAD"), gitT(t, up, "rev-parse", "HEAD"); h != u {
		t.Fatalf("앞당기지 않음: %s ≠ %s", h, u)
	}
}

// ④ 이미 최신 — 아무 일 없음. ② 로컬에 push 못 한 커밋이 있고 원격도 나아감 — 갈라짐으로 멈춘다(안내는 이때만)
func TestFFMainUpToDateAndDiverged(t *testing.T) {
	up := newRepo(t)
	w := cloneOf(t, up)
	h := gitT(t, w, "rev-parse", "HEAD")
	if why, div := ffMain(w); why != "" || div || gitT(t, w, "rev-parse", "HEAD") != h {
		t.Fatalf("이미 최신은 그대로: why=%q 갈라짐=%v", why, div)
	}
	os.WriteFile(w+"/mine", []byte("x\n"), 0o644)
	gitT(t, w, "add", "mine")
	gitT(t, w, "commit", "-qm", "mine")
	commitUp(t, up, "next")
	if why, div := ffMain(w); why == "" || !div {
		t.Fatalf("갈라짐은 멈추고 갈라짐 표시: why=%q 갈라짐=%v", why, div)
	}
}

// ③ 잠금 경합(2026-11-24): fetch의 참조 잠금은 gitx.Fetchr가, merge의 색인 잠금은 여기서 다시 해서 된다. 끝까지 안 풀리면 상한 뒤 까닭 한 줄
func TestFFMainRetriesLock(t *testing.T) {
	n := t.TempDir() + "/n"
	fakeBin(t, "git", "c=$(cat "+n+" 2>/dev/null || echo 0); c=$((c+1)); echo $c > "+n+"\n"+
		"case \"$3 $FAKE_MODE\" in\n"+
		"  'fetch lockref') [ $c -ge 3 ] && exit 0; echo \"error: cannot lock ref 'refs/remotes/origin/main': is at x but expected y\" >&2; exit 1 ;;\n"+
		"  fetch*) exit 0 ;;\n"+
		"  merge-base*) exit 0 ;;\n"+
		"  'merge lock') [ $c -ge 3 ] && exit 0; echo \"fatal: Unable to create '/r/.git/index.lock': File exists.\" >&2; exit 128 ;;\n"+
		"  'merge lockforever') echo \"fatal: Unable to create '/r/.git/index.lock': File exists.\" >&2; exit 128 ;;\n"+
		"  merge*) exit 0 ;;\n"+
		"esac\nexit 2\n")
	t.Setenv("HC_LANE_FETCH_WAITS", "0 0 0")
	old := pullWait
	pullWait = 0
	defer func() { pullWait = old }()
	count := func() string { b, _ := os.ReadFile(n); return strings.TrimSpace(string(b)) }

	for _, m := range []string{"lockref", "lock"} { // fetch 참조 잠금 두 번 / merge 색인 잠금 한 번 뒤 됨
		os.Remove(n)
		t.Setenv("FAKE_MODE", m)
		if why, div := ffMain("/r"); why != "" || div {
			t.Fatalf("%s: 잠금 뒤 됨: why=%q 부른 수 %s", m, why, count())
		}
	}
	os.Remove(n)
	t.Setenv("FAKE_MODE", "lockforever")
	if why, div := ffMain("/r"); !strings.Contains(why, "index.lock") || div || count() != "7" { // fetch 1 + merge 5 + merge-base 1
		t.Fatalf("잠금이 안 풀리면 상한 뒤 까닭: why=%q 갈라짐=%v 부른 수 %s", why, div, count())
	}
}
