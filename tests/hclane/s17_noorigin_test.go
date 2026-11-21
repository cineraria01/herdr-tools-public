package hclane_test

// tests/hc-lane.test.sh 17·18절: 옛 이름(wt-*)은 찾지도 부르지도 않음, 원격(origin)이 없는 저장소의 띄우기·land·clean·guard.

import (
	"os"
	"os/exec"
	"strings"
)

func init() { reg(60, "s17-18", s17Run) }

func s17Run(k *K) {
	s17Old(k)
	s17NoOrigin(k)
}

func s17Old(k *K) {
	// 17. 옛 이름은 찾지도 부르지도 않는다
	k.mk("t17")
	k.seed(".gitignore", ".omc/")
	k.seed(".claude/wt-teardown.sh", `echo old >"$HOME/old-td"`)
	k.land("c-old")
	k.run("clean", "--repo", k.R, "--branch", "c-old")
	k.check("clean: 옛 이름 wt-teardown.sh는 부르지 않음", k.rc == 0 && k.gone("c-old") && !exists(k.T+"/old-td") && k.last(`"teardown":"none"`))
	k.check("hc-lane 본문에 옛 이름 갈래(.claude/wt-*·WT_*·wt-land/verify/setup/teardown) 없음", !grep(read(k.LANE), `\.claude/wt-|WT_[A-Z]|wt-(land|verify|setup|teardown)`))
}

// s17lc: 메인 폴더에 커밋(셸 lc). s17ltop: 로컬 그 가지 끝 커밋 제목(셸 ltop).
func (k *K) s17lc(file, content string) {
	k.t.Helper()
	k.sh(k.R, `mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1" -- "$1"`, file, content)
}

func (k *K) s17ltop(br ...string) string {
	b := "main"
	if len(br) > 0 {
		b = br[0]
	}
	return k.git(k.R, "log", "-1", "--format=%s", b)
}

func s17NoOrigin(k *K) {
	// 18. 원격(origin)이 없는 저장소(맥의 game·hwanhon): 로컬 <기본>이 기준, 착지는 메인 클론의 로컬 <기본>을 fast-forward
	k.D, k.B = k.W+"/t18", "main"
	os.Remove(k.HCLOG)
	os.Remove(k.HCLOG + ".exec")
	k.R = k.D + "/로컬 저장소"
	k.sh(k.T, `git init -q -b main "$1"`, k.R)
	k.s17lc(".gitignore", ".omc/")
	k.s17lc(".claude/herdr-setup.sh", `echo "$LANE_REPO|$LANE_BASE" > setup-ran`)
	k.run("--name", "o-a", "--repo", k.R, "--", "true")
	wt := k.R + ".wt/o-a"
	k.check("원격 없음: 레인 만들기 종료 0, 로컬 main 기준, 준비 훅 돎", k.rc == 0 && k.git(wt, "rev-parse", "HEAD") == k.git(k.R, "rev-parse", "main") &&
		strings.TrimRight(read(wt+"/setup-ran"), "\n") == k.R+"|main" && k.has("LANE_BASE=main"))
	k.check("원격 없음: 임대 없이 띄움(HC_LANE_LEASE 빈 값 — origin 없음이 확인된 저장소만)", k.rc == 0 && k.has("HC_LANE_LEASE="))
	k.check("원격 없음: 착지 줄은 hc-lane land main, 기준 로컬 main", strings.Contains(k.out, "&& hc-lane land main") && strings.Contains(k.out, "기준 로컬 main"))
	os.Remove(wt + "/setup-ran")
	k.sh(wt, `echo o-a >o-a.txt && git add -- o-a.txt && git commit -qm o-a -- o-a.txt`)
	k.s17lc("m1", "m1")
	k.lnd(wt)
	k.check("원격 없음 land: herdr-verify.sh가 없으면 실패, 로컬 main 그대로", k.rc == 1 && k.s17ltop() == "m1" && strings.Contains(k.errf(), "herdr-verify.sh"))
	k.s17lc(".claude/herdr-verify.sh", `echo "verify $(git log -1 --format=%s)" >>"$HOME/lverify.log"; n=$(cat "$HOME/lrace" 2>/dev/null || echo 0); echo $((n + 1)) >"$HOME/lrace"; [ "$n" -lt "${RACE_N:-0}" ] || exit 0; cd "$RACE_MAIN" && echo "$n" >"lrace$n" && git add "lrace$n" && git commit -qm "lrace$n"`)
	k.lnd(wt)
	k.check("원격 없음 land: 로컬 main 위로 rebase·검증 뒤 메인 클론의 main을 fast-forward(작업 트리도)", k.rc == 0 && k.s17ltop() == "o-a" &&
		lastLine(k.git(k.R, "log", "-2", "--format=%s", "main")) == ".claude/herdr-verify.sh" && exists(k.R+"/o-a.txt") &&
		k.git(k.R, "status", "--porcelain") == "" && contains(strings.Split(read(k.T+"/lverify.log"), "\n"), "verify o-a"))
	k.run("clean", "--repo", k.R, "--branch", "o-a")
	k.check("원격 없음 clean: fetch 없이 로컬 main 기준으로 정리", k.rc == 0 && k.gone("o-a"))
	k.run("--name", "o-b", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/o-b"
	k.sh(wt, `echo o-b >o-b.txt && git add -- o-b.txt && git commit -qm o-b -- o-b.txt`)
	k.run("clean", "--repo", k.R, "--branch", "o-b")
	k.check("원격 없음 clean: 착지 전 브랜치는 멈춤(로컬 main 안내)", k.rc != 0 && strings.Contains(k.errf(), "로컬 main") && k.kept("o-b"))
	k.lndE([]string{"RACE_MAIN=" + k.R, "RACE_N=2"}, wt) // 앞 착지에서 검증이 한 번 돌아 셈은 1부터
	k.check("원격 없음 land: 검증 사이 로컬 main이 앞서 나가면 처음부터 다시 해 착지", k.rc == 0 && k.s17ltop() == "o-b" &&
		lastLine(k.git(k.R, "log", "-2", "--format=%s", "main")) == "lrace1" && strings.Contains(k.errf(), "거절됨"))
	k.run("--name", "o-c", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/o-c"
	k.sh(wt, `echo lane >o-b.txt && git commit -qm o-c -- o-b.txt`)
	write(k.t, k.R+"/o-b.txt", "dirty\n")
	k.lnd(wt)
	k.check("원격 없음 land: 메인 폴더에 겹치는 커밋 안 된 변경이 있으면 멈추고 main 그대로", k.rc == 1 && k.s17ltop() == "o-b" &&
		strings.TrimRight(read(k.R+"/o-b.txt"), "\n") == "dirty" && strings.Contains(k.errf(), "fast-forward로 올리지 못했다"))
	k.git(k.R, "checkout", "-q", "--", "o-b.txt")
	k.git(k.R, "checkout", "-q", "-b", "side")
	k.lnd(wt)
	k.check("원격 없음 land: main이 아무 데도 체크아웃 안 됐으면 update-ref로 올림(메인 폴더 브랜치는 그대로)", k.rc == 0 && k.s17ltop() == "o-c" &&
		k.git(k.R, "symbolic-ref", "--short", "HEAD") == "side" && k.s17ltop("side") == "o-b")
	k.git(k.R, "checkout", "-q", "main")
	// git worktree list가 실패하면 빈 목록으로 읽지 않는다 — 체크아웃된 main에 update-ref하면 메인 폴더 작업 트리·색인이 뒤처진다
	rg, err := exec.LookPath("git")
	if err != nil {
		k.t.Fatal(err)
	}
	write(k.t, k.T+"/go-wl/git", "#!/bin/sh\n[ \"$1 $2\" = \"worktree list\" ] && exit 128\nexec \""+rg+"\" \"$@\"\n")
	if err := os.Chmod(k.T+"/go-wl/git", 0o755); err != nil {
		k.t.Fatal(err)
	}
	k.run("--name", "o-w", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/o-w"
	k.sh(wt, `echo o-w >o-w.txt && git add -- o-w.txt && git commit -qm o-w -- o-w.txt`)
	lm0 := k.git(k.R, "rev-parse", "main")
	k.lndE([]string{"PATH=" + k.T + "/go-wl:" + k.env["PATH"]}, wt)
	k.check("원격 없음 land: git worktree list가 실패하면 멈춤 — 로컬 main 그대로", k.rc != 0 && strings.Contains(k.errf(), "worktree list") && k.git(k.R, "rev-parse", "main") == lm0)
	// 저장소 설정 branch.main.mergeOptions=--squash면 merge --ff-only가 main을 안 움직이고 색인에만 올린 채 0 — 착지가 된 줄 알았다(아스트라 최종 2차 높음 1의 안전 몫)
	k.run("--name", "o-s", "--repo", k.R, "--", "true")
	wt = k.R + ".wt/o-s"
	k.sh(wt, `echo o-s >o-s.txt && git add -- o-s.txt && git commit -qm o-s -- o-s.txt`)
	k.git(k.R, "config", "branch.main.mergeOptions", "--squash")
	k.lnd(wt)
	k.git(k.R, "config", "--unset", "branch.main.mergeOptions")
	k.check("원격 없음 land: squash 설정이 있어도 로컬 main을 후보로 fast-forward — 메인 폴더 깨끗", k.rc == 0 && k.s17ltop() == "o-s" &&
		k.git(k.R, "status", "--porcelain", "--untracked-files=no") == "")
	k.git(k.R, "branch", "-q", "-m", "main", "master")
	k.run("--name", "o-m", "--repo", k.R, "--", "true")
	k.check("원격 없음: main이 없고 master가 있으면 master 기준", k.rc == 0 && k.has("LANE_BASE=master") && strings.Contains(k.out, "hc-lane land master"))
	k.git(k.R, "branch", "-q", "-m", "master", "main")
	k.run("guard", "install", "--repo", k.R)
	k.check("원격 없음 guard install: 설치할 것 없음(종료 0), 훅을 만들지 않음", k.rc == 0 && strings.Contains(k.out, "설치할 것 없음") && !exists(k.R+"/.git/hooks/pre-push"))
	k.run("guard", "status", "--repo", k.R)
	k.check("원격 없음 guard status: 설치할 것 없음(종료 0)", k.rc == 0 && strings.Contains(k.out, "설치할 것 없음"))
}
