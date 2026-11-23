package main

// seat-clean-detached(2026-11-23): 감사 스크립트가 `git worktree add --detach`로 직접 만든 읽기 전용 작업 트리의 끝난 터미널을
// 30분 뒤 닫고 그 작업 트리를 지운다(gofix5에서 기본 다시 켬 — =0이면 닫기만). 변경·가지에 없는 커밋·가지 위·git 실패·.qa 보관물·
// 멈춘 git 작업은 지금 규칙(작업 트리 남음)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"herdr-tools/internal/clitest"
)

// withLs: hc ls는 ls를 돌려주고(빈 글이면 실패) hc exec git은 진짜로 돌리는 실행기. hc get <주소>는 ls의 그 항목에
// foreground.cwd를 붙여 돌려준다 — 항목의 시험 칸 "now"가 지금 폴더(없으면 cwd와 같음, "FAIL"이면 get 실패, null이면 그 칸 없음 —
// 옛 판 꼴), ls에 없는 주소는 not_found(종료 3)
func withLs(ls string) runner {
	return func(d time.Duration, env []string, name string, args ...string) result {
		if name == "hc" && len(args) > 0 && args[0] == "ls" {
			if ls == "" {
				return result{rc: 1, errs: "hc ls 실패"}
			}
			return result{out: ls}
		}
		if name == "hc" && len(args) > 1 && args[0] == "get" {
			return fakeGet(ls, args[1])
		}
		return j2HcExecGit(d, env, name, args...)
	}
}

func fakeGet(ls, addr string) result {
	var l struct{ Terminals []map[string]any }
	json.Unmarshal([]byte(ls), &l)
	for _, t := range l.Terminals {
		if t["address"] != addr {
			continue
		}
		now, has := t["now"]
		if !has {
			now = t["cwd"]
		}
		if now == "FAIL" {
			return result{rc: 1, errs: "error: cannot reach the core"}
		}
		delete(t, "now")
		fg := map[string]any{"name": "node", "pid": 7}
		if now != nil {
			fg["cwd"] = now
		}
		t["foreground"] = fg
		if s, ok := t["status"].(string); ok {
			t["status"] = map[string]any{"state": s}
		}
		b, _ := json.Marshal(t)
		return result{out: string(b)}
	}
	return result{rc: 3, errs: `{"error":{"kind":"not_found"}}`}
}

const noTerms = `{"terminals":[]}`

func TestDetachedClean(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/aud"
	j2Git(t, G, "init", "-q", "-b", "main", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)

	if got, top, _ := detachedClean(W, j2HcExecGit); got != R || top != W {
		t.Fatalf("분리·깨끗: 메인 저장소 %q·맨 위 %q 아님(%q %q)", R, W, got, top)
	}
	os.MkdirAll(W+"/sub", 0o755)
	if got, top, _ := detachedClean(W+"/sub", j2HcExecGit); got != R || top != W { // R-4A10: 루트는 cwd가 아니라 맨 위
		t.Fatalf("아래 폴더 cwd: 맨 위 %q 아님(%q %q)", W, got, top)
	}
	// R-4M01: 무시된 .qa 보관물(status에 안 나옴)·R-4A03: 멈춘 git 작업이면 아님
	os.WriteFile(R+"/.git/info/exclude", []byte(".qa/\n"), 0o644)
	os.MkdirAll(W+"/sub/.qa/x", 0o755)
	os.WriteFile(W+"/sub/.qa/x/REPORT.md", []byte("r\n"), 0o644)
	if got, _, why := detachedClean(W, j2HcExecGit); got != "" || !strings.Contains(why, ".qa") {
		t.Fatalf("무시된 .qa 보관물인데 분리·깨끗(%q %q)", got, why)
	}
	os.RemoveAll(W + "/sub/.qa")
	gd := j2Git(t, W, "rev-parse", "--absolute-git-dir")
	os.WriteFile(gd+"/MERGE_HEAD", []byte("x\n"), 0o644)
	if got, _, why := detachedClean(W, j2HcExecGit); got != "" || !strings.Contains(why, "MERGE_HEAD") {
		t.Fatalf("멈춘 merge인데 분리·깨끗(%q %q)", got, why)
	}
	os.Remove(gd + "/MERGE_HEAD")
	// R-4M02: 분리를 묻는 hc exec가 스스로 실패(종료 1)하면 모름
	failRev := func(d time.Duration, env []string, name string, args ...string) result {
		if strings.Contains(strings.Join(args, " "), "--symbolic-full-name") {
			return result{rc: 1, errs: "hc exec 실패"}
		}
		return j2HcExecGit(d, env, name, args...)
	}
	if got, _, _ := detachedClean(W, failRev); got != "" {
		t.Fatalf("hc exec 실패를 분리로 읽음(%q)", got)
	}
	// judge: worker가 아니어도(parent·hc-lane 라벨 없음) 30분 지난 done이면 R2, keep 목록·배경 실행이면 남김
	now := float64(time.Now().UnixMilli())
	term := map[string]any{"state": "live", "address": "t1", "name": "fable-audit-1", "cwd": W,
		"status": map[string]any{"app": "claude-code", "state": "done"}, "created_ms": now - 90*minute, "status_ms": now - 40*minute}
	quiet, busy := "끝", "still running"
	f := facts{worktree: no, unlanded: no, detached: true, tail: &quiet}
	if r, why := judge(term, f, now, nil); r != "R2" {
		t.Fatalf("분리·깨끗 40분: R2 아님(%q)", why)
	}
	if r, _ := judge(term, f, now, map[string]bool{"fable-audit-1": true}); r != "" {
		t.Fatal("keep 목록인데 닫음")
	}
	fb := f
	fb.tail = &busy
	if r, _ := judge(term, fb, now, nil); r != "" {
		t.Fatal("배경 실행 흔적인데 닫음")
	}
	if r, _ := judge(term, facts{worktree: no, unlanded: no, tail: &quiet}, now, nil); r != "" {
		t.Fatal("분리·깨끗 아닌 non-worker를 30분에 닫음(지금 규칙은 R3 6시간)")
	}

	// 변경 있음 → 아님, 지우기도 손대지 않음
	os.WriteFile(W+"/new.txt", []byte("x\n"), 0o644)
	if got, _, _ := detachedClean(W, j2HcExecGit); got != "" {
		t.Fatalf("변경 있음인데 분리·깨끗(%q)", got)
	}
	if ok, _, msg := worktreeRemove(R, W, withLs(noTerms)); ok || !isDir(W) {
		t.Fatalf("변경 있는 작업 트리를 지움(%v %s)", ok, msg)
	}
	os.Remove(W + "/new.txt")
	// R-4M06: 닫은 뒤 다시 볼 때 자신의 주소도 빼지 않는다(그 주소가 다시 쓰여 같은 폴더에 붙었을 수 있다)
	if ok, _, msg := worktreeRemove(R, W, withLs(`{"terminals":[{"address":"t1","state":"live","cwd":"`+W+`"}]}`)); ok || !isDir(W) {
		t.Fatalf("닫은 주소가 다시 그 폴더에 live인데 지움(%v %s)", ok, msg)
	}
	// 지우기: 깨끗하면 git worktree remove
	if ok, _, msg := worktreeRemove(R, W, withLs(noTerms)); !ok || isDir(W) {
		t.Fatalf("분리·깨끗한 작업 트리를 못 지움(%v %s)", ok, msg)
	}

	// 가지에 없는 커밋 → 아님
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)
	j2Git(t, W, "commit", "-q", "--allow-empty", "-m", "lane")
	if got, _, _ := detachedClean(W, j2HcExecGit); got != "" {
		t.Fatalf("가지에 없는 커밋인데 분리·깨끗(%q)", got)
	}
	// 가지 위 → 아님
	j2Git(t, W, "switch", "-q", "-c", "aud-b")
	if got, _, why := detachedClean(W, j2HcExecGit); got != "" || why != "" {
		t.Fatalf("가지 위인데 분리·깨끗·까닭(%q %q)", got, why)
	}
	// git 실패(저장소 아님) → 아님
	N := G + "/plain"
	os.MkdirAll(N, 0o755)
	if got, _, _ := detachedClean(N, j2HcExecGit); got != "" {
		t.Fatalf("git 실패인데 분리·깨끗(%q)", got)
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// seat-clean-fix(2026-11-23): 분리 작업 트리를 닫고 지우기 전에 hc ls로 그 폴더(또는 그 아래)를 cwd로 쓰는 다른 live 터미널을 본다.
// 11-23 끝난 Mythos 감사 터미널 여섯을 닫으며 같은 폴더를 쓰던 확인 세션들의 작업 트리를 지운 일 뒤
func TestDetachedInUse(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/aud"
	j2Git(t, G, "init", "-q", "-b", "main", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)
	os.MkdirAll(W+"/sub", 0o755) // 빈 폴더는 git status에 안 보인다
	os.MkdirAll(W+"2", 0o755)
	os.Symlink(W, G+"/link") // 다른 터미널이 심볼릭 링크로 들어가 있어도 같은 폴더
	os.MkdirAll(W+"/sub/deep", 0o755)
	os.Symlink(W+"/sub", G+"/lsub") // S5-01: 링크가 맨 위가 아니라 그 아래 폴더를 가리킴

	term := func(addr, cwd string) string {
		return `{"address":"` + addr + `","state":"live","status":"working","cwd":"` + cwd + `"}`
	}
	list := func(ts ...string) string { return `{"terminals":[` + strings.Join(ts, ",") + `]}` }
	owner := term("t1", W)
	// cwd-now-inuse(2026-11-23): 띄운 폴더(cwd)는 G인데 띄운 뒤 cd로 들어온 지금 폴더(hc get foreground.cwd, 시험 칸 now)
	moved := func(addr, now string) string {
		return `{"address":"` + addr + `","state":"live","status":"working","cwd":"` + G + `","now":` + now + `}`
	}

	for _, c := range []struct {
		name   string
		others []string // nil이면 hc ls 실패
		remove bool
	}{
		{"hc ls 실패", nil, false},
		{"다른 live 터미널이 그 아래 cwd", []string{term("t2", W+"/sub")}, false},
		{"다른 live 터미널이 링크로 같은 폴더", []string{term("t2", G+"/link")}, false},
		{"다른 live 터미널이 아래 폴더를 가리키는 링크(S5-01)", []string{term("t2", G+"/lsub")}, false},
		{"다른 live 터미널이 그 링크 밑 폴더(S5-01)", []string{term("t2", G+"/lsub/deep")}, false},
		{"다른 live 터미널이 다른 철자(대소문자)로 같은 폴더(R-4A03)", []string{term("t2", clitest.AltSpelling(t, W, false))}, false},
		{"이 기계 모름 항목", []string{`{"address":"t3","state":"live","cwd":7}`}, false},
		{"띄운 뒤 그 작업 트리로 들어온 터미널(지금 폴더)", []string{moved("t2", `"`+W+`"`)}, false},
		{"띄운 뒤 그 아래 폴더로 들어온 터미널(지금 폴더)", []string{moved("t2", `"`+W+`/sub/deep"`)}, false},
		{"지금 폴더를 모름(hc get 실패)", []string{moved("t2", `"FAIL"`)}, false},
		{"지금 폴더를 모름(foreground.cwd 없음 — 옛 판)", []string{moved("t2", `null`)}, false},
		{"띄운 폴더가 그 작업 트리, 지금은 딴 곳(지금처럼 쓰는 중)", []string{`{"address":"t2","state":"live","cwd":"` + W + `","now":"` + G + `"}`}, false},
		// 지우는 장면은 맨 끝 하나만(지우고 나면 W가 없다). cwd-now-inuse: 띄운·지금 폴더 모두 딴 곳이면 지금처럼 지운다 —
		// 다른 기계 터미널의 get 실패는 이 기계 후보를 막지 않는다
		{"끝난 터미널·다른 기계·형제 폴더 이름 접두만·지금 폴더도 딴 곳", []string{`{"address":"t4","state":"finished","cwd":"` + W + `"}`,
			term("m1/t5", W), term("t6", W+"2"), moved("t2", `"`+G+`"`), `{"address":"m1/t7","state":"live","cwd":"/x","now":"FAIL"}`}, true},
	} {
		judgeLs, closedLs := "", "" // 판정 때는 자신(t1)이 목록에 있고, 닫은 뒤에는 빠진다
		if c.others != nil {
			judgeLs, closedLs = list(append([]string{owner}, c.others...)...), list(c.others...)
		}
		if why := dirInUse(W, "t1", withLs(judgeLs)); (why == "") != c.remove {
			t.Fatalf("%s: dirInUse %q", c.name, why)
		}
		ok, _, msg := worktreeRemove(R, W, withLs(closedLs))
		if ok != c.remove || isDir(W) == c.remove {
			t.Fatalf("%s: 지움 %v(기대 %v) — %s", c.name, ok, c.remove, msg)
		}
	}
}

// gofix5(다시 켜기): 기본은 닫고 지운다. HC_SWEEP_DETACHED_REMOVE=0이면 닫기만 하고 작업 트리를 남긴다(끄는 손잡이)
func TestDetachedRemoveOff(t *testing.T) {
	G, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	R, W := G+"/repo", G+"/repo.wt/aud"
	j2Git(t, G, "init", "-q", "-b", "main", R)
	j2Git(t, R, "commit", "-q", "--allow-empty", "-m", "base")
	j2Git(t, R, "worktree", "add", "-q", "--detach", W)

	t.Setenv("HC_SWEEP_DETACHED_REMOVE", "0")
	line, why, _, bad := detachedAfterClose(R, W, withLs(noTerms))
	if !strings.Contains(line, "작업 트리 남김(분리 지우기 꺼짐)") || why != "" || bad || !isDir(W) {
		t.Fatalf("=0인데 지움·기록함(%q %q, 남음 %v)", line, why, isDir(W))
	}
	for _, v := range []string{"", "1"} {
		if !isDir(W) {
			j2Git(t, R, "worktree", "add", "-q", "--detach", W)
		}
		t.Setenv("HC_SWEEP_DETACHED_REMOVE", v)
		line, why, msg, bad := detachedAfterClose(R, W, withLs(noTerms))
		if isDir(W) || bad || why != "작업 트리 정리(분리·깨끗, git worktree remove)" || !strings.Contains(msg, W) {
			t.Fatalf("=%q인데 못 지움(%q %q %q)", v, line, why, msg)
		}
	}
}
