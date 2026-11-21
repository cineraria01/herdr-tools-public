package main

// fetch: hc-lane fetch [--on <기계>] --repo <저장소> [<참조…>] — 그 기계의 그 저장소에서 git fetch origin(로그인 셸, 참조 잠금 경합이면 다시)

import (
	"fmt"
	"os"
	"strings"
)

func init() { register("fetch", fetchMain, can("fetch")) }

func fetchMain(a []string) int {
	o := mustParse("fetch", a)
	w := repo
	if on != "" {
		w = on + ":" + repo
	}
	argv := zxArgs(repo, append([]string{"git", "fetch", "origin"}, o.rest...)...)
	if on == "" && core == "" {
		// 이 기계: 그 폴더에서 바로(셸 판 zx의 cd → exec와 같음)
		if st, err := os.Stat(repo); err != nil || !st.IsDir() {
			eprintf("sh: cd: %s: No such file or directory\n", repo)
			die("fetch 실패: %s (위 git 출력을 본다)", w)
		}
		argv = append([]string{"git", "-C", repo, "fetch", "origin"}, o.rest...)
	}
	if fetchrCmd(os.Stdout, os.Stderr, argv...) != 0 {
		die("fetch 실패: %s (위 git 출력을 본다)", w)
	}
	r := ""
	if len(o.rest) > 0 {
		r = " " + strings.Join(o.rest, " ")
	}
	fmt.Printf("hc-lane fetch: 받음 %s origin%s\n", w, r)
	return 0
}
