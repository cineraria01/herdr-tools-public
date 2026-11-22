package hclane_test

// tests/hc-lane.test.sh 13·14절: hc-lane clean(무시된 파일만 남았고 브랜치가 origin에 들어갔을 때만 지운다).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() { reg(30, "s13-14", cleanSec) }

// s13cat: 셸 "$(cat 파일)"(끝 줄바꿈 뗌, 없으면 빈 값).
func s13cat(p string) string { return strings.TrimRight(read(p), "\n") }

// s13f: 셸 [ -f 파일 ].
func s13f(p string) bool { fi, err := os.Stat(p); return err == nil && fi.Mode().IsRegular() }

// s13pair: grep -Fx -A1 -- opt | grep -Fxq val.
func s13pair(l []string, opt, val string) bool {
	for i, s := range l {
		if s == opt && i+1 < len(l) && l[i+1] == val {
			return true
		}
	}
	return false
}

// s13lsj: 셸 lsj <주소> <cwd>(가짜 hc ls --json 한 터미널).
func s13lsj(addr, cwd string) string {
	return fmt.Sprintf(`{"machines":[{"name":"box","state":"connected"}],"terminals":[{"address":"%s","agent":"claude-code","cwd":"%s","state":"live"}]}`, addr, cwd)
}

func (k *K) s13ref(name string) bool {
	return k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/"+name)
}

func (k *K) s13clean(br string, extra ...string) {
	k.run(append([]string{"clean", "--repo", k.R, "--branch", br}, extra...)...)
}

func cleanSec(k *K) {
	td := filepath.Join(k.T, "teardown-ran")
	k.mk("t13")
	k.seed(".gitignore", ".omc/")
	k.seed(".claude/herdr-teardown.sh", `echo "$PWD" >"$HOME/teardown-ran"`)
	w := func(n string) string { return k.R + ".wt/" + n }

	k.land("c-ok")
	os.Remove(td)
	k.s13clean("c-ok")
	k.check("clean: 무시 폴더만 남은 워크트리 → 종료 0", k.rc == 0)
	k.check("clean: 폴더·등록·브랜치 정리", k.gone("c-ok"))
	k.check("clean: teardown은 워크트리 안에서", s13cat(td) == w("c-ok"))
	k.check("clean: 결과 JSON(done, teardown ran)", k.last(`"clean":"done"`) && k.last(`"teardown":"ran"`))
	k.check("clean: 같이 지우는 무시 파일을 알림", strings.Contains(k.out, "무시된 것도 같이 지운다: .omc/"))
	k.s13clean("c-ok")
	k.check("clean: 다시 돌리면 정리할 것 없음, 종료 0", k.rc == 0 && k.last(`"clean":"nothing"`))

	k.land("c-untracked")
	write(k.t, w("c-untracked")+"/note.txt", "keep\n")
	os.Remove(td)
	k.s13clean("c-untracked")
	k.check("clean: 무시되지 않은 untracked → 멈추고 목록", k.rc != 0 && strings.Contains(k.errf(), "note.txt") && k.last(`"clean":"stopped"`))
	k.check("clean: 그때 아무것도 안 지움(파일·무시 폴더·등록·브랜치, teardown도 안 돌림)", k.kept("c-untracked") && k.regd("c-untracked") &&
		s13f(w("c-untracked")+"/note.txt") && s13f(w("c-untracked")+"/.omc/state/s.json") && !exists(td))

	// 정리가 worktree remove 도중 끊긴 자리: 변경이 지움뿐이면 이어서 끝내고, 다른 변경이 섞이면 멈춘다
	k.land("c-cut")
	os.Remove(w("c-cut") + "/c-cut.txt")
	k.s13clean("c-cut")
	k.check("clean: 끊긴 정리(변경이 지움뿐) → 이어서 지움, 종료 0", k.rc == 0 && k.gone("c-cut") && strings.Contains(k.out, "정리가 끊긴 자리: 추적 파일 1개"))
	k.land("c-cut2")
	os.Remove(w("c-cut2") + "/c-cut2.txt")
	write(k.t, w("c-cut2")+"/new.txt", "x\n")
	k.s13clean("c-cut2")
	k.check("clean: 지움에 새 파일이 섞이면 멈춤(안 지움)", k.rc == 1 && k.kept("c-cut2") && k.regd("c-cut2") && strings.Contains(k.errf(), "new.txt"))
	os.Remove(w("c-cut2") + "/new.txt")
	k.sh(w("c-cut2"), "git checkout -q -- c-cut2.txt")
	k.s13clean("c-cut2") // 뒤 시험에 남기지 않게

	k.land("c-tracked")
	appendf(k.t, w("c-tracked")+"/a", "changed\n")
	k.s13clean("c-tracked")
	k.check("clean: 추적 파일 변경 → 멈추고 아무것도 안 지움", k.rc != 0 && strings.Contains(k.errf(), "M a") && k.kept("c-tracked") && k.regd("c-tracked") &&
		lastLine(s13cat(w("c-tracked")+"/a")) == "changed")

	k.run("--name", "c-unpushed", "--repo", k.R, "--", "true")
	k.sh(w("c-unpushed"), "echo u >u && git add -- u && git commit -qm u -- u")
	k.s13clean("c-unpushed")
	k.check("clean: origin/main에 안 들어간 브랜치 → 멈추고 아무것도 안 지움", k.rc != 0 && strings.Contains(k.errf(), "origin/main") && k.kept("c-unpushed") &&
		k.regd("c-unpushed") && s13f(w("c-unpushed")+"/u"))

	// --archive(명시할 때만): 작업 폴더가 깨끗하면 원격 refs/archive/<가지>-<sha8>에 보관한 뒤 지운다
	k.run("--name", "c-arch", "--repo", k.R, "--", "true")
	k.sh(w("c-arch"), "echo r >r && git add -- r && git commit -qm r -- r")
	ca := k.git(k.R, "rev-parse", "c-arch")
	hook := k.D + "/origin.git/hooks/pre-receive"
	write(k.t, hook, "#!/bin/sh\nwhile read o n r; do case $r in refs/archive/*) echo no-archive >&2; exit 1 ;; esac; done\n")
	os.Chmod(hook, 0o755)
	k.s13clean("c-arch", "--archive")
	k.check("clean --archive: push가 거절되면 아무것도 안 지움", k.rc != 0 && k.kept("c-arch") && k.regd("c-arch") && s13f(w("c-arch")+"/r"))
	os.Remove(hook)
	write(k.t, w("c-arch")+"/dirt.txt", "dirt\n")
	k.s13clean("c-arch", "--archive")
	k.check("clean --archive: 작업 폴더가 깨끗하지 않으면 보관·삭제 없이 멈춤", k.rc != 0 && k.kept("c-arch") && k.git(k.D+"/origin.git", "for-each-ref", "refs/archive") == "")
	os.Remove(w("c-arch") + "/dirt.txt")
	k.s13clean("c-arch", "--archive")
	k.check("clean --archive: 원격 refs/archive/<가지>-<sha8>에 보관하고 정리", k.rc == 0 && k.gone("c-arch") &&
		len(ca) >= 8 && k.git(k.D+"/origin.git", "rev-parse", "refs/archive/c-arch-"+ca[:8]) == ca)

	// 메인 폴더 뒤처짐: 앞당김을 건너뛰는 꼴(다른 가지 — .gitignore 전 첫 커밋)로 만든다
	k.sh(k.R, `git checkout -q -b behind "$(git rev-list --max-parents=0 HEAD)"`)
	k.land("c-left") // remove가 등록은 지웠고 무시 폴더(와 지워진 등록을 가리키는 .git 파일)만 남음
	k.sh(k.R, `git worktree remove --force "$1"`, w("c-left"))
	write(k.t, w("c-left")+"/.omc/state/s.json", "s\n")
	write(k.t, w("c-left")+"/.git", "gitdir: "+k.R+"/.git/worktrees/c-left\n")
	k.s13clean("c-left")
	k.check("clean: 메인 폴더에 그 .gitignore가 아직 없으면(뒤처짐) 남은 폴더는 멈추고 그대로(안전 쪽)", k.rc != 0 && s13f(w("c-left")+"/.omc/state/s.json") && k.s13ref("c-left"))
	k.sh(k.R, "git checkout -q main && git branch -q -D behind")
	k.sh(k.R, "git pull -q --ff-only")
	k.s13clean("c-left")
	k.check("clean: 등록 없이 무시 폴더만 남은 폴더 → 폴더·브랜치 정리", k.rc == 0 && k.gone("c-left"))
	k.land("c-left2")
	k.sh(k.R, `git worktree remove --force "$1"`, w("c-left2"))
	write(k.t, w("c-left2")+"/.omc/s", "s\n")
	write(k.t, w("c-left2")+"/mine.txt", "keep\n")
	k.s13clean("c-left2")
	k.check("clean: 등록 없는 폴더에 무시되지 않은 파일 → 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), "mine.txt") && s13f(w("c-left2")+"/mine.txt") &&
		s13f(w("c-left2")+"/.omc/s") && k.s13ref("c-left2"))
	k.sh(k.T, `mkdir -p "$1" && git init -q "$1" && mkdir -p "$1/.omc" && echo s >"$1/.omc/s"`, w("c-left3"))
	k.s13clean("c-left3")
	k.check("clean: 등록 없는 폴더가 다른 git 저장소면 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), ".git") && isDir(w("c-left3")+"/.git"))

	k.land("c-cwd")
	cmd := exec.Command(k.LANE, "clean", "--repo", k.R, "--branch", "c-cwd")
	cmd.Dir, cmd.Env = w("c-cwd"), k.environ(nil)
	b, _ := cmd.CombinedOutput()
	write(k.t, k.T+"/cwd.out", string(b))
	k.rc = -1
	if cmd.ProcessState != nil {
		k.rc = cmd.ProcessState.ExitCode()
	}
	k.check("clean: 워크트리 안에서 불러도 정리하고 cd 안내", k.rc == 0 && k.gone("c-cwd") && strings.Contains(read(k.T+"/cwd.out"), "cd "+sq(k.R)))

	k.land("c-on")
	os.Remove(k.HCLOG + ".exec")
	k.run("clean", "--on", "box", "--repo", k.R, "--name", "c-on")
	ex := k.hclog(".exec")
	k.check("clean --on: git·파일 일은 hc exec --on box로, 정리됨", k.rc == 0 && k.gone("c-on") && s13pair(ex, "--on", "box") && contains(ex, "worktree") &&
		contains(ex, "branch") && k.last(`"on":"box"`))

	k.land("c-dry")
	os.Remove(td)
	ref := k.git(k.R, "rev-parse", "origin/main")
	k.run("clean", "--dry-run", "--repo", k.R, "--branch", "c-dry")
	k.check("clean --dry-run: 종료 0, 아무것도 안 지우고 teardown·fetch도 안 함", k.rc == 0 && k.kept("c-dry") && k.regd("c-dry") &&
		s13f(w("c-dry")+"/.omc/state/s.json") && !exists(td) && k.git(k.R, "rev-parse", "origin/main") == ref)
	k.check("clean --dry-run: 할 일과 JSON", strings.Contains(k.out, "[dry-run] git -C") && k.last(`"dry_run":true`))
	k.run("clean", "--dry-run", "--repo", k.R, "--branch", "c-unpushed")
	k.check("clean --dry-run: 안 들어간 브랜치는 dry-run도 멈춤", k.rc != 0 && k.kept("c-unpushed"))

	k.s13clean("main")
	k.check("clean: 기본 브랜치는 거절(그때도 JSON 한 줄)", k.rc != 0 && k.s13ref("main") && k.last(`"clean":"stopped"`))
	k.run("clean", "--repo", k.R)
	k.check("clean: --branch·--name 없으면 거절", k.rc != 0)

	// 14. 검토 반영(2026-11-05): 설정이 숨긴 untracked, 폴더 이름만 같은 다른 브랜치, 살아 있는 터미널,
	//     메인 .gitignore 로컬 변경, 무시 규칙에도 걸리는 추적 파일, 폴더 없는 분리 HEAD
	k.land("c-hide")
	k.sh(k.R, "git config status.showUntrackedFiles no")
	write(k.t, w("c-hide")+"/note.txt", "keep\n")
	k.s13clean("c-hide")
	k.sh(k.R, "git config --unset status.showUntrackedFiles")
	k.check("clean: status.showUntrackedFiles=no여도 untracked를 보고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "note.txt") && s13f(w("c-hide")+"/note.txt") && k.kept("c-hide"))

	k.run("--name", "c-fx", "--branch", "feat/x", "--repo", k.R, "--", "true")
	k.s13clean("feat-x")
	k.check("clean: 폴더 이름만 같은 다른 브랜치(feat/x)의 워크트리는 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), "feat/x") && k.regd("feat-x") && k.s13ref("feat/x"))

	k.land("c-live")
	k.runE([]string{"HC_FAKE_LS=" + s13lsj("t7", w("c-live")+"/sub")}, "clean", "--repo", k.R, "--branch", "c-live")
	k.check("clean: 살아 있는 다른 터미널이 그 폴더에서 돌면 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), "t7") && k.kept("c-live") && k.regd("c-live"))
	k.runE([]string{"HC_FAKE_LS_FAIL=1"}, "clean", "--repo", k.R, "--branch", "c-live")
	k.check("clean: hc ls가 실패하면 멈춤(확인 못 한 것을 없음으로 읽지 않음)", k.rc != 0 && strings.Contains(k.errf(), "hc ls") && k.kept("c-live"))
	k.runE([]string{"HC_TERMINAL_ID=t7", "HC_FAKE_LS=" + s13lsj("t7", w("c-live"))}, "clean", "--repo", k.R, "--branch", "c-live")
	k.check("clean: 그 터미널이 부른 자신이면(착지 절차의 레인) 정리", k.rc == 0 && k.gone("c-live"))
	k.land("c-live2")
	k.runE([]string{"HC_FAKE_LS=" + s13lsj("box/t7", w("c-live2"))}, "clean", "--repo", k.R, "--branch", "c-live2")
	k.check("clean: 다른 기계 터미널의 같은 경로는 막지 않음", k.rc == 0 && k.gone("c-live2"))
	k.land("c-live3")
	k.runE([]string{"HC_FAKE_LS=" + s13lsj("box/t7", w("c-live3"))}, "clean", "--on", "box", "--repo", k.R, "--branch", "c-live3")
	k.check("clean --on: 그 기계 터미널이 폴더에서 돌면 멈춤", k.rc != 0 && strings.Contains(k.errf(), "box/t7") && k.kept("c-live3"))
	// 재감사 R-06: JSON을 구조로 읽으니 칸 차례(cwd가 address 앞)와 상관없이 읽고, 칸이 빠지면 못 읽음으로 멈춘다
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"cwd":"` + w("c-live3") + `","address":"t8","state":"live"}]}`}, "clean", "--repo", k.R, "--branch", "c-live3")
	k.check("clean: cwd가 address 앞이어도 읽고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "t8") && k.kept("c-live3"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"cwd":"/x"}]}`}, "clean", "--repo", k.R, "--branch", "c-live3")
	k.check("clean: hc ls 형식을 못 읽으면(address 없음) 멈춤", k.rc != 0 && strings.Contains(k.errf(), "hc ls --json 출력을 읽지 못했다") && k.kept("c-live3"))
	k.runE([]string{`HC_FAKE_LS={"terminals":[{"address" : "t7", "cwd" : "` + w("c-live3") + `", "state" : "live"}]}`}, "clean", "--repo", k.R, "--branch", "c-live3")
	k.check("clean: 콜론 뒤 공백 꼴도 읽고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "t7") && k.kept("c-live3"))
	k.sh(k.R, `git worktree add -q --detach "$1" origin/main && git branch -q --no-track c-dh origin/main`, w("c-dh"))
	k.s13clean("c-dh")
	k.check("clean: 분리 HEAD 폴더인데 같은 이름 브랜치가 따로 있으면 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), "분리 HEAD") && k.regd("c-dh") && k.s13ref("c-dh"))

	k.land("c-gi")
	k.sh(k.R, `git worktree remove --force "$1"`, w("c-gi"))
	write(k.t, w("c-gi")+"/.omc/s", "s\n")
	appendf(k.t, k.R+"/.gitignore", "*.tmp\n")
	k.s13clean("c-gi")
	k.check("clean: 메인 .gitignore에 로컬 변경이 있으면 남은 폴더 판정을 멈춤", k.rc != 0 && strings.Contains(k.errf(), ".gitignore") && s13f(w("c-gi")+"/.omc/s"))
	k.sh(k.R, "git checkout -q -- .gitignore")
	k.s13clean("c-gi")
	k.check("clean: 되돌리면 정리", k.rc == 0 && k.gone("c-gi"))

	k.sh(k.D+"/seed", "git pull -q --rebase origin main && mkdir -p .omc && echo k >.omc/keep && git add -f .omc/keep && git commit -qm keep && git push -q origin HEAD:main")
	k.sh(k.R, "git pull -q --ff-only")
	k.land("c-trk")
	k.sh(k.R, `git worktree remove --force "$1"`, w("c-trk"))
	write(k.t, w("c-trk")+"/.omc/keep", "changed\n")
	k.s13clean("c-trk")
	k.check("clean: 등록 없는 폴더의 추적 파일(무시 규칙에도 걸림)은 멈추고 그대로", k.rc != 0 && strings.Contains(k.errf(), ".omc/keep") && s13cat(w("c-trk")+"/.omc/keep") == "changed")

	k.sh(k.R, `git worktree add -q --detach "$1" origin/main`, w("c-det"))
	k.sh(w("c-det"), "echo d >d && git add -- d && git commit -qm d -- d")
	os.RemoveAll(w("c-det"))
	k.s13clean("c-det")
	k.check("clean: 폴더가 없어도 등록의 분리 HEAD가 origin에 없으면 멈춤", k.rc != 0 && strings.Contains(k.errf(), "워크트리 HEAD") && k.regd("c-det"))

	// clean-qa(2026-11-19): 무시된 보관물 .qa는 메인 폴더의 같은 자리로 복사(덮지 않음)한 뒤 지운다
	appendf(k.t, k.R+"/.git/info/exclude", ".qa/\n")
	k.land("c-qa")
	write(k.t, w("c-qa")+"/.qa/d1/run/out 1.txt", "new\n")
	write(k.t, w("c-qa")+"/.qa/d1/s.txt", "same\n")
	write(k.t, k.R+"/.qa/d1/s.txt", "same\n")
	write(k.t, w("c-qa")+"/sub/.qa/x", "deep\n")
	k.run("clean", "--dry-run", "--repo", k.R, "--branch", "c-qa")
	k.check("clean .qa --dry-run: 옮길 것만 알리고 복사·삭제 없음", k.rc == 0 && !exists(k.R+"/.qa/d1/run") && k.kept("c-qa") && strings.Contains(k.out, "[dry-run] 무시된 보관물"))
	k.s13clean("c-qa")
	k.check("clean .qa: 메인 폴더 같은 자리로 복사하고 정리(하위 폴더 .qa도)", k.rc == 0 && k.gone("c-qa") && s13cat(k.R+"/.qa/d1/run/out 1.txt") == "new" &&
		s13cat(k.R+"/sub/.qa/x") == "deep")
	moved := false
	for _, l := range strings.Split(k.out, "\n") {
		if strings.Contains(l, "옮김 "+w("c-qa")+"/.qa → "+k.R+"/.qa") && strings.Contains(l, "파일 3개") {
			moved = true
		}
	}
	k.check("clean .qa: 옮긴 경로를 한 줄로", moved)
	k.land("c-qa2")
	write(k.t, w("c-qa2")+"/.qa/d1/s.txt", "mine\n")
	write(k.t, w("c-qa2")+"/.qa/d1/e.txt", "extra\n")
	k.s13clean("c-qa2")
	k.check("clean .qa: 메인에 같은 이름의 다른 파일 → 덮지 않고 그 파일을 알리고 멈춤(안 지움)", k.rc != 0 && s13cat(k.R+"/.qa/d1/s.txt") == "same" &&
		s13cat(w("c-qa2")+"/.qa/d1/s.txt") == "mine" && k.kept("c-qa2") && k.regd("c-qa2") && strings.Contains(k.errf(), "다름(메인 것 둠) .qa/d1/s.txt"))
	os.Remove(k.R + "/.qa/d1/s.txt")
	k.s13clean("c-qa2")
	k.check("clean .qa: 비교·정리한 뒤 다시 돌리면 옮기고 정리", k.rc == 0 && k.gone("c-qa2") && s13cat(k.R+"/.qa/d1/s.txt") == "mine" && s13cat(k.R+"/.qa/d1/e.txt") == "extra")
	k.land("c-qa3")
	write(k.t, w("c-qa3")+"/.qa/on/f", "far\n")
	k.run("clean", "--on", "box", "--repo", k.R, "--branch", "c-qa3")
	k.check("clean --on .qa: 그 기계 안에서 메인 폴더로 복사하고 정리", k.rc == 0 && k.gone("c-qa3") && s13cat(k.R+"/.qa/on/f") == "far")
	// clean-qa-quotepath(2026-11-19): 한글·공백 이름도 옮김
	k.land("c-qa4")
	write(k.t, w("c-qa4")+"/.qa/전체 검증/결과 1.txt", "ko\n")
	k.s13clean("c-qa4")
	k.check("clean .qa: 한글·공백 이름 파일도 메인 폴더로 옮기고 정리", k.rc == 0 && k.gone("c-qa4") && s13cat(k.R+"/.qa/전체 검증/결과 1.txt") == "ko")

	k.mk("t14")
	k.seed(".gitignore", ".omc/")
	k.seed(".claude/herdr-teardown.sh", "echo td-boom; exit 3")
	k.land("c-td")
	k.s13clean("c-td")
	k.check("clean: teardown이 실패해도 알리고 정리는 계속(herdr-teardown.sh 약속)", k.rc == 0 && k.gone("c-td") && strings.Contains(k.errf(), "td-boom") && k.last(`"teardown":"failed"`))
}
