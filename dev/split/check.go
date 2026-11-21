package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// check: 기준 rev의 원본과 지금 작업 트리를 대조(hc-split.py check). 돌려주는 값이 종료 코드
func check(rev string) int {
	bad := 0
	for _, p := range readPlan() {
		src := gitShow(rev, p.script)
		g := generate(p, src)
		tool := filepath.Base(p.script)
		// 3) 본체
		cur := readText(filepath.Join(root, p.script))
		if cur != join(g.main) {
			fmt.Printf("FAIL %s 본체가 원본-덩이+로더와 다르다\n", p.script)
			bad = 1
		}
		if !parses(cur) {
			fmt.Printf("FAIL %s bash -n\n", p.script)
			bad = 1
		}
		// lib 폴더에 계획 밖 파일이 없어야
		d := filepath.Join(root, p.lib)
		ents, err := os.ReadDir(d)
		if err != nil {
			die(fmt.Sprintf("lib 폴더 읽기: %v", err))
		}
		var extra []string
		for _, e := range ents {
			if _, ok := g.libs[strings.TrimSuffix(e.Name(), ".sh")]; !ok || !strings.HasSuffix(e.Name(), ".sh") {
				extra = append(extra, e.Name())
			}
		}
		if len(extra) > 0 {
			sort.Strings(extra)
			fmt.Printf("FAIL %s 계획 밖 파일: %s\n", p.lib, reprList(extra))
			bad = 1
		}
		allsrc := ""
		where := map[string][]string{}
		var whereOrder []string
		for _, m := range g.order {
			want := g.libs[m]
			got := readText(filepath.Join(d, m+".sh"))
			if got != want {
				fmt.Printf("FAIL %s/%s.sh 기대와 다르다\n", p.lib, m)
				bad = 1
			}
			if !parses(got) {
				fmt.Printf("FAIL %s/%s.sh bash -n\n", p.lib, m)
				bad = 1
			}
			// 1)·2) lib를 다시 덩이로 나눠 견준다: 덩이 말고는 머리 주석·빈 줄뿐, 순서·글자 그대로
			gl := splitlines(got)
			gb := blocks(gl, fmt.Sprintf("%s/%s.sh", p.lib, m))
			used := map[int]bool{}
			for _, b := range gb {
				for k := b.s; k <= b.e; k++ {
					used[k] = true
				}
			}
			var rest []string
			for k, x := range gl {
				if !used[k] && strip(x) != "" && k != 0 {
					rest = append(rest, x)
				}
			}
			if len(rest) > 0 {
				fmt.Printf("FAIL %s/%s.sh 덩이 밖 줄: %s\n", p.lib, m, reprList(rest[:min(3, len(rest))]))
				bad = 1
			}
			var exp []block
			for j, b := range g.bl {
				if g.mods[j] == m {
					exp = append(exp, b)
				}
			}
			if !sameNames(gb, exp) {
				fmt.Printf("FAIL %s/%s.sh 순서·목록 다름\n", p.lib, m)
				bad = 1
			}
			for j := 0; j < len(gb) && j < len(exp); j++ {
				if join(gl[gb[j].s:gb[j].e+1]) != join(g.lines[exp[j].s:exp[j].e+1]) {
					fmt.Printf("FAIL %s/%s.sh %s 글자 다름\n", p.lib, m, gb[j].name)
					bad = 1
				}
			}
			for _, b := range gb {
				if _, ok := where[b.name]; !ok {
					whereOrder = append(whereOrder, b.name)
				}
				where[b.name] = append(where[b.name], m)
			}
			allsrc += got
		}
		names := map[string]bool{}
		var once []string
		for _, b := range g.bl {
			names[b.name] = true
			if len(where[b.name]) != 1 {
				once = append(once, b.name)
			}
		}
		var stray []string
		for _, n := range whereOrder {
			if !names[n] {
				stray = append(stray, n)
			}
		}
		sort.Strings(stray)
		if len(once) > 0 || len(stray) > 0 {
			fmt.Printf("FAIL %s 정확히 한 번이 아닌 함수: %s / 원본에 없던 함수: %s\n", tool, reprList(once), reprList(stray))
			bad = 1
		}
		// 4) bash가 본 정의 그대로
		var orig strings.Builder
		for _, b := range g.bl {
			orig.WriteString(join(g.lines[b.s : b.e+1]))
		}
		origDefs := defined(orig.String(), true) // 파이썬 판과 같은 차례(원본 먼저 — source 실패 문구가 같게)
		if defined(allsrc, true) != origDefs {
			fmt.Printf("FAIL %s declare -f 다름\n", tool)
			bad = 1
		}
		// 5) 앞당겨지는 정의
		for _, b := range g.bl {
			pat := regexp.MustCompile(`(^|[;&|({` + "`" + `]|\$\(|then|do|else|\|\||&&)` + pyS + `*` + regexp.QuoteMeta(b.name) + `(` + pyS + `|;|$|\))`)
			var hits []int
			for n := 0; n < b.s; n++ {
				if pat.MatchString(g.lines[n]) && !inBlock(g.bl, n) {
					hits = append(hits, n+1)
				}
			}
			if len(hits) > 0 {
				fmt.Printf("알림 %s: %s()는 %d줄에 정의되는데 그 앞 최상위 줄 %s에 명령 낱말로 나온다\n", tool, b.name, b.i+1, intList(hits[:min(8, len(hits))]))
			}
		}
		state := "통과"
		if bad != 0 {
			state = "실패"
		}
		fmt.Printf("%s %s: 함수 %d개, 모듈 %s, 본체 %d줄\n", state, tool, len(g.bl), strings.Join(g.order, ","), len(g.main))
	}
	return bad
}

func sameNames(a, b []block) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name {
			return false
		}
	}
	return true
}

func inBlock(bl []block, n int) bool {
	for _, b := range bl {
		if b.s <= n && n <= b.e {
			return true
		}
	}
	return false
}
