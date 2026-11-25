//go:build !windows

package main

// 추측 검증 기록 건너뛰기: 병합기가 넘긴 추측 트리(HERDR_SPEC_TREE)가 rebase한 트리이고 원격 기록이 있을 때만 HERDR_VERIFIED_TREE를 둔다.
// 병합기 밖이거나, 트리가 다르거나, 기록이 없거나, 검증 스크립트가 바뀌었으면 두지 않는다

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"herdr-tools/internal/gitx"
	"herdr-tools/internal/verified"
)

func lvGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	o, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, o)
	}
	return strings.TrimSpace(string(o))
}

func TestVerifiedEnv(t *testing.T) {
	d, _ := filepath.EvalSymlinks(t.TempDir())
	for k, v := range map[string]string{"HOME": d, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	lvGit(t, d, "init", "-q", "--bare", "-b", "main", d+"/o.git")
	lvGit(t, d, "clone", "-q", d+"/o.git", d+"/w")
	w := d + "/w"
	os.MkdirAll(w+"/.claude", 0o777)
	// 검증 스크립트(이 저장소 것)를 그대로 — 건너뛰기 조건도 함께 본다
	src, err := os.ReadFile("../../.claude/herdr-verify.sh")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(w+"/.claude/herdr-verify.sh", src, 0o777)
	lvGit(t, w, "add", "-A")
	lvGit(t, w, "commit", "-qm", "c")
	lvGit(t, w, "push", "-q", "origin", "main")
	t.Chdir(w)
	cand := lvGit(t, w, "rev-parse", "HEAD")
	tree := lvGit(t, w, "rev-parse", "HEAD^{tree}")
	t.Setenv("HERDR_VERIFIED_TREE", "stale")
	t.Setenv("HERDR_SPEC_TREE", tree)
	t.Setenv("HERDR_VERIFY_PARTS_FROM", "/x/direct.parts")
	specTreeTake(false)
	verifiedEnv(cand)
	if _, ok := os.LookupEnv("HERDR_VERIFIED_TREE"); ok || specTree != "" {
		t.Fatal("병합기 밖: 물려받은 값을 지우고 두지 않는다")
	}
	if _, ok := os.LookupEnv("HERDR_VERIFY_PARTS_FROM"); ok {
		t.Fatal("병합기 밖: 직접 검증 조각 파일도 지운다")
	}
	t.Setenv("HERDR_SPEC_TREE", tree)
	t.Setenv("HERDR_VERIFY_PARTS_FROM", "/x/direct.parts")
	specTreeTake(true)
	if os.Getenv("HERDR_VERIFY_PARTS_FROM") != "/x/direct.parts" {
		t.Fatal("병합기 안: 조각 파일은 검증·착지 스크립트에 물려준다")
	}
	verifiedEnv(cand)
	if _, ok := os.LookupEnv("HERDR_VERIFIED_TREE"); ok {
		t.Fatal("기록이 없으면 두지 않는다")
	}
	if verified.Record(w, verified.Rec{Tree: tree, Machine: "box", At: 1}, os.Stderr) != gitx.Yes {
		t.Fatal("기록 실패")
	}
	verifiedEnv(cand)
	if os.Getenv("HERDR_VERIFIED_TREE") != tree {
		t.Fatal("같은 트리·기록 있음이면 둔다")
	}
	// 검증 스크립트가 건너뛴다(트리 같음·깨끗함·원격 기록 있음)
	o, err := exec.Command("bash", ".claude/herdr-verify.sh").CombinedOutput()
	if err != nil || !strings.Contains(string(o), "건너뜀 검증") {
		t.Fatalf("검증 스크립트가 건너뛰어야: %v\n%s", err, o)
	}
	// 지금 트리가 다르면(추적 파일 변경) 그대로 검증한다 — 첫 줄 안내만 본다
	os.WriteFile(w+"/x.txt", []byte("x\n"), 0o666)
	lvGit(t, w, "add", "x.txt")
	c := exec.Command("bash", "-c", "bash .claude/herdr-verify.sh 2>&1 | head -1")
	if o, _ := c.CombinedOutput(); !strings.Contains(string(o), "맞지 않다") {
		t.Fatalf("트리가 다르면 건너뛰지 않음: %s", o)
	}
	lvGit(t, w, "reset", "-q", "x.txt")
	os.Remove(w + "/x.txt")
	// 내장 길: rebase한 트리가 추측 트리와 다르면 두지 않는다
	os.WriteFile(w+"/y.txt", []byte("y\n"), 0o666)
	lvGit(t, w, "add", "y.txt")
	lvGit(t, w, "commit", "-qm", "y")
	verifiedEnv(lvGit(t, w, "rev-parse", "HEAD"))
	if _, ok := os.LookupEnv("HERDR_VERIFIED_TREE"); ok {
		t.Fatal("트리가 다르면 두지 않는다")
	}
}
