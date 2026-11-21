package main

// herdr-sync 계약 시험 1~4: 맥 확인·개인 저장소 없음·옛 묶음 이름·prebuild, 처음 --check·맞추기, 충돌과 --take

import (
	"os"
	"regexp"
	"strings"

	ct "herdr-tools/internal/clitest"
)

func (s *sx) firstRuns() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	// 1. 맥에서만
	s.runEnv([]string{"HOME=" + W1}, "--check")
	s.ok("맥 아님: 종료 2와 「맥에서만」", s.r.RC == 2 && s.contains("맥에서만"))
	s.runEnv([]string{"HC_FAKE_MACHINES="}, "--check")
	s.ok("기계를 모르는 hc: 종료 2", s.r.RC == 2)
	s.run("--check", "--take", "w1:x")
	s.ok("잘못 부름(--check와 --take): 종료 2", s.r.RC == 2)
	s.run("--take", "w1:.claude/docs/zzz.md")
	s.ok("sync.list에 없는 경로: 종료 1", s.r.RC == 1 && s.contains("없는 경로"))
	os.MkdirAll(T+"/sys/Herdr.app", 0o755)
	s.runEnv([]string{"HOME=" + W1, "HERDR_SYNC_SYSAPPS=" + T + "/sys", "HERDR_TOOLS_CONFIG=" + T + "/none"}, "--check")
	s.ok("맥 확인: /Applications/Herdr.app도 받는다 (다음 관문 「개인 저장소」까지 감)", s.r.RC == 2 && s.contains("개인 저장소가 없다"))

	// 1-2. 개인 저장소가 없으면 종료 2, 아무 데도 쓰지 않는다(범용 저장소 pull도 하지 않는다)
	before, h0 := s.snap(), s.heads()
	for _, a := range [][]string{{"--check"}, nil} {
		s.runEnv([]string{"HERDR_TOOLS_CONFIG=" + T + "/none"}, a...)
		name := "맞추기"
		if a != nil {
			name = a[0]
		}
		s.ok("개인 저장소 없음 ("+name+"): 종료 2와 안내", s.r.RC == 2 && s.contains("개인 저장소가 없다: "+T+"/none") && s.contains("config.example"))
	}
	s.ok("개인 저장소 없음: 기계·맥·두 저장소 그대로", s.snap() == before && s.heads() == h0)

	// 1-3. 옛 묶음 이름(all9·mac7·mac-only)은 종료 2 (옛 코드는 기계 이름으로 읽어 「같음」으로 보였다)
	group := regexp.MustCompile(` (all|macos|local)$`)
	for _, o := range []string{"all9", "mac7", "mac-only"} {
		s.cpR(T+"/conf", T+"/conf-old")
		s.editLines(T+"/conf-old/sync.list", func(l string) (string, bool) { return group.ReplaceAllString(l, " "+o), true })
		s.git(T+"/conf-old", "commit", "-qam", "old")
		s.runEnv([]string{"HERDR_TOOLS_CONFIG=" + T + "/conf-old"})
		s.ok("옛 묶음 이름 "+o+": 종료 2와 새 이름 안내, 아무 데도 안 씀", s.r.RC == 2 && s.contains("옛 묶음 이름 "+o) && s.snap() == before && s.heads() == h0)
	}
	os.RemoveAll(T + "/conf-old")

	// 1-4. prebuild는 개인 설정 sync.prebuild를 sh -c로 돌린다. 실패하면 맞추지 않고 멈춘다
	s.cpR(T+"/conf", T+"/conf-bad")
	s.put(T+"/conf-bad/config", "sync.prebuild  echo built > \"$HOME/prebuilt\"; exit 3\n")
	s.git(T+"/conf-bad", "commit", "-qam", "bad")
	s.runEnv([]string{"HERDR_TOOLS_CONFIG=" + T + "/conf-bad"})
	hf := strings.Fields(h0)
	s.ok("prebuild: sh -c로 돌고, 실패하면 종료 1·저장소에 안 씀", s.r.RC == 1 && s.contains("sync.prebuild 실패") && s.is(M+"/prebuilt", "built\n") && s.origin("rev-parse", "main")+" "+s.corigin("rev-parse", "main") == hf[0]+" "+hf[1])
	os.RemoveAll(T + "/conf-bad")
	os.Remove(M + "/prebuilt")

	// 2. 처음 --check: 아무것도 고치지 않는다
	before, h0 = s.snap(), s.heads()
	s.run("--check")
	s.ok("처음 --check: 종료 1", s.r.RC == 1)
	s.ok("처음 --check: 기계·맥·저장소 그대로", s.snap() == before && s.heads() == h0 && ct.Git(T+"/repo", "status", "--porcelain")+ct.Git(T+"/conf", "status", "--porcelain") == "")
	s.ok("처음 --check: 저장소로 모을 것", s.has("저장소 다름:모음 .claude/docs/a.md") && s.has("저장소 다름:깖 .local/bin/hc-lane"))
	s.ok("처음 --check: base 없이 다르면 충돌", s.has("충돌 w1 .claude/docs/b.md (2줄 다름)") && s.has("충돌 lx .claude/CLAUDE.md#body (2줄 다름)"))
	s.ok("처음 --check: 표식 없는 Codex 지침도 충돌", ct.HasPrefixLine(s.r.Out, "충돌 lx .codex/AGENTS.md#codex-shared "))
	s.ok("처음 --check: 기계마다 한 줄", s.rx(`^w1: .*충돌 1건`) && s.rx(`^lx: .*충돌 2건`) && s.has("맥: 저장소가 새로움 3개 (bin/hc-lane bin/herdr-settings-merge settings/keys.json) — herdr-sync로 깐다 · 저장소보다 새로움 7개 (claude/CLAUDE.body.md claude/docs/a.md claude/docs/b.md …) — 커밋 안 됨? · 다름 (지움)"))

	// 3. 처음 맞추기
	s.run()
	s.ok("처음: 충돌이 남아 종료 1", s.r.RC == 1)
	s.ok("모음: 맥 → 개인 저장소 커밋·push (HERDR_LAND=1)", strings.Contains(s.corigin("log", "-1", "--format=%s", "main"), "herdr-sync 모음") && s.corigin("show", "main:claude/docs/a.md") == "a1" && s.corigin("show", "main:claude/CLAUDE.body.md") == "본문 1")
	s.ok("모음: Codex 구역은 prebuild로 다시 만든 값", s.corigin("show", "main:codex/AGENTS.shared.md") == "codex built")
	s.ok("모음: tools: 줄은 범용 저장소로 따로 커밋, 실행 권한도", strings.Contains(s.origin("log", "-1", "--format=%s", "main"), "herdr-sync 모음: bin/herdr-balance") && strings.HasPrefix(s.origin("ls-tree", "main", "bin/herdr-balance"), "100755"))
	s.ok("모음: 두 저장소가 섞이지 않는다", !ct.GitOK(T+"/origin.git", "cat-file", "-e", "main:claude/docs/a.md") && !ct.GitOK(T+"/conf.git", "cat-file", "-e", "main:bin/herdr-balance"))
	s.ok("깖: 저장소 → 맥, 실행 권한 따름", s.is(M+"/.local/bin/hc-lane", "#!/bin/sh\necho lane\n") && ct.IsExec(M+"/.local/bin/hc-lane"))
	s.ok("씀: 워커 맥에 도구·스킬, 실행 권한", ct.IsExec(W1+"/.local/bin/hc-lane") && s.is(W1+"/.agents/skills/herdr-desktop/SKILL.md", "skill 1\n") && s.has("w1 씀 .local/bin/hc-lane"))
	s.ok("씀: 링크를 만든다", ct.Readlink(W1+"/.claude/skills/herdr-desktop") == W1+"/.agents/skills/herdr-desktop")
	s.ok("같음: OMC 블록은 그 기계 것 그대로", s.is(W1+"/.claude/CLAUDE.md", "OMC w1\n<!-- User customizations -->\n본문 1\n"))
	s.ok("충돌: 쓰지 않는다", s.is(W1+"/.claude/docs/b.md", "b-old\n") && s.is(LX+"/.claude/CLAUDE.md", "OMC lx\n<!-- User customizations -->\n본문 0\n"))
	s.ok("리눅스: all9만 받는다", s.is(LX+"/.claude/docs/a.md", "a1\n") && ct.IsExec(LX+"/.local/bin/herdr-settings-merge") && !ct.Exists(LX+"/.local/bin/hc-lane") && !ct.Exists(LX+"/.agents/skills/herdr-desktop"))
	s.ok("Codex: 표식 없는 지침은 쓰지 않는다(충돌)", s.is(LX+"/.codex/AGENTS.md", "lx 머리\n<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n꼬리\n"))
	s.ok("Codex: 지침 없는 기계는 건너뛴다", !ct.Exists(W1+"/.codex"))
	s.ok("맥 전용은 기계에 안 간다", !ct.Exists(W1+"/.local/bin/herdr-balance") && !ct.Exists(W1+"/.local/share/claude-md"))
	s.ok("지움: 워커 맥은 백업으로 옮긴다, 리눅스는 mac7이라 둔다", !ct.Exists(W1+"/.agents/skills/herdr-ovis") && s.is(ct.Glob1(W1+"/.local/state/herdr-sync/bak/*/.agents/skills/herdr-ovis/SKILL.md"), "ovis\n") && ct.Exists(LX+"/.agents/skills/herdr-ovis"))
	s.ok("지움: 맥도", !ct.Exists(M+"/.local/bin/claude-md-sync") && ct.Glob1(M+"/.local/state/herdr-sync/bak/*/.local/bin/claude-md-sync") != "" && s.has("맥 지움 .local/bin/claude-md-sync"))
	s.ok("설정: 예외 키는 그 기계 값, 정한 키 밖은 그대로", jsonAt(LX+"/.claude/settings.json", "effortLevel") == "medium" && s.has("lx 예외 .effortLevel") && jsonAt(W1+"/.claude/settings.json", "theme") == "w1")
	s.ok("설정: 정한 훅만 더하고 맥 $HOME은 대상 $HOME으로, 다른 훅은 그대로", hookCommands(LX+"/.claude/settings.json") == `["moshi-lx.sh","`+LX+`/.claude/hooks/cotenant-guard.sh"]` && s.has("lx 씀 hook:PreToolUse:cotenant-guard.sh"))
	s.ok("설정: 참고 차이는 보인다", s.has("lx 참고 .permissions"))
	s.ok("base: 기계마다 남는다", ct.HasPrefixLine(ct.Read(W1+"/.local/state/herdr-sync/base.tsv"), ".claude/CLAUDE.md#body\t") && ct.HasPrefixLine(ct.Read(LX+"/.local/state/herdr-sync/base.tsv"), ".claude/docs/a.md\t"))

	// 4. 다시 돌려도 충돌은 그대로, --take로 맥 값
	s.run()
	s.ok("다시: 충돌은 쓰지 않는다", s.r.RC == 1 && s.is(W1+"/.claude/docs/b.md", "b-old\n"))
	s.run("--take", "w1:.claude/docs/b.md")
	s.ok("--take: 맥 값으로 덮고 백업", s.is(W1+"/.claude/docs/b.md", "b1\n") && ct.GrepRx(W1+"/.local/state/herdr-sync/bak", "b-old") && s.has("w1: 같음"))
	s.run("--take", "lx:.codex/AGENTS.md#codex-shared")
	s.ok("--take: 표식 없는 Codex 지침에 구역을 넣고 cbm 블록은 남긴다(백업)", s.is(LX+"/.codex/AGENTS.md", "<!-- codebase-memory-mcp:start -->\ncbm\n<!-- codebase-memory-mcp:end -->\n\n<!-- codex-shared:start -->\ncodex built\n<!-- codex-shared:end -->\n") && ct.GrepRx(LX+"/.local/state/herdr-sync/bak", "꼬리"))
}
