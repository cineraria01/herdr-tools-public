package main

// herdr-sync 계약 시험 5~12: --pull, 따로 고친 기계, 맥 ↔ 저장소(레인 착지), 실행 권한, push 거절, 깨진 전송, 연결 실패,
// sh 없는 기계·다른 OS, 설정 충돌, 고른 기계, 더러운 저장소, 범용으로 갈 개인 값

import (
	"os"
	"strings"

	ct "herdr-tools/internal/clitest"
	"herdr-tools/internal/ojson"
)

func (s *sx) repoAndMore() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	// 5. --pull: 그 기계 값을 맥으로 가져와 저장소·다른 기계까지
	s.run("--pull", "lx:.claude/CLAUDE.md#body")
	s.ok("--pull: 종료 0", s.r.RC == 0)
	s.ok("--pull: 맥 본문이 lx 값, 맥 OMC는 그대로", s.is(M+"/.claude/CLAUDE.md", "OMC mac\n<!-- User customizations -->\n본문 0\n"))
	s.ok("--pull: 저장소와 w1까지", s.corigin("show", "main:claude/CLAUDE.body.md") == "본문 0" && s.is(W1+"/.claude/CLAUDE.md", "OMC w1\n<!-- User customizations -->\n본문 0\n"))
	s.run("--check")
	s.ok("다 맞은 뒤 --check: 종료 0, 모두 같음", s.r.RC == 0 && s.has("w1: 같음") && s.has("lx: 같음") && s.has("맥: 저장소와 같음"))

	// 6. base가 있는 뒤 그 기계에서 따로 고치면 충돌, --take <기계>:*
	s.put(W1+"/.claude/docs/a.md", "w1이 고침\n")
	s.run()
	s.ok("따로 고친 기계: 충돌", s.r.RC == 1 && s.has("충돌 w1 .claude/docs/a.md (2줄 다름)") && s.is(W1+"/.claude/docs/a.md", "w1이 고침\n"))
	s.run("--take", "w1:*")
	s.ok("--take w1:*: 그 기계 충돌 전부", s.r.RC == 0 && s.is(W1+"/.claude/docs/a.md", "a1\n"))

	// 6-2. --pull <기계>:*는 그 기계의 대상만: 맥 전용 도구의 옛 사본은 가져오지 않는다
	s.put(W1+"/.local/bin/herdr-balance", "stale\n")
	s.run("--pull", "w1:*")
	s.ok("--pull 기계:*: 대상 밖 파일은 가져오지 않는다", s.is(M+"/.local/bin/herdr-balance", "#!/bin/sh\necho bal\n") && !s.contains("가져옴 .local/bin/herdr-balance"))
	s.run("--pull", "w1:.codex/AGENTS.md#codex-shared")
	s.ok("--pull Codex 구역: 거절", s.r.RC == 1 && s.contains("Codex 구역"))

	// 7. 맥 ↔ 저장소: 저장소만 바뀜(레인 착지) → 맥에 깔고 기계까지
	s.git(T, "clone", "-q", T+"/conf.git", T+"/lane")
	s.lane("lane", func() { s.put(T+"/lane/claude/docs/a.md", "a2\n") })
	s.run()
	s.ok("저장소만 바뀜: 맥에 깖(백업)·기계에 씀", s.r.RC == 0 && s.is(M+"/.claude/docs/a.md", "a2\n") && ct.GrepRx(M+"/.local/state/herdr-sync/bak", "a1") && s.is(W1+"/.claude/docs/a.md", "a2\n") && s.is(LX+"/.claude/docs/a.md", "a2\n"))
	// 둘 다 바뀜 → 멈춘다
	s.put(M+"/.claude/docs/a.md", "a3-mac\n")
	s.git(T+"/lane", "pull", "-q")
	s.lane("lane2", func() { s.put(T+"/lane/claude/docs/a.md", "a3-repo\n") })
	head1 := s.corigin("rev-parse", "main")
	s.run()
	s.ok("둘 다 바뀜: 충돌로 멈추고 아무 데도 안 쓴다", s.r.RC == 1 && s.has("충돌 저장소 .claude/docs/a.md (2줄 다름)") && s.corigin("rev-parse", "main") == head1 && s.is(M+"/.claude/docs/a.md", "a3-mac\n") && s.is(W1+"/.claude/docs/a.md", "a2\n"))
	s.run("--take", "repo:.claude/docs/a.md")
	s.ok("--take repo: 맥 값을 저장소와 기계에", s.r.RC == 0 && s.corigin("show", "main:claude/docs/a.md") == "a3-mac" && s.is(W1+"/.claude/docs/a.md", "a3-mac\n"))

	// 8. 실행 권한은 맥 값을 따른다
	ct.Chmod(s.t, M+"/.local/bin/hc-lane", 0o644)
	s.run()
	s.ok("실행 권한: 맥에서 뺀 x가 저장소·기계로", s.r.RC == 0 && strings.HasPrefix(s.origin("ls-tree", "main", "bin/hc-lane"), "100644") && ct.Exists(W1+"/.local/bin/hc-lane") && !ct.IsExec(W1+"/.local/bin/hc-lane"))

	// 7-2. 범용 저장소만 바뀜(범용 레인 착지) → tools: 줄을 맥에 깖
	s.git(T, "clone", "-q", T+"/origin.git", T+"/tlane")
	s.put(T+"/tlane/bin/hc-lane", "#!/bin/sh\necho lane2\n")
	s.git(T+"/tlane", "commit", "-qam", "tlane")
	s.git(T+"/tlane", "push", "-q", "origin", "main")
	// 착지 직후 --check: 맥이 저장소보다 뒤인 것을 보인다. 기계 줄 「같음」은 맥 옛 판과 같다는 뜻이다 (2026-11-19 「이미 퍼졌다」 오판)
	s.run("--check")
	s.ok("맥이 저장소보다 뒤 --check: 맥 줄에 보이고 종료 1", s.r.RC == 1 && s.has("맥: 저장소가 새로움 1개 (bin/hc-lane) — herdr-sync로 깐다") && s.is(M+"/.local/bin/hc-lane", "#!/bin/sh\necho lane\n"))
	s.ok("맥이 저장소보다 뒤 --check: 기계 줄 같음은 맥 판 기준", s.has("w1: 같음 (맥 판 기준 — 저장소 새 판은 아직)") && s.has("lx: 같음 (맥 판 기준 — 저장소 새 판은 아직)") && ct.LineNo(s.r.Out, "맥: ") < ct.LineNo(s.r.Out, "w1: "))
	s.run()
	s.ok("범용 저장소만 바뀜: 맥에 깖·기계에 씀", s.r.RC == 0 && s.has("맥 깖 .local/bin/hc-lane") && s.is(M+"/.local/bin/hc-lane", "#!/bin/sh\necho lane2\n") && s.is(W1+"/.local/bin/hc-lane", "#!/bin/sh\necho lane2\n"))

	// 8-2. push가 거절되면 멈추고, 다음 실행에서 그 커밋을 다시 보낸다
	s.putx(T+"/conf/.git/hooks/pre-push", "#!/bin/sh\n[ \"$HERDR_LAND\" = 1 ] && [ ! -e \""+T+"/nopush\" ]\n")
	s.put(M+"/.claude/docs/c.md", "c1\n")
	s.put(T+"/nopush", "")
	s.run()
	s.ok("push 거절: 종료 1, origin에 없음", s.r.RC == 1 && !ct.GitOK(T+"/conf.git", "cat-file", "-e", "main:claude/docs/c.md"))
	os.Remove(T + "/nopush")
	s.run()
	s.ok("push 거절 뒤 다시: 그 커밋을 보낸다", s.r.RC == 0 && s.corigin("show", "main:claude/docs/c.md") == "c1")

	// 9. 반쯤 쓴 파일이 없음: 전송이 깨지면 쓰지 않고 임시 파일도 남지 않는다
	s.put(M+"/.claude/docs/a.md", "a4 긴 내용\n")
	s.runEnv([]string{"HC_FAKE_CORRUPT=w1"})
	s.ok("깨진 전송: 쓰지 않음, 다름으로 보임 (설정 입력도 깨져 도구가 거절)", s.r.RC == 1 && s.is(W1+"/.claude/docs/a.md", "a3-mac\n") && s.has("w1: 다름 (주제지침·도구문제:설정)"))
	tmpLeft := false
	for _, d := range []string{T + "/mac", T + "/m", T + "/repo", T + "/conf"} {
		tmpLeft = tmpLeft || ct.FindName(d, "*.hsync.*")
	}
	s.ok("깨진 전송: 임시 파일 없음", !tmpLeft)
	s.run()
	s.ok("깨진 뒤 다시: 맞춘다", s.r.RC == 0 && s.is(W1+"/.claude/docs/a.md", "a4 긴 내용\n"))

	// 9-2. --pull repo: 저장소 값을 남긴다
	s.put(M+"/.claude/docs/a.md", "a5-mac\n")
	s.git(T+"/lane", "pull", "-q")
	s.lane("lane3", func() { s.put(T+"/lane/claude/docs/a.md", "a5-repo\n") })
	s.run("--pull", "repo:.claude/docs/a.md")
	s.ok("--pull repo: 저장소 값을 맥과 기계에", s.r.RC == 0 && s.is(M+"/.claude/docs/a.md", "a5-repo\n") && s.is(W1+"/.claude/docs/a.md", "a5-repo\n"))
	// 저장소에서 지운 파일은 맥에서 지우고(백업) 기계에는 둔다
	s.git(T+"/lane", "pull", "-q")
	s.git(T+"/lane", "rm", "-q", "claude/docs/b.md")
	s.git(T+"/lane", "commit", "-qm", "rm")
	s.git(T+"/lane", "push", "-q", "origin", "main")
	s.run()
	s.ok("저장소에서 지움: 맥에서 지우고(맥 지움 줄) 기계는 둔다", s.r.RC == 0 && !ct.Exists(M+"/.claude/docs/b.md") && ct.GrepRx(M+"/.local/state/herdr-sync/bak", "b1") && ct.Exists(W1+"/.claude/docs/b.md") && s.has("맥 지움 .claude/docs/b.md (저장소에서 지워짐)"))
	// 링크 자리에 폴더가 있으면 충돌, --take로 폴더를 백업 폴더로 옮기고 링크
	os.Remove(W1 + "/.claude/skills/herdr-desktop")
	s.put(W1+"/.claude/skills/herdr-desktop/mine", "")
	s.run()
	s.ok("링크 자리에 폴더: 충돌, 건드리지 않음", s.r.RC == 1 && ct.Exists(W1+"/.claude/skills/herdr-desktop/mine") && !ct.IsLink(W1+"/.claude/skills/herdr-desktop"))
	s.run("--take", "w1:.claude/skills/herdr-desktop@")
	s.ok("링크 --take: 폴더는 백업, 링크를 만든다", s.r.RC == 0 && ct.IsLink(W1+"/.claude/skills/herdr-desktop") && ct.FindName(W1+"/.local/state/herdr-sync/bak", "mine"))

	// 10. 연결 실패
	s.put(W1+"/OFFLINE", "")
	s.run("--check")
	s.ok("연결 실패: 그 기계는 다름, 종료 1", s.r.RC == 1 && s.has("w1: 다름 (연결)") && s.has("lx: 같음"))
	os.Remove(W1 + "/OFFLINE")

	s.otherOS()
	s.settingsAndRepos()
}

// 10-2. sh를 못 띄우는 기계(Windows 등)는 건너뛰고 종료 코드에 넣지 않는다.
// uname이 Darwin·Linux가 아닌 기계(Git sh의 MINGW64_NT 등)에는 --take·--pull로 집어도 아무것도 쓰지 않는다
func (s *sx) otherOS() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	NS, WG := T+"/m/ns", T+"/m/wg"
	s.put(NS+"/NOSH", "")
	s.putx(WG+"/.fakebin/uname", "#!/bin/sh\necho MINGW64_NT-10.0\n")
	s.put(WG+"/.claude/docs/a.md", "wg-own\n")
	wg0, mac0 := snapFL(WG), strings.TrimRight(ct.Read(M+"/.claude/docs/a.md"), "\n")
	s.put(M+"/.claude/docs/a.md", "a6\n")
	s.t.Setenv("HC_FAKE_MACHINES", `{"name":"w1"},{"name":"lx"},{"name":"ns"},{"name":"wg"}`)
	s.run("--check")
	s.ok("--check: sh 없는 기계·다른 OS는 건너뜀 한 줄씩", s.has("ns: 건너뜀 (sh 없음 — Windows 등)") && s.has("wg: 건너뜀 (OS MINGW64_NT-10.0)") && !s.contains("spawn_failed"))
	s.run()
	s.ok("sh 없는 기계: 건너뜀 한 줄, 종료 0, 다른 기계는 맞춘다", s.r.RC == 0 && s.has("ns: 건너뜀 (sh 없음 — Windows 등)") && !s.contains("ns 연결 실패") && s.is(W1+"/.claude/docs/a.md", "a6\n") && s.is(LX+"/.claude/docs/a.md", "a6\n"))
	s.ok("uname MINGW64_NT: 건너뜀 한 줄, 아무 파일도 안 씀", s.has("wg: 건너뜀 (OS MINGW64_NT-10.0)") && snapFL(WG) == wg0)
	s.run("--check")
	s.ok("--check: 다른 기계가 같으면 건너뛴 기계가 있어도 종료 0", s.r.RC == 0 && s.has("ns: 건너뜀 (sh 없음 — Windows 등)") && s.has("wg: 건너뜀 (OS MINGW64_NT-10.0)") && s.has("w1: 같음"))
	s.run("--take", "wg:*")
	s.ok("다른 OS: --take로 집어도 쓰지 않는다", s.r.RC == 0 && snapFL(WG) == wg0)
	s.run("--pull", "wg:.claude/docs/a.md")
	s.ok("다른 OS: --pull로 집어도 가져오지 않는다", s.r.RC == 0 && s.is(M+"/.claude/docs/a.md", "a6\n") && snapFL(WG) == wg0)
	s.run("--pull", "ns:.claude/docs/a.md")
	s.ok("sh 없는 기계: --pull은 건너뛰고 종료 0", s.r.RC == 0 && s.is(M+"/.claude/docs/a.md", "a6\n"))
	s.t.Setenv("HC_FAKE_MACHINES", `{"name":"w1"},{"name":"lx"}`)
	os.RemoveAll(NS)
	os.RemoveAll(WG)
	s.put(M+"/.claude/docs/a.md", mac0+"\n")
	s.run()
	s.ok("10-2 뒤 되돌림: 다시 같음", s.r.RC == 0 && s.is(W1+"/.claude/docs/a.md", mac0+"\n"))
}

func (s *sx) settingsAndRepos() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	// 11. 설정: 그 기계에서 따로 고친 키는 충돌, --take <기계>:<키>, 도구 문제는 충돌과 구분
	lxs := LX + "/.claude/settings.json"
	v, err := ojson.Parse([]byte(ct.Read(lxs)))
	if err != nil {
		s.t.Fatal(err)
	}
	v.(*ojson.Object).Set("model", "sonnet") // jq '.model = "sonnet"'
	s.put(lxs, ojson.Pretty(v))
	s.run()
	s.ok("설정 충돌: 충돌로 세고 쓰지 않는다", s.r.RC == 1 && s.has("충돌 lx .model") && s.has("lx: 충돌 1건") && jsonAt(lxs, "model") == "sonnet")
	s.run("--take", "lx:.model")
	s.ok("설정 --take: 맥 값으로", s.r.RC == 0 && s.has("lx 씀 .model") && jsonAt(lxs, "model") == "opus")
	keep := ct.Read(lxs)
	s.put(lxs, "{깨짐\n")
	s.run("--check")
	s.ok("설정 도구 문제: 충돌이 아니라 도구문제로", s.r.RC == 1 && s.has("lx: 다름 (도구문제:설정)") && !s.has("충돌 lx .model"))
	s.put(lxs, keep)
	s.run("--check")
	s.ok("설정 --check: 다 맞으면 같음", s.r.RC == 0 && s.has("lx: 같음"))

	// 12. 고른 기계만
	s.put(M+"/.claude/docs/b.md", "b2\n")
	s.run("lx")
	s.ok("고른 기계만: lx만 맞추고 w1은 그대로", s.r.RC == 0 && s.is(LX+"/.claude/docs/b.md", "b2\n") && s.is(W1+"/.claude/docs/b.md", "b1\n") && !ct.HasPrefixLine(s.r.Out, "w1:"))

	// 12-2. 어느 저장소든 커밋하지 않은 변경이 있으면 아무것도 하지 않고 멈춘다
	s.put(M+"/.claude/docs/b.md", "b3\n")
	before := s.snap()
	for _, f := range []string{"repo/bin/hc-lane", "conf/sync.list"} {
		ct.Append(s.t, T+"/"+f, "dirty\n")
		s.run()
		s.ok("더러운 저장소 ("+f+"): 멈추고 아무 데도 안 쓴다", s.r.RC == 1 && s.contains("커밋하지 않은 변경") && s.snap() == before)
		repo, path, _ := strings.Cut(f, "/")
		s.git(T+"/"+repo, "checkout", "-q", "--", path)
	}

	// 12-3. 범용 저장소로 모을 맥 값에 개인 값이 있으면 충돌, 아무 데도 안 쓴다 (개인 저장소로 가는 파일은 괜찮다)
	s.run()
	bal, _ := os.ReadFile(M + "/.local/bin/herdr-balance")
	s.putx(M+"/.local/bin/herdr-balance", "#!/bin/sh\necho secret-word\n")
	s.put(M+"/.claude/docs/d.md", "secret-word 는 개인 저장소엔 괜찮다\n")
	before, h0 := s.snap(), s.heads()
	s.run("--check")
	s.ok("개인 값 --check: 충돌로 보인다", s.r.RC == 1 && s.has("충돌 저장소 .local/bin/herdr-balance (개인 값)") && s.has("저장소 다름:모음 .claude/docs/d.md"))
	s.ok("맥이 저장소보다 새로움 --check: 맥 줄에 보이고, 기계 줄에 맥 판 표시 없음", s.has("맥: 저장소보다 새로움 1개 (claude/docs/d.md) — 커밋 안 됨? · 충돌 1건") && ct.HasPrefixLine(s.r.Out, "w1: ") && !s.contains("맥 판 기준"))
	s.run()
	s.ok("개인 값: 종료 1, 두 저장소·맥·기계 그대로", s.r.RC == 1 && s.has("충돌 저장소 .local/bin/herdr-balance (개인 값)") && s.heads() == h0 && s.snap() == before)
	s.run("--take", "repo:.local/bin/herdr-balance")
	s.ok("개인 값: --take repo로도 범용 저장소에 넣지 않는다", s.r.RC == 1 && s.heads() == h0)
	// 패턴 파일이 없으면(또는 패턴 줄이 없으면) 검사를 못 하니 범용으로 모으지 않는다(fail-closed)
	s.putx(M+"/.local/bin/herdr-balance", "#!/bin/sh\necho bal2\n")
	for _, pat := range []string{"# 주석만\n", "-"} {
		np := T + "/conf-np"
		s.cpR(T+"/conf", np)
		name := "-"
		if pat == "-" {
			s.git(np, "rm", "-q", "private-patterns.txt")
		} else {
			name = strings.TrimSuffix(pat, "\n")
			s.put(np+"/private-patterns.txt", pat)
			s.git(np, "add", "private-patterns.txt")
		}
		s.git(np, "commit", "-qm", "np")
		s.runEnv([]string{"HERDR_TOOLS_CONFIG=" + np})
		s.ok("패턴 없음("+name+"): 범용 모음은 충돌, 범용 origin 그대로", s.r.RC == 1 && s.has("충돌 저장소 .local/bin/herdr-balance (개인 값 검사 못 함 — "+np+"/private-patterns.txt에 패턴이 없다)") && s.origin("rev-parse", "main") == strings.Fields(h0)[0])
	}
	os.RemoveAll(T + "/conf-np")
	s.putx(M+"/.local/bin/herdr-balance", string(bal))
	s.run()
	s.ok("개인 값을 뺀 뒤: 개인 저장소 파일은 모은다", s.r.RC == 0 && strings.Contains(s.corigin("show", "main:claude/docs/d.md"), "secret-word") && !strings.Contains(s.origin("log", "-p", "main"), "secret-word"))
}
