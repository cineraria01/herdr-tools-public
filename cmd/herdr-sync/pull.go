package main

// --pull <기계>:… 로 맥에 가져오기, 범용 저장소로 모을 값의 개인 값 검사

import (
	"bytes"
	"io"
	"os"
	"strings"

	"herdr-tools/internal/toolcfg"
)

// doPulls: --pull <기계>:<경로|*|<뿌리>/**> — 그 기계 값을 맥 실제 파일로 가져온다 (저장소 쪽은 repoSync가 한다).
// 나무 파일은 그 기계에서 tar 묶음으로 받아 맥에 한 번에 쓴다
func doPulls() {
	for _, s := range pulls {
		m, k := before(s), after(s)
		if strings.HasSuffix(k, "#codex-shared") {
			die("Codex 구역은 맥 지침에서 다시 만드므로 --pull을 받지 않는다. --take를 쓴다: %s", s)
		}
		if m == "repo" {
			continue
		}
		if k != "*" && !unitKey(k) && !treespec(k) {
			die("설정 키는 --pull을 받지 않는다. 맥 settings.json을 고친 뒤 --take를 쓴다: %s", s)
		}
		mq, ok := query("")
		if !ok {
			die("맥 값을 읽지 못했다")
		}
		q, ok := query(m)
		if !ok {
			if !q.nosh {
				die("%s 연결 실패", m)
			}
			warn("%s 건너뜀 (sh 없음 — Windows 등)", s)
			continue
		}
		if q.O != "Darwin" && q.O != "Linux" { // 그 기계 값을 맥에 쓰지 않는다
			warn("%s 건너뜀 (OS %s)", s, orUnknown(q.O))
			continue
		}
		got := 0
		var pl []string
		for i, u := range U {
			c := q.C[i]
			if !targets(u.t, m, q.O) || strings.HasSuffix(u.k, "@") || strings.HasSuffix(u.k, "#codex-shared") {
				continue
			}
			switch c {
			case "-", "!", "~", "?", "L", "G":
				continue
			}
			if mq.C[i] == "L" || mq.C[i] == "G" {
				continue
			}
			if k == "*" || strings.HasSuffix(k, "/**") {
				if !(spec([]string{s}, m, u.k) && mq.C[i] != "-" && c != mq.C[i] && c != q.B[i]) { // 표에 충돌로 보인 것만
					continue
				}
			} else if k != u.k {
				continue
			}
			if u.tree {
				pl = append(pl, u.k+"\t"+sigx(c)+"\t"+c+"\t"+u.k)
				continue
			}
			pf := TMP + "/pull"
			f, err := os.Create(pf)
			if err != nil {
				die("임시 파일을 쓰지 못했다: %s", pf)
			}
			rr := on(m, false, "r", u.k)
			rr.Stdout = f
			r1 := run(rr)
			f.Close()
			if r1 != 0 {
				die("%s에서 %s를 가져오지 못했다", m, u.k)
			}
			in, _ := os.Open(pf)
			r2 := runIn(macCmd("-", "w", u.k, sigx(c), c, STAMP), in, nil)
			in.Close()
			if r2 != 0 {
				die("%s에서 %s를 가져오지 못했다", m, u.k)
			}
			say("맥 가져옴 %s (%s)", u.k, m)
			got++
		}
		if len(pl) > 0 {
			var wo bytes.Buffer
			for _, ch := range chunks(pl, 3) {
				var keys []string
				for _, l := range ch {
					keys = append(keys, field(l, 1))
				}
				t := on(m, true, "T")
				t.Stdin = bytes.NewReader(joinLines(keys))
				pipeline(append(joinLines(ch), ".\n"...), t, macCmd("-", "W", STAMP), &wo)
			}
			bad := false
			for _, r := range wres(pl, wo.String()) {
				if r[0] == "ok" {
					say("맥 가져옴 %s (%s)", r[1], m)
					got++
				} else {
					warn("%s에서 %s를 가져오지 못했다 (%s)", m, r[1], r[2])
					bad = true
				}
			}
			if bad {
				exit(1)
			}
		}
		if got == 0 {
			warn("%s에 가져올 것이 없다", s)
		}
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "알 수 없음"
	}
	return s
}

// leak: 범용 저장소로 모을 맥 값이 개인 값 검사에 걸리면(또는 검사를 못 하면) 참과 까닭
func leak(i int) (bool, string) {
	if U[i].d != REPO {
		return false, ""
	}
	if !toolcfg.HasPatterns(CONF + "/private-patterns.txt") { // 검사를 못 하면 모으지 않는다(fail-closed)
		return true, "개인 값 검사 못 함 — " + CONF + "/private-patterns.txt에 패턴이 없다"
	}
	p := TMP + "/leak"
	f, err := os.Create(p)
	if err != nil {
		return true, "개인 값 검사 못 함"
	}
	c := mac("r", U[i].k)
	c.Stdout = f
	r := run(c)
	f.Close()
	if r != 0 {
		return true, "개인 값 검사 못 함"
	}
	return toolcfg.Lint(CONF, false, []string{p}, io.Discard, io.Discard) != 0, "개인 값"
}

// tleaks: 범용 저장소로 모을 수 있는 나무 파일을 개인 값 검사에 한 번에 넣는다. 걸린 단위 → 까닭
func tleaks(q *qres, RS []string) map[int]string {
	LK := map[int]string{}
	var idx []int
	var files []string
	for i, u := range U {
		if !u.tree || u.d != REPO {
			continue
		}
		switch q.C[i] {
		case "-", "L", "G":
			continue
		}
		if q.C[i] != RS[i] {
			idx = append(idx, i)
			files = append(files, u.k)
		}
	}
	if len(files) == 0 {
		return LK
	}
	why := "개인 값 검사 못 함 — " + CONF + "/private-patterns.txt에 패턴이 없다"
	var lo bytes.Buffer
	if toolcfg.HasPatterns(CONF + "/private-patterns.txt") {
		switch toolcfg.LintAt(home, CONF, false, files, &lo, io.Discard) {
		case 0:
			return LK
		case 1:
			why = "개인 값"
		default:
			why = "개인 값 검사 못 함"
		}
	}
	hit := splitLines(lo.String())
	for _, i := range idx {
		if why == "개인 값" { // 걸린 줄은 「키:줄: 패턴」
			found := false
			for _, h := range hit {
				if strings.HasPrefix(h, U[i].k+":") {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		LK[i] = why
	}
	return LK
}
