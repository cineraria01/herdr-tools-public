package main

// herdr-sync 계약 시험 15: 나무 줄(<폴더>/**) — 모음·씀·걷지 않는 것·hc 호출 수·--take·--pull·깖·깨진 전송·8MB 나누기·
// 줄 옮김·.gitignore·커밋 실패 되돌리기·묵은 임시 폴더·옛 꼴 repo-base

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	ct "herdr-tools/internal/clitest"
)

func (s *sx) trees() {
	T, M, W1, LX := s.T, s.MAC, s.W1, s.LX
	// 15. 나무 줄: <폴더>/** = 그 아래 모든 층의 파일. 링크·숨김·__pycache__·node_modules·제외·50MB 넘음은 걷지 않는다
	s.git(T+"/lane", "pull", "-q")
	s.lane("tree", func() {
		ct.Append(s.t, T+"/lane/sync.list", `스킬나무 .agents/skills/**             skills/agents/          macos
스킬나무 .claude/skills/**             skills/claude/          all
제외     .agents/skills/app-managed    -                       -
링크     .claude/skills-alias/beta@    .claude/skills/beta     all
링크     .claude/skills-alias/gone@    .agents/skills/gone     all
`)
	})
	s.git(T+"/conf", "pull", "-q") // herdr-sync가 처음에 하는 pull을 미리 (아래 heads 비교가 그것을 쓰기로 보지 않게)
	A, C := M+"/.agents/skills", M+"/.claude/skills"
	s.A, s.C = A, C
	s.put(A+"/alpha/SKILL.md", "alpha 1\n")
	s.putx(A+"/alpha/scripts/run.sh", "#!/bin/sh\necho run\n")
	s.put(A+"/alpha/ref/deep/er/notes.md", "deep 1\n")
	s.put(A+"/alpha/한 글.md", "space 1\n")
	s.put(A+"/alpha/.hidden", "h\n")
	s.put(A+"/alpha/.git/config", "g\n")
	s.put(A+"/alpha/__pycache__/x.pyc", "p\n")
	s.put(A+"/alpha/node_modules/m/index.js", "n\n")
	os.Symlink("SKILL.md", A+"/alpha/link.md")
	s.put(M+"/elsewhere/in.md", "e\n")
	os.Symlink(M+"/elsewhere", A+"/linked")
	s.put(A+"/app-managed/SKILL.md", "app mac\n")
	s.put(A+"/gamma/SKILL.md", "gamma 1\n")
	s.put(A+"/big/ok.md", "ok\n")
	s.put(A+"/big/huge.bin", "")
	os.Truncate(A+"/big/huge.bin", 7340033)
	s.put(C+"/beta/SKILL.md", "beta 1\n")
	s.put(C+"/beta/sub/x.md", "x\n")
	// w1: 손댄 파일(base 없음, 내용 다름), 내용은 같고 실행 권한만 다른 파일, 그 기계에만 있는 파일, 제외 경로, 링크로 된 폴더
	W1A := W1 + "/.agents/skills"
	s.W1A = W1A
	s.put(W1A+"/alpha/SKILL.md", "alpha w1\n")
	s.put(W1A+"/alpha/scripts/run.sh", "#!/bin/sh\necho run\n")
	s.put(W1A+"/w1only/SKILL.md", "mine\n")
	s.put(W1A+"/app-managed/SKILL.md", "app w1\n")
	os.MkdirAll(W1+"/elsewhere", 0o755)
	os.Symlink(W1+"/elsewhere", W1A+"/gamma")
	before, h0 := s.snap(), s.heads()
	s.run("--check")
	s.ok("나무 --check: 아무 데도 안 쓴다", s.r.RC == 1 && s.snap() == before && s.heads() == h0 && s.has("저장소 다름:모음 .agents/skills/alpha/ref/deep/er/notes.md"))
	s.run()
	skills := func() string {
		ls := ct.Lines(ct.Git(T+"/conf.git", "-c", "core.quotePath=false", "ls-tree", "-r", "--name-only", "main", "skills/"))
		sort.Strings(ls) // LC_ALL=C sort
		return strings.Join(ls, " ") + " "
	}
	s.ok("나무 모음: 하위 폴더 모든 층, 걷지 않는 것은 빼고, 따로 적은 줄과 겹치면 그 줄이 맡는다", skills() == "skills/agents/alpha/SKILL.md skills/agents/alpha/ref/deep/er/notes.md skills/agents/alpha/scripts/run.sh skills/agents/alpha/한 글.md skills/agents/big/ok.md skills/agents/gamma/SKILL.md skills/claude/beta/SKILL.md skills/claude/beta/sub/x.md skills/herdr-desktop/SKILL.md ")
	s.ok("나무 모음: 실행 권한", strings.HasPrefix(s.corigin("ls-tree", "main", "skills/agents/alpha/scripts/run.sh"), "100755"))
	s.ok("7MB 넘음: 건너뜀 한 줄, 아무 데도 안 감", s.has("건너뜀 .agents/skills/big/huge.bin (7MB 넘음)") && !ct.Exists(W1A+"/big/huge.bin") && s.is(W1A+"/big/ok.md", "ok\n"))
	s.ok("나무 씀: 새 파일, 여러 층, 빈칸·한글 이름", s.is(W1A+"/alpha/ref/deep/er/notes.md", "deep 1\n") && s.is(W1A+"/alpha/한 글.md", "space 1\n") && s.has("w1 씀 .agents/skills/alpha/한 글.md"))
	s.ok("나무 씀: 실행 권한만 다르면 충돌이 아니라 씀", ct.IsExec(W1A+"/alpha/scripts/run.sh") && !s.has("충돌 w1 .agents/skills/alpha/scripts/run.sh (0줄 다름)") && s.has("w1 씀 .agents/skills/alpha/scripts/run.sh"))
	s.ok("걷지 않음: 숨김·.git·__pycache__·node_modules·링크", !ct.Exists(W1A+"/alpha/.hidden") && !ct.Exists(W1A+"/alpha/.git") && !ct.Exists(W1A+"/alpha/__pycache__") && !ct.Exists(W1A+"/alpha/node_modules") && !ct.Exists(W1A+"/alpha/link.md") && !ct.Exists(W1A+"/linked"))
	s.ok("제외 줄: 그 아래는 안 걷고 기계에 있는 것도 그대로", s.is(W1A+"/app-managed/SKILL.md", "app w1\n") && !ct.GitOK(T+"/conf.git", "cat-file", "-e", "main:skills/agents/app-managed/SKILL.md"))
	s.ok("그 기계에만 있는 파일: 그대로, 표에도 안 셈", s.is(W1A+"/w1only/SKILL.md", "mine\n") && !s.contains("w1only"))
	s.ok("그 기계에서 손댄 파일: 충돌, 덮지 않음", s.has("충돌 w1 .agents/skills/alpha/SKILL.md (2줄 다름)") && s.is(W1A+"/alpha/SKILL.md", "alpha w1\n") && s.has("w1: 충돌 1건"))
	s.ok("그 기계에서 링크인 폴더: 따라가지 않고 건너뜀", s.has("건너뜀 w1 .agents/skills/gamma (링크 — 따라가지 않음)") && !ct.Exists(W1+"/elsewhere/SKILL.md"))
	s.ok("리눅스: all 나무만, dash로 씀", s.is(LX+"/.claude/skills/beta/sub/x.md", "x\n") && !ct.Exists(LX+"/.agents/skills/alpha") && s.has("lx: 같음"))
	s.ok("링크: 가리킬 곳을 이번에 썼으면 같은 실행에서 만든다", ct.Readlink(LX+"/.claude/skills-alias/beta") == LX+"/.claude/skills/beta" && ct.Readlink(W1+"/.claude/skills-alias/beta") == W1+"/.claude/skills/beta")
	s.ok("링크: 가리킬 곳이 없으면 만들지 않고 건너뜀", s.has("건너뜀 lx .claude/skills-alias/gone@ (가리킬 곳 없음)") && !ct.IsLink(LX+"/.claude/skills-alias/gone") && !ct.IsLink(W1+"/.claude/skills-alias/gone"))
	s.ok("나무 base: 파일마다 남는다", ct.HasPrefixLine(ct.Read(W1+"/.local/state/herdr-sync/base.tsv"), ".agents/skills/alpha/ref/deep/er/notes.md\t") && ct.HasPrefixLine(ct.Read(M+"/.local/state/herdr-sync/repo-base.tsv"), ".agents/skills/alpha/ref/deep/er/notes.md\t"))
	s.run("--check")
	s.ok("나무 --check: 남은 것은 그 충돌 하나", s.r.RC == 1 && s.has("w1: 충돌 1건") && s.has("lx: 같음") && s.has("맥: 저장소와 같음"))

	// 15-2. 기계당 hc 호출 수는 나무 파일 수와 상관없다
	calls := func() string {
		s.run()
		return fmt.Sprintf("%d %d", countLine(T+"/calls", "w1"), countLine(T+"/calls", "lx"))
	}
	s.put(C+"/few/a.md", "a\n")
	s.put(C+"/few/b/c.md", "c\n")
	os.Remove(T + "/calls")
	few := calls()
	for x := 1; x <= 40; x++ {
		s.put(fmt.Sprintf("%s/many/d%d/f%d.md", C, x%4, x), fmt.Sprintf("f%d\n", x))
	}
	os.Remove(T + "/calls")
	many := calls()
	s.ok("hc 호출 수: 새 파일 2개 ("+few+")와 40개 ("+many+")가 같다", few == many && s.is(LX+"/.claude/skills/many/d1/f9.md", "f9\n"))
	s.put(C+"/many/d0/f4.md", "f4 바뀜\n")
	os.Remove(T + "/calls")
	one := calls()
	s.ok("hc 호출 수: 바뀐 파일 하나도 같은 꼴 ("+one+")", one == many && s.is(W1+"/.claude/skills/many/d0/f4.md", "f4 바뀜\n"))

	// 15-3. --take: 파일 하나, <폴더>/**
	s.run("--take", "w1:.nope/**")
	s.ok("--take 없는 나무: 종료 1", s.r.RC == 1 && s.contains("없는 경로"))
	s.run("--take", "w1:.agents/skills/alpha/SKILL.md")
	s.ok("--take 나무 파일 하나: 맥 값, 백업", s.r.RC == 0 && s.is(W1A+"/alpha/SKILL.md", "alpha 1\n") && ct.GrepRx(W1+"/.local/state/herdr-sync/bak", "alpha w1"))
	s.put(W1A+"/alpha/ref/deep/er/notes.md", "w1 deep\n")
	s.put(W1A+"/alpha/한 글.md", "w1 space\n")
	s.run()
	s.ok("손댄 나무 파일 둘: 충돌 2건, 그대로", s.r.RC == 1 && s.has("w1: 충돌 2건") && s.is(W1A+"/alpha/한 글.md", "w1 space\n"))
	s.run("--take", "w1:.agents/skills/**")
	s.ok("--take 기계:<폴더>/**: 그 나무 충돌 전부", s.r.RC == 0 && s.is(W1A+"/alpha/ref/deep/er/notes.md", "deep 1\n") && s.is(W1A+"/alpha/한 글.md", "space 1\n"))

	// 15-4. --pull: 파일 하나, <폴더>/**
	s.put(W1A+"/alpha/SKILL.md", "alpha from w1\n")
	s.run("--pull", "w1:.agents/skills/alpha/SKILL.md")
	s.ok("--pull 나무 파일 하나: 맥·저장소로", s.r.RC == 0 && s.is(A+"/alpha/SKILL.md", "alpha from w1\n") && s.corigin("show", "main:skills/agents/alpha/SKILL.md") == "alpha from w1")
	s.put(W1A+"/alpha/ref/deep/er/notes.md", "w1 deep 2\n")
	s.put(W1A+"/alpha/한 글.md", "w1 space 2\n")
	s.run("--pull", "w1:.agents/skills/**")
	s.ok("--pull 기계:<폴더>/**: 그 나무 충돌 전부를 맥·저장소로", s.r.RC == 0 && s.is(A+"/alpha/한 글.md", "w1 space 2\n") && s.corigin("show", "main:skills/agents/alpha/ref/deep/er/notes.md") == "w1 deep 2" && s.is(W1A+"/w1only/SKILL.md", "mine\n"))

	// 15-5. 저장소만 바뀜(레인 착지) → 맥에 깖, 기계까지. 맥만 바뀜 → 저장소로. 실행 권한
	s.git(T+"/lane", "pull", "-q")
	s.put(T+"/lane/skills/agents/alpha/ref/deep/er/notes.md", "lane deep\n")
	s.put(T+"/lane/skills/agents/alpha/new/from-lane.md", "lane new\n")
	s.git(T+"/lane", "add", "--", "skills")
	s.git(T+"/lane", "commit", "-qm", "lane-tree")
	s.git(T+"/lane", "push", "-q", "origin", "main")
	s.run()
	s.ok("나무 깖: 바뀐 파일·새 폴더의 새 파일이 맥과 기계에", s.r.RC == 0 && s.is(A+"/alpha/ref/deep/er/notes.md", "lane deep\n") && s.is(A+"/alpha/new/from-lane.md", "lane new\n") && s.is(W1A+"/alpha/new/from-lane.md", "lane new\n") && s.has("맥 깖 .agents/skills/alpha/new/from-lane.md"))
	ct.Chmod(s.t, A+"/alpha/scripts/run.sh", 0o644)
	s.put(C+"/beta/SKILL.md", "beta 2\n")
	s.run()
	s.ok("나무: 맥만 바뀐 내용과 실행 권한이 저장소·기계로", s.r.RC == 0 && strings.HasPrefix(s.corigin("ls-tree", "main", "skills/agents/alpha/scripts/run.sh"), "100644") && !ct.IsExec(W1A+"/alpha/scripts/run.sh") && s.corigin("show", "main:skills/claude/beta/SKILL.md") == "beta 2" && s.is(LX+"/.claude/skills/beta/SKILL.md", "beta 2\n"))
	os.Remove(A + "/gamma/SKILL.md")
	s.run()
	s.ok("나무: 맥에서 지운 파일은 저장소에서 지우고 기계는 둔다", s.r.RC == 0 && !ct.GitOK(T+"/conf.git", "cat-file", "-e", "main:skills/agents/gamma/SKILL.md"))

	// 15-6. 깨진 전송: 나무 파일을 쓰지 않고 임시 폴더도 남기지 않는다
	s.put(C+"/beta/SKILL.md", "beta 3\n")
	s.runEnv([]string{"HC_FAKE_CORRUPT=w1"})
	s.ok("나무 깨진 전송: w1은 그대로, lx는 씀", s.r.RC == 1 && s.is(W1+"/.claude/skills/beta/SKILL.md", "beta 2\n") && s.is(LX+"/.claude/skills/beta/SKILL.md", "beta 3\n"))
	s.ok("나무 깨진 전송: 받는 쪽 임시 폴더 없음", !s.findPath(T+"/m", "/herdr-sync/w.") && !s.findPath(M, "/herdr-sync/w."))
	s.run()
	s.ok("깨진 뒤 다시: 맞춘다", s.r.RC == 0 && s.is(W1+"/.claude/skills/beta/SKILL.md", "beta 3\n"))

	// 15-7. hc는 입력·출력을 8MB에서 자른다: 나무 쓰기·충돌 읽기·가져오기를 7MB 아래 묶음 여러 번으로 나눈다
	heavy := func(p, id string) {
		var b strings.Builder
		for i := 0; i < 100000; i++ {
			fmt.Fprintf(&b, "%s line %07d xxxxxxxxxxxxxxxxxxxx\n", id, i)
		}
		s.put(p, b.String())
	}
	for x := 1; x <= 3; x++ { // 3.4MB씩, 합 10MB
		heavy(fmt.Sprintf("%s/heavy/h%d.md", C, x), fmt.Sprintf("h%d", x))
	}
	same := func(a, b string) bool {
		x, e1 := os.ReadFile(a)
		y, e2 := os.ReadFile(b)
		return e1 == nil && e2 == nil && bytes.Equal(x, y)
	}
	os.Remove(T + "/calls")
	s.run()
	s.ok("큰 묶음(10MB): 나눠 써서 w1·lx에 다 간다", s.r.RC == 0 && same(C+"/heavy/h3.md", W1+"/.claude/skills/heavy/h3.md") && same(C+"/heavy/h1.md", LX+"/.claude/skills/heavy/h1.md") && countLine(T+"/calls", "w1") >= 4)
	for x := 1; x <= 3; x++ {
		ct.Append(s.t, fmt.Sprintf("%s/.claude/skills/heavy/h%d.md", W1, x), "w1 덧붙임\n")
	}
	s.run()
	s.ok("큰 충돌 내용 읽기도 나눈다: 줄 수가 맞다", s.r.RC == 1 && s.has("충돌 w1 .claude/skills/heavy/h1.md (1줄 다름)") && s.has("충돌 w1 .claude/skills/heavy/h3.md (1줄 다름)"))
	s.run("--pull", "w1:.claude/skills/**", "w1", "lx") // lx까지 퍼뜨린다(기계 인자 없이는 w1만 — ht-sync-arg-guards)
	h2 := ct.Lines(ct.Read(C + "/heavy/h2.md"))
	s.ok("큰 --pull도 나눠 받는다", s.r.RC == 0 && same(W1+"/.claude/skills/heavy/h3.md", C+"/heavy/h3.md") && len(h2) > 0 && h2[len(h2)-1] == "w1 덧붙임" && same(C+"/heavy/h2.md", LX+"/.claude/skills/heavy/h2.md"))

	s.treeMoves()
}

// findPath: find root -path "*<조각>*" 이 있는지
func (s *sx) findPath(root, frag string) bool {
	found := false
	for _, p := range walkAll(root) {
		if strings.Contains(p, frag) {
			found = true
		}
	}
	return found
}

func walkAll(root string) []string {
	var out []string
	var walk func(string)
	walk = func(d string) {
		es, _ := os.ReadDir(d)
		for _, e := range es {
			p := d + "/" + e.Name()
			out = append(out, p)
			if e.IsDir() {
				walk(p)
			}
		}
	}
	walk(root)
	return out
}

// 15-8~15-12
func (s *sx) treeMoves() {
	T, A, W1, W1A := s.T, s.A, s.W1, s.W1A
	// 15-8. 줄을 옮겨도 맥 파일을 지우지 않는다 (repo-base에 저장소 경로를 함께 적는다)
	s.git(T+"/lane", "pull", "-q")
	s.lane("s1", func() {
		s.editLines(T+"/lane/sync.list", func(l string) (string, bool) {
			return l, !strings.HasPrefix(l, "스킬     .agents/skills/herdr-desktop/SKILL.md")
		})
	})
	s.run()
	s.ok("S1 따로 적은 줄을 지움: 나무가 맡아 저장소로 모은다, 맥 파일 그대로", s.r.RC == 0 && s.is(A+"/herdr-desktop/SKILL.md", "skill 1\n") && s.has("맥 모음 .agents/skills/herdr-desktop/SKILL.md") && s.corigin("show", "main:skills/agents/herdr-desktop/SKILL.md") == "skill 1" && !ct.HasPrefixLine(s.r.Out, "맥 지움"))
	s.git(T+"/lane", "pull", "-q")
	sub := func(from, to string) func() {
		return func() {
			s.editLines(T+"/lane/sync.list", func(l string) (string, bool) { return strings.Replace(l, from, to, 1), true })
		}
	}
	s.lane("s2", sub("skills/agents/  ", "skills/agents2/ "))
	s.run()
	s.ok("S2 나무의 저장소 폴더를 바꿈: 새 폴더로 모은다, 맥 파일 그대로", s.r.RC == 0 && s.is(A+"/alpha/한 글.md", "w1 space 2\n") && ct.Exists(A+"/alpha/new/from-lane.md") && s.corigin("show", "main:skills/agents2/alpha/SKILL.md") == "alpha from w1" && !ct.HasPrefixLine(s.r.Out, "맥 지움"))

	// 15-9. 저장소 .gitignore에 걸리는 나무 파일은 맥·저장소·기계 모두에서 뺀다
	s.git(T+"/lane", "pull", "-q")
	s.put(T+"/lane/.gitignore", "*.bak-*\n")
	s.git(T+"/lane", "add", ".gitignore")
	s.git(T+"/lane", "commit", "-qm", "ign")
	s.git(T+"/lane", "push", "-q", "origin", "main")
	s.put(A+"/alpha/SKILL.md.bak-1", "old\n")
	s.run()
	s.ok(".gitignore: 건너뜀 줄, 저장소 깨끗, 기계에 안 감", s.r.RC == 0 && s.has("건너뜀 .agents/skills/alpha/SKILL.md.bak-1 (저장소 .gitignore)") && ct.Git(T+"/conf", "status", "--porcelain") == "" && !ct.Exists(W1A+"/alpha/SKILL.md.bak-1"))
	// 커밋이 실패하면 이번에 스테이징·쓴 것을 되돌리고, 「맥 모음」 줄을 찍지 않는다. 다음 실행은 막히지 않는다
	s.putx(T+"/conf/.git/hooks/pre-commit", "#!/bin/sh\nexit 1\n")
	s.put(A+"/alpha/SKILL.md", "alpha 2\n")
	s.put(A+"/zeta/new.md", "zeta\n")
	s.run()
	s.ok("커밋 실패: 종료 1, 저장소 깨끗, 「맥 모음」 없음", s.r.RC == 1 && ct.Git(T+"/conf", "status", "--porcelain") == "" && !ct.HasPrefixLine(s.r.Out, "맥 모음"))
	os.Remove(T + "/conf/.git/hooks/pre-commit")
	s.run()
	s.ok("커밋 실패 뒤 다시: 모은다", s.r.RC == 0 && s.corigin("show", "main:skills/agents2/zeta/new.md") == "zeta" && s.is(W1A+"/alpha/SKILL.md", "alpha 2\n"))

	// 15-10. 받는 쪽 W는 끝난 pid의 w.* 임시 폴더를 지운다
	os.MkdirAll(W1+"/.local/state/herdr-sync/w.99999999/x", 0o755)
	s.put(A+"/zeta/new.md", "zeta 2\n")
	s.run()
	s.ok("묵은 w.* 임시 폴더를 지운다", s.r.RC == 0 && !ct.Exists(W1+"/.local/state/herdr-sync/w.99999999") && s.is(W1A+"/zeta/new.md", "zeta 2\n"))

	// 15-11. 옛 꼴 repo-base(셋째 칸 없음)에서 줄을 옮긴 첫 실행: 저장소 쪽이 없다고 맥 파일을 지우지 않고 충돌로 멈춘다
	s.editLines(s.MAC+"/.local/state/herdr-sync/repo-base.tsv", func(l string) (string, bool) { // cut -f1,2
		f := strings.SplitN(l, "\t", 3)
		if len(f) > 2 {
			f = f[:2]
		}
		return strings.Join(f, "\t"), true
	})
	s.git(T+"/lane", "pull", "-q")
	s.lane("legacy", sub("skills/agents2/ ", "skills/agents3/ "))
	s.run()
	s.ok("옛 꼴 base + 줄 옮김: 충돌로 멈추고 맥 파일 그대로", s.r.RC == 1 && s.is(A+"/alpha/SKILL.md", "alpha 2\n") && s.has("충돌 저장소 .agents/skills/alpha/SKILL.md (저장소에 없음 — 옛 base라 지운 것인지 모른다)") && !ct.HasPrefixLine(s.r.Out, "맥 지움"))
	s.run("--take", "repo:.agents/skills/**")
	s.ok("옛 꼴 base 충돌: --take repo:<폴더>/**로 새 자리에 모은다", s.r.RC == 0 && s.corigin("show", "main:skills/agents3/alpha/SKILL.md") == "alpha 2" && s.is(A+"/alpha/SKILL.md", "alpha 2\n"))

	// 15-12. 모으기 실패 되돌리기는 이번에 쓴 경로만: 그 사이 남이 같은 저장소에 쓴 새 파일·고친 줄은 남는다
	s.putx(T+"/conf/.git/hooks/pre-commit", "#!/bin/sh\necho other > other-agent.txt\necho \"edit by other\" >> private-patterns.txt\nexit 1\n")
	s.put(A+"/alpha/SKILL.md", "alpha 3\n")
	s.put(A+"/eta/new.md", "eta\n")
	s.run()
	st := ct.Lines(ct.Git(T+"/conf", "status", "--porcelain", "--untracked-files=all"))
	sort.Strings(st)
	s.ok("되돌리기 범위: 남의 새 파일·남의 줄은 남고, 이번에 쓴 것만 되돌린다", s.r.RC == 1 && ct.Exists(T+"/conf/other-agent.txt") && ct.HasLine(ct.Read(T+"/conf/private-patterns.txt"), "edit by other") && strings.Join(st, "|")+"|" == " M private-patterns.txt|?? other-agent.txt|")
	os.Remove(T + "/conf/.git/hooks/pre-commit")
	os.Remove(T + "/conf/other-agent.txt")
	s.git(T+"/conf", "checkout", "-q", "--", "private-patterns.txt")
	s.run()
	s.ok("되돌린 뒤 다시: 모은다", s.r.RC == 0 && s.corigin("show", "main:skills/agents3/eta/new.md") == "eta")
}
