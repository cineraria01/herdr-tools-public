package main

// herdr-tools-publish 계약 시험 — Go 전환 감사(2026-11-21) 지적 고침:
//   A02 결과 파일(HERDR_TOOLS_PUBLISH_RESULT): 게시 성공(내보냄·같음)일 때만 검사한 비공개 커밋 전체 sha 한 줄, 그 밖은 없음(있던 것은 지움)
//   A12 성공·실패 출력에 publish.repo 주소 원문을 싣지 않는다(git 오류 글 포함) · L02 남는 인자는 사용법·종료 2
//   (Go 판만 — 셸 판은 곧 걷는 알려진 차이) B04 객체 전송 상한 · B12 작성자 비밀 꼴 · B13 막힌 미리보기는 변경 목록 없이 종료 ·
//   B14 diff-tree 실패는 종료 1 · B16 push 뒤 로컬 ref 실패를 따로 알리고 다음 실행에서 원격 값으로 복구

import (
	"os"
	"os/exec"
	"strings"
	"time"

	ct "herdr-tools/internal/clitest"
)

func fixContract(f *pubFix) {
	t, T, c := f.t, f.T, f.c
	put := func(p, s string) { ct.Put(t, p, s) }
	url := T + "/public.git"
	res := T + "/res/out"
	os.MkdirAll(T+"/res", 0o755)
	renv := append(append([]string(nil), f.unAuthor...), "HERDR_TOOLS_PUBLISH_RESULT="+res)
	stale := func() { put(res, "stale\n") }
	full := func() string { return ct.Git(T+"/priv", "rev-parse", "refs/remotes/origin/main") }

	// A02
	put(T+"/priv/docs/a02.md", "a02\n")
	f.commit("a02")
	stale()
	f.runEnv(renv)
	c.Ok("결과 파일: 내보냄이면 검사한 비공개 커밋 전체 sha 한 줄", f.r.RC == 0 && f.has("내보냄") && len(full()) == 40 && ct.Read(res) == full()+"\n")
	c.Ok("내보냄 줄에 주소 원문 없음(설정 키 이름만)", f.has("publish.repo") && !f.has(url))
	stale()
	f.runEnv(renv)
	c.Ok("결과 파일: 같음이어도 그 sha", f.r.RC == 0 && f.has("같음") && ct.Read(res) == full()+"\n")
	put(T+"/priv/docs/a02.md", "a02-dry\n")
	f.commit("a02")
	stale()
	f.runEnv(renv, "--dry-run")
	c.Ok("결과 파일: --dry-run이면 없음(있던 것은 지움)", f.r.RC == 0 && !ct.Lexists(res))
	h := f.pmain()
	put(T+"/priv/docs/a02.md", "secret-word\n")
	f.commit("a02")
	stale()
	f.runEnv(renv)
	c.Ok("결과 파일: 막히면 없음", f.r.RC == 1 && !ct.Lexists(res) && f.pmain() == h)
	put(T+"/priv/docs/a02.md", "a02-2\n")
	f.commit("a02")

	// A12: push 거절 — git 오류 글의 주소도 가린다
	hook := url + "/hooks/pre-receive"
	put(hook, "#!/bin/sh\necho rejected-by-test >&2\nexit 1\n")
	ct.Chmod(t, hook, 0o755)
	stale()
	f.runEnv(renv)
	c.Ok("push 실패: 종료 1, 주소 원문 없음, 결과 파일 없음", f.r.RC == 1 && f.has("push 실패") && !f.has(url) && !ct.Lexists(res) && f.pmain() == h)
	os.Remove(hook)
	f.run()
	c.Ok("거절을 뺀 뒤: 내보냄", f.r.RC == 0 && f.has("내보냄"))
	f.setConf("publish.repo", "publish.repo "+T+"/nowhere.git")
	f.setConf("publish.dir", "publish.dir "+T+"/fresh-a12")
	f.run()
	c.Ok("공개 저장소를 못 읽음: 종료 1, 주소 원문 없음", f.r.RC == 1 && f.has("공개 저장소를 읽지 못했다") && !f.has(T+"/nowhere.git"))
	f.setConf("publish.repo", "publish.repo "+url)
	f.setConf("publish.dir", "publish.dir "+T+"/fresh-dir")

	// L02
	h = f.pmain()
	f.run("--dry-run", "extra")
	c.Ok("남는 인자: 사용법·종료 2", f.r.RC == 2 && f.pmain() == h)

	if os.Getenv("HERDR_TOOLS_PUBLISH_IMPL") != "go" {
		return // 아래는 셸 판에도 있는 옛 결함 — Go 판만 고쳤다(셸 판은 곧 걷는다, README 알려진 차이)
	}
	// B13: 막힌 미리보기는 가린 진단 뒤 바로 끝 — 변경 목록(원래 경로)을 찍지 않는다
	name := "docs/" + a16 + b16 + "-dry.md"
	put(T+"/priv/"+name, "x\n")
	f.commit("b13")
	f.run("--dry-run")
	c.Ok("막힌 --dry-run: 종료 1, 비밀 꼴 이름을 안 찍음", f.r.RC == 1 && !f.has(a16+b16))
	f.priv("rm", "-q", name)
	f.commit("b13")

	// B12: 작성자·커미터에도 비밀 꼴 검사, 값은 안 찍음
	f.setConf("publish.author", "publish.author Pub Bot <"+a16+b16+"@public.example>")
	put(T+"/priv/docs/b12.md", "b12\n")
	f.commit("b12")
	f.run()
	c.Ok("작성자의 비밀 꼴: 종료 1, 값 안 찍음, push 없음", f.r.RC == 1 && f.has("작성자") && !f.has(a16+b16) && f.pmain() == h)
	f.setConf("publish.author", "publish.author  Pub Bot <bot@public.example>")

	// 가짜 git: fault/<하위 명령> 파일이 있으면 diff-tree·update-ref는 128, pack-objects는 늦춘다
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	put(T+"/fake/git", "#!/bin/sh\nfor a; do case $a in\n  diff-tree|update-ref) [ ! -f '"+T+"/fault/'$a ] || exit 128 ;;\n  pack-objects) [ ! -f '"+T+"/fault/pack-objects' ] || sleep 40 ;;\nesac; done\nexec '"+real+"' \"$@\"\n")
	ct.Chmod(t, T+"/fake/git", 0o755)
	os.MkdirAll(T+"/fault", 0o755)
	fenv := append(append([]string(nil), f.unAuthor...), "PATH="+T+"/fake:"+os.Getenv("PATH"))
	fault := func(s string) { put(T+"/fault/"+s, "") }

	// B14
	fault("diff-tree")
	f.runEnv(fenv, "--dry-run")
	c.Ok("diff-tree 실패: 종료 1과 실패 단계", f.r.RC == 1 && f.has("변경 목록") && !f.has("--dry-run: push 안 함"))
	os.Remove(T + "/fault/diff-tree")

	// B16
	local := func() string {
		return ct.Git(T, "--git-dir="+T+"/fresh-dir", "rev-parse", "-q", "--verify", "refs/heads/main")
	}
	put(T+"/priv/docs/b16.md", "b16\n")
	f.commit("b16")
	fault("update-ref")
	f.runEnv(fenv)
	c.Ok("push 뒤 로컬 ref 실패: 종료 0(원격 결과), 따로 경고", f.r.RC == 0 && f.has("내보냄") && f.has("로컬 refs/heads/main") && f.pmain() != h && local() != f.pmain())
	os.Remove(T + "/fault/update-ref")
	f.run()
	c.Ok("다음 실행: 같음, 로컬 ref를 원격 값으로 복구", f.r.RC == 0 && f.has("같음") && local() == f.pmain())

	// B04
	h = f.pmain()
	put(T+"/priv/docs/b04.md", "b04\n")
	f.commit("b04")
	fault("pack-objects")
	t0 := time.Now()
	f.runEnv(append(fenv, "HERDR_TIMEOUT_GIT=5"))
	el := time.Since(t0)
	c.Ok("객체 전송 상한: 종료 1(모름), 시간 넘김, push 없음, 늦지 않게", f.r.RC == 1 && f.has("시간 넘김") && f.pmain() == h && el < 30*time.Second)
	os.Remove(T + "/fault/pack-objects")
	f.run()
	c.Ok("상한을 뺀 뒤: 내보냄", f.r.RC == 0 && f.pubTree() == f.privTree() && !strings.Contains(f.r.Out, "시간 넘김"))
}
