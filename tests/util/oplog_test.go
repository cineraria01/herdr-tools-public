package util_test

// 운영 기록(internal/oplog, 2026-11-23 ht-oplog) 길목 계약: 도구가 겪은 이상이 ops.tsv에 한 줄로 남고, 표준 오류·종료 코드는 그대로다.
// 기록 경로는 HERDR_TOOLS_OPLOG로 시험 폴더에(XDG_STATE_HOME을 바꾸지 않는다 — 기본 경로 한 장면만 임시 HOME 아래)

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// opRows: ops.tsv의 줄을 칸으로
func opRows(t *testing.T, p string) [][]string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("운영 기록이 없다: %v", err)
	}
	var rs [][]string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		c := strings.Split(l, "\t")
		if len(c) != 8 {
			t.Fatalf("8칸이어야: %q", l)
		}
		rs = append(rs, c)
	}
	return rs
}

// hc-lane die: 표준 오류 한 줄·종료 1 그대로, 기록에 die 줄 하나(exit 줄은 겹쳐 남지 않는다)
func TestOpLogHcLaneDie(t *testing.T) {
	T := tmpdir(t)
	d := T + "/nogit"
	mkdirs(t, d, T+"/h")
	ops := T + "/ops.tsv"
	e := isolated(T).With("HOME="+T+"/h", "PWD="+d) // hc-lane은 셸처럼 $PWD를 쓴다
	so, se, rc := run2(d, e.With("HERDR_TOOLS_OPLOG="+ops), tool(t, "hc-lane"), "land", "main")
	want := "hc-lane: land: git 작업 폴더 안에서 부른다(레인 워크트리): " + d + "\n"
	if rc != 1 || so != "" || se != want {
		t.Fatalf("표준 오류·종료 코드 그대로여야: rc=%d out=%q err=%q", rc, so, se)
	}
	rs := opRows(t, ops)
	if len(rs) != 1 || rs[0][1] != "hc-lane" || rs[0][2] != "land" || rs[0][4] != "die" || rs[0][5] != "1" || !strings.HasSuffix(d, rs[0][6]) || !strings.HasSuffix(rs[0][6], "/nogit") ||
		rs[0][7] != strings.TrimSuffix(strings.TrimPrefix(want, "hc-lane: "), "\n") {
		t.Fatalf("die 줄 하나: %q", rs)
	}
	// 지정이 비면 기본 자리 ~/.local/state/herdr-tools/ops.tsv(시험 바이너리는 지정이 없으면 /dev/null — internal/oplog init)
	if _, _, rc := run2(d, e.With("HERDR_TOOLS_OPLOG="), tool(t, "hc-lane"), "land", "main"); rc != 1 {
		t.Fatalf("rc %d", rc)
	}
	if rs := opRows(t, T+"/h/.local/state/herdr-tools/ops.tsv"); len(rs) != 1 || rs[0][4] != "die" {
		t.Fatalf("기본 자리에 die 줄: %q", rs)
	}
}

// 감사 L1: 사용법(-h·--help, 종료 2)은 이상이 아니다 — 표준 오류·종료 코드는 그대로, 기록은 남기지 않는다
func TestOpLogUsageNotLogged(t *testing.T) {
	T := tmpdir(t)
	ex, err := os.ReadFile(filepath.Join(repo, "config.example", "hc-fill.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, T+"/conf/hc-fill.tsv", string(ex))
	ops := T + "/ops.tsv"
	e := isolated(T).With("HOME="+T+"/h", "HERDR_TOOLS_CONFIG="+T+"/conf", "HERDR_TOOLS_OPLOG="+ops)
	for _, c := range [][]string{{"hc-lane", "-h"}, {"hc-lane", "report", "--help"}, {"hc-fill", "--help"}} {
		_, se, rc := run2(T, e, tool(t, c[0]), c[1:]...)
		if rc != 2 || !strings.Contains(se, "사용법") {
			t.Fatalf("%q: 종료 2·사용법 그대로여야: rc=%d err=%.80q", c, rc, se)
		}
	}
	if _, err := os.Stat(ops); !os.IsNotExist(err) {
		t.Fatalf("사용법은 기록하지 않는다: %q", read(ops))
	}
}

// hc-fill 0이 아닌 종료: 대기열을 못 읽어 exit(1) — 표준 오류·종료 코드 그대로, 기록에 exit 줄
func TestOpLogHcFillExit(t *testing.T) {
	T := tmpdir(t)
	ex, err := os.ReadFile(filepath.Join(repo, "config.example", "hc-fill.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, T+"/conf/hc-fill.tsv", string(ex))
	q := T + "/h/.local/share/hc-fill/queue.tsv"
	mkdirs(t, q) // 대기열이 폴더 — 있는데 못 읽는다
	ops := T + "/ops.tsv"
	so, se, rc := run2(T, isolated(T).With("HOME="+T+"/h", "HERDR_TOOLS_CONFIG="+T+"/conf", "HERDR_TOOLS_OPLOG="+ops),
		tool(t, "hc-fill"), "rm", "nope")
	if rc != 1 || so != "" || se != "hc-fill: 대기열을 읽지 못해 고치지 않는다: "+q+"\n" {
		t.Fatalf("표준 오류·종료 코드 그대로여야: rc=%d out=%q err=%q", rc, so, se)
	}
	rs := opRows(t, ops)
	if len(rs) != 1 || rs[0][1] != "hc-fill" || rs[0][2] != "rm" || rs[0][4] != "exit" || rs[0][5] != "1" {
		t.Fatalf("exit 줄 하나: %q", rs)
	}
}
