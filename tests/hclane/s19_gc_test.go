package hclane_test

// tests/hc-lane.test.sh 19·19b·19c절: hc-lane gc(작업 폴더·가지, --integ, 작업 폴더 없이 남은 가지·--remote-branches).

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func init() { reg(70, "s19", gcSections) }

// s19lines: 출력 줄들. s19hasLine: 그 줄이 그대로 있다(grep -Fx).
func s19lines(s string) []string  { return strings.Split(s, "\n") }
func s19hasLine(s, x string) bool { return contains(s19lines(s), x) }
func s19old() string              { return fmt.Sprintf("%d +0000", time.Now().Unix()-172800) } // 이틀 전
func s19gw(k *K, br string) {
	k.sh(k.T, `git -C "$1" worktree add -q -b "$2" "$1.wt/$2" origin/main`, k.R, br)
}
func s19gwt(k *K, br string) bool { return k.regd(br) }
func s19gbr(k *K, br string) bool {
	return k.gitOK(k.R, "show-ref", "-q", "--verify", "refs/heads/"+br)
}
func s19oref(k *K, ref string) bool {
	return k.gitOK(k.D+"/origin.git", "show-ref", "-q", "--verify", ref)
}
func s19gcm(k *K, br string) {
	k.sh(k.R+".wt/"+br, `echo "$1" >"$1.txt" && git add -- "$1.txt" && git commit -qm "$1" -- "$1.txt"`, br)
}

// s19gstate: 작업 폴더 목록·로컬 참조·원격 참조(셸 gstate의 cksum 대신 글 그대로 견줌).
func s19gstate(k *K) string {
	return k.git(k.R, "worktree", "list", "--porcelain") + "\n" + k.git(k.R, "for-each-ref") + "\n" + k.git(k.D+"/origin.git", "for-each-ref")
}

// s19lineHas: a가 든 줄 가운데 b도 든 줄이 있다(셸 line_has).
func s19lineHas(k *K, a, b string) bool {
	for _, l := range s19lines(k.out) {
		if strings.Contains(l, a) && strings.Contains(l, b) {
			return true
		}
	}
	return false
}

func s19gidx(k *K) int64 {
	fi, err := os.Stat(k.git(k.R+".wt/g-old", "rev-parse", "--absolute-git-dir") + "/index")
	if err != nil {
		return 0
	}
	return fi.ModTime().Unix()
}

func gcSections(k *K) {
	s19gc(k)
	s19integ(k, s19old())
	s19branches(k, s19old())
}

func s19gc(k *K) {
	// 19. hc-lane gc
	k.mk("t19")
	k.seed(".gitignore", ".omc/\n.qa/")
	k.seed(".claude/herdr-teardown.sh", `echo "$LANE_BRANCH" >>"$HOME/gc-td"`)
	k.git(k.R, "pull", "-q", "--ff-only")
	s19gw(k, "g-ff")
	s19gcm(k, "g-ff")
	k.git(k.R+".wt/g-ff", "push", "-q", "origin", "HEAD:main") // fast-forward로 들어감
	write(k.t, k.R+".wt/g-ff/.omc/s", "s\n")                   // 무시 폴더는 같이 지운다
	s19gw(k, "g-rb")
	s19gcm(k, "g-rb")
	k.sh(k.D+"/seed", `git pull -q --rebase origin main`) // rebase로 들어감(다른 sha)
	k.seed("z", "z")
	k.sh(k.D+"/seed", `git fetch -q "$1" g-rb && git cherry-pick FETCH_HEAD >/dev/null && git push -q origin HEAD:main`, k.R)
	s19gw(k, "g-un")
	s19gcm(k, "g-un") // 안 들어감
	s19gw(k, "g-dirty")
	appendf(k.t, k.R+".wt/g-dirty/a", "changed\n") // 추적 파일 변경
	s19gw(k, "g-untr")
	write(k.t, k.R+".wt/g-untr/note.txt", "keep\n") // 무시 안 된 untracked
	s19gw(k, "g-qa")
	write(k.t, k.R+".wt/g-qa/.qa/x/REPORT.md", "r\n") // 무시된 보관물
	s19gw(k, "g-live")
	k.git(k.R, "worktree", "add", "-q", "--detach", k.R+".wt/g-det", "origin/main") // 분리 HEAD, 들어감
	k.git(k.R, "fetch", "-q", "origin")
	liveLS := fmt.Sprintf(`{"terminals":[{"address":"t5","agent":"claude-code","cwd":"%s","state":"live"}]}`, k.R+".wt/g-live/sub")
	before := s19gstate(k)
	k.runE([]string{"HC_FAKE_LS=" + liveLS}, "gc", "--repo", k.R, "--dry-run", "--min-age", "0")
	k.check("gc --dry-run: 종료 0, 아무것도 안 바뀜", k.rc == 0 && s19gstate(k) == before && !exists(k.T+"/gc-td"))
	k.check("gc --dry-run: 지울 것·남길 것과 요약", s19lineHas(k, k.R+".wt/g-ff", "지움") && s19lineHas(k, k.R+".wt/g-un", "남김") && strings.Contains(lastLine(k.out), "지움 3 · 남김 5"))
	k.runE([]string{"HC_FAKE_LS=" + liveLS}, "gc", "--repo", k.R, "--min-age", "0")
	k.check("gc: 종료 0, 요약 줄이 마지막", k.rc == 0 && lastLine(k.out) == "지움 3 · 남김 5")
	k.check("gc: fast-forward로 들어간 가지 → 폴더·가지 지움(무시 폴더도)", !s19gwt(k, "g-ff") && !exists(k.R+".wt/g-ff") && !s19gbr(k, "g-ff"))
	k.check("gc: rebase로 들어간 가지(git cherry) → 지움", !s19gwt(k, "g-rb") && !exists(k.R+".wt/g-rb") && !s19gbr(k, "g-rb"))
	k.check("gc: 분리 HEAD가 들어간 폴더 → 지움", !s19gwt(k, "g-det") && !exists(k.R+".wt/g-det"))
	td := read(k.T + "/gc-td")
	k.check("gc: 지울 때 teardown을 돌림", s19hasLine(td, "g-ff") && s19hasLine(td, "g-rb"))
	k.check("gc: 안 들어간 가지 → 남김", s19gwt(k, "g-un") && s19gbr(k, "g-un") && s19lineHas(k, k.R+".wt/g-un", "남김"))
	k.check("gc: 추적 변경·무시 안 된 untracked → 남김", s19gwt(k, "g-dirty") && lastLine(strings.TrimRight(read(k.R+".wt/g-dirty/a"), "\n")) == "changed" &&
		s19gwt(k, "g-untr") && exists(k.R+".wt/g-untr/note.txt") && s19lineHas(k, k.R+".wt/g-dirty", "남김") && s19lineHas(k, k.R+".wt/g-untr", "남김"))
	k.check("gc: 무시된 보관물(.qa) → 남김과 보고", s19gwt(k, "g-qa") && exists(k.R+".wt/g-qa/.qa/x/REPORT.md") && s19lineHas(k, k.R+".wt/g-qa", ".qa"))
	k.check("gc: 살아 있는 터미널 cwd → 남김(주소)", s19gwt(k, "g-live") && s19gbr(k, "g-live") && s19lineHas(k, k.R+".wt/g-live", "t5"))
	k.check("gc: 메인 폴더·기본 가지는 그대로", isDir(k.R+"/.git") && s19gbr(k, "main") && k.git(k.R, "symbolic-ref", "--short", "HEAD") == "main" && !s19hasLine(k.out, "지움 "+k.R))
	s19gw(k, "g-new")
	k.run("gc", "--repo", k.R)
	k.check("gc: 기본 --min-age(24시간)면 방금 만든 폴더는 남김", k.rc == 0 && s19gwt(k, "g-new") && s19lineHas(k, k.R+".wt/g-new", "새것"))
	k.runE([]string{"HC_FAKE_LS_FAIL=1"}, "gc", "--repo", k.R, "--min-age", "0")
	k.check("gc: hc ls 실패면 아무것도 안 지우고 멈춤", k.rc != 0 && strings.Contains(k.errf(), "hc ls") && s19gwt(k, "g-new"))
	// 감사 반영: 병합 커밋이 낀 가지·일부만 들어간 가지·잠긴 폴더·멈춘 rebase는 남김, 묵은 폴더는 status가 index를 다시 써도 새것이 아님
	s19gw(k, "g-merge")
	s19gcm(k, "g-merge")
	// 작업 중 발견해 함께 고침: 셸 원본은 origin/main이 가지 기준 그대로라 merge가 「Already up to date」로 병합 커밋을 못 만들고
	// 뒤 cherry-pick이 비어 실패했다(셸은 실패를 안 봐서 이 단언은 병합 커밋 없는 가지를 봤다). origin/main을 한 칸 올리고 받아 와 진짜 병합 커밋을 만든다
	k.seedp("g-merge-up.txt", "up")
	k.sh(k.R+".wt/g-merge", `git fetch -q origin && git merge -q --no-ff --no-edit origin/main >/dev/null && [ "$(git rev-list --count --merges -1 HEAD)" = 1 ] && git push -q origin HEAD^1:refs/heads/tmp-m`)
	k.sh(k.D+"/seed", `git fetch -q origin && git checkout -q --detach origin/main && git cherry-pick origin/tmp-m >/dev/null && git push -q origin HEAD:main HEAD:refs/heads/x-sink :refs/heads/tmp-m && git checkout -q -B main origin/main`)
	s19gw(k, "g-part")
	s19gcm(k, "g-part")
	k.sh(k.R+".wt/g-part", `echo p2 >p2 && git add -- p2 && git commit -qm p2 -- p2`)
	k.sh(k.D+"/seed", `git fetch -q origin && git fetch -q "$1" g-part && git checkout -q --detach origin/main && git cherry-pick FETCH_HEAD~1 >/dev/null && git push -q origin HEAD:main && git checkout -q -B main origin/main`, k.R)
	s19gw(k, "g-lock")
	k.git(k.R, "worktree", "lock", k.R+".wt/g-lock")
	s19gw(k, "g-reb")
	k.sh(k.R+".wt/g-reb", `GIT_SEQUENCE_EDITOR="sed -i.b 1s/^/break\\n/" git rebase -q -i HEAD~1 >/dev/null 2>&1; true`)
	k.git(k.R, "fetch", "-q", "origin")
	s19gw(k, "g-old")
	k.sh(k.R+".wt/g-old", `g=$(git rev-parse --absolute-git-dir); touch -t 202601010000 . ./* ./.??* "$g"/* "$g"/logs/HEAD 2>/dev/null; true`)
	i0 := s19gidx(k)
	k.run("gc", "--repo", k.R, "--dry-run")
	k.check("gc --dry-run: index를 다시 쓰지 않음, 묵은 폴더는 새것이 아님", s19gidx(k) == i0 && s19lineHas(k, k.R+".wt/g-old", "지움"))
	k.run("gc", "--repo", k.R, "--min-age", "0")
	k.check("gc: 병합 커밋이 낀 가지 → 남김", s19gwt(k, "g-merge") && s19lineHas(k, k.R+".wt/g-merge", "안 들어감"))
	k.check("gc: 커밋 일부만 들어간 가지 → 남김", s19gwt(k, "g-part") && s19lineHas(k, k.R+".wt/g-part", "안 들어감"))
	k.check("gc: 잠긴 작업 폴더 → 남김", s19gwt(k, "g-lock") && s19lineHas(k, k.R+".wt/g-lock", "잠긴"))
	k.check("gc: 멈춘 rebase가 있는 폴더 → 남김", s19gwt(k, "g-reb") && s19lineHas(k, k.R+".wt/g-reb", "진행 중 rebase-merge"))
	k.check("gc: rebase 중인 작업 폴더의 가지(porcelain엔 분리 HEAD)는 가지로도 안 건드림, 종료 0", k.rc == 0 && s19gbr(k, "g-reb") && !strings.Contains(k.out, "가지 g-reb"))
	k.check("gc: 묵은 폴더는 지움", !s19gwt(k, "g-old") && !s19gbr(k, "g-old"))
	k.run("gc", "--repo", k.R, "--branch", "x")
	k.check("gc: --branch·--name은 받지 않음", k.rc != 0)
	k.run("clean", "--repo", k.R, "--branch", "g-un", "--integ")
	k.check("gc 밖에서는 --integ를 받지 않음", k.rc != 0)
}

// s19ic <참조> <파일> [이틀 전]: seed에서 origin/main 위 커밋을 그 참조로 올린다(main에는 안 넣음, R에는 객체 없음).
func s19ic(k *K, ref, file, old string) {
	k.t.Helper()
	k.sh(k.D+"/seed", `git fetch -q origin && git checkout -q --detach origin/main && echo "$2" >"$2" && git add -- "$2" &&
    if [ -n "$3" ]; then GIT_COMMITTER_DATE=$3 git commit -qm "$2" -- "$2"; else git commit -qm "$2" -- "$2"; fi &&
    git push -q origin "HEAD:$1"`, ref, file, old)
}

func s19integ(k *K, old string) {
	// 19b. --integ (19절 저장소 t19를 이어 쓴다)
	s19ic(k, "refs/integ/i-old-in", "i1", old)
	k.sh(k.D+"/seed", `git push -q origin HEAD:main`) // 들어감(조상) + 오래됨
	s19ic(k, "refs/integ/i-rb", "i2", old)
	s2 := k.git(k.D+"/seed", "rev-parse", "HEAD")
	k.sh(k.D+"/seed", `git checkout -q --detach origin/main && git pull -q --rebase origin main`) // rebase로 들어감 + 오래됨
	k.seed("z2", "z2")
	k.sh(k.D+"/seed", `git cherry-pick "$1" >/dev/null && git push -q origin HEAD:main`, s2)
	s19ic(k, "refs/integ/i-new", "i3", "")
	k.sh(k.D+"/seed", `git push -q origin HEAD:main`) // 들어감 + 새것
	s19ic(k, "refs/integ/i-out", "i4", old)           // 안 들어감 + 오래됨
	s19ic(k, "refs/claims/i-claim", "i5", old)
	k.sh(k.D+"/seed", `git push -q origin HEAD:refs/heads/refs/integ/i-trap HEAD:refs/tags/v1`) // 건드리면 안 되는 것
	k.sh(k.D+"/seed", `git checkout -q -B main origin/main`)
	before := k.git(k.D+"/origin.git", "for-each-ref")
	k.run("gc", "--repo", k.R, "--integ", "--dry-run", "--min-age", "0")
	k.check("gc --integ --dry-run: 원격 참조 그대로", k.rc == 0 && k.git(k.D+"/origin.git", "for-each-ref") == before)
	k.run("gc", "--repo", k.R, "--integ")
	k.check("gc --integ: 종료 0", k.rc == 0)
	k.check("gc --integ: 들어감+오래됨 → 지우고 참조·sha를 찍음", !s19oref(k, "refs/integ/i-old-in") && grep(k.out, `^지움 refs/integ/i-old-in [0-9a-f]{40}$`))
	k.check("gc --integ: rebase로 들어감+오래됨 → 지움", !s19oref(k, "refs/integ/i-rb"))
	k.check("gc --integ: 들어감+새것 → 남김", s19oref(k, "refs/integ/i-new") && s19lineHas(k, "refs/integ/i-new", "남김"))
	k.check("gc --integ: 안 들어감 → 남김", s19oref(k, "refs/integ/i-out") && s19lineHas(k, "refs/integ/i-out", "남김"))
	k.check("gc --integ: refs/claims·태그·기본 가지·이름 끝만 같은 가지는 안 건드림", s19oref(k, "refs/claims/i-claim") && s19oref(k, "refs/tags/v1") &&
		s19oref(k, "refs/heads/main") && s19oref(k, "refs/heads/refs/integ/i-trap"))
	k.run("gc", "--repo", k.R, "--integ=refs/heads/")
	k.check("gc --integ: 접두가 refs/heads·tags·claims면 거절", k.rc != 0 && s19oref(k, "refs/heads/main"))
}

// s19bo <가지> [old]: origin/main 위 커밋 하나인 가지(작업 폴더 없음). old면 커밋·reflog 시각을 이틀 전으로.
func s19bo(k *K, br, old string) {
	k.t.Helper()
	k.sh(k.R, `if [ -n "$2" ]; then export GIT_COMMITTER_DATE=$2; fi
   git fetch -q origin && git checkout -q -b "$1" origin/main && echo "$1" >"$1.txt" && git add -- "$1.txt" &&
   git commit -qm "$1" -- "$1.txt" && git checkout -q main`, br, old)
}

func s19bp(k *K, br string) { k.sh(k.R, `git push -q origin "$1:main"`, br) } // fast-forward로 들어감

func s19branches(k *K, old string) {
	// 19c. 작업 폴더 없이 남은 가지
	k.mk("t19c")
	k.git(k.R, "pull", "-q", "--ff-only")
	s19bo(k, "b-old", old)
	s19bp(k, "b-old")
	for _, b := range []string{"b-ff", "b-new", "b-keep", "b-head"} {
		s19bo(k, b, "")
		s19bp(k, b)
	}
	s19bo(k, "b-rb", "")
	k.sh(k.D+"/seed", `git pull -q --rebase origin main`) // rebase로 들어감
	k.seed("z3", "z3")
	k.sh(k.D+"/seed", `git fetch -q "$1" b-rb && git cherry-pick FETCH_HEAD >/dev/null && git push -q origin HEAD:main`, k.R)
	s19bo(k, "b-un", "")
	s19bo(k, "b-merge", "")
	k.seed("z4", "z4")
	k.sh(k.R, `git fetch -q origin && git checkout -q b-merge && git merge -q --no-ff --no-edit origin/main >/dev/null && git checkout -q main`)
	k.sh(k.D+"/seed", `git fetch -q "$1" b-merge && git cherry-pick FETCH_HEAD^1 >/dev/null && git push -q origin HEAD:main`, k.R) // 내용은 들어갔지만 병합 커밋이 낌
	k.sh(k.D+"/seed", `git fetch -q origin && git push -q origin origin/main:refs/heads/r-in origin/main:refs/heads/r-keep`)
	k.git(k.R, "push", "-q", "origin", "b-un:refs/heads/r-out")
	k.git(k.R, "fetch", "-q", "origin")
	k.git(k.R, "checkout", "-q", "b-head") // 지금 HEAD 가지(들어감)
	k.conf("gc.keep_branches b-keep r-k*")
	before := s19gstate(k)
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "0", "--remote-branches")
	k.check("gc 가지 --dry-run: 종료 0, 아무것도 안 바뀜", k.rc == 0 && s19gstate(k) == before)
	k.check("gc 가지 --dry-run: 지울 가지·원격 가지를 찍음", s19lineHas(k, "가지 b-ff", "지움") && s19lineHas(k, "가지 b-old", "지움") && s19lineHas(k, "refs/heads/r-in", "지움"))
	k.run("gc", "--repo", k.R)
	k.check("gc 가지: 기본 --min-age면 오래된 들어간 가지만 지우고 sha를 찍음", k.rc == 0 && !s19gbr(k, "b-old") && grep(k.out, `^지움 가지 b-old [0-9a-f]{40}$`))
	k.check("gc 가지: 새것(--min-age 안) → 남김", s19gbr(k, "b-new") && s19gbr(k, "b-ff") && s19lineHas(k, "가지 b-new", "새것"))
	k.check("gc 가지: --remote-branches 없으면 원격 가지 그대로", s19oref(k, "refs/heads/r-in") && s19oref(k, "refs/heads/r-out") && !strings.Contains(k.out, "refs/heads/r-"))
	k.run("gc", "--repo", k.R, "--min-age", "0")
	k.check("gc 가지: 종료 0", k.rc == 0)
	k.check("gc 가지: fast-forward·rebase로 들어간 가지 → 지움", !s19gbr(k, "b-ff") && !s19gbr(k, "b-rb") && !s19gbr(k, "b-new"))
	k.check("gc 가지: 안 들어감 → 남김(개수·마지막 날짜)", s19gbr(k, "b-un") && grep(k.out, `^남김 가지 b-un 안 들어감 1개 · 마지막 [0-9]{4}-[0-9]{2}-[0-9]{2}$`))
	k.check("gc 가지: 병합 커밋이 낀 가지 → 남김", s19gbr(k, "b-merge") && s19lineHas(k, "가지 b-merge", "안 들어감"))
	k.check("gc 가지: 남길 목록 → 남김", s19gbr(k, "b-keep") && s19lineHas(k, "가지 b-keep", "남길"))
	k.check("gc 가지: 기본·현재 HEAD 가지 → 그대로", s19gbr(k, "main") && s19gbr(k, "b-head") && k.git(k.R, "symbolic-ref", "--short", "HEAD") == "b-head")
	k.run("gc", "--repo", k.R, "--min-age", "0", "--remote-branches")
	k.check("gc --remote-branches: 들어간 원격 가지 → 지우고 sha를 찍음", k.rc == 0 && !s19oref(k, "refs/heads/r-in") && grep(k.out, `^지움 refs/heads/r-in [0-9a-f]{40}$`))
	k.check("gc --remote-branches: 안 들어감 → 남김(개수)", s19oref(k, "refs/heads/r-out") && s19lineHas(k, "refs/heads/r-out", "안 들어감 1개"))
	k.check("gc --remote-branches: 남길 목록(무늬)·기본 가지 → 그대로", s19oref(k, "refs/heads/r-keep") && s19oref(k, "refs/heads/main"))
	k.run("clean", "--repo", k.R, "--branch", "b-un", "--remote-branches")
	k.check("gc 밖에서는 --remote-branches를 받지 않음", k.rc != 0)
	// 감사 반영: dry-run은 로컬에 없는 원격 가지 커밋을 받아도 원격 추적 참조(origin/main 포함)를 바꾸지 않는다
	k.sh(k.D+"/seed", `git fetch -q origin && git checkout -q --detach origin/main && echo m1 >m1 && git add -- m1 && git commit -qm m1 -- m1 &&
  git push -q origin HEAD:main HEAD:refs/heads/r-m1 && git checkout -q -B main origin/main`)
	before = s19gstate(k)
	k.run("gc", "--repo", k.R, "--dry-run", "--min-age", "0", "--remote-branches")
	k.check("gc --remote-branches --dry-run: 커밋만 받고 참조는 그대로(옛 origin/main으로 판정)", k.rc == 0 && s19gstate(k) == before && s19lineHas(k, "refs/heads/r-m1", "남김"))
	// 감사 반영: patch-id는 공백을 무시한다 — 공백만 다른 내용이 main에 들어간 가지는 안 들어감(지우지 않음)
	k.git(k.R, "checkout", "-q", "main")
	k.sh(k.R, `git fetch -q origin && git checkout -q -b b-ws origin/main && echo 'z = 3' >w.txt && git add -- w.txt && git commit -qm ws -- w.txt && git checkout -q main`)
	k.sh(k.D+"/seed", `git fetch -q origin && git checkout -q --detach origin/main && echo '    z = 3' >w.txt && git add -- w.txt && git commit -qm ws -- w.txt &&
  git push -q origin HEAD:main && git checkout -q -B main origin/main`)
	k.run("gc", "--repo", k.R, "--min-age", "0")
	k.check("gc 가지: 공백만 다른 내용이 들어간 가지 → 남김(안 들어감)", k.rc == 0 && s19gbr(k, "b-ws") && s19lineHas(k, "가지 b-ws", "안 들어감 1개"))
	k.noconf()
}
