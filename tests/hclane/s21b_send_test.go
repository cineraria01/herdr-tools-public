package hclane_test

// tests/hc-lane.test.sh 21절 뒷부분(3): send --all·broadcast(이 지휘의 살아 있는 레인 전부), fetch 참조 잠금 경합 다시 하기(lane-fetch-race).

import (
	"os"
	"sort"
	"strings"
)

func init() { reg(112, "s21b3", s21b3) }

func s21b3(k *K) {
	k.unset("XDG_STATE_HOME")
	L := k.HCLOG
	send := L + ".send"

	// send --all(2026-11-17 지휘 「실수는 바로 코드로」)
	SD := k.T + "/.local/state/hc-lane"
	mkdirs(k.t, SD)
	os.Remove(send)
	os.Remove(L + ".sendfail")
	write(k.t, SD+"/lanes.tsv", "tA1\timpl-a\ttC\nmini/tA2\timpl-remote\ttC\ntA3\taudit-x\ttC\tread\ntA4\tother-lane\ttO\n"+
		"tA5\tdead-lane\ttC\ntA6\tfill-lane\t-\tfill\n")
	lsS := `{"terminals":[{"address":"tA1","cwd":"/","name":"impl-a","parent":"mac/tC","state":"live"},{"address":"mini/tA2","cwd":"/","name":"impl-remote","parent":"mac/tC","state":"live"},
{"address":"tA3","cwd":"/","name":"audit-x","parent":"mac/tC","state":"live"},{"address":"tA4","cwd":"/","name":"other-lane","parent":"mac/tO","state":"live"},
{"address":"tA5","name":"dead-lane","parent":"mac/tC","state":"finished"},{"address":"tA6","cwd":"/","name":"fill-lane","parent":"mac/tC","state":"live"},
{"address":"mini/tA7","cwd":"/","name":"audit-y","parent":"mac/tC","state":"live"},
{"address":"tA9","cwd":"/","name":"no-rec","parent":"tC","state":"live"}]}`
	tc := []string{"HC_TERMINAL_ID=tC", "HC_FAKE_LS=" + lsS}
	k.runE(tc, "send", "--all", "마감 07:30으로 연장")
	sent := func() string { // cut -f1 | sort | tr '\n' ' '
		var l []string
		for _, s := range s21bLines(send) {
			f, _, _ := strings.Cut(s, "\t")
			l = append(l, f)
		}
		sort.Strings(l)
		o := ""
		for _, s := range l {
			o += s + " "
		}
		return o
	}
	k.check("send --all: 이 지휘의 살아 있는 구현 레인 전부(원격·기록 없는 parent·fill 포함), 감사·남의 지휘·끝난 레인은 뺌", k.rc == 0 && sent() == "mini/tA2 tA1 tA6 tA9 ")
	k.check("send --all: 글 그대로, 보낸 주소를 한 줄씩", contains(s21bLines(send), "tA1\t마감 07:30으로 연장") &&
		strings.Contains(k.out, "보냄 mini/tA2 impl-remote") && strings.Contains(k.out, "보냄 4 · 실패 0"))
	os.Remove(send)
	k.runE(tc, "broadcast", "--audits", "x")
	k.check("broadcast --audits: 감사 레인(read 기록·audit- 이름)도 포함", k.rc == 0 && sent() == "mini/tA2 mini/tA7 tA1 tA3 tA6 tA9 ")
	os.Remove(send)
	write(k.t, L+".sendfail", "")
	k.runE(tc, "send", "--all", "x")
	os.Remove(L + ".sendfail")
	k.check("send --all: 보내기 실패는 한 줄씩 알리고 종료 1", k.rc == 1 && strings.Contains(k.out, "실패 tA1 impl-a") && strings.Contains(k.out, "보냄 0 · 실패 4"))
	k.runE([]string{"HC_TERMINAL_ID=tC", "HC_FAKE_LS_FAIL=1"}, "send", "--all", "x")
	k.check("send --all: hc ls 실패면 아무에게도 안 보내고 종료 1", k.rc == 1 && !s21bS(send) && strings.Contains(k.errf(), "hc ls"))
	k.runE(tc, "send", "x")
	k.check("send: --all 없으면 종료 2(일부에만 보내는 꼴은 없다)", k.rc == 2 && !s21bS(send))
	k.runE([]string{"HC_FAKE_LS=" + lsS}, "send", "--all", "x")
	k.check("send --all: 지휘 주소(HC_TERMINAL_ID·--from)가 없으면 종료 2", k.rc == 2 && !s21bS(send))
	k.runE([]string{"HC_FAKE_LS=" + lsS}, "send", "--all", "--from", "tO", "x")
	k.check("send --all --from: 그 지휘의 레인에만", k.rc == 0 && sent() == "tA4 ")
	os.Remove(send)
	k.runE(tc, "send", "--all", "--", "--all", "--audits", "끝")
	tA3 := false
	for _, s := range s21bLines(send) {
		tA3 = tA3 || strings.HasPrefix(s, "tA3")
	}
	k.check("send --all --: 뒤 낱말은 옵션 꼴이어도 글(감사 L4)", k.rc == 0 && contains(s21bLines(send), "tA1\t--all --audits 끝") && !tA3)
	os.Remove(send)
	// 같은 주소·이름의 기록이 두 지휘(tC 다음 tO)이고 지금 부모는 tC — 소유 충돌이라 어느 쪽도 보내지 않고 보류(재감사2 lane/R-209: 마지막 기록이 이기던 것)
	write(k.t, SD+"/lanes.tsv", "tA8\tmoved\ttC\ntA8\tmoved\ttO\n")
	lsM := `{"terminals":[{"address":"tA8","cwd":"/","name":"moved","parent":"mac/tC","state":"live"}]}`
	for _, from := range []string{"tC", "tO"} {
		k.runE([]string{"HC_FAKE_LS=" + lsM}, "send", "--all", "--from", from, "x")
		k.check("send --all --from "+from+": 주인이 엇갈린 tA8은 보류(종료 1, 안 보냄)", k.rc == 1 && !s21bS(send) && strings.Contains(k.out, "보류 tA8 moved"))
	}
	os.Remove(SD + "/lanes.tsv")

	// lane-fetch-race: git fetch가 참조 잠금 경합으로 실패하면 짧게 쉬고 최대 3번 다시 한다(2026-11-18 워커 기계 실측)
	gitBin, _ := k.exe(k.T, "", nil, "sh", "-c", "command -v git")
	write(k.t, k.T+"/racebin/git", `#!/bin/bash
for a; do
  [ "$a" = fetch ] || continue
  echo f >>"$GITRACE.n"
  c=$(cat "$GITRACE" 2>/dev/null || echo 0)
  [ "$c" -gt 0 ] && { echo $((c - 1)) >"$GITRACE"; printf '%s\n' "$GITRACE_MSG" >&2; exit 1; }
  break
done
exec `+gitBin+` "$@"
`)
	if err := os.Chmod(k.T+"/racebin/git", 0o755); err != nil {
		k.t.Fatal(err)
	}
	race := func(n string) { write(k.t, k.T+"/race", n+"\n"); os.Remove(k.T + "/race.n") }
	fn := func() int { // wc -l <"$T/race.n" (없으면 -1 — 셸은 빈 값)
		if !exists(k.T + "/race.n") {
			return -1
		}
		return strings.Count(read(k.T+"/race.n"), "\n")
	}
	rmsg := "error: cannot lock ref 'refs/remotes/origin/main': is at 6323151 but expected 45c382a"
	re := func(msg string) []string {
		return []string{"GITRACE=" + k.T + "/race", "GITRACE_MSG=" + msg, "PATH=" + k.T + "/racebin:" + k.env["PATH"]}
	}
	k.set("HC_LANE_FETCH_WAITS=0 0 0")
	k.mk("tfr")
	race("1")
	k.runE(re(rmsg), "--name", "fr-start", "--repo", k.R, "--", "true")
	k.check("fetch 경합: 시작 fetch가 잠금 경합이면 다시 해 레인을 띄운다", k.rc == 0 && isDir(k.R+".wt/fr-start") && fn() == 2 && strings.Contains(k.errf(), "다시"))
	race("1")
	k.runE(re("error: unable to update local ref 'refs/remotes/origin/main'"), "fetch", "--repo", k.R)
	k.check("fetch 경합: hc-lane fetch도 unable to update local ref면 다시", k.rc == 0 && fn() == 2)
	race("1")
	k.runE(re("error: 로컬 레퍼런스를 업데이트할 수 없습니다"), "fetch", "--repo", k.R)
	k.check("fetch 경합: 한국어 git 메시지(로컬 레퍼런스를 업데이트할 수 없습니다)도 다시", k.rc == 0 && fn() == 2)
	// 한국어 git 카탈로그(git 2.56.1 ko): "Unable to create '%s.lock': %s" → "'%s.lock'을 만들 수 없습니다: %s"
	race("1")
	k.runE(re("error: '"+k.R+"/.git/refs/remotes/origin/main.lock'을 만들 수 없습니다: File exists."), "fetch", "--repo", k.R)
	k.check("fetch 경합: 한국어 .lock 만들기 실패('….lock'을 만들 수 없습니다)도 다시", k.rc == 0 && fn() == 2)
	race("1")
	k.runE(re("fatal: could not read Username for 'https://github.com'"), "--name", "fr-auth", "--repo", k.R, "--", "true")
	k.check("fetch 경합: 다른 실패는 다시 하지 않고 그대로 실패", k.rc != 0 && !isDir(k.R+".wt/fr-auth") && fn() == 1 && strings.Contains(k.errf(), "could not read Username"))
	race("9")
	k.runE(re(rmsg), "--name", "fr-many", "--repo", k.R, "--", "true")
	k.check("fetch 경합: 다시 하기는 3번까지(모두 4번), 그래도 실패면 실패", k.rc != 0 && !isDir(k.R+".wt/fr-many") && fn() == 4)
	k.seed(".claude/herdr-verify.sh", "exit 0")
	k.mklane("fr-land")
	k.seedp("c2", "c2")
	race("1")
	k.runIn(k.wt, re(rmsg), "land")
	// fetch 3번 = 낡은 착지 스크립트 확인 fetch(경합 1번 → 다시 1번) + 착지 반복의 fetch 1번
	k.check("fetch 경합: land의 fetch도 다시 해 착지", k.rc == 0 && k.otop() == "fr-land" && fn() == 3)
	race("1")
	k.runE(re(rmsg), "clean", "--repo", k.R, "--branch", "fr-land")
	k.check("fetch 경합: clean의 fetch도 다시 해 정리", k.rc == 0 && !exists(k.R+".wt/fr-land") && fn() == 2)
	k.unset("HC_LANE_FETCH_WAITS")
}
