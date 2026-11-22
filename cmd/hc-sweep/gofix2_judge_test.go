package main

// gofix2-base R-01: 제출·착지 증거는 같은 실행에 묶는다 — 옛 꼴(실행 키 없음) 사건의 시각이 이 터미널이 생기기 전이거나 모호하면
// 증거가 아니고, 원격 표식 refs/hc-landed/<가지>/<초>는 그 커밋이 이 가지의 착지여야 한다(생성 직전 1초 허용도 없음)

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func j2Term(addr, name string, created float64) map[string]any {
	return map[string]any{"address": addr, "name": name, "created_ms": created}
}

func j2Row(at time.Time, addr, name, ev string) []string {
	return []string{at.UTC().Format("2006-01-02T15:04:05Z"), "m", addr, name, ev, "abc1234", "-", "3"}
}

// [R-01] 2020년 옛 꼴 「제출」 줄이 1초 전에 생긴 같은 주소·이름 터미널의 제출 증거가 되면 안 된다(R2를 바로 고름)
func TestJ2SubmittedOldRow(t *testing.T) {
	ev := filepath.Join(t.TempDir(), "events.tsv")
	created := float64(time.Now().Add(-time.Second).UnixMilli())
	os.WriteFile(ev, []byte("2020-01-01T00:00:00Z\tm\tt1\tlane-a\t제출\tabc1234\tsub → 큐\t3\n"), 0o644)
	if submitted(j2Term("t1", "lane-a", created), ev) {
		t.Error("2020년 옛 꼴 제출 줄을 생성 1초 터미널의 제출로 봄")
	}
	// 대조: 같은 실행 키(<주소>@<created_ms>)의 제출은 증거
	os.WriteFile(ev, []byte("2026-11-21T10:00:00Z\tm\tt1\tlane-a\t제출\tabc1234\tsub → 큐\t3\t-\tt1@"+pyStr(created)+"\n"), 0o644)
	if !submitted(j2Term("t1", "lane-a", created), ev) {
		t.Error("같은 실행 키의 제출을 못 봄(대조)")
	}
}

// [R-01] 옛 꼴 사건의 초가 생성 전(X-1초)이면 이 실행 것이 아니고, 생성과 같은 초(생성이 그 초 중간)면 모호 — 둘 다 증거 아님
func TestJ2ThisRunOldRowTime(t *testing.T) {
	x := time.Now().Truncate(time.Second).Add(-time.Hour)
	whole := float64(x.UnixMilli())
	if thisRun(j2Term("t1", "lane-a", whole), j2Row(x.Add(-time.Second), "t1", "lane-a", "착지")) {
		t.Error("생성 1초 전 옛 꼴 사건을 이 실행 것으로 봄")
	}
	half := whole + 500
	if thisRun(j2Term("t1", "lane-a", half), j2Row(x, "t1", "lane-a", "착지")) {
		t.Error("생성과 같은 초(생성이 그 초 중간)의 옛 꼴 사건 — 모호한데 이 실행 것으로 봄")
	}
	key := append(j2Row(x.Add(time.Minute), "t1", "lane-a", "착지"), "-", "t1@"+pyStr(whole))
	if !thisRun(j2Term("t1", "lane-a", whole), key) {
		t.Error("같은 실행 키 사건을 못 봄(대조)")
	}
}

// j2GitEnv: 바깥 git 설정·GIT_* 없이
func j2GitEnv() []string {
	var e []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			e = append(e, kv)
		}
	}
	return append(e, "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
}

func j2Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir, c.Env = dir, j2GitEnv()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// j2HcExecGit: hcGit이 부르는 「hc exec --cwd D … -- git …」을 진짜 git으로(어떤 git 명령으로 대조하든 맞게)
func j2HcExecGit(_ time.Duration, _ []string, name string, args ...string) result {
	if name != "hc" || len(args) == 0 || args[0] != "exec" {
		return result{rc: 1, errs: "j2: 모르는 명령 " + name}
	}
	cwd, i := "", 1
	for ; i < len(args) && args[i] != "--"; i++ {
		if args[i] == "--cwd" && i+1 < len(args) {
			cwd = args[i+1]
		}
	}
	if i+1 >= len(args) {
		return result{rc: 1, errs: "j2: -- 없음"}
	}
	c := exec.Command(args[i+1], args[i+2:]...)
	c.Dir, c.Env = cwd, j2GitEnv()
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	r := result{out: o.String(), errs: e.String()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		r.rc = ee.ExitCode()
	} else if err != nil {
		r.rc, r.err = -1, err
	}
	return r
}

// [R-01] 원격 표식: 생성 1초 전 초의 표식, 이 가지의 착지가 아닌(관계없는) 커밋을 가리키는 표식은 증거 아님.
// 이 터미널 뒤 초에 이 가지 HEAD(=origin/main으로 착지한 커밋)를 가리키는 표식은 증거(대조)
func TestJ2LandedMarkerCommitAndTime(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/land-n"
	j2Git(t, G, "init", "-q", "--bare", "-b", "main", G+"/origin.git")
	j2Git(t, G, "clone", "-q", G+"/origin.git", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "push", "-q", "origin", "HEAD:main")
	j2Git(t, R, "worktree", "add", "-q", "-b", "land-n", W, "origin/main")
	os.WriteFile(W+"/a", []byte("a\n"), 0o644)
	j2Git(t, W, "add", "a")
	j2Git(t, W, "commit", "-q", "-m", "lane")
	j2Git(t, W, "push", "-q", "origin", "HEAD:main")
	j2Git(t, R, "fetch", "-q", "origin")
	head := j2Git(t, W, "rev-parse", "HEAD")
	side := j2Git(t, R, "commit-tree", "-m", "side", j2Git(t, R, "rev-parse", "HEAD^{tree}")) // 이 가지·main과 관계없는 커밋

	x := time.Now().Truncate(time.Second).Add(-time.Hour).Unix()
	term := map[string]any{"cwd": W, "name": "land-n", "address": "t2", "created_ms": float64(x * 1000)}
	ev := filepath.Join(G, "no-events.tsv")
	mark := func(sha string, sec int64) func() {
		ref := "refs/hc-landed/land-n/" + strconv.FormatInt(sec, 10)
		j2Git(t, R, "push", "-q", "origin", sha+":"+ref)
		return func() { j2Git(t, R, "push", "-q", "origin", ":"+ref) }
	}

	undo := mark(head, x-1)
	if repo, _, why := landedClean(term, j2HcExecGit, ev); repo != "" {
		t.Errorf("생성 1초 전 초의 원격 표식을 착지 증거로 봄(%q)", why)
	}
	undo()
	undo = mark(side, x+60)
	if repo, _, why := landedClean(term, j2HcExecGit, ev); repo != "" {
		t.Errorf("이 가지의 착지가 아닌 커밋을 가리킨 원격 표식을 착지 증거로 봄(%q)", why)
	}
	undo()
	mark(head, x+60)
	if repo, br, why := landedClean(term, j2HcExecGit, ev); repo != R || br != "land-n" {
		t.Errorf("대조: 이 가지 HEAD를 가리킨 뒤 초 표식은 착지여야 함: %q %q %q", repo, br, why)
	}
}
