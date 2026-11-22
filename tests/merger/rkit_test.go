package merger_test

// 복구 시험(옛 tests/hc-lane-merger-recovery.test.sh) 도우미: 사례마다 fixture로 bare origin·작성 클론(writer, 가지 lane)·병합기 클론을
// 새로 만든다. 셸 판의 `set -e` 사례 함수 하나가 Go 하위 시험 하나이고, 사례 안의 test 줄 하나가 단언 하나다.

import (
	"path/filepath"
	"strings"
	"testing"
)

// F: fixture 하나(셸 fixture의 HOME·origin·writer·merger). HOME·TMPDIR는 그 폴더
type F struct {
	*K
	home, origin, writer, mclone string // mclone: 병합기 전용 클론
}

// newRK: 복구 시험 바깥 환경 — LC_ALL=C·전역 git 설정 없음·가짜 hc(ls → 빈 목록, 그 밖은 종료 3)
func newRK(t *testing.T) *K {
	t.Helper()
	k := newK(t)
	k.set("LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.org",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.org")
	k.stub(filepath.Join(k.T, "bin/hc"), "#!/bin/sh\ncase $1 in ls) echo '{\"terminals\":[]}';; *) exit 3;; esac\n")
	return k
}

// fixture <이름>: T/<이름>을 HOME으로, origin·writer(가지 lane, 검증 훅 exit 0·shared=base)·merger 클론
func (k *K) fixture(name string) *F {
	k.t.Helper()
	h := filepath.Join(k.T, name)
	mkdirs(k.t, h)
	k.set("HOME="+h, "TMPDIR="+h)
	f := &F{K: k, home: h, origin: h + "/origin.git", writer: h + "/writer", mclone: h + "/merger"}
	k.sh(h, `git init -q --bare -b main "$1" && git init -q -b main "$2" && cd "$2" && mkdir .claude && echo 'exit 0' >.claude/herdr-verify.sh &&
echo base >shared && git add -- .claude shared && git commit -qm base && git remote add origin "$1" && git push -q origin main &&
git clone -q "$1" "$3" && git checkout -qb lane`, f.origin, f.writer, f.mclone)
	return f
}

// commitFile <파일> <내용>: writer에서 커밋(셸 commit_file)
func (f *F) commitFile(file, content string) {
	f.t.Helper()
	f.sh(f.writer, `mkdir -p "$(dirname "$1")" && printf '%s\n' "$2" >"$1" && git add -- "$1" && git commit -qm "$1"`, file, content)
}

// w: writer에서 셸 한 줄(준비 — 실패면 멈춤)
func (f *F) w(script string, args ...string) string {
	f.t.Helper()
	return f.sh(f.writer, script, args...)
}

// g: writer에서 git … 의 출력
func (f *F) g(args ...string) string { return f.git(f.writer, args...) }

// og: git --git-dir=origin … 의 출력. ogOK: 0으로 끝났는지
func (f *F) og(args ...string) string {
	return f.exe(f.home, nil, 0, "git", append([]string{"--git-dir=" + f.origin}, args...)...).Out
}

func (f *F) ogOK(args ...string) bool {
	return f.exe(f.home, nil, 0, "git", append([]string{"--git-dir=" + f.origin}, args...)...).RC == 0
}

// enable: 켬 표식 refs/herdr/merger/box
func (f *F) enable() { f.t.Helper(); f.og("update-ref", "refs/herdr/merger/box", "main") }

// submit <레인> [VAR=값…]: writer에서 HC_LANE=<레인> hc-lane land main
func (f *F) submit(lane string, extra ...string) R {
	return f.lane(f.writer, append([]string{"HC_LANE=" + lane}, extra...), "land", "main")
}

// tick [VAR=값…]: 병합기 한 바퀴(merger --repo <병합기 클론> --once main)
func (f *F) tick(extra ...string) R {
	return f.merger(f.writer, extra, 0, "--repo", f.mclone, "--once", "main")
}

// stubPath: PATH 앞에 $HOME/stub을 둔 VAR=값(셸 PATH="$HOME/stub:$PATH")
func (f *F) stubPath() string { return "PATH=" + f.home + "/stub:" + f.env["PATH"] }

// events: $HOME/.local/state/hc-lane/events.tsv 줄들. journal: merger-journal.tsv
func (f *F) events() [][]string { return rows(f.home + "/.local/state/hc-lane/events.tsv") }

func (f *F) journal() [][]string { return rows(f.home + "/.local/state/hc-lane/merger-journal.tsv") }

// nev: 사건 가운데 레인($4)·상태($5)가 맞는 줄 수
func (f *F) nev(lane, state string) int {
	n := 0
	for _, r := range f.events() {
		if col(r, 4) == lane && col(r, 5) == state {
			n++
		}
	}
	return n
}

// refs: for-each-ref 결과가 비었는지 보기 쉽게(공백 줄 없이)
func (f *F) refs(prefix ...string) string {
	return strings.TrimSpace(f.og(append([]string{"for-each-ref"}, prefix...)...))
}
