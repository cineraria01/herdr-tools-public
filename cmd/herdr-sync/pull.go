package main

// --pull <기계>:… 로 맥에 가져오기, 범용 저장소로 모을 값의 개인 값 검사

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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
			ok, bad := pullTree(m, pl)
			for _, k := range ok {
				say("맥 가져옴 %s (%s)", k, m)
				got++
			}
			for _, r := range bad {
				warn("%s에서 %s를 가져오지 못했다 (%s)", m, r[0], r[1])
			}
			if len(bad) > 0 {
				exit(1)
			}
		}
		if got == 0 {
			warn("%s에 가져올 것이 없다", s)
		}
	}
}

// pullTree: 나무 파일 묶음(pl: 「키 x 서명 키」 줄)을 그 기계에서 받아 맥에 쓴다. 맥에 쓴 키와 (키, 까닭).
// 두 단계(ops/R-217): 묶음마다 먼저 맥 임시 폴더에 다 받고(원격 T | 로컬 tar -x — 양쪽 종료 0일 때만), 그 뒤에 그 폴더에서 맥 W로 넣는다.
// 받기가 한쪽이라도 실패면 그 묶음은 맥에 하나도 쓰지 않는다(맥·개인 저장소·base 보존 — 부르는 쪽이 종료 1로 멈춘다)
func pullTree(m string, pl []string) (ok []string, bad [][2]string) {
	fail := func(ch []string, why string) {
		for _, l := range ch {
			bad = append(bad, [2]string{field(l, 1), why})
		}
	}
	for n, ch := range chunks(pl, 3) {
		d, err := os.MkdirTemp(TMP, "pull.")
		if err != nil {
			fail(ch, "임시 폴더를 만들지 못했다")
			continue
		}
		var keys []string
		for _, l := range ch {
			keys = append(keys, field(l, 1))
		}
		t := on(m, true, "T")
		t.Stdin = bytes.NewReader(joinLines(keys))
		x := exec.Command("tar", "-xf", "-")
		x.Dir, x.Stderr = d, os.Stderr
		if p, c := pipeline(nil, t, x, nil); p != 0 || c != 0 {
			fail(ch, fmt.Sprintf("받기가 실패로 끝났다 — 보냄 %d · 받음 %d", p, c))
			continue
		}
		var wo bytes.Buffer
		pre, tar := wpack(d, ch, fmt.Sprintf("pull%d.src", n))
		p, c := pipeline(pre, tar, macCmd("-", "W", STAMP), &wo)
		for _, r := range wres(ch, wo.String()) {
			if r[0] == "ok" { // W가 cksum을 맞춘 뒤 넣었다고 알린 것 — 넣은 것은 그대로 알린다
				ok = append(ok, r[1])
			} else {
				bad = append(bad, [2]string{r[1], r[2]})
			}
		}
		if p != 0 || c != 0 { // 다 넣었다고 알렸어도 묶음이 실패로 끝났으면 성공으로 치지 않는다(base를 못 적었을 수 있다)
			bad = append(bad, [2]string{field(ch[0], 1), fmt.Sprintf("맥 넣기 묶음이 실패로 끝났다 — 보냄 %d · 받음 %d", p, c)})
		}
	}
	return ok, bad
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
