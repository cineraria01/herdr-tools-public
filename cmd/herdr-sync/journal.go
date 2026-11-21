package main

// 모음 일지(gofix-ops B05): 저장소에 쓰고 스테이징하기 전에 「저장소 · 그때 HEAD · 경로 · 키 · 쓸 값의 서명」을 STATE/repo-journal.tsv에
// 적고, 커밋하거나 되돌린 뒤 지운다. git add와 commit 사이에 죽으면(SIGKILL) 다음 실행이 ready 앞에서 읽어,
// HEAD가 그대로이고 그 경로들이 일지에 적은 값 그대로면(작업 트리 = 적은 서명, 색인 = 작업 트리) 그 경로만 되돌리고 이어 간다.
// 하나라도 다르면 남이 그 뒤에 손댄 것일 수 있어 건드리지 않고 멈춘다(예전처럼 「커밋하지 않은 변경」 — 손으로 본다)

import (
	"fmt"
	"os"
	"strings"

	"herdr-tools/internal/atomic"
)

func journalPath() string { return STATE + "/repo-journal.tsv" }

// jentry: 일지 한 줄
type jentry struct{ repo, head, path, key, want string }

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
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", e.repo, h, e.path, e.key, e.want)
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
		if len(f) != 5 {
			die("모음 일지가 깨졌다: %s — 저장소를 손으로 확인한 뒤 지운다", journalPath())
		}
		e := jentry{f[0], f[1], f[2], f[3], f[4]}
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
		var paths []string
		for _, e := range es {
			if rsig(repo+"/"+e.path, e.key) != e.want {
				die("앞 herdr-sync가 모으다 멈춘 뒤 %s/%s가 바뀌었다 — 손으로 확인한 뒤 %s를 지운다", repo, e.path, journalPath())
			}
			if _, rc := gitq("-C", repo, "--literal-pathspecs", "diff", "--quiet", "--", e.path); rc != 0 {
				die("앞 herdr-sync가 모으다 멈춘 뒤 %s/%s의 색인과 작업 트리가 다르다 — 손으로 확인한 뒤 %s를 지운다", repo, e.path, journalPath())
			}
			paths = append(paths, e.path)
		}
		undo(repo, paths, "jr")
		warn("앞 herdr-sync가 모으다 멈춘 스테이징을 되돌렸다 (%s: %d개) — 이번에 다시 모은다", repo, len(paths))
	}
	dropJournal()
}
