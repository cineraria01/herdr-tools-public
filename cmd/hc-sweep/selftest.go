package main

// --self-test: 셸 판 lib/hc-sweep/selftest.py와 같은 단언(가짜 기록으로 규칙 판정, hc를 부르지 않는다). go test도 부른다

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func fakeRun(rc int, out, errs string, err error) runner {
	return func(time.Duration, []string, string, ...string) result { return result{out, errs, rc, err} }
}

func selfTest() (failed error) {
	n := 0
	check := func(ok bool, what string, a ...any) {
		n++
		if !ok && failed == nil {
			failed = fmt.Errorf("단언 %d: "+what, append([]any{n}, a...)...)
		}
	}
	never := func(string) bool { return false }
	check(indexGone([]string{"/a/x.wt/gone"}, never) && indexGone([]string{"/a/x.wt/g1", "/a/x.wt/g2"}, never), "indexGone 사라짐")
	check(!indexGone(nil, never), "빈 root")
	check(!indexGone([]string{"/a/x.wt/g1", "/a/live"}, func(p string) bool { return p == "/a/live" }), "하나라도 있음")
	check(!indexGone([]string{"/Volumes/usb/repo"}, never) && !indexGone([]string{"rel/repo"}, never), "/Volumes·상대")
	check(!indexGone([]string{"/Users/u/Documents/app/x"}, func(string) bool { return true }), "TCC 거부 = 있음")
	root, err := os.MkdirTemp("", "hc-sweep-self")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	root = realpath(root)
	os.Symlink("/Volumes/hc-sweep-self-test-없음", filepath.Join(root, "usb"))
	check(!indexGone([]string{filepath.Join(root, "usb", "repo")}, never), "링크로 /Volumes")
	check(indexRoots("/nonexistent/x.db", run) == nil, "indexRoots 없는 DB")

	now, h := 1_000_000_000_000.0, 60*minute
	keep := map[string]bool{"keepme": true, "t9": true}
	base := func(kv ...any) map[string]any {
		t := map[string]any{"address": "t1", "name": "claude-x", "state": "live", "status": map[string]any{"app": "claude-code", "state": "idle"},
			"created_ms": now - 3*h, "status_ms": now - 3*h + 500, "turn": 0.0, "parent": nil, "labels": map[string]any{}}
		for i := 0; i+1 < len(kv); i += 2 {
			if kv[i+1] == "<del>" {
				delete(t, kv[i].(string))
			} else {
				t[kv[i].(string)] = kv[i+1]
			}
		}
		return t
	}
	tail := ""
	ok := facts{worktree: no, unlanded: no, tail: &tail}
	rule := func(t map[string]any, f facts) string { r, _ := judge(t, f, now, keep); return r }
	done := map[string]any{"app": "claude-code", "state": "done"}
	check(rule(base(), ok) == "R1", "R1")
	check(rule(base("created_ms", now-20*minute, "status_ms", now-20*minute), ok) == "", "30분 전")
	check(rule(base("status_ms", now-2*h), ok) == "", "일했다 쉼 2시간")
	check(rule(base("turn", 3.0), ok) == "", "turn 3")
	check(rule(base("parent", "t0", "turn", 2.0, "status_ms", now-31*minute), ok) == "R2", "R2")
	check(rule(base("parent", "t0", "turn", 2.0, "status_ms", now-10*minute), ok) == "", "R2 10분")
	check(rule(base("parent", "t0", "status", done, "status_ms", now-h), ok) == "R2", "R2 done")
	check(rule(base("turn", 5.0, "created_ms", now-9*h, "status_ms", now-7*h), ok) == "R3", "R3")
	check(rule(base("turn", 5.0, "created_ms", now-9*h, "status_ms", now-5*h), ok) == "", "R3 전")
	for _, s := range []string{"working", "blocked", "error"} {
		check(rule(base("status", map[string]any{"app": "claude-code", "state": s}), ok) == "", "상태 %s", s)
	}
	for i, kv := range [][]any{{"status", nil}, {"status", map[string]any{"state": "idle"}}, {"state", "finished"}, {"address", "w1/t4rn"},
		{"name", "d1-loop-w1"}, {"name", "claude-proxy"}, {"name", "keepme"}, {"address", "t9"}, {"labels", map[string]any{"where": "proj / 1"}}} {
		check(rule(base(kv...), ok) == "", "보호 %d", i)
	}
	check(rule(base("name", "lane-watch-t7cc", "parent", "t7cc", "status_ms", now-2*h), ok) == "", "감시")
	check(rule(base("name", "w", "labels", map[string]any{"hc-lane-watch": "1"}, "parent", "t7cc", "status_ms", now-7*h), ok) == "", "감시 라벨")
	lane := []any{"parent", "t0", "turn", 4.0, "status", done, "status_ms", now - 31*minute, "labels", map[string]any{"where": "proj / Claude Code"}}
	with := func(kv ...any) map[string]any { return base(append(append([]any{}, lane...), kv...)...) }
	check(rule(with(), ok) == "R2", "탭에 붙인 worker")
	check(rule(with("parent", nil), ok) == "", "사람이 연 칸")
	check(rule(with("shell", true), ok) == "", "셸")
	f := ok
	f.unlanded = yes
	check(rule(with(), f) == "", "착지 전 커밋")
	check(rule(with("status_ms", now-7*h), f) == "R3", "R3는 보호 없음")
	for _, s := range []string{"  ⏵⏵ bypass permissions on · 2 shells", "⎿ 1 monitor still running", "  ⏵⏵ bypass permissions on · 1 shell · 1 monitor"} {
		s := s
		g := ok
		g.tail = &s
		check(rule(with(), g) == "", "배경 %s", s)
	}
	prompt := "  ⏵⏵ bypass permissions on (shift+tab to cycle)"
	g := ok
	g.tail = &prompt
	check(rule(with(), g) == "R2", "프롬프트")
	check(rule(base("shell", true, "labels", map[string]any{"herdr.space": "sp1", "herdr.tab": "tb1"}), ok) == "", "Ovis 셸")
	check(rule(base("labels", map[string]any{"herdr.space": "sp1", "herdr.tab": "tb1", "ovis.tab": "지휘"}), ok) == "R1", "Ovis 지휘 R1")
	check(rule(base("status_ms", nil, "created_ms", nil), ok) == "", "시각 없음")
	for _, w := range []tri{yes, missing} {
		g := ok
		g.worktree = w
		check(rule(with(), g) == "" && rule(with("status_ms", now-7*h), g) == "", "작업 트리 남음·모름")
	}
	g = ok
	g.unlanded = missing
	check(rule(with(), g) == "", "unlanded 모름")
	g = ok
	g.tail = nil
	check(rule(with(), g) == "", "tail 없음")
	check(rule(base("turn", nil), ok) == "", "turn 모름")
	fill := []any{"parent", nil, "turn", 5.0, "status", done, "status_ms", now - 31*minute, "labels", map[string]any{"hc-lane": "1"}}
	fl := func(kv ...any) map[string]any { return base(append(append([]any{}, fill...), kv...)...) }
	check(rule(fl(), ok) == "R2" && rule(fl("labels", map[string]any{"hc-lane": "1", "where": "p / C"}), ok) == "R2", "hc-fill 레인")
	g = ok
	g.fill = "up"
	check(rule(fl(), g) == "", "fill up")
	g.fill = "sent"
	check(rule(fl(), g) == "R2", "fill sent")
	check(rule(fl("turn", 0.0, "status_ms", now-3*h+500), ok) == "R2", "레인은 R1 아님")
	g = ok
	g.submitted = true
	check(rule(fl("status_ms", now-minute), g) == "R2", "제출 뒤 바로")
	g.worktree = yes
	check(rule(fl("status_ms", now-minute), g) == "", "제출 뒤라도 작업 트리")

	q := filepath.Join(root, "queue.tsv")
	check(fillState("x", "t5", q) == "", "대기열 없음")
	os.WriteFile(q, []byte("x\tp\t/b\t-\t-\tup\tm\tt5\t-\t2026\tmini:/in/done/x.md\ny\tp\t/b\t-\t-\tsent\tm\tt6\t-\t2026\tmini:/in/done/y.md\n"+
		"z\tp\t/b\t-\t-\tup\tm\tt7\t-\t2026\t-\no\tp\t/b\t-\t-\tup\tm\tt8\t-\t2026\n"), 0o644)
	check(fillState("x", "t5", q) == "up" && fillState("x", "t9", q) == "" && fillState("y", "t6", q) == "sent", "대기열 줄")
	check(fillState("z", "t7", q) == "" && fillState("o", "t8", q) == "", "되돌릴 곳 없음")
	if os.Getuid() != 0 {
		os.Chmod(q, 0)
		check(fillState("x", "t5", q) == "up", "못 읽으면 up")
		os.Chmod(q, 0o644)
	}

	mainR, wt := filepath.Join(root, "repo"), filepath.Join(root, "repo.wt", "lane")
	os.MkdirAll(filepath.Join(mainR, ".git"), 0o755)
	os.MkdirAll(filepath.Join(mainR, "sub"), 0o755)
	os.MkdirAll(filepath.Join(wt, "frontend"), 0o755)
	os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+mainR+"/.git/worktrees/lane\n"), 0o644)
	check(linkedWorktree(wt) && linkedWorktree(filepath.Join(wt, "frontend")), "링크된 워크트리")
	check(!linkedWorktree(mainR) && !linkedWorktree(filepath.Join(mainR, "sub")) && !linkedWorktree(filepath.Join(root, "repo.wt", "gone")), "메인·지운 것")
	check(linkedWorktree("") && linkedWorktree("rel/x"), "cwd 모름")
	if os.Getuid() != 0 {
		os.Chmod(filepath.Join(root, "repo.wt"), 0)
		check(linkedWorktree(wt), "읽기 오류는 남김")
		os.Chmod(filepath.Join(root, "repo.wt"), 0o755)
	}

	here, gone := root, filepath.Join(root, "no-such-dir")
	check(unlanded(here, fakeRun(0, "+ abc\n", "", nil)) && !unlanded(here, fakeRun(0, "- abc\n", "", nil)) && !unlanded(here, fakeRun(0, "", "", nil)), "cherry")
	check(!unlanded("", fakeRun(0, "+ x", "", nil)), "cwd 없음")
	check(!unlanded(here, fakeRun(128, "", "fatal: not a git repository (or any of the parent directories): .git", nil)), "저장소 아님")
	check(!unlanded(gone, fakeRun(0, "+ x\n", "", nil)), "폴더 없음(파일로 확인 — git을 부르지 않음)")
	// E07: 폴더가 있는데 git이 다른 까닭으로 실패 — 문구에 No such file or directory가 들어도 모름
	for _, bad := range []runner{fakeRun(-1, "", "", fmt.Errorf("시간 초과")), fakeRun(124, "", "timed out", nil), fakeRun(128, "", "fatal: bad revision 'origin/main'", nil),
		fakeRun(128, "", "fatal: unable to read .git/objects/info/alternates: No such file or directory", nil)} {
		check(unlanded(here, bad), "판정 실패는 남김")
	}
	check(busyRe.MatchString(screenTail("t1", fakeRun(1, "", "", nil))), "화면 못 읽음")
	check(!busyRe.MatchString(screenTail("t1", fakeRun(0, "a\n\n"+prompt+"\n", "", nil))), "화면 프롬프트")
	for _, a := range [][]string{{"/bin/zsh", "-l"}, {"-zsh"}, {"bash"}, {"zsh", "-i"}, {"/bin/bash", "--login"}} {
		check(isShellPane(a), "셸 %v", a)
	}
	for _, a := range [][]string{{"/bin/zsh", "-lc", "exec claude run"}, {"zsh", "-c", "x"}, {"bash", "/x/hc-loop-start"}, {"env", "claude"}, {"claude", "run"}} {
		check(!isShellPane(a), "셸 아님 %v", a)
	}
	check(laneRoot("/a/app.wt/l1/tests/../bin/x", ".wt") == "/a/app.wt/l1" && laneRoot("/a/app.wt/l1", ".wt") == "/a/app.wt/l1", "laneRoot")
	check(laneRoot("/a/app.wt", ".wt") == "" && laneRoot("/a/.wt/l1/x", ".wt") == "" && laneRoot("rel/app.wt/l1", ".wt") == "", "laneRoot 아님")
	check(laneRoot("/a/app.lanes/l1/x", ".lanes") == "/a/app.lanes/l1", "laneRoot 접미사")
	for s, want := range map[string]int{"05:03": 303, "1:00:00": 3600, "2-01:00:00": 2*86400 + 3600} {
		got, err := secs(s)
		check(err == nil && got == want, "secs %s", s)
	}
	there := map[string]bool{"/a/app.wt/l1": true, "/a/app.wt/l2": true, "/a/app.wt/l3": true, "/a/main.wt/m1": true}
	in := func(p string) bool { return there[p] }
	procs := map[int]procInfo{10: {1, 3600, "/usr/bin/python3 /a/app.wt/l1/tests/../bin/hc-fill watch --every 1"}, 11: {1, 3600, "sleep 300"},
		12: {1, 3600, "/a/app.wt/l3/bin/x"}, 13: {1, 3600, "/Applications/Foo.app/Contents/MacOS/Foo"},
		14: {1, 3600, "/a/app.wt/gone/bin/x --root=/a/app.wt/gone"}, 15: {99, 3600, "/a/app.wt/l1/bin/y"}, 16: {1, 30, "/a/app.wt/l1/bin/z"},
		17: {1, 3600, "/usr/local/bin/agent"}, 18: {1, 3600, "/b/app.wt/l9/bin/w"}, 19: {1, 3600, "/Users/u/.local/bin/cbm-mcp --daemon"},
		20: {1, 3600, "/a/main.wt/m1/bin/x"}, 22: {1, 3600, "/opt/h/herdr-core server"}}
	cwdOf := map[int]string{11: "/a/app.wt/l2/sub", 17: "/Users/u", 19: "/a/app.wt/gone", 22: "/a/app.wt/gone"}
	type row struct {
		pid       int
		root, why string
		kill      bool
	}
	var got []row
	for _, o := range findOrphans(procs, cwdOf, []string{"/a/app.wt/l3/frontend", "/a/app.wt/l2x", "/a/main/sub"}, ".wt", 600, 0, "/a", in,
		map[string]bool{"cbm-mcp": true, "herdr-core": true}, nil) {
		got = append(got, row{o.pid, o.root, o.why, o.kill})
	}
	check(reflect.DeepEqual(got, []row{{10, "/a/app.wt/l1", "쓰는 터미널 없음", true}, {11, "/a/app.wt/l2", "보류: cwd로만 걸림, 작업 트리 있음", false},
		{14, "/a/app.wt/gone", "작업 트리 지워짐", true}, {20, "/a/main.wt/m1", "보류: 본 저장소 터미널 있음", false}}), "고아 표 %v", got)
	var kill, hold []int
	for _, o := range findOrphans(procs, cwdOf, nil, ".wt", 600, 10, "", in, nil, nil) {
		if o.kill {
			kill = append(kill, o.pid)
		} else {
			hold = append(hold, o.pid)
		}
	}
	check(reflect.DeepEqual(kill, []int{12, 14, 18, 19, 20, 22}) && reflect.DeepEqual(hold, []int{11}), "고아 me·keep 없음 %v %v", kill, hold)
	core := "/x/Herdr Core.app/Contents/MacOS/herdr-core"
	sp := map[int]procInfo{23: {1, 3600, core + " server"}, 24: {1, 3600, "/x/My Tool.app/Contents/MacOS/tool"}}
	spc := map[int]string{23: "/a/app.wt/gone", 24: "/a/app.wt/gone"}
	o1 := findOrphans(sp, spc, nil, ".wt", 600, 0, "", never, map[string]bool{"herdr-core": true}, map[int]string{23: core, 24: "/x/My Tool.app/Contents/MacOS/tool"})
	check(len(o1) == 1 && o1[0].pid == 24, "공백 든 keep 경로")
	o2 := findOrphans(sp, map[int]string{23: "/a/app.wt/gone"}, nil, ".wt", 600, 0, "", never, map[string]bool{"herdr-core": true}, nil)
	check(len(o2) == 1 && o2[0].pid == 23, "comm 없으면 첫 낱말")
	e, err := exes(fakeRun(0, "  23 "+core+"\n 7 herdr-core\n x y\n", "", nil))
	check(err == nil && reflect.DeepEqual(e, map[int]string{23: core}), "exes %v", e)
	_, err = exes(fakeRun(1, "", "", nil))
	check(err != nil, "ps 실패를 삼킴")
	check(reflect.DeepEqual(tree([]int{10}, map[int]procInfo{10: {1, 0, ""}, 11: {10, 0, ""}, 12: {11, 0, ""}, 13: {1, 0, ""}}),
		map[int]bool{10: true, 11: true, 12: true}), "tree")
	v, okc := confWith("/nonexistent/tool", "sweep.wt_suffix", ".wt")
	check(okc && v == ".wt", "설정 도구 없음")
	tool := filepath.Join(root, "cfg")
	os.WriteFile(tool, []byte("#!/bin/sh\nexit 2\n"), 0o755)
	_, okc = confWith(tool, "sweep.wt_suffix", ".wt")
	check(!okc, "설정을 못 읽으면 끄지 않음")
	log := filepath.Join(root, "closed.log")
	os.WriteFile(log, []byte("t\tpid:5\t-\t고아 관찰(쓰는 터미널 없음)\t/a.wt/l\t-\t나이 0분 sleep 300\n"), 0o644)
	check(seen(5, "sleep 300", log) && !seen(6, "sleep 300", log) && !seen(5, "sleep 30", log), "seen")
	check(pyFloat(3) == "3.0" && pyFloat(0.5) == "0.5" && pyFloat(9e-06) == "9e-06" && pyFloat(1e9) == "1000000000.0", "pyFloat")
	return failed
}
