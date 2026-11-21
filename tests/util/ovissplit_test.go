package util_test

// 옛 tests/ovis-split.test.sh: --self-test, 가짜 hc(PATH 앞)로 --main(라벨 없는 지휘·이미 붙은 지휘)·--add·--drop 한 바퀴씩 JSON 출력과 hc label 인자.
// 실제 hc·Ovis는 쓰지 않는다.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const fakeOvisHc = `#!/bin/bash
printf '%s\n' "$*" >>"$FAKE_HC_LOG"
[ -z "${FAKE_HC_FAIL-}" ] || { echo "hc 꺼짐" >&2; exit 4; }
case "$*" in
  "get --json t9")
    if [ "$FAKE_MAIN" = placed ]; then echo '{"address": "t9", "cwd": "/r/proj", "labels": {"herdr.space": "sp9", "herdr.tab": "tb9", "ovis.workspace": "ws9"}}'
    else echo '{"address": "t9", "cwd": "/r/proj", "labels": {}}'; fi ;;
  "get --json t1") echo '{"address": "t1", "labels": {"herdr.space": "sp1"}}' ;;
  "get --json t5") echo '{"address": "t5", "parent": "t9", "name": "lane-a"}' ;;
  "get --json t6") echo '{"address": "t6", "parent": "t9", "labels": {"where": "proj / 1"}}' ;;
  "get --json t7") echo '{"address": "t7", "parent": "t8"}' ;;
  "ls --json") echo '{"terminals": [
    {"address": "w1/t2", "cwd": "/r/proj", "where": "proj / 1", "state": "live"},
    {"address": "t1", "cwd": "/r/proj", "where": "proj / 1", "state": "live"},
    {"address": "t9", "cwd": "/r/proj", "state": "live"},
    {"address": "t5", "parent": "t9", "name": "lane-a", "state": "live"},
    {"address": "t4", "parent": "t9", "state": "live"},
    {"address": "t8", "parent": "t9", "name": "old", "state": "done"}]}' ;;
  "ls --json --label ovis.workspace=proj") echo '{"terminals": []}' ;;
  label\ *) ;;
  *) echo "모르는 hc 인자: $*" >&2; exit 9 ;;
esac
`

func TestOvisSplit(t *testing.T) { eachImpl(t, "ovis-split", testOvisSplit) }

func testOvisSplit(t *testing.T, T string, e Env) {
	mkdirs(t, T+"/bin", T+"/home")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/bin/python3")
	script(t, T+"/bin/hc", T, fakeOvisHc)
	iv := implVar("ovis-split")
	LOG := T + "/hc.log"
	var errOut string
	// osp: env -i 꼴. FAKE_MAIN 기본 bare
	osp := func(kv []string, args ...string) (string, int) {
		write(t, LOG, "")
		env := Env{"HOME": T + "/home", "PATH": T + "/bin:/usr/bin:/bin", "FAKE_HC_LOG": LOG, "FAKE_MAIN": "bare", iv: e[iv]}.With(kv...)
		out, se, rc := run2(T, env, filepath.Join(repo, "bin", "ovis-split"), args...)
		errOut = se
		return out, rc
	}
	U := regexp.MustCompile(`ovis-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	uu := func(s string) string { return U.ReplaceAllString(s, "ovis-UUID") }
	eq := func(name, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("FAIL - %s: [%s] != [%s] err=[%s]", name, got, want, errOut)
		}
	}
	trim := func(s string) string { return strings.TrimRight(s, "\n") }
	labels := func() string {
		var l []string
		for _, x := range strings.Split(read(LOG), "\n") {
			if strings.HasPrefix(x, "label") {
				l = append(l, x)
			}
		}
		return strings.Join(l, "\n")
	}

	out, rc := osp(nil, "--self-test")
	eq("--self-test는 ok", fmt.Sprintf("%d %s", rc, out), "0 ok")
	out, rc = osp(nil, "--main", "t9")
	eq("--main(라벨 없음): 같은 폴더 앱 칸 공간, 새 탭, 이름 붙은 산 자식", fmt.Sprintf("%d %s", rc, uu(out)), `0 {"main": "t9", "space": "sp1", "tab": "ovis-UUID", "added": ["t5"]}`)
	eq("--main(라벨 없음): hc 부른 순서·label 인자", trim(uu(read(LOG))), `get --json t9
ls --json
ls --json --label ovis.workspace=proj
get --json t1
label t9 herdr.space=sp1 herdr.tab=ovis-UUID ovis.workspace=proj ovis.tab=지휘 t9
label t5 herdr.space=sp1 herdr.tab= ovis.workspace=proj ovis.tab=lane-a · t9`)
	out, rc = osp([]string{"FAKE_MAIN=placed"}, "--main", "t9", "--add", "t5", "t42")
	eq("--add(이미 붙은 지휘): 지휘 라벨 그대로, 준 worker만", fmt.Sprintf("%d %s", rc, out), `0 {"main": "t9", "space": "sp9", "tab": "tb9", "added": ["t5", "t42"]}`)
	eq("--add: label 인자(살아 있지 않은 worker는 주소로 이름)", labels(), `label t5 herdr.space=sp9 herdr.tab= ovis.workspace=ws9 ovis.tab=lane-a · t9
label t42 herdr.space=sp9 herdr.tab= ovis.workspace=ws9 ovis.tab=t42 · t9`)
	out, rc = osp(nil, "--main", "t9", "--drop", "t5", "t7", "t6")
	eq("--drop: 자식만 빼고 JSON", fmt.Sprintf("%d %s", rc, out), `0 {"main": "t9", "dropped": ["t5"]}`)
	eq("--drop: 라벨 넷을 비움", labels(), "label t5 herdr.space= herdr.tab= ovis.workspace= ovis.tab=")
	eq("--drop: 남의 자식·앱 칸은 건너뜀 알림", trim(errOut), `건너뜀 t7: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다
건너뜀 t6: 지휘나 그 자식이 아니거나 Herdr 앱 칸이다`)
	_, rc = osp(nil)
	eq("인자 없음: usage·종료 2", fmt.Sprintf("%d|%s", rc, trim(errOut)), `2|usage: ovis-split [-h] [--main MAIN] [--add TERMINAL [TERMINAL ...]]
                  [--drop TERMINAL [TERMINAL ...]] [--self-test]
ovis-split: error: --main이 필요하다`)
	_, rc = osp(nil, "--main", "t9", "--add")
	el := strings.Split(trim(errOut), "\n")
	eq("--add 값 없음: 종료 2", fmt.Sprintf("%d|%s", rc, el[len(el)-1]), "2|ovis-split: error: argument --add: expected at least one argument")
	out, rc = osp([]string{"FAKE_HC_FAIL=1"}, "--main", "t9")
	eq("hc 실패: 종료 1·라벨 안 붙임", fmt.Sprintf("%d %s %d", rc, out, grepc(read(LOG), "^label")), "1  0")
}
