package main

// guard: 클론의 공용 git 폴더 hooks/pre-push에 관문(셸 lib/hc-lane/guard.sh). 표지 줄로 자기가 넣은 것을 알아보고, 남의 훅은
// 건드리지 않는다. 설정·원격 조회가 실패하면 설치·상태를 말하지 않고 멈춘다(마지막 감사 guard.sh:8 — 127aa80).
// 종료: status 0 깔림·1 없음(또는 hooksPath 때문에 안 불림)·3 남의 훅, install·remove 0, 멈춤 1

import (
	"fmt"
	"os"
	"strings"
)

func init() {
	register("guard", guardMain, can("guard"))
	argSpecs["guard"] = argSpec{
		head: func(a []string) bool { return len(a) > 0 }, // install|status|remove
		check: func(o *opts, rds bool) *parseErr {
			if o.head != "install" && o.head != "status" && o.head != "remove" {
				return &parseErr{usage: true}
			}
			if o.repo == "" {
				return &parseErr{usage: true}
			}
			if o.on+o.name != "" || !rds || len(o.rest) > 0 {
				return die1("guard에는 --repo(와 --branch <기본 브랜치>)만 쓴다. 다른 기계는 hc exec --on <기계> -- hc-lane guard …")
			}
			return nil
		},
	}
}

const gmark = "# hc-lane guard v1"

func guardMain(a []string) int {
	o := mustParse("guard", a)
	return guardCmd(o.head, o.branch)
}

func guardCmd(act, branch string) int {
	common, rc := gitOutQ("-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if rc != 0 {
		die("guard: git 저장소가 아니다: %s", repo)
	}
	hp := ""
	if act != "remove" {
		var rc int
		hp, rc = gitOut("-C", repo, "config", "--get", "core.hooksPath")
		if rc != 0 && rc != 1 {
			die("guard: core.hooksPath 조회 실패")
		}
		remotes, rc := gitOut("-C", repo, "remote")
		if rc != 0 {
			die("guard: 원격 목록 조회 실패")
		}
		if remotes == "" {
			fmt.Printf("guard: 원격이 없는 저장소라 push가 없다. 설치할 것 없음(착지는 hc-lane land가 로컬 <기본>을 fast-forward로 올린다): %s\n", repo)
			return 0
		}
	}
	f := common + "/hooks/pre-push"
	st := "none"
	if _, err := os.Lstat(f); err == nil {
		st = "other"
		if b, err := os.ReadFile(f); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if l == gmark {
					st = "ours"
					break
				}
			}
		}
	}
	switch act {
	case "status":
		switch st {
		case "ours":
			var bs []string
			if b, err := os.ReadFile(f); err == nil {
				for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
					if v, ok := strings.CutPrefix(l, "base="); ok {
						bs = append(bs, v)
					}
				}
			}
			if hp != "" {
				fmt.Printf("guard: 깔려 있지만 core.hooksPath=%s 라 git이 부르지 않는다: %s\n", hp, f)
				return 1
			}
			fmt.Printf("guard: 깔림(기본 브랜치 %s): %s\n", strings.Join(bs, "\n"), f)
			return 0
		case "none":
			fmt.Printf("guard: 없음: %s\n", f)
			return 1
		default:
			fmt.Printf("guard: 다른 pre-push 훅이 있다(hc-lane 관문 아님): %s\n", f)
			return 3
		}
	case "install":
		if hp != "" {
			die("guard: core.hooksPath=%s 라 %s 는 git이 부르지 않는다. 건드리지 않았다(그 훅 폴더에 연결할지 지휘가 정한다)", hp, f)
		}
		if st == "other" {
			die("guard: 다른 pre-push 훅이 있다: %s. 덮지 않았다(그 훅에 관문을 이을지 지휘가 정한다)", f)
		}
		b := branch
		if b == "" {
			var ok bool
			if b, ok = originBase(repo); !ok {
				die("guard: 기본 브랜치 조회 실패")
			}
		}
		if !plain(b) {
			die("guard: 기본 브랜치 이름에 쓸 수 없는 글자가 있다: %s", b)
		}
		if os.MkdirAll(common+"/hooks", 0o777) != nil {
			die("guard: 훅 폴더를 만들지 못했다: %s", common+"/hooks")
		}
		tmp := fmt.Sprintf("%s.hc-lane.%d", f, os.Getpid())
		if os.WriteFile(tmp, []byte(guardHook(b)), 0o666) != nil {
			die("guard: 쓰지 못했다: %s", tmp)
		}
		// chmod +x: umask가 막지 않는 실행 비트를 더한다
		um := umask()
		fi, err := os.Stat(tmp)
		if err == nil {
			err = os.Chmod(tmp, fi.Mode().Perm()|(0o111&^os.FileMode(um)))
		}
		if err == nil {
			err = os.Rename(tmp, f)
		}
		if err != nil {
			os.Remove(tmp)
			die("guard: 넣지 못했다: %s", f)
		}
		if st == "ours" {
			fmt.Printf("guard: 새로 씀(기본 브랜치 %s): %s\n", b, f)
		} else {
			fmt.Printf("guard: 깔았다(기본 브랜치 %s): %s\n", b, f)
		}
	case "remove":
		switch st {
		case "ours":
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				die("guard: 지우지 못했다: %s", f)
			}
			fmt.Printf("guard: 지웠다: %s\n", f)
		case "none":
			fmt.Printf("guard: 지울 것 없음: %s\n", f)
		default:
			die("guard: hc-lane 관문이 아닌 pre-push 훅이라 지우지 않았다: %s", f)
		}
	}
	return 0
}

// guardHook: pre-push 관문 본문(셸 판과 바이트까지 같게 — 이미 깔린 훅과 새 훅이 같은 글이어야 status·다시 깔기가 같게 읽는다)
func guardHook(b string) string {
	return `#!/bin/sh
` + gmark + `
# hc-lane guard install이 넣었다(지우기: hc-lane guard remove --repo <경로>). refs/heads/<기본>으로 가는 push는
# 착지 줄(hc-lane land, .claude/herdr-land.sh)이 주는 HERDR_LAND=1일 때만 받는다. 다른 브랜치·참조는 통과.
base=` + b + `
[ "${HERDR_LAND-}" = 1 ] && exit 0
while read -r lref lsha rref rsha; do
  [ "$rref" = "refs/heads/$base" ] || continue
  cat >/dev/null
  echo "push 막힘: $base 로 가는 push는 착지 줄로만 한다." >&2
  echo "새 착지 줄: hc-lane land $base" >&2
  echo "(사람이 손으로 넣을 때만 HERDR_LAND=1 git push … 로 명시)" >&2
  exit 1
done
exit 0
`
}
