package hclane_test

// hc-exec-result(2026-11-23): --on(hc exec)으로 묻는 있음·없음은 hc 자신의 실패(종료 1)를 「없음」으로 읽지 않는다 —
// 그 바퀴에 아무것도 지우지 않고 멈추거나 남긴다. 가짜 hc의 $HC_FAKE_EXEC_FAIL이 맞는 명령 줄만 hc 실패(종료 1)로 끝낸다.

import (
	"path/filepath"
	"strings"
	"testing"
)

const dirTest = `*-e "$1"*` // exists()·gc의 폴더 있음 조회(sh -c '[ -e "$1" ] || [ -L "$1" ]')

func TestHcExecResult(t *testing.T) {
	withK(t, func(k *K) {
		k.mk("txr")
		k.seed(".gitignore", ".omc/") // land가 남기는 무시 파일
		// 변이 없는 대조: hc가 멀쩡하면 --on clean이 정리한다(아래 장면들이 「아무 일도 안 함」으로 거저 통과하지 않게)
		k.land("x-ok")
		k.run("clean", "--on", "box", "--repo", k.R, "--branch", "x-ok")
		k.check("대조: --on clean 정리", k.rc == 0 && k.gone("x-ok"))
		k.check("--on 조회는 hc exec --json", strings.Contains(read(k.HCLOG+".exec"), "\n--json\n"))

		for _, c := range []struct{ br, name, pat string }{
			{"x-d", "폴더 있음 조회(exists)", dirTest},
			{"x-b", "가지 있음 조회(hasBranch)", "*show-ref --verify -q refs/heads/x-b*"},
		} {
			k.land(c.br)
			k.runE([]string{"HC_FAKE_EXEC_FAIL=" + c.pat}, "clean", "--on", "box", "--repo", k.R, "--branch", c.br)
			k.check("clean --on: "+c.name+"에서 hc 실패 → 없음으로 읽지 않고 멈춤, 아무것도 안 지움",
				k.rc != 0 && k.kept(c.br) && k.regd(c.br) && !k.last(`"clean":"nothing"`))
			k.run("clean", "--on", "box", "--repo", k.R, "--branch", c.br)
			k.check("clean --on: hc가 돌아오면 정리("+c.br+")", k.rc == 0 && k.gone(c.br))
		}

		// gc: 들어간 가지의 작업 폴더. 조상 조회·폴더 조회가 hc 실패면 남기고 종료 ≠0.
		// g-m(merge-base)은 회귀 방지용 — 고치기 전에도 뒤의 gin 스크립트(R-09 지킴)가 고유 커밋을 못 찾아 「확인 못 함」으로 남겼다
		for _, c := range []struct{ br, name, pat string }{
			{"g-m", "들어갔나 조회(merge-base)", "*merge-base --is-ancestor*"},
			{"g-d", "폴더 있음 조회", dirTest},
		} {
			k.land(c.br)
			k.runE([]string{"HC_FAKE_EXEC_FAIL=" + c.pat}, "gc", "--on", "box", "--repo", k.R, "--min-age", "0")
			k.check("gc --on: "+c.name+"에서 hc 실패 → 남김(종료 ≠0)", k.rc != 0 && k.kept(c.br) && k.regd(c.br))
			k.run("gc", "--on", "box", "--repo", k.R, "--min-age", "0")
			k.check("gc --on: hc가 돌아오면 지움("+c.br+")", k.rc == 0 && k.gone(c.br))
		}

		// 띄우기: 가지·폴더가 이미 있나 조회가 hc 실패면 「없음」으로 잇지 않고 멈춘다(작업 트리를 만들지 않음)
		for _, c := range []struct{ br, name, pat string }{
			{"l-b", "로컬 가지", "*show-ref --verify -q refs/heads/l-b*"},
			{"l-r", "원격 가지", "*show-ref --verify -q refs/remotes/origin/l-r*"},
			{"l-e", "폴더", "*test -e *l-e"},
		} {
			k.runE([]string{"HC_FAKE_EXEC_FAIL=" + c.pat}, "--on", "box", "--name", c.br, "--repo", k.R, "--", "true")
			k.check("띄우기 --on: "+c.name+" 있나 조회에서 hc 실패 → 멈춤, 작업 트리 안 만듦", k.rc != 0 && !exists(k.R+".wt/"+c.br) && !k.regd(c.br))
		}
		k.run("--on", "box", "--name", "l-ok", "--repo", k.R, "--", "true")
		k.check("대조: hc가 멀쩡하면 --on 띄우기", k.rc == 0 && k.regd("l-ok"))
		// 가짜 hc --json도 진짜처럼 못 띄우는 프로그램은 hc 실패(spawn_failed·종료 1)
		_, rc := k.exe(k.T, "", nil, filepath.Join(k.T, "bin", "hc"), "exec", "--json", "--", "/no/such/prog")
		k.check("가짜 hc: --json 못 띄움은 종료 1", rc == 1)
	})
}
