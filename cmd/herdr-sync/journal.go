package main

// 모음 일지(gofix-ops B05): 저장소에 쓰고 스테이징하기 전에 「저장소 · 그때 HEAD · 경로 · 키 · 쓸 값의 서명 · 쓰기 전 서명」을
// STATE/repo-journal.tsv에 적고, 커밋하거나 되돌린 뒤 지운다. 쓰기와 commit 사이에 죽으면(SIGKILL) 다음 실행이 ready 앞에서 읽어,
// HEAD가 그대로이고 그 경로들이 일지 그대로면 그 경로만 되돌리고 이어 간다.
// 하나라도 다르면 남이 그 뒤에 손댄 것일 수 있어 건드리지 않고 멈춘다(예전처럼 「커밋하지 않은 변경」 — 손으로 본다)

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"herdr-tools/internal/atomic"
)

func journalPath() string { return STATE + "/repo-journal.tsv" }

// jentry: 일지 한 줄. old = 쓰기 전 저장소 서명(없으면 -, 옛 다섯 칸 일지는 "")
type jentry struct{ repo, head, path, key, want, old string }

// writeJournal: 모을 것들을 쓰기 전에 적는다(저장소마다 지금 HEAD와 함께)
func writeJournal(es []jentry) {
	if len(es) == 0 {
		return
	}
	heads := map[string]string{}
	var b strings.Builder
	for _, e := range es {
		h, ok := heads[e.repo]
		if !ok {
			var rc int
			if h, rc = gitq("-C", e.repo, "rev-parse", "HEAD"); rc != 0 || h == "" {
				die("저장소 HEAD를 읽지 못했다: %s", e.repo)
			}
			heads[e.repo] = h
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\n", e.repo, h, e.path, e.key, e.want, e.old)
	}
	if err := atomic.WriteFile(journalPath(), []byte(b.String()), 0o600); err != nil {
		die("모음 일지를 쓰지 못했다: %v", err)
	}
}

func dropJournal() {
	if err := os.Remove(journalPath()); err != nil && !os.IsNotExist(err) {
		die("모음 일지를 지우지 못했다: %v", err)
	}
}

// recoverJournal: 앞 실행이 남긴 일지가 있으면 자기 작업만 되돌린다(위 설명)
func recoverJournal() {
	data, err := os.ReadFile(journalPath())
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		die("모음 일지를 읽지 못했다: %v", err)
	}
	byRepo := map[string][]jentry{}
	var order []string
	for _, l := range splitLines(string(data)) {
		f := strings.Split(l, "\t")
		if len(f) == 5 {
			f = append(f, "")
		}
		if len(f) != 6 {
			die("모음 일지가 깨졌다: %s — 저장소를 손으로 확인한 뒤 지운다", journalPath())
		}
		e := jentry{f[0], f[1], f[2], f[3], f[4], f[5]}
		if _, ok := byRepo[e.repo]; !ok {
			order = append(order, e.repo)
		}
		byRepo[e.repo] = append(byRepo[e.repo], e)
	}
	for _, repo := range order {
		es := byRepo[repo]
		h, rc := gitq("-C", repo, "rev-parse", "HEAD")
		if rc != 0 {
			die("저장소 HEAD를 읽지 못했다: %s", repo)
		}
		if h != es[0].head {
			continue // 그 뒤에 커밋됐다 — 되돌릴 스테이징이 없다(안 보낸 커밋은 이번 실행이 보낸다)
		}
		// 경로마다 셋 가운데 하나여야 자기 것이다(R-06): 아직 안 씀(작업 트리 = old, 색인 = HEAD = 작업 트리),
		// 바꿔 넣음(작업 트리 = want, 색인 = HEAD), 스테이징함(작업 트리 = want = 색인). 그 밖은 남이 손댄 것일 수 있어 멈춘다
		var ours []jentry
		for _, e := range es {
			s := rsig(repo+"/"+e.path, e.key)
			wq := gitDiffQuiet(repo, e.path)
			cq := gitDiffQuiet(repo, e.path, "--cached")
			switch {
			case e.old != "" && s == e.old && wq && cq:
			case s == e.want && (wq || cq):
				ours = append(ours, e)
			default:
				die("앞 herdr-sync가 모으다 멈춘 뒤 %s/%s가 바뀌었다 — 손으로 확인한 뒤 %s를 지운다", repo, e.path, journalPath())
			}
		}
		if len(ours) == 0 {
			continue
		}
		if err := undo(repo, ours, "jr"); err != nil {
			die("앞 herdr-sync가 모으다 멈춘 것을 되돌리지 못했다 (%s: %v) — 일지 %s를 남긴다, 다시 돌린다", repo, err, journalPath())
		}
		warn("앞 herdr-sync가 모으다 멈춘 스테이징을 되돌렸다 (%s: %d개) — 이번에 다시 모은다", repo, len(ours))
	}
	dropJournal()
}

// undo: 모으다 실패하면 이번에 쓴·스테이징한 경로만 되돌린다(다음 실행이 「커밋하지 않은 변경」으로 막히지 않게).
// 추적 파일은 HEAD 값으로, 새 파일은 지운다. 그 사이 남이 같은 저장소에 쓴 것은 건드리지 않는다.
// git 명령·지우기가 하나라도 실패하거나, 끝난 뒤 자기 경로가 쓰기 전 값(old)·색인 = HEAD로 돌아왔음을 확인하지 못하면 오류 —
// 부른 쪽은 일지를 지우지 않는다(다음 실행이 다시 본다, R-07)
func undo(repo string, es []jentry, name string) error {
	var paths []string
	for _, e := range es {
		paths = append(paths, e.path)
	}
	if len(paths) == 0 {
		return nil
	}
	lf := writeLines(name, paths)
	if _, rc := gitq("-C", repo, "--literal-pathspecs", "reset", "-q", "--pathspec-from-file="+lf); rc != 0 {
		return fmt.Errorf("git reset 종료 %d", rc)
	}
	st, rc := gitq("-C", repo, "status", "--porcelain", "-z", "--untracked-files=all")
	if rc != 0 {
		return fmt.Errorf("git status 종료 %d", rc)
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	var tracked, untracked []string
	for _, e := range strings.Split(st, "\x00") {
		if len(e) < 4 || !want[e[3:]] {
			continue
		}
		if e[:2] == "??" {
			untracked = append(untracked, e[3:])
		} else {
			tracked = append(tracked, e[3:])
		}
	}
	if len(tracked) > 0 {
		if _, rc := gitq("-C", repo, "--literal-pathspecs", "checkout", "-q", "--pathspec-from-file="+writeLines(name+".t", tracked)); rc != 0 {
			return fmt.Errorf("git checkout 종료 %d", rc)
		}
	}
	for _, p := range untracked {
		if err := os.Remove(filepath.Join(repo, p)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, e := range es { // 옛 다섯 칸 일지는 old를 모른다 — 아래 status만 본다
		if e.old != "" && rsig(repo+"/"+e.path, e.key) != e.old {
			return fmt.Errorf("%s가 쓰기 전 값으로 돌아오지 않았다", e.path)
		}
	}
	st, rc = gitq("-C", repo, "status", "--porcelain", "-z", "--untracked-files=all")
	if rc != 0 {
		return fmt.Errorf("git status 종료 %d", rc)
	}
	for _, e := range strings.Split(st, "\x00") {
		if len(e) >= 4 && want[e[3:]] {
			return fmt.Errorf("되돌린 뒤에도 바뀜이 남았다: %s", e)
		}
	}
	return nil
}

// gitDiffQuiet: git diff [--cached] --quiet -- p가 같음(0)인가. 다름(1)이면 거짓, 그 밖(실패·시간 넘김 — 모름)이면 멈춘다
func gitDiffQuiet(repo, p string, opt ...string) bool {
	args := append(append([]string{"-C", repo, "--literal-pathspecs", "diff"}, opt...), "--quiet", "--", p)
	_, rc := gitq(args...)
	switch rc {
	case 0:
		return true
	case 1:
		return false
	}
	die("저장소 상태를 읽지 못했다 (git diff 종료 %d — 모름): %s/%s — 일지 %s를 남긴다", rc, repo, p, journalPath())
	return false
}
