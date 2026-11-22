package hclane_test

// tests/hc-lane-stale-land.test.sh를 옮긴 것: hc-lane land가 가지의 .claude/herdr-land.sh가 origin/<기본>보다 낡았으면 먼저 rebase해
// 새 판으로 착지하는지. 셸 시험처럼 자기 머리(가짜 hc는 ls만, isolate.sh 격리)를 따로 둔다 — hc-lane.test.sh의 newK를 쓰지 않는다.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 착지 스크립트 판: 이름을 찍고 rebase·push(이름이 「old」면 push 없이 0 — 옛 판이 무언가를 빠뜨린 꼴)
const (
	stlLandNew = `git fetch -q origin && git rebase -q "origin/$1" && HERDR_LAND=1 git push -q origin "HEAD:$1" && echo land-new`
	stlLandOld = `echo land-old`
)

// isolate.sh(lib/hc-lane/test-isolate.sh)와 이 셸 시험 머리가 지우는 바깥 값.
var stlDropEnv = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_PREFIX",
	"XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
	"HC_LANE", "HC_LANE_OWNER", "HC_LANE_OWNER_RUN", "HC_LANE_LEASE", "HERDR_LAND_PRIORITY", "HC_LANE_PRIORITY", "HC_LANE_MERGER_BIN",
	"HC_TERMINAL_ID", "HC_LANE_COORD", "HC_LANE_FILL"}

func TestStaleLand(t *testing.T) {
	k := stlNewK(t)
	stlRun(k)
	t.Logf("%d개 중 실패 %d개", k.n, k.fails)
}

func stlNewK(t *testing.T) *K {
	T, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	k := &K{t: t, T: T, env: map[string]string{}}
	k.LANE = laneBin(t)
	for _, kv := range os.Environ() {
		key, v, _ := strings.Cut(kv, "=")
		k.env[key] = v
	}
	for _, key := range stlDropEnv {
		delete(k.env, key)
	}
	bin := filepath.Join(T, "bin")
	write(t, filepath.Join(bin, "hc"), "#!/bin/bash\n[ \"$1\" = ls ] && echo '{\"terminals\":[]}'\nexit 0\n")
	if err := os.Chmod(filepath.Join(bin, "hc"), 0o755); err != nil {
		t.Fatal(err)
	}
	k.set("PATH="+bin+":"+k.env["PATH"], "HOME="+T, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CEILING_DIRECTORIES="+T+":"+filepath.Dir(T))
	return k
}

// stlMk <이름>: origin(bare) + 시드 클론. k.D를 정한다.
func (k *K) stlMk(name string) {
	k.t.Helper()
	k.D = k.T + "/" + name
	k.sh(k.T, `git init -q --bare -b main "$1/o.git" && { git clone -q "$1/o.git" "$1/s" 2>/dev/null; true; }`, k.D)
}

// stlSd <파일> <내용>: 시드 클론을 당겨 커밋하고 origin/main으로 push.
func (k *K) stlSd(file, content string) {
	k.t.Helper()
	k.sh(k.D+"/s", `git pull -q --rebase origin main 2>/dev/null; mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1" && git push -q origin HEAD:main`, file, content)
}

// stlLane [내용] [파일]: 레인 클론(가지 lane, 커밋 하나).
func (k *K) stlLane(a ...string) {
	k.t.Helper()
	content, file := "x", "x.txt"
	if len(a) > 0 {
		content = a[0]
	}
	if len(a) > 1 {
		file = a[1]
	}
	k.sh(k.T, `git clone -q "$1/o.git" "$1/l" && cd "$1/l" && git checkout -qb lane && printf '%s\n' "$2" >"$3" && git add -A && git commit -qm lane`, k.D, content, file)
}

func (k *K) stlLnd(extra ...string) { k.runIn(k.D+"/l", extra, "land", "main") }

func (k *K) stlOtree() []string {
	return strings.Split(k.git(k.D+"/o.git", "log", "--format=%s", "main"), "\n")
}

func stlRun(k *K) {
	outHas := func(s string) bool { return strings.Contains(k.out, s) }
	outLine := func(s string) bool { return contains(strings.Split(k.out, "\n"), s) }
	errHas := func(s string) bool { return strings.Contains(k.errf(), s) }

	// 1. 낡은 판: 가지는 옛 판, origin에 새 판 → 먼저 rebase하고 새 판으로 착지
	k.stlMk("a")
	k.stlSd(".claude/herdr-land.sh", stlLandOld)
	k.stlLane()
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLnd()
	k.check("낡은 가지: 새 판 스크립트로 착지(옛 판을 돌리지 않음)", k.rc == 0 && outLine("land-new") && !outHas("land-old") && contains(k.stlOtree(), "lane"))
	k.check("낡은 가지: 먼저 rebase한다고 한 줄 찍음", errHas("가 origin/main 보다 낡음 → 먼저 rebase"))

	// 2. 가지가 스크립트를 스스로 바꿈 → 그 판 그대로(먼저 rebase 없음)
	k.stlMk("b")
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLane("echo land-lane", ".claude/herdr-land.sh")
	k.stlSd("y", "y")
	k.stlLnd()
	k.check("스크립트를 바꾼 가지: 가지 판을 돌림", k.rc == 0 && outLine("land-lane") && !errHas("먼저 rebase") && !contains(k.stlOtree(), "lane"))

	// 3. 스크립트는 그대로, origin만 앞섬 → 먼저 rebase 없이 가지 판(=origin 판)을 돌림
	k.stlMk("c")
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLane()
	k.stlSd("y", "y")
	k.stlLnd()
	k.check("스크립트가 같으면: 먼저 rebase 없이 착지", k.rc == 0 && outLine("land-new") && !errHas("먼저 rebase") && contains(k.stlOtree(), "lane"))

	// 4. 가지에 없던 스크립트를 origin이 새로 둠 → 먼저 rebase해 그 스크립트로
	k.stlMk("d")
	k.stlSd(".claude/herdr-verify.sh", "echo verify-ran")
	k.stlLane()
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLnd()
	k.check("가지에 없던 스크립트: 먼저 rebase해 새 스크립트로 착지", k.rc == 0 && outLine("land-new") && !outHas("verify-ran") && contains(k.stlOtree(), "lane"))

	// 5. 먼저 rebase가 충돌 → 종료 3, rebase는 멈춘 채(버리지 않음), 옛 판은 돌지 않음
	k.stlMk("e")
	k.stlSd(".claude/herdr-land.sh", stlLandOld)
	k.stlSd("x.txt", "base")
	k.stlLane("mine")
	k.stlSd("x.txt", "theirs")
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLnd()
	k.check("먼저 rebase 충돌: 종료 3, rebase 멈춘 채, 안내 줄", k.rc == 3 && k.gitOK(k.D+"/l", "rev-parse", "-q", "--verify", "REBASE_HEAD") &&
		errHas("git rebase --continue 한 뒤 착지 줄(hc-lane land main)을 다시") && !outHas("land-old"))

	// 6. 커밋 안 된 변경이 있으면 먼저 rebase하지 않고 멈춤(옛 판도 안 돌림)
	k.stlMk("f")
	k.stlSd(".claude/herdr-land.sh", stlLandOld)
	k.stlLane()
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	appendf(k.t, k.D+"/l/x.txt", "dirty\n")
	k.stlLnd()
	k.check("커밋 안 된 변경: 멈춤, 옛 판 안 돌림", k.rc == 1 && errHas("커밋 안 된 변경") && !outHas("land-old"))

	// 7. 병합기 안 착지(HERDR_MERGER=1): 기본 브랜치에 스크립트가 있으면 그 기본 브랜치 판에 맡긴다 — 가지가 고친 판(land-lane)이 아니라
	//    (2단계 2026-11-20 지휘 t1cj. 감사 7c010d9 중간 1의 「먼저 rebase가 들인 스크립트」도 이 길 — 기본 브랜치 판이다)
	k.stlMk("g")
	k.stlSd(".claude/herdr-verify.sh", "echo verify-ran")
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLane("echo land-lane", ".claude/herdr-land.sh")
	k.stlLnd("HERDR_MERGER=1")
	k.check("병합기 안 착지: 가지가 고친 판이 아니라 기본 브랜치 판 스크립트로 착지", k.rc == 0 && outLine("land-new") && !outHas("land-lane") && contains(k.stlOtree(), "lane"))
	// 8. 병합기 안 착지인데 기본 브랜치에 없는 스크립트가 작업 트리에 있으면(가지가 새로 들임) 맡기지 않고 종료 7
	k.stlMk("h")
	k.stlSd(".claude/herdr-verify.sh", "echo verify-ran")
	k.stlLane("echo land-lane", ".claude/herdr-land.sh")
	k.stlLnd("HERDR_MERGER=1")
	k.check("병합기 안 착지: 기본 브랜치에 없는 스크립트는 안 돌리고 종료 7, main 그대로", k.rc == 7 && !outHas("land-lane") && !contains(k.stlOtree(), "lane"))

	// 9. 먼저 rebase 충돌을 rerere가 기록한 풀이로 다 풀면 이어 가 새 판으로 착지(land-stale-lows 감사 낮음 1 — 이 자리의 rerere 길)
	k.stlMk("i")
	k.stlSd(".claude/herdr-land.sh", stlLandOld)
	k.stlSd("x.txt", "base")
	k.stlLane("mine")
	k.git(k.D+"/l", "config", "rerere.enabled", "true")
	k.git(k.D+"/l", "config", "rerere.autoUpdate", "true")
	l0 := k.git(k.D+"/l", "rev-parse", "HEAD")
	k.stlSd("x.txt", "theirs")
	k.stlSd(".claude/herdr-land.sh", stlLandNew)
	k.stlLnd()
	k.exe(k.D+"/l", "", []string{"GIT_EDITOR=true"}, "bash", "-c", `printf 'mine\ntheirs\n' >x.txt && git add -- x.txt && git rebase --continue >/dev/null 2>&1`)
	k.git(k.D+"/l", "reset", "-q", "--hard", l0)
	k.stlLnd()
	k.check("먼저 rebase 충돌을 rerere가 다 풀면: 이어 가 새 판으로 착지", k.rc == 0 && outLine("land-new") && errHas("rerere가 기록한 풀이") &&
		strings.Join(strings.Split(k.git(k.D+"/o.git", "show", "main:x.txt"), "\n"), ",") == "mine,theirs")

	// 10. 먼저 rebase 출력도 착지 로그에 남는다(감사 낮음 3 — llog_init이 먼저 rebase 뒤였다)
	k.check("먼저 rebase 안내 줄이 land-logs 로그에도", stlTreeHas(k.T+"/.local/state/hc-lane/land-logs", "낡음 → 먼저 rebase"))

	// 11. origin 없는 저장소: 로컬 <기본>이 기준, 안내 줄은 「로컬 main」(감사 낮음 2 — refs/heads/main 꼴로 찍혔다)
	k.D = k.T + "/k"
	k.sh(k.T, `git init -q -b main "$1/r" && cd "$1/r" && mkdir .claude && echo 'echo land-old' >.claude/herdr-land.sh && git add -- .claude && git commit -qm seed &&
  git worktree add -q -b lane "$1/l" && echo 'echo land-new' >.claude/herdr-land.sh && git commit -qam new`, k.D)
	k.sh(k.D+"/l", `echo x >x.txt && git add -- x.txt && git commit -qm lane`)
	k.stlLnd()
	k.check("origin 없는 저장소: 「로컬 main 보다 낡음」, 새 판으로", k.rc == 0 && outLine("land-new") && errHas("가 로컬 main 보다 낡음 → 먼저 rebase") && !errHas("refs/heads"))
}

// stlTreeHas: 폴더 아래 어느 파일에든 그 글이 있다(grep -rlFq).
func stlTreeHas(dir, s string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(read(p), s) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
