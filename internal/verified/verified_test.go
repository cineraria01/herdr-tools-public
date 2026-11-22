package verified

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/gitx"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	o, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, o)
	}
	return strings.TrimSpace(string(o))
}

// fixture: 임시 bare 원격과 클론 둘(a·b). a에 검증 스크립트가 든 커밋
func fixture(t *testing.T) (a, b, tree string) {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"HOME": d, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	git(t, d, "init", "-q", "--bare", "-b", "main", d+"/o.git")
	a, b = d+"/a", d+"/b"
	git(t, d, "init", "-q", "-b", "main", a)
	os.MkdirAll(a+"/.claude", 0o777)
	os.WriteFile(a+"/.claude/herdr-verify.sh", []byte("#!/bin/bash\nexit 0\n"), 0o777)
	git(t, a, "add", "-A")
	git(t, a, "commit", "-qm", "base")
	git(t, a, "remote", "add", "origin", d+"/o.git")
	git(t, a, "push", "-q", "origin", "main")
	git(t, d, "clone", "-q", d+"/o.git", b)
	return a, b, git(t, a, "rev-parse", "HEAD^{tree}")
}

func TestRecordLookup(t *testing.T) {
	a, b, tree := fixture(t)
	if _, st := Lookup(b, tree, io.Discard); st != gitx.No {
		t.Fatalf("기록 전 Lookup = %v, 기대 없음", st)
	}
	if st := Record(a, Rec{Tree: tree, Machine: "box", At: 100}, io.Discard); st != gitx.Yes {
		t.Fatalf("Record = %v", st)
	}
	// 다른 클론(기록 객체가 없음)에서 찾는다 — 받아 와서 본다
	r, st := Lookup(b, tree, io.Discard)
	if st != gitx.Yes || r.Machine != "box" || r.At != 100 || r.Tree != tree {
		t.Fatalf("Lookup = %+v %v", r, st)
	}
	blob := git(t, a, "rev-parse", "HEAD:.claude/herdr-verify.sh")
	if r.Script != blob {
		t.Fatalf("script = %s, 기대 %s", r.Script, blob)
	}
	// 두 번째 기록은 있는 것을 그대로(덮지 않음)
	if st := Record(a, Rec{Tree: tree, Machine: "other", At: 200}, io.Discard); st != gitx.Yes {
		t.Fatalf("두 번째 Record = %v", st)
	}
	if r, _ := Lookup(b, tree, io.Discard); r.Machine != "box" {
		t.Fatalf("두 번째 기록이 덮었다: %+v", r)
	}
	was := git(t, a, "ls-remote", "origin", Prefix+tree)
	was, _, _ = strings.Cut(was, "\t")
	if !Remove(a, tree, was, io.Discard) {
		t.Fatal("Remove 실패")
	}
	if _, st := Lookup(b, tree, io.Discard); st != gitx.No {
		t.Fatalf("지운 뒤 Lookup = %v", st)
	}
}

// 기록 제목이 다른 트리를 말하거나 스크립트 blob이 다르면 없음
func TestLookupRejectsMismatch(t *testing.T) {
	a, b, tree := fixture(t)
	bad := Rec{Tree: tree, Script: strings.Repeat("0", 40), Machine: "box", At: 1}
	c := git(t, a, "commit-tree", tree, "-m", bad.Message())
	git(t, a, "push", "-q", "origin", c+":"+Prefix+tree)
	if _, st := Lookup(b, tree, io.Discard); st != gitx.No {
		t.Fatalf("blob 다른 기록 Lookup = %v, 기대 없음", st)
	}
	// 트리가 다른 커밋을 그 이름에 둠
	et := git(t, a, "hash-object", "-t", "tree", "-w", "/dev/null")
	c2 := git(t, a, "commit-tree", et, "-m", Rec{Tree: tree, Script: strings.Repeat("1", 40), Machine: "x", At: 1}.Message())
	git(t, a, "push", "-q", "-f", "origin", c2+":"+Prefix+tree)
	if _, st := Lookup(b, tree, io.Discard); st != gitx.No {
		t.Fatalf("트리 다른 기록 Lookup = %v, 기대 없음", st)
	}
}

// 원격을 못 보면 모름(없음으로 읽지 않는다)
func TestLookupUnknown(t *testing.T) {
	_, b, tree := fixture(t)
	git(t, b, "remote", "set-url", "origin", "/nonexistent/o.git")
	if _, st := Lookup(b, tree, io.Discard); st != gitx.Unknown {
		t.Fatalf("원격 없음 Lookup = %v, 기대 모름", st)
	}
}

// 검증 스크립트가 없는 트리는 기록하지 않는다
func TestRecordNoScript(t *testing.T) {
	a, _, _ := fixture(t)
	et := git(t, a, "hash-object", "-t", "tree", "-w", "/dev/null")
	if st := Record(a, Rec{Tree: et, Machine: "box", At: 1}, io.Discard); st != gitx.No {
		t.Fatalf("스크립트 없는 Record = %v, 기대 없음", st)
	}
}

func TestParse(t *testing.T) {
	r := Rec{Tree: strings.Repeat("a", 40), Script: strings.Repeat("b", 40), Machine: "m", At: 5}
	got, ok := Parse(r.Message() + "\n\n본문")
	if !ok || got != r {
		t.Fatalf("Parse = %+v %v", got, ok)
	}
	for _, bad := range []string{"", "herdr-verified tree=x", "other tree=" + r.Tree, strings.Replace(r.Message(), "at=5", "at=x", 1)} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) ok", bad)
		}
	}
}
