package main

// 「모름은 상태를 안 바꾼다」 단위 시험 — 2026-11-22 gofix2-ops(재감사 R-02·R-03·R-06·R-07).
// 가짜 git·hc를 PATH 앞에 두고 판정 함수를 직접 부른다(실제 기계·네트워크 없음)

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// fakeBin: PATH 앞 폴더에 이름 name의 sh 조각을 둔다. 진짜 git은 $REAL_GIT
func fakeBin(t *testing.T, name, body string) {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(d+"/"+name, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REAL_GIT") == "" { // 앞서 둔 가짜 git을 진짜로 잡지 않게 처음 한 번만
		if g, err := exec.LookPath("git"); err == nil {
			t.Setenv("REAL_GIT", g)
		}
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo: main 브랜치에 asset 하나를 커밋한 저장소
func newRepo(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	gitT(t, r, "init", "-q", "-b", "main")
	os.WriteFile(r+"/asset", []byte("initial\n"), 0o644)
	gitT(t, r, "add", "asset")
	gitT(t, r, "commit", "-qm", "i")
	return r
}

// R-02: symbolic-ref가 main을 찍고 실패(128)로 끝나면 main으로 읽지 않는다 — 실패한 조회의 출력은 버린다
func TestReadyRefFailsWithMainOutput(t *testing.T) {
	r := newRepo(t)
	fakeBin(t, "git", `for a; do [ "$a" = symbolic-ref ] && { echo main; exit 128; }; done
exec "$REAL_GIT" "$@"
`)
	if msg := readyState(r); msg == "" {
		t.Fatal("symbolic-ref 128인데 준비됨으로 읽었다")
	}
	fakeBin(t, "git", `exec "$REAL_GIT" "$@"
`)
	if msg := readyState(r); msg != "" {
		t.Fatalf("정상 저장소인데: %s", msg)
	}
}

// R-03: hc machine ls — 정상 빈 목록·hc 없음만 「기계 없음」, 실패·깨진 JSON·필수 칸 없는 행은 모름(오류)
func TestMachinesFromHC(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       string // 이름들(쉼표), 오류면 "!"
	}{
		{"정상", `echo '{"machines":[{"name":"a"},{"name":"b"}]}'`, "a,b"},
		{"정상 빈 목록", `echo '{"machines":[]}'`, ""},
		{"hc 실패", `echo '{"machines":[{"name":"a"}]}'; exit 1`, "!"},
		{"깨진 JSON", `echo '{"machines":[{"name":"a"}'`, "!"},
		{"값 둘", `echo '{"machines":[]}{"machines":[]}'`, "!"},
		{"machines 칸 없음", `echo '{}'`, "!"},
		{"이름 없는 행", `echo '{"machines":[{"name":"a"},{}]}'`, "!"},
		{"이름이 글자가 아님", `echo '{"machines":[{"name":3}]}'`, "!"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeBin(t, "hc", c.body+"\n")
			ms, err := machinesFromHC()
			got := strings.Join(ms, ",")
			if err != nil {
				got = "!"
			}
			if got != c.want {
				t.Errorf("%q, 바람 %q (err %v)", got, c.want, err)
			}
		})
	}
	t.Run("hc 없음", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if ms, err := machinesFromHC(); err != nil || len(ms) != 0 {
			t.Errorf("hc가 없으면 빈 목록: %v %v", ms, err)
		}
	})
}

// jsetup: 모으다 죽은 자리 — asset을 want 값으로 바꿔 쓰고(stage가 참이면 git add까지) 그 일지를 남긴다
func jsetup(t *testing.T, stage bool) (repo, old, want string) {
	t.Helper()
	repo = newRepo(t)
	STATE, TMP = t.TempDir(), t.TempDir()
	old = rsig(repo+"/asset", "asset")
	os.WriteFile(repo+"/asset", []byte("new value\n"), 0o644)
	want = rsig(repo+"/asset", "asset")
	if stage {
		gitT(t, repo, "add", "asset")
	}
	writeJournal([]jentry{{repo: repo, path: "asset", key: "asset", want: want, old: old}})
	return
}

func restored(t *testing.T, repo string) {
	t.Helper()
	if b, _ := os.ReadFile(repo + "/asset"); string(b) != "initial\n" {
		t.Errorf("작업 트리가 되돌아가지 않았다: %q", b)
	}
	if s := gitT(t, repo, "status", "--porcelain"); s != "" {
		t.Errorf("저장소가 깨끗하지 않다: %q", s)
	}
	if exists(journalPath()) {
		t.Error("일지가 남았다")
	}
}

// R-06: 파일을 바꿔 넣고 git add 전에 죽은 자리(작업 트리 = 새 값, 색인 = 옛 값)도 자기 변경으로 알아보고 되돌린다
func TestJournalRecoversReplacedNotStaged(t *testing.T) {
	repo, _, _ := jsetup(t, false)
	recoverJournal()
	restored(t, repo)
}

// 스테이징까지 한 자리(작업 트리 = 색인 = 새 값)도 그대로 되돌린다
func TestJournalRecoversStaged(t *testing.T) {
	repo, _, _ := jsetup(t, true)
	recoverJournal()
	restored(t, repo)
}

// 새 파일(쓰기 전 없음)·지움(쓸 값 없음)도 같은 셋 — 아직 안 씀은 건드리지 않고, 남이 손댄 값이 아니면 되돌린다
func TestJournalNewAndRemoved(t *testing.T) {
	for _, c := range []struct {
		name string
		do   func(repo string)
	}{
		{"새 파일 바꿔 넣음", func(r string) { os.WriteFile(r+"/fresh", []byte("f\n"), 0o644) }},
		{"새 파일 스테이징", func(r string) { os.WriteFile(r+"/fresh", []byte("f\n"), 0o644); gitT(t, r, "add", "fresh") }},
		{"지움 스테이징", func(r string) { gitT(t, r, "rm", "-q", "asset") }},
		{"아직 안 씀", func(string) {}},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := newRepo(t)
			STATE, TMP = t.TempDir(), t.TempDir()
			os.WriteFile(TMP+"/f", []byte("f\n"), 0o644)
			es := []jentry{
				{repo: repo, path: "fresh", key: "fresh", want: rsig(TMP+"/f", "fresh"), old: "-"},
				{repo: repo, path: "asset", key: "asset", want: "-", old: rsig(repo+"/asset", "asset")},
			}
			writeJournal(es)
			c.do(repo)
			recoverJournal()
			restored(t, repo)
			if exists(repo + "/fresh") {
				t.Error("새 파일이 남았다")
			}
		})
	}
}

// R-07: 되돌리기의 git 명령이 실패하면 오류 — 부른 쪽이 일지를 지우지 않는다(다음 실행이 다시 본다)
func TestUndoReportsFailure(t *testing.T) {
	repo, old, want := jsetup(t, true)
	fakeBin(t, "git", `for a; do [ "$a" = reset ] && exit 128; done
exec "$REAL_GIT" "$@"
`)
	es := []jentry{{repo: repo, path: "asset", key: "asset", want: want, old: old}}
	if err := undo(repo, es, "u"); err == nil {
		t.Fatal("reset이 실패했는데 오류가 없다")
	}
	fakeBin(t, "git", `exec "$REAL_GIT" "$@"
`)
	if err := undo(repo, es, "u2"); err != nil {
		t.Fatalf("정상 되돌리기: %v", err)
	}
	dropJournal()
	restored(t, repo)
}
