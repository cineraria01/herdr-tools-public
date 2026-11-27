package main

// herdr-sync 계약 시험 19(ht-sync-arg-guards, 2026-11-28): 기계 자리·--take·--pull의 <기계>:는 기계 표(hc machine ls)·이 기계·repo만 받고
// (모르는 이름은 아무것도 하기 전에 거절 — 배포 기록·원격 호출 없음), --take·--pull로 기계를 가리키고 기계 인자를 따로 안 적으면 그 기계만 돈다.
// 원격 호출은 가짜 hc가 기계마다 적는 $HCT/calls로 센다

import (
	"os"
	"strings"
	"testing"

	ct "herdr-tools/internal/clitest"
)

func TestContractArgs(t *testing.T) {
	s := newSX(t)
	s.synced()
	T := s.T
	dlog := s.MAC + "/.local/state/herdr-sync/deploy.tsv"
	calls := func() map[string]int {
		n := map[string]int{}
		for _, l := range ct.Lines(ct.Read(T + "/calls")) {
			n[l]++
		}
		return n
	}
	runC := func(args ...string) map[string]int { s.put(T+"/calls", ""); s.run(args...); return calls() }

	// 이미 쌓인 상태: 옛 실행이 모르는 이름을 기계로 다뤄 남긴 줄 — 읽을 때 지금처럼, 새로 안 생김
	ls := ct.Lines(ct.Read(dlog))
	f := strings.Split(ls[len(ls)-1], "\t")
	f[len(f)-1] = "status=실패"
	ct.Append(t, dlog, strings.Join(f, "\t")+"\n")
	log0, snap0 := ct.Read(dlog), s.snap()
	s.run("--log", "1")
	s.ok("19 옛 가짜 이름 줄: --log가 지금처럼 읽는다", s.r.RC == 0 && s.contains("status=실패"))

	// ① 모르는 기계 이름 → 거절·기록 없음·원격 호출 0, 두 번째 실행도 같음(쌓이는 것 없음)
	for i := 0; i < 2; i++ {
		c := runC("status")
		s.ok("19① 모르는 기계: 거절 한 줄·종료 2", s.r.RC == 2 && s.has("herdr-sync: 모르는 기계 status — 기계 표: 맥 w1 lx"))
		s.ok("19① 모르는 기계: 기록·파일·원격 호출 없음", ct.Read(dlog) == log0 && s.snap() == snap0 && len(c) == 0)
	}
	c := runC("--check", "W1")
	s.ok("19 대소문자가 다른 이름도 모르는 기계", s.r.RC == 2 && s.contains("모르는 기계 W1") && len(c) == 0)
	c = runC("w1", "old-name")
	s.ok("19 옛 이름(기계 표에 없음)이 섞여도 아무 데도 안 감", s.r.RC == 2 && s.contains("모르는 기계 old-name") && len(c) == 0 && ct.Read(dlog) == log0)

	// ② --take·--pull 모르는:<경로> → 거절
	for _, o := range []string{"--take", "--pull"} {
		c = runC(o, "nope:.claude/docs/b.md")
		s.ok("19② "+o+" 모르는 기계: 거절", s.r.RC == 2 && s.has("herdr-sync: 모르는 기계 nope (nope:.claude/docs/b.md) — 기계 표: 맥 w1 lx") &&
			len(c) == 0 && ct.Read(dlog) == log0 && s.snap() == snap0)
	}

	// ③ --take <기계>:<경로> → 그 기계만
	c = runC("--take", "w1:.claude/docs/b.md")
	s.ok("19③ --take w1: w1만 돈다", s.r.RC == 0 && c["w1"] > 0 && c["lx"] == 0 && s.has("w1: 같음") && !s.contains("lx:"))
	c = runC("--pull", "w1:.claude/docs/a.md")
	s.ok("19③ --pull w1: w1만 돈다", s.r.RC == 0 && c["w1"] > 0 && c["lx"] == 0 && !s.contains("lx:"))
	c = runC("--take", "맥:.claude/docs/b.md")
	s.ok("19③ --take 맥: 이 기계만(원격 호출 0)", s.r.RC == 0 && len(c) == 0 && s.has("맥: 같음"))
	// ④ 여럿이면 합집합
	c = runC("--take", "w1:.claude/docs/b.md", "--take", "lx:.claude/CLAUDE.md#body")
	s.ok("19④ --take w1: --take lx: 둘 다", s.r.RC == 0 && c["w1"] > 0 && c["lx"] > 0)
	// ⑤ 기계 인자를 적으면 그 목록(지금처럼)
	c = runC("--take", "w1:.claude/docs/b.md", "w1", "lx")
	s.ok("19⑤ --take w1: w1 lx → 둘 다", s.r.RC == 0 && c["w1"] > 0 && c["lx"] > 0)
	// ⑥ 가리킨 기계가 목록에 없음 → 거절 한 줄
	log6 := ct.Read(dlog)
	c = runC("--take", "w1:.claude/docs/b.md", "lx")
	s.ok("19⑥ --take w1: lx → 거절", s.r.RC == 2 && s.has("herdr-sync: --take·--pull이 가리킨 기계 w1 — 고른 기계(lx)에 없다 (w1:.claude/docs/b.md)") &&
		len(ct.Lines(s.r.Out)) == 1 && len(c) == 0 && ct.Read(dlog) == log6)
	// ⑦ repo: 는 지금 그대로(저장소 단계, 기계는 모두)
	c = runC("--take", "repo:.claude/docs/b.md")
	s.ok("19⑦ --take repo: 모든 기계", s.r.RC == 0 && c["w1"] > 0 && c["lx"] > 0)
	// ⑧ --check <기계>·인자 없는 배포는 그대로
	c = runC("--check", "w1")
	s.ok("19⑧ --check w1: w1만", s.r.RC == 0 && c["w1"] > 0 && c["lx"] == 0)
	c = runC()
	s.ok("19⑧ 인자 없는 배포: 모든 기계", s.r.RC == 0 && c["w1"] > 0 && c["lx"] > 0 && s.has("w1: 같음") && s.has("lx: 같음"))

	// 외부 가정: sh 없는 기계(Windows 등)를 가리킴 → 지금의 「건너뜀」 한 줄, 다른 기계는 안 감
	os.MkdirAll(T+"/m/win", 0o755)
	s.put(T+"/m/win/NOSH", "")
	t.Setenv("HC_FAKE_MACHINES", `{"name":"w1"},{"name":"lx"},{"name":"win"}`)
	c = runC("--take", "win:.claude/docs/b.md")
	s.ok("19 sh 없는 기계를 가리킴: 건너뜀 한 줄, 다른 기계 0", s.r.RC == 0 && s.has("win: 건너뜀 (sh 없음 — Windows 등)") && c["w1"] == 0 && c["lx"] == 0)
	// 외부 가정: 기계 표가 비었음 → 지금처럼 맥 확인에서 멈춤(아무것도 안 씀)
	t.Setenv("HC_FAKE_MACHINES", "")
	log1 := ct.Read(dlog)
	c = runC("status")
	s.ok("19 기계 표가 빔: 맥 확인에서 멈춤", s.r.RC == 2 && s.contains("맥에서만 돈다") && len(c) == 0 && ct.Read(dlog) == log1)
	s.c.Done()
}
