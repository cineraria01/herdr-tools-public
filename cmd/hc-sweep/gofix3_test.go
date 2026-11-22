package main

// gofix3-sweep: Go 전환 재감사(아스트라 2바퀴) base/R-201 시험 — 원인(실제 git 상태)을 먼저 단언한 뒤 금지 결과를 본다(②-F 시험 규칙 4)

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// g3Merge: dir에서 other를 병합하다 충돌하면 f를 body로 풀어 커밋(충돌은 git이 1로 끝나니 오류를 보지 않는다)
func g3Merge(t *testing.T, dir, other, body string) {
	t.Helper()
	c := exec.Command("git", "merge", "-q", "--no-edit", other)
	c.Dir, c.Env = dir, j2GitEnv()
	c.CombinedOutput()
	os.WriteFile(filepath.Join(dir, "f"), []byte(body+"\n"), 0o644)
	j2Git(t, dir, "add", "f")
	j2Git(t, dir, "commit", "-q", "--no-edit", "-m", "merge "+body)
}

// [R-201] 같은 두 부모를 레인과 main이 각각 병합하고 충돌을 다르게 풀면 git cherry는 병합을 보지 않아 빈 출력이다. main 쪽 병합을
// 가리킨 뒤 초의 착지 표식이 있어도 HEAD가 그 표식 커밋의 조상이 아니면 레인 결과가 main에 보존됐는지 입증할 수 없다 — 남긴다
func TestG3LandedRewrittenMerge(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/land-m"
	j2Git(t, G, "init", "-q", "--bare", "-b", "main", G+"/origin.git")
	j2Git(t, G, "clone", "-q", G+"/origin.git", R)
	os.WriteFile(R+"/f", []byte("base\n"), 0o644)
	j2Git(t, R, "add", "f")
	j2Git(t, R, "commit", "-q", "-m", "base")
	j2Git(t, R, "push", "-q", "origin", "HEAD:main")
	for _, p := range []string{"one", "two"} {
		j2Git(t, R, "checkout", "-q", "-b", "p-"+p, "origin/main")
		os.WriteFile(R+"/f", []byte(p+"\n"), 0o644)
		j2Git(t, R, "commit", "-q", "-am", p)
	}
	j2Git(t, R, "checkout", "-q", "--detach", "p-one")
	g3Merge(t, R, "p-two", "main-result")
	j2Git(t, R, "push", "-q", "origin", "HEAD:main")
	j2Git(t, R, "fetch", "-q", "origin")
	j2Git(t, R, "worktree", "add", "-q", "-b", "land-m", W, "p-one")
	g3Merge(t, W, "p-two", "lane-result")
	x := time.Now().Truncate(time.Second).Add(-time.Hour).Unix()
	j2Git(t, R, "push", "-q", "origin", "origin/main:refs/hc-landed/land-m/"+strconv.FormatInt(x+60, 10))
	// 원인 먼저: cherry는 빔, 두 결과는 다르다
	if c := j2Git(t, W, "cherry", "origin/main", "HEAD"); c != "" {
		t.Fatalf("준비: git cherry가 비지 않음 %q", c)
	}
	if a, b := j2Git(t, W, "show", "HEAD:f"), j2Git(t, W, "show", "origin/main:f"); a != "lane-result" || b != "main-result" {
		t.Fatalf("준비: 두 병합 결과 %q %q", a, b)
	}
	term := map[string]any{"cwd": W, "name": "land-m", "address": "t3", "created_ms": float64(x * 1000)}
	if repo, _, why := landedClean(term, j2HcExecGit, filepath.Join(G, "no-events.tsv")); repo != "" {
		t.Errorf("레인 병합 결과가 main에 없는데 착지·정리 후보로 봄(%q)", why)
	}
}
