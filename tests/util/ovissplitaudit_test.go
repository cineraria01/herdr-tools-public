package util_test

// 2026-11-21 Go 전환 전체 감사(go-final-audit base) 가운데 ovis-split 지적의 계약 시험. 이름 앞 [ID]는 보고서 지적 ID.
// 셸 판(파이썬)은 KeyError·JSONDecodeError로 종료 1, 라벨 0회 — Go 판도 그래야 한다(계약은 셸 판 동작).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const fakeOvisAuditHc = `#!/bin/bash
printf '%s\n' "$*" >>"$FAKE_HC_LOG"
case "$*" in
  "get --json t9") case $FAKE_MAIN in
      noaddr) echo '{"cwd": "/r/proj", "labels": {}}' ;;
      empty) ;;
      *) echo '{"address": "t9", "cwd": "/r/proj", "labels": {}}' ;;
    esac ;;
  "get --json t2") echo '{"address": "t2", "parent": "t9", "name": "lane-b"}' ;;
  "ls --json") echo '{"terminals": [{"address": "t9", "cwd": "/r/proj", "state": "live"}, {"address": "t2", "parent": "t9", "name": "lane-b", "state": "live"}]}'
    [ -z "${FAKE_TRAIL-}" ] || echo ']' ;;
  "ls --json --label ovis.workspace=proj") echo '{"terminals": []}' ;;
  label\ *) ;;
  *) echo "모르는 hc 인자: $*" >&2; exit 9 ;;
esac
`

func TestOvisSplitAudit(t *testing.T) { eachImpl(t, "ovis-split", testOvisSplitAudit) }

func testOvisSplitAudit(t *testing.T, T string, e Env) {
	mkdirs(t, T+"/bin", T+"/home")
	PY, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 없음")
	}
	os.Symlink(PY, T+"/bin/python3")
	script(t, T+"/bin/hc", T, fakeOvisAuditHc)
	iv := implVar("ovis-split")
	LOG := T + "/hc.log"
	osp := func(kv ...string) (string, int) {
		write(t, LOG, "")
		env := Env{"HOME": T + "/home", "PATH": T + "/bin:/usr/bin:/bin", "FAKE_HC_LOG": LOG, iv: e[iv]}.With(kv...)
		out, se, rc := run2(T, env, filepath.Join(repo, "bin", "ovis-split"), "--main", "t9", "--add", "t2")
		return out + se, rc
	}
	out, rc := osp("FAKE_MAIN=bare")
	if rc != 0 || grepc(read(LOG), "^label") != 2 {
		t.Errorf("FAIL - 정상 입력은 라벨 둘(시험 준비 확인) rc=%d %s", rc, out)
	}
	for _, c := range []struct{ id, kv, what string }{
		{"B21", "FAKE_MAIN=noaddr", "address 없는 hc get"},
		{"B21", "FAKE_MAIN=empty", "빈 hc get"},
		{"B13", "FAKE_TRAIL=1", "완결된 JSON 뒤 찌꺼기(])가 든 hc ls"},
	} {
		out, rc := osp(c.kv)
		if n := grepc(read(LOG), "^label"); rc != 1 || n != 0 {
			t.Errorf("FAIL - [%s] %s: 종료 1·라벨 0회여야 함 (rc=%d 라벨 %d)\n%s", c.id, c.what, rc, n, fmt.Sprint(out))
		}
	}
}
